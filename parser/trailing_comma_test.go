package parser

import "testing"

// MOO's grammar admits no trailing comma in a list, map or scatter: Toast
// compiles each of these to "syntax error" (issue #871, where Barn ran
// `{a,} = {1}; return rest;` and raised E_VARNF instead).
func TestTrailingCommaIsSyntaxError(t *testing.T) {
	for _, source := range []string{
		"{a,} = {1}; return rest;",
		"{a, b,} = {1, 2};",
		"{a, ?b,} = {1};",
		"{a, @r,} = {1};",
		"return {1, 2, 3,};",
		"return {@a,};",
		`return ["a" -> 1, "b" -> 2,];`,
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := NewParser(source).ParseProgram(); err == nil {
				t.Fatalf("ParseProgram(%q) succeeded, want syntax error", source)
			}
		})
	}
}
