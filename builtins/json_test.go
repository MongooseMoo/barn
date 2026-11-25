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

// Distinct MOO keys can share a JSON member name; generate_json writes both,
// as Toast does, even though parse_json rejects such a document.
func TestGenerateJsonWritesRepeatedMemberNames(t *testing.T) {
	ctx := newTestExecution()
	value := types.NewMap([][2]types.Value{
		{types.NewInt(1), types.NewStr("number")},
		{types.NewStr("1"), types.NewStr("string")},
	})
	res := admitTestBuiltin("generate_json", ctx, []types.Value{value})
	if want := `{"1":"number","1":"string"}`; !res.IsNormal() || res.Val.Str() != want {
		t.Fatalf("generate_json = flow %v error %v value %v, want %s", res.Flow, res.Error, res.Val, want)
	}
}

// Toast's parse_json accepts only "common-subset" and "embedded-types", in any
// case, and raises E_INVARG for any other mode string, even before a valid
// document (issue #1173: parse_json("1", "\t") returned 1).
func TestParseJsonMode(t *testing.T) {
	ctx := newTestExecution()
	doc := types.NewStr(`"#1|obj"`)
	for mode, want := range map[string]string{
		"common-subset":  `"#1|obj"`,
		"CoMmOn-SuBsEt":  `"#1|obj"`,
		"embedded-types": "#1",
		"EMBEDDED-TYPES": "#1",
	} {
		res := admitTestBuiltin("parse_json", ctx, []types.Value{doc, types.NewStr(mode)})
		if res.IsError() || res.Val.String() != want {
			t.Fatalf("parse_json(%q) = %v (%v), want %s", mode, res.Val, res.Error, want)
		}
	}
	for _, mode := range []string{"\t", "", "x", "embedded", "pretty", "not-embedded-types", "common-subset "} {
		res := admitTestBuiltin("parse_json", ctx, []types.Value{types.NewStr("1"), types.NewStr(mode)})
		if !res.IsError() || res.Error != types.E_INVARG {
			t.Fatalf("parse_json(\"1\", %q) = %v (%v), want E_INVARG", mode, res.Val, res.Error)
		}
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

// ToastStunt 2.8 parse_json fails when two members of one object map to the
// same MOO key (handle_end_map in json.cc). These are its test_json.rb cases,
// less the two whose collision depends on Toast decoding escapes to byte
// escape text; Barn's keys are compared as Barn decodes them.
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
		{name: "tab unicode escape", json: `"a\u0009b"`, want: "a\tb"},
		{name: "newline short escape", json: `"a\nb"`, want: "a~0Ab"},
		{name: "windows path", json: `"C:\\temp"`, want: `C:\temp`},
		{name: "backspace short escape", json: `"a\bb"`, want: "a~08b"},
		{name: "form feed short escape", json: `"a\fb"`, want: "a~0Cb"},
		{name: "carriage return short escape", json: `"a\rb"`, want: "a~0Db"},
		{name: "surrogate pair", json: `"\uD83D\uDE00"`, want: "~F0~9F~98~80"},
		{name: "private use character", json: `"\uE000"`, want: "~EE~80~80"},
		// The v1 decoder's internal escape marker began with this text and
		// decoded such a string to "~0A"; it is ordinary text.
		{name: "old escape marker text", json: `"\uE000barn-json-control-0-n~0A"`, want: "~EE~80~80barn-json-control-0-n~7E0A"},
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
