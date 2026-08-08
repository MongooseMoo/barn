package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestParseJsonNullMapsToENone(t *testing.T) {
	ctx := kernel.NewTaskContext()

	res := builtinParseJson(ctx, []types.Value{types.NewStr(`null`)})
	if res.IsError() {
		t.Fatalf("parse_json(null) failed: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_ERR {
		t.Fatalf("parse_json(null) = %T, want ErrValue", res.Val)
	}
	if res.Val.Code() != types.E_NONE {
		t.Fatalf("parse_json(null) = %v, want E_NONE", res.Val.Code())
	}

	res = builtinParseJson(ctx, []types.Value{types.NewStr(`[null]`)})
	if res.IsError() {
		t.Fatalf("parse_json([null]) failed: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_LIST {
		t.Fatalf("parse_json([null]) = %T, want ListValue", res.Val)
	}
	elem := res.Val.Get(1)
	if elem.Type() != types.TYPE_ERR || elem.Code() != types.E_NONE {
		t.Fatalf("parse_json([null])[1] = %#v, want E_NONE", res.Val.Get(1))
	}
}
