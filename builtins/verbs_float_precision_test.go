package builtins

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast keeps a float literal's full value in the compiled verb while
// verb_code() prints it with 15 significant digits. Barn compiles from the
// code set_verb_code stores, so that code must spell the value exactly.
func TestSetVerbCodeKeepsFloatLiteralPrecision(t *testing.T) {
	ctx, store, object := b2aTestContext(t)
	code := types.NewList([]types.Value{types.NewStr("return 18.46744073709553614;")})
	result := admitTestBuiltin("set_verb_code", ctx, []types.Value{types.NewObj(object), types.NewStr("scratch"), code})
	if result.IsError() || result.Val.Type() != types.TYPE_LIST || result.Val.Len() != 0 {
		t.Fatalf("set_verb_code failed: %v", result)
	}
	if got, want := b2aVerbCode(t, store, object), []string{"return 18.467440737095536;"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored code = %q, want %q", got, want)
	}
	result = admitTestBuiltin("verb_code", ctx, []types.Value{types.NewObj(object), types.NewStr("scratch")})
	if result.IsError() || result.Val.Len() != 1 || result.Val.Get(1).Str() != "return 18.4674407370955;" {
		t.Fatalf("verb_code = %v, want {\"return 18.4674407370955;\"}", result)
	}
}
