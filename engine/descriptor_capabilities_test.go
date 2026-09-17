package engine

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
)

func TestRuntimeConstructsExplicitCapabilities(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagUser|dbstore.FlagWizard|dbstore.FlagProgrammer)
	caps := config.Core
	rt := NewRuntimeWithOptions(store, config.Options{BuiltinCapabilities: &caps})
	defer rt.Stop()
	if rt.Registry().Has("upcase") || rt.Registry().Has("sqlite_open") || rt.Registry().Has("open_network_connection") {
		t.Fatal("disabled family registered")
	}
	if !rt.Registry().Has("eval") || !rt.Registry().Has("pass") {
		t.Fatal("VM descriptors missing")
	}
	output := rt.EvalCommandOutput(0, `return server_version("features");`)
	if strings.Contains(output, "builtin.open_network_connection") || strings.Contains(output, "builtin.upcase") {
		t.Fatalf("disabled feature exposed: %s", output)
	}
	if !strings.Contains(output, "builtin.eval") {
		t.Fatalf("VM feature missing: %s", output)
	}
}
