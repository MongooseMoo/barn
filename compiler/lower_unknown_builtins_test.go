package compiler

import (
	"slices"
	"testing"
)

func TestLowerUnknownBuiltinCallsRewritesToCallFunction(t *testing.T) {
	compiler := New(map[string]int{"length": 0, "call_function": 1})
	lowered, warnings := compiler.LowerUnknownBuiltinCalls([]string{
		"try",
		`return {"called", Missing_One(1, length(args))};`,
		"except e (ANY)",
		"return e;",
		"endtry",
	})
	want := []string{
		"try",
		`  return {"called", call_function("Missing_One", 1, length(args))};`,
		"except e (ANY)",
		"  return e;",
		"endtry",
	}
	if !slices.Equal(lowered, want) {
		t.Fatalf("lowered = %q, want %q", lowered, want)
	}
	if len(warnings) != 1 || warnings[0].Error() != "Line 2:  Unknown built-in function: Missing_One" {
		t.Fatalf("warnings = %v", warnings)
	}
	if _, diagnostics := compiler.CompileMOO(lowered); len(diagnostics) != 0 {
		t.Fatalf("lowered source does not compile: %v", diagnostics)
	}
}

func TestLowerUnknownBuiltinCallsWarnsInnerCallFirst(t *testing.T) {
	_, warnings := New(map[string]int{"call_function": 0}).LowerUnknownBuiltinCalls([]string{"missing_outer(missing_inner());"})
	var got []string
	for _, warning := range warnings {
		got = append(got, warning.Error())
	}
	want := []string{"Line 1:  Unknown built-in function: missing_inner", "Line 1:  Unknown built-in function: missing_outer"}
	if !slices.Equal(got, want) {
		t.Fatalf("warnings = %q, want %q", got, want)
	}
}

func TestLowerUnknownBuiltinCallsLeavesOtherSourceAlone(t *testing.T) {
	compiler := New(map[string]int{"length": 0, "call_function": 1})
	for _, source := range [][]string{
		{"return length(args);"},
		{"return missing(;"},
	} {
		if lowered, warnings := compiler.LowerUnknownBuiltinCalls(source); lowered != nil || warnings != nil {
			t.Fatalf("LowerUnknownBuiltinCalls(%q) = %q, %v; want nil", source, lowered, warnings)
		}
	}
}
