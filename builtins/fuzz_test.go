package builtins

import (
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func FuzzStringBuiltins(f *testing.F) {
	// Producer: player-reachable MOO verbs can pass strings to these public
	// registrations: json.go:622-623 (generate_json, parse_json), crypto.go:1505-1507
	// (decode_base64, encode_binary, decode_binary), url.go:70 (url_decode),
	// ansi.go:82 (parse_ansi), pcre.go:293-294 (pcre_match, pcre_replace), and
	// strings.go:808-810 (match, rmatch, substitute).
	names := [...]string{
		"parse_json", "generate_json", "decode_binary", "encode_binary",
		"decode_base64", "url_decode", "parse_ansi", "pcre_match",
		"pcre_replace", "match", "rmatch", "substitute",
	}
	for selector := range names {
		for _, input := range []string{
			"", "\x00\xff\xfe\xc0\x80", "héllo😀", "~", "~e", "~00~09~7E~FF", "%", "%00%ff%GG",
			"[red]hello[normal][unknown]", "\x1b[31mhello\x1b[0m",
		} {
			f.Add(byte(selector), input, "", "")
		}
	}
	// Existing json_test.go, crypto_test.go, strings_test.go, and
	// pcre_syntax_test.go inputs, plus malformed cases. No seed is built to
	// backtrack catastrophically: each such call runs to Barn's 2 s regexp2
	// timeout, and mutating near one stalled campaigns at 0 execs/sec.
	for _, seed := range []struct {
		selector byte
		input    string
		pattern  string
		template string
	}{
		{0, `null`, "", ""},
		{0, `[null]`, "", ""},
		{0, `"C:\\temp"`, "", ""},
		{0, `"\uD83D\uDE00"`, "", ""},
		{0, `{"x":1,"x":2}`, "", ""},
		{0, strings.Repeat("[", 1024) + "0" + strings.Repeat("]", 1024), "", ""},
		{0, strings.Repeat("{\"x\":", 1024), "", ""},
		{1, "~00~08~09~0a~0C~0d~1F~20~ff", "", ""},
		{1, `\~0a`, "", ""},
		{2, "xx", "", ""},
		{3, "\xc8", "", ""},
		{4, "SGVsbG8=", "", ""},
		{4 + 12, "_-4", "", ""},
		{5, "%20+%2F%00tail%", "", ""},
		{7, "see https://example.com/x, now", mongooseURLRe, ""},
		{7, "price: $42 or $7", `(?<=\$)\d+`, ""},
		{7, "abcabc xyzxy", `(\w{3})\1`, ""},
		{7, "a", `[`, ""},
		{7 + 12, "aaa", `a*`, ""},
		{8, "price: $42", `s/(?<=\$)\d+/99/`, ""},
		{8, "aaa", `s/a/$&$1/g`, ""},
		{9, ".", `%.`, ""},
		{9, "foo bar", `%bbar`, ""},
		{10, "foo bar", `foo%>`, ""},
		{11, "abc", `%(a%)`, "group=[%1] %% %0 %9 %"},
		{11, "abc", `[`, "x"},
	} {
		f.Add(seed.selector, seed.input, seed.pattern, seed.template)
	}
	// One Execution per fuzz process: building the registry and session took
	// 11-18 ms, far more than the calls. Execs in a worker run sequentially,
	// and these builtins only read the session (crypto.go:87 rewrites the
	// same MaxStringConcat each time).
	ctx := newTestExecution()
	f.Fuzz(func(t *testing.T, selector byte, input, pattern, template string) {
		// Keep allocations and quadratic suffix scans bounded while retaining
		// deep JSON and adversarial backtracking inputs. Do not repair UTF-8.
		if len(input) > 8192 || len(pattern) > 8192 || len(template) > 8192 {
			return
		}
		call := func(name string, args ...types.Value) types.Result {
			// Toast src/pcre_moo.cc:45-53 bounds PCRE2 at 10,000,000 match
			// steps and depth 10,000. Barn's regexp2 uses pcreMatchTimeout
			// instead; those engine-specific counters are not wall-clock units.
			// This watchdog detects a stuck call, including compilation and
			// repeated matches, without abandoning a worker goroutine. A panic
			// kills the fuzz worker so Go saves the offending input.
			watchdog := time.AfterFunc(10*time.Second, func() {
				panic("string builtin exceeded 10s: " + name)
			})
			defer watchdog.Stop()
			return admitTestBuiltin(name, ctx, args)
		}
		str := types.NewStr(input)
		pat := types.NewStr(pattern)
		flag := types.NewInt(int64(selector/byte(len(names))) & 1)
		switch name := names[int(selector)%len(names)]; name {
		case "parse_json":
			_ = call(name, str)
			_ = call(name, str, types.NewStr("embedded-types"))
		case "generate_json":
			// Arbitrary strings are only checked for panic/termination: Toast's
			// binary escapes and JSON string escaping need not preserve bytes.
			_ = call(name, str, types.NewStr("common-subset"), flag)
			_ = call(name, str, types.NewStr("embedded-types"), flag)
			// Toast src/json.cc:457-458,490-504 emits integers, booleans and
			// recursive lists; :203-229,351-376 restores those same types.
			// Exclude floats, strings, objects, errors and map-key conversions.
			items := make([]types.Value, len(input))
			for i := 0; i < len(input); i++ {
				items[i] = types.NewInt(int64(input[i]))
			}
			value := types.NewList([]types.Value{
				types.NewInt(int64(len(input))), types.NewBool(flag.Truthy()),
				types.NewList(items),
			})
			encoded := call(name, value)
			if !encoded.IsNormal() {
				t.Fatalf("generate_json: %v", encoded)
			}
			decoded := call("parse_json", encoded.Val)
			if !decoded.IsNormal() || !decoded.Val.Identical(value) {
				t.Fatalf("JSON round trip: %v", decoded)
			}
		case "decode_binary":
			_ = call(name, str, flag)
		case "encode_binary":
			_ = call(name, str, types.NewStr(pattern), types.NewStr(template))
			// Toast src/list.cc:1549-1552,1579-1583 writes byte integers;
			// src/utils.cc:672-682,700-732 encodes/decodes every byte; and
			// src/list.cc:1485-1489 returns integer bytes with fully=true.
			// Use integer input to include NUL without C-string truncation.
			items := make([]types.Value, len(input))
			for i := 0; i < len(input); i++ {
				items[i] = types.NewInt(int64(input[i]))
			}
			value := types.NewList(items)
			encoded := call(name, value)
			if !encoded.IsNormal() {
				t.Fatalf("encode_binary: %v", encoded)
			}
			decoded := call("decode_binary", encoded.Val, types.NewInt(1))
			if !decoded.IsNormal() || !decoded.Val.Identical(value) {
				t.Fatalf("binary round trip: %v", decoded)
			}
		case "decode_base64":
			_ = call(name, str, flag)
		case "url_decode", "parse_ansi":
			_ = call(name, str)
		case "pcre_match":
			_ = call(name, str, pat, flag, types.NewInt(int64(selector/24)&1))
		case "pcre_replace":
			// Raw specs cover malformed delimiters; constructed specs let
			// mutations of the replacement string reach the regex engine too.
			_ = call(name, str, pat)
			_ = call(name, str, types.NewStr("s/"+pattern+"/"+template+"/g"))
		case "match", "rmatch":
			_ = call(name, str, pat, flag)
		case "substitute":
			matched := call("match", str, pat, flag)
			if matched.IsNormal() {
				_ = call(name, types.NewStr(template), matched.Val)
			}
			// Player code may also supply its own malformed match list.
			_ = call(name, types.NewStr(template), types.NewList([]types.Value{str, pat}))
		}
	})
}
