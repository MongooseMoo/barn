package compiler

import (
	"fmt"
	"strings"
)

// scope tracks variables in a lexical scope.
type scope struct {
	Variables map[string]int
	Parent    *scope
}

// canonicalIdentifier returns the semantic identity of a MOO identifier.
// Source spelling is retained on syntax nodes and in diagnostics; only name
// resolution uses this key.
func canonicalIdentifier(name string) string {
	return strings.ToUpper(name)
}

// beginScope starts a new variable scope
func (c *lowerer) beginScope() {
	scope := scope{
		Variables: make(map[string]int),
	}
	if len(c.scopes) > 0 {
		scope.Parent = &c.scopes[len(c.scopes)-1]
	}
	c.scopes = append(c.scopes, scope)
}

// endScope ends the current variable scope
func (c *lowerer) endScope() {
	if len(c.scopes) > 0 {
		c.scopes = c.scopes[:len(c.scopes)-1]
	}
}

// declareVariable declares a variable in current scope.
// If the variable count exceeds 256 (the maximum addressable by a single byte),
// sets c.err and returns 0 as a safe fallback index.
func (c *lowerer) declareVariable(name string) int {
	key := canonicalIdentifier(name)
	// Check if already exists in global variable table
	if idx, ok := c.variables[key]; ok {
		return idx
	}

	// Check overflow before adding
	idx := len(c.program.VarNames)
	if idx > 255-len(c.internalVariables) {
		if c.err == nil {
			c.err = fmt.Errorf("too many local variables (max 256)")
		}
		return 0 // safe fallback; c.err will be checked at Compile boundary
	}

	// Add to global variable table
	displayName := name
	if _, isTypeName := builtinConstants[key]; isTypeName {
		displayName = key
	}
	c.program.VarNames = append(c.program.VarNames, displayName)
	c.variables[key] = idx
	c.program.BuiltinSlots.Set(strings.ToLower(key), idx)

	// Track max locals
	if idx+1 > c.program.NumLocals {
		c.program.NumLocals = idx + 1
	}

	// Add to current scope
	if len(c.scopes) > 0 {
		c.scopes[len(c.scopes)-1].Variables[key] = idx
	}

	return idx
}

// declareInternalVariable allocates a compiler-only local from the top of the
// one-byte local operand range. Internal locals live outside VarNames and the
// source-variable lookup table, so no legal MOO identifier can alias or expose
// compiler bookkeeping. Repeated internal names intentionally reuse a slot
// when their lifetimes are known not to overlap.
func (c *lowerer) declareInternalVariable(name string) int {
	if idx, ok := c.internalVariables[name]; ok {
		return idx
	}

	idx := 255 - len(c.internalVariables)
	if idx < len(c.program.VarNames) {
		if c.err == nil {
			c.err = fmt.Errorf("too many local variables (max 256 including compiler temporaries)")
		}
		return 0
	}

	c.internalVariables[name] = idx
	if idx+1 > c.program.NumLocals {
		c.program.NumLocals = idx + 1
	}
	return idx
}

// resolveVariable resolves a variable name to its index
func (c *lowerer) resolveVariable(name string) (int, bool) {
	idx, ok := c.variables[canonicalIdentifier(name)]
	return idx, ok
}

// tempVar generates a unique temporary variable name
func (c *lowerer) tempVar(prefix string) string {
	c.tempCount++
	return fmt.Sprintf("__%s_%d__", prefix, c.tempCount)
}
