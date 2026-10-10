package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MongooseMoo/barn/internal/mooprof"
	"github.com/google/pprof/profile"
)

func TestMOOProfileRouteReturnsPprofProfile(t *testing.T) {
	res := httptest.NewRecorder()
	debugMux().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/debug/pprof/moo?seconds=1", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", res.Code, res.Body.String())
	}
	prof, err := profile.Parse(res.Body)
	if err != nil {
		t.Fatalf("parse profile: %v", err)
	}
	if prof.DefaultSampleType != "ticks" || len(prof.SampleType) != 2 || prof.SampleType[0].Type != "ticks" || prof.SampleType[1].Type != "wall" {
		t.Fatalf("sample types = %v, want ticks and wall", prof.SampleType)
	}
	if mooprof.Active() != nil {
		t.Fatal("session still active after the request returned")
	}
}

func TestMOOProfileRouteRejectsConcurrentSession(t *testing.T) {
	session, err := mooprof.Start()
	if err != nil {
		t.Fatalf("start profile: %v", err)
	}
	defer session.Stop()
	res := httptest.NewRecorder()
	debugMux().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/debug/pprof/moo?seconds=1", nil))
	if res.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.Code)
	}
}

func TestMOOProfileRouteRejectsBadSeconds(t *testing.T) {
	for _, q := range []string{"seconds=abc", "seconds=0", "seconds=-5"} {
		res := httptest.NewRecorder()
		debugMux().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/debug/pprof/moo?"+q, nil))
		if res.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, res.Code)
		}
	}
}
