# Verb Mutation Store Cleanup Fixed Point

Date: 2026-06-17

## Target

`db.Store` owns verb map/list consistency and verb source/code mutation. Builtins, VM, and server callers no longer write `obj.Verbs`, splice `obj.VerbList`, or clear verb code caches directly when changing verb code.

## Deleted Call-Site Logic

- Removed add/delete/map-refresh logic from `builtins/verbs.go`.
- Removed verb info, args, and code field mutation from `builtins/verbs.go`.
- Removed `.program` verb code mutation and local verb finder from `server/scheduler.go`.
- Removed direct parent verb traversal over `obj.Verbs` from VM `pass()`.

## Store-Owned Operations Added

- `VerbNames`
- `VerbByIndex`
- `AddVerb`
- `DeleteVerb`
- `SetVerbInfo`
- `SetVerbArgs`
- `SetVerbCode`
- `SetVerbCodeByIndex`
- `FindParentVerb`
- `FindLocalVerbForProgramming`

## Search Gate

Command:

```text
rg -n "obj\\.Verbs\\[|delete\\(obj\\.Verbs|obj\\.VerbList\\s*=|append\\(obj\\.VerbList" builtins server vm db
```

Result classification:

- Remaining `db/store.go` hits are the store-owned verb lookup and mutation methods.
- Remaining `db/reader_v4.go` and `db/reader_object.go` hits are snapshot reader load paths, not runtime verb mutation. They stay for Phase 4 unless snapshot ownership changes earlier.
- `server/scheduler_login_test.go` hits are test setup helpers.

## Gates

```text
go test ./db ./builtins ./vm ./server
```

Result:

- `db`, `builtins`, and `vm` passed.
- `server` failed only at the known unrelated baseline: `TestTLSListenerLoginAndEval` returned `"I couldn't understand that.\r\n"` instead of `{1, 3}`.

Targeted gates:

```text
go test -timeout 120s ./builtins -run "Test.*Verb"
go test -timeout 120s ./vm -run "Test.*(Verb|Pass)"
go test -timeout 120s ./server -run "TestDoLoginCommandDispatchesOnListenerWithArgstr|TestLoginPlayerRunsListenerCreatedAndConnectedHooks"
go test -timeout 120s ./server -run "TestTLSListenerLoginAndEval"
git diff --check
```

Results:

- Builtins verb gate passed with no matching tests.
- VM verb/pass gate passed.
- Focused scheduler login/verb-dispatch gate passed.
- Isolated TLS eval test reproduced the known unrelated failure.
- Diff hygiene passed.

## Commit

Pending.

## Next Slice

Phase 3: close object lifecycle and relationship mutation through `db.Store`.
