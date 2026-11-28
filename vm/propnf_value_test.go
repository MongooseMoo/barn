package vm

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Toast raises E_PROPNF from OP_GET_PROP and OP_PUT_PROP through
// PUSH_X_NOT_FOUND (execute.cc): the message is "Property not found: #N.name"
// and the value is {#N, name}, for a read and an assignment alike.
func TestPropertyNotFoundErrorValue(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		// Nightly fuzz ToastDifferential input (issue #1201): the assignment
		// raised a bare "Property not found" with value 0.
		{"fuzz input", "try #0.p = 5; return {#0.p}; except e (ANY) return e[1..3]; endtry", `{E_PROPNF, "Property not found: #0.p", {#0, "p"}}`},
		{"read", "try return #0.nosuch; except e (ANY) return e[1..3]; endtry", `{E_PROPNF, "Property not found: #0.nosuch", {#0, "nosuch"}}`},
		{"dynamic read", `try return #0.("nosuch"); except e (ANY) return e[1..3]; endtry`, `{E_PROPNF, "Property not found: #0.nosuch", {#0, "nosuch"}}`},
		{"dynamic assignment", `try #0.("nosuch") = 1; except e (ANY) return e[1..3]; endtry`, `{E_PROPNF, "Property not found: #0.nosuch", {#0, "nosuch"}}`},
		{"system property assignment", "try $nosuch = 1; except e (ANY) return e[1..3]; endtry", `{E_PROPNF, "Property not found: #0.nosuch", {#0, "nosuch"}}`},
		{"keyed read", "try return #0.nosuch[1]; except e (ANY) return e[1..3]; endtry", `{E_PROPNF, "Property not found: #0.nosuch", {#0, "nosuch"}}`},
		{"keyed assignment base", "try x = #0; x.nosuch[1] = 1; except e (ANY) return e[1..3]; endtry", `{E_PROPNF, "Property not found: #0.nosuch", {#0, "nosuch"}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := dbstore.NewStore()
			if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
				t.Fatalf("Add(#0): %v", err)
			}
			result := runBytecodeProgram(t, test.source, store, nil)
			if result.Flow != types.FlowReturn || result.Error != types.E_NONE || result.Val.String() != test.want {
				t.Fatalf("result = flow %v, value %v, error %v; want %s", result.Flow, result.Val, result.Error, test.want)
			}
		})
	}
}
