# Toast conformance checks current state

- Scope: moo-conformance-tests issue 17 and PR 34, with Barn untouched by this PR.
- Exact PR head: `020277532d6bb2a7620c9312e29bbc86f9d274f6`.
- Current fix: the protected `Full suite` job now needs both `quality` and the Toast matrix, and explicitly requires both dependency results to be `success` despite `if: always()`.
- The superseding GitHub Actions run is `30979863272`; it is currently pending/running.
- Local quality proof before the dependency-only change: 222 harness tests passed, 11,566 packaged cases collected, duplicate baseline exact, Ruff green, mypy green, actionlint green, and `git diff --check` green.
- Managed WSL Toast evidence: the first admitted profile ran 3,468 eligible cases and exposed three assertions; the GC assertion was repaired as isolation-sensitive, while two exec queue assertions still need exact Toast-oracle grounding.
- Constraint: use only the managed harness with `/root/src/toaststunt/build-release/moo`; do not manually launch or kill servers and do not use a tracked database.
- Review finding repaired: branch protection required only `Full suite`, and that job previously omitted the quality dependency. Commit `0202775` closes that fail-open path.
- Current blocker: exact-head Toast matrix has not completed, and raw Toast behavior for the two queued exec assertions is not yet confirmed.
- Next action: run a narrowly selected managed-harness oracle probe for the exec queue shape while exact-head CI runs, then update the tests only to behavior demonstrated by Toast; rerun and obtain adversarial plus final verifier approval before merge.

## Update at head 379f1a4

- Managed WSL Toast returned suspended exec queue field 2 as an absolute per-run path (`/tmp/moo_conformance_.../executables/sleep`), proving the prior relative path assertion invalid.
- The two real exec cases now match their owned task IDs without assuming the temporary server directory; exact managed rerun: 2 passed, 15 deselected.
- Full local quality rerun is green: 222 units, 11,566 collection, exact 45/28 duplicate baseline, Ruff, mypy, and diff check.
- Exact PR head is `379f1a404abdd710af3157c99faa6b0a7640fd6f`; workflow run is `30980190093`.
- Quality job is green. All three Toast shards are building/running. PR remains draft and must not merge until the exact-profile union ledger is green and final review approves it.
- Current PR audit: 28 files and five commits, literal `Closes #17`, mergeable state, no review decision yet.
- Next action: wait for all three exact-head Toast profile jobs and protected `Full suite`; inspect any failure artifact rather than infer; then final read-only verifier, ready PR, full merge audit, normal merge commit, two-parent verification, issue closure, and branch-protection verification.

## Red shard evidence from run 30980190093

- `Toast / 64bit-outbound-off` failed after executing its entire profile: 11,480 passed, 74 declared skips, 12 failed in 275.91 seconds.
- Retained artifact `toast-conformance-64bit-outbound-off` was downloaded to the issue worktree `.tmp` for inspection; it is untracked and must not be staged.
- All 12 failures are in `builtins/url_curl.yaml`: seven value-level `url_encode`/`url_decode` cases and three protocol/header curl result cases plus related cases reported E_PERM with Toast started using `--no-outbound`.
- Arity/type/permission cases in the same suite still passed, showing outbound-disabled Toast preserves validation errors but denies behavior that reaches curl-backed operation.
- The YAML currently gates only on builtin presence, so these cases were admitted despite profile metadata declaring `option.OUTBOUND_NETWORK: false`.
- This is exact managed Toast CI evidence, not an inferred Barn diagnosis.
- Current blocker: the outbound-off eligibility contract is incomplete; those behavior cases need exact option gating while validation/permission cases should remain exercised.
- Next action: classify all `url_curl.yaml` cases against the exact outbound-off results, add the narrow `skip_if: not option.OUTBOUND_NETWORK` only where Toast returned outbound-policy E_PERM, run focused managed Toast on both outbound profiles, rerun quality and duplicate accounting, then push a new exact head.

## Compound admission construction point

- Exact classification is nine URL encode/decode value cases plus three curl result-map cases. Arity, type, permission, and high-byte validation cases remain executable with outbound disabled and must not be skipped.
- Replacing each existing builtin-presence guard outright would weaken portable conformance for implementations missing that builtin.
- The first focused managed attempt failed closed during collection because the current grammar accepts one atomic `skip_if` only; both outbound-on and outbound-off probes collected zero and raised the exact malformed-condition error.
- New uncommitted red tests now specify a narrow disjunction: schema accepts `missing builtin.url_encode or not option.OUTBOUND_NETWORK`, rejects `and` and trailing `or`, runtime emits the first matching atomic reason, and strict skip accounting authorizes either exact declared atomic reason.
- Intended implementation: retain atomic `parse_skip_condition`, add a closed `parse_skip_conditions` splitter for exact ` or `, evaluate alternatives left-to-right, and expose each alternative reason to strict accounting. No general expression evaluator, parentheses, or implicit syntax.
- Live CI update: 64bit-outbound-on is green; 64bit-outbound-off is red with the 12 classified failures; 32bit still running; protected `Full suite` must fail. PR remains draft and blocked.
- Next action: run the new focused tests red, implement the minimal closed disjunction parser/evaluator/accounting, rerun focused units and managed outbound on/off suite, then the complete quality surface and a new exact-head CI run.

## Exact head fe4d4b5

- Commit `fe4d4b52580ea26c05efcac4cbf070ad4d35129b` implements the closed exact-`or` grammar, atomic strict-reason accounting, unit coverage, and the 12 outbound-sensitive URL/curl gates.
- Red proof before implementation: three focused tests failed in schema parsing, runtime enforcement, and strict accounting.
- Green focused unit proof: 159 passed, then strengthened first/second alternative coverage brought the full harness to 228 passed.
- Managed Toast focused proof: outbound-off 10 passed and exactly 12 skipped with `Requires option: OUTBOUND_NETWORK`; outbound-on 22 passed.
- Full local proof: 228 units, 11,566 packaged identities collected, exact 45/28 duplicate baseline, Ruff, mypy, and diff check all green.
- Superseding workflow run is `30980948415`. Quality is green. `Toast / 64bit-outbound-off` is green in 5m35s. `Toast / 64bit-outbound-on` is green in 5m37s. `Toast / 32bit-outbound-on` remains in progress.
- Final verifier reports the seven-file admission commit is clean and limited to the exact closed grammar plus 3 encode, 6 decode, and 3 curl behavior gates; validation/security cases remain active outbound-off.
- PR remains draft. Next action: await 32-bit completion and protected exact-union `Full suite`, inspect ledger artifact, obtain final MERGE verdict, then update PR proof and perform guarded normal merge workflow.

## 32-bit fatal Toast case

- Run `30980948415` is terminal failure. Quality and both 64-bit profiles are green; 32-bit is red; protected `Full suite` failed closed at its prerequisite gate.
- 32-bit exact accounting: 5,269 passed, 60 skipped, 6,237 failed = all 11,566 packaged identities represented.
- The first causal failure is `builtins/math.yaml::minint_modulus_edge_case`, executing `-2147483648 % -1`: expected 0, transport returned no value. The next normal division also returned no value, then broken-pipe and connection-refused errors cascade through the remaining 6,236 cases.
- This establishes that the pinned 32-bit Toast process dies at the INT_MIN modulus edge; the later failures are transport fallout, not independent behavior divergences.
- Both the adversary and verifier correctly return NO-MERGE for exact head `fe4d4b5`.
- Proposed narrow correction: add a literal exact skip reason to that one fatal upstream Toast case and add the same identity/reason to `ci/toast-never-executed.json`; retain its existing 64-bit gate. This increases the explicit reviewed never-executed baseline from 40 to 41 and allows the 32-bit profile to execute every other eligible case.
- Next action: add red unit proof that the 41-entry baseline matches packaged declarations, implement the one-case skip/baseline, run local quality and a focused managed proof where possible, push a new head, and rerun all profiles. Do not suppress the cascade generically or weaken the protected gate.
