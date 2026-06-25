# VM Package Review

## Architecture Summary

The bytecode VM is a single-goroutine, single-stack interpreter (`vm.VM`). Each task runs one VM; forked tasks get a new VM with a snapshot of parent locals. The execution model is frame-based: `StackFrame` holds the instruction pointer, local-variable array (indexed by `Program.VarNames`), exception-handler stack (`ExceptStack`), and saved context fields for frame-restore on return. Verb calls are _native_ — instead of a Go call, `executeCallVerb` pushes a new `StackFrame` and the main loop continues dispatching into it. This keeps the Go call stack shallow.

`kernel.TaskContext` is the dependency-injection vehicle for every builtin and the VM itself. It carries player, programmer, tick counters, and references to the Store and Builtins registry. Three fields (`Task`, `CallerVM`, `Registry`) are typed `interface{}` to break import cycles (`vm -> kernel -> task`, `vm -> kernel -> vm`). Every consumer performs a type assertion, meaning silent failure when the wrong type is stored.

`environment.go` and `operators.go` are vestigial from an earlier tree-walking interpreter. Neither is called by the bytecode VM.

---

## ARCHITECTURAL FINDINGS

### ARCH-1 — interface{} coupling in kernel.TaskContext (HIGH)

`Task`, `CallerVM`, and `Registry` are stored as `interface{}`. There are approximately ten cast sites (`if t, ok := vm.Context.Task.(*task.Task); ok { ... }`). The silent-failure branch causes entire code blocks to silently do nothing on type mismatch. No compiler enforcement. The real import cycle (`vm <-> kernel <-> task`) should be broken with a thin interface in `kernel` rather than erasing types.

Files: `kernel/context.go`, `vm/vm.go`, `vm/op_verb.go`, `vm/traceback.go`, `vm/registry.go`

### ARCH-2 — `environment.go` is dead code (MEDIUM)

`NewEnvironment()` and the `Environment` type are never referenced by the bytecode VM, which uses `frame.Locals[]` indexed by `Program.VarNames`. The file pre-populates `"player"` with `#1` (hardcoded) while the VM initialises all locals to `UnboundValue`. Vestigial from the tree-walking interpreter.

File: `vm/environment.go`

### ARCH-3 — `operators.go` is dead code, parallel to `op_*.go` (MEDIUM)

`add`, `subtract`, `multiply`, `divide`, `modulo`, `power`, `equal`, `notEqual`, `lessThan`, `greaterThan`, `inOp`, `compare`, `unaryMinus`, `unaryNot`, `bitwiseNot`, `bitwiseAnd`, `bitwiseOr`, `bitwiseXor`, `leftShift`, `rightShift` — none called outside the file. The VM dispatches to `op_arith.go`, `op_compare.go`, `op_bitwise.go`. Both sets implement the same semantics independently; the `in`-for-maps bug (BUG-1) is already present in both, proving they have drifted.

File: `vm/operators.go`

### ARCH-4 — IsEvalFrame scan is O(depth) per every verb call and pass() (MEDIUM)

Every `executeCallVerb` and `executePass` scans all frames linearly to decide whether to propagate command-parsing variables (`argstr`, `dobj`, etc.). For N nested verb calls inside `eval()` this is O(N^2) total across the call chain.

File: `vm/op_verb.go:207`, `vm/op_verb.go:415`

### ARCH-5 — ReadByte/ReadShort panic on truncated bytecode (MEDIUM)

Neither `ReadByte` nor `ReadShort` bounds-checks `frame.IP` against `len(frame.Program.Code)`. Malformed or truncated bytecode causes an index-out-of-range panic that escapes `executeLoop`; the panic is not converted to `E_EXEC`. `go vet` already flags `ReadByte` for its wrong return signature.

File: `vm/stack.go:49-63`

### ARCH-6 — Frame limit checked after full frame construction (LOW efficiency)

`checkFrameLimit` is called after the verb has been looked up, the bytecode compiled, and the new `StackFrame` heap-allocated. If the limit fires, all that work is discarded. The check should move before the lookup.

File: `vm/op_verb.go:263`

### ARCH-7 — Tick accounting split across two counters (LOW)

`VM.Ticks` counts up; `Context.TicksRemaining` counts down. `syncContextTicks()` is called per tick to keep them in sync. A builtin that writes `TicksRemaining` directly can cause the VM's `Ticks >= TickLimit` check to behave unexpectedly. The `eval()` fallback (when `CallerVM` assertion fails) creates a new VM with a hardcoded `TickLimit=30000`, ignoring the calling task's remaining budget.

File: `vm/vm.go:155-164`, `vm/registry.go:68`

### ARCH-8 — `VM.FP` field is never read (LOW)

`FP` is written to `0` in `Run()` and `PrepareVerbFrame()` and never read anywhere. Dead field.

File: `vm/vm.go:20`

---

## CONFIRMED BUGS

### BUG-1 — `in` operator on maps searches VALUES, not KEYS (CRITICAL)

Both `executeIn` (op_compare.go:163-177) and `inOp` (operators.go:413-452) iterate `pair[1]` (the value slot) when the container is a `MapValue`. ToastStunt semantics: `x in map` returns the 1-based position of `x` among the map's **keys** in canonical sorted order, or 0 if not found.

Tests: `TestReview_MapInChecksValuesNotKeys`, `TestReview_MapInValueFoundAsKey_ReturnsZero`

Failing output:
```
--- FAIL: TestReview_MapInChecksValuesNotKeys
    "a" in ["a" -> 1] = 0, want 1 (key lookup)
--- FAIL: TestReview_MapInValueFoundAsKey_ReturnsZero
    1 in ["a" -> 1] = 1, want 0; current impl finds the value instead of a key
```

### BUG-2 — WaifValue.properties map is shared across all struct copies (CRITICAL)

`WaifValue` is a Go struct (value type) whose `properties` field is a `map[string]Value` (reference type). Copying a `WaifValue` (assignment to a local variable, pass-by-value into a function, any struct copy) copies the map pointer, not the map contents. `SetProperty` mutates the shared map in place. Every copy of the same waif sees every mutation — full aliasing in violation of value-type semantics.

The comment in the code says "copy-on-write semantics" but the implementation is not copy-on-write.

Secondary: `op_property.go:249` discards the returned `WaifValue` (`_ = waif.SetProperty(...)`). This is currently harmless because mutations go through the shared map. Once the type is fixed to true copy-on-write the discard will silently break all property writes on locally-held waifs.

Tests: `TestReview_WaifPropertyMutationAliasesAcrossStructCopies`, `TestReview_WaifSetPropertyMutatesOriginalNotCopy`

Failing output:
```
--- FAIL: TestReview_WaifPropertyMutationAliasesAcrossStructCopies
    localB.foo = 99 after mutating localA.foo; WaifValue.properties map is shared across struct copies
--- FAIL: TestReview_WaifSetPropertyMutatesOriginalNotCopy
    WaifValue.SetProperty mutated the original struct via shared map; copy-on-write semantics are broken (types/waif.go:68-75)
```

### BUG-3 — containsWaif false-positive for same-class, same-owner distinct instances (HIGH)

The circular-reference guard in `setWaifProp` calls `containsWaif` which tests `v.Class() == waif.Class() && v.Owner() == waif.Owner()`. Two distinct waif instances sharing the same class and owner are incorrectly treated as identical, causing `E_RECMOVE` on a legitimate property assignment.

Test: `TestReview_ContainsWaifFalsePositive_SameClassOwnerDistinctInstances`

Failing output:
```
--- FAIL: TestReview_ContainsWaifFalsePositive_SameClassOwnerDistinctInstances
    containsWaif(waifB, waifA) = true for distinct instances with same class+owner; should compare by instance identity
```

---

## SUSPECTED BUGS (no oracle confirmation)

### SUSP-1 — String `in` may be wrong case-sensitivity (LOW)

`op_compare.go:154-158` lowercases both operands before `strings.Index`. If ToastStunt `in` on strings is case-sensitive, results diverge for mixed-case needles.

### SUSP-2 — String range/index uses byte offsets; iteration uses rune offsets (MEDIUM)

`executeIndex` (string): `coll.Value()[indexInt.Val-1]` — byte indexing.
`executeIterPrep` (string): `runes := []rune(s)` — rune iteration.
For multi-byte UTF-8 these disagree. MOO traditionally treats strings as byte arrays; if so, the rune-based iteration path is the bug.

### SUSP-3 — Waif GC dedup is O(n^2) (LOW)

`waif_gc.go:7-12`: `collectWaifsForGC` checks every existing entry before appending. O(K^2) for K unique waifs per frame.

---

## Test file created

`vm/review_bugs_test.go` — 5 red tests (BUG-1 x2, BUG-2 x2, BUG-3 x1), 3 passing happy-path guards.
