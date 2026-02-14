package main

import (
	"barn/db"
	"barn/types"
	"flag"
	"fmt"
	"os"
)

func main() {
	dbPath := flag.String("db", "Test.db", "database file to test")
	outPath := flag.String("out", "test_output.db", "output file for written database")
	flag.Parse()

	// Load original database
	fmt.Printf("Loading %s...\n", *dbPath)
	database, err := db.LoadDatabase(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading database: %v\n", err)
		os.Exit(1)
	}
	store := database.NewStoreFromDatabase()

	origMax := store.MaxObject()
	origPlayers := len(store.Players())
	origAll := len(store.All())
	fmt.Printf("Loaded: maxObj=#%d, players=%d, objects=%d\n", origMax, origPlayers, origAll)

	// Write to output file
	fmt.Printf("Writing to %s...\n", *outPath)
	outFile, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}

	writer := db.NewWriter(outFile, store)
	if err := writer.WriteDatabase(); err != nil {
		outFile.Close()
		fmt.Fprintf(os.Stderr, "Error writing database: %v\n", err)
		os.Exit(1)
	}
	outFile.Close()
	fmt.Println("Write complete.")

	// Reload written database
	fmt.Printf("Reloading %s...\n", *outPath)
	database2, err := db.LoadDatabase(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reloading database: %v\n", err)
		os.Exit(1)
	}
	store2 := database2.NewStoreFromDatabase()

	newMax := store2.MaxObject()
	newPlayers := len(store2.Players())
	newAll := len(store2.All())
	fmt.Printf("Reloaded: maxObj=#%d, players=%d, objects=%d\n", newMax, newPlayers, newAll)

	// Compare
	errors := 0
	if origMax != newMax {
		fmt.Printf("MISMATCH: maxObj %d vs %d\n", origMax, newMax)
		errors++
	}
	if origPlayers != newPlayers {
		fmt.Printf("MISMATCH: players %d vs %d\n", origPlayers, newPlayers)
		errors++
	}
	if origAll != newAll {
		fmt.Printf("MISMATCH: objects %d vs %d\n", origAll, newAll)
		errors++
	}

	// Compare individual objects
	for id := int64(0); id <= int64(origMax); id++ {
		obj1 := store.GetUnsafe(types.ObjID(id))
		obj2 := store2.GetUnsafe(types.ObjID(id))

		if (obj1 == nil) != (obj2 == nil) {
			fmt.Printf("MISMATCH: object #%d existence differs\n", id)
			errors++
			continue
		}
		if obj1 == nil {
			continue
		}

		if obj1.Name != obj2.Name {
			fmt.Printf("MISMATCH: #%d name %q vs %q\n", id, obj1.Name, obj2.Name)
			errors++
		}
		if obj1.Flags != obj2.Flags {
			fmt.Printf("MISMATCH: #%d flags %v vs %v\n", id, obj1.Flags, obj2.Flags)
			errors++
		}
		if obj1.Owner != obj2.Owner {
			fmt.Printf("MISMATCH: #%d owner %d vs %d\n", id, obj1.Owner, obj2.Owner)
			errors++
		}
		if len(obj1.VerbList) != len(obj2.VerbList) {
			fmt.Printf("MISMATCH: #%d verbs %d vs %d\n", id, len(obj1.VerbList), len(obj2.VerbList))
			errors++
		}
		if len(obj1.Properties) != len(obj2.Properties) {
			fmt.Printf("MISMATCH: #%d props %d vs %d\n", id, len(obj1.Properties), len(obj2.Properties))
			errors++
		}
	}

	if errors > 0 {
		fmt.Printf("\nFAILED: %d mismatches\n", errors)
		os.Exit(1)
	}
	fmt.Println("\nSUCCESS: Round-trip test passed!")
}
