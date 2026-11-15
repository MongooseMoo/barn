package builtins

import (
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// Toast links PCRE2 10.42, which compiles a lookbehind only when each of its
// top-level branches has a fixed width. Every case below is Toast-green in
// moo-conformance-tests builtins/pcre.yaml (pcre_lookbehind_*).
func TestPCRELookbehindWidthRule(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		ok      bool
	}{
		{`(?<=\$)\d+`, true},
		{mongooseURLRe, true},
		{`(?<=ab|c)d`, true},
		{`(?<=(?:ab){2})c`, true},
		{`(?<=a|)b`, true},
		{`(?<=^a{2}\b)b|(?<=\ba{2,2})b`, true},
		{`(?<=a(?=b+))b`, true},
		{`(?<=\Qa+\E)b`, true},
		{`(?<=x{y[]])b`, true},
		{`(a)(?<=\1)b`, true},
		{`(x)?(?<=(?(1)a|b))b`, true},
		{`(?<=a+)b`, false},
		{`(?<!x(..?.)+).`, false},
		{`(?<=a?)b`, false},
		{`(?<=a{1,3})b`, false},
		{`(?<=a(b|cd))e`, false},
		{`(?<=a*)b`, false},
		{`(a+)(?<=\1)b`, false},
		{`(x)?(?<=(?(1)a|bc))b`, false},
		{`(?<=` + strings.Repeat("a", pcreMaxLookbehind+1) + `)b`, false},
	} {
		res := admitTestBuiltin("pcre_match", newTestExecution(), []types.Value{
			types.NewStr("aab"), types.NewStr(tc.pattern),
		})
		if got := res.Error == types.E_NONE; got != tc.ok {
			shown := tc.pattern[:min(len(tc.pattern), 40)]
			t.Errorf("pcre_match(%q) compiled=%v, want %v (%v)", shown, got, tc.ok, res.Error)
		}
	}
}

// Issue #843: a FuzzStringBuiltins input ran pcre_match and a global
// pcre_replace with this unbounded negative lookbehind. regexp2 accepted it and
// re-ran the lookbehind backwards from every subject position, each call
// taking 5-6 s, long enough under load for the fuzz worker to be reported as
// hung. Toast's PCRE2 rejects the pattern at compile time.
func TestPCREIssue843VariableLookbehindRejectedQuickly(t *testing.T) {
	subject := "04\xe0g\t\xba0\xf2\xf2\xf2\xf2\xf20000000\xcd\xcd\xcd\xcd\xcd0000\x01\x03\xcc\xe6\x0e\xa0\xa0\xff\xb0H\x17\x93\x930݁Y\x04\xd9<gr9\xf80\x1e\xa1g\xb9\n\x0f\xb4\x8a\xb4\x11U\xea!\x9f\x9f\xaf\x1e\t\xff\x7f\n~i̠\xa0\xa0\xa0\xa0g\xb9\n\x0f\xb46\xa4\xf9"
	pattern := "(?<!p(\x16\x00?\x01\x000+(..?.)+).)"
	ctx := newTestExecution()
	for _, call := range []struct {
		name string
		args []types.Value
	}{
		{"pcre_match", []types.Value{types.NewStr(subject), types.NewStr(pattern)}},
		{"pcre_replace", []types.Value{types.NewStr(subject), types.NewStr("s/" + pattern + "/\xed\xedw/g")}},
	} {
		start := time.Now()
		res := admitTestBuiltin(call.name, ctx, call.args)
		if res.Error != types.E_INVARG {
			t.Errorf("%s: got %v, want E_INVARG", call.name, res.Error)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("%s took %v", call.name, elapsed)
		}
	}
}
