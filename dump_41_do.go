package main
import (
	"fmt"
	"barn/db"
	"barn/types"
)
func main() {
	database, err := db.LoadDatabase("toastcore_barn.db")
	if err != nil {
		panic(err)
	}
	store := database.NewStoreFromDatabase()
	obj := store.Get(types.ObjID(41))
	if obj == nil {
		fmt.Println("Object #41 not found")
		return
	}
	fmt.Printf("Object #41: %s\n", obj.Name)
	verb, ok := obj.Verbs["_do"]
	if !ok {
		fmt.Println("Verb _do not found")
		return
	}
	fmt.Printf("\nVerb: %s\n", verb.Name)
	fmt.Printf("Owner: %d\n", verb.Owner)
	fmt.Printf("Names: %v\n", verb.Names)
	fmt.Printf("\nCode (%d lines):\n", len(verb.Code))
	for i, line := range verb.Code {
		fmt.Printf("%3d: %s\n", i+1, line)
	}
}
