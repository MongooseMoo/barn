package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/internal/listener"
)

type startupReadyObserver struct{ ready chan struct{} }

func (o *startupReadyObserver) Ready()    { close(o.ready) }
func (o *startupReadyObserver) Draining() {}
func (o *startupReadyObserver) Stopped()  {}
func (o *startupReadyObserver) Failed()   {}

func TestShutdownRetainedAcrossStartup(t *testing.T) {
	for _, timing := range []string{"before_start", "during_start", "after_start"} {
		t.Run(timing, func(t *testing.T) {
			s, err := NewServer(filepath.Join(t.TempDir(), "unused.db"), []listener.Spec{{
				Protocol: listener.ProtocolTCP, Interface: "127.0.0.1", Port: 0,
			}}, 0)
			if err != nil {
				t.Fatal(err)
			}
			// As in application startup, reserve the socket before installing the
			// runtime and allowing application cancellation to reach Shutdown.
			release, err := s.ReserveListeners()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(release)
			s.store = dbstore.NewStore()
			addTestObject(t, s.store, 0, dbstore.FlagWizard)
			s.runtime = engine.NewRuntimeWithRegistry(s.store, s.options, s.registry)
			s.input = NewInputProcessor(s.store, s.runtime)
			s.input.SetConnectionManager(s.connManager)
			ready := &startupReadyObserver{ready: make(chan struct{})}
			s.SetLifecycleObserver(ready)
			t.Cleanup(func() {
				s.cancel()
				s.input.Stop()
				s.runtime.Stop()
				if err := s.runtime.Session().Close(); err != nil {
					t.Error(err)
				}
			})

			const message = "application startup canceled"
			if timing == "before_start" {
				s.Shutdown(message)
				if !errors.Is(s.ctx.Err(), context.Canceled) {
					t.Fatal("Shutdown before Start discarded application cancellation")
				}
			}

			done := make(chan error, 1)
			if timing == "during_start" {
				// Queue both operations at the existing transition lock. Either
				// ordering must retain the request and complete normal cleanup.
				s.mu.Lock()
				go func() { done <- s.Start() }()
				shutdownDone := make(chan struct{})
				go func() {
					s.Shutdown(message)
					close(shutdownDone)
				}()
				s.mu.Unlock()
				<-shutdownDone
			} else {
				go func() { done <- s.Start() }()
				if timing == "after_start" {
					select {
					case <-ready.ready:
					case err := <-done:
						t.Fatalf("Start returned before readiness: %v", err)
					case <-time.After(5 * time.Second):
						s.cancel()
						<-done
						t.Fatal("Start did not reach readiness")
					}
					s.Shutdown(message)
				}
			}

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("Start: %v", err)
				}
			case <-time.After(5 * time.Second):
				s.cancel()
				<-done
				t.Fatal("Start did not complete shutdown after application cancellation")
			}
			if !errors.Is(s.input.ctx.Err(), context.Canceled) {
				t.Fatal("startup cancellation did not stop input")
			}
			if got := s.connManager.ListenerInfos(); len(got) != 0 {
				t.Fatalf("startup cancellation retained listeners: %+v", got)
			}
			if s.running {
				t.Fatal("server remained running after startup cancellation")
			}
			s.Shutdown("late shutdown must be ignored")
			if s.shutdownMessage != message {
				t.Fatalf("shutdown message = %q, want %q", s.shutdownMessage, message)
			}
		})
	}
}
