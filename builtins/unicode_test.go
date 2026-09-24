package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func callUnicode(t *testing.T, fn BuiltinFunc, args ...types.Value) types.Result {
	t.Helper()
	return fn(newTestExecution(), args)
}

func expectUnicodeValue(t *testing.T, name string, res types.Result, want string) {
	t.Helper()
	if res.Flow != types.FlowNormal {
		t.Fatalf("%s: flow %v error %v, want %s", name, res.Flow, res.Error, want)
	}
	if got := res.Val.String(); got != want {
		t.Fatalf("%s = %s, want %s", name, got, want)
	}
}

func expectUnicodeError(t *testing.T, name string, res types.Result, want types.ErrorCode) {
	t.Helper()
	if res.Flow != types.FlowException || res.Error != want {
		t.Fatalf("%s: flow %v error %v value %v, want %v", name, res.Flow, res.Error, res.Val, want)
	}
}

var (
	str = types.NewStr
	num = func(n int64) types.Value { return types.NewInt(n) }
)

func TestOrdAndTochar(t *testing.T) {
	expectUnicodeValue(t, "ord(a)", callUnicode(t, builtinOrd, str("a")), "97")
	expectUnicodeValue(t, "ord(€)", callUnicode(t, builtinOrd, str("€")), "8364")
	expectUnicodeValue(t, "ord(😀)", callUnicode(t, builtinOrd, str("😀")), "128512")
	for _, bad := range []string{"", "ab", "e\u0301", "\xe9"} {
		expectUnicodeError(t, "ord("+bad+")", callUnicode(t, builtinOrd, str(bad)), types.E_INVARG)
	}

	expectUnicodeValue(t, "tochar(8364)", callUnicode(t, builtinTochar, num(8364)), `"€"`)
	expectUnicodeValue(t, "tochar(9)", callUnicode(t, builtinTochar, num(9)), "\"\t\"")
	for _, bad := range []int64{0, 10, 27, 127, 0x85, 0xd800, 0xfffe, 0xfdd0, 0x110000, -1} {
		expectUnicodeError(t, "tochar", callUnicode(t, builtinTochar, num(bad)), types.E_INVARG)
	}
	expectUnicodeValue(t, "tochar(name)", callUnicode(t, builtinTochar, str("euro sign")), `"€"`)
	expectUnicodeValue(t, "tochar(cjk)", callUnicode(t, builtinTochar, str("CJK UNIFIED IDEOGRAPH-4E2D")), `"中"`)
	expectUnicodeValue(t, "tochar(hangul)", callUnicode(t, builtinTochar, str("HANGUL SYLLABLE HAN")), `"한"`)
	expectUnicodeError(t, "tochar(unknown)", callUnicode(t, builtinTochar, str("NO SUCH CHARACTER")), types.E_INVARG)
	expectUnicodeError(t, "tochar(cjk non-ideograph)", callUnicode(t, builtinTochar, str("CJK UNIFIED IDEOGRAPH-0041")), types.E_INVARG)
	expectUnicodeError(t, "tochar(list)", callUnicode(t, builtinTochar, types.NewList(nil)), types.E_TYPE)
}

func TestCharname(t *testing.T) {
	for in, want := range map[string]string{
		"A": "LATIN CAPITAL LETTER A",
		"€": "EURO SIGN",
		"中": "CJK UNIFIED IDEOGRAPH-4E2D",
		"한": "HANGUL SYLLABLE HAN",
		"가": "HANGUL SYLLABLE GA",
		"😀": "GRINNING FACE",
	} {
		expectUnicodeValue(t, "charname("+in+")", callUnicode(t, builtinCharname, str(in)), `"`+want+`"`)
		back := callUnicode(t, builtinTochar, str(want))
		expectUnicodeValue(t, "tochar("+want+")", back, `"`+in+`"`)
	}
	for _, bad := range []string{"\t", "\ue000", "ab", "\xe9"} {
		expectUnicodeError(t, "charname", callUnicode(t, builtinCharname, str(bad)), types.E_INVARG)
	}
}

func TestEncodeDecodeChars(t *testing.T) {
	encodeCases := []struct {
		in   types.Value
		enc  string
		want string
	}{
		{str("Σa~"), "utf-8", `"~CE~A3a~7E"`},
		{types.NewList([]types.Value{str("a"), num(931), num(10)}), "UTF-8", `"a~CE~A3~0A"`},
		{str("é"), "latin1", `"~E9"`},
		{str("€"), "cp1252", `"~80"`},
		{str("A"), "utf-16be", `"~00A"`},
		{str("A"), "utf-32le", `"A~00~00~00"`},
		{str("a\xe9"), "utf-8", `"a~E9"`}, // UTF-8 keeps invalid bytes verbatim
	}
	for _, tc := range encodeCases {
		expectUnicodeValue(t, "encode_chars "+tc.enc, callUnicode(t, builtinEncodeChars, tc.in, str(tc.enc)), tc.want)
	}
	expectUnicodeError(t, "encode_chars unencodable", callUnicode(t, builtinEncodeChars, str("€"), str("latin1")), types.E_INVARG)
	expectUnicodeError(t, "encode_chars invalid byte", callUnicode(t, builtinEncodeChars, str("\xe9"), str("latin1")), types.E_INVARG)
	expectUnicodeError(t, "encode_chars surrogate", callUnicode(t, builtinEncodeChars, num(0xd800), str("utf-8")), types.E_INVARG)
	expectUnicodeError(t, "encode_chars unknown encoding", callUnicode(t, builtinEncodeChars, str("a"), str("klingon")), types.E_INVARG)

	expectUnicodeValue(t, "decode_chars fully", callUnicode(t, builtinDecodeChars, str("~ce~a3"), str("utf8"), num(1)), "{931}")
	expectUnicodeValue(t, "decode_chars grouped", callUnicode(t, builtinDecodeChars, str("a~CE~A3~0Ab~09c"), str("utf-8")), "{\"aΣ\", 10, \"b\tc\"}")
	expectUnicodeValue(t, "decode_chars latin1", callUnicode(t, builtinDecodeChars, str("caf~E9"), str("iso-8859-1")), `{"café"}`)
	expectUnicodeValue(t, "decode_chars utf-16 BOM", callUnicode(t, builtinDecodeChars, str("~FE~FF~00A"), str("utf-16")), `{"A"}`)
	expectUnicodeError(t, "decode_chars invalid utf-8", callUnicode(t, builtinDecodeChars, str("~E9"), str("utf-8")), types.E_INVARG)
	expectUnicodeError(t, "decode_chars bad binary", callUnicode(t, builtinDecodeChars, str("~ZZ"), str("utf-8")), types.E_INVARG)
}

func TestStringWidthAndGraphemes(t *testing.T) {
	for in, want := range map[string]string{
		"":                "0",
		"abc":             "3",
		"café":            "4",
		"e\u0301":         "1",
		"漢字":              "4",
		"😀":               "2",
		"👨\u200d👩\u200d👧": "2",
	} {
		expectUnicodeValue(t, "string_width("+in+")", callUnicode(t, builtinStringWidth, str(in)), want)
	}

	expectUnicodeValue(t, "graphemes empty", callUnicode(t, builtinGraphemes, str("")), "{}")
	res := callUnicode(t, builtinGraphemes, str("ne\u0301👨\u200d👩\u200d👧\xe9!"))
	if res.Flow != types.FlowNormal || res.Val.Len() != 5 {
		t.Fatalf("graphemes = %v", res.Val)
	}
	want := []string{"n", "e\u0301", "👨\u200d👩\u200d👧", "\xe9", "!"}
	for i, w := range want {
		if got := res.Val.Get(i + 1).Str(); got != w {
			t.Fatalf("graphemes[%d] = %q, want %q", i+1, got, w)
		}
	}
}

func TestPcreReplacePreservesInvalidBytes(t *testing.T) {
	for _, tc := range []struct{ subject, spec, want string }{
		{"caf\xe9 bar", "s/bar/baz/", "caf\xe9 baz"},
		{"a\xe9b\xffc", "s/b/B/g", "a\xe9B\xffc"},
		{"x\xe9y", "s/x(.)y/[$1]/", "[\xe9]"},
		{"é\xe9", "s/é/e/", "e\xe9"},
	} {
		res := builtinPcreReplace(newTestExecution(), []types.Value{str(tc.subject), str(tc.spec)})
		if res.Flow != types.FlowNormal || res.Val.Str() != tc.want {
			t.Errorf("pcre_replace(%q, %q) = %q (flow %v, %v), want %q", tc.subject, tc.spec, res.Val.Str(), res.Flow, res.Error, tc.want)
		}
	}
}

func TestPcreMatchInvalidBytesAreOneCharacter(t *testing.T) {
	got := pcreMatchList(t, "a\xe9b", `.b`, 0, 0)
	match, pos := pcreCapture(t, got.Get(1), "0")
	if match != "\xe9b" || len(pos) != 2 || pos[0] != 2 || pos[1] != 3 {
		t.Fatalf("got %q at %v", match, pos)
	}
	// An invalid byte is not U+FFFD.
	if got := pcreMatchList(t, "a\xe9b", "�", 0, 0); got.Len() != 0 {
		t.Fatalf("U+FFFD matched an invalid byte: %v", got)
	}
}
