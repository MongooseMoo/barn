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
				res := admitTestBuiltin("generate_json", ctx, []types.Value{value, types.NewStr(mode), test.flag})
				if res.IsError() || res.Val.Str() != want {
					t.Fatalf("generate_json = %v (%v), want %s", res.Val, res.Error, want)
				}
			})
		}
	}
	for _, mode := range []string{"", "pretty", "pretty-embedded", "not-embedded-types", "common-subset "} {
		t.Run("invalid/"+mode, func(t *testing.T) {
			res := admitTestBuiltin("generate_json", ctx, []types.Value{types.NewInt(1), types.NewStr(mode)})
			if !res.IsError() || res.Error != types.E_INVARG {
				t.Fatalf("generate_json = %v (%v), want E_INVARG", res.Val, res.Error)
			}
		})
	}
	res := admitTestBuiltin("generate_json", ctx, []types.Value{types.NewStr("~0a")})
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

	res := admitTestBuiltin("parse_json", ctx, []types.Value{types.NewStr(`null`)})
	if res.IsError() {
		t.Fatalf("parse_json(null) failed: %v", res.Error)
	}
	if res.Val.Type() != types.TYPE_ERR {
		t.Fatalf("parse_json(null) = %T, want ErrValue", res.Val)
	}
	if res.Val.Code() != types.E_NONE {
		t.Fatalf("parse_json(null) = %v, want E_NONE", res.Val.Code())
	}

	res = admitTestBuiltin("parse_json", ctx, []types.Value{types.NewStr(`[null]`)})
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

// jsonBackslash spells JSON \u escapes in test inputs without the escape
// appearing literally in this source file.
const jsonBackslash = `\`

// ToastStunt 2.8 parse_json fails when two members of one object map to the
// same MOO key (handle_end_map in json.cc); these are its test_json.rb cases.
func TestParseJsonRejectsDuplicateObjectKeys(t *testing.T) {
	tests := []struct {
		json string
		mode string
	}{
		{json: `{"a": 1, "a": 2}`},
		{json: `{"roles": [], "roles": ["_admin"]}`},
		{json: `{"a": 1, "b": 2, "a": 3}`},
		{json: `{"x": {"a": 1, "a": 2}}`},
		{json: `[1, {"a": 1, "a": 2}]`},
		{json: `{"a": 1, "A": 2}`},
		{json: `{"~41": 1, "` + jsonBackslash + `u0041": 2}`},
		{json: `{"~0A": 1, "\n": 2}`},
		{json: `{"/": 1, "\/": 2}`},
		{json: `{"a": 1, "a": 2}`, mode: "common-subset"},
		{json: `{"a": 1, "a|str": 2}`, mode: "embedded-types"},
		{json: `{"#5|obj": 1, "5|obj": 2}`, mode: "embedded-types"},
	}

	ctx := newTestExecution()
	for _, test := range tests {
		t.Run(test.json+" "+test.mode, func(t *testing.T) {
			args := []types.Value{types.NewStr(test.json)}
			if test.mode != "" {
				args = append(args, types.NewStr(test.mode))
			}
			res := admitTestBuiltin("parse_json", ctx, args)
			if res.Flow != types.FlowException || res.Error != types.E_INVARG {
				t.Fatalf("parse_json(%q, %q) = flow %v error %v value %v, want E_INVARG", test.json, test.mode, res.Flow, res.Error, res.Val)
			}
		})
	}
}

func TestParseJsonAcceptsDistinctObjectKeys(t *testing.T) {
	tests := []struct {
		json string
		mode string
		want types.Value
	}{
		{json: `{"a": 1, "b": 2}`, want: types.NewMap([][2]types.Value{
			{types.NewStr("a"), types.NewInt(1)},
			{types.NewStr("b"), types.NewInt(2)},
		})},
		{json: `[{"a": 1}, {"a": 2}]`, want: types.NewList([]types.Value{
			types.NewMap([][2]types.Value{{types.NewStr("a"), types.NewInt(1)}}),
			types.NewMap([][2]types.Value{{types.NewStr("a"), types.NewInt(2)}}),
		})},
		{json: `{"a": {"a": 1}}`, want: types.NewMap([][2]types.Value{
			{types.NewStr("a"), types.NewMap([][2]types.Value{{types.NewStr("a"), types.NewInt(1)}})},
		})},
		{json: `{"1|int": 1, "1": 2}`, mode: "embedded-types", want: types.NewMap([][2]types.Value{
			{types.NewInt(1), types.NewInt(1)},
			{types.NewStr("1"), types.NewInt(2)},
		})},
		{json: `{"5|obj": 1}`, mode: "embedded-types", want: types.NewMap([][2]types.Value{
			{types.NewObj(5), types.NewInt(1)},
		})},
	}

	ctx := newTestExecution()
	for _, test := range tests {
		t.Run(test.json+" "+test.mode, func(t *testing.T) {
			args := []types.Value{types.NewStr(test.json)}
			if test.mode != "" {
				args = append(args, types.NewStr(test.mode))
			}
			res := admitTestBuiltin("parse_json", ctx, args)
			if !res.IsNormal() || !res.Val.Equal(test.want) {
				t.Fatalf("parse_json(%q, %q) = flow %v error %v value %v, want %v", test.json, test.mode, res.Flow, res.Error, res.Val, test.want)
			}
		})
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
		// ToastStunt's yajl decodes every \uXXXX below U+0080 to MOO
		// binary escape text (Utf32toUtf8 in yajl_encode.c), and leaves raw
		// text, tildes included, as it is.
		{name: "tab unicode escape", json: `"a\u0009b"`, want: "a~09b"},
		{name: "printable unicode escape", json: `"` + jsonBackslash + `u0041` + jsonBackslash + `u007e"`, want: "~41~7E"},
		{name: "raw tilde", json: `"~41~"`, want: "~41~"},
		{name: "unicode escaped quote and backslash", json: `"` + jsonBackslash + `u0022` + jsonBackslash + `u005c"`, want: "~22~5C"},
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
			res := admitTestBuiltin("parse_json", ctx, []types.Value{types.NewStr(test.json)})
			if res.IsError() {
				t.Fatalf("parse_json(%q) failed: %v", test.json, res.Error)
			}
			if got := res.Val.Str(); got != test.want {
				t.Fatalf("parse_json(%q) bytes = %v, want %v", test.json, []byte(got), []byte(test.want))
			}
		})
	}
}
