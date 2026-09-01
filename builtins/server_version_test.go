package builtins

import (
	"runtime"
	"testing"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/internal/buildinfo"
	"github.com/MongooseMoo/barn/types"
)

func TestServerVersionUsesInjectedBuildMetadataConsistently(t *testing.T) {
	tests := []struct {
		name  string
		build buildinfo.Info
	}{
		{
			name:  "release",
			build: buildinfo.Info{Major: 2, Minor: 7, Patch: 3, Prerelease: "rc.1", String: "2.7.3-rc.1", Revision: "abc123"},
		},
		{
			name:  "development",
			build: buildinfo.Info{Prerelease: "dev", String: "0.0.0-dev+abc123.dirty", Revision: "abc123", Modified: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := runtimeOptionCtx(config.Options{})
			zeroArg := serverVersion(ctx, nil, test.build)
			if !zeroArg.IsNormal() || zeroArg.Val.Str() != test.build.String {
				t.Fatalf("server_version() = %#v, want %q", zeroArg, test.build.String)
			}

			assertVersionKey(t, ctx, test.build, "major", types.NewInt(test.build.Major))
			assertVersionKey(t, ctx, test.build, "minor", types.NewInt(test.build.Minor))
			assertVersionKey(t, ctx, test.build, "release", types.NewInt(test.build.Patch))
			assertVersionKey(t, ctx, test.build, "ext", types.NewStr(versionExtension(test.build.Prerelease)))
			assertVersionKey(t, ctx, test.build, "string", types.NewStr(test.build.String))
			assertVersionKey(t, ctx, test.build, "os", types.NewStr(runtime.GOOS))
			assertVersionKey(t, ctx, test.build, "options/RUNTIME", types.NewStr(runtime.Version()))
			assertVersionKey(t, ctx, test.build, "options/ARCHITECTURE", types.NewStr(runtime.GOARCH))
			assertVersionKey(t, ctx, test.build, "source/commit", types.NewStr(test.build.Revision))
			assertVersionKey(t, ctx, test.build, "source/modified", types.NewInt(boolInt(test.build.Modified)))

			all := serverVersion(ctx, []types.Value{types.NewStr("")}, test.build)
			if !all.IsNormal() {
				t.Fatalf("server_version(\"\") returned %s", all.Error)
			}
			wantKeys := []string{"major", "minor", "release", "ext", "string", "os", "features", "options", "source"}
			entries := all.Val.Elements()
			if len(entries) != len(wantKeys) {
				t.Fatalf("length(server_version(1)) = %d, want %d", len(entries), len(wantKeys))
			}
			for i, entry := range entries {
				pair := entry.Elements()
				if len(pair) != 2 || pair[0].Type() != types.TYPE_STR || pair[0].Str() != wantKeys[i] {
					t.Fatalf("server_version(1)[%d] = %s, want {%q, value}", i+1, entry.String(), wantKeys[i])
				}
				assertVersionKey(t, ctx, test.build, pair[0].Str(), pair[1])
			}
		})
	}
}

func TestServerVersionNestedGroupsMatchKeyedLookups(t *testing.T) {
	build := buildinfo.Info{String: "0.0.0-dev+abc123.dirty", Prerelease: "dev", Revision: "abc123", Modified: true}
	ctx := runtimeOptionCtx(config.Options{OutboundNetwork: true, PromoteNumbers: true})

	assertVersionKey(t, ctx, build, "options/OUTBOUND_NETWORK", types.NewStr("ON"))
	assertVersionKey(t, ctx, build, "options/PROMOTE_NUMBERS", types.NewStr("ON"))
	assertVersionKey(t, ctx, build, "source/commit", types.NewStr("abc123"))
	assertVersionKey(t, ctx, build, "source/modified", types.NewInt(1))

	for _, group := range []string{"features", "options", "source"} {
		result := serverVersion(ctx, []types.Value{types.NewStr(group)}, build)
		if !result.IsNormal() || result.Val.Type() != types.TYPE_LIST {
			t.Fatalf("server_version(%q) = %#v, want nested list", group, result)
		}
	}
}

func assertVersionKey(t *testing.T, ctx *Execution, build buildinfo.Info, key string, want types.Value) {
	t.Helper()
	result := serverVersion(ctx, []types.Value{types.NewStr(key)}, build)
	if !result.IsNormal() {
		t.Fatalf("server_version(%q) returned %s", key, result.Error)
	}
	if result.Val.Type() != want.Type() || result.Val.String() != want.String() {
		t.Errorf("server_version(%q) = %s, want %s", key, result.Val.String(), want.String())
	}
}
