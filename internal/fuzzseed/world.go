package fuzzseed

import (
	"fmt"
	"strings"
)

// WorldHistory generates setup and a dependent action sequence together. Each
// prefix has a live parent/child, a defined property and an installed probe verb.
// No unbounded loops or optional external capabilities enter this lane. Choice
// bytes replay only through this renderer, not the raw-source corpus decoder.
func WorldHistory(choices []byte) string { return worldHistory(choices, false) }

// ToastWorld is WorldHistory for a world Barn writes and Toast then loads
// (scripts/toast_reads_barn_db.py). Its actions can also store a string with
// characters above 127 in a property value, an object name and verb code,
// because Barn strings are Unicode and Toast strings are bytes. Every pause
// but the last is suspend(0), so the world holds every action; the program
// then leaves a fork queued for an hour and suspends until resumed, so the
// checkpoint holds one queued and one suspended task.
func ToastWorld(choices []byte) string { return worldHistory(choices, true) }

// textPieces are stringPieces plus characters of two, three and four UTF-8 bytes.
var textPieces = append(append([]string(nil), stringPieces...), "ü", "€", "😀")

// mooQuote spells s as a MOO string literal.
func mooQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func worldHistory(choices []byte, toast bool) string {
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
	actions := 8
	if toast {
		actions = 9
		source.WriteString("add_property(ancestor, \"text\", \"\", {#0, \"rc\"});\n")
	}
	count := 1 + g.choose(16)
	for i := 0; i < count; i++ {
		value := g.choose(255) - 127
		switch g.choose(actions) {
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
		case 8:
			var text strings.Builder
			for n := 1 + g.choose(maxStringLen); n > 0; n-- {
				text.WriteString(textPieces[g.choose(len(textPieces))])
			}
			quoted := mooQuote(text.String())
			fmt.Fprintf(&source, "child.text = %s;\nancestor.name = %s;\n", quoted, quoted)
			fmt.Fprintf(&source, "set_verb_code(child, \"read_value\", {%s});\n",
				mooQuote("return this.value;")+", "+mooQuote(quoted+";"))
		}
		// These observations include definitions/metadata and dispatch results,
		// not only the final returned scalar. Suspend preserves each action's
		// partially built trace and queues across a real checkpoint boundary.
		pause := "suspend();"
		if toast {
			pause = "suspend(0);"
		}
		fmt.Fprintf(&source, `info = property_info(child, "value");
expected = index(info[2], "r") ? child.value | E_PERM;
try
  observed = child:read_value();
except error (ANY)
  observed = error[1];
endtry
if (!equal(expected, observed)) raise(E_INVARG, "permission effect disagrees", {expected, observed}); endif
observations = {@observations, {ancestor.value, child.value, child:probe(), info, observed, verb_info(ancestor, "probe"), verb_code(ancestor, "probe")}};
%s
`, pause)
	}
	if toast {
		source.WriteString("fork (3600)\n#0.p = observations;\nendfork\nsuspend();\n")
	}
	source.WriteString("return observations;\n")
	return source.String()
}
