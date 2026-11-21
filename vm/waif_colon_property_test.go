package vm

import "testing"

// Toast waif_get_prop prefixes every name read from a waif with a colon, one
// that already starts with a colon included, so w.(":foo") and w.:foo name the
// class property "::foo", not ":foo".
func TestWaifColonNamedReadAddsSecondColon(t *testing.T) {
	source := `class = create(#-1);
add_property(class, ":foo", 5, {#0, "r"});
add_verb(class, {#0, "rxd", "make"}, {"this", "none", "this"});
set_verb_code(class, "make", {"return new_waif();"});
w = class:make();
missing = {` + "`w.:foo ! ANY', `w.(\":foo\") ! ANY'" + `};
add_property(class, "::foo", 9, {#0, "r"});
w = class:make();
return {w.foo, @missing, w.:foo, w.(":foo")};`
	results := runSourceFuzzWorld(t, newSourceFuzzWorld(t), source, sourceFuzzPrincipals[0])
	if got, want := sourceFuzzOutcome(results[0]), "{5, E_PROPNF, E_PROPNF, 9, 9}"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
