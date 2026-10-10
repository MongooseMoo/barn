package dbtool

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
)

// MemoryReport loads a database, drops everything the loader no longer needs,
// collects garbage, and prints what the loaded store retains: Go heap figures
// and a census of property slots. With a profilePath it also writes a pprof
// heap profile taken at the same point.
//
// It is the gate for load-memory work: run it before and after a change on the
// same database file and compare the heap_alloc line.
func MemoryReport(out io.Writer, source, profilePath string) error {
	return memoryReport(out, source, profilePath, dbformat.LoadDatabase)
}

func memoryReport(out io.Writer, source, profilePath string, load databaseLoader) error {
	store, err := loadStoreOnly(source, load)
	if err != nil {
		return err
	}

	// Two collections: the first frees the loader's garbage and runs finalizers,
	// the second frees what those finalizers released.
	runtime.GC()
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	objects, slots, clearSlots, privateSlots := store.PropertySlotCensus()
	fmt.Fprintf(out, "objects:          %d\n", objects)
	fmt.Fprintf(out, "property_slots:   %d\n", slots)
	fmt.Fprintf(out, "clear_slots:      %d\n", clearSlots)
	fmt.Fprintf(out, "private_slots:    %d\n", privateSlots)
	fmt.Fprintf(out, "heap_alloc:       %d\n", mem.HeapAlloc)
	fmt.Fprintf(out, "heap_objects:     %d\n", mem.HeapObjects)
	fmt.Fprintf(out, "heap_inuse:       %d\n", mem.HeapInuse)
	fmt.Fprintf(out, "sys:              %d\n", mem.Sys)
	fmt.Fprintf(out, "total_alloc:      %d\n", mem.TotalAlloc)
	fmt.Fprintf(out, "gc_cycles:        %d\n", mem.NumGC)

	if profilePath != "" {
		file, err := os.Create(profilePath)
		if err != nil {
			return fmt.Errorf("create heap profile: %w", err)
		}
		if err := pprof.WriteHeapProfile(file); err != nil {
			file.Close()
			return fmt.Errorf("write heap profile: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close heap profile: %w", err)
		}
	}
	runtime.KeepAlive(store)
	return nil
}

// loadStoreOnly returns the store alone so the loader's Database, and every
// intermediate it holds, is unreachable by the time the caller measures.
func loadStoreOnly(source string, load databaseLoader) (*dbstore.Store, error) {
	database, err := load(source)
	if err != nil {
		return nil, fmt.Errorf("load database: %w", err)
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		return nil, fmt.Errorf("construct store from database: %w", err)
	}
	return store, nil
}
