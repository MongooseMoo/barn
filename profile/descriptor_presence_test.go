package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/vm"
)

func TestManifestPresenceComesFromRuntimeRegistry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, caps := range []config.Capabilities{config.Core, config.DefaultCapabilities()} {
		r, err := builtins.NewRegistryFromDescriptors(caps, vm.Descriptors())
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := BuildManifest(BuildInput{ProfileID: "test", DatabasePath: path, ConfigPath: path, Options: config.Options{OutboundNetwork: false, BuiltinCapabilities: &caps}, Registry: r})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"open_network_connection", "read_stdin", "background_test", "malloc_stats", "finished_tasks", "eval", "pass"} {
			_, present := manifest.Features["builtin."+name]
			if present != r.Has(name) {
				t.Fatalf("%s presence disagrees", name)
			}
		}
		if manifest.Features[config.FeatureOutboundNetwork] != false {
			t.Fatal("availability changed runtime permission")
		}
	}
}

func TestSuiteFeaturesFollowRegistryAndListeners(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		caps     config.Capabilities
		dispatch bool
		want     map[string]bool
	}{
		{"default build with a listener", config.DefaultCapabilities(), true, map[string]bool{
			"index_extensions": true, "anonymous": true, "tasks": true, "exec_support": true,
			"server_options": true, "command_dispatch": true, "toastcore_fixture": true,
		}},
		// The parser and database reader are unchanged; every builtin-backed
		// name drops.
		{"no builtins and no listener", 0, false, map[string]bool{
			"index_extensions": true, "anonymous": false, "tasks": false, "exec_support": false,
			"server_options": false, "command_dispatch": false, "toastcore_fixture": true,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			caps := test.caps
			r, err := builtins.NewRegistryFromDescriptors(caps, vm.Descriptors())
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := BuildManifest(BuildInput{ProfileID: "test", DatabasePath: path, ConfigPath: path, Options: config.Options{BuiltinCapabilities: &caps}, Registry: r, CommandDispatch: test.dispatch})
			if err != nil {
				t.Fatal(err)
			}
			for name, want := range test.want {
				if got := manifest.Features["feature."+name]; got != want {
					t.Errorf("feature.%s = %#v, want %v", name, got, want)
				}
			}
			// Held back until the rows they gate pass on Barn.
			for _, name := range []string{"fork", "forking"} {
				if got, present := manifest.Features["feature."+name]; present {
					t.Errorf("feature.%s = %#v, want it undeclared", name, got)
				}
			}
		})
	}
}
