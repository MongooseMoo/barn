package bytecode

import "testing"

func TestCountsTickExactOpcodeSet(t *testing.T) {
	counted := map[OpCode]bool{
		OP_CALL_BUILTIN:   true,
		OP_CALL_VERB:      true,
		OP_LOOP:           true,
		OP_FOR_RANGE_NEXT: true,
		OP_PASS:           true,
	}

	for raw := 0; raw <= 255; raw++ {
		op := OpCode(raw)
		if got, want := CountsTick(op), counted[op]; got != want {
			t.Fatalf("CountsTick(%d) = %t, want %t", raw, got, want)
		}
	}
}
