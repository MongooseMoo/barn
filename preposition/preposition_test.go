package preposition

import "testing"

func TestPrepositionDefinitions(t *testing.T) {
	wantCanonical := []string{
		"with/using", "at/to", "in front of", "in/inside/into",
		"on top of/on/onto/upon", "out of/from inside/from", "over",
		"through", "under/underneath/beneath", "behind", "beside",
		"for/about", "is", "as", "off/off of",
	}
	prepositions := All()
	if len(prepositions) != len(wantCanonical) {
		t.Fatalf("Prepositions() returned %d entries, want %d", len(prepositions), len(wantCanonical))
	}
	for code, prep := range prepositions {
		if got, ok := prep.Code(); !ok || got != code {
			t.Errorf("preposition %d Code() = %d, %v", code, got, ok)
		}
		if got, ok := FromCode(code); !ok || got != prep {
			t.Errorf("PrepositionFromCode(%d) = %d, %v", code, got, ok)
		}
		canonical, ok := prep.Canonical()
		if !ok || canonical != wantCanonical[code] {
			t.Errorf("preposition %d Canonical() = %q, %v; want %q", code, canonical, ok, wantCanonical[code])
		}
		if parsed, ok := Parse(canonical); !ok || parsed != prep {
			t.Errorf("ParsePreposition(%q) = %d, %v; want %d", canonical, parsed, ok, prep)
		}
		if len(prep.Aliases()) > 1 {
			if _, ok := ParseAlias(canonical); ok {
				t.Errorf("ParsePrepositionAlias(%q) accepted slash-delimited spelling", canonical)
			}
		}
		for _, alias := range prep.Aliases() {
			if parsed, ok := ParseAlias(alias); !ok || parsed != prep {
				t.Errorf("ParsePreposition(%q) = %d, %v; want %d", alias, parsed, ok, prep)
			}
		}
	}
}

func TestSpecialAndUnknownPrepositions(t *testing.T) {
	for input, want := range map[string]Spec{
		"none": None,
		"NONE": None,
		"any":  Any,
		"ANY":  Any,
	} {
		got, ok := ParseAlias(input)
		if !ok || got != want {
			t.Errorf("ParsePreposition(%q) = %d, %v; want %d, true", input, got, ok, want)
		}
		canonical, ok := got.Canonical()
		if !ok || canonical != inputLower(input) {
			t.Errorf("Canonical() = %q, %v", canonical, ok)
		}
	}

	if got, ok := Parse("toward"); ok || got != None {
		t.Errorf("unknown ParsePreposition = %d, %v; want none, false", got, ok)
	}
	for _, code := range []int{-3, 15, 99} {
		if _, ok := FromCode(code); ok {
			t.Errorf("PrepositionFromCode(%d) unexpectedly succeeded", code)
		}
	}
}

func inputLower(input string) string {
	if input == "NONE" {
		return "none"
	}
	if input == "ANY" {
		return "any"
	}
	return input
}

func TestAliasesReturnsCopy(t *testing.T) {
	aliases := With.Aliases()
	aliases[0] = "changed"
	if got := With.Aliases()[0]; got != "with" {
		t.Fatalf("Aliases exposed canonical table: got %q", got)
	}
}
