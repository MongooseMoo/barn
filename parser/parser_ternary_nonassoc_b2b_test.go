package parser

import "testing"

// B2b: Toast declares `?`/`|` as %nonassoc. These cases were captured live from
// ToastStunt (toastcore.db) via set_verb_code:
//
//	a ? b | c                 ACCEPT
//	a ? b ? c | d | e         ACCEPT   (ternary in CONSEQUENT/middle position)
//	a ? (b ? c | d) | e       ACCEPT
//	1 == 2 ? 3 | (4 == 5 ? 6 | 7)   ACCEPT
//	(a ? b | c) ? d | e       ACCEPT   (parenthesized condition)
//	1 + (a ? b | c)           ACCEPT
//	a ? b | c ? d | e         REJECT   (ternary in ELSE position, no parens)
//	1 == 2 ? 3 | 4 == 5 ? 6 | 7      REJECT
//	(a ? b | c) chained as `?` condition without parens   REJECT
func TestTernaryNonAssociative_B2b(t *testing.T) {
	accept := []string{
		"a ? b | c",
		"a ? b ? c | d | e",
		"a ? (b ? c | d) | e",
		"1 == 2 ? 3 | (4 == 5 ? 6 | 7)",
		"(a ? b | c) ? d | e",
		"1 + (a ? b | c)",
	}
	reject := []string{
		"a ? b | c ? d | e",
		"1 == 2 ? 3 | 4 == 5 ? 6 | 7",
	}

	for _, src := range accept {
		t.Run("accept/"+src, func(t *testing.T) {
			p := NewParser(src)
			if _, err := p.ParseExpression(PREC_LOWEST); err != nil {
				t.Fatalf("expected %q to parse, got error: %v", src, err)
			}
		})
	}
	for _, src := range reject {
		t.Run("reject/"+src, func(t *testing.T) {
			p := NewParser(src)
			if _, err := p.ParseExpression(PREC_LOWEST); err == nil {
				t.Fatalf("expected %q to be a syntax error, but it parsed", src)
			}
		})
	}
}
