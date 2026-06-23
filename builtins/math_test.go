package builtins

import (
	"testing"

	"barn/kernel"
	"barn/types"
)

func TestChrReturnsRawByteString(t *testing.T) {
	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true

	res := builtinChr(ctx, []types.Value{types.NewInt(200)})
	if res.IsError() {
		t.Fatalf("chr(200) failed: %v", res.Error)
	}
	str, ok := res.Val.(types.StrValue)
	if !ok {
		t.Fatalf("chr(200) = %T, want StrValue", res.Val)
	}
	if got := str.Value(); got != string([]byte{200}) {
		t.Fatalf("chr(200) raw value = %#v, want one byte C8", []byte(got))
	}
	if got := str.Len(); got != 1 {
		t.Fatalf("chr(200) length = %d, want 1", got)
	}

	encoded := builtinEncodeBinary(ctx, []types.Value{str})
	if encoded.IsError() {
		t.Fatalf("encode_binary(chr(200)) failed: %v", encoded.Error)
	}
	encodedStr, ok := encoded.Val.(types.StrValue)
	if !ok {
		t.Fatalf("encode_binary(chr(200)) = %T, want StrValue", encoded.Val)
	}
	if got := encodedStr.Value(); got != "~C8" {
		t.Fatalf("encode_binary(chr(200)) = %q, want ~C8", got)
	}
}
