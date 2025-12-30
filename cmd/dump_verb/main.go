package main

import (
	"fmt"
	"barn/db"
	"barn/types"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: dump_verb <objnum> <verbname>")
		return
	}

	database, err := db.LoadDatabase("toastcore.db")
	if err != nil {
		panic(err)
	}
	store := database.NewStoreFromDatabase()

	var objNum int
	fmt.Sscanf(os.Args[1], "%d", &objNum)
	verbName := os.Args[2]

	obj := store.Get(types.ObjID(objNum))
	if obj == nil {
		fmt.Printf("Object #%d not found\n", objNum)
		return
	}

	fmt.Printf("Object #%d: %s\n", objNum, obj.Name)

	verb, ok := obj.Verbs[verbName]
	if !ok {
		fmt.Printf("Verb '%s' not found on #%d\n", verbName, objNum)
		fmt.Println("Available verbs:")
		for name := range obj.Verbs {
			fmt.Printf("  %s\n", name)
		}
		return
	}

	fmt.Printf("Verb: %s\n", verb.Name)
	fmt.Printf("Code (%d lines):\n", len(verb.Code))
	for i, line := range verb.Code {
		fmt.Printf("%3d: %s\n", i+1, line)
	}
}
