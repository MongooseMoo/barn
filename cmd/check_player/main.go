package main

import (
	"fmt"
	"os"
	"barn/db"
	"barn/types"
)

func main() {
	dbPath := "toastcore.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	database, err := db.LoadDatabase(dbPath)
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
		fmt.Printf("  password = %q (Clear=%v, Owner=#%d)\n", prop.Value, prop.Clear, prop.Owner)
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
		fmt.Printf("  %s = %v (Clear=%v)\n", name, prop.Value, prop.Clear)
	}
}

func findPassword(store *db.Store, objID types.ObjID, visited map[types.ObjID]bool, depth int) {
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
		fmt.Printf("%s  .password = %v (type: %T, Clear=%v)\n", indent, prop.Value, prop.Value, prop.Clear)
	}

	// Recurse to parents
	for _, parentID := range obj.Parents {
		findPassword(store, parentID, visited, depth+1)
	}
}
