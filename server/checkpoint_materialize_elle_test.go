//go:build elle

package server

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/types"
)

// Hold the actual store walk reached by the server, after its first object was
// copied. Both runtime commit paths must finish before materialization resumes.
func TestCheckpointMaterializationAllowsRuntimeCommit(t *testing.T) {
	for _, writer := range []string{"ordinary", "coarse"} {
		t.Run(writer, func(t *testing.T) {
			store := dbstore.NewStore()
			for _, id := range []types.ObjID{0, 1} {
				builder := dbstore.NewObjectBuilder(id)
				builder.SetOwner(0)
				builder.SetLocation(types.ObjNothing)
				if id == 0 {
					builder.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer)
				}
				if err := store.Add(builder.Build()); err != nil {
					t.Fatal(err)
				}
				if code := store.DirectTxn().DefineProperty(id, "committed", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
					t.Fatal(code)
				}
			}
			rt := engine.NewRuntimeWithOptions(store, config.Options{AdmissionLimit: 1})
			defer rt.Stop()
			server := &Server{store: store, runtime: rt, connManager: NewConnectionManager(0), dbPath: filepath.Join(t.TempDir(), "walk.db")}
			entered, released := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(released) })
			hold := sync.OnceFunc(func() { close(entered); <-released })
			isolationprobe.SetPause(func(point string) {
				if point == "checkpoint/object-captured/0" {
					hold()
				}
			})
			defer isolationprobe.SetPause(nil)
			done := make(chan error, 1)
			joined := false
			defer func() {
				release()
				if !joined {
					select {
					case <-done:
					case <-time.After(10 * time.Second):
						t.Error("held checkpoint did not terminate during cleanup")
					}
				}
			}()
			go func() { done <- server.checkpoint() }()
			select {
			case <-entered:
			case err := <-done:
				joined = true
				t.Fatalf("checkpoint finished before materialization hold: %v", err)
			case <-time.After(10 * time.Second):
				t.Fatal("checkpoint did not reach its object walk")
			}
			source := "#0.committed = 42; #1.committed = 42; return #1.committed;"
			if writer == "coarse" {
				source = `add_verb(#0, {#0, "rxd", "post_capture"}, {"this", "none", "this"}); ` + source
			}
			committed := make(chan engine.EvalOutcome, 1)
			go func() { committed <- rt.Eval(0, []string{source}) }()
			select {
			case outcome := <-committed:
				if outcome.Panic != nil || len(outcome.Diagnostics) != 0 || outcome.Result.IsError() || outcome.Result.Val.Int() != 42 {
					t.Fatalf("runtime commit during materialization: %+v", outcome)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("runtime writer waited for held materialization")
			}
			for _, id := range []types.ObjID{0, 1} {
				value, code := store.DirectTxn().PropertyValue(id, "committed")
				if code != types.E_NONE || value.Int() != 42 {
					t.Fatalf("live object %d: %v, %v", id, value, code)
				}
			}
			release()
			err := <-done
			joined = true
			if err != nil {
				t.Fatal(err)
			}
			database, err := dbformat.LoadDatabaseReadOnly(server.dbPath + ".new")
			if err != nil {
				t.Fatal(err)
			}
			restored, err := database.NewStoreFromDatabase()
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []types.ObjID{0, 1} {
				value, code := restored.DirectTxn().PropertyValue(id, "committed")
				if code != types.E_NONE || value.Int() != 0 {
					t.Fatalf("captured object %d: %v, %v", id, value, code)
				}
			}
			if restored.HasLocalVerb(0, "post_capture") {
				t.Fatal("captured post-boundary coarse definition")
			}
		})
	}
}
