package vm

import (
	"bytes"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/internal/mooprof"
	"github.com/google/pprof/profile"
)

func startProfile(t *testing.T) *mooprof.Session {
	t.Helper()
	session, err := mooprof.Start()
	if err != nil {
		t.Fatalf("start profile: %v", err)
	}
	t.Cleanup(func() { session.Stop() })
	return session
}

// The ticks billed across a session add up to the ticks the VM charged in it,
// and the encoded profile is one go tool pprof can read.
func TestProfileTicksSumToChargedTicks(t *testing.T) {
	machine, prog := newTickTestVM(t, "x = 0;\nfor i in [1..5000]\n  x = x + i;\nendfor\nreturn x;")
	session := startProfile(t)
	result := machine.Run(prog)
	if result.Val.Int() != 12502500 {
		t.Fatalf("result = %v", result.Val)
	}
	prof := session.Stop()

	var buf bytes.Buffer
	if err := prof.Write(&buf); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	parsed, err := profile.Parse(&buf)
	if err != nil {
		t.Fatalf("parse profile: %v", err)
	}
	if len(parsed.Sample) < 2 {
		t.Fatalf("got %d samples, want checkpoint samples plus the exit sample", len(parsed.Sample))
	}
	var ticks int64
	for _, s := range parsed.Sample {
		ticks += s.Value[0]
	}
	if ticks != machine.Ticks {
		t.Fatalf("sampled ticks = %d, VM charged %d", ticks, machine.Ticks)
	}
}

// With no session active nothing is recorded and later sessions do not see
// ticks charged before they started.
func TestProfileIgnoresTicksOutsideSession(t *testing.T) {
	machine, prog := newTickTestVM(t, "x = 0; for i in [1..3000] x = x + i; endfor return x;")
	machine.Run(prog)
	before := machine.Ticks

	session := startProfile(t)
	machine2, prog2 := newTickTestVM(t, "x = 0; for i in [1..3000] x = x + i; endfor return x;")
	machine2.Run(prog2)
	prof := session.Stop()
	var ticks int64
	for _, s := range prof.Sample {
		ticks += s.Value[0]
	}
	if ticks != machine2.Ticks {
		t.Fatalf("sampled ticks = %d, want %d (first run charged %d outside the session)", ticks, machine2.Ticks, before)
	}
}

// Each sampled frame's line is LineForIP of that frame, and a nested VM's
// samples continue into the frames of the VM that started it.
func TestProfileStackLinesAndNestedFrames(t *testing.T) {
	outer, outerProg := newTickTestVM(t, "x = 1;\ny = 2;\nz = 3;\nreturn x;")
	outer.beginProgram(outerProg)
	for range 4 {
		if err := outer.Step(); err != nil {
			t.Fatalf("outer step: %v", err)
		}
	}
	inner, innerProg := newTickTestVM(t, "a = 1;\nb = 2;\nreturn a;")
	inner.beginProgram(innerProg)
	if err := inner.Step(); err != nil {
		t.Fatalf("inner step: %v", err)
	}
	inner.SetProfileParent(outer)

	stack := inner.profileStack()
	var want []*StackFrame
	for i := len(inner.Frames) - 1; i >= 0; i-- {
		want = append(want, inner.Frames[i])
	}
	for i := len(outer.Frames) - 1; i >= 0; i-- {
		want = append(want, outer.Frames[i])
	}
	if len(stack) != len(want) {
		t.Fatalf("stack has %d frames, want %d (inner then outer)", len(stack), len(want))
	}
	for i, f := range want {
		if line := int64(f.Program.LineForIP(f.IP)); stack[i].Line != line {
			t.Errorf("frame %d line = %d, want LineForIP = %d", i, stack[i].Line, line)
		}
	}
	if stack[0].Line == stack[1].Line {
		t.Fatalf("inner and outer frames report the same line %d; the test is not distinguishing them", stack[0].Line)
	}
}

// Ticks a nested VM bills are taken off its caller's next sample, since the
// caller absorbs them into its own Ticks.
func TestProfileNestedTicksNotBilledTwice(t *testing.T) {
	outer, outerProg := newTickTestVM(t, "x = 0; for i in [1..3000] x = x + i; endfor return x;")
	session := startProfile(t)
	outer.beginProgram(outerProg)
	outer.profileEnter()
	for range 100 {
		if err := outer.Step(); err != nil {
			t.Fatalf("outer step: %v", err)
		}
	}

	inner, innerProg := newTickTestVM(t, "a = 0; for i in [1..3000] a = a + i; endfor return a;")
	inner.SetProfileParent(outer)
	inner.Run(innerProg)
	outer.Ticks += inner.Ticks // what CallVerbInContext and executeCallBuiltin do

	outer.profileExit()
	prof := session.Stop()
	var ticks int64
	var nested bool
	for _, s := range prof.Sample {
		ticks += s.Value[0]
		if len(s.Location) > 1 {
			nested = true
		}
	}
	if ticks != outer.Ticks {
		t.Fatalf("sampled ticks = %d, want the caller's total %d", ticks, outer.Ticks)
	}
	if !nested {
		t.Fatal("no sample carried both the nested and the calling frames")
	}
}

func TestProfileSessionIsExclusive(t *testing.T) {
	startProfile(t)
	if _, err := mooprof.Start(); err != mooprof.ErrBusy {
		t.Fatalf("second Start error = %v, want ErrBusy", err)
	}
}

func TestProfileFuncName(t *testing.T) {
	if got := profileFuncName(1007, "s_run"); got != "#1007:s_run" {
		t.Fatalf("profileFuncName = %q", got)
	}
	if !strings.HasPrefix(profileFuncName(-1, ""), "#-1:") {
		t.Fatal("negative object ids keep their sign")
	}
}
