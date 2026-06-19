package main

import (
	"fmt"
	"os"

	dbformat "barn/db/format"
	dbstore "barn/db/store"
	"barn/types"
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
	wizard := store.Get(2)
	if wizard == nil {
		fmt.Println("Wizard #2 is nil")
		return
	}

	fmt.Printf("Wizard #2 name: %s\n", wizard.Name)
	fmt.Printf("Wizard parents: %v\n", wizard.Parents)
	fmt.Printf("Wizard flags: %d\n", wizard.Flags)

	// Look for password property directly
	fmt.Println("\nDirect password property:")
	if prop, ok := wizard.Properties["password"]; ok {
		v := prop.View()
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
	for name, prop := range wizard.Properties {
		v := prop.View()
		fmt.Printf("  %s = %v (Clear=%v)\n", name, v.Value, v.Clear)
	}
}

func findPassword(store *dbstore.Store, objID types.ObjID, visited map[types.ObjID]bool, depth int) {
	if visited[objID] {
		return
	}
	visited[objID] = true

	obj := store.Get(objID)
	if obj == nil {
		return
	}

	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}

	fmt.Printf("%s#%d (%s):\n", indent, objID, obj.Name)

	// Check Properties for password
	if prop, ok := obj.Properties["password"]; ok {
		v := prop.View()
		fmt.Printf("%s  .password = %v (type: %T, Clear=%v)\n", indent, v.Value, v.Value, v.Clear)
	}

	// Recurse to parents
	for _, parentID := range obj.Parents {
		findPassword(store, parentID, visited, depth+1)
	}
}
