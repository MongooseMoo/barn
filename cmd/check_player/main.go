package main

import (
	"fmt"
	"os"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func main() {
	dbPath := "toastcore.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	database, err := dbformat.LoadDatabase(dbPath)
	if err != nil {
		panic(err)
	}
	store := database.NewStoreFromDatabase()

	// Check wizard (#2)
	wizard, ok := store.Get(2)
	if !ok {
		fmt.Println("Wizard #2 is nil")
		return
	}

	parents, _ := store.Parents(2)
	fmt.Printf("Wizard #2 name: %s\n", wizard.Name)
	fmt.Printf("Wizard parents: %v\n", parents)
	fmt.Printf("Wizard flags: %d\n", wizard.Flags)

	// Look for password property directly
	fmt.Println("\nDirect password property:")
	if v, ok, _ := store.LocalProperty(2, "password"); ok {
		fmt.Printf("  password = %q (Clear=%v, Owner=#%d)\n", v.Value, v.Clear, v.Owner)
	} else {
		fmt.Println("  No direct password property")
	}

	// Walk up the parent chain to find password
	fmt.Println("\nLooking for password in parent chain:")
	visited := make(map[types.ObjID]bool)
	findPassword(store, types.ObjID(2), visited, 0)

	// Show all properties on wizard
	fmt.Println("\nAll properties on wizard #2:")
	if names, errCode := store.DefinedPropertyNames(2); errCode == types.E_NONE {
		for _, name := range names {
			if v, ok, _ := store.LocalProperty(2, name); ok {
				fmt.Printf("  %s = %v (Clear=%v)\n", name, v.Value, v.Clear)
			}
		}
	}
}

func findPassword(store *dbstore.Store, objID types.ObjID, visited map[types.ObjID]bool, depth int) {
	if visited[objID] {
		return
	}
	visited[objID] = true

	obj, ok := store.Get(objID)
	if !ok {
		return
	}

	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}

	fmt.Printf("%s#%d (%s):\n", indent, objID, obj.Name)

	// Check Properties for password
	if v, ok, _ := store.LocalProperty(objID, "password"); ok {
		fmt.Printf("%s  .password = %v (type: %T, Clear=%v)\n", indent, v.Value, v.Value, v.Clear)
	}

	// Recurse to parents
	parents, _ := store.Parents(objID)
	for _, parentID := range parents {
		findPassword(store, parentID, visited, depth+1)
	}
}
