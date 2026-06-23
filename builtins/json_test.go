package builtins

import (
	"testing"

	"barn/kernel"
	"barn/types"
)

func TestParseJsonNullMapsToENone(t *testing.T) {
	ctx := kernel.NewTaskContext()

	res := builtinParseJson(ctx, []types.Value{types.NewStr(`null`)})
	if res.IsError() {
		t.Fatalf("parse_json(null) failed: %v", res.Error)
	}
	errVal, ok := res.Val.(types.ErrValue)
	if !ok {
		t.Fatalf("parse_json(null) = %T, want ErrValue", res.Val)
	}
	if errVal.Code() != types.E_NONE {
		t.Fatalf("parse_json(null) = %v, want E_NONE", errVal.Code())
	}

	res = builtinParseJson(ctx, []types.Value{types.NewStr(`[null]`)})
	if res.IsError() {
		t.Fatalf("parse_json([null]) failed: %v", res.Error)
	}
	list, ok := res.Val.(types.ListValue)
	if !ok {
		t.Fatalf("parse_json([null]) = %T, want ListValue", res.Val)
	}
	elem, ok := list.Get(1).(types.ErrValue)
	if !ok || elem.Code() != types.E_NONE {
		t.Fatalf("parse_json([null])[1] = %#v, want E_NONE", list.Get(1))
	}
}
