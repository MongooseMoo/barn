package vm

import (
	"runtime"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/types"
)

// sourceFuzzCallBytes bounds the arguments of one builtin call in a fuzz case:
// types.ValueBytes summed over them. A program can build a value up to the
// value limits (64,537,861 bytes) within its ticks, and a builtin may do work
// that is a multiple of its arguments' size before any limit stops it, or
// with no limit at all: strsub's result is the product of two argument
// lengths, slice's of two list lengths. At this bound such a product is
// 64 MiB, the scale of the value limits.
const sourceFuzzCallBytes = 16 << 10

// newSourceFuzzRegistry returns a registry of the core capabilities whose
// builtins are not called with more than sourceFuzzCallBytes of arguments:
// such a call calls skipf, which must end the case, in place of the builtin.
// The case then has no result to check, and nothing is measured to decide
// it: no clock and no memory reading. The lanes' own calls through the
// session, such as the toliteral of a returned value, are bounded the same
// way. A builtin that allocates by the value of a small argument, not its
// size, is not bounded here.
//
// Lowering the value limits instead would need a $server_options object in
// the world, which programs could see, and would not bound slice.
func newSourceFuzzRegistry(tb testing.TB, skipf func(format string, args ...any)) *builtins.Registry {
	tb.Helper()
	descriptors := Descriptors()
	for i := range descriptors {
		name, call := descriptors[i].Name, descriptors[i].Implementation
		descriptors[i].Implementation = func(ctx *builtins.Execution, args []types.Value) types.Result {
			size := 0
			for _, arg := range args {
				size += types.ValueBytes(arg)
			}
			if size > sourceFuzzCallBytes {
				skipf("%s() was called with %d bytes of arguments, over the %d-byte bound of a fuzz case; the call was not made, so the case has no result to check", name, size, sourceFuzzCallBytes)
			}
			return call(ctx, args)
		}
	}
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, descriptors)
	if err != nil {
		tb.Fatal(err)
	}
	return registry
}

// oversizedCallAllocation is the most a case cut at an oversized builtin call
// may allocate: the values it built before the call, not the call's result.
const oversizedCallAllocation = 64 << 20

// oversizedCallPrologue is the structured generator's variable prologue.
const oversizedCallPrologue = `n = 0;
f = 0.0;
s = "aaaaaa";
l = {0, 0, ""};
m = ["" -> 0, 0 -> ""];
o = #0;
`

// Within the 100-tick budget each program doubles a value in a loop and then
// makes one builtin call whose work is a multiple of its arguments' size. Run
// to the end, one case allocated (2026-11-14, both runs of the checkpoint
// property): strsub 6.3 GB, slice 3.0 GB, pcre_match 3.9 GB; two more
// doublings pass a fuzz worker's 12 GiB memory limit.
var oversizedCallPrograms = []struct{ name, source string }{
	{"strsub", oversizedCallPrologue + `for x in [1..12]
  s = s + s;
endfor
strsub(s, "a", s);
suspend();`},
	{"slice", oversizedCallPrologue + `x = {s};
y = {1};
for z in [1..12]
  x = {@x, @x};
  y = {@y, @y};
endfor
slice(x, y);
suspend();`},
	{"pcre_match", oversizedCallPrologue + `for x in [1..18]
  s = s + s;
endfor
pcre_match(s, "a");
suspend();`},
}

// A generated program's lane skips the case at the oversized call instead of
// making it.
func TestFuzzCaseSkipsOversizedBuiltinCall(t *testing.T) {
	world := newSourceFuzzWorld(t)
	lanes := []struct {
		name string
		run  func(t *testing.T, source string)
	}{
		{"FuzzCheckpointResumeEquivalence", checkCheckpointResumeEquivalence},
		{"FuzzStructuredSourceExecution", func(t *testing.T, source string) { executeFuzzSource(t, world, source, true) }},
	}
	for _, program := range oversizedCallPrograms {
		for _, lane := range lanes {
			t.Run(program.name+"/"+lane.name, func(t *testing.T) {
				allocated, skipped := sourceFuzzCaseAllocation(t, func(t *testing.T) { lane.run(t, program.source) })
				if allocated > oversizedCallAllocation {
					t.Errorf("the case allocated %d bytes, over the %d-byte bound", allocated, oversizedCallAllocation)
				}
				if !skipped {
					t.Error("the case made the oversized builtin call instead of being skipped")
				}
			})
		}
	}
}

// The source-execution lanes pass a returned value to toliteral and compile
// the literal (checkLiteralRoundTrip), which for this list of 2^20 integers
// allocated 1.07 GB over the two principals. That call is bounded like the
// program's own.
func TestFuzzCaseSkipsOversizedLiteralRoundTrip(t *testing.T) {
	world := newSourceFuzzWorld(t)
	source := `x = {1};
for z in [1..20]
  x = {@x, @x};
endfor
return x;`
	allocated, skipped := sourceFuzzCaseAllocation(t, func(t *testing.T) { executeFuzzSource(t, world, source, true) })
	if allocated > oversizedCallAllocation {
		t.Errorf("the case allocated %d bytes, over the %d-byte bound", allocated, oversizedCallAllocation)
	}
	if !skipped {
		t.Error("the case made the oversized toliteral call instead of being skipped")
	}
}

// sourceFuzzCaseAllocation runs one fuzz case as a subtest and returns the
// bytes allocated while it ran and whether it was skipped.
func sourceFuzzCaseAllocation(t *testing.T, run func(t *testing.T)) (allocated uint64, skipped bool) {
	t.Helper()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	t.Run("case", func(t *testing.T) {
		defer func() { skipped = t.Skipped() }()
		run(t)
	})
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc, skipped
}

// Two property values that together pass max_list_value_bytes (64,537,861)
// are state like any other: the observer returns it, and tells it from a
// state that differs in one character.
func TestPersistentStateObserverReadsStateOverListLimit(t *testing.T) {
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	session := newTestSessionWithTaskManager(registry)
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	const length = 33 << 20
	text := strings.Repeat("a", length)
	store := newCheckpointResumeStore(t)
	for _, id := range []types.ObjID{0, 2} {
		if code := store.DirectTxn().SetPropertyValue(id, "p", types.NewStr(text)); code != types.E_NONE {
			t.Fatalf("set #%d.p: %v", id, code)
		}
	}
	state := observePersistentState(t, store, session)
	if !sameCheckpointedValue(state, observePersistentState(t, store, session)) {
		t.Fatal("two observations of one state differ")
	}
	if code := store.DirectTxn().SetPropertyValue(2, "p", types.NewStr(text[:length-1]+"b")); code != types.E_NONE {
		t.Fatalf("change #2.p: %v", code)
	}
	if sameCheckpointedValue(state, observePersistentState(t, store, session)) {
		t.Fatal("the observer did not see the change to #2.p")
	}
}
