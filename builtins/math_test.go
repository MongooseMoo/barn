package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestChrReturnsRawByteString(t *testing.T) {
	ctx := newTestExecution()
	ctx.IsWizard = true

	res := builtinChr(ctx, []types.Value{types.NewInt(200)})
	if res.IsError() {
		t.Fatalf("chr(200) failed: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_STR {
		t.Fatalf("chr(200) = %T, want StrValue", res.Val)
	}
	if got := res.Val.Str(); got != string([]byte{200}) {
		t.Fatalf("chr(200) raw value = %#v, want one byte C8", []byte(got))
	}
	if got := res.Val.Len(); got != 1 {
		t.Fatalf("chr(200) length = %d, want 1", got)
	}

	encoded := builtinEncodeBinary(ctx, []types.Value{res.Val})
	if encoded.IsError() {
		t.Fatalf("encode_binary(chr(200)) failed: %v", encoded.Error)
	}
	if encoded.Val.Type() != types.TYPE_STR {
		t.Fatalf("encode_binary(chr(200)) = %T, want StrValue", encoded.Val)
	}
	if got := encoded.Val.Str(); got != "~C8" {
		t.Fatalf("encode_binary(chr(200)) = %q, want ~C8", got)
	}
}
