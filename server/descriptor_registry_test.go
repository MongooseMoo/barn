package server

import (
	"testing"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/internal/listener"
)

func TestServerExposesConstructedRegistryBeforeDatabaseLoad(t *testing.T) {
	caps := config.Core
	s, err := NewServerWithOptions("unused.db", []listener.Spec{{}}, 0, config.Options{BuiltinCapabilities: &caps})
	if err != nil {
		t.Fatal(err)
	}
	defer s.cancel()
	if s.runtime != nil {
		t.Fatal("fixture already loaded runtime")
	}
	r := s.BuiltinRegistry()
	if r == nil || !r.Has("eval") || r.Has("sqlite_open") {
		t.Fatal("profile cannot obtain selected registry during startup")
	}
}
