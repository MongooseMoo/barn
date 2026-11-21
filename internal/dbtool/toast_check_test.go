package dbtool

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// toastCheckStore is inspectionStore plus a property named ord on #1 and one
// verb on #1 per code given, named a, b, c, ... in order.
func toastCheckStore(t *testing.T, codes ...[]string) *dbstore.Store {
	t.Helper()
	store := inspectionStore(t)
	if code := store.DirectTxn().DefineProperty(1, "ord", dbstore.NewProperty(types.NewInt(1), 1, dbstore.PropRead, false, true)); code != types.E_NONE {
		t.Fatalf("define property: %s", code)
	}
	for i, code := range codes {
		verb := dbstore.NewVerb([]string{string(rune('a' + i))}, 1, dbstore.VerbRead|dbstore.VerbExecute,
			dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, code)
		if _, errCode := store.AddVerb(1, verb); errCode != types.E_NONE {
			t.Fatalf("add verb: %s", errCode)
		}
	}
	return store
}

func toastBuiltins(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "builtins.txt")
	if err := os.WriteFile(path, []byte("call_function\neval\nlength\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A planted call to ord(), which Toast does not register, is named and fails
// the check.
func TestToastCheckNamesPlantedOrdCall(t *testing.T) {
	store := toastCheckStore(t, []string{"x = 1;", "return ord(\"a\");"})
	var out, errOut bytes.Buffer
	err := ToastCheck(&out, &errOut, store, toastBuiltins(t))
	want := "#1:a: line 2: calls ord, which Barn registers and Toast does not\n" +
		"toast-check: verbs=2 calls_barn_only_builtin=1 unparsable=0 calls_unregistered_builtin=0 not_checkable=0\n"
	if got := out.String(); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
	if err == nil {
		t.Fatal("ToastCheck returned nil for a verb calling ord()")
	}
}

// A property, variable and verb named ord are not calls to the builtin.
func TestToastCheckIgnoresPropertyNamedOrd(t *testing.T) {
	store := toastCheckStore(t, []string{"ord = this.ord + $ord;", "return this:ord(length(ord));"})
	var out, errOut bytes.Buffer
	if err := ToastCheck(&out, &errOut, store, toastBuiltins(t)); err != nil {
		t.Fatalf("ToastCheck: %v\noutput: %s", err, out.String())
	}
	if got, want := out.String(), "toast-check: verbs=2 calls_barn_only_builtin=0 unparsable=0 calls_unregistered_builtin=0 not_checkable=0\n"; got != want {
		t.Fatalf("output %q, want %q", got, want)
	}
}

// call_function with a literal name calls that name, in any letter case; eval
// and a computed call_function name are not checkable; a name neither engine
// registers is listed; none of those fail the check.
func TestToastCheckClassesThatDoNotFail(t *testing.T) {
	store := toastCheckStore(t,
		[]string{"return call_function(\"LENGTH\", \"a\");"},
		[]string{"return eval(\"return ord(1);\");", "return call_function(f, 1);"},
		[]string{"return call_function(\"no_such_builtin\");"},
	)
	var out, errOut bytes.Buffer
	if err := ToastCheck(&out, &errOut, store, toastBuiltins(t)); err != nil {
		t.Fatalf("ToastCheck: %v\noutput: %s", err, out.String())
	}
	want := "#1:b: line 1: not checkable: eval\n" +
		"#1:b: line 2: not checkable: call_function with a computed name\n" +
		"#1:c: line 1: calls no_such_builtin, which neither Toast nor Barn registers\n" +
		"toast-check: verbs=4 calls_barn_only_builtin=0 unparsable=0 calls_unregistered_builtin=1 not_checkable=1\n"
	if got := out.String(); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
}

// A literal call_function name Toast lacks and source that does not parse each
// fail the check.
func TestToastCheckClassesThatFail(t *testing.T) {
	for _, code := range []string{"return call_function(\"ORD\", \"a\");", "return 1 +;"} {
		store := toastCheckStore(t, []string{code})
		var out, errOut bytes.Buffer
		if err := ToastCheck(&out, &errOut, store, toastBuiltins(t)); err == nil {
			t.Fatalf("%s: ToastCheck returned nil\noutput: %s", code, out.String())
		}
		if !strings.Contains(out.String(), "#1:a: line 1: ") {
			t.Fatalf("%s: output %q does not name #1:a", code, out.String())
		}
	}
}

// An empty or missing builtin list is refused rather than reporting every call.
func TestToastCheckRefusesEmptyBuiltinList(t *testing.T) {
	store := toastCheckStore(t)
	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, []byte("\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{empty, filepath.Join(t.TempDir(), "missing.txt")} {
		var out, errOut bytes.Buffer
		if err := ToastCheck(&out, &errOut, store, path); err == nil || out.Len() != 0 {
			t.Fatalf("%s: err %v output %q", path, err, out.String())
		}
	}
}
