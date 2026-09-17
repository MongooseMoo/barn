package vm

import (
	"fmt"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestNonDebugInvalidCollectionSkipsLoop(t *testing.T) {
	for _, variables := range []string{"item", "item, key"} {
		for _, container := range []string{"args[2]", "E_RANGE", "42", "1.5", "#0"} {
			t.Run(variables+"/"+container, func(t *testing.T) {
				store := newBytecodeVerbStore()
				code := fmt.Sprintf(`item = 7; key = 9; count = 0;
					for outer in ({1, 2})
						for %s in (%s) count = count + 1; endfor
					endfor
					return {item, key, count};`, variables, container)
				store.AddVerb(0, dbstore.NewVerb("probe", []string{"probe"}, 0,
					dbstore.VerbRead|dbstore.VerbExecute, dbstore.VerbArgs{}, []string{code}))
				result := runBytecodeProgram(t, `return {"before", #0:probe({}), "after"};`, store, nil)
				if result.Flow != types.FlowReturn || result.Val.String() != `{"before", {7, 9, 0}, "after"}` {
					t.Fatalf("invalid collection result = %+v, want preserved locals and caller stack", result)
				}
			})
		}
	}
}

func TestDebugInvalidCollectionRaises(t *testing.T) {
	for _, variables := range []string{"item", "item, key"} {
		t.Run(variables, func(t *testing.T) {
			code := fmt.Sprintf(`for %s in (42) return "body"; endfor return "continued";`, variables)
			requireError(t, runBytecodeProgram(t, code, nil, nil), types.E_TYPE)
		})
	}
}
