package bytecode

import "testing"

// A fork body's OP_TRY_EXCEPT / OP_TRY_FINALLY operands encode ABSOLUTE
// handler IPs in the parent program's coordinates (vm/control.go reads them
// as absolutes). ExtractForkBody copies the body into a fresh program whose
// coordinates start at 0, so those operands must be rebased by -bodyIP or the
// handler jump lands outside the extracted code (mongoose #0:server_started:
// parent IP 171 fetched inside a 32-byte fork program → panic).
// Relative operands (OP_JUMP family) must NOT be touched.

func encodeShort(v int) (byte, byte) {
	return byte(uint16(v) >> 8), byte(uint16(v) & 0xFF)
}

func decodeShort(hi, lo byte) int {
	return int(uint16(hi)<<8 | uint16(lo))
}

func TestExtractForkBodyRebasesTryExceptHandlerIP(t *testing.T) {
	const bodyIP = 10
	// Parent layout: 10 bytes of padding, then the fork body:
	//   OP_TRY_EXCEPT numClauses=1 [numCodes=1 code=1 var+1=2 hi lo]
	//   OP_POP                                  (try body stand-in)
	//   OP_END_EXCEPT 1
	//   OP_JUMP hi lo                           (jump over handler; RELATIVE)
	//   OP_POP                                  (handler code; absolute IP 21)
	const handlerAbs = bodyIP + 11
	hi, lo := encodeShort(handlerAbs)
	jmpHi, jmpLo := encodeShort(1) // relative offset, must survive unchanged
	body := []byte{
		byte(OP_TRY_EXCEPT), 1, 1, 1, 2, hi, lo,
		byte(OP_POP),
		byte(OP_END_EXCEPT), 1,
		byte(OP_JUMP), jmpHi, jmpLo,
		byte(OP_POP),
	}
	parent := &Program{Code: append(make([]byte, bodyIP), body...)}

	sub := parent.ExtractForkBody(bodyIP, len(body))

	gotHandler := decodeShort(sub.Code[5], sub.Code[6])
	if want := handlerAbs - bodyIP; gotHandler != want {
		t.Errorf("TRY_EXCEPT handler IP = %d, want %d (rebased by bodyIP %d)", gotHandler, want, bodyIP)
	}
	if gotJump := decodeShort(sub.Code[11], sub.Code[12]); gotJump != 1 {
		t.Errorf("relative OP_JUMP operand = %d, want 1 (must not be rebased)", gotJump)
	}
	if last := sub.Code[len(sub.Code)-1]; OpCode(last) != OP_RETURN_NONE {
		t.Errorf("extracted body must end with OP_RETURN_NONE, got %d", last)
	}
}

func TestExtractForkBodyRebasesMultiClauseAndFinallyIPs(t *testing.T) {
	const bodyIP = 7
	// Two-clause TRY_EXCEPT (clause 0: two codes, no var; clause 1: catch-all
	// with var) followed by a TRY_FINALLY.
	h0hi, h0lo := encodeShort(bodyIP + 30)
	h1hi, h1lo := encodeShort(bodyIP + 40)
	finHi, finLo := encodeShort(bodyIP + 50)
	body := []byte{
		byte(OP_TRY_EXCEPT), 2,
		2, 1, 2, 0, h0hi, h0lo, // clause 0: numCodes=2, codes {1,2}, no var
		0, 3, h1hi, h1lo, // clause 1: numCodes=0 (ANY), var+1=3
		byte(OP_TRY_FINALLY), finHi, finLo,
		byte(OP_POP),
	}
	parent := &Program{Code: append(make([]byte, bodyIP), body...)}

	sub := parent.ExtractForkBody(bodyIP, len(body))

	if got := decodeShort(sub.Code[6], sub.Code[7]); got != 30 {
		t.Errorf("clause 0 handler IP = %d, want 30", got)
	}
	if got := decodeShort(sub.Code[10], sub.Code[11]); got != 40 {
		t.Errorf("clause 1 handler IP = %d, want 40", got)
	}
	if got := decodeShort(sub.Code[13], sub.Code[14]); got != 50 {
		t.Errorf("TRY_FINALLY IP = %d, want 50", got)
	}
}
