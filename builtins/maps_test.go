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

func TestMaphaskeyPreservesToastMixedBooleanIntegerReachability(t *testing.T) {
	ctx := &kernel.TaskContext{}
	mapping := types.NewEmptyMap()
	mapping = mapping.MapSet(types.NewBool(true), types.NewStr("boolean one"))
	mapping = mapping.MapSet(types.NewInt(1), types.NewStr("integer one"))
	mapping = mapping.MapSet(types.NewBool(false), types.NewStr("boolean zero"))
	mapping = mapping.MapSet(types.NewInt(0), types.NewStr("integer zero"))

	tests := []struct {
		name string
		key  types.Value
		want int64
	}{
		{name: "true", key: types.NewBool(true), want: 0},
		{name: "one", key: types.NewInt(1), want: 1},
		{name: "false", key: types.NewBool(false), want: 1},
		{name: "zero", key: types.NewInt(0), want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := builtinMaphaskey(ctx, []types.Value{mapping, test.key})
			if result.IsError() {
				t.Fatalf("maphaskey: %s", result.Error)
			}
			if got := result.Val.Int(); got != test.want {
				t.Fatalf("maphaskey = %d, want %d", got, test.want)
			}
		})
	}
}
