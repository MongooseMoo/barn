package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast's regex compiler (regexpr.c, Reol under RE_CONTEXT_INDEP_OPS) accepts
// '$' only at the end of a pattern or before "%|" or "%)"; any other '$'
// outside a bracket class makes match() and rmatch() raise E_INVARG (#1142).
// Expectations were taken from the stock Toast oracle in emergency mode.
func TestBuiltinMatchMisplacedEndAnchor(t *testing.T) {
	tests := []struct {
		subject, pattern string
		invalid          bool
	}{
		{subject: "$$ b", pattern: "e\t% $b", invalid: true},
		{subject: "ab", pattern: "a$b", invalid: true},
		{subject: "x", pattern: "$b", invalid: true},
		{subject: "a", pattern: "a$$", invalid: true},
		{subject: "a", pattern: "a$%%", invalid: true},
		{subject: "a", pattern: "a$"},
		{subject: "a", pattern: "$"},
		{subject: "a", pattern: "a$%|b"},
		{subject: "a", pattern: "%(a$%)"},
		{subject: "x", pattern: "%$b"},
		{subject: "a$", pattern: "[$]"},
	}

	ctx := newTestExecution()
	for _, name := range []string{"match", "rmatch"} {
		for _, tc := range tests {
			result := admitTestBuiltin(name, ctx, []types.Value{types.NewStr(tc.subject), types.NewStr(tc.pattern)})
			if tc.invalid {
				if result.Flow != types.FlowException || result.Error != types.E_INVARG {
					t.Errorf("%s(%q, %q) = flow %v error %v value %v, want E_INVARG",
						name, tc.subject, tc.pattern, result.Flow, result.Error, result.Val)
				}
				continue
			}
			if result.Flow != types.FlowNormal {
				t.Errorf("%s(%q, %q) flow = %v, error = %v, want a value",
					name, tc.subject, tc.pattern, result.Flow, result.Error)
			}
		}
	}
}
