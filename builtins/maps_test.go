package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func BenchmarkMapvaluesTreeOrder(b *testing.B) {
	pairs := make([][2]types.Value, 1024)
	for i := range pairs {
		pairs[i] = [2]types.Value{types.NewInt(int64(i)), types.NewInt(int64(i + 1))}
	}
	args := []types.Value{types.NewMap(pairs)}
	builtinMapvalues(nil, args) // Warm the lazy tree traversal, as with a reused ledger.
	b.ReportAllocs()
	for b.Loop() {
		result := builtinMapvalues(nil, args)
		if !result.IsNormal() || result.Val.Len() != len(pairs) {
			b.Fatalf("mapvalues = %+v", result)
		}
	}
}

func TestMapvaluesMissingCompositeKeysReturnRange(t *testing.T) {
	ctx := newTestExecution()
	emptyMap := types.NewEmptyMap()

	for _, tc := range []struct {
		name string
		key  types.Value
	}{
		{name: "list", key: types.NewList(nil)},
		{name: "map", key: types.NewMap([][2]types.Value{{types.NewInt(1), types.NewInt(2)}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := admitTestBuiltin("mapvalues", ctx, []types.Value{emptyMap, tc.key})
			if !result.IsError() || result.Error != types.E_RANGE {
				t.Fatalf("mapvalues([], %s) = %+v, want E_RANGE", tc.name, result)
			}
		})
	}
}

func TestMapvaluesDispatchMissingCompositeKeysReturnRange(t *testing.T) {
	ctx := newTestExecution()
	registry := NewRegistry()
	session := NewSession(registry, NoHost())
	emptyMap := types.NewEmptyMap()

	for _, tc := range []struct {
		name string
		key  types.Value
	}{
		{name: "list", key: types.NewList(nil)},
		{name: "map", key: types.NewMap([][2]types.Value{{types.NewInt(1), types.NewInt(2)}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx.Registry = registry
			ctx.Session = session
			result, ok := session.CallByNameWithExecution("mapvalues", ctx, []types.Value{emptyMap, tc.key})
			if !ok {
				t.Fatal("mapvalues not registered")
			}
			if !result.IsError() || result.Error != types.E_RANGE {
				t.Fatalf("mapvalues([], %s) dispatch = %+v, want E_RANGE", tc.name, result)
			}
		})
	}
}

// Toast 2.8 orders false < true in compare(), so every key of this mixed map
// is reachable (conformance boolean_and_equal_integer_mixed_map_reachability).
func TestMaphaskeyReachesEveryMixedBooleanIntegerKey(t *testing.T) {
	ctx := newTestExecution()
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
		{name: "true", key: types.NewBool(true), want: 1},
		{name: "one", key: types.NewInt(1), want: 1},
		{name: "false", key: types.NewBool(false), want: 1},
		{name: "zero", key: types.NewInt(0), want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := admitTestBuiltin("maphaskey", ctx, []types.Value{mapping, test.key})
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
	ctx := newTestExecution()
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

	result := admitTestBuiltin("mapdelete", ctx, []types.Value{mapping, keys})
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

// Toast ff990b8: mapdelete() and maphaskey() accept anonymous object keys.
func TestMapKeyBuiltinsAcceptAnonymousKeys(t *testing.T) {
	ctx := newTestExecution()
	first := types.NewAnon(1)
	second := types.NewAnon(2)
	absent := types.NewAnon(3)
	mapping := types.NewMap([][2]types.Value{
		{first, types.NewInt(1)},
		{second, types.NewInt(2)},
	})

	for _, test := range []struct {
		key  types.Value
		want int64
	}{
		{key: first, want: 1},
		{key: absent, want: 0},
	} {
		result := admitTestBuiltin("maphaskey", ctx, []types.Value{mapping, test.key})
		if result.IsError() || result.Val.Int() != test.want {
			t.Fatalf("maphaskey(%v) = %+v, want %d", test.key, result, test.want)
		}
	}

	single := admitTestBuiltin("mapdelete", ctx, []types.Value{mapping, first})
	if single.IsError() || single.Val.Len() != 1 {
		t.Fatalf("mapdelete(first) = %+v, want one-entry map", single)
	}
	if _, found := single.Val.MapGet(second); !found {
		t.Fatalf("mapdelete(first) = %v, want second kept", single.Val)
	}

	multi := admitTestBuiltin("mapdelete", ctx, []types.Value{mapping, types.NewList([]types.Value{first, second})})
	if multi.IsError() || multi.Val.Len() != 0 {
		t.Fatalf("mapdelete({first, second}) = %+v, want empty map", multi)
	}

	missing := admitTestBuiltin("mapdelete", ctx, []types.Value{mapping, absent})
	if missing.Error != types.E_RANGE {
		t.Fatalf("mapdelete(absent) = %+v, want E_RANGE", missing)
	}
}

func TestMultiKeyMapdeleteRaisesMissingAnonymousKeyDetail(t *testing.T) {
	ctx := newTestExecution()
	present := types.NewAnon(1)
	absent := types.NewAnon(2)
	mapping := types.NewMap([][2]types.Value{{present, types.NewInt(1)}})

	result := admitTestBuiltin("mapdelete", ctx, []types.Value{mapping, types.NewList([]types.Value{present, absent})})
	if result.Flow != types.FlowException || result.Error != types.E_RANGE || result.Val.Len() != 3 {
		t.Fatalf("mapdelete result = %+v, want E_RANGE raise payload", result)
	}
	if got := result.Val.Get(2); got.Type() != types.TYPE_STR || got.Str() != "Key *anonymous* not found in map" {
		t.Fatalf("exception message = %v, want Toast unparse of the anonymous key", got)
	}
	if got := result.Val.Get(3); !got.Equal(absent) {
		t.Fatalf("exception detail = %v, want the missing anonymous key", got)
	}
}

func TestMapdeleteUsesPendingMapValueByteLimit(t *testing.T) {
	mapping := types.NewMap([][2]types.Value{
		{types.NewInt(1), types.NewStr("one")},
		{types.NewInt(2), types.NewStr("two")},
	})
	resultMap := mapping.MapDelete(types.NewInt(1))

	for _, test := range []struct {
		name     string
		mapLimit int
		wantErr  types.ErrorCode
	}{
		{name: "ignores list limit", mapLimit: ValueBytes(resultMap) + 1, wantErr: types.E_NONE},
		{name: "enforces map limit", mapLimit: ValueBytes(resultMap) - 1, wantErr: types.E_QUOTA},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := newTestExecution()
			ctx.PendingEffects = []task.PendingEffect{{
				Kind: task.PendingEffectServerOptions,
				ServerOptions: &task.PendingServerOptions{
					MaxListValueBytes: ValueBytes(resultMap),
					MaxMapValueBytes:  test.mapLimit,
				},
			}}

			for _, key := range []types.Value{
				types.NewInt(1),
				types.NewList([]types.Value{types.NewInt(1)}),
			} {
				result := admitTestBuiltin("mapdelete", ctx, []types.Value{mapping, key})
				if result.Error != test.wantErr {
					t.Fatalf("mapdelete(%v) error = %v, want %v", key, result.Error, test.wantErr)
				}
			}
		})
	}
}
