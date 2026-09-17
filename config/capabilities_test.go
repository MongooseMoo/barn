package config

import (
	"strings"
	"testing"
)

func TestParseExplicitCapabilities(t *testing.T) {
	for _, tc := range []struct {
		text string
		want Capabilities
	}{
		{"core,sqlite", Core | SQLite}, {"none", 0},
		{"core,barn-extensions,background-tasks,allocator-stats,process-stdin,network,sqlite", DefaultCapabilities()},
	} {
		options, err := Parse(strings.NewReader("BUILTIN_CAPABILITIES = "+tc.text+"\nOUTBOUND_NETWORK = 0\n"), "capabilities.conf")
		if err != nil || options.Capabilities() != tc.want {
			t.Fatalf("%s: %v, %v", tc.text, options, err)
		}
		if options.OutboundNetwork {
			t.Fatal("runtime permission was coupled to availability")
		}
	}
	for _, value := range []string{"core,core", "unknown", "core,", "none,core"} {
		if _, err := ParseCapabilities(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
