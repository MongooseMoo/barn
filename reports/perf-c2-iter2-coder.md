# C2 iteration 2 — Migrate `bytecode` package to the `Value` struct (coder report)

Branch: `perf/c2-value-unbox` (continued; HEAD was iter1 report commit `09ac982`).
Commit (WIP): **`cfe94c33e762d54251e5d15485ee2436752460ab`**
Scope: `bytecode/` package ONLY. Did not touch `vm/`, `builtins/`, `db/`, `trace/`.

---

## 1. Headline: the blast radius in `bytecode` is tiny

The scout's map predicted bytecode would be one of the smallest packages, and that
held. After reading every value site in the package, the entire migration was **6 edit
points across 2 files** — all mechanical, no semantic changes, no shims.

The big-ticket landmines for this campaign (keyHash `%T`, nil sentinel audit, on-disk
format) **do not live in bytecode** — they are in `types` (already done in iter1) and
`db`. Bytecode only *constructs* literal Values for the constant pool and *carries*
them in `Program.Constants []types.Value`; it never type-switches or compares Values.

## 2. Sites changed (by kind)

| Kind | Count | Locations |
|---|---|---|
| Raw composite literal `types.IntValue{Val: x}` → `types.NewInt(x)` | 4 | `compiler.go:818, 1544, 1555, 2666` |
| `nil` returned as a `types.Value` → `types.None` | 2 | `parser_literals.go:24, 28` (the `(types.Value, error)` return) |
| **Total** | **6** | 2 files |

Notes:
- The 4 `IntValue{}` literals were: loop-index seed (`i+1`), `++`/`--` step (`1` / `-1`),
  and a stack-reserve count. All are plain int constructors now.
- `parser_literals.go:41` also has a `return nil, ...`, but that function returns
  `([]types.ErrorCode, error)` — a nil *slice*, not a Value. Left unchanged (correct).
- All other `return nil` / `== nil` / `!= nil` hits in the package are on `error`,
  pointer (AST nodes, `oldest *cacheEntry`), or slice values — none on `types.Value`.
- The `.(type)` switches in `compiler.go` (lines 451, 782, 1984, 2822) switch on
  **parser AST node** types (`node.(type)`, `target.(type)`, `expr.(type)`,
  `stmt.(type)`), not on `types.Value`. Untouched, correct.

## 3. Equality / `==` audit (the correctness-critical point)

**No `==` → `.Equal()` change was required in `bytecode`, and that is a verified finding,
not an omission.**

The constant-pool deduplication in `addConstant` (`compiler.go:251-271`) already keys on
the **string form**, not on Go `==`:

```go
func (c *Compiler) addConstant(v types.Value) int {
	key := v.String()                 // map[string]int dedup key
	if idx, ok := c.constants[key]; ok {
		return idx
	}
	...
	c.program.Constants = append(c.program.Constants, v)
	c.constants[key] = idx
	return idx
}
```

So the pool never compared two `Value`s with `==`; it compared `v.String()` results via a
`map[string]int`. This is identical behavior before and after the de-box (the interface era
used the same `v.String()` key). Because the struct now contains an `unsafe.Pointer`, a
naive `==` dedup *would* have been wrong for heap types — but this code never did that.

I also grep-swept the whole package for any `Value == Value` / `== types.` comparison and
found none. (Every `==`/`!=` in the package is on errors, AST-node pointers, slices, ints,
or string fields.)

Caveat worth flagging to iter3+: dedup-by-`String()` collapses any two values whose
`String()` renders identically. For the literals bytecode emits (ints, floats, strings,
objs, errs, bools) this matches the long-standing behavior and is unchanged by this
iteration — I did **not** alter it. If a future iteration wants exact type-aware dedup it
would switch the key to include `v.Type()` (like `keyHash` did in iter1), but that is a
behavior change and out of scope here.

## 4. Gate output (raw)

```
=== go build ./bytecode ===
BUILD_OK
=== go vet ./bytecode ===
VET_OK
=== go test ./bytecode -count=1 ===
ok  	barn/bytecode	0.375s
```

(`go build` and `go vet` produced no diagnostics; `BUILD_OK`/`VET_OK` are the echo
markers confirming each exited 0.)

## 5. Downstream still red — confirmation it's downstream, not `bytecode`

`go build ./...` (head):

```
# barn/trace
trace\tracer.go:88:15: invalid operation: result != nil (mismatched types types.Value and untyped nil)
# barn/db/store
db\store\store_core.go:26:45: undefined: types.WaifValue
db\store\store_core.go:295:17: invalid operation: prop.value (variable of struct type types.Value) is not an interface
db\store\store_lifecycle.go:404:63: undefined: types.WaifValue
db\store\store_metrics.go:48:13: undefined: types.StrValue
db\store\store_metrics.go:50:13: undefined: types.FloatValue
...
```

**No `# barn/bytecode` section appears** in the error output — the package compiles
clean within the full-repo build. The remaining errors are exactly the predicted classes
(undefined `types.XValue`, nil-as-Value, struct-is-not-an-interface) in `trace`,
`db/store`, `vm`, `builtins` — the later iterations.

## 6. Commit

`cfe94c33e762d54251e5d15485ee2436752460ab` on `perf/c2-value-unbox`
(`git add bytecode/` only; message notes the `==` finding).

## 7. Handoff to iteration 3 (`vm` package)

- **bytecode is fully done and green.** `Program.Constants []types.Value` is now a slice of
  the struct; vm reads it via `OP_PUSH` indexing — no change needed there beyond the vm's
  own assert/literal migration.
- **Constant pool is string-keyed, not `==`-keyed.** When you migrate vm, you can rely on
  the constant pool already holding well-formed struct Values built via constructors.
- **Surprises / nothing alarming:** the only non-obvious thing was confirming the 4 raw
  `IntValue{}` literals (the scout flagged 136 such literals repo-wide, "concentrated on
  hot VM paths"). vm/iter3 is where the bulk of `IntValue{}`/`FloatValue{}` literal
  rewrites and the `v.(types.UnboundValue)` → `IsUnbound()` substitution (scout cited
  `vm/vm.go:436`) will land. Use the iter1 substitution table (report §8) — list
  `Get/Set/Append` stay natural, map uses `MapGet/MapSet/MapDelete`, str uses
  `StrAppend`/`Str()`, unified `Len()`, and `nil`-as-Value → `types.None` + `IsNone()`.
- **Watch for `== nil`/`!= nil` on Value in vm** (e.g. the `trace` failure above is exactly
  this pattern: `result != nil` where `result` is a `types.Value`). Those become
  `!result.IsNone()` / `result.IsNone()`. Enumerate them; do not blind-regex (most `== nil`
  in vm are pointer/error checks).
