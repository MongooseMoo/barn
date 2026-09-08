package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/kernel"
)

func TestIncludeRTVarsCacheRefresh(t *testing.T) {
	session := &Session{runtime: newSessionRuntime()}
	if session.runtime.serverOptions.includeRTVars {
		t.Fatal("runtime variables enabled by default")
	}
	session.applyServerOptionsSnapshot(&kernel.PendingServerOptions{IncludeRTVars: true})
	if !session.runtime.serverOptions.includeRTVars {
		t.Fatal("runtime variable option was not loaded")
	}
	session.applyServerOptionsSnapshot(nil)
	if session.runtime.serverOptions.includeRTVars {
		t.Fatal("runtime variable option was not reset")
	}
}
