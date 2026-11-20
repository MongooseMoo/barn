# Barn refuses a program with more than 256 constants

## Goal
Make Barn compile and run programs whose constant count exceeds 256, as Toast
does. Found by the first scheduled extended "Jepsen checkpoint" run
(43426360560, 2026-11-13 12:34 UTC), which nobody was told about.

## State
Fixed on branch `fix/constant-pool-over-255`; the conformance test is on
branch `test/over-255-literals` of moo-conformance-tests. Both are local
commits: nothing is pushed and no PR exists.

- What happened: job "Checkpoint durability campaign" (131888678719), step
  "Run campaign", exit code 1. Log: `recovery-failure: final observation
  before checkpoint of generation 0 did not succeed within 30.0s` with
  `raw='{0, {"Line 1:  too many constants (max 255)"}}'`, repeated for every
  attempt. The harness's observation program has more constants in extended
  mode than Barn's compiler accepted.
- Where it was: `addConstant` in `compiler/lower_emit.go` refused pool index
  256 and up, because `OP_PUSH` carries a one-byte index (`emitConstant`, the
  `OP_PUSH` case of `VM.Execute` and the `fkPush` fast path in `vm/vm.go`,
  `verifyInstruction` in `bytecode/verify.go`).
- What Toast does (stock ToastStunt, `src/code_gen.cc`): `add_literal` grows
  the literal table by doubling with no upper bound, and `ref_size` makes a
  literal reference 1 byte up to 256 entries, 2 bytes up to 65536 and 4 bytes
  above. Toast has no such compile error.
- The fix: `OP_PUSH_WIDE [index:short]`. `emitConstant` emits it for index
  256 and up and keeps one-byte `OP_PUSH` for 0..255. The VM, the verifier,
  `instructionOperandCount`, `InstructionStackEffect` and the native-import
  layout (`compiler/native_barn_layout.go`) each gained the one case.
- Conformance: `language/literal_count_boundary.yaml` (four rows: a verb with
  256, 257 and 600 distinct string and float literals, and an `eval()` program
  with 600). Stock WSL Toast: 5 passed with `capability_admission`. Barn
  before the fix: the 257, 600 and eval rows failed with the constants error
  and the 256 row passed. Barn after: all pass, along with
  `parser_depth_boundary`, `parser_resource_limits` and `tick_accounting`
  (75 passed).
- Fixture for all three runs: the conformance worktree's
  `src/moo_conformance/_db/Test.db`, 2035 bytes, SHA-256
  `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
- Commands, from the Barn worktree inside WSL Debian, with
  `CONFORMANCE_SUITE` naming the conformance worktree:
  `make conformance-toast K=distinct_literals` and
  `make conformance K=distinct_literals`.

## Not fixed
- A program with more than 65536 constants still fails to compile, now with
  `too many constants (max 65536)`. Every wide constant operand in Barn is a
  16-bit index (`OP_PUSH_WIDE`, the `*_WIDE` and keyed property opcodes,
  `OP_CALL_VERB_WIDE`), while Toast goes to four bytes. No producer of such a
  program is known.
- `OP_PUSH_WIDE` runs on the generic `Execute` path only, not the dispatch
  fast path, so a push of constant 256 and up leaves the fast loop for that
  one instruction. Programs within 256 constants are unaffected.
- `BenchmarkVM/list_iter_1M` in `vm/perf_bench_test.go` fails with
  `compile failed: [Line 1:  syntax error]` on the unchanged base too.

## The same run's other failures
- Job "Checkpoint stage schedule and negative controls" (131888678611) ran
  five seeds: two failed on this constants error, one was healthy, and two
  failed because Elle rejected an ordinary history with G2-item. That is a
  separate fault with its own branch and note
  (`agent-notes/fix-verb-walk-misses-chparent.md`).
- Unverified: that the campaign and the schedule pass once both fixes are on
  master. The next scheduled extended run is the check.

## Decisions and why
- Toast first: the conformance test passed on the managed WSL Toast oracle and
  failed on unchanged Barn before any Barn production change (CLAUDE.md).
- `OP_PUSH_WIDE` takes the number of the dead `OP_BREAK`. All 256 opcode
  bytes are assigned; `OP_MAPHASKEY_KEYED` took the dead `OP_RAISE` the same
  way. No compiler emitted `OP_BREAK` and the decoder rejected it, so no saved
  program can hold that byte as an opcode.
- One-byte `OP_PUSH` stays for index 0..255. A suspended task is restored by
  recompiling its source and comparing the result with what it ran (the
  fingerprint for a v2 frame, code and constants for a v1 frame, in
  `vm/persistence.go`), so every program that compiled before must compile to
  the same bytes.
- The limit moved to 65536 rather than away: removing it needs four-byte
  forms of every opcode that names a constant, which nothing asked for.
- No fast-path case for `OP_PUSH_WIDE`: the dispatch loop is the hot path of
  every program, and only programs that could not compile before would use
  the new case.

## Surprises
- The opcode byte space is full, so a new opcode has to replace a dead one.
- The first conformance draft failed on Toast with `E_TYPE` on every row,
  the 256 one included: the test built its expected floats with `i + 0.5`,
  which stock Toast (no `PROMOTE_NUMBERS`) rejects. `tofloat(i) + 0.5` fixed
  the draft; Barn was not run on the broken one.
- `BenchmarkVM` base against fix, two interleaved rounds of seven runs on a
  loaded workstation: allocations identical for every workload; times differ
  by less than the run-to-run spread, with the sign changing between rounds
  (`while_lt_1M` minimum 44.6 ms and 44.9 ms base, 44.9 ms and 46.0 ms fix;
  `prop_name_1M` minimum 119.2 ms and 119.9 ms base, 117.0 ms and 122.7 ms
  fix).

## Next step
Barn's CI runs the conformance suite at the head of its `main`
(`.github/workflows/ci.yml`, the `git ls-remote ... refs/heads/main` step), so
the Barn fix merges first and the conformance PR only after it; the other
order turns every Barn PR red. Then read the next scheduled extended "Jepsen
checkpoint" run.
