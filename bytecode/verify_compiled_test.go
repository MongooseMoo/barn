package bytecode_test

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/vm"
)

// Real compiler output exercises every edge kind the stack-height pass
// follows: handlers, finally bodies, short-circuit joins, loops, forks, calls.
func TestStackHeightsAcceptsCompiledPrograms(t *testing.T) {
	sources := []string{
		"x = 1; return x + 2 * 3;",
		"return a && b || !c;",
		"for i in [1..10] if (i % 2) continue; endif x = {@x, i}; endfor return x;",
		"for v, k in (m) x[k] = v; endfor for v in ({1, 2}) y = v; endfor",
		"while (x < 10) x = x + 1; if (x == 5) break; endif endwhile",
		"try x = 1 / 0; except e (E_DIV, E_TYPE) return e; except (ANY) return 1; endtry",
		"try return #0:foo(@args); finally x = 1; endtry",
		"y = `1 / 0 ! E_DIV => {1, 2}[1]'; return y;",
		"{a, ?b = 2, @c} = args; return [a -> b, \"k\" -> c];",
		"fork t (5) suspend(1); endfork return t;",
		"#0.p[\"k\"] = 3; return #0.p[\"k\"] + #0.(\"q\") + #0:(\"v\")(1, 2);",
		"l[2..$] = {3}; s = \"abc\"[^..2]; return pass(@args);",
	}
	registry := vm.BuildVMRegistry() // includes the VM-owned pass()
	for _, source := range sources {
		program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
		if len(diagnostics) > 0 {
			t.Fatalf("CompileMOO(%q) diagnostics = %v", source, diagnostics)
		}
		heights, err := bytecode.StackHeights(program)
		if err != nil {
			t.Fatalf("StackHeights(%q) error = %v", source, err)
		}
		if heights[0] != 0 {
			t.Fatalf("StackHeights(%q) entry height = %d, want 0", source, heights[0])
		}
	}
}
