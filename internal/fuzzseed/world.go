package fuzzseed

import (
	"fmt"
	"strings"
)

// WorldHistory generates setup and a dependent action sequence together. Each
// prefix has a live parent/child, a defined property and an installed probe verb.
// No unbounded loops or optional external capabilities enter this lane. Choice
// bytes replay only through this renderer, not the raw-source corpus decoder.
func WorldHistory(choices []byte) string {
	g := generator{choices: choices}
	var source strings.Builder
	source.WriteString(`
ancestor = create(#2);
add_property(ancestor, "value", 0, {#0, "rw"});
child = create(ancestor);
add_verb(ancestor, {#0, "rxd", "probe"}, {"this", "none", "this"});
set_verb_code(ancestor, "probe", {"return this.value;"});
reader = create(#2);
reader.programmer = 1;
add_verb(child, {reader, "rxd", "read_value"}, {"this", "none", "this"});
set_verb_code(child, "read_value", {"return this.value;"});
observations = {};
`)
	for i, count := 0, 1+g.choose(16); i < count; i++ {
		value := g.choose(255) - 127
		switch g.choose(8) {
		case 0:
			fmt.Fprintf(&source, "ancestor.value = %d;\n", value)
		case 1:
			fmt.Fprintf(&source, "child.value = %d;\n", value)
		case 2:
			source.WriteString("clear_property(child, \"value\");\n")
		case 3:
			fmt.Fprintf(&source, "set_verb_code(ancestor, \"probe\", {\"return this.value + (%d);\"});\n", value)
		case 4:
			permissions := []string{"r", "rw", ""}[g.choose(3)]
			fmt.Fprintf(&source, "set_property_info(child, \"value\", {#0, \"%s\"});\n", permissions)
		case 5:
			source.WriteString(`delete_verb(ancestor, "probe");
add_verb(ancestor, {#0, "rxd", "probe"}, {"this", "none", "this"});
set_verb_code(ancestor, "probe", {"return this.value;"});
`)
		case 6:
			fmt.Fprintf(&source, "add_property(child, \"scratch\", %d, {#0, \"rw\"});\ndelete_property(child, \"scratch\");\n", value)
		case 7:
			fmt.Fprintf(&source, "captured = %d;\nfork (0)\n#0.p = captured;\nendfork\n", value)
		}
		// These observations include definitions/metadata and dispatch results,
		// not only the final returned scalar. Suspend preserves each action's
		// partially built trace and queues across a real checkpoint boundary.
		source.WriteString(`info = property_info(child, "value");
expected = index(info[2], "r") ? child.value | E_PERM;
try
  observed = child:read_value();
except error (ANY)
  observed = error[1];
endtry
if (!equal(expected, observed)) raise(E_INVARG, "permission effect disagrees", {expected, observed}); endif
observations = {@observations, {ancestor.value, child.value, child:probe(), info, observed, verb_info(ancestor, "probe"), verb_code(ancestor, "probe")}};
suspend();
`)
	}
	source.WriteString("return observations;\n")
	return source.String()
}
