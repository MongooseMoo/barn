package vm

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Issues #943 and #949: a queued fork whose body began with an empty
// statement reported a line one higher after each checkpoint. Barn numbered
// lines by source position, so the `;` took a line in the uninterrupted run,
// while the header's first line skipped it. Toast numbers a traceback's lines
// by counting statements in the decompiled listing (decompile.cc
// find_hot_node), so the source layout never matters. The expected lines are
// the Toast results of moo-conformance-tests
// language/traceback_line_numbering.yaml.
func TestTracebackLinesCountListedStatements(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		line   int64
	}{
		{"empty statements take no line", "x = 1;\n;\n;\nreturn 1/0;", 2},
		{"blank source line is not counted", "x = 1;\n\nreturn 1/0;", 2},
		{"two statements on one source line", "x = 1; return 1/0;", 2},
		{"expression over several source lines", "x = {1,\n2,\n1/0};", 1},
		{"one-line elseif arm", "if (0) elseif (1) return 1/0; endif", 3},
		{"one-line else with nested if", "if (0) else if (1) return 1/0; endif endif", 4},
		{"except codes on the except line", "try x = 1; except (E_RANGE, 1/0) endtry", 3},
		{"loop body after a blank line", "for x in ({1})\n\ny = 1/0;\nendfor", 2},
		{"empty else is not listed", "if (0) else ; endif\nreturn 1/0;", 3},
		{"fork body after an empty statement", "fork (0)\n;\nx = 1;\nendfork\nreturn 1/0;", 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := newCheckpointResumeRegistry(t)
			program, diagnostics := registry.Compiler().CompileMOO(strings.Split(test.source, "\n"))
			if len(diagnostics) != 0 {
				t.Fatalf("compile: %v", diagnostics)
			}
			results, _, _ := runCheckpointResumeCase(t, registry, program, nil, sourceFuzzTicks, false)
			result := results[0]
			if result.Flow != types.FlowException || result.Val.Type() != types.TYPE_LIST || result.Val.Len() < 4 {
				t.Fatalf("result = flow %v, value %v; want an exception", result.Flow, result.Val)
			}
			stack := result.Val.Get(4)
			if stack.Type() != types.TYPE_LIST || stack.Len() < 1 {
				t.Fatalf("exception value %v has no traceback frame", result.Val)
			}
			frame := stack.Get(1)
			if got := frame.Get(6); got.Type() != types.TYPE_INT || got.Int() != test.line {
				t.Fatalf("traceback line = %v; want %d (frame %v)", got, test.line, frame)
			}
		})
	}
}
