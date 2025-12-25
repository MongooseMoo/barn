package eval

import "barn/types"

// Environment manages variable bindings with lexical scoping
// Supports nested scopes (local variables, global variables, etc.)
type Environment struct {
	vars   map[string]types.Value
	parent *Environment
}

// NewEnvironment creates a new environment with no parent (global scope)
// Pre-populates with MOO's built-in type constants
func NewEnvironment() *Environment {
	env := &Environment{
		vars:   make(map[string]types.Value),
		parent: nil,
	}

	// Define MOO type constants
	// These match the values from typeof() returns
	env.vars["INT"] = types.NewInt(int64(types.TYPE_INT))
	env.vars["OBJ"] = types.NewInt(int64(types.TYPE_OBJ))
	env.vars["STR"] = types.NewInt(int64(types.TYPE_STR))
	env.vars["ERR"] = types.NewInt(int64(types.TYPE_ERR))
	env.vars["LIST"] = types.NewInt(int64(types.TYPE_LIST))
	env.vars["FLOAT"] = types.NewInt(int64(types.TYPE_FLOAT))
	env.vars["MAP"] = types.NewInt(int64(types.TYPE_MAP))
	env.vars["WAIF"] = types.NewInt(int64(types.TYPE_WAIF))
	env.vars["BOOL"] = types.NewInt(int64(types.TYPE_BOOL))

	// Define special object constants
	env.vars["$nothing"] = types.NewObj(types.ObjNothing)
	env.vars["$ambiguous_match"] = types.NewObj(types.ObjAmbiguous)
	env.vars["$failed_match"] = types.NewObj(types.ObjFailedMatch)

	return env
}

// NewNestedEnvironment creates a new environment with a parent scope
func NewNestedEnvironment(parent *Environment) *Environment {
	return &Environment{
		vars:   make(map[string]types.Value),
		parent: parent,
	}
}

// Get looks up a variable by name
// Searches current scope, then parent scopes
// Returns (value, true) if found, (nil, false) if not found
func (e *Environment) Get(name string) (types.Value, bool) {
	// Check current scope
	if val, ok := e.vars[name]; ok {
		return val, true
	}

	// Check parent scopes
	if e.parent != nil {
		return e.parent.Get(name)
	}

	// Not found
	return nil, false
}

// Set assigns a value to a variable in the current scope
// Creates the variable if it doesn't exist
func (e *Environment) Set(name string, value types.Value) {
	e.vars[name] = value
}

// Define creates a new variable in the current scope
// This is the same as Set for now, but semantically distinct
// (useful for distinguishing between declaration and assignment)
func (e *Environment) Define(name string, value types.Value) {
	e.vars[name] = value
}
