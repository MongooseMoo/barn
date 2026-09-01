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
			assertVersionKey(t, ctx, test.build, "patch", types.NewInt(test.build.Patch))
			assertVersionKey(t, ctx, test.build, "prerelease", types.NewStr(test.build.Prerelease))
			assertVersionKey(t, ctx, test.build, "string", types.NewStr(test.build.String))
			assertVersionKey(t, ctx, test.build, "revision", types.NewStr(test.build.Revision))
			assertVersionKey(t, ctx, test.build, "modified", types.NewInt(boolInt(test.build.Modified)))
			assertVersionKey(t, ctx, test.build, "runtime", types.NewStr(runtime.Version()))
			assertVersionKey(t, ctx, test.build, "platform", types.NewStr(runtime.GOOS))
			assertVersionKey(t, ctx, test.build, "architecture", types.NewStr(runtime.GOARCH))

			all := serverVersion(ctx, []types.Value{types.NewStr("")}, test.build)
			if !all.IsNormal() {
				t.Fatalf("server_version(\"\") returned %s", all.Error)
			}
			for _, entry := range all.Val.Elements() {
				pair := entry.Elements()
				assertVersionKey(t, ctx, test.build, pair[0].Str(), pair[1])
			}
		})
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
