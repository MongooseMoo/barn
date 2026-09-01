package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestResolveReleaseMetadata(t *testing.T) {
	got := Resolve(&debug.BuildInfo{
		Main: debug.Module{Version: "v2.7.3-beta.2"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef"},
			{Key: "vcs.modified", Value: "false"},
		},
	}, "")

	if got.Major != 2 || got.Minor != 7 || got.Patch != 3 || got.Prerelease != "beta.2" {
		t.Fatalf("Resolve() components = %d.%d.%d-%s, want 2.7.3-beta.2", got.Major, got.Minor, got.Patch, got.Prerelease)
	}
	if got.String != "2.7.3-beta.2" {
		t.Fatalf("Resolve().String = %q, want %q", got.String, "2.7.3-beta.2")
	}
	if got.Revision != "0123456789abcdef" || got.Modified {
		t.Fatalf("Resolve() VCS metadata = (%q, %t), want revision and clean tree", got.Revision, got.Modified)
	}
}

func TestResolveDevelopmentMetadata(t *testing.T) {
	got := Resolve(&debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef"},
			{Key: "vcs.modified", Value: "true"},
		},
	}, "")

	if got.Major != 0 || got.Minor != 0 || got.Patch != 0 || got.Prerelease != "dev" {
		t.Fatalf("Resolve() components = %d.%d.%d-%s, want 0.0.0-dev", got.Major, got.Minor, got.Patch, got.Prerelease)
	}
	if got.String != "0.0.0-dev+0123456789ab.dirty" {
		t.Fatalf("Resolve().String = %q, want development revision", got.String)
	}
	if !got.Modified {
		t.Fatal("Resolve().Modified = false, want true")
	}
}

func TestResolveUnknownMetadataDoesNotClaimARelease(t *testing.T) {
	got := Resolve(nil, "")
	if got.String != "0.0.0-dev+unknown" || got.Prerelease != "dev" {
		t.Fatalf("Resolve(nil) = %#v, want an explicit unknown development build", got)
	}
}

func TestResolveLinkerReleaseOverridesModuleVersion(t *testing.T) {
	got := Resolve(&debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, "v4.5.6-rc.1+packaged")
	if got.String != "4.5.6-rc.1+packaged" || got.Major != 4 || got.Minor != 5 || got.Patch != 6 || got.Prerelease != "rc.1" {
		t.Fatalf("Resolve() = %#v, want linker-injected release", got)
	}
}
