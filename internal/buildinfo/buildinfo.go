// Package buildinfo provides the single source of truth for Barn's executable
// identity. Release builds may set Release with -ldflags -X; otherwise Go's
// embedded module and VCS metadata are used.
package buildinfo

import (
	"runtime/debug"
	"strconv"
	"strings"
)

// Release may be populated at link time with a semantic version such as v2.1.0.
var Release string

// Info is Barn's normalized build identity.
type Info struct {
	Major      int64
	Minor      int64
	Patch      int64
	Prerelease string
	String     string
	Revision   string
	Modified   bool
}

// Current returns metadata for the running executable.
func Current() Info {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		info = nil
	}
	return Resolve(info, Release)
}

// Resolve normalizes injected release data and Go build metadata. It is
// exported so callers can test both packaged and development build identities.
func Resolve(info *debug.BuildInfo, release string) Info {
	revision, modified := vcsMetadata(info)
	version := strings.TrimSpace(release)
	if version == "" && info != nil {
		version = info.Main.Version
	}

	major, minor, patch, prerelease, normalized, ok := parseVersion(version)
	if ok {
		return Info{major, minor, patch, prerelease, normalized, revision, modified}
	}

	identity := "unknown"
	if revision != "" {
		identity = revision
		if len(identity) > 12 {
			identity = identity[:12]
		}
	}
	if modified {
		identity += ".dirty"
	}
	return Info{0, 0, 0, "dev", "0.0.0-dev+" + identity, revision, modified}
}

func vcsMetadata(info *debug.BuildInfo) (revision string, modified bool) {
	if info == nil {
		return "", false
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, modified
}

func parseVersion(version string) (major, minor, patch int64, prerelease, normalized string, ok bool) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version == "" || version == "(devel)" {
		return 0, 0, 0, "", "", false
	}
	coreAndPre := version
	if i := strings.IndexByte(coreAndPre, '+'); i >= 0 {
		coreAndPre = coreAndPre[:i]
	}
	core := coreAndPre
	if i := strings.IndexByte(coreAndPre, '-'); i >= 0 {
		core, prerelease = coreAndPre[:i], coreAndPre[i+1:]
		if prerelease == "" {
			return 0, 0, 0, "", "", false
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return 0, 0, 0, "", "", false
	}
	values := []*int64{&major, &minor, &patch}
	for i, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil || value < 0 {
			return 0, 0, 0, "", "", false
		}
		*values[i] = value
	}
	return major, minor, patch, prerelease, version, true
}
