package main

import (
	"flag"
	"fmt"
	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/types"
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

	database, err := dbformat.LoadDatabase(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading database: %v\n", err)
		os.Exit(1)
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error constructing store: %v\n", err)
		os.Exit(1)
	}

	var objNum int
	fmt.Sscanf(args[0], "%d", &objNum)
	propName := args[1]

	if _, ok := store.Get(types.ObjID(objNum)); !ok {
		fmt.Printf("Object #%d not found\n", objNum)
		os.Exit(1)
	}

	prop, ok, _ := store.DirectTxn().LocalProperty(types.ObjID(objNum), propName)
	if !ok {
		fmt.Printf("Property '%s' not found on #%d\n", propName, objNum)
		fmt.Println("Available properties:")
		if names, errCode := store.DirectTxn().DefinedPropertyNames(types.ObjID(objNum)); errCode == types.E_NONE {
			for _, name := range names {
				fmt.Printf("  %s\n", name)
			}
		}
		os.Exit(1)
	}

	fmt.Printf("#%d.%s = %v\n", objNum, propName, prop.Value)
}
