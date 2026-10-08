package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
)

func TestTZOffsetCapability(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			store := dbstore.NewStore()
			addServerVerbTestObject(t, store, 0, dbstore.FlagUser|dbstore.FlagWizard|dbstore.FlagProgrammer)
			caps := config.DefaultCapabilities() &^ config.BarnExtensions
			if enabled {
				caps |= config.BarnExtensions
			}
			rt := NewRuntimeWithOptions(store, config.Options{BuiltinCapabilities: &caps})
			defer rt.Stop()
			if rt.Registry().Has("tz_offset") != enabled {
				t.Fatalf("tz_offset presence differs from capability: enabled=%v", enabled)
			}
			output := rt.EvalCommandOutput(0, `return call_function("tz_offset", "UTC", 0);`)
			want := "E_INVARG"
			if enabled {
				want = `"+0000"`
			}
			if !strings.Contains(output, want) {
				t.Fatalf("dynamic tz_offset output = %q; want %s", output, want)
			}
			if enabled {
				output = rt.EvalCommandOutput(0, `return tz_offset("America/Edmonton", 0);`)
				if !strings.Contains(output, `"-0700"`) {
					t.Fatalf("compiled tz_offset output = %q", output)
				}
			} else if _, ok := rt.Registry().GetID("tz_offset"); ok {
				t.Fatal("disabled tz_offset has a compiler lookup")
			}
		})
	}
}

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
