package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/types"
)

// CONFLICT_CENSUS reaches the loaded store, and Store is nil until the load.
func TestConflictCensusOptionTracksLoadedStore(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add #0: %v", err)
	}
	path := filepath.Join(t.TempDir(), "census.db")
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

	for _, on := range []bool{false, true} {
		options := config.DefaultOptions()
		options.ConflictCensus = on
		s, err := NewServerWithOptions(path, []listener.Spec{{Port: 1}}, 0, options)
		if err != nil {
			t.Fatal(err)
		}
		if s.Store() != nil {
			t.Fatal("Store() before LoadDatabase is not nil")
		}
		if err := s.LoadDatabase(); err != nil {
			t.Fatalf("LoadDatabase: %v", err)
		}
		if s.Store() == nil {
			t.Fatal("Store() after LoadDatabase is nil")
		}
		if got := s.Store().ConflictCensusTracking(); got != on {
			t.Errorf("CONFLICT_CENSUS = %v: tracking = %v", on, got)
		}
		s.runtime.Stop()
	}
}
