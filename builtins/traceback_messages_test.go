package builtins

import (
	"barn/types"
	"strings"
	"testing"
)

func TestBuiltinArgsMessageUsesSignature(t *testing.T) {
	reg := NewRegistry()
	id, ok := reg.GetID("abs")
	if !ok {
		t.Fatal("abs builtin not registered")
	}

	result := reg.CallByID(id, &types.TaskContext{}, []types.Value{})
	if result.Flow != types.FlowException || result.Error != types.E_ARGS {
		t.Fatalf("expected E_ARGS exception, got flow=%v err=%v", result.Flow, result.Error)
	}

	msg, ok := result.Val.(types.StrValue)
	if !ok {
		t.Fatalf("expected string message value, got %T", result.Val)
	}
	if !strings.Contains(msg.Value(), "expected 1; got 0") {
		t.Fatalf("expected detailed args message, got: %q", msg.Value())
	}
}

func TestBuiltinTypeMessageUsesSignature(t *testing.T) {
	reg := NewRegistry()
	id, ok := reg.GetID("upcase")
	if !ok {
		t.Fatal("upcase builtin not registered")
	}

	result := reg.CallByID(id, &types.TaskContext{}, []types.Value{types.NewInt(7)})
	if result.Flow != types.FlowException || result.Error != types.E_TYPE {
		t.Fatalf("expected E_TYPE exception, got flow=%v err=%v", result.Flow, result.Error)
	}

	msg, ok := result.Val.(types.StrValue)
	if !ok {
		t.Fatalf("expected string message value, got %T", result.Val)
	}
	text := msg.Value()
	if !strings.Contains(text, "args[1] of upcase()") {
		t.Fatalf("expected argument index/function context, got: %q", text)
	}
	if !strings.Contains(text, "expected string; got integer") {
		t.Fatalf("expected expected/got types, got: %q", text)
	}
}

func TestBuiltinDetailedMessageIsPreserved(t *testing.T) {
	reg := NewRegistry()
	reg.Register("custom_detailed", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return types.Result{
			Flow:  types.FlowException,
			Error: types.E_TYPE,
			Val:   types.NewStr("already detailed"),
		}
	})

	id, ok := reg.GetID("custom_detailed")
	if !ok {
		t.Fatal("custom_detailed builtin not registered")
	}

	result := reg.CallByID(id, &types.TaskContext{}, []types.Value{types.NewInt(1)})
	msg, ok := result.Val.(types.StrValue)
	if !ok {
		t.Fatalf("expected string message value, got %T", result.Val)
	}
	if msg.Value() != "already detailed" {
		t.Fatalf("expected original message to be preserved, got: %q", msg.Value())
	}
}

func TestBuiltinGenericMessageForOtherErrorCodes(t *testing.T) {
	reg := NewRegistry()
	reg.Register("custom_invarg", func(ctx *types.TaskContext, args []types.Value) types.Result {
		return types.Err(types.E_INVARG)
	})

	id, ok := reg.GetID("custom_invarg")
	if !ok {
		t.Fatal("custom_invarg builtin not registered")
	}

	result := reg.CallByID(id, &types.TaskContext{}, []types.Value{types.NewInt(7)})
	if result.Flow != types.FlowException || result.Error != types.E_INVARG {
		t.Fatalf("expected E_INVARG exception, got flow=%v err=%v", result.Flow, result.Error)
	}

	msg, ok := result.Val.(types.StrValue)
	if !ok {
		t.Fatalf("expected string message value, got %T", result.Val)
	}
	text := msg.Value()
	if !strings.Contains(text, "Invalid argument") {
		t.Fatalf("expected base message, got: %q", text)
	}
	if !strings.Contains(text, "custom_invarg()") {
		t.Fatalf("expected builtin name context, got: %q", text)
	}
	if !strings.Contains(text, "args[1]=7") {
		t.Fatalf("expected argument context, got: %q", text)
	}
}

func TestCallFunctionPathUsesEnrichedMessages(t *testing.T) {
	reg := NewRegistry()
	id, ok := reg.GetID("call_function")
	if !ok {
		t.Fatal("call_function builtin not registered")
	}

	result := reg.CallByID(id, &types.TaskContext{}, []types.Value{types.NewStr("abs")})
	if result.Flow != types.FlowException || result.Error != types.E_ARGS {
		t.Fatalf("expected E_ARGS exception, got flow=%v err=%v", result.Flow, result.Error)
	}

	msg, ok := result.Val.(types.StrValue)
	if !ok {
		t.Fatalf("expected string message value, got %T", result.Val)
	}
	if !strings.Contains(msg.Value(), "expected 1; got 0") {
		t.Fatalf("expected enriched args message through call_function, got: %q", msg.Value())
	}
}
