package builtins

import (
	"runtime"
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func respondToSharingExecution(t testing.TB, names []string) *Execution {
	t.Helper()
	ctx := loweredPermsExecution(t)
	verb := dbstore.NewVerb(names, 0, dbstore.VerbRead|dbstore.VerbExecute, dbstore.VerbArgs{}, nil)
	if _, code := ctx.Store.AddVerb(3, verb); code != types.E_NONE {
		t.Fatalf("add verb: %s", code)
	}
	ctx.StoreTxn = ctx.Store.BeginSnapshot(0)
	if code := ctx.StoreTxn.SetObjectFlag(3, dbstore.FlagRead, true); code != types.E_NONE {
		t.Fatalf("stage read flag: %s", code)
	}
	t.Cleanup(func() {
		if ctx.StoreTxn != nil {
			ctx.StoreTxn.Release()
		}
	})
	return ctx
}

func TestRespondToKeepsCanonicalNames(t *testing.T) {
	for _, tc := range []struct {
		name, query, canonical string
		names                  []string
	}{
		{"exact", "tell", "tell", []string{"tell"}},
		{"case", "tell", "Tell", []string{"Tell"}},
		{"wildcard", "tell", "te*ll", []string{"te*ll"}},
		{"aliases", "tell", "tell speak", []string{"tell", "speak"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := respondToSharingExecution(t, tc.names)
			name := types.NewStr(tc.query[:1]).StrAppend(types.NewStr(tc.query[1:]))
			result := admitTestBuiltin("respond_to", ctx, []types.Value{types.NewObj(3), name})
			if !result.IsNormal() || result.Val.Type() != types.TYPE_LIST || result.Val.Len() != 2 {
				t.Fatalf("respond_to = %+v, want defining object and canonical name", result)
			}
			got := result.Val.Get(2)
			if result.Val.Get(1).Obj() != 3 || got.Str() != tc.canonical {
				t.Fatalf("respond_to = %v, want {#3, %q}", result.Val, tc.canonical)
			}
			again := admitTestBuiltin("respond_to", ctx, []types.Value{types.NewObj(3), name})
			if !again.IsNormal() || again.Val.Get(2) != got {
				t.Fatal("repeated canonical name allocated another representation")
			}
			// Appending to the response must preserve both existing values.
			appended := got.StrAppend(types.NewStr("!"))
			if appended.Str() != tc.canonical+"!" || got.Str() != tc.canonical || name.Str() != tc.query {
				t.Fatal("appending changed a shared string")
			}
		})
	}
}

func TestRespondToDoesNotRetainLargeInputBacking(t *testing.T) {
	ctx := respondToSharingExecution(t, []string{"tell"})
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	result := func() types.Value {
		large := strings.Repeat("x", 16<<20) + "tell"
		// VM string slicing also constructs a Value from a substring.
		name := types.NewStr(large[len(large)-4:])
		return admitTestBuiltin("respond_to", ctx, []types.Value{types.NewObj(3), name}).Val
	}()
	if result.Type() != types.TYPE_LIST || result.Get(2).Str() != "tell" {
		t.Fatalf("respond_to = %v, want canonical tell response", result)
	}
	// Drop the lookup's memo as a finished task does; measure the response only.
	ctx.StoreTxn.Release()
	ctx.StoreTxn = nil
	ctx.Store = nil
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(result)
	retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("retained bytes with only the response alive: %d", retained)
	if retained > 1<<20 {
		t.Fatalf("short canonical response retained the large input backing: %d bytes", retained)
	}
}

func BenchmarkRespondToCanonicalName(b *testing.B) {
	for _, tc := range []struct {
		name, query, canonical string
		names                  []string
	}{
		{"exact", "tell", "tell", []string{"tell"}},
		{"case", "tell", "Tell", []string{"Tell"}},
		{"wildcard", "tell", "te*ll", []string{"te*ll"}},
		{"aliases", "tell", "tell speak", []string{"tell", "speak"}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			ctx := respondToSharingExecution(b, tc.names)
			args := []types.Value{types.NewObj(3), types.NewStr(tc.query)}
			b.ReportAllocs()
			for b.Loop() {
				result := admitTestBuiltin("respond_to", ctx, args)
				if !result.IsNormal() || result.Val.Get(2).Str() != tc.canonical {
					b.Fatalf("respond_to = %+v", result)
				}
			}
		})
	}
}
