package main

import (
	"barn/db"
	"barn/types"
	"flag"
	"fmt"
	"os"
)

func main() {
	dbPath := flag.String("db", "Test.db", "Path to database file")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		fmt.Println("Usage: dump_verb [-db database] <objnum> <verbname>")
		fmt.Println("  -db    Database file (default: Test.db)")
		fmt.Println("Example: dump_verb -db mongoose.db 10 connect")
		os.Exit(1)
	}

	database, err := db.LoadDatabase(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading database %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	store := database.NewStoreFromDatabase()

	var objNum int
	_, err = fmt.Sscanf(args[0], "%d", &objNum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid object number: %s\n", args[0])
		os.Exit(1)
	}
	verbName := args[1]

	obj := store.Get(types.ObjID(objNum))
	if obj == nil {
		fmt.Printf("Object #%d not found\n", objNum)
		os.Exit(1)
	}

	fmt.Printf("Object #%d: %s\n", objNum, obj.Name)

	verb, ok := obj.Verbs[verbName]
	if !ok {
		// Try with colon prefix
		verb, ok = obj.Verbs[":"+verbName]
	}
	if !ok {
		fmt.Printf("Verb '%s' not found on #%d\n", verbName, objNum)
		fmt.Println("Available verbs:")
		for name := range obj.Verbs {
			fmt.Printf("  %s\n", name)
		}
		os.Exit(1)
	}

	fmt.Printf("Verb: %s\n", verb.Name)
	fmt.Printf("Code (%d lines):\n", len(verb.Code))
	for i, line := range verb.Code {
		fmt.Printf("%3d: %s\n", i+1, line)
	}
}
