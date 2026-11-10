package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/server"
	"github.com/MongooseMoo/barn/types"
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

// The debug routes are registered with the server's getters before the
// database loads, as Run does. Requests made while LoadDatabase runs answer
// 503 or 200 and do not race the load (run with -race); once it returns,
// every route answers 200.
func TestRuntimeDiagnosticsRoutesDuringLoad(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add #0: %v", err)
	}
	path := filepath.Join(t.TempDir(), "load.db")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbformat.NewWriter(file, store.Snapshot()).WriteDatabase(); err != nil {
		file.Close()
		t.Fatalf("write database: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	srv, err := server.NewServerWithOptions(path, []listener.Spec{{Port: 1}}, 0, config.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	mux := debugMux(srv.Store)
	registerRuntimeDiagnostics(mux, srv.Runtime)
	paths := []string{"/debug/commit-gate", "/debug/task-timelines", "/debug/conflict-census"}

	var stop atomic.Bool
	var wg sync.WaitGroup
	for _, p := range paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			for !stop.Load() {
				res := httptest.NewRecorder()
				mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, p, nil))
				if res.Code != http.StatusOK && res.Code != http.StatusServiceUnavailable {
					t.Errorf("GET %s during load = %d", p, res.Code)
					return
				}
			}
		}(p)
	}
	loadErr := srv.LoadDatabase()
	stop.Store(true)
	wg.Wait()
	if loadErr != nil {
		t.Fatalf("LoadDatabase: %v", loadErr)
	}
	defer srv.Runtime().Stop()
	for _, p := range paths {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, p, nil))
		if res.Code != http.StatusOK {
			t.Errorf("GET %s after load = %d, body %q", p, res.Code, res.Body.String())
		}
	}
}
