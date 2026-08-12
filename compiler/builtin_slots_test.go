package compiler

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
)

func TestCompilerResolvesBuiltinVariableSlots(t *testing.T) {
	lowerer := newLowerer(nil)
	lowerer.beginScope()

	lowerer.declareVariable("ordinary")
	lowerer.declareVariable("this")
	lowerer.declareVariable("player")
	lowerer.declareVariable("iobj")

	want := bytecode.BuiltinSlots{This: 2, Player: 3, Iobj: 4}
	if got := lowerer.program.BuiltinSlots; got != want {
		t.Fatalf("BuiltinSlots = %+v, want %+v", got, want)
	}
}

func TestBuiltinSlotsZeroValueMeansUnused(t *testing.T) {
	var slots bytecode.BuiltinSlots
	if slots.Set("ordinary", 0) {
		t.Fatal("Set reported an ordinary local as built-in")
	}
	if slots != (bytecode.BuiltinSlots{}) {
		t.Fatalf("ordinary local changed BuiltinSlots to %+v", slots)
	}
}
