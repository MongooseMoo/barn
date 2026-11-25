package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast bf_prop_info (src/property.cc) raises E_INVARG for an object that is
// not valid, whatever its number (issue #1176: property_info(#-1, "be\t")
// raised E_INVIND on Barn).
func TestPropertyInfoInvalidObjectIsInvarg(t *testing.T) {
	for _, id := range []types.ObjID{-1, -2, 99999} {
		for _, name := range []string{"be\t", "name", "marker"} {
			ctx, _ := verbMetadataTxnTestContext(t)
			result := builtinPropertyInfo(ctx, []types.Value{types.NewObj(id), types.NewStr(name)})
			if !result.IsError() || result.Error != types.E_INVARG {
				t.Fatalf("property_info(#%d, %q) = %v, want E_INVARG", id, name, result)
			}
		}
	}
}
