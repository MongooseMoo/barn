# Frontend Review: types/ + parser/ + bytecode/

Analyst review of the language frontend packages. All tests pass and `go vet` is clean before this review.

---

## Architecture Summary

### types/

A standard MOO value hierarchy. `Value` interface has four methods: `Type()`, `String()`, `Equal()`, `Truthy()`. Concrete types: `IntValue`, `FloatValue`, `StrValue`, `ObjValue`, `ErrValue`, `ListValue`, `MapValue`, `WaifValue`, `BoolValue`, `UnboundValue`.

`ListValue` wraps a `MooList` interface backed by `sliceList`, which implements a watermark-based amortized-O(1) COW append. Sound for single-threaded use. `MapValue` wraps a `goMap` that maintains insertion order via a parallel `[]string` slice of key hashes. `StrValue` has a parallel `val string` / `data []byte` representation with the same watermark trick for string appends.

`WaifValue` is a struct value type containing a reference-type `properties map[string]Value`. This is structurally incompatible with COW semantics and is the most critical flaw in the package.

### parser/

Standard recursive-descent parser with a one-token lookahead. Lexer operates byte-by-byte (MOO is ASCII-oriented). Parser uses a Pratt-style precedence table. Statement parsing dispatches on keyword tokens. Scatter assignment detection uses a two-token lookahead heuristic. Error nodes carry line numbers for Toast-compatible error formatting.

### bytecode/

Single-pass compiler: AST → `Program` (flat `[]byte` code + constant pool + variable table). 154-opcode set with immediate-integer encoding for small integers (-10..143), 16-bit jump/offset operands. Short-circuit `&&`/`||` via `OP_AND`/`OP_OR` conditional-jump opcodes. Loops use forward-patched break jumps and either backward `OP_LOOP` (while) or forward-patched `OP_JUMP` (for-continue). Content-addressed LRU verb-program cache keyed by FNV-1a hash of source lines.

---

## ARCHITECTURAL FINDINGS

**A1 [CRITICAL] WaifValue: value-type struct with reference-type map field**
`WaifValue` holds `properties map[string]Value` in a struct. Every struct copy (function argument, variable assignment) shares the same backing map. `SetProperty` uses a value receiver and writes directly into the shared map. There is no COW: every alias is silently corrupted. This is a fundamental design error. Fix requires either making `WaifValue` a pointer type or using an immutable persistent map like the other collection types.

**A2 [HIGH] ObjValue.Equal ignores the anonymous flag**
`Equal()` checks `o.id == otherObj.id` but never checks `anonymous`. This means `Equal()` disagrees with `Type()`: `NewObj(5)` and `NewAnon(5)` have different types but are reported equal. Any code that relies on `Equal()` for identity checks (map key equality, list membership) will produce wrong results for anonymous objects.

**A3 [HIGH] looksLikeScatter heuristic has no backtracking**
`looksLikeScatter()` returns true whenever peek is `TOKEN_IDENTIFIER`, `TOKEN_QUESTION`, or `TOKEN_AT`. This correctly identifies `{x = ...} = list;` scatter assignments but also matches any list expression starting with an identifier at statement position. When misidentified as scatter, `parseScatterStatement` consumes the entire list, then fails "scatter must be followed by '='" with no recovery. There is no backtracking.

**A4 [MEDIUM] MapValue.String() sorts by canonical type order, discarding insertion order**
`String()` calls `sortMapPairsForOutput()` before rendering. The in-memory `Pairs()` preserves insertion order, but the string representation does not. If `String()` is used anywhere for round-trip comparison or display, the order will differ from the insertion order shown in MOO output. Consistency with Toast needs verification.

**A5 [MEDIUM] ValueBytes is O(n) per call for MapValue, O(1) for ListValue**
`ListValue` caches its byte size incrementally — append is O(1). `MapValue` walks all pairs on every `ValueBytes()` call. There is no caching. Large maps with frequent size queries pay O(n) each time.

**A6 [MEDIUM] keyHash uses `fmt.Sprintf("%T:", v)` as prefix**
The Go type name is baked into every map key hash. If the `types` package is ever renamed or types are moved, all persisted/hashed keys silently break. No documentation or test pins this behaviour.

**A7 [LOW] UnboundValue.Type() returns TYPE_INT**
The internal "declared but unassigned" marker reports the wrong type. Any code that calls `Type()` without also checking for `UnboundValue` will misclassify an unbound variable slot as an integer.

**A8 [LOW] OP_BREAK and OP_CONTINUE are declared but dead**
Both opcodes exist in the opcode table and name map, but the compiler comments say "never emitted" and the actual code paths use OP_JUMP patching. Dead entries inflate the table and could confuse disassemblers or future implementors.

**A9 [LOW] Compiler break/continue label detection relies on compiler-side heuristic to compensate for parser omission**
The parser never sets `BreakStmt.Label`; the compiler's `compileBreak` compensates by checking if the `Value` expression is an identifier matching a loop name. This works for the common case but means `break nonexistent_label;` silently compiles as a break-with-value expression rather than producing the "Invalid loop name" error that `continue` correctly raises.

---

## CONFIRMED BUGS

All seven tests are red. Test files: `types/review_test.go`, `parser/review_test.go`.

---

### BUG-1 [HIGH] ObjValue.Equal ignores anonymous flag

**Test:** `TestReview_ObjEqualIgnoresAnonFlag` in `types/review_test.go`

**Root cause:** `types/obj.go` line 58–63 — `Equal()` checks only `o.id == otherObj.id`.

**Red output:**
```
=== RUN   TestReview_ObjEqualIgnoresAnonFlag
    review_test.go:24: BUG: NewObj(5).Equal(NewAnon(5)) returned true — Equal ignores the anonymous flag; regular and anonymous objects with the same ID must not be equal
--- FAIL: TestReview_ObjEqualIgnoresAnonFlag (0.00s)
FAIL	barn/types	0.316s
```

---

### BUG-2 [CRITICAL] WaifValue.SetProperty mutates shared map (aliasing)

**Test:** `TestReview_WaifSetPropertyMutatesOriginal` in `types/review_test.go`

**Root cause:** `types/waif.go` line 68–75 — value receiver + direct map mutation.

**Red output:**
```
=== RUN   TestReview_WaifSetPropertyMutatesOriginal
    review_test.go:41: BUG: SetProperty on a WaifValue copy mutated the original's property map — the map is shared between struct copies, not COW
--- FAIL: TestReview_WaifSetPropertyMutatesOriginal (0.00s)
FAIL	barn/types	0.316s
```

---

### BUG-3 [HIGH] WaifValue.Equal uses structural comparison, not reference identity

**Test:** `TestReview_WaifEqualUsesDeepequalNotIdentity` in `types/review_test.go`

**Root cause:** `types/waif.go` line 36–43 — `equalMaps()` does deep value comparison.

**Red output:**
```
=== RUN   TestReview_WaifEqualUsesDeepequalNotIdentity
    review_test.go:55: BUG: two independently created WaifValues with same class/owner compare Equal — waif equality must use reference identity, not structural comparison
--- FAIL: TestReview_WaifEqualUsesDeepequalNotIdentity (0.00s)
FAIL	barn/types	0.316s
```

---

### BUG-4 [MEDIUM] E_INTRPT missing from parser error-name table

**Test:** `TestReview_EIntrptLiteralRejected` in `parser/review_test.go`

**Root cause:** `parser/parser_error.go` — `errorNames` map has 18 entries (E_NONE=0 through E_EXEC=17) but omits E_INTRPT=18. `isErrorName("E_INTRPT")` returns false, so the lexer produces `TOKEN_ERROR_LIT` but `parseLiteralExpr` rejects it.

**Red output:**
```
=== RUN   TestReview_EIntrptLiteralRejected
    review_test.go:20: BUG: E_INTRPT is a valid MOO error literal but the parser rejected it: syntax error
--- FAIL: TestReview_EIntrptLiteralRejected (0.00s)
FAIL	barn/parser	0.275s
```

---

### BUG-5 [HIGH] looksLikeScatter false-positive: {x, y}; fails to parse

**Test:** `TestReview_ListExprAsStatementMistakenForScatter` in `parser/review_test.go`

**Root cause:** `parser/parser_stmt.go` `looksLikeScatter()` — no backtracking after misidentification.

**Red output:**
```
=== RUN   TestReview_ListExprAsStatementMistakenForScatter
    review_test.go:35: BUG: {x, y}; is a valid list expression statement but the parser rejected it: syntax error
--- FAIL: TestReview_ListExprAsStatementMistakenForScatter (0.00s)
FAIL	barn/parser	0.275s
```

---

### BUG-6 [MEDIUM] UnparseProgram ForStmt with index variable emits garbage

**Test:** `TestReview_UnparseForWithIndexVar` in `parser/review_test.go`

**Root cause:** `parser/unparse.go` `unparseStmt` ForStmt case — when `s.Index != ""`, emits `value in [index..len(body)]` using the body statement count as the range end, instead of `label value, index in (container)`.

**Red output:**
```
=== RUN   TestReview_UnparseForWithIndexVar
    review_test.go:59: BUG: UnparseProgram for for-with-index-variable produced wrong output:
        for L x in [k..1]
          return x;
        endfor
        
        Expected output containing 'x, k in (mylist)'
--- FAIL: TestReview_UnparseForWithIndexVar (0.00s)
FAIL	barn/parser	0.275s
```

---

### BUG-7 [MEDIUM] BreakStmt.Label is never set; break label ends up in Value field

**Test:** `TestReview_BreakLabelAsIdentExpr` in `parser/review_test.go`

**Root cause:** `parser/parser_stmt.go` `parseBreakStatement` — `label` variable is declared but never assigned; any identifier after `break` is parsed as a value expression and stored in `BreakStmt.Value`. `parseContinueStatement` correctly populates `ContinueStmt.Label`.

**Red output:**
```
=== RUN   TestReview_BreakLabelAsIdentExpr
    review_test.go:107: BUG: BreakStmt.Label="" (empty), but ContinueStmt.Label="myloop" — break puts the loop name in Value instead of Label
--- FAIL: TestReview_BreakLabelAsIdentExpr (0.00s)
FAIL	barn/parser	0.275s
```

---

## SUSPECTED (not confirmed with red test)

**S1 [MEDIUM] Float formatting may diverge from Toast for edge cases**
`FloatValue.String()` uses `strconv.FormatFloat(f.Val, 'g', 15, 64)`. The comment claims this matches Toast's `%.15g`. Could not confirm with oracle (binary not present). If Toast uses a different precision or format for special cases (very small/large numbers), `tostr(x)` output will diverge.

**S2 [LOW] isLetter on multi-byte UTF-8 input**
`isLetter(ch byte)` calls `unicode.IsLetter(rune(ch))`. For bytes 0x80–0xFF cast directly to rune, some Latin-1 Supplement code points are classified as letters. This means malformed UTF-8 input could produce unexpected identifier tokens. MOO is ASCII-only in practice, so this is low risk.

**S3 [LOW] addConstant deduplication via String() can alias distinct values**
`WaifValue.String()` returns `"<waif #N>"`. Two waifs of the same class would hash to the same constant-pool slot. The compiler would emit the same waif constant for both. Waifs are not typically used as compile-time constants, but the structural flaw exists.
