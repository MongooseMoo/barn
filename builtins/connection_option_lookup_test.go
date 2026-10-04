package builtins

import (
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestSingleConnectionOptionReadsPreserveValues(t *testing.T) {
	ctx := ctxWithConnManager(&stubConnManager{conn: &stubConn{}})
	ctx.IsWizard = true
	expected := map[string]types.Value{
		"binary": types.NewInt(0), "hold-input": types.NewInt(0),
		"client-echo": types.NewInt(1), "disable-oob": types.NewInt(0),
		"keep-alive": types.NewInt(0), "flush-command": types.NewStr(""),
		"intrinsic-commands": types.NewList([]types.Value{
			types.NewStr(".program"), types.NewStr("PREFIX"), types.NewStr("SUFFIX"),
			types.NewStr("OUTPUTPREFIX"), types.NewStr("OUTPUTSUFFIX"),
		}),
	}
	for _, stored := range []bool{false, true} {
		if stored {
			ctx.Session.setConnectionOption(7, "binary", types.NewInt(1))
			ctx.Session.setConnectionOption(7, "flush-command", types.NewStr(".flush"))
			expected["binary"] = types.NewInt(1)
			expected["flush-command"] = types.NewStr(".flush")
		}
		for name, want := range expected {
			args := []types.Value{types.NewObj(7), types.NewStr(name)}
			for _, got := range []types.Result{builtinConnectionOption(ctx, args), builtinConnectionOptions(ctx, args)} {
				if got.IsError() || !got.Val.Equal(want) {
					t.Fatalf("stored=%v option %q = %+v, want %v", stored, name, got, want)
				}
			}
			if got := ctx.Session.ConnectionOptionTruthy(7, name); got != want.Truthy() {
				t.Fatalf("stored=%v option %q truthiness=%v", stored, name, got)
			}
		}
		args := []types.Value{types.NewObj(7), types.NewStr("missing")}
		for _, got := range []types.Result{builtinConnectionOption(ctx, args), builtinConnectionOptions(ctx, args)} {
			if !got.IsError() || got.Error != types.E_INVARG {
				t.Fatalf("missing option = %+v, want E_INVARG", got)
			}
		}
		if ctx.Session.ConnectionOptionTruthy(7, "missing") {
			t.Fatal("missing option was truthy")
		}
		if ctx.Session.ConnectionOptionTruthy(8, "binary") {
			t.Fatal("another player's defaults inherited this player's override")
		}
	}
}

func TestConnectionOptionSnapshotsRemainIndependent(t *testing.T) {
	first, second := NewSession(NewRegistry(), NoHost()), NewSession(NewRegistry(), NoHost())
	for _, stored := range []bool{false, true} {
		if stored {
			first.setConnectionOption(7, "binary", types.NewInt(1))
		}
		snapshot := first.getConnectionOptions(7)
		snapshot["binary"] = types.NewInt(99)
		delete(snapshot, "client-echo")
		if first.ConnectionOptionTruthy(7, "binary") != stored || !first.ConnectionOptionTruthy(7, "client-echo") {
			t.Fatal("snapshot mutation changed stored options")
		}
		if second.ConnectionOptionTruthy(7, "binary") {
			t.Fatal("option state leaked across sessions")
		}
		if got := first.getConnectionOptions(7)["binary"].Int(); got != int64(map[bool]int{false: 0, true: 1}[stored]) {
			t.Fatalf("snapshot mutation changed binary to %d", got)
		}
	}
}

func TestSingleConnectionOptionConcurrentReadsAndWrites(t *testing.T) {
	ctx := ctxWithConnManager(&stubConnManager{conn: &stubConn{}})
	ctx.IsWizard = true
	ctx.Session.setConnectionOption(7, "binary", types.NewInt(0))
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(3)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 2000; i++ {
			ctx.Session.setConnectionOption(7, "binary", types.NewInt(int64(i&1)))
		}
	}()
	for range 2 {
		go func() {
			defer workers.Done()
			<-start
			args := []types.Value{types.NewObj(7), types.NewStr("binary")}
			for i := 0; i < 2000; i++ {
				_ = ctx.Session.ConnectionOptionTruthy(7, "binary")
				got := builtinConnectionOptions(ctx, args)
				if got.IsError() || got.Val.Type() != types.TYPE_INT || got.Val.Int() < 0 || got.Val.Int() > 1 {
					t.Errorf("concurrent option = %+v", got)
					return
				}
			}
		}()
	}
	close(start)
	workers.Wait()
}

var singleOptionBoolSink bool
var singleOptionResultSink types.Result
var connectionOptionsSnapshotSink map[string]types.Value

func TestSingleConnectionOptionScalarReadsDoNotAllocate(t *testing.T) {
	for _, stored := range []bool{false, true} {
		ctx := ctxWithConnManager(&stubConnManager{conn: &stubConn{}})
		ctx.IsWizard = true
		session := ctx.Session
		if stored {
			session.setConnectionOption(7, "binary", types.NewInt(1))
		}
		for _, name := range []string{"binary", "hold-input", "client-echo", "disable-oob", "keep-alive", "flush-command", "missing"} {
			if allocs := testing.AllocsPerRun(100, func() { singleOptionBoolSink = session.ConnectionOptionTruthy(7, name) }); allocs != 0 {
				t.Errorf("stored=%v option=%s allocs=%g, want zero", stored, name, allocs)
			}
			args := []types.Value{types.NewObj(7), types.NewStr(name)}
			for _, getter := range []struct {
				name string
				call func(*Execution, []types.Value) types.Result
			}{{"connection_option", builtinConnectionOption}, {"connection_options", builtinConnectionOptions}} {
				if allocs := testing.AllocsPerRun(100, func() { singleOptionResultSink = getter.call(ctx, args) }); allocs != 0 {
					t.Errorf("stored=%v %s(%s) allocs=%g, want zero", stored, getter.name, name, allocs)
				}
			}
		}
		if allocs := testing.AllocsPerRun(100, func() { singleOptionBoolSink = session.heldInputEnabled(7) }); allocs != 0 {
			t.Errorf("stored=%v held-input allocs=%g, want zero", stored, allocs)
		}
	}
}

func BenchmarkSingleConnectionOption(b *testing.B) {
	for _, state := range []string{"Default", "Stored"} {
		for _, operation := range []string{"Truthy", "Held", "Snapshot"} {
			b.Run(operation+state, func(b *testing.B) {
				session := NewSession(NewRegistry(), NoHost())
				stored := state == "Stored"
				if stored {
					session.setConnectionOption(7, "binary", types.NewInt(1))
					session.setConnectionOption(7, "hold-input", types.NewInt(1))
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					switch operation {
					case "Truthy":
						singleOptionBoolSink = session.ConnectionOptionTruthy(7, "binary")
					case "Held":
						singleOptionBoolSink = session.heldInputEnabled(7)
					case "Snapshot":
						connectionOptionsSnapshotSink = session.getConnectionOptions(7)
					}
				}
				b.StopTimer()
				if operation == "Snapshot" {
					if len(connectionOptionsSnapshotSink) != 7 || connectionOptionsSnapshotSink["binary"].Truthy() != stored || !connectionOptionsSnapshotSink["client-echo"].Truthy() {
						b.Fatal("snapshot workload changed")
					}
				} else if singleOptionBoolSink != stored {
					b.Fatal("scalar workload changed")
				}
			})
		}
	}
}
