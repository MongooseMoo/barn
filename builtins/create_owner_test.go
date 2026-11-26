package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// An owner below $nothing is not a valid object, so create() raises E_INVARG
// whether or not an anonymous flag follows (Toast objects.cc bf_create:
// !valid(owner) && owner != NOTHING). Issue #1181: Barn made an anonymous
// object for create(#0, #-2) instead.
func TestCreateInvalidNegativeOwnerIsInvarg(t *testing.T) {
	ctx, _ := verbMetadataTxnTestContext(t)

	cases := map[string][]types.Value{
		"create(#0, #-2)":    {types.NewObj(0), types.NewObj(-2)},
		"create(#0, #-3)":    {types.NewObj(0), types.NewObj(-3)},
		"create(#0, #-2, 0)": {types.NewObj(0), types.NewObj(-2), types.NewInt(0)},
		"create(#0, #-2, 1)": {types.NewObj(0), types.NewObj(-2), types.NewInt(1)},
	}
	for name, args := range cases {
		got := builtinCreate(ctx, args)
		if !got.IsError() || got.Error != types.E_INVARG {
			t.Errorf("%s = %+v, want E_INVARG", name, got)
		}
	}
}
