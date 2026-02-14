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
		fmt.Println("Usage: dump_prop [-db database] <objnum> <propname>")
		os.Exit(1)
	}

	database, err := db.LoadDatabase(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading database: %v\n", err)
		os.Exit(1)
	}
	store := database.NewStoreFromDatabase()

	var objNum int
	fmt.Sscanf(args[0], "%d", &objNum)
	propName := args[1]

	obj := store.Get(types.ObjID(objNum))
	if obj == nil {
		fmt.Printf("Object #%d not found\n", objNum)
		os.Exit(1)
	}

	prop, ok := obj.Properties[propName]
	if !ok {
		fmt.Printf("Property '%s' not found on #%d\n", propName, objNum)
		fmt.Println("Available properties:")
		for name := range obj.Properties {
			fmt.Printf("  %s\n", name)
		}
		os.Exit(1)
	}

	fmt.Printf("#%d.%s = %v\n", objNum, propName, prop.Value)
}
