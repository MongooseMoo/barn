# C2 iteration 6 — Build-verify `vm` after de-box; fix latent errors (coder report)

Branch: `perf/c2-value-unbox` (continued; HEAD before this iter `5dd4808`).
Commit (WIP): **`a093f46242cc04547163dec20808ae1c00f86c57`** —
`perf(c2-iter6): build-verify vm after de-box; fix latent errors` (3 files changed,
+3/-3).

Scope: `vm/` only. Did NOT touch scheduler/server (out of scope, later iterations) —
and `go build ./vm` did not require touching them.

---

## 1. Headline

`vm` compiled for the FIRST time against the migrated `types`/`bytecode`/`db`/`builtins`
stack. The latent de-box damage was **exactly the three nil-sentinel sites the 5b handoff
predicted, and nothing else.** iter3's mechanical migration of `vm` was complete: after
fixing the three `nil`→`types.None` sites, `go build ./vm` went green on the very next
pass — zero grep-invisible renamed-method casualties (no `gc.go`-style surprise), zero
residual concrete-type literals/asserts, zero `Value ==/!=` conversions required. The full
vm test suite passes, `-race` is clean, and the first real correctness signal for the whole
refactor is GREEN.

---

## 2. Latent errors found + fixes (validates iter3's WIP migration)

`go build ./vm` (first run) reported exactly three errors, all the `nil`-as-`types.Value`
sentinel class:

| # | File:line | Old | New | What it is |
|---|---|---|---|---|
| 1 | `vm/environment.go:79` | `return nil, false` | `return types.None, false` | `Environment.Get` "variable not found" return (returns `(types.Value, bool)`) |
| 2 | `vm/registry.go:130` | `ctx.ThisValue = nil` | `ctx.ThisValue = types.None` | reset effective `this` to absence when seeding a fresh login/eval context |
| 3 | `vm/traceback.go:90` | `ThisValue: nil` | `ThisValue: types.None` | `ActivationFrame.ThisValue` in a synthesized traceback frame (no value-typed `this`) |

All three preserve the old interface-`nil`-means-absence semantics: each consumer of these
slots already tests via `IsNone()` (e.g. `op_verb.go:365` `!vm.Context.ThisValue.IsNone()`),
so `types.None` is the correct sentinel and the zero-value trap (`Value{}` == integer-0, NOT
None) is avoided.

**No second build pass surfaced any further errors** — i.e. no renamed-method casualties
(`MapValue.Set`→`MapSet`, list-vs-map `Get`, str `Append`→`StrAppend`), no leftover
`types.XValue{}` literals, no `.(types.X)` asserts. iter3 had already converted all of those
(its report: 25 type-switches, 150 asserts, 64 case arms, 103+21+5 literals, 14 waif refs),
and this build confirms that work compiles cleanly against the now-migrated dependencies.

---

## 3. `==`/`!=` → `.Equal()` audit (correctness-critical)

**ZERO `Value == Value` / `Value != Value` comparisons exist in `vm`. ZERO `.Equal()`
conversions were required.** This confirms the iter3 §3 finding and the iter5b builtins
finding. I re-audited the full vm surface independently (not trusting the prior report):

- All MOO value equality already routes through the `.Equal()` **method**:
  `operators.go` `equal`/`notEqual` (lines 341/355 `left.Equal(right)`), `op_compare.go`
  `executeEq`/`executeNe` (lines 30/57 `a.Equal(b)`), the `in`/membership operator
  (`op_compare.go:142` `element.Equal(collection.Get(i))`, `:168` `pair[1].Equal(element)`),
  and waif dedup. Unchanged and correct.
- Every literal `==`/`!=` operates on a **non-Value** extracted scalar or a non-Value type,
  and correctly stays `==`/`!=`:
  - `types.ErrorCode`: `err != types.E_NONE`, `errCode == types.E_NONE` (the dominant pattern
    across `op_property.go`/`op_index.go`/`op_list.go`/`collection_helpers.go`).
  - `types.ObjID`: `collection_helpers.go:68` `val.Class() == waif.Class() && val.Owner() ==
    waif.Owner()` — verified `Class()`/`Owner()` return `ObjID` (`types/waif.go:37,40`), NOT
    `Value`, so `==` is the right identity check (preserves old class+owner waif identity).
  - `int64`/`float64`/`string`/`bool`: `value.Int() != 0`, `len(value.Str()) != 1`,
    `rightInt == 0`, `rightFloat == 0.0`, etc.
  - `interface{}` boxed numerics: `operators.go` `leftNum == nil || rightNum == nil` (7 sites)
    — these are `interface{}`-boxed `int64`/`float64` from `toNumeric`, NOT Values; iter3
    deliberately left them, and that remains correct.
  - Go pointer/error/interface nil: `e.parent != nil`, `ctx.Task != nil`, `err != nil`.

The dangerous unsafe-pointer-`==` class (Go `==` comparing the struct's `unsafe.Pointer`
bits for heap types) **does not occur anywhere in vm**.

---

## 4. Semantic landmines — confirmed intact (not re-touched)

These were established by iter3 and remain in the compiled, tested code:
- **ObjValue-matches-anonymous**: `isObjLike(v) = Type()==TYPE_OBJ || Type()==TYPE_ANON`
  (`collection_helpers.go`) at every former `ObjValue` assertion. Intact.
- **C1 `op_arith.go` numeric-first reorder**: present and unchanged; arithmetic dispatches on
  `.Type()` tags. Intact.
- **Unbound `Type()` lie**: `OP_GET_VAR` gates an unbound local with `E_VARNF`
  (`vm.go` detection via `IsUnbound()`) before it can reach `toNumeric`. Intact.

---

## 5. Gate output (raw, at commit `a093f46`)

```
=== go build ./vm ===
build_exit=0
=== go vet ./vm ===
vm\stack.go:49:15: method ReadByte() byte should have signature ReadByte() (byte, error)
vet_exit=1
=== go test ./vm -count=1 ===
ok  	barn/vm	1.069s
=== go test ./vm -race -count=1 ===
ok  	barn/vm	1.200s
```

- `go build ./vm` → GREEN.
- `go test ./vm -count=1` → `ok barn/vm` (full suite, no skips/weakening; no test touched).
- `go test ./vm -race -count=1` → `ok barn/vm` — clean (matters because the struct holds an
  `unsafe.Pointer`; no data race, no GC-visibility issue surfaced).
- `go vet ./vm` → **one warning, pre-existing and de-box-unrelated** (see §6).

### No test failures

The full vm suite passed on the first run after the build went green — there was no test
failure to root-cause. A de-box behavior regression (wrong tag dispatch, None mishandled,
equality via `==`) is precisely what these tests would have caught; they all pass, which is
the positive correctness signal this iteration exists to produce.

---

## 6. The one `go vet` warning — pre-existing, NOT a de-box error

```
vm\stack.go:49:15: method ReadByte() byte should have signature ReadByte() (byte, error)
```

This is vet's `stdmethods` check: a method named `ReadByte` is expected to match
`io.ByteReader`'s `ReadByte() (byte, error)`. Barn's `VM.ReadByte` reads from the in-memory
instruction stream and returns a bare `byte` by design.

**Evidence it is pre-existing and unrelated to the de-box:** the method signature is
byte-identical on `master` —
```
$ git show master:vm/stack.go | grep -n ReadByte
48:// ReadByte reads a byte from the current instruction stream
49:func (vm *VM) ReadByte() byte {
```
The de-box changed neither this method nor its signature; vet would emit the same warning on
the pre-refactor baseline. It is a naming-convention nit, not an `unsafe.Pointer` misuse (vet's
unsafe/GC-relevant checks are clean). Renaming `ReadByte` would ripple through the bytecode
read hot path and is out of scope for de-box verification, so I left it untouched and flagged
it here rather than conflating it with the migration. If a fully-clean `go vet` is desired,
that rename is a separate, independently-reviewable change.

---

## 7. Remaining `go build ./...` red list

```
# barn/scheduler
scheduler\waif_lifecycle.go:11:26: undefined: types.WaifValue
scheduler\eval.go:131:15: result.Val (... types.Value) is not an interface
scheduler\eval.go:132:14: undefined: types.FloatValue
scheduler\eval.go:134:14: undefined: types.IntValue
scheduler\eval.go:183:21: invalid operation: t.WakeValue != nil (... types.Value and untyped nil)
scheduler\eval.go:185:18: cannot use nil as ... types.Value value in assignment
... (more in scheduler)
```

Root red package: **`barn/scheduler`** only. `server` and `cmd/...` are blocked *downstream*
on `scheduler` (they import it), so they do not yet build — but they have no independent root
errors reported. This is exactly the expected out-of-scope remainder for the next iteration(s):
the de-box is now complete and GREEN through `types → bytecode → db/format → db/store →
builtins → vm`; `scheduler` (then `server`/`task`/`cmd`) is the remaining surface, carrying the
same mechanical classes (`undefined: types.XValue`, `nil`-as-Value, `.Val is not an interface`,
`WaifValue` refs).

---

## 8. Commit

`a093f46242cc04547163dec20808ae1c00f86c57` on `perf/c2-value-unbox`
(`git add vm/`; 3 files, +3/-3).

---

## 9. Handoff to the scheduler-verify iteration

- Apply the same mechanical table (iter1/iter3): asserts→`Type()`+accessors,
  `.(type)`→`Type()` switches, `ObjValue`→obj-or-anon (`Type()==OBJ||ANON`),
  `UnboundValue{}`→`types.Unbound` (`IsUnbound()`), map `.Get/.Set/.Delete`→
  `MapGet/MapSet/MapDelete`, str `.Append`→`StrAppend`, `nil`-as-Value→`types.None`
  (init `var x types.Value` sentinels to `types.None` explicitly — zero-value is int-0).
- Known first errors in `scheduler`: `eval.go` (`result.Val` type-switch, `WakeValue != nil`,
  `nil` assignment, `FloatValue`/`IntValue` literals) and `waif_lifecycle.go`
  (`types.WaifValue` refs → `TYPE_WAIF`/`Value`).
- Watch for grep-invisible renamed-method casualties (trust `go build`, not just grep) and
  re-audit for any genuine `Value == Value` (none existed in types/bytecode/builtins/vm, but
  audit each new surface).
- `vm`'s pre-existing `ReadByte` vet nit (§6) is unrelated and can be addressed separately if
  a clean repo-wide `go vet` is wanted.
