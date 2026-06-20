package conformance

import (
	"fmt"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/parser"
	"barn/types"
)

// setupStoreForTests applies test-required properties to the store
// This ensures $sysobj, $anon, prototype properties etc. exist
func setupStoreForTests(store *dbstore.Store) {
	// Ensure standard MOO system properties exist on #0
	// These are expected by conformance tests (same as cow_py transport.py)
	if _, ok := store.Get(0); !ok {
		return
	}

	localPropValue := func(objID types.ObjID, name string) (types.Value, bool) {
		view, exists, errCode := store.LocalProperty(objID, name)
		if errCode != types.E_NONE || !exists {
			return nil, false
		}
		return view.Value, true
	}

	// $sysobj -> #0 (the system object itself)
	if _, ok := localPropValue(0, "sysobj"); !ok {
		store.DefineProperty(0, dbstore.NewProperty("sysobj", types.NewObj(0), 0, dbstore.PropRead, false, true))
	}
	// $object -> #1 (root object class)
	if _, ok := localPropValue(0, "object"); !ok {
		store.DefineProperty(0, dbstore.NewProperty("object", types.NewObj(1), 0, dbstore.PropRead, false, true))
	}
	// $anon -> anonymous object parent
	// First check if $anonymous exists, reuse that
	if _, ok := localPropValue(0, "anon"); !ok {
		var anonID types.ObjID = -1
		if val, exists := localPropValue(0, "anonymous"); exists {
			if objVal, ok := val.(types.ObjValue); ok {
				anonID = objVal.ID()
			}
		}
		// If no $anonymous property, find an object with anonymous flag
		if anonID == -1 {
			for id := types.ObjID(0); id <= store.MaxObject(); id++ {
				if has, errCode := store.HasObjectFlag(id, dbstore.FlagAnonymous); errCode == types.E_NONE && has {
					anonID = id
					break
				}
			}
		}
		// If still not found, create one
		if anonID == -1 {
			anonID = store.NextID()
			builder := dbstore.NewObjectBuilder(anonID)
			builder.SetOwner(0)
			builder.SetName("Anonymous Object Parent")
			builder.SetAnonymous(true)
			builder.SetFlags(dbstore.ObjectFlags(0).Set(dbstore.FlagAnonymous).Set(dbstore.FlagFertile))
			store.Add(builder.Build())
		}
		store.DefineProperty(0, dbstore.NewProperty("anon", types.NewObj(anonID), 0, dbstore.PropRead, false, true))
	}
	// Ensure the anonymous parent has the fertile flag so non-wizards can create from it
	if val, ok := localPropValue(0, "anon"); ok {
		if anonObjVal, ok := val.(types.ObjValue); ok {
			store.SetObjectFlag(anonObjVal.ID(), dbstore.FlagFertile, true)
			store.SetObjectFlag(anonObjVal.ID(), dbstore.FlagAnonymous, true)
		}
	}
	// Also ensure $anonymous has fertile flag if it exists
	if val, ok := localPropValue(0, "anonymous"); ok {
		if anonObjVal, ok := val.(types.ObjValue); ok {
			store.SetObjectFlag(anonObjVal.ID(), dbstore.FlagFertile, true)
			store.SetObjectFlag(anonObjVal.ID(), dbstore.FlagAnonymous, true)
		}
	}
	// Add prototype properties for primitive types (needed by primitives.yaml tests)
	// These are writable so tests can set their own prototypes
	protoProps := []string{"int_proto", "str_proto", "list_proto", "map_proto", "float_proto", "err_proto"}
	for _, propName := range protoProps {
		if _, ok := localPropValue(0, propName); !ok {
			store.DefineProperty(0, dbstore.NewProperty(propName, types.NewObj(-1), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
	}
}

// runSetupBlock executes a setup or teardown block
func (r *Runner) runSetupBlock(block *SetupBlock, ctx *kernel.TaskContext) error {
	if block == nil {
		return nil
	}

	// Save original wizard state and apply setup's permissions
	origWizard := ctx.IsWizard
	if block.Permission == "wizard" {
		ctx.IsWizard = true
	}
	defer func() { ctx.IsWizard = origWizard }()

	var code string
	if block.Statement != "" {
		code = block.Statement
	} else if block.Code != "" {
		code = block.Code
	} else {
		return nil
	}

	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		return fmt.Errorf("setup parse error: %w", err)
	}

	result := r.executeStatements(stmts, ctx)
	if result.Flow == types.FlowException {
		return fmt.Errorf("setup error: %s", errorCodeToName(result.Error))
	}
	if result.Flow == types.FlowParseError {
		return fmt.Errorf("setup compile error: %v", result.Val)
	}

	return nil
}
