package builtins

import (
	"fmt"
	"strings"
	"testing"

	"barn/types"
)

func setVerbCodeConstantBoundarySource(tail string) []string {
	fillers := make([]string, 255)
	for i := range fillers {
		fillers[i] = fmt.Sprintf("%q", fmt.Sprintf("constant-%03d", i))
	}
	return []string{"fillers = {" + strings.Join(fillers, ", ") + "};", tail}
}

func TestSetVerbCodeAcceptsStaticNameAtConstant255(t *testing.T) {
	tests := []struct {
		name string
		tail string
	}{
		{name: "property get", tail: "return player.edge;"},
		{name: "property set", tail: "player.edge = 1;"},
		{name: "verb call", tail: "return player:edge();"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store, objID := b2aTestContext(t)
			source := setVerbCodeConstantBoundarySource(tc.tail)
			result := builtinSetVerbCode(ctx, []types.Value{
				types.NewObj(objID),
				types.NewStr("scratch"),
				types.NewList([]types.Value{types.NewStr(source[0]), types.NewStr(source[1])}),
			})
			if result.IsError() || result.Val.Type() != types.TYPE_LIST || result.Val.Len() != 0 {
				t.Fatalf("set_verb_code() = value %v error %v, want {}", result.Val, result.Error)
			}
			stored := b2aVerbCode(t, store, objID)
			if got := strings.Join(stored, "\n"); got != strings.Join(source, "\n") {
				t.Fatalf("stored verb code differs after accepted boundary source")
			}
		})
	}
}
