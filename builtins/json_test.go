package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestGenerateJsonModeAndBinaryEscapes(t *testing.T) {
	ctx := newTestExecution()
	for _, mode := range []string{"common-subset", "CoMmOn-SuBsEt", "embedded-types", "EmBeDdEd-TyPeS"} {
		for _, test := range []struct {
			flag     types.Value
			disabled bool
		}{
			{types.NewInt(0), false},
			{types.NewInt(1), true},
			{types.NewList(nil), false},
			{types.NewList([]types.Value{types.NewInt(0)}), true},
			{types.NewStr(""), false},
			{types.NewStr("yes"), true},
		} {
			t.Run(mode+"/"+test.flag.String(), func(t *testing.T) {
				value := types.NewList([]types.Value{
					types.NewStr("~00~08~09~0a~0C~0d~1F~20~ff"),
					types.NewMap([][2]types.Value{{types.NewStr("~09"), types.NewStr("~0a")}}),
					types.NewStr(`\~0a`),
				})
				want := `["\u0000\b\u0009\n\f\r\u001F~20~ff",{"\u0009":"\n"},"~0a"]`
				if test.disabled {
					want = `["~00~08~09~0a~0C~0d~1F~20~ff",{"~09":"~0a"},"~0a"]`
				}
				res := builtinGenerateJson(ctx, []types.Value{value, types.NewStr(mode), test.flag})
				if res.IsError() || res.Val.Str() != want {
					t.Fatalf("generate_json = %v (%v), want %s", res.Val, res.Error, want)
				}
			})
		}
	}
	for _, mode := range []string{"", "pretty", "pretty-embedded", "not-embedded-types", "common-subset "} {
		t.Run("invalid/"+mode, func(t *testing.T) {
			res := builtinGenerateJson(ctx, []types.Value{types.NewInt(1), types.NewStr(mode)})
			if !res.IsError() || res.Error != types.E_INVARG {
				t.Fatalf("generate_json = %v (%v), want E_INVARG", res.Val, res.Error)
			}
		})
	}
	res := builtinGenerateJson(ctx, []types.Value{types.NewStr("~0a")})
	if res.IsError() || res.Val.Str() != `"\n"` {
		t.Fatalf("default generate_json = %v (%v)", res.Val, res.Error)
	}
	info := builtinFunctionInfo(ctx, []types.Value{types.NewStr("generate_json")})
	wantTypes := types.NewList([]types.Value{types.NewInt(-1), types.NewInt(2), types.NewInt(-1)})
	if info.IsError() || info.Val.Get(2).Int() != 1 || info.Val.Get(3).Int() != 3 || info.Val.Get(4).String() != wantTypes.String() {
		t.Fatalf("function_info(generate_json) = %v (%v)", info.Val, info.Error)
	}
}

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
