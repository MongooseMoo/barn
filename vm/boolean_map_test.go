package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestBooleanMapKeysSurviveBothInsertionOrders(t *testing.T) {
	tests := []struct {
		name string
		keys []types.Value
	}{
		{name: "false first", keys: []types.Value{types.NewBool(false), types.NewBool(true)}},
		{name: "true first", keys: []types.Value{types.NewBool(true), types.NewBool(false)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := BuildVMRegistry()
			mapping := types.NewEmptyMap()
			for _, key := range test.keys {
				var err types.ErrorCode
				mapping, err = setAtIndex(registry, nil, mapping, key, types.NewStr(key.String()))
				if err != types.E_NONE {
					t.Fatalf("set key %s: %s", key, err)
				}
			}
			if mapping.Len() != 2 {
				t.Fatalf("map length = %d, want 2", mapping.Len())
			}
			for _, key := range test.keys {
				got, ok := mapping.MapGet(key)
				if !ok || got.Str() != key.String() {
					t.Fatalf("get key %s = (%v, %v)", key, got, ok)
				}
			}
		})
	}
}
