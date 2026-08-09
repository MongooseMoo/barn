package builtins

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func setVerbCodeStaticNameBoundarySource(operation string, count int, tail string) []string {
	source := []string{"object = player;", "if (0)"}
	for i := 0; i < count-1; i++ {
		name := fmt.Sprintf("boundary_%03d", i)
		switch operation {
		case "get":
			source = append(source, fmt.Sprintf("object.%s;", name))
		case "set":
			source = append(source, fmt.Sprintf("object.%s = 0;", name))
		case "call":
			source = append(source, fmt.Sprintf("object:%s();", name))
		default:
			panic("unknown static-name operation: " + operation)
		}
	}
	return append(source, "endif", tail)
}

func TestSetVerbCodeAccepts255And256DistinctStaticNames(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		tail      string
	}{
		{name: "property get", operation: "get", tail: "return object.edge;"},
		{name: "property set", operation: "set", tail: "object.edge = 1;"},
		{name: "verb call", operation: "call", tail: "return object:edge();"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, count := range []int{255, 256} {
				t.Run(fmt.Sprintf("%d names", count), func(t *testing.T) {
					ctx, store, objID := b2aTestContext(t)
					source := setVerbCodeStaticNameBoundarySource(tc.operation, count, tc.tail)
					values := make([]types.Value, len(source))
					for i, line := range source {
						values[i] = types.NewStr(line)
					}
					result := builtinSetVerbCode(ctx, []types.Value{
						types.NewObj(objID),
						types.NewStr("scratch"),
						types.NewList(values),
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
		})
	}
}
