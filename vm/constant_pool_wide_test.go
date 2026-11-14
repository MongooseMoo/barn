package vm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
)

// distinctLiteralProgram appends count distinct string and float literals to
// a list, one statement each, and returns the source with the list it builds.
func distinctLiteralProgram(count int) (source, want string) {
	var code strings.Builder
	code.WriteString("r = {};")
	literals := make([]string, count)
	for i := range count {
		literals[i] = fmt.Sprintf("%q", fmt.Sprintf("k%d", i+1))
		if i%2 == 1 {
			literals[i] = fmt.Sprintf("%d.5", i+1)
		}
		fmt.Fprintf(&code, " r = {@r, %s};", literals[i])
	}
	code.WriteString(" return r;")
	return code.String(), "{" + strings.Join(literals, ", ") + "}"
}

// Toast's literal table has no 256-entry limit (code_gen.cc add_literal,
// ref_size). Constants past index 255 take PUSH_WIDE; programs that fit the
// byte index never do.
func TestProgramsWithMoreThan256ConstantsRun(t *testing.T) {
	for _, tc := range []struct {
		count int
		wide  bool
	}{{256, false}, {257, true}, {600, true}} {
		t.Run(fmt.Sprint(tc.count), func(t *testing.T) {
			source, want := distinctLiteralProgram(tc.count)
			program, diagnostics := BuildVMRegistry().Compiler().CompileMOO([]string{source})
			if len(diagnostics) > 0 {
				t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
			}
			if got := len(program.Constants); got != tc.count {
				t.Fatalf("constant pool holds %d entries, want %d", got, tc.count)
			}
			if err := bytecode.VerifyProgram(program); err != nil {
				t.Fatalf("VerifyProgram() error = %v", err)
			}
			disassembly := strings.Join(bytecode.Disassemble(program), "\n")
			if got := strings.Contains(disassembly, "PUSH_WIDE"); got != tc.wide {
				t.Fatalf("disassembly holds PUSH_WIDE = %v, want %v:\n%s", got, tc.wide, disassembly)
			}
			expectMOO(t, source, want)
		})
	}
}
