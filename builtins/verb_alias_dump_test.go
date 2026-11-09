package builtins

import (
	"os"
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/types"
)

// TestVerbAliasesSurviveDump writes the database after add_verb and
// set_verb_info give a verb several aliases, reloads it, and checks that every
// alias is still there. The writer emits the verb's name line, so a name line
// holding only the first alias drops the rest on reload.
func TestVerbAliasesSurviveDump(t *testing.T) {
	ctx, store := verbMetadataTxnTestContext(t)

	added := builtinAddVerb(ctx, []types.Value{
		types.NewObj(0),
		types.NewList([]types.Value{types.NewObj(0), types.NewStr("rxd"), types.NewStr("foo bar")}),
		types.NewList([]types.Value{types.NewStr("this"), types.NewStr("none"), types.NewStr("this")}),
	})
	if added.IsError() {
		t.Fatalf("add_verb failed: %v", added.Error)
	}
	renamed := builtinSetVerbInfo(ctx, []types.Value{
		types.NewObj(0),
		types.NewStr("look"),
		types.NewList([]types.Value{types.NewObj(0), types.NewStr("rxd"), types.NewStr("peek glance")}),
	})
	if renamed.IsError() {
		t.Fatalf("set_verb_info failed: %v", renamed.Error)
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "verb-alias-*.db")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer tmpFile.Close()
	if err := format.NewWriter(tmpFile, store.Snapshot()).WriteDatabase(); err != nil {
		t.Fatalf("WriteDatabase failed: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	reloaded, err := format.LoadDatabase(tmpFile.Name())
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	reloadedStore, _ := reloaded.NewStoreFromDatabase()

	obj := reloadedStore.Snapshot().Objects[0]
	if obj == nil {
		t.Fatal("reloaded #0 missing")
	}
	var got [][]string
	for _, verb := range obj.VerbList {
		got = append(got, verb.Names)
	}
	want := [][]string{{"peek", "glance"}, {"foo", "bar"}}
	if !slices.EqualFunc(got, want, slices.Equal[[]string]) {
		t.Fatalf("reloaded verb aliases = %q, want %q", got, want)
	}
}
