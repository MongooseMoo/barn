package app

import (
	"encoding/json"
	"log/slog"
	"net/http"

	dbstore "github.com/MongooseMoo/barn/db/store"
)

// conflictCensusView is the JSON served at /debug/conflict-census: whether
// census tracking is on, and every field of the census. Without tracking the
// per-key Diag and SliceAttempts stay zero.
type conflictCensusView struct {
	Tracking bool
	dbstore.ConflictCensus
}

// conflictCensusHandler serves the store's conflict census, read only. store
// returns nil until the database is loaded, and the handler answers 503 then.
// CONFLICT_CENSUS = 1 in the server's config turns tracking on.
//
//	curl localhost:PORT/debug/conflict-census
func conflictCensusHandler(store func() *dbstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var s *dbstore.Store
		if store != nil {
			s = store()
		}
		if s == nil {
			http.Error(w, "database not loaded", http.StatusServiceUnavailable)
			return
		}
		view := conflictCensusView{Tracking: s.ConflictCensusTracking(), ConflictCensus: s.ConflictCensus()}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(view); err != nil {
			slog.Warn("write conflict census", slog.String("err", err.Error()))
		}
	}
}
