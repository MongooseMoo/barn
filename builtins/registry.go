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

	// Register list builtins (Layer 7.2)
	r.Register("listappend", builtinListappend)
	r.Register("listinsert", builtinListinsert)
	r.Register("listdelete", builtinListdelete)
	r.Register("listset", builtinListset)
	r.Register("setadd", builtinSetadd)
	r.Register("setremove", builtinSetremove)
	r.Register("is_member", builtinIsMember)
	r.Register("sort", builtinSort)
	r.Register("reverse", builtinReverse)
	r.Register("unique", builtinUnique)

	// Register math builtins (Layer 7.3)
	r.Register("abs", builtinAbs)
	r.Register("min", builtinMin)
	r.Register("max", builtinMax)
	r.Register("random", builtinRandom)
	r.Register("sqrt", builtinSqrt)
	r.Register("sin", builtinSin)
	r.Register("cos", builtinCos)
	r.Register("tan", builtinTan)
	r.Register("asin", builtinAsin)
	r.Register("acos", builtinAcos)
	r.Register("atan", builtinAtan)
	r.Register("sinh", builtinSinh)
	r.Register("cosh", builtinCosh)
	r.Register("tanh", builtinTanh)
	r.Register("exp", builtinExp)
	r.Register("log", builtinLog)
	r.Register("log10", builtinLog10)
	r.Register("ceil", builtinCeil)
	r.Register("floor", builtinFloor)
	r.Register("trunc", builtinTrunc)
	r.Register("floatstr", builtinFloatstr)

	// Register map builtins (Layer 7.5)
	r.Register("mapkeys", builtinMapkeys)
	r.Register("mapvalues", builtinMapvalues)
	r.Register("mapdelete", builtinMapdelete)
	r.Register("maphaskey", builtinMaphaskey)
	r.Register("mapmerge", builtinMapmerge)

	// Register JSON builtins (Layer 10.1)
	r.Register("generate_json", builtinGenerateJson)
	r.Register("parse_json", builtinParseJson)

	// Register network builtins (Layer 12.5)
	r.Register("notify", builtinNotify)
	r.Register("connected_players", builtinConnectedPlayers)
	r.Register("connection_name", builtinConnectionName)
	r.Register("boot_player", builtinBootPlayer)
	r.Register("idle_seconds", builtinIdleSeconds)
	r.Register("connected_seconds", builtinConnectedSeconds)

	// Note: eval() builtin is registered by the Evaluator via RegisterEvalBuiltin()
	// to avoid circular dependencies (eval needs parser which needs eval)

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
