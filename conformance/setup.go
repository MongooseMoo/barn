package conformance

import (
	"barn/db"
	"barn/parser"
	"barn/types"
	"fmt"
)

// setupStoreForTests applies test-required properties to the store
// This ensures $sysobj, $anon, prototype properties etc. exist
func setupStoreForTests(store *db.Store) {
	// Ensure standard MOO system properties exist on #0
	// These are expected by conformance tests (same as cow_py transport.py)
	if obj := store.Get(0); obj != nil {
		// $sysobj -> #0 (the system object itself)
		if _, ok := obj.Properties["sysobj"]; !ok {
			obj.Properties["sysobj"] = &db.Property{
				Name:    "sysobj",
				Value:   types.NewObj(0),
				Owner:   0,
				Perms:   db.PropRead,
				Defined: true,
			}
		}
		// $object -> #1 (root object class)
		if _, ok := obj.Properties["object"]; !ok {
			obj.Properties["object"] = &db.Property{
				Name:    "object",
				Value:   types.NewObj(1),
				Owner:   0,
				Perms:   db.PropRead,
				Defined: true,
			}
		}
		// $anon -> anonymous object parent
		// First check if $anonymous exists, reuse that
		if _, ok := obj.Properties["anon"]; !ok {
			var anonID types.ObjID = -1
			if anonymousProp, exists := obj.Properties["anonymous"]; exists {
				if objVal, ok := anonymousProp.Value.(types.ObjValue); ok {
					anonID = objVal.ID()
				}
			}
			// If no $anonymous property, find an object with anonymous flag
			if anonID == -1 {
				for id := types.ObjID(0); id <= store.MaxObject(); id++ {
					if o := store.Get(id); o != nil && o.Flags.Has(db.FlagAnonymous) {
						anonID = id
						break
					}
				}
			}
			// If still not found, create one
			if anonID == -1 {
				anonID = store.NextID()
				anonObj := db.NewObject(anonID, 0)
				anonObj.Name = "Anonymous Object Parent"
				anonObj.Anonymous = true
				anonObj.Flags = anonObj.Flags.Set(db.FlagAnonymous).Set(db.FlagFertile)
				store.Add(anonObj)
			}
			obj.Properties["anon"] = &db.Property{
				Name:    "anon",
				Value:   types.NewObj(anonID),
				Owner:   0,
				Perms:   db.PropRead,
				Defined: true,
			}
		}
		// Ensure the anonymous parent has the fertile flag so non-wizards can create from it
		if anonProp, ok := obj.Properties["anon"]; ok {
			if anonObjVal, ok := anonProp.Value.(types.ObjValue); ok {
				if anonObj := store.Get(anonObjVal.ID()); anonObj != nil {
					anonObj.Flags = anonObj.Flags.Set(db.FlagFertile).Set(db.FlagAnonymous)
				}
			}
		}
		// Also ensure $anonymous has fertile flag if it exists
		if anonymousProp, ok := obj.Properties["anonymous"]; ok {
			if anonObjVal, ok := anonymousProp.Value.(types.ObjValue); ok {
				if anonObj := store.Get(anonObjVal.ID()); anonObj != nil {
					anonObj.Flags = anonObj.Flags.Set(db.FlagFertile).Set(db.FlagAnonymous)
				}
			}
		}
		// Add prototype properties for primitive types (needed by primitives.yaml tests)
		// These are writable so tests can set their own prototypes
		protoProps := []string{"int_proto", "str_proto", "list_proto", "map_proto", "float_proto", "err_proto"}
		for _, propName := range protoProps {
			if _, ok := obj.Properties[propName]; !ok {
				obj.Properties[propName] = &db.Property{
					Name:    propName,
					Value:   types.NewObj(-1), // $nothing by default
					Owner:   0,
					Perms:   db.PropRead | db.PropWrite,
					Defined: true,
				}
			}
		}
	}
}

// runSetupBlock executes a setup or teardown block
func (r *Runner) runSetupBlock(block *SetupBlock, ctx *types.TaskContext) error {
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
