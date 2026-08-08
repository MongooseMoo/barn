package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
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

func TestMultiKeyMapdeleteRaisesMissingKeyDetail(t *testing.T) {
	ctx := &kernel.TaskContext{}
	mapping := types.NewMap([][2]types.Value{
		{types.NewInt(1), types.NewStr("one")},
		{types.NewInt(2), types.NewStr("two")},
		{types.NewInt(3), types.NewStr("three")},
	})
	keys := types.NewList([]types.Value{
		types.NewInt(1),
		types.NewInt(99),
		types.NewInt(3),
	})

	result := builtinMapdelete(ctx, []types.Value{mapping, keys})
	if result.Flow != types.FlowException || result.Error != types.E_RANGE {
		t.Fatalf("mapdelete result = %+v, want E_RANGE exception", result)
	}
	if result.Val.Type() != types.TYPE_LIST || result.Val.Len() != 3 {
		t.Fatalf("mapdelete exception value = %v, want three-element raise payload", result.Val)
	}
	if got := result.Val.Get(1); got.Type() != types.TYPE_ERR || got.Code() != types.E_RANGE {
		t.Fatalf("exception code = %v, want E_RANGE", got)
	}
	if got := result.Val.Get(2); got.Type() != types.TYPE_STR || got.Str() != "Key 99 not found in map" {
		t.Fatalf("exception message = %v, want missing-key detail", got)
	}
	if got := result.Val.Get(3); got.Type() != types.TYPE_INT || got.Int() != 99 {
		t.Fatalf("exception detail = %v, want 99", got)
	}
}

func TestMapdeleteUsesPendingListValueByteLimit(t *testing.T) {
	ctx := kernel.NewTaskContext()
	mapping := types.NewMap([][2]types.Value{
		{types.NewInt(1), types.NewStr("one")},
		{types.NewInt(2), types.NewStr("two")},
	})
	resultMap := mapping.MapDelete(types.NewInt(1))
	ctx.PendingEffects = []kernel.PendingEffect{{
		Kind: kernel.PendingEffectServerOptions,
		ServerOptions: kernel.PendingServerOptions{
			MaxListValueBytes: ValueBytes(resultMap),
			MaxMapValueBytes:  ValueBytes(mapping) + 1,
		},
	}}

	result := builtinMapdelete(ctx, []types.Value{mapping, types.NewInt(1)})
	if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
		t.Fatalf("mapdelete at pending list byte limit = flow %v error %v value %v, want E_QUOTA", result.Flow, result.Error, result.Val)
	}
}
