package builtins

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestSetVerbCodeReturnsOrderedDiagnosticsAndPreservesBody(t *testing.T) {
	ctx, store, object := b2aTestContext(t)
	for range 2 {
		result := builtinSetVerbCode(ctx, []types.Value{types.NewObj(object), types.NewStr("scratch"), types.NewList([]types.Value{types.NewStr("break;"), types.NewStr("continue;")})})
		if result.IsError() || result.Val.Type() != types.TYPE_LIST || result.Val.Len() != 2 {
			t.Fatalf("result = %v", result)
		}
		want := []string{"Line 1:  No enclosing loop for `break' statement", "Line 2:  No enclosing loop for `continue' statement"}
		for i, message := range want {
			if got := result.Val.Get(i + 1).Str(); got != message {
				t.Fatalf("diagnostic %d = %q, want %q", i, got, message)
			}
		}
		if got := b2aVerbCode(t, store, object); !reflect.DeepEqual(got, []string{"x = 1;"}) {
			t.Fatalf("failed compilation changed source: %q", got)
		}
	}
}
