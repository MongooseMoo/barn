package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Barn strings count Unicode code points for every MOO-visible position and
// length. A byte that is not part of valid UTF-8 counts as one character and
// is preserved exactly.

func expectMOO(t *testing.T, code string, want string) {
	t.Helper()
	result := runBytecodeProgram(t, code, nil, nil)
	if result.Flow != types.FlowReturn {
		t.Fatalf("%s\nflow = %v, error = %v; want return %s", code, result.Flow, result.Error, want)
	}
	if got := result.Val.String(); got != want {
		t.Fatalf("%s\n got %s\nwant %s", code, got, want)
	}
}

func expectMOOError(t *testing.T, code string, want types.ErrorCode) {
	t.Helper()
	result := runBytecodeProgram(t, code, nil, nil)
	if result.Flow != types.FlowException || result.Error != want {
		t.Fatalf("%s\nflow = %v, error = %v, value = %v; want %v", code, result.Flow, result.Error, result.Val, want)
	}
}

func TestUnicodeStringLengthAndIndexing(t *testing.T) {
	expectMOO(t, `return length("café€");`, `5`)
	expectMOO(t, `return {"café€"[4], "café€"[5], "café€"[$]};`, `{"é", "€", "€"}`)
	expectMOO(t, `return "café€"[2..4];`, `"afé"`)
	expectMOO(t, `return "café€"[4..$];`, `"é€"`)
	expectMOOError(t, `return "café€"[6];`, types.E_RANGE)
	expectMOOError(t, `return "café€"[2..6];`, types.E_RANGE)
}

func TestUnicodeStringAssignment(t *testing.T) {
	expectMOO(t, `s = "café€"; s[4] = "E"; return s;`, `"cafE€"`)
	expectMOO(t, `s = "cafe"; s[4] = "é"; return s;`, `"café"`)
	expectMOOError(t, `s = "café"; s[4] = "ab"; return s;`, types.E_INVARG)
	expectMOO(t, `s = "café€"; s[4..4] = "ee"; return s;`, `"cafee€"`)
	expectMOO(t, `s = "café€"; s[5..$] = "!"; return s;`, `"café!"`)
}

func TestUnicodeStringIteration(t *testing.T) {
	expectMOO(t, `l = {}; for c in ("né€") l = {@l, c}; endfor return l;`, `{"n", "é", "€"}`)
	expectMOO(t, `l = {}; for c, i in ("né€") l = {@l, {c, i}}; endfor return l;`, `{{"n", 1}, {"é", 2}, {"€", 3}}`)
	// The $string_utils:char_list idiom must agree with for-in.
	expectMOO(t, `s = "né€"; l = {}; for i in [1..length(s)] l = {@l, s[i]}; endfor return l;`, `{"n", "é", "€"}`)
}

func TestUnicodeStringSearchBuiltins(t *testing.T) {
	expectMOO(t, `return {index("café€", "€"), rindex("é€é", "é"), index("CAFÉ", "é")};`, `{5, 3, 4}`)
	// $string_utils:substitute's s[i + length(needle)..$] surgery stays exact.
	expectMOO(t, `s = "é=€;x"; i = index(s, "€;"); return s[i + length("€;")..$];`, `"x"`)
	expectMOO(t, `return strsub("CAFÉ café", "É", "e");`, `"CAFe cafe"`)
	expectMOO(t, `return reverse("aé€");`, `"€éa"`)
	expectMOO(t, `return strtr("café", "é", "e");`, `"cafe"`)
}

func TestUnicodeStringMatchPositions(t *testing.T) {
	expectMOO(t, `m = match("naïve", "ï"); return {m[1], m[2]};`, `{3, 3}`)
	expectMOO(t, `m = match("é€xé€", "€"); return {m[1], m[2]};`, `{2, 2}`)
	expectMOO(t, `m = rmatch("é€xé€", "€"); return {m[1], m[2]};`, `{5, 5}`)
	expectMOO(t, `return substitute("[%1]", match("naïve!", "%(ï.%)"));`, `"[ïv]"`)
	expectMOO(t, `return pcre_match("naïve", "ï")[1]["0"]["position"];`, `{3, 3}`)
}

func TestInvalidBytesAreSingleCharactersAndPreserved(t *testing.T) {
	// "a\xe9b" holds a lone Latin-1 byte: three characters.
	cases := []struct {
		code string
		want string
	}{
		{"return length(\"a\xe9b\");", "3"},
		{"return \"a\xe9b\"[2] == \"\xe9\";", "1"},
		{"return reverse(\"a\xe9b\") == \"b\xe9a\";", "1"},
		{"l = {}; for c in (\"a\xe9b\") l = {@l, c}; endfor return l[2] == \"\xe9\";", "1"},
		{"s = \"a\xe9b\"; s[1] = \"é\"; return s == \"é\xe9b\";", "1"},
	}
	for _, tc := range cases {
		expectMOO(t, tc.code, tc.want)
	}
}
