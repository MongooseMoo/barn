package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MongooseMoo/barn/engine"
)

// The runtime routes answer 503 before the runtime exists and refuse writes.
func TestRuntimeDiagnosticsRoutesBeforeRuntime(t *testing.T) {
	mux := debugMux(nil)
	registerRuntimeDiagnostics(mux, func() *engine.Runtime { return nil })
	for _, path := range []string{"/debug/commit-gate", "/debug/task-timelines"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s = %d, want 503", path, res.Code)
		}
		res = httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, nil))
		if res.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", path, res.Code)
		}
	}
}
