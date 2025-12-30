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
	fmt.Println("Verbs:")
	for name := range obj.Verbs {
		fmt.Printf("  %s\n", name)
	}
}
