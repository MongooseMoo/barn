package dbtool

import (
	"bytes"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestDumpVerbCodePreservesTextContract(t *testing.T) {
	store := inspectionStore(t)
	var out, errOut bytes.Buffer

	if err := DumpVerbCode(&out, &errOut, store, "#1:look"); err != nil {
		t.Fatalf("DumpVerbCode: %v\nstderr: %s", err, errOut.String())
	}

	want := "=== #1:look ===\n" +
		"Names: look l*ook\n" +
		"Owner: #1\n" +
		"Perms: rx\n" +
		"--- Code (1 lines) ---\n" +
		"   1: return \"seen\";\n"
	if got := out.String(); got != want {
		t.Fatalf("DumpVerbCode output:\n%s\nwant:\n%s", got, want)
	}
	if errOut.Len() != 0 {
		t.Fatalf("DumpVerbCode stderr = %q, want empty", errOut.String())
	}
}

func TestInspectionCommandsUseSharedStore(t *testing.T) {
	store := inspectionStore(t)
	tests := []struct {
		name string
		run  func(*bytes.Buffer, *bytes.Buffer) error
		want []string
	}{
		{name: "list verbs", run: func(out, errOut *bytes.Buffer) error {
			return DumpListVerbs(out, errOut, store, "1")
		}, want: []string{"=== Verbs on #1 (Root) ===", "look l*ook", "lines=1"}},
		{name: "object info", run: func(out, errOut *bytes.Buffer) error {
			return DumpObjInfo(out, errOut, store, "#1")
		}, want: []string{"=== Object #1 ===", "Name:     Root", "Flags:    0x", "wizard", "--- Verbs (1) ---"}},
		{name: "raw object", run: func(out, errOut *bytes.Buffer) error {
			return DumpObjRawCommand(out, errOut, store, "1")
		}, want: []string{"=== Raw Object Data #1 ===", "Name:       \"Root\"", "VerbList:   1 verbs"}},
		{name: "verb lookup", run: func(out, errOut *bytes.Buffer) error {
			return VerbLookupCommand(out, errOut, store, "1:look")
		}, want: []string{"=== Verb Lookup: #1:look ===", "Result: FOUND on #1", "defined directly on this object"}},
		{name: "ancestry", run: func(out, errOut *bytes.Buffer) error {
			return AncestryCommand(out, errOut, store, "#1")
		}, want: []string{"=== Ancestry for #1 (Root) ===", "#1 - Root", "root object - no parent", "Total depth: 0"}},
		{name: "eval", run: func(out, errOut *bytes.Buffer) error {
			return EvalExpression(out, errOut, store, "1 + 2", config.DefaultOptions())
		}, want: []string{"=> 3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if err := tt.run(&out, &errOut); err != nil {
				t.Fatalf("inspection: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q:\n%s", want, out.String())
				}
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty", errOut.String())
			}
		})
	}
}

func TestInspectionRejectsInvalidSpec(t *testing.T) {
	store := inspectionStore(t)
	var out, errOut bytes.Buffer

	if err := DumpObjInfo(&out, &errOut, store, "not-an-object"); err == nil {
		t.Fatal("DumpObjInfo succeeded, want parse error")
	}
	if got := errOut.String(); !strings.Contains(got, "Error: invalid object ID: not-an-object") {
		t.Fatalf("stderr = %q, want invalid object ID", got)
	}
}

func inspectionStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	builder := dbstore.NewObjectBuilder(1)
	builder.SetName("Root")
	builder.SetOwner(1)
	builder.SetLocation(types.ObjNothing)
	builder.SetFlags(dbstore.FlagUser | dbstore.FlagWizard | dbstore.FlagRead)
	if err := store.Add(builder.Build()); err != nil {
		t.Fatalf("add object: %v", err)
	}
	verb := dbstore.NewVerb(
		"look l*ook",
		[]string{"look", "l*ook"},
		1,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{"return \"seen\";"},
	)
	if _, errCode := store.AddVerb(1, verb); errCode != types.E_NONE {
		t.Fatalf("add verb: %s", errCode)
	}
	return store
}
