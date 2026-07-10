package builtins

import (
	"testing"

	"barn/kernel"
	"barn/types"
)

func TestMapvaluesMissingCompositeKeysReturnRange(t *testing.T) {
	ctx := &kernel.TaskContext{}
	emptyMap := types.NewEmptyMap()

	for _, tc := range []struct {
		name string
		key  types.Value
	}{
		{name: "list", key: types.NewList(nil)},
		{name: "map", key: types.NewMap([][2]types.Value{{types.NewInt(1), types.NewInt(2)}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := builtinMapvalues(ctx, []types.Value{emptyMap, tc.key})
			if !result.IsError() || result.Error != types.E_RANGE {
				t.Fatalf("mapvalues([], %s) = %+v, want E_RANGE", tc.name, result)
			}
		})
	}
}

func TestMapvaluesDispatchMissingCompositeKeysReturnRange(t *testing.T) {
	ctx := &kernel.TaskContext{}
	registry := NewRegistry()
	emptyMap := types.NewEmptyMap()

	for _, tc := range []struct {
		name string
		key  types.Value
	}{
		{name: "list", key: types.NewList(nil)},
		{name: "map", key: types.NewMap([][2]types.Value{{types.NewInt(1), types.NewInt(2)}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := registry.CallByName("mapvalues", ctx, []types.Value{emptyMap, tc.key})
			if !ok {
				t.Fatal("mapvalues not registered")
			}
			if !result.IsError() || result.Error != types.E_RANGE {
				t.Fatalf("mapvalues([], %s) dispatch = %+v, want E_RANGE", tc.name, result)
			}
		})
	}
}
