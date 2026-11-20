# Standalone verb snapshot cleanup

## Goal

Release the current store snapshot owned by standalone CallVerbWithArgstr on
every return, including snapshot replacement during GC renewal.

## State

Read the store scout report, isolation runbook, login/phantom-login production
callers, runtime GC renewal and existing snapshot/GC regressions. Added one new
focused regression to existing engine/call_verb_budget_test.go. With Go GC
disabled it proves all four ordinary hook paths retain one active snapshot.
The direct cleanup is now implemented; the focused regression passes. Other
local work is outside this slice.

RED command inside verified Debian WSL from this worktree:

```text
go test ./engine -run '^TestCallVerbReleasesCurrentSnapshot$' -count=1 -timeout=30s
```

Raw RED excerpt:

```text
--- FAIL: TestCallVerbReleasesCurrentSnapshot/argstr/read
call_verb_budget_test.go:81: active read transactions after hook = 1, want 0
--- FAIL: TestCallVerbReleasesCurrentSnapshot/argstr/write
call_verb_budget_test.go:81: active read transactions after hook = 1, want 0
--- FAIL: TestCallVerbReleasesCurrentSnapshot/argstr/error
call_verb_budget_test.go:81: active read transactions after hook = 1, want 0
--- FAIL: TestCallVerbReleasesCurrentSnapshot/argstr/run_gc
call_verb_budget_test.go:81: active read transactions after hook = 1, want 0
FAIL github.com/MongooseMoo/barn/engine 0.123s
```

## Decisions and why

Keep the existing Context/transaction owner. An unconditional late-bound defer
releases ctx.StoreTxn after execution and therefore sees any renewed snapshot.
Remove the sweep-only defer that captures the initial transaction receiver.
Do not change roots, admission or continuation semantics.

The new regression calls real compiled MOO through CallVerbWithArgstr for read,
write, native exception with committed writes, and run_gc cases. An observation
builtin records actual transaction pointers; the ordinary run_gc case asserts
the transaction was replaced. Corresponding sweep-hook cases call the real
session VerbCaller with a registered sweep context under held SweepMu and
VMStartMu, preserving the production barrier contract. Reentrant run_gc is
explicitly asserted to retain that sweep snapshot rather than renew it.
Every case verifies the result/published value and zero active read transactions
immediately after return, without waiting for Go finalizers. No VM frames are
manually installed.

Production change: add one unconditional late-bound release immediately after
BeginSnapshot and delete the sweep-only defer that captured its original
receiver. All ownership, publication, roots and continuation code remains in
its existing locations.

GREEN output from the identical explicit new-test selector:

```text
gofmt_exit=0
diff_check_exit=0
ok github.com/MongooseMoo/barn/engine 0.116s
```

Only the two verified literal Go paths were formatted. No local broad tests,
build, Elle checker or benchmark ran. Focused success is not isolation evidence;
the parent must collect the pinned PR workflow's raw histories, path manifests
and checker verdicts before claiming isolation coverage.

## Next step

Return the two-file diff for parent review and independent PR CI/Elle. No git
action, publication or merge was performed by this worker.
