package builtins

import (
	"regexp"
	"testing"
)

func TestMooPatternPercentEscapesAreLiteralByDefault(t *testing.T) {
	pattern, err := mooPatternToGoRegex("%.")
	if err != nil {
		t.Fatalf("mooPatternToGoRegex failed: %v", err)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compiled pattern %q failed: %v", pattern, err)
	}
	if !re.MatchString(".") {
		t.Fatalf("%q did not match literal dot", pattern)
	}
	if re.MatchString("x") {
		t.Fatalf("%q matched non-dot character", pattern)
	}

	pattern, err = mooPatternToGoRegex("%d")
	if err != nil {
		t.Fatalf("mooPatternToGoRegex failed: %v", err)
	}
	re = regexp.MustCompile(pattern)
	if !re.MatchString("d") || re.MatchString("1") {
		t.Fatalf("%%d translated to %q, want literal d", pattern)
	}
}

func TestMooPatternPercentWordBoundaryEscapes(t *testing.T) {
	pattern, err := mooPatternToGoRegex("%bbar")
	if err != nil {
		t.Fatalf("mooPatternToGoRegex failed: %v", err)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compiled pattern %q failed: %v", pattern, err)
	}
	loc := re.FindStringIndex("foo bar")
	if loc == nil || loc[0] != 4 || loc[1] != 7 {
		t.Fatalf("%%bbar matched %v, want [4 7]", loc)
	}

	pattern, err = mooPatternToGoRegex("foo%>")
	if err != nil {
		t.Fatalf("mooPatternToGoRegex failed: %v", err)
	}
	re = regexp.MustCompile(pattern)
	loc = re.FindStringIndex("foo bar")
	if loc == nil || loc[0] != 0 || loc[1] != 3 {
		t.Fatalf("foo%%> matched %v, want [0 3]", loc)
	}
	if re.MatchString("foobar") {
		t.Fatalf("foo%%> matched inside word")
	}
}
