package app

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/MongooseMoo/barn/internal/mooprof"
)

// mooProfileHandler collects a MOO-level profile: stacks are MOO verbs, not Go
// functions (internal/mooprof). It runs one session for ?seconds=N (default
// 30, as /debug/pprof/profile) and returns the gzipped pprof profile.
//
//	curl -o moo.pprof 'localhost:PORT/debug/pprof/moo?seconds=30'
//	go tool pprof -top moo.pprof
func mooProfileHandler(w http.ResponseWriter, r *http.Request) {
	seconds := int64(30)
	if raw := r.URL.Query().Get("seconds"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n <= 0 {
			http.Error(w, "seconds must be a positive integer", http.StatusBadRequest)
			return
		}
		seconds = n
	}
	session, err := mooprof.Start()
	if errors.Is(err, mooprof.ErrBusy) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-r.Context().Done():
		session.Stop()
		return
	}
	prof := session.Stop()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="moo.pprof"`)
	if err := prof.Write(w); err != nil {
		slog.Warn("write MOO profile", slog.String("err", err.Error()))
	}
}
