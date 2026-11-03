package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// censusStore returns a store with tracking on in which one commit has lost on
// #1.p, and the slice that lost resolved after two attempts.
func censusStore(t *testing.T) *dbstore.Store {
	t.Helper()
	s := dbstore.NewStore()
	if err := s.Add(dbstore.NewObjectBuilder(0).Build()); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	id, ec := s.DirectTxn().CreateObject(nil, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject: %v", ec)
	}
	if ec := s.DirectTxn().DefineProperty(id, "p", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	s.SetConflictCensusTracking(true)
	loser := s.BeginSnapshot(0)
	defer loser.Release()
	if _, ec := loser.FindProperty(id, "p"); ec != types.E_NONE {
		t.Fatalf("FindProperty: %v", ec)
	}
	winner := s.BeginSnapshot(0)
	if ec := winner.SetPropertyValue(id, "p", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("winner SetPropertyValue: %v", ec)
	}
	if ec := winner.Commit(); ec != types.E_NONE {
		t.Fatalf("winner Commit: %v", ec)
	}
	winner.Release()
	if ec := loser.SetPropertyValue(id, "p", types.NewInt(2)); ec != types.E_NONE {
		t.Fatalf("loser SetPropertyValue: %v", ec)
	}
	if ec := loser.Commit(); ec == types.E_NONE {
		t.Fatal("loser committed")
	}
	s.NoteSliceAttempts(2)
	return s
}

func TestConflictCensusRouteServesCensus(t *testing.T) {
	s := censusStore(t)
	res := httptest.NewRecorder()
	debugMux(func() *dbstore.Store { return s }).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/debug/conflict-census", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var got struct {
		Tracking      bool
		LostTxns      uint64
		SliceAttempts []uint64
		Keys          []struct {
			Kind string
			Name string
			Lost uint64
			Diag struct {
				ValueCompared uint64
				ReadSites     []any
				Writers       []any
			}
		}
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", res.Body.String(), err)
	}
	if !got.Tracking || got.LostTxns != 1 {
		t.Errorf("tracking %v lost %d, want true and 1", got.Tracking, got.LostTxns)
	}
	if len(got.Keys) != 1 || got.Keys[0].Kind != "property" || got.Keys[0].Name != "p" || got.Keys[0].Lost != 1 || got.Keys[0].Diag.ValueCompared != 1 {
		t.Errorf("keys = %+v, want one property row for p with one compared value", got.Keys)
	}
	if len(got.SliceAttempts) < 2 || got.SliceAttempts[1] != 1 {
		t.Errorf("SliceAttempts = %v, want one slice at two attempts", got.SliceAttempts)
	}
}

func TestConflictCensusRouteBeforeLoadAndReadOnly(t *testing.T) {
	res := httptest.NewRecorder()
	debugMux(func() *dbstore.Store { return nil }).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/debug/conflict-census", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Errorf("before load: status = %d, want 503", res.Code)
	}
	s := censusStore(t)
	res = httptest.NewRecorder()
	debugMux(func() *dbstore.Store { return s }).ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/debug/conflict-census", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: status = %d, want 405", res.Code)
	}
	if s.ConflictCensus().LostTxns != 1 {
		t.Error("POST changed the census")
	}
}
