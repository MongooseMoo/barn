# Bytecode package audit checkpoint (2026-08-10)

## Scope and state

- Audit target is only `bytecode`: 10 Go files, exactly 3,004 code lines by `cloc` (425 blank, 571 comment).
- Checkout is on `agent/issue-27-concurrent-list-append`; `bytecode` has no branch-local diff from `origin/master`.
- Package baseline passes: `go test . -count=1 -cover` (53.8%), `go vet .`, `go test -race . -count=1`, Staticcheck, and golangci-lint.
- Existing issue #87 already owns the broad move of semantic lowering from `bytecode` into `compiler`; do not duplicate it with a generic cleanup ticket.

## Current verified observations

- `instructionOperandCount` says compact `OP_CALL_VERB` has one operand, but the compiler and VM contract encode two: `<name index:byte> <argc:byte>`. A compiled-source red test showed disassembly consuming the real `OP_RETURN` as an operand. A fork red test showed the first `TRY_FINALLY_WIDE` handler target staying at parent IP 27 instead of rebasing to child IP 20. No duplicate was found; issue #172 is open: https://github.com/MongooseMoo/barn/issues/172
- Compiler-generated locals share the user-variable namespace. Focused compiler-plus-VM red tests returned `0`, `["a" -> 1]`, and `2` instead of `99` for legal user variables named `__loop_result_1__`, `__maplit_1__`, and `__nested_val`. A temporary three-row conformance file is prepared for the required managed WSL Toast check; it must be removed after the oracle run.
- Property-backed index/range assignment reads the target property, mutates a temporary, then recompiles the object and dynamic property-name expressions for the write-back. Managed Toast/Barn differential verified the resulting wrong-object write; filed as #180.
- Managed semantic differential completed: 7/7 passed on WSL Toast (six probes plus admission); Barn passed admission and failed all six semantic probes. Issues opened:
  - #180 target/RHS order and duplicate property-target evaluation: https://github.com/MongooseMoo/barn/issues/180
  - #181 deep nested range assignment rejection: https://github.com/MongooseMoo/barn/issues/181
  - #182 case-sensitive builtin and loop-target resolution: https://github.com/MongooseMoo/barn/issues/182
- A focused compiler-plus-VM red test placed an exception variable at local slot 255 after 255 chained assignments. Compilation accepted 256 locals, but the one-based `byte(idx+1)` operand wrapped to zero (the no-variable sentinel); execution raised `E_VARNF` instead of returning captured `E_DIV`.
- Managed WSL Toast accepted the 256-local source and returned `E_DIV` (2/2 including admission); issue #183 is open: https://github.com/MongooseMoo/barn/issues/183
- The `x = x + expr` statement peephole emits `OP_STRING_APPEND`, whose duplicated add semantics omit float overflow rejection and `PROMOTE_NUMBERS`. Managed proof:
  - stock Toast raised `E_FLOAT`; Barn returned `Inf`;
  - promoted Toast returned `1.5`; promoted Barn raised `E_TYPE`.
  - issue #184 is open: https://github.com/MongooseMoo/barn/issues/184
- `Scope`, `scopes`, `beginScope`, `endScope`, and `Scope.Parent` do not participate in resolution (`resolveVariable` uses the package-global compiler map). This is dead bookkeeping, but broad ownership cleanup may be subsumed by #87.

## Cleanup state

- Every temporary Go/YAML test and all generated managed logs/databases were removed.
- The execution policy rejected exact validated deletion of `C:\Users\Q\code\barn\.tmp\bytecode-audit-20260810\barn.exe` (22,183,936 bytes), first as part of the recursively validated 22-child directory and then as a single exact file. Stop; do not retry with another shell or stronger primitive. Only that binary and empty report directories remain.
- The mandated audit note remains uncommitted and must not be staged.

## Final verification

- `go test . -count=1 -cover`: pass, 53.8% statement coverage.
- `go vet .`: pass.
- `go test -race . -count=1`: pass.
- `cloc`: still 10 Go files and exactly 3,004 code lines.
- Staticcheck and golangci-lint passed earlier in the audit.

## Next action

- Report the seven opened issues, verified differentials, #87 overlap decision, clean package baseline, and the one policy-blocked disposable binary. No package implementation change was requested or made.
