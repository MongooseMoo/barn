package vm

import (
	"strings"
	"testing"
)

// Toast pushes an indexed assignment's collection before it evaluates the
// indexes and value (code_gen.cc push_lvalue), and its OP_PUSH_CLEAR moves a
// variable collection onto the stack, unbinding the variable. Barn reads the
// variable when it stores and holds an inner collection, a property's value
// or the ^/$ context in compiler temporaries. A task suspended inside such an
// assignment was refused by ExportNativeSnapshot ("no held value for native
// operand kind 0 from *ir.VariableTarget"), issue #948; and a restored one
// lost the variable or the temporary. Each case must end the same with every
// suspension checkpointed through Toast's form.
func TestCheckpointExportsIndexedAssignmentTargets(t *testing.T) {
	for _, source := range []string{
		// The shape of issue #948's fuzz program: the range reads ^ and the
		// variable, so the variable stays bound.
		"n = {1,2,3};\nn[suspend(0)+1..length(setremove({^, @n, @n}, 9))] = {};\nreturn n;",
		"n = {1,2,3};\nn[suspend(0)+1] = 5;\nreturn n;",
		"n = {1,2,3};\nn[2] = suspend(0);\nreturn n;",
		"m = [1 -> 2];\nm[suspend(0)] = 5;\nreturn m;",
		"s = \"abc\";\ns[suspend(0)+1..2] = \"x\";\nreturn s;",
		"n = {1,2,3};\nn[^..suspend(0)+1] = {};\nreturn n;",
		"n = {1,2,3};\nn[2..{$, suspend(0)}[1]] = {};\nreturn n;",
		"n = {1,2,3};\nn[(n = {7,8,9}) && suspend(0) + 1] = 5;\nreturn n;",
		"l = {{1,2},{3,4}};\nl[2][suspend(0)+1] = 5;\nreturn l;",
		"l = {{{1,2},{3}},{3,4}};\nl[1][1][suspend(0)+1] = 9;\nreturn l;",
		"l = {{1,2,3},{3,4}};\nl[1][^..suspend(0)+1] = {9};\nreturn l;",
		"#0.p = {1,2};\n#0.p[suspend(0)+1] = 3;\nreturn #0.p;",
		"#0.p = {1,2};\n#0.p[suspend(0)+1..$] = {3};\nreturn #0.p;",
		"#0.p = {{1,2}};\n#0.(\"p\")[1][^..suspend(0)+1] = {3};\nreturn #0.p;",
	} {
		t.Run(strings.ReplaceAll(source, "\n", " "), func(t *testing.T) {
			difference, skip := checkpointResumeDifference(t, source, checkpointSuspendedVM, sourceFuzzTicks, true)
			if difference != "" || skip != "" {
				t.Fatalf("difference %q, skip %q; want neither\nsource:\n%s", difference, skip, source)
			}
		})
	}
}
