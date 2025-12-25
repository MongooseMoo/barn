package eval

import "barn/types"

// Environment manages variable bindings with lexical scoping
// Supports nested scopes (local variables, global variables, etc.)
type Environment struct {
	vars   map[string]types.Value
	parent *Environment
}

// NewEnvironment creates a new environment with no parent (global scope)
func NewEnvironment() *Environment {
	return &Environment{
		vars:   make(map[string]types.Value),
		parent: nil,
	}
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
