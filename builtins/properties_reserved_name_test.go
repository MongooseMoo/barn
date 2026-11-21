package builtins

import (
	"slices"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// A database can carry a property definition named like a built-in property
// (issue #1091). Toast's db_find_property (src/db_properties.cc) finds the
// built-in property first, so property_info and set_property_info
// (src/property.cc bf_prop_info, set_prop_info) raise E_PROPNF and leave the
// definition unchanged; properties() still lists it.
func TestReservedNamePropdefIsNotFoundByName(t *testing.T) {
	list := func(vals ...types.Value) types.Value { return types.NewList(vals) }
	str := types.NewStr
	cases := []struct {
		name string
		fn   func(*Execution, []types.Value) types.Result
		args []types.Value
	}{
		{"property_info", builtinPropertyInfo, []types.Value{str("location")}},
		{"property_info other case", builtinPropertyInfo, []types.Value{str("LOCATION")}},
		{"set_property_info", builtinSetPropertyInfo, []types.Value{str("location"), list(types.NewObj(0), str("r"))}},
		{"set_property_info rename", builtinSetPropertyInfo, []types.Value{str("location"), list(types.NewObj(0), str("r"), str("renamed"))}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store := verbMetadataTxnTestContext(t)
			if errCode := store.DirectTxn().DefineProperty(0, "location", dbstore.NewProperty(types.NewStr("kept"), 0, dbstore.PropRead|dbstore.PropChown, false, true)); errCode != types.E_NONE {
				t.Fatalf("DefineProperty failed: %v", errCode)
			}
			ctx.StoreTxn.Release()
			ctx.StoreTxn = store.BeginSnapshot(0)

			result := tc.fn(ctx, append([]types.Value{types.NewObj(0)}, tc.args...))
			if !result.IsError() || result.Error != types.E_PROPNF {
				t.Fatalf("got %v, want E_PROPNF", result)
			}

			names, errCode := definedPropertyNamesForRead(ctx, 0)
			if errCode != types.E_NONE || !slices.Equal(names, []string{"marker", "location"}) {
				t.Fatalf("properties = %q (%v), want [marker location]", names, errCode)
			}
			prop, ok, errCode := localPropertyForRead(ctx, 0, "location")
			if !ok || errCode != types.E_NONE || prop.Perms != dbstore.PropRead|dbstore.PropChown {
				t.Fatalf("definition changed: %+v present %v (%v)", prop, ok, errCode)
			}
		})
	}
}
