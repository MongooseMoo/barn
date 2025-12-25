package builtins

import (
	"barn/types"
)

// BuiltinFunc is a function type for builtin functions
// Takes a task context and list of arguments, returns a Result
type BuiltinFunc func(ctx *types.TaskContext, args []types.Value) types.Result

// Registry holds all registered builtin functions
type Registry struct {
	funcs map[string]BuiltinFunc
}

// NewRegistry creates a new builtin function registry
func NewRegistry() *Registry {
	r := &Registry{
		funcs: make(map[string]BuiltinFunc),
	}

	// Register type conversion builtins
	r.Register("typeof", builtinTypeof)
	r.Register("tostr", builtinTostr)
	r.Register("toint", builtinToint)
	r.Register("tofloat", builtinTofloat)

	// Register string builtins (Layer 7.1)
	r.Register("length", builtinLength)
	r.Register("strsub", builtinStrsub)
	r.Register("index", builtinIndex)
	r.Register("rindex", builtinRindex)
	r.Register("strcmp", builtinStrcmp)
	r.Register("upcase", builtinUpcase)
	r.Register("downcase", builtinDowncase)
	r.Register("capitalize", builtinCapitalize)
	r.Register("explode", builtinExplode)
	r.Register("implode", builtinImplode)
	r.Register("trim", builtinTrim)
	r.Register("ltrim", builtinLtrim)
	r.Register("rtrim", builtinRtrim)

	return r
}

// Register adds a builtin function to the registry
func (r *Registry) Register(name string, fn BuiltinFunc) {
	r.funcs[name] = fn
}

// Get retrieves a builtin function by name
// Returns (function, true) if found, (nil, false) if not found
func (r *Registry) Get(name string) (BuiltinFunc, bool) {
	fn, ok := r.funcs[name]
	return fn, ok
}

// Has checks if a builtin function is registered
func (r *Registry) Has(name string) bool {
	_, ok := r.funcs[name]
	return ok
}
