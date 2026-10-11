# Barn performance execution checkpoint

- Active objective: clear the scheduler correctness blocker, then fully confirm the C4 one-argument `tostr` optimization.
- Repository state: `master` at `67f7725eb428d6f1540897c01aa66f16126954ee`; tracked state is clean. Extensive pre-existing untracked files remain untouched.
- C4 triage evidence already verified: `599,912 -> 399,912 allocs/op`, median `35,773,548 -> 33,288,606 ns/op`, and `12,808,091 -> 10,927,776 B/op`; source remains isolated on the experiment branch.
- Full confirmation is blocked by `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`.
- Current L1 hypothesis: `scheduler.Scheduler` and global `task.Manager` independently allocate task IDs; eval creates via the manager while scheduler paths allocate from `Scheduler.nextTaskID`, allowing manager registration to overwrite a live task on collision.
- Evidence read so far: the failing test explicitly probes the manager's next ID, coerces `Scheduler.nextTaskID` to the same value, creates one task through each owner, and observes the manager entry overwritten.
- Current blocker: none; exact production construction paths remain to be read before choosing the owner-preserving fix.
- Next action: read `task/manager.go`, scheduler ID allocation, eval task creation, and queue/restore paths; reproduce the one targeted test once; then make one bounded source fix and run targeted/race/full gates.
- This checkpoint file is untracked operational handoff only and must not be staged or committed.

## Checkpoint 2

- Targeted red reproduction passed as a reproducer: the test failed at ID 3 with manager registration overwriting the eval task.
- Ownership audit found `task.Manager.CreateTask` had exactly one production caller, `scheduler/eval.go`; all other production task IDs already came from `Scheduler.newTaskID`.
- Source slice applied:
  - removed `Manager.nextTaskID` and `Manager.CreateTask`;
  - changed eval to construct `task.NewTask(s.newTaskID(), ...)` and register it;
  - rewrote the regression to simulate restored high-water ID 99, verify eval gets 100, queued task gets 101, and manager registration remains present.
- No helper, adapter, counter synchronization, or compatibility path was added.
- Current blocker: none; the edited slice has not yet been formatted or tested.
- Next action: run `gofmt` on the three edited Go files, run the exact targeted test, then targeted scheduler/race and full-suite gates before committing the source fix.

## Checkpoint 3

- Scheduler fix validation passed: exact regression, whole scheduler package, exact test under `-race`, and `go test ./...`.
- Source fix committed as `41b904c` (`fix(scheduler): unify task ID allocation`), changing only `scheduler/eval.go`, its regression test, and `task/manager.go`; tracked state returned clean.
- C4 full confirmation is now unblocked.
- The current Experiment skill was reread. Its Ward activation and mandatory documentation/record commits conflict with explicit user instructions and are overridden for this run.
- Substantive confirmation controls retained: new isolated branch/worktree from green `41b904c`, fixed seven-row development command with 10 samples per side, unchanged evaluator, exact C4 source delta, targeted/full tests, ≥3% geomean gate, no allocation regression, and separate verification.
- Holdout remains sealed and will not be run without a new user authorization naming the exact candidate commit.
- Current blocker: none.
- Next action: read Go performance/statistics skills, create the clean confirmation worktree, collect the 10-sample baseline to temporary output, apply the exact C4 source commit, test, collect candidate samples, and compute the gate without documentation commits.

## Checkpoint 4

- Created `experiment/barn-vm-c4-confirm-20260715` in `C:\Users\Q\code\barn-worktrees\c4-confirm-20260715` from exact green base `41b904c34b1aee2e6d118f564985feec046055e3`; initial tracked state was clean.
- Frozen seven-row development baseline completed successfully with `-benchmem -count=10 -cpu=1` in 89.895 seconds. No holdout rows were run.
- Baseline `tostr_200k` remained at 599,912 allocs/op, approximately 12,808,090 B/op, and approximately 36-37 ms/op. Raw output is outside Git at `C:\Users\Q\AppData\Local\Temp\barn-c4-confirm-20260715\before.txt`.
- Cherry-picked the exact triaged C4 source commit without conflict. Candidate commit is `375ac99` (`perf: bypass builder for one-argument tostr`) and changes only `builtins/types.go`; tracked state is clean.
- Current blocker: none.
- Next action: run `go test ./builtins ./vm`, then `go test ./...`; if both pass, collect the matching ten-sample candidate run and evaluate it with `benchstat` against the frozen baseline.

## Checkpoint 5

- C4 candidate commit `375ac99` passed `go test ./builtins ./vm` and `go test ./...`.
- Matching ten-sample candidate benchmark completed successfully; raw output is outside Git at `C:\Users\Q\AppData\Local\Temp\barn-c4-confirm-20260715\after.txt`. No holdout rows were run.
- `benchstat` result: `tostr_200k` improved 8.15% (`p=0.023`), B/op improved 14.68%, and allocs/op improved 33.34% (exactly 200,000 fewer). The development-set geomean nevertheless regressed 0.47%, and unrelated `builtin_abs_200k` regressed 41.61% (`p=0.001`) amid severe timing disturbance.
- C4 therefore failed the fixed full-experiment keep threshold and was rejected; no rerun or reinterpretation was substituted for the gate.
- Revert commit `dac140f` removes the entire C4 source delta. `builtins/types.go` now matches green base `41b904c` exactly and the confirmation worktree is tracked-clean.
- Current blocker: none.
- Next action: read the exact preregistered C2 probe definition, then execute only that bounded probe on its own clean experiment branch/worktree.

## Checkpoint 6

- Created clean C2 worktree `C:\Users\Q\code\barn-worktrees\c2-ticks-20260715` on `experiment/barn-vm-c2-ticks-20260715` from exact green base `41b904c`.
- Ran the preregistered isolated `int_arith_1M` CPU profile only; no holdout and no source edit. Raw profile is outside Git at `C:\Users\Q\AppData\Local\Temp\barn-c2-ticks-20260715\int-arith.cpu.prof`.
- `go tool pprof -list=CountsTick` reports `CountsTick` at 80 ms flat out of 1.17 s total, or 6.84% flat. This exceeds the 2% killing threshold, so C2 survives its probe.
- Source inspection confirms `OpCode` is a byte and the current five-case switch has no contract test. A 256-entry opcode-indexed boolean metadata table can replace it without a bounds guard or new helper; an exhaustive test can preserve the exact counted set.
- Current blocker: none.
- Next action: collect the three-sample isolated `int_arith_1M` pre-change measurement, implement the table and exhaustive exact-set test as one bounded slice, then run correctness and matching post-change samples.

## Checkpoint 7

- C2 pre-change `int_arith_1M` samples: 33,566,792; 32,080,436; and 32,473,832 ns/op, with 8,864 B/op and 11 allocs/op.
- Applied one bounded slice in `bytecode/opcodes.go` and new `bytecode/opcodes_test.go`: replace the five-case `CountsTick` switch with a 256-entry opcode-indexed boolean table and exhaustively verify all byte values preserve exactly the five counted opcodes.
- `go test ./bytecode ./vm` and `go test ./...` passed.
- C2 post-change samples: 30,766,336; 30,970,342; and 30,777,225 ns/op, with unchanged 8,864 B/op and 11 allocs/op.
- `benchstat` median changed 32.47 ms -> 30.78 ms (approximately 5.2% faster), with `p=0.100` at n=3. This satisfies directional triage but is not promotion evidence.
- Current blocker: none.
- Next action: commit only `bytecode/opcodes.go` and `bytecode/opcodes_test.go` as the kept C2 triage slice, then create a fresh full-confirmation worktree and run the exact ten-sample development A/B gate.

## Checkpoint 8

- C2 triage source is committed as `1532289038c45347eaaa91471f3a4a49ffef1d9b` (`perf(bytecode): table-drive tick accounting`) with only `bytecode/opcodes.go` and `bytecode/opcodes_test.go`; its worktree is tracked-clean.
- Created full-confirmation worktree `C:\Users\Q\code\barn-worktrees\c2-confirm-20260715` on `experiment/barn-vm-c2-confirm-20260715` from untouched green base `41b904c34b1aee2e6d118f564985feec046055e3`; tracked state is clean.
- C4 consumed the first full-experiment slot and failed its fixed gate. C2 full confirmation will consume the second of the campaign's three slots.
- Current blocker: none.
- Next action: collect the exact seven-row ten-sample development baseline on untouched `41b904c`, cherry-pick exact C2 commit `1532289`, run correctness gates, collect the identical candidate samples, and apply the fixed `benchstat` threshold.

## Checkpoint 9

- C2 full confirmation used untouched `41b904c` baseline and candidate `8467f84` (exact cherry-pick of triage source), with ten samples per side on exactly the seven development rows. No holdout rows were run.
- Candidate passed `go test ./bytecode ./vm` and `go test ./...`.
- `benchstat` timing: `int_arith_1M` -1.70% (`p=0.000`), `float_arith_1M` -1.64% (`p=0.001`), `list_index_1M` -5.23% (`p=0.000`), development-set geomean -1.08%. Other timing rows had no significant change.
- B/op and allocs/op had no regressions.
- C2 failed the fixed at-least-3% geomean keep threshold and was rejected despite coherent targeted improvements.
- Revert commit `37666e6` removes the entire C2 source/test delta from the confirmation branch. Both bytecode paths match green base `41b904c` exactly and tracked state is clean.
- Current blocker: none. Two of three full-experiment slots have been consumed; neither passed.
- Next action: execute the next preregistered target, C3, beginning with its isolated `builtin_abs_200k` CPU profile and fixed visibility/5% killing threshold before any source edit.

## Checkpoint 10

- Created clean C3 worktree `C:\Users\Q\code\barn-worktrees\c3-builtin-resolution-20260715` on `experiment/barn-vm-c3-builtin-resolution-20260715` from exact green base `41b904c`.
- Ran the preregistered isolated `builtin_abs_200k` CPU profile only; no holdout and no source edit. Raw profile is outside Git at `C:\Users\Q\AppData\Local\Temp\barn-c3-builtin-resolution-20260715\builtin-abs.cpu.prof`.
- C3 survives: `NeedsLineSyncByID` is separately visible at 0.81% flat, `CallByID`'s entry load is 3.25% flat, and the registry cluster is far above the 5% threshold (`Registry.dispatch` alone is 23.58% flat).
- Caller audit found one production `CallByID` call in `vm/op_misc.go`; all other direct calls are characterization tests. `NeedsLineSyncByID` is used only by the same VM call surface.
- Registry source confirms the duplicate sequence resolves `r.entries[id]` once for line-sync metadata and again for dispatch; protected redirect-before-validation ordering is inside `dispatch` and must remain unchanged.
- Attempted read of guessed path `kernel/task_context.go` failed because it does not exist; no design conclusion was drawn from that missing artifact.
- Current blocker: none.
- Next action: locate the actual `kernel.TaskContext` definition, inspect its existing VM/call-stack ownership, then choose the smallest change that removes the duplicate lookup without adding a parallel dispatch API.

## Checkpoint 11

- Located `kernel.TaskContext` in `kernel/context.go`. It already carries `CallerVM`; adding another context interface would broaden the design and was rejected.
- Froze three pre-change samples for exactly C3's expected rows. `builtin_abs_200k`: 14,064,320; 14,120,635; 13,944,449 ns/op. `tostr_200k`: 41,617,971; 37,026,990; 37,102,303 ns/op. Allocation metrics match the established baseline.
- Applied one untested C3 slice across existing owners only:
  - `Registry.CallByID` now accepts the existing VM line-sync callback, resolves `entries[id]` once, conditionally syncs for the entry, and calls unchanged `dispatch`;
  - deleted `NeedsLineSyncByID`;
  - VM passes `vm.syncTaskLineNumbers` to `CallByID`;
  - direct characterization callers pass nil, and a new characterization verifies sync occurs once before a `callers` dispatch and does not occur for `abs`.
- No parallel dispatch API, exported entry type, context interface, or adapter was added.
- Caller audit confirms the VM is the sole production `CallByID` caller and all Go call sites match the new signature.
- Current blocker: none; the slice has not yet been formatted or tested.
- Next action: run `gofmt`, inspect the exact diff, run targeted dispatch/VM tests, then collect matching three-sample post-change rows and decide triage survival before any commit.

## Checkpoint 12

- C3 slice formatted cleanly and changed only `builtins/registry.go`, `vm/op_misc.go`, and dispatch characterization tests. `go test ./builtins ./vm` passed.
- Matching three-sample post-change result: `builtin_abs_200k` median 14.06 ms -> 13.86 ms (about 1.4%); `tostr_200k` median 37.10 ms -> 36.75 ms (about 0.9%); two-row geomean improved 1.20%. B/op and allocs/op were unchanged.
- C3 failed its predicted at-least-2% improvement on both expected rows. The callback/API complexity was therefore rejected without consuming the third full-experiment slot.
- Used Git restore to remove the entire uncommitted C3 source/test slice. The three paths match green base `41b904c` and the C3 worktree is tracked-clean.
- Current blocker: none. Round order now advances to C1, the final preregistered first-round target.
- Next action: read C1's exact isolated `int_arith_1M` decode line-profile procedure and 8%/compiler-elision killing condition, create its clean worktree, and run only that probe before any source edit.

## Checkpoint 13

- Created clean C1 worktree `C:\Users\Q\code\barn-worktrees\c1-frame-decode-20260715` on `experiment/barn-vm-c1-frame-decode-20260715` from exact green base `41b904c`.
- Ran the preregistered isolated `int_arith_1M` CPU profile only; no holdout and no source edit. Raw profile is outside Git at `C:\Users\Q\AppData\Local\Temp\barn-c1-frame-decode-20260715\int-arith.cpu.prof`.
- C1 survives strongly: `CurrentFrame` is 10.83% flat, `ReadByte` is 6.67% flat/17.50% cumulative, and `ReadShort` is 2.50% flat, about 20% combined versus the 8% killing threshold.
- Line attribution assigns 130 ms directly to `return vm.frame`, so the frame reload is not compiler-elided; byte/code/IP lines are separately charged.
- Ownership audit found `executeLoop` already caches the instruction's frame, but `Execute` and operand-decoding handlers discard it and reload `vm.frame` through `ReadByte`, `ReadShort`, and `CurrentFrame`.
- Proposed bounded design: pass the already-cached `*StackFrame` from `executeLoop`/`Step` into existing `Execute`, pass it through only handlers that decode operands, and change existing `ReadByte`/`ReadShort` to read that frame directly. No reader/helper/adapter or extra VM state.
- Current blocker: none.
- Next action: freeze three pre-change `int_arith_1M` samples, implement only the frame-threading slice, run terminator and VM correctness guards, then collect matching post-change samples and decide whether C1 earns the final full-experiment slot.

## Checkpoint 14

- Froze C1 pre-change `int_arith_1M` samples: 31,459,865; 31,750,853; and 32,300,533 ns/op, with 8,864 B/op and 11 allocs/op.
- Frame-threading slice is in progress and intentionally incomplete:
  - existing `ReadByte`/`ReadShort` now accept the cached instruction frame;
  - `executeLoop` and `Step` pass their fetched frame into `Execute`;
  - direct stack/variable/control/range operand decoding in `Execute` uses that frame;
  - dispatch calls now pass the frame to existing operand-decoding handlers;
  - `control.go`, `op_iter.go`, and the decoding functions in `op_index.go` have been updated.
- No new helper, reader type, adapter, VM field, or cached mutable state was introduced.
- Current blocker: none. The slice does not compile yet because the remaining existing handler signatures/read calls still need the same mechanical frame parameter.
- Next action: finish only the remaining decoding handlers in `op_list.go`, `op_logic.go`, `op_misc.go`, `op_property.go`, and `op_verb.go`; then audit all `ReadByte`/`ReadShort`/`Execute` call sites before formatting or testing.

## Checkpoint 15

- Completed and formatted the C1 frame-threading slice across ten existing VM files: 100 insertions and 111 deletions; `git diff --check` passes.
- Audit finds no remaining zero-argument `ReadByte`, `ReadShort`, or single-argument `Execute` call, and no stale zero-argument decoding-handler signature.
- Full diff review confirms changes are limited to passing the instruction's already-fetched `*StackFrame`, reading bytecode/constants/locals/IP from that frame, and removing redundant `CurrentFrame` reloads.
- Call, return, error handling, yield/resume, tick accounting, stack behavior, and exception dispatch logic are unchanged. No new helper/type/adapter/field/state was introduced.
- Current blocker: none; the completed slice has not yet compiled or run tests.
- Next action: run the exact terminator tests, then `go test ./vm`; if green, collect matching three-sample `int_arith_1M` post-change data and apply the directional triage prediction before any commit.

## Checkpoint 16

- Initial C1 compile gate found one mechanical parameter/local naming collision in `executeCallVerb`; renamed only the passed instruction frame to `callerFrame` and kept callee-frame construction unchanged.
- Exact terminator guards and `go test ./vm -count=1` then passed.
- Matching post-change `int_arith_1M` samples: 36,221,847; 36,379,742; and 38,954,891 ns/op, with unchanged 8,864 B/op and 11 allocs/op.
- `benchstat` median regressed 31.75 ms -> 36.38 ms, approximately 14.6%. C1 decisively failed its directional prediction; the frame parameter increased dispatch cost.
- Used Git restore to remove the entire ten-file C1 slice. All paths match green base `41b904c` and the C1 worktree is tracked-clean.
- Current blocker: none. The frozen first-round order C4, C2, C3, C1 is complete. C4 and C2 survived triage but failed full gates; C3 and C1 were rejected at triage. Two of three full-experiment slots are consumed.
- Next action: reread the campaign control surface and select only the next explicitly authorized eligible target; do not invent another probe or consume holdout/full-experiment budget without its existing authority.

## Checkpoint 17

- Campaign reread explicitly leaves C5 and C6 eligible after Round 1 and ranks C5 first on the allocation/string front; selected C5 without expanding budget or opening holdout.
- Created clean C5 worktree `C:\Users\Q\code\barn-worktrees\c5-str-header-20260715` on `experiment/barn-vm-c5-str-header-20260715` from exact green base `41b904c`.
- Current `strRep` is a 48-byte 64-bit header (`string` + `[]byte` + watermark pointer), and every append allocates a fresh header even when backing bytes are reused.
- Exact prior art requires keeping the shared monotonic frontier, writing only uncommitted backing slots, advancing the frontier, returning a fresh header, and copying when an alias no longer owns the frontier. It rejects refcount and alias-analysis mutation.
- Proposed representation-only prototype: `strRep` retains materialized `string`, a shared backing pointer, and per-header logical length (32-byte header); backing bytes and the shared frontier move together into one backing object allocated only on copy/growth paths. No existing header is mutated.
- Expected effect from 16 bytes saved per frequent header: exceed C5's 15% B/op gate on `string_concat_10k` and 5% gate on `tostr_200k`, with unchanged allocation count.
- Current blocker: none.
- Next action: freeze three pre-change samples of exactly `string_concat_10k` and `tostr_200k`, implement only `types/str.go`, run string aliasing/type/VM guards, and collect matching post-change samples before deciding survival.

## Checkpoint 18

- C5 pre-change samples are frozen. `string_concat_10k`: 531,944 B/op, 10,034 allocs/op, 866,172-876,889 ns/op. `tostr_200k`: 12,808,089-12,808,090 B/op, 599,912 allocs/op, 36.54-38.39 ms/op.
- Applied the representation-only prototype in `types/str.go`: 32-byte immutable string headers hold materialized string/backing pointer/logical length; a shared backing object owns bytes and monotonic frontier.
- Initial exact alias guard exposed one accessor bug: `literal()` directly indexed the backing slice whose stored length can remain shorter than a newer header's logical length. Corrected only that accessor to reslice `data[:length]` before indexing.
- After correction, both `TestStringInplaceAppendAliasing` and `TestStringInplaceAppendPropertyCapture` pass, and `go test ./types ./vm -count=1` passes.
- Current blocker: none.
- Next action: collect matching three-sample post-change data for exactly `string_concat_10k` and `tostr_200k`, then apply C5's B/op, allocs/op, and timing survival gates before any commit or full experiment.

## Checkpoint 19

- C5 first prototype reduced B/op by 30% on `string_concat_10k` and 25% on `tostr_200k` with unchanged alloc counts, but regressed timing by about 6.9% and 19.1% respectively; two-row geomean regressed 12.84%. Rejected and fully restored `types/str.go` to `41b904c`.
- Read the Go zero-copy performance and string-correctness skills before a second design. Their controlling rule is that bytes exposed through `unsafe.String` must never be modified for the returned string's lifetime.
- Applied C5's second and final slice in only `types/str.go`: compact direct header is `[]byte + watermark pointer` (32 bytes); `NewStr` stores a read-only zero-copy byte view with nil watermark; `Str` returns a zero-copy string view; append-backed headers retain the existing shared frontier.
- Safety argument: string-backed views can never enter the in-place branch because watermark is nil; append-backed reuse writes only beyond every existing header's length, so no byte within an existing zero-copy string is modified; stored slices preserve backing lifetime.
- `go test ./types -count=1`, both exact string alias/property-capture guards, and `go test ./vm -count=1` pass.
- Current blocker: none.
- Next action: collect the same three post-change samples for `string_concat_10k` and `tostr_200k`; if any C5 allocation or timing gate fails, restore and end C5 immediately.

## Checkpoint 20

- C5 second-slice triage result: `string_concat_10k` median 872.5 us -> 817.3 us (-6.3%), B/op 531,944 -> 361,704 (-32.0%), allocs/op 10,034 -> 10,033. `tostr_200k` median 37.13 ms -> 34.80 ms (-6.3%), B/op about 12.21 MiB -> 9.16 MiB (-25.0%), allocs/op unchanged at 599,912.
- Two-row timing geomean improved 6.30% and B/op geomean improved 28.58%; C5 passes every directional triage gate.
- Race/checkptr-enabled exact alias/property-capture guards and `go test ./...` passed.
- Committed only `types/str.go` as `97f0700272b7a8318f44f4aeb8d427f82d5e21cd` (`perf(types): compact string representation`); triage worktree is tracked-clean. No notes, raw benchmarks, or documentation were committed.
- Current blocker: none. C5 is a triage survivor, not promotion evidence; two of three full-experiment slots are consumed.
- Next action: create a fresh C5 confirmation worktree from untouched green `41b904c`, collect the exact seven-row ten-sample baseline, cherry-pick exact C5 commit, run correctness/alias/race gates, collect identical candidate samples, and apply the final full-experiment threshold.

## Checkpoint 21

- Created clean C5 confirmation worktree `C:\Users\Q\code\barn-worktrees\c5-confirm-20260715` on `experiment/barn-vm-c5-confirm-20260715` from untouched green base `41b904c`.
- Exact seven-row ten-sample baseline completed in 90.809 seconds; raw output is outside Git at `C:\Users\Q\AppData\Local\Temp\barn-c5-confirm-20260715\before.txt`. No holdout rows were run.
- Baseline has visible late-run timing disturbance in unrelated `builtin_abs_200k` and `tostr_200k` samples. It remains frozen; no selective rerun or substitution was made.
- Cherry-picked exact C5 source as candidate `46d7042`; it changes only `types/str.go` and tracked state is clean.
- Candidate passes `go test ./types -count=1`, race/checkptr-enabled exact string alias/property-capture guards, and `go test ./...`.
- Current blocker: none.
- Next action: reread the campaign gate after the passing full suite, collect the identical seven-row ten-sample candidate run, and apply `benchstat` without reinterpreting or replacing the noisy frozen baseline.

## Checkpoint 22

- C5 final full experiment completed with ten samples per side on exactly the seven development rows; no holdout rows were run.
- Timing: `string_concat_10k` -4.14% (`p=0.002`), `float_arith_1M` +1.15% (`p=0.002`, below the 3% regression guard), other untargeted rows had no prohibited regression, and development-set geomean improved 1.53%.
- Allocation: `string_concat_10k` B/op -32.00% and allocs/op -0.01%; `tostr_200k` B/op -24.98% with unchanged allocs/op; no row regressed.
- C5 nevertheless failed the fixed at-least-3% development geomean threshold. Revert commit `b2e5762` removes the entire candidate from the confirmation branch; `types/str.go` matches green base `41b904c` and tracked state is clean.
- All three authorized full-experiment slots are now consumed: C4 failed, C2 failed, C5 failed. No candidate was integrated and holdout remained sealed.
- Current blocker: the campaign's literal budget-exhaustion kill rule. Further C6 or other triage cannot change an integration decision without user-authorized full-experiment budget expansion or a changed keep threshold.
- Next action: stop the performance campaign, verify the exact main-branch landed state, and report the blocker and the strongest rejected candidates without documentation commits.

## Checkpoint 23

- User authorized one cumulative development experiment combining the measured C2, C4, and C5 survivors under the original seven-row gate; the sealed holdout remains unopened.
- The unrelated primary checkout remains untouched on `work/mvcc-concurrent-moo`. Created isolated worktree `C:\Users\Q\code\barn-worktrees\cumulative-c2-c4-c5-20260715` on `experiment/barn-vm-cumulative-c2-c4-c5-20260715` from exact green base `41b904c34b1aee2e6d118f564985feec046055e3`.
- Verified survivor source commits `1532289` (C2), `b806c9d` (C4), and `97f0700` (C5), evaluator tree hash `bf3b40994378d527ee7753fb1fbee2e3b345aba0`, Go 1.26.0, and installed `benchstat`.
- Fresh base `go test ./...` passed in the cumulative worktree. No source change has been applied yet.
- Current blocker: none.
- Next action: collect the exact fresh seven-row `-count=10 -cpu=1` baseline outside Git, then apply the three source commits as an auditable stack and measure the identical candidate command.

## Checkpoint 24

- Fresh baseline completed successfully in 86.213 seconds with exactly ten samples for every development row, zero holdout samples, and a 11.73 ms timing geomean. External artifact: `C:\Users\Q\AppData\Local\Temp\barn-cumulative-c2-c4-c5-20260715\baseline.txt`.
- Applied the survivor stack as three source commits: C2 `48a7e1b`, C4 `35ef113`, and C5 `5755a7d`. The cumulative diff changes only `bytecode/opcodes.go`, its exhaustive test, `builtins/types.go`, and `types/str.go`; tracked state is clean.
- C2 bytecode/VM tests, C4 builtins/VM tests, C5 types/VM tests, and the actual VM alias/property-capture guards pass. An earlier alias command against `./types` matched no tests and was discarded before running the same names correctly under `./vm`.
- The identical candidate benchmark is currently running from exact candidate `5755a7dcfb9a3ff0d96628ca9735eeb541482015`; output is being written outside Git to `C:\Users\Q\AppData\Local\Temp\barn-cumulative-c2-c4-c5-20260715\candidate.txt`. No second candidate run has been started.
- Current blocker: none; the direct process launcher returned before its still-running child, so completion is being tracked against the existing `vm.test` process rather than relaunching.
- Next action: let the existing candidate run finish, verify ten samples per row and zero holdout rows, then run `benchstat` against the frozen baseline and apply the preregistered gate.
