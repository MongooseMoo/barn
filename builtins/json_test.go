package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestParseJsonNullMapsToENone(t *testing.T) {
	ctx := newTestExecution()

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

func TestParseJsonToastEscapeSemantics(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "escaped backslash before t", json: `"a\\temp"`, want: `a\temp`},
		{name: "tab short escape", json: `"a\tb"`, want: "a\tb"},
		{name: "tab unicode escape", json: `"a\u0009b"`, want: "a\tb"},
		{name: "newline short escape", json: `"a\nb"`, want: "a~0Ab"},
		{name: "windows path", json: `"C:\\temp"`, want: `C:\temp`},
		{name: "backspace short escape", json: `"a\bb"`, want: "a~08b"},
		{name: "form feed short escape", json: `"a\fb"`, want: "a~0Cb"},
		{name: "carriage return short escape", json: `"a\rb"`, want: "a~0Db"},
		{name: "surrogate pair", json: `"\uD83D\uDE00"`, want: "~F0~9F~98~80"},
		{name: "private use character", json: `"\uE000"`, want: "~EE~80~80"},
	}

	ctx := newTestExecution()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := builtinParseJson(ctx, []types.Value{types.NewStr(test.json)})
			if res.IsError() {
				t.Fatalf("parse_json(%q) failed: %v", test.json, res.Error)
			}
			if got := res.Val.Str(); got != test.want {
				t.Fatalf("parse_json(%q) bytes = %v, want %v", test.json, []byte(got), []byte(test.want))
			}
		})
	}
}
