package bytecode

import "testing"

func TestCompilerResolvesBuiltinVariableSlots(t *testing.T) {
	compiler := NewCompiler()
	compiler.beginScope()

	compiler.declareVariable("ordinary")
	compiler.declareVariable("this")
	compiler.declareVariable("player")
	compiler.declareVariable("iobj")

	want := BuiltinSlots{This: 2, Player: 3, Iobj: 4}
	if got := compiler.program.BuiltinSlots; got != want {
		t.Fatalf("BuiltinSlots = %+v, want %+v", got, want)
	}
}

func TestBuiltinSlotsZeroValueMeansUnused(t *testing.T) {
	var slots BuiltinSlots
	if slots.Set("ordinary", 0) {
		t.Fatal("Set reported an ordinary local as built-in")
	}
	if slots != (BuiltinSlots{}) {
		t.Fatalf("ordinary local changed BuiltinSlots to %+v", slots)
	}
}
