# Split vm/operations.go report

## Summary

`vm/operations.go` (2489 lines) split by op-family into 10 new files. Pure
relocation: zero signature, body, comment, or rename changes. The
`indexing.go`, `vm.go`, and other vm/* files are untouched.

## Final line counts

| File              | Lines |
| ----------------- | ----- |
| vm/op_arith.go    |   277 |
| vm/op_bitwise.go  |   119 |
| vm/op_compare.go  |   224 |
| vm/op_index.go    |   481 |
| vm/op_iter.go     |   127 |
| vm/op_list.go     |   128 |
| vm/op_logic.go    |    51 |
| vm/op_misc.go     |   116 |
| vm/op_property.go |   523 |
| vm/op_verb.go     |   500 |
| **total**         |  2546 |

`vm/operations.go` deleted (0 lines).

The 57-line delta vs the original 2489 is the per-file `package vm` +
`import (...)` block overhead times 10 files.

## Function placement

All target-layout functions existed in `operations.go`. Nothing was missing.
Nothing extra was found. Functions appear in each new file in the same source
order they had in `operations.go`.

| File              | Functions placed (in source order) |
| ----------------- | ---------------------------------- |
| op_arith.go       | executeAdd, executeSub, executeMul, executeDiv, executeMod, executePow, executeNeg |
| op_compare.go     | executeEq, executeNe, executeLt, executeLe, executeGt, executeGe, executeIn, compareValues |
| op_logic.go       | executeNot, executeAnd, executeOr |
| op_bitwise.go     | executeBitOr, executeBitAnd, executeBitXor, executeBitNot, executeShl, executeShr |
| op_index.go       | executeIndex, executeIndexSet, executeRangeSet, executeRange, executeIndexMarker, executeListRange |
| op_list.go        | executeMakeList, executeMakeMap, executeLength, executeListAppend, executeListExtend, executeSplice |
| op_property.go    | executeGetProp, vmGetWaifProp, executeSetProp, vmSetWaifProp, checkPropertyReadPerm, checkPropertyWritePerm, vmFindProperty, vmGetBuiltinProperty, vmSetBuiltinProperty |
| op_iter.go        | executeIterPrep, executeScatter, setLocalByName |
| op_verb.go        | executeCallVerb, executePass |
| op_misc.go        | executeCallBuiltin, getPrimitivePrototypeFromStore |

Note: `executeIndexMarker` and `executeListRange` were listed in the prompt's
target layout for `op_index.go` in a different order than they appear in
`operations.go`. I followed the source order (IndexMarker before ListRange,
matching the original layout) per the "preserve original function order"
instruction.

## Imports per file

Each new file imports only what its functions actually use.

| File              | Imports |
| ----------------- | ------- |
| op_arith.go       | barn/builtins, barn/types, fmt, math |
| op_compare.go     | barn/types, fmt, strings |
| op_logic.go       | barn/types |
| op_bitwise.go     | barn/types, fmt |
| op_index.go       | barn/builtins, barn/types, fmt, sort |
| op_list.go        | barn/builtins, barn/types, fmt |
| op_property.go    | barn/db, barn/types, fmt, strings |
| op_iter.go        | barn/types, fmt |
| op_verb.go        | barn/db, barn/task, barn/trace, barn/types, fmt, strings |
| op_misc.go        | barn/db, barn/types, fmt |

## Verification

### Build

`go build ./vm/...` — clean.

`go build ./...` — fails on `barn/server` and `barn/builtins` due to a
concurrent in-flight refactor (codex's "Split builtins/objects.go by topic"
and "Split server/scheduler.go into themed files" landed during this work);
not caused by this split. `vm/` is unaffected.

### Vet

`go vet ./vm/...` — one pre-existing warning unchanged by this split:

```
vm\vm.go:752:15: method ReadByte() byte should have signature ReadByte() (byte, error)
```

`vm.go` is on the hold-back list and was not touched.

### Tests

`go test ./vm/... -count=1`

Baseline (master, before split) — 10 top-level FAIL test groups, captured at
`.tmp/vm-baseline.log`:

```
TestBuiltinExceptionTracebackLines
TestParity_ScatterComplex
TestParity_ScatterOptional
TestParity_VerbCallCrossFrameFinally
TestParity_VerbCallDeepFinallyUnwind
TestParity_VerbCallDeepUnwind
TestParity_VerbCallThrows
TestParity_VerbCallThrowsCaught
TestParity_VerbCallUnhandledException
TestTryExcept
```

Post-split — same 10 top-level FAIL test groups, captured at
`.tmp/vm-postsplit.log`. Identical sets, identical sub-test failures. No new
failures. The pre-existing failures cluster around cross-frame
exception/finally propagation and scatter-with-optionals, exactly as the
workstream prompt described — they are **not** part of this work.

## Commit

(Filled in after commit.)
