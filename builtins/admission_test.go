package builtins

import "github.com/MongooseMoo/barn/types"

var admissionTestRegistry = NewRegistry()

func admissionTestFunction(name string) BuiltinFunc {
	fn, ok := admissionTestRegistry.Get(name)
	if !ok {
		panic("unknown test builtin: " + name)
	}
	return fn
}

// Tests of public admission enter through the registry signature wrapper.
// They can still exercise a leaf implementation without a VM or host.
func admitTestBuiltin(name string, ctx *Execution, args []types.Value) types.Result {
	fn, ok := admissionTestRegistry.Get(name)
	if !ok {
		panic("unknown test builtin: " + name)
	}
	return fn(ctx, args)
}
