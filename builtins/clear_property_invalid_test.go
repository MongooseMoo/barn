package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast bf_clear_prop raises E_INVARG for any object that is not valid:
// #-1, a never-created number and a recycled object alike (issue #1155).
func TestClearPropertyInvalidObjectIsInvarg(t *testing.T) {
	ctx := clearPropertyFixture(t)
	for _, obj := range []types.ObjID{-1, 2, 99} {
		for _, name := range []string{"visible", "absent", "name"} {
			got := builtinClearProperty(ctx, []types.Value{types.NewObj(obj), types.NewStr(name)})
			if got.Error != types.E_INVARG {
				t.Errorf("clear_property(#%d, %q) = %v, want E_INVARG", obj, name, got)
			}
		}
	}
}
