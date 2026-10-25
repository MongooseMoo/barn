# Deletion exploration — 2026-10-25

## Goal

Find opportunities to remove at least 5,000 lines of code or equivalent complexity without removing functionality or slowing Barn. Q requested extensive notes and multiple exploratory agents. This is exploration, not implementation authorization. No code, tests, spec, issues, or PRs are to be changed or published.

## State

Baseline: master at 796df225. `git status --short --untracked-files=no` returned no output before exploration; many pre-existing untracked files are present and must be preserved. Latest reflog pull is 704a1b20 → 796df225.

Raw latest-pull summary:

```text
136 files changed, 17939 insertions(+), 189 deletions(-)
docs/notes/other added 3795 removed 4 files 31
tests added 6143 removed 6 files 45
Go production added 612 removed 174 files 37
JSON evidence/fixtures added 2523 removed 0 files 6
tools/experiments code added 4866 removed 5 files 17
```

Classification uses filename suffix/path: `_test.go`, `scripts/test_*`, `tests/`; then `.go`; then `.py/.cc/.ps1/.sh`; then JSON; then other. It is an accounting summary, not a claim about semantic complexity. Go runtime net growth is 438 lines in this range. The user's recollected 82 deletions differs from this latest reflog range's 189; no claim is made that this is exactly their displayed range.

## Decisions and why

- Smallest requested deliverable: source-backed candidate inventory and notes; zero implementation lines. Do not force 5,000 by removing tests, evidence, functionality, or moving code elsewhere.
- Count production, tooling, tests, generated code, and documentation separately. Estimate gross deletion, replacement, and net savings; do not add overlapping candidates.
- Use cleanup-refactor skill ownership questions, but Q's exploration request overrides its delete-first implementation workflow.
- Three agents inspect disjoint broad areas. Their reports are leads until parent reads source. Subtask notes: deletion-exploration-storage.md; deletion-exploration-execution.md; deletion-exploration-surfaces.md. Parent owns tooling, growth accounting, and synthesis.
- Existing conformance is necessary evidence for eventual changes, not proof of unchanged throughput, allocation, isolation, external APIs, or operational tooling. No checks have been run for this exploration.
- Read docs/elle-isolation.md and docs/workload-diagnostics.md. Any eventual relevant storage/runtime changes retain Elle's actual path-count/verdict requirements, and performance claims need valid comparable workloads. Local CI reruns are not planned.

## Surprises

- The latest 18K-line pull is mostly tests, tools, and retained evidence, rather than 18K runtime implementation lines.

## Next step

Exploration deliverable is ready. No implementation is authorized. If Q elects to proceed, start with deletion of the unused compiler scope representation and redundant identifier lookup, then the descriptor-backed admission subset. The 5,000-line aspiration remains unsubstantiated; do not turn it into a quota that permits lost functionality or slower execution.

## Parent investigation: tooling and growth

### Inventory method

Read-only Python scans used tracked paths from `git ls-files`, physical line counts, AST-normalized Python function bodies, and identical 15-significant-line Go windows. These locate leads; they do not prove behavioral equivalence or deadness. The Python function scan ignored names, retained argument names and bodies, and required at least six lines; it printed no duplicate groups among tracked non-test scripts. This rules out only exact structural duplicates under that narrow comparison.

Largest tracked Go production areas by physical lines (including blanks/comments): builtins 18,407; db 18,092; vm 7,822; engine 4,668; server 3,865; parser 3,419; types 3,194; compiler 3,037. This is why the agents cover those areas before smaller wrappers.

### T1: shared managed conformance command — modest tooling lead

`scripts/check-conformance.sh:10-45` and `scripts/check-map-conformance.sh:3-41` repeat engine selection, profile, DB, admission, and invocation configuration. The map script adds ten file selections and uses different defaults and artifact names. The general script already accepts suite files and selectors.

- Smallest approach: let the existing general owner handle the invocation; retain map suite selection and the existing map command's documented defaults/artifacts if that CLI remains supported. No new runner framework.
- Gross overlap about 25-30 lines; replacement/default handling about 10-15; likely net 10-20 tooling lines. A direct command/documentation migration could remove more, but changing an externally used command is not automatically functionality-preserving.
- Unresolved: artifact-name consumers and external callers; no recommendation to delete the script outright. Both full files read. No execution required or performed.

### T2: bootstrap monkeypatch — complexity lead, zero budgeted savings

`scripts/profile_offline_mongoose.py:105-129` mutates `bench.TOAST_MOO` and replaces `bench.wsl` with a native executor. `scripts/bench_players.py:1012` owns account bootstrap; callers include its CLI at 1098, offline profiling at 350, and gameplay at `scripts/run_mongoose_gameplay.py:449-450`.

- Possible simplification: make that existing bootstrap owner accept the invocation dependency and preserve the exact emergency-mode split/redaction behavior. This removes global monkeypatch coupling rather than adding another adapter layer.
- Net savings unknown and likely small. Must read complete bootstrap contract and platform callers before design; do not count it toward 5,000. Current exploration inspected the adapter and caller sites, not a complete redesign.

### Rejected tooling shortcuts

- Do not delete new collectors because they are large. The gameplay runner actually invokes the Barn/Toast process owners (`run_mongoose_gameplay.py:452-453`), records inputs, and consumes their results.
- Do not collapse `process_state` merely because the name repeats. `profile_offline_mongoose.py:132-141` reads main-thread `schedstat`; `mongoose_barn_process.py:71-90` computes process-wide CPU from user/system ticks. `run_mongoose_gameplay.py:70-79` explicitly selects the different CPU counters. Unifying their output meaning would corrupt comparisons.
- Do not treat `OwnedServer` and `OwnedBarnServer` as dead duplicates. Their launch protocols and checkpoint verification differ. `mongoose_barn_process.py` owns Barn profile/readiness and database/WAIF sidecar publication evidence; deleting that would remove functionality. Shared signal/handle cleanup would save little while introducing lifecycle coupling.
- Do not delete projection validation as repetitive type checking. `mongoose_error_capture.py:187-271` validates an external JSON boundary and its correlation provenance. Registry-level builtin checks do not replace this boundary.
- Do not delete retained JSON evidence, tests, or docs to hit the code target. Such deletions would neither establish runtime simplification nor preserve the requested evidence base.

### Existing verification surfaces inspected

`.github/workflows/ci.yml` runs Go build/tests, race tests for types, Python driver tests, and managed MOO conformance. `docs/elle-isolation.md` separately requires generated histories, actual exercised paths, and explicit checker verdicts for affected isolation work. These are descriptions of checked-in workflow source, not claims about current passing CI. No tests, benchmarks, conformance, or Elle runs were performed in this exploration.

## Synthesis: candidates independently checked by the parent

The parent read the source listed below after receiving the agent leads. These are source-backed opportunities, **not verified safe patches**. Estimates count physical production Go lines including accompanying comments/whitespace. They are net estimates, not executable-statement counts. No savings from tests, generated files, docs, or moving code into a dependency are included.

| Candidate | Net estimate | Source and exact limit of evidence |
| --- | ---: | --- |
| Delete duplicate file-I/O and trim admission checks | 132 | Parent inspected all 41 three-line fileio guards listed in the surfaces appendix and descriptors at builtins/fileio.go:1085-1115; trim guards at strings.go:312,328,344 and descriptors at 805-807. Registry checks before invocation at registry.go:139-145 and 245-248; VM calls it at vm/op_misc.go:79. Non-test direct-call searches found only timestamp wrappers for fileio and declarations for trim. Descriptor.Implementation is exported (descriptors.go:41-49), so external direct callers remain an API caveat. |
| Remove unused compiler scopes and duplicate identifier resolution | 57-69 | Parent read compiler/lower_locals.go completely; lower.go fields/initialization/program lowering; lower_expressions.go:46-75. Variables/Parent are written but never consulted for lookup; declareVariable already checks the same variables map. The resolveVariable search found only its definition and compileIdentifier caller. |
| Share V4/V5 parse orchestration and duplicate player readers | 80-95 | Parent read reader_v4.go:13-101, reader_v5.go completely, reader_v17.go:90-108, reader.go:207-224. All supported formats must remain. Preserve version-specific object bodies, section order and error strings. Startup performance is unmeasured. |
| Remove four store methods without repository Go callers | 50-55 | Parent read HasLocalProperty, IsPropertyDefinedOnObject, ResetInheritedProperties and InvalidateAnonymousChildren bodies, and reran the whole-repo Go reference search: declarations/comments only. Keep invalidateAnonymousChildrenLocked. This is repository absence evidence, not proof about external consumers of exported APIs. |
| One binary encoder and direct timestamp descriptor targets | 29-34 | Parent compared crypto.go:1447-1459 with encodeByte at 165-173 and fileio.go:177-192; same byte branches. Parent read timestamp wrappers at fileio.go:751-763, callee at 737-748 and all three descriptors at 1102-1104. Preserve the separate all-bytes hash encoder. Actual compiler inlining/performance remains unmeasured. |
| Share compile-time argument emission | 40-55 | Parent read pass/builtin lowering at lower_expressions.go:248-345 and verb-call lowering at 460-525. Same fixed/splice construction, with different caller-specific overflow wording and receiver/dynamic-name ordering. Preserve those and exact emitted instructions. |
| **Total, non-overlapping selected subset** | **388-440** | No implementation exists; final patch size and equivalence remain unknown. |

The 132-line admission subtotal is independently reproducible as `41 * 3 + 3 * 3`. The optional mkdir second-argument check is excluded. Descriptor admission uses a stricter call shape for some implementations (notably file_seek and mkdir); this strengthens the internal unreachable-guard case but is another reason not to claim raw exported implementations are interchangeable with validated calls.

### Bigger leads and limits of this pass

The appendices record wider admission-check removal (300-650 inclusive of the 132 above), crypto formatting/constructor sharing, verb selection, relationship enumeration, inheritance scans, bytecode clause decoding, parser arguments, and task initialization. These are **agent hypotheses not fully independently verified by the parent**; their figures are not added to the selected total or offered as safe changes. Each has source leads, gross/replacement estimates, callers, caveats, and rejected alternatives for another pass. The parent saw repeated verb and storage traversal windows but did not trace all their semantic differences.

This was a broad reconnaissance of major implementation areas, not an exhaustive reachability audit, external Go API audit, or proof that 5,000 lines cannot be removed. Server lifecycle/concurrency and internal CLI code were sampled rather than exhaustively audited. No convincing multi-thousand-line subsystem deletion emerged. Removing specialized VM paths, persistent data structures, legacy persistence support, or diagnostic evidence is not an acceptable shortcut to the target.

### Artifact and verification state

- Main note and three agent appendices are the exploration deliverable. Agents made no implementation changes.
- Immediately before final synthesis, `git diff --name-only` and `git status --short --untracked-files=no` printed no output. The four exploration notes were untracked. Existing unrelated untracked content was preserved.
- Developer checkpoint hooks additionally required root-level notes-deletion-exploration-checkpoint.md and notes-deletion-storage-checkpoint.md. Those checkpoint-only files remain uncommitted as instructed; the living task record remains here.
- No tests, benchmarks, conformance sessions, servers, or Elle checks ran. Nothing here proves the requested no-break/no-slow condition yet. No issue, PR, review or external message was published.
