package main

import (
	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/profile"
	"testing"
)

func TestBuildListenerSpecsUsesPortShorthand(t *testing.T) {
	specs, err := buildListenerSpecs(7777, nil, true)
	if err != nil {
		t.Fatalf("build listener specs: %v", err)
	}
	if len(specs) != 1 ||
		specs[0].Protocol != builtins.ListenerProtocolTCP ||
		specs[0].Port != 7777 {
		t.Fatalf("unexpected specs: %+v", specs)
	}
}

func TestBuildListenerSpecsParsesRepeatableListenFlags(t *testing.T) {
	specs, err := buildListenerSpecs(7777, []string{
		"tcp://127.0.0.1:7788",
		"ws://:7789/moo",
	}, false)
	if err != nil {
		t.Fatalf("build listener specs: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("got %d specs, want 2", len(specs))
	}
	if specs[0].Protocol != builtins.ListenerProtocolTCP ||
		specs[0].Interface != "127.0.0.1" ||
		specs[0].Port != 7788 {
		t.Fatalf("unexpected tcp spec: %+v", specs[0])
	}
	if specs[1].Protocol != "ws" ||
		specs[1].Port != 7789 ||
		specs[1].Path != "/moo" {
		t.Fatalf("unexpected ws spec: %+v", specs[1])
	}
}

func TestBuildListenerSpecsRejectsPortAndListen(t *testing.T) {
	_, err := buildListenerSpecs(7777, []string{"tcp://:7788"}, true)
	if err == nil {
		t.Fatalf("combined -port and -listen without error")
	}
}

func TestBuildListenerSpecsRejectsInvalidListen(t *testing.T) {
	_, err := buildListenerSpecs(7777, []string{"tcp://:70000"}, false)
	if err == nil {
		t.Fatalf("accepted invalid listener spec")
	}
}

func TestListProfilesLoadsCommittedRegistry(t *testing.T) {
	registry, err := profile.LoadRegistry("../../profiles/barn/profiles.json")
	if err != nil {
		t.Fatalf("load profile registry: %v", err)
	}
	if _, ok := registry.Find("barn-linux-testdb-outbound-off"); !ok {
		t.Fatal("missing outbound-off profile")
	}
}
