package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunListsProfilesToConfiguredOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := DefaultConfig()
	cfg.ListProfiles = true
	cfg.ProfileRegistry = filepath.Join("..", "..", "profiles", "barn", "profiles.json")
	if err := Run(context.Background(), cfg, &stdout, &stderr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(stdout.String(), "barn-linux-testdb-outbound-off") {
		t.Fatalf("profile output missing expected profile: %q", stdout.String())
	}
}

func TestOperatorProbesFollowLifecycle(t *testing.T) {
	state := newLifecycle()
	mux := operatorMux(state)

	assertProbe := func(method, path string, want int) {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s status = %d, want %d (body %q)", method, path, res.Code, want, res.Body.String())
		}
		if method == http.MethodHead && res.Body.Len() != 0 {
			t.Fatalf("HEAD %s body = %q, want empty", path, res.Body.String())
		}
	}

	assertProbe(http.MethodGet, "/livez", http.StatusOK)
	assertProbe(http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	state.Ready()
	assertProbe(http.MethodHead, "/readyz", http.StatusOK)
	state.Draining()
	assertProbe(http.MethodGet, "/livez", http.StatusOK)
	assertProbe(http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	state.Stopped()
	assertProbe(http.MethodGet, "/livez", http.StatusServiceUnavailable)
	state.Failed()
	assertProbe(http.MethodGet, "/livez", http.StatusServiceUnavailable)
	assertProbe(http.MethodPost, "/livez", http.StatusMethodNotAllowed)
}

func TestOperatorMuxDoesNotExposeDebugRoutes(t *testing.T) {
	mux := operatorMux(newLifecycle())
	for _, path := range []string{"/debug/vars", "/debug/pprof/", "/debug/loglevel"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", path, res.Code)
		}
	}
}

func TestDebugMuxRetainsPrivilegedRoutes(t *testing.T) {
	mux := debugMux()
	for _, path := range []string{"/debug/vars", "/debug/pprof/", "/debug/loglevel"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code == http.StatusNotFound {
			t.Errorf("GET %s unexpectedly returned 404", path)
		}
	}
}

func TestRunStartupReturnsWrappedLoadError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = filepath.Join(t.TempDir(), "missing.db")
	cfg.DebugAddr = "off"
	err := Run(context.Background(), cfg, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "load database") {
		t.Fatalf("Run error = %v, want wrapped load database error", err)
	}
}

func TestRunInspectionReturnsErrorRatherThanExiting(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = filepath.Join("..", "..", "Test.db")
	cfg.ObjectInfo = "not-an-object"
	err := Run(context.Background(), cfg, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run inspection succeeded, want parse error")
	}
}
