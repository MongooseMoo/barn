package engine

import (
	"runtime"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// Test callbacks can be wired after runtime construction (some capture the
// runtime), while the complete registry layout is fixed before any execution.
func testBuiltinSlot(name string, min, max int64, argTypes []int64, callback *builtins.BuiltinFunc) builtins.Descriptor {
	return builtins.Descriptor{Name: name, Signature: &builtins.Signature{MinArgs: min, MaxArgs: max, ArgTypes: argTypes}, Visibility: builtins.Hidden, Effect: builtins.Transactional, Capability: config.Core,
		Implementation: func(ctx *builtins.Execution, args []types.Value) types.Result { return (*callback)(ctx, args) },
	}
}

func newTestRuntimeWithBuiltins(t *testing.T, store *dbstore.Store, descriptors ...builtins.Descriptor) *Runtime {
	t.Helper()
	return newTestRuntimeWithWorkersAndBuiltins(t, store, config.DefaultOptions(), runtime.GOMAXPROCS(0), descriptors...)
}

func newTestRuntimeWithWorkersAndBuiltins(t *testing.T, store *dbstore.Store, options config.Options, workers int, substitutions ...builtins.Descriptor) *Runtime {
	t.Helper()
	base := vm.Descriptors()
	for _, substitution := range substitutions {
		for i, d := range base {
			if d.Name == substitution.Name {
				base = append(base[:i], base[i+1:]...)
				break
			}
		}
	}
	r, err := builtins.NewRegistryFromDescriptors(options.Capabilities(), append(base, substitutions...))
	if err != nil {
		t.Fatal(err)
	}
	return newRuntimeWithRegistry(store, options, workers, r)
}
