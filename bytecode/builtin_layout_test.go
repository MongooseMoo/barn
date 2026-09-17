package bytecode

import "testing"

func TestForkBodyRetainsBuiltinLayout(t *testing.T) {
	program := &Program{Code: []byte{byte(OP_RETURN_NONE)}, BuiltinLayout: [32]byte{42}}
	fork := program.ExtractForkBody(0, 1)
	if fork == nil || fork.BuiltinLayout != program.BuiltinLayout {
		t.Fatal("fork lost builtin layout")
	}
}
