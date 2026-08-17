package builtins

import (
	"sync"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Characterization tests for the per-builtin-call dispatch path (CallByID,
// argument validation, and protected-builtin redirection). These pin the
// observable behavior + error codes BEFORE the perf refactor that removes the
// double map lookup, the per-call validation closure, and the RWMutex in
// IsProtectedBuiltin. They must stay green across that refactor unchanged.

// protectBuiltinViaStore builds a store whose #0.server_options points at an
// object carrying protect_<name>=1, then drives the real loader so the
// registry's protected set is populated exactly the way the running server does it. The
// returned store also has #0 present so FindVerb(#0, ...) works.
func protectBuiltinViaStore(t *testing.T, s *Session, name string) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()

	root := dbstore.NewObject(types.ObjID(0), types.ObjID(0))
	if err := store.Add(root); err != nil {
		t.Fatalf("add #0: %v", err)
	}
	opts := dbstore.NewObject(types.ObjID(2), types.ObjID(2))
	if err := store.Add(opts); err != nil {
		t.Fatalf("add #2: %v", err)
	}

	if code := store.DirectTxn().DefineProperty(0, "server_options", dbstore.NewProperty(
		types.NewObj(types.ObjID(2)), 0, dbstore.PropRead|dbstore.PropWrite, false, true,
	)); code != types.E_NONE {
		t.Fatalf("define server_options: %v", code)
	}
	if code := store.DirectTxn().DefineProperty(2, "protect_"+name, dbstore.NewProperty(
		types.NewInt(1), 0, dbstore.PropRead|dbstore.PropWrite, false, true,
	)); code != types.E_NONE {
		t.Fatalf("define protect_%s: %v", name, code)
	}

	s.LoadProtectedBuiltinsFromStore(store)
	if !s.IsProtectedBuiltin(name) {
		t.Fatalf("setup: %q not protected after load", name)
	}
	return store
}

// TestProtectedBuiltinConcurrentReadWrite hammers the lock-free protected set
// with concurrent readers while a writer swaps the snapshot. Under `-race` this
// asserts there is no data race between IsProtectedBuiltin's atomic load and
// LoadProtectedBuiltinsFromStore's atomic store, validating the memory-model
// contract documented in protected.go.
func TestProtectedBuiltinConcurrentReadWrite(t *testing.T) {
	s := NewSession(NewRegistry(), NoHost())
	store := protectBuiltinViaStore(t, s, "abs")
	var wg sync.WaitGroup

	// Readers.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5000; j++ {
				_ = s.IsProtectedBuiltin("abs")
				_ = s.IsProtectedBuiltin("tostr")
			}
		}()
	}
	// Writer: alternately publishes the protected set and an empty set.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 2000; j++ {
			if j%2 == 0 {
				s.LoadProtectedBuiltinsFromStore(store)
			} else {
				s.LoadProtectedBuiltinsFromStore(nil)
			}
		}
	}()
	wg.Wait()
}

func TestCallByIDInvalidIDErrors(t *testing.T) {
	r := NewRegistry()
	s := NewSession(r, NoHost())
	ctx := newTestExecutionForSession(s)

	for _, id := range []int{-1, 1 << 30, 1 << 20} {
		res := s.CallByIDWithExecution(id, ctx, nil)
		if !res.IsError() || res.Error != types.E_VERBNF {
			t.Fatalf("CallByID(%d) = %+v, want E_VERBNF", id, res)
		}
	}
}

func TestCallByIDValidatesArgCountAndType(t *testing.T) {
	r := NewRegistry()
	s := NewSession(r, NoHost())
	ctx := newTestExecutionForSession(s)

	id, ok := r.GetID("sqlite_close") // signature: minArg 1, maxArg 1, argTypes {TYPE_INT}
	if !ok {
		t.Fatal("sqlite_close not registered")
	}

	// Wrong arg count -> E_ARGS (validation runs before the builtin body).
	res := s.CallByIDWithExecution(id, ctx, nil)
	if !res.IsError() || res.Error != types.E_ARGS {
		t.Fatalf("sqlite_close() = %+v, want E_ARGS", res)
	}
	res = s.CallByIDWithExecution(id, ctx, []types.Value{types.NewInt(1), types.NewInt(2)})
	if !res.IsError() || res.Error != types.E_ARGS {
		t.Fatalf("sqlite_close(1,2) = %+v, want E_ARGS", res)
	}

	// Wrong arg type -> E_TYPE.
	res = s.CallByIDWithExecution(id, ctx, []types.Value{types.NewStr("x")})
	if !res.IsError() || res.Error != types.E_TYPE {
		t.Fatalf("sqlite_close(\"x\") = %+v, want E_TYPE", res)
	}
}

// CallByName must validate identically to CallByID.
func TestCallByNameValidatesArgs(t *testing.T) {
	r := NewRegistry()
	s := NewSession(r, NoHost())
	ctx := newTestExecutionForSession(s)

	res, ok := s.CallByNameWithExecution("sqlite_close", ctx, []types.Value{types.NewStr("x")})
	if !ok {
		t.Fatal("sqlite_close not found by name")
	}
	if !res.IsError() || res.Error != types.E_TYPE {
		t.Fatalf("sqlite_close(\"x\") by name = %+v, want E_TYPE", res)
	}

	if _, ok := s.CallByNameWithExecution("no_such_builtin", ctx, nil); ok {
		t.Fatal("CallByName(unknown) returned ok=true")
	}
}

func TestProtectedBuiltinNonWizardDeniedWizardFallsThrough(t *testing.T) {
	r := NewRegistry()
	s := NewSession(r, NoHost())

	store := protectBuiltinViaStore(t, s, "abs")
	id, ok := r.GetID("abs")
	if !ok {
		t.Fatal("abs not registered")
	}

	// Non-wizard caller, this != #0, no #0:bf_abs verb -> E_PERM.
	nonwiz := newTestExecutionForSession(s)
	nonwiz.Store = store
	nonwiz.ThisObj = types.ObjID(1)
	nonwiz.IsWizard = false
	res := s.CallByIDWithExecution(id, nonwiz, []types.Value{types.NewInt(-5)})
	if !res.IsError() || res.Error != types.E_PERM {
		t.Fatalf("protected abs, non-wizard = %+v, want E_PERM", res)
	}

	// Wizard caller, no wrapper verb -> falls through to the real builtin.
	wiz := newTestExecutionForSession(s)
	wiz.Store = store
	wiz.ThisObj = types.ObjID(1)
	wiz.IsWizard = true
	res = s.CallByIDWithExecution(id, wiz, []types.Value{types.NewInt(-5)})
	if !res.IsNormal() {
		t.Fatalf("protected abs, wizard fallthrough = %+v, want normal", res)
	}
	if res.Val.Type() != types.TYPE_INT || res.Val.Int() != 5 {
		t.Fatalf("abs(-5) fallthrough = %+v, want 5", res.Val)
	}
}

func TestProtectedBuiltinRedirectsToWrapperVerb(t *testing.T) {
	r := NewRegistry()
	s := NewSession(r, NoHost())

	store := protectBuiltinViaStore(t, s, "abs")
	// Add #0:bf_abs so the redirect path resolves the wrapper verb.
	verb := dbstore.NewVerb(
		"bf_abs", []string{"bf_abs"}, types.ObjID(0),
		dbstore.VerbPerms(0),
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"return 12345;"},
	)
	if _, code := store.AddVerb(types.ObjID(0), verb); code != types.E_NONE {
		t.Fatalf("add bf_abs: %v", code)
	}

	sentinel := types.NewInt(99887766)
	called := false
	configureTestHost(s, func(host *Host) {
		host.VerbCaller = func(objID types.ObjID, verbName string, args []types.Value, ctx *Execution) types.Result {
			called = true
			if objID != types.ObjID(0) || verbName != "bf_abs" {
				t.Fatalf("redirect called %d:%s, want #0:bf_abs", objID, verbName)
			}
			return types.Ok(sentinel)
		}
	})

	id, _ := r.GetID("abs")
	ctx := newTestExecutionForSession(s)
	ctx.Store = store
	ctx.ThisObj = types.ObjID(1)
	ctx.IsWizard = false

	res := s.CallByIDWithExecution(id, ctx, []types.Value{types.NewInt(-5)})
	if !called {
		t.Fatal("redirect verb caller was not invoked")
	}
	if !res.IsNormal() || res.Val.Int() != sentinel.Int() {
		t.Fatalf("redirect result = %+v, want sentinel %d", res, sentinel.Int())
	}
}

// caller this == #0 must always run the real builtin even when protected.
func TestProtectedBuiltinThisZeroRunsRealBuiltin(t *testing.T) {
	r := NewRegistry()
	s := NewSession(r, NoHost())

	store := protectBuiltinViaStore(t, s, "abs")
	id, _ := r.GetID("abs")

	ctx := newTestExecutionForSession(s)
	ctx.Store = store
	ctx.ThisObj = types.ObjID(0)
	ctx.IsWizard = false

	res := s.CallByIDWithExecution(id, ctx, []types.Value{types.NewInt(-5)})
	if !res.IsNormal() || res.Val.Int() != 5 {
		t.Fatalf("this==#0 protected abs(-5) = %+v, want 5", res)
	}
}
