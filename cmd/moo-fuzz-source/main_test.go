package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestListBuiltinCalls(t *testing.T) {
	input := strings.Join([]string{
		`"x = LENGTH(\"ab\"); if (x) return {tostr(x), call_function(\"Abs\", -1)}; endif"`,
		`"try fork (0) suspend(0); endfork except (ANY) return [1 -> min(1, 2)]; endtry"`,
		`"return \"length(1)\";"`,
		`"return f(;"`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := listBuiltinCalls(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{
		`{"builtins":["abs","call_function","length","tostr"]}`,
		`{"builtins":["min","suspend"]}`,
		`{"builtins":[]}`,
	}
	if len(lines) != 4 {
		t.Fatalf("got %d lines: %q", len(lines), out.String())
	}
	for i, line := range want {
		if lines[i] != line {
			t.Errorf("line %d = %s, want %s", i+1, lines[i], line)
		}
	}
	if !strings.HasPrefix(lines[3], `{"builtins":[],"error":`) {
		t.Errorf("line 4 = %s, want a parse error", lines[3])
	}
}
