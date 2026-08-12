package builtins

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestSliceInvalidElementDoesNotWriteToStdout(t *testing.T) {
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = originalStdout
		reader.Close()
		writer.Close()
	})

	result := builtinSlice(newTestExecution(), []types.Value{
		types.NewList([]types.Value{types.NewInt(1)}),
	})
	os.Stdout = originalStdout
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout capture: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout capture: %v", err)
	}

	if !result.IsError() || result.Error != types.E_INVARG {
		t.Fatalf("slice({1}) = %#v, want E_INVARG", result)
	}
	if len(output) != 0 {
		t.Fatalf("slice({1}) wrote to stdout: %q", output)
	}
}

// TestIsMemberPromoteNumbers: Toast mongoose branch (PROMOTE_NUMBERS on),
// verified live 2026-07-01: is_member(1, {1.0}) and is_member(1.0, {1}) => 1.
// Strict Toast master: both => 0.
func TestIsMemberPromoteNumbers(t *testing.T) {
	strict := newTestExecution()
	promote := newTestExecution()
	promote.RuntimeOptions.PromoteNumbers = true

	lst := types.NewList([]types.Value{types.NewFloat(1.0)})
	if got := builtinIsMember(strict, []types.Value{types.NewInt(1), lst}); got.Val.Int() != 0 {
		t.Fatalf("strict is_member(1, {1.0}) = %v, want 0", got.Val)
	}
	if got := builtinIsMember(promote, []types.Value{types.NewInt(1), lst}); got.Val.Int() != 1 {
		t.Fatalf("promote is_member(1, {1.0}) = %v, want 1", got.Val)
	}
	intList := types.NewList([]types.Value{types.NewInt(1)})
	if got := builtinIsMember(promote, []types.Value{types.NewFloat(1.0), intList}); got.Val.Int() != 1 {
		t.Fatalf("promote is_member(1.0, {1}) = %v, want 1", got.Val)
	}
	m := types.NewMap([][2]types.Value{{types.NewStr("a"), types.NewFloat(1.0)}})
	if got := builtinIsMember(promote, []types.Value{types.NewInt(1), m}); got.Val.Int() != 1 {
		t.Fatalf("promote is_member(1, [\"a\"->1.0]) = %v, want 1", got.Val)
	}
	if got := builtinIsMember(strict, []types.Value{types.NewInt(1), m}); got.Val.Int() != 0 {
		t.Fatalf("strict is_member(1, [\"a\"->1.0]) = %v, want 0", got.Val)
	}
}

func TestListappendClampsExplicitPosition(t *testing.T) {
	ctx := newTestExecution()
	list := types.NewList([]types.Value{types.NewInt(1), types.NewInt(2)})
	tests := []struct {
		position int64
		want     []int64
	}{
		{position: -5, want: []int64{3, 1, 2}},
		{position: 0, want: []int64{3, 1, 2}},
		{position: 1, want: []int64{1, 3, 2}},
		{position: 2, want: []int64{1, 2, 3}},
		{position: 3, want: []int64{1, 2, 3}},
		{position: 99, want: []int64{1, 2, 3}},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("position_%d", tc.position), func(t *testing.T) {
			result := builtinListappend(ctx, []types.Value{
				list,
				types.NewInt(3),
				types.NewInt(tc.position),
			})
			if result.Flow != types.FlowNormal {
				t.Fatalf("listappend position %d flow = %v error = %v, want normal", tc.position, result.Flow, result.Error)
			}
			got := result.Val.Elements()
			if len(got) != len(tc.want) {
				t.Fatalf("listappend position %d length = %d, want %d", tc.position, len(got), len(tc.want))
			}
			for i, want := range tc.want {
				if got[i].Type() != types.TYPE_INT || got[i].Int() != want {
					t.Fatalf("listappend position %d element %d = %v, want %d", tc.position, i+1, got[i], want)
				}
			}
		})
	}
}

func TestSetaddUsesPendingListValueByteLimit(t *testing.T) {
	ctx := newTestExecution()
	list := types.NewList([]types.Value{types.NewInt(1)})
	value := types.NewInt(2)
	limit := ValueBytes(list.Append(value))
	ctx.PendingEffects = []kernel.PendingEffect{{
		Kind: kernel.PendingEffectServerOptions,
		ServerOptions: kernel.PendingServerOptions{
			MaxListValueBytes: limit,
		},
	}}

	result := builtinSetadd(ctx, []types.Value{list, value})
	if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
		t.Fatalf("setadd at pending byte limit = flow %v error %v value %v, want E_QUOTA", result.Flow, result.Error, result.Val)
	}
}

func TestListinsertUsesPendingListValueByteLimit(t *testing.T) {
	ctx := newTestExecution()
	list := types.NewList([]types.Value{types.NewInt(1)})
	value := types.NewInt(2)
	limit := ValueBytes(list.InsertAt(1, value))
	ctx.PendingEffects = []kernel.PendingEffect{{
		Kind: kernel.PendingEffectServerOptions,
		ServerOptions: kernel.PendingServerOptions{
			MaxListValueBytes: limit,
		},
	}}

	result := builtinListinsert(ctx, []types.Value{list, value})
	if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
		t.Fatalf("listinsert at pending byte limit = flow %v error %v value %v, want E_QUOTA", result.Flow, result.Error, result.Val)
	}
}

func TestListappendUsesPendingListValueByteLimit(t *testing.T) {
	ctx := newTestExecution()
	list := types.NewList([]types.Value{types.NewInt(1)})
	value := types.NewInt(2)
	limit := ValueBytes(list.InsertAt(list.Len()+1, value))
	ctx.PendingEffects = []kernel.PendingEffect{{
		Kind: kernel.PendingEffectServerOptions,
		ServerOptions: kernel.PendingServerOptions{
			MaxListValueBytes: limit,
		},
	}}

	result := builtinListappend(ctx, []types.Value{list, value})
	if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
		t.Fatalf("listappend at pending byte limit = flow %v error %v value %v, want E_QUOTA", result.Flow, result.Error, result.Val)
	}
}

func TestListsetUsesPendingListValueByteLimit(t *testing.T) {
	ctx := newTestExecution()
	list := types.NewList([]types.Value{types.NewInt(1)})
	value := types.NewList([]types.Value{types.NewInt(2)})
	limit := ValueBytes(list.Set(1, value))
	ctx.PendingEffects = []kernel.PendingEffect{{
		Kind: kernel.PendingEffectServerOptions,
		ServerOptions: kernel.PendingServerOptions{
			MaxListValueBytes: limit,
		},
	}}

	result := builtinListset(ctx, []types.Value{list, value, types.NewInt(1)})
	if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
		t.Fatalf("listset at pending byte limit = flow %v error %v value %v, want E_QUOTA", result.Flow, result.Error, result.Val)
	}
}
