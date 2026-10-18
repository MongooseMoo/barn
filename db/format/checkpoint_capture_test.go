package format

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestCheckpointSerializationReleasesCaptureBeforeBlockedFileWriter(t *testing.T) {
	for _, coarse := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "coarse"}[coarse], func(t *testing.T) {
			objectStore := store.NewStore()
			builder := store.NewObjectBuilder(0)
			builder.SetOwner(0)
			builder.SetLocation(types.ObjNothing)
			builder.SetFlags(store.FlagWizard)
			if err := objectStore.Add(builder.Build()); err != nil {
				t.Fatal(err)
			}
			anon, code := objectStore.DirectTxn().CreateObject(nil, 0, true)
			if code != types.E_NONE {
				t.Fatal(code)
			}
			waif := types.NewWaif(0, 0).SetProperty("marker", types.NewInt(1))
			for name, value := range map[string]types.Value{
				":marker":   types.NewInt(0),
				"committed": types.NewInt(0), "graph": types.NewList([]types.Value{waif, types.NewAnon(anon)}),
				"padding": types.NewStr(strings.Repeat("checkpoint-payload", 131072)),
			} {
				if code := objectStore.DirectTxn().DefineProperty(0, name, store.NewProperty(value, 0, store.PropRead|store.PropWrite, false, true)); code != types.E_NONE {
					t.Fatal(code)
				}
			}
			if code := objectStore.DirectTxn().DefineProperty(anon, "marker", store.NewProperty(types.NewInt(1), 0, store.PropRead|store.PropWrite, false, true)); code != types.E_NONE {
				t.Fatal(code)
			}
			path := filepath.Join(t.TempDir(), "pipe.db")
			checkpoint, err := BeginCheckpoint(path)
			if err != nil {
				t.Fatal(err)
			}
			defer checkpoint.Close()
			if err := checkpoint.Capture(objectStore, nil, nil, nil); err != nil {
				t.Fatal(err)
			}
			capture := checkpoint.capture
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := checkpoint.file.Close(); err != nil {
				t.Fatal(err)
			}
			checkpoint.file = writer
			prefixRead, drainAllowed := make(chan struct{}), make(chan struct{})
			allowDrain := sync.OnceFunc(func() { close(drainAllowed) })
			var output bytes.Buffer
			drained := make(chan error, 1)
			go func() {
				prefix := make([]byte, 1)
				_, err := io.ReadFull(reader, prefix)
				if err != nil {
					drained <- err
					return
				}
				output.Write(prefix)
				close(prefixRead)
				<-drainAllowed
				_, err = io.Copy(&output, reader)
				drained <- err
			}()
			serialized := make(chan error, 1)
			go func() { serialized <- checkpoint.Serialize() }()
			finished := false
			defer func() {
				allowDrain()
				reader.Close()
				writer.Close()
				if !finished {
					<-serialized
				}
			}()
			select {
			case <-prefixRead:
			case err := <-serialized:
				finished = true
				t.Fatalf("serialization ended before pipe prefix: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("file writer never emitted a prefix")
			}
			// The retained handle rejects materialization once its registered
			// timestamp is released. This check uses its atomic closed guard.
			if _, _, err := capture.Materialize(); err == nil {
				t.Error("serialization retained its store capture during file writing")
			}
			committed := make(chan types.ErrorCode, 1)
			go func() {
				if coarse {
					committed <- objectStore.DirectTxn().SetPropertyValue(0, "committed", types.NewInt(42))
					return
				}
				tx := objectStore.BeginSnapshot(0)
				defer tx.Release()
				if code := tx.SetPropertyValue(0, "committed", types.NewInt(42)); code != types.E_NONE {
					committed <- code
					return
				}
				committed <- tx.Commit()
			}()
			select {
			case code := <-committed:
				if code != types.E_NONE {
					t.Error(code)
				}
			case <-time.After(2 * time.Second):
				t.Error("live writer blocked during checkpoint file serialization")
			}
			if code := objectStore.DirectTxn().SetWaifProperty(waif, "marker", types.NewInt(2)); code != types.E_NONE {
				t.Error(code)
			}
			if code := objectStore.DirectTxn().SetPropertyValue(anon, "marker", types.NewInt(2)); code != types.E_NONE {
				t.Error(code)
			}
			select {
			case err := <-serialized:
				finished = true
				t.Errorf("serialization completed while pipe reader was held: %v", err)
			default:
			}
			allowDrain()
			if !finished {
				err = <-serialized
				finished = true
				if err != nil {
					t.Fatal(err)
				}
			}
			writer.Close()
			checkpoint.file = nil
			if err := <-drained; err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, output.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadDatabase(path)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := loaded.NewStoreFromDatabase()
			if err != nil {
				t.Fatal(err)
			}
			value, code := restored.DirectTxn().PropertyValue(0, "committed")
			if code != types.E_NONE || value.Int() != 0 {
				t.Errorf("portable state included later write: %v %v", value, code)
			}
			graph, code := restored.DirectTxn().PropertyValue(0, "graph")
			if code != types.E_NONE || graph.Len() != 2 {
				t.Fatalf("restored graph %v %v", graph, code)
			}
			marker, ok := graph.Elements()[0].GetProperty("marker")
			if !ok || marker.Int() != 1 {
				t.Errorf("restored WAIF marker=%v present=%v", marker, ok)
			}
			marker, code = restored.DirectTxn().PropertyValue(graph.Elements()[1].ID(), "marker")
			if code != types.E_NONE || marker.Int() != 1 {
				t.Errorf("restored anonymous marker=%v code=%v", marker, code)
			}
		})
	}
}

func TestCheckpointMaterializationFailureReleasesOwnedState(t *testing.T) {
	objectStore, foreign := store.NewStore(), store.NewStore()
	waif := types.NewWaif(0, 0)
	if code := foreign.DirectTxn().SetWaifProperty(waif, "marker", types.NewInt(1)); code != types.E_NONE {
		t.Fatal(code)
	}
	objectStore.SetPendingFinalizations([]types.Value{waif})
	checkpoint, err := BeginCheckpoint(filepath.Join(t.TempDir(), "invalid.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer checkpoint.Close()
	if err := checkpoint.Capture(objectStore, nil, nil, []ActiveConnection{{Player: 0}}); err != nil {
		t.Fatal(err)
	}
	capture := checkpoint.capture
	if err := checkpoint.Serialize(); err == nil || !strings.Contains(err.Error(), "has no image at checkpoint timestamp") {
		t.Fatalf("materialization error = %v", err)
	}
	if checkpoint.capture != nil || checkpoint.queued != nil || checkpoint.suspended != nil || checkpoint.connections != nil {
		t.Fatal("failed materialization retained owned state")
	}
	if _, _, err := capture.Materialize(); err == nil {
		t.Fatal("failed serialization retained its timestamp")
	}
}
