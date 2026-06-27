package config

import (
	"strings"
	"testing"
)

func TestDefaultOptions(t *testing.T) {
	options := DefaultOptions()
	if !options.OutboundNetwork {
		t.Fatal("default OUTBOUND_NETWORK should be enabled")
	}
	if options.PromoteNumbers {
		t.Fatal("default PROMOTE_NUMBERS should be disabled")
	}
}

func TestFeatureMap(t *testing.T) {
	on := Options{OutboundNetwork: true, PromoteNumbers: true}.FeatureMap()
	if on[FeatureOutboundNetwork] != true {
		t.Fatalf("enabled outbound feature = %v, want true", on[FeatureOutboundNetwork])
	}
	if on[FeatureOpenNetworkConnection] != "present" {
		t.Fatalf("open_network_connection feature = %v, want present", on[FeatureOpenNetworkConnection])
	}
	if on[FeaturePromoteNumbers] != true {
		t.Fatalf("promote feature = %v, want true", on[FeaturePromoteNumbers])
	}

	off := Options{OutboundNetwork: false}.FeatureMap()
	if off[FeatureOutboundNetwork] != false {
		t.Fatalf("disabled outbound feature = %v, want false", off[FeatureOutboundNetwork])
	}
	if off[FeatureOpenNetworkConnection] != "present" {
		t.Fatalf("open_network_connection feature = %v, want present", off[FeatureOpenNetworkConnection])
	}
}

func TestFeatureNames(t *testing.T) {
	on := Options{OutboundNetwork: true, PromoteNumbers: true}.FeatureNames()
	if !contains(on, FeatureOutboundNetwork) {
		t.Fatalf("enabled feature names %v missing %s", on, FeatureOutboundNetwork)
	}
	if !contains(on, FeaturePromoteNumbers) {
		t.Fatalf("enabled feature names %v missing %s", on, FeaturePromoteNumbers)
	}

	off := Options{OutboundNetwork: false}.FeatureNames()
	if contains(off, FeatureOutboundNetwork) {
		t.Fatalf("disabled feature names %v unexpectedly include %s", off, FeatureOutboundNetwork)
	}
	if !contains(off, FeatureOpenNetworkConnection) {
		t.Fatalf("disabled feature names %v missing %s", off, FeatureOpenNetworkConnection)
	}
}

func TestParseOptions(t *testing.T) {
	text := "# comment\nOUTBOUND_NETWORK = 0\nPROMOTE_NUMBERS = 1\n"
	options, err := Parse(strings.NewReader(text), "test.conf")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if options.OutboundNetwork {
		t.Fatal("OutboundNetwork = true, want false")
	}
	if !options.PromoteNumbers {
		t.Fatal("PromoteNumbers = false, want true")
	}
}

func TestParseRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "true value", text: "OUTBOUND_NETWORK = true\n"},
		{name: "yes value", text: "OUTBOUND_NETWORK = yes\n"},
		{name: "duplicate", text: "OUTBOUND_NETWORK = 1\nOUTBOUND_NETWORK = 0\n"},
		{name: "unknown", text: "FOO = 1\n"},
		{name: "malformed", text: "OUTBOUND_NETWORK 1\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(test.text), "bad.conf")
			if err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
			if !strings.Contains(err.Error(), "bad.conf:") {
				t.Fatalf("Parse() error = %q, want source and line", err.Error())
			}
		})
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
