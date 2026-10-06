package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// runMidTask runs setup as one task and body as a later one, so the verbs body
// calls were resolved and published by an earlier transaction. It runs body
// once as an eval and once, on a fresh database, as a committing verb task.
func runMidTask(t *testing.T, setup, body, want string) {
	t.Helper()
	t.Run("eval", func(t *testing.T) {
		s := NewRuntime(protectedRedirectStore(t))
		defer s.Stop()
		if got := s.EvalCommandOutput(2, setup+"return 1;"); got != "{1, 1}" {
			t.Fatalf("setup: %s", got)
		}
		if got := s.EvalCommandOutput(2, body); got != "{1, "+want+"}" {
			t.Fatalf("got %s, want {1, %s}", got, want)
		}
	})
	t.Run("verb task", func(t *testing.T) {
		store := protectedRedirectStore(t)
		s := NewRuntime(store)
		defer s.Stop()
		if got := s.EvalCommandOutput(2, setup+"return 1;"); got != "{1, 1}" {
			t.Fatalf("setup: %s", got)
		}
		verb := dbstore.NewVerb("go", []string{"go"}, 2, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
			dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, []string{body})
		if _, errCode := store.AddVerb(1, verb); errCode != types.E_NONE {
			t.Fatalf("add verb: %s", errCode)
		}
		result := s.CallVerb(1, "go", nil, 2)
		if result.Flow == types.FlowException {
			t.Fatalf("go raised %s", result.Error)
		}
		if got := result.Val.String(); got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	})
}

// BenchmarkVerbDispatch times 5,000 calls per iteration through a task's
// snapshot transaction: a verb inherited from a parent, and a protected builtin
// redirected to its #0:bf_ wrapper (an activation and a real builtin call).
// The loops stay inside an eval's default tick allowance.
func BenchmarkVerbDispatch(b *testing.B) {
	setup := protectValidSetup +
		`add_property(#1, "kid", 0, {player, "rw"});` +
		`p = create(#-1); #1.kid = create(p);` +
		`add_verb(p, {player, "rxd", "foo"}, {"this", "none", "this"});` +
		`set_verb_code(p, "foo", {"return 1;"});` +
		`return 1;`
	for _, w := range []struct{ name, code string }{
		{"verb_call_5k", `c = #1.kid; for i in [1..5000] c:foo(); endfor return 1;`},
		{"protected_builtin_5k", `for i in [1..5000] valid(#0); endfor return 1;`},
	} {
		b.Run(w.name, func(b *testing.B) {
			s := NewRuntime(protectedRedirectStore(b))
			defer s.Stop()
			if got := s.EvalCommandOutput(2, setup); got != "{1, 1}" {
				b.Fatalf("setup: %s", got)
			}
			if got := s.EvalCommandOutput(2, w.code); got != "{1, 1}" {
				b.Fatalf("warm-up: %s", got)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got := s.EvalCommandOutput(2, w.code); got != "{1, 1}" {
					b.Fatalf("got %s", got)
				}
			}
		})
	}
}

// A protected builtin called in a loop must follow, call by call, what the task
// itself does to the wrapper verb and to the protect flag: new wrapper code, the
// flag cleared and set again, the wrapper deleted (a wizard then gets the real
// builtin) and added back.
func TestProtectedBuiltinRedirectFollowsMidTaskChanges(t *testing.T) {
	body := `r = {};` +
		`for i in [1..3] r = {@r, valid(#0)}; endfor ` +
		`set_verb_code(#0, "bf_valid", {"return valid(args[1]) + 20;"});` +
		`for i in [1..2] r = {@r, valid(#0)}; endfor ` +
		`#1.protect_valid = 0; load_server_options();` +
		`for i in [1..2] r = {@r, valid(#0)}; endfor ` +
		`#1.protect_valid = 1; load_server_options();` +
		`for i in [1..2] r = {@r, valid(#0)}; endfor ` +
		`delete_verb(#0, "bf_valid");` +
		`for i in [1..2] r = {@r, valid(#0)}; endfor ` +
		`add_verb(#0, {player, "rxd", "bf_valid"}, {"this", "none", "this"});` +
		`set_verb_code(#0, "bf_valid", {"return 30;"});` +
		`for i in [1..2] r = {@r, valid(#0)}; endfor ` +
		`return r;`
	runMidTask(t, protectValidSetup, body, "{11, 11, 11, 21, 21, 1, 1, 21, 21, 1, 1, 30, 30}")
}

// A verb called in a loop must follow the task's own changes to which verb the
// call resolves to: one added on the object itself, that one deleted again, a
// reparent, and the execute bit cleared on the definition.
func TestVerbCallFollowsMidTaskShapeChanges(t *testing.T) {
	setup := `add_property(#1, "kid", 0, {player, "rw"});` +
		`add_property(#1, "other", 0, {player, "rw"});` +
		`p = create(#-1); c = create(p); q = create(#-1); #1.kid = c; #1.other = q;` +
		`add_verb(p, {player, "rxd", "foo"}, {"this", "none", "this"});` +
		`set_verb_code(p, "foo", {"return 1;"});` +
		`add_verb(q, {player, "rxd", "foo"}, {"this", "none", "this"});` +
		`set_verb_code(q, "foo", {"return 3;"});` +
		`c:foo(); q:foo();`
	body := `c = #1.kid; r = {};` +
		`for i in [1..3] r = {@r, c:foo()}; endfor ` +
		`add_verb(c, {player, "rxd", "foo"}, {"this", "none", "this"});` +
		`set_verb_code(c, "foo", {"return 2;"});` +
		`for i in [1..2] r = {@r, c:foo()}; endfor ` +
		`delete_verb(c, "foo");` +
		`for i in [1..2] r = {@r, c:foo()}; endfor ` +
		`chparent(c, #1.other);` +
		`for i in [1..2] r = {@r, c:foo()}; endfor ` +
		`set_verb_code(#1.other, "foo", {"return 4;"});` +
		`for i in [1..2] r = {@r, c:foo()}; endfor ` +
		"set_verb_info(#1.other, \"foo\", {player, \"rd\", \"foo\"});" +
		"for i in [1..2] r = {@r, `c:foo() ! E_VERBNF => 0'}; endfor " +
		`return r;`
	runMidTask(t, setup, body, "{1, 1, 1, 2, 2, 1, 1, 3, 3, 4, 4, 0, 0}")
}
