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
