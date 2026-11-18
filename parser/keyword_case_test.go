package parser

import "testing"

// Toast matches keywords in any letter case, so `ANY', `IN' and `Return' are
// keywords and never variables or property names (issue #880, where Barn ran
// `return {"c", @`pass() !"c", @ ANY => {}'};` and raised E_VARNF on the
// variable ANY instead of failing to compile). Each case below was checked
// on the pinned Toast oracle.
func TestKeywordsMatchInAnyCase(t *testing.T) {
	for _, source := range []string{
		"return {\"c\", @`pass() !\"c\", @ ANY => {}'};",
		"return ANY;",
		"return {Any};",
		"return `1 ! \"c\", ANY => 2';",
		"return `1 ! E_INVARG, @ANY => 2';",
		"return `1 ! @ANY => 2';",
		"try return 1; except (E_INVARG, ANY) return 2; endtry",
		"ANY = 3; return ANY;",
		"{ANY} = {1};",
		"for ANY in ({1}) endfor",
		"x = $nothing; return x.ANY;",
		"x = $nothing; return x.IN;",
		"x = $nothing; return x:ANY();",
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := NewParser(source).ParseProgram(); err == nil {
				t.Fatalf("ParseProgram(%q) succeeded, want syntax error", source)
			}
		})
	}
	for _, source := range []string{
		"return `1 ! ANY => 2';",
		"return `1 ! Any => 2';",
		"try return 1; except (any) return 2; endtry",
		"IF (1) return 2; ENDIF",
		"RETURN 5;",
		"For x in ({1}) endfor return 7;",
		"return 1 IN {1};",
		`return ["ANY" -> 1];`,
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := NewParser(source).ParseProgram(); err != nil {
				t.Fatalf("ParseProgram(%q): %v, want success", source, err)
			}
		})
	}
}
