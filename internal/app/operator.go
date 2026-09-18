package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

type lifecycleState uint32

const (
	lifecycleStarting lifecycleState = iota
	lifecycleReady
	lifecycleDraining
	lifecycleStopped
	lifecycleFailed
)

// lifecycle is deliberately per Run invocation: parallel servers must never
// observe or mutate one another's probe state.
type lifecycle struct{ state atomic.Uint32 }

func newLifecycle() *lifecycle {
	l := &lifecycle{}
	l.state.Store(uint32(lifecycleStarting))
	return l
}

func (l *lifecycle) Ready()    { l.state.Store(uint32(lifecycleReady)) }
func (l *lifecycle) Draining() { l.state.Store(uint32(lifecycleDraining)) }
func (l *lifecycle) Stopped()  { l.state.Store(uint32(lifecycleStopped)) }
func (l *lifecycle) Failed()   { l.state.Store(uint32(lifecycleFailed)) }

func (l *lifecycle) live() bool {
	s := lifecycleState(l.state.Load())
	return s == lifecycleStarting || s == lifecycleReady || s == lifecycleDraining
}

func (l *lifecycle) ready() bool { return lifecycleState(l.state.Load()) == lifecycleReady }

func probeHandler(ok func() bool, success, unavailable string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		body, status := unavailable, http.StatusServiceUnavailable
		if ok() {
			body, status = success, http.StatusOK
		}
		w.WriteHeader(status)
		if r.Method == http.MethodGet {
			fmt.Fprintln(w, body)
		}
	}
}

func operatorMux(state *lifecycle) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", probeHandler(state.live, "ok", "stopped"))
	mux.HandleFunc("/readyz", probeHandler(state.ready, "ready", "not ready"))
	return mux
}

type httpEndpoint struct {
	name string
	srv  *http.Server
}

func startHTTPEndpoint(name, addr string, handler http.Handler) (*httpEndpoint, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	e := &httpEndpoint{name: name, srv: &http.Server{Handler: handler}}
	slog.Info(name+" endpoint listening", slog.String("addr", ln.Addr().String()))
	go func() {
		if err := e.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error(name+" endpoint serve failed", slog.Any("err", err))
		}
	}()
	return e, nil
}

func (e *httpEndpoint) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.srv.Shutdown(ctx); err != nil {
		slog.Warn(e.name+" endpoint shutdown failed", slog.Any("err", err))
		_ = e.srv.Close()
		return
	}
	slog.Info(e.name + " endpoint stopped")
}
