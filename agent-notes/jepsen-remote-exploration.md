# Jepsen remote exploration (plan section 6, Delivery item 5)

## Goal
PR H of `plans/jepsen-expansion.md`: bounded 30-minute exploratory Jepsen
batches on the dedicated `barn-bug-discovery` / `barn-fuzz` compute path through
the existing fuzz compute owners; covering-schedule rotation; retained evidence
bundles; offline and re-execution replay; per-batch yield report. Q authorized
the full plan 2026-11-10.

## State
Branch `jepsen/remote-exploration` (worktree `../barn-jepsen-explore`) from
master after PR G (#762). Built:
- `tests/jepsen-checkpoint/.../core.clj`: every generated client op carries
  `:phase` (its key phase offset); `--replay FILE` sends each phase's recorded
  transactions in order instead of generated ones, phases and nemesis schedule
  unchanged.
- `scripts/jepsen_checkpoint.py`: `--replay` (writes `replay.jsonl` into the new
  evidence directory; refuses a recording without phases before creating it),
  `--hot-keys N` (key count of the list-append families), provenance records
  both; analysis `replay_fidelity` (sent transactions must be each phase's
  recorded prefix, else invalid; none sent is incomplete).
- `scripts/jepsen_explore.py`: `plan`, `batch`, `replay`. Ledger directory
  (coverage pairs, round, measured run seconds per lane, findings with their
  recorded transactions and re-execution counts) carried by the cache.
- `scripts/fuzz_compute.cjs accept()`: `JepsenExploration` rule.
- `.github/workflows/fuzz.yml`: `jepsen` job (name JepsenExploration), accept
  needs it and downloads `jepsen-exploration-report`.
- `.github/workflows/jepsen.yml` campaign job: a short run plus its `--replay`,
  so the real jar's replay path runs on every PR.
- Tests: `scripts/test_jepsen_explore.py`, `test_fuzz_compute.py` Jepsen cases,
  `test_jepsen_checkpoint.py` ReplayTests and the provenance test.

## Live runner policy (read-only gh api, 2026-11-11 ~09:20Z)
- Repo variables: FUZZ_RUNNER_POLICY_VERIFIED=true,
  FUZZ_RUNNER_LABELS=["self-hosted","Linux","X64","barn-fuzz"],
  FUZZ_MAX_PARALLEL=4, FUZZ_CONTINUOUS_ENABLED=true,
  CI_RUNNER_LABELS=["self-hosted","Linux","X64","barn"].
- Org runner group 4 `barn-bug-discovery`: selected visibility (MongooseMoo/barn
  only), restricted_to_workflows=true, selected workflows exactly
  `.github/workflows/fuzz.yml@refs/heads/master` and
  `.github/workflows/fuzz-controller.yml@refs/heads/master`.
- Runners mongoose-barn-fuzz-1..4 online, labels self-hosted,Linux,X64,barn-fuzz.
- Consequence: exploration must be a job in fuzz.yml (a new workflow cannot use
  the group), and only a master dispatch can reach it (group policy and
  fuzz_compute.cjs configure() both refuse other refs), so the first batch is
  dispatched after merge.

## Decisions and why
- Replay identifies phases by a `:phase` tag, not by key ranges: the
  authority/topology families use base + role keys with no phase offset
  (core.clj shaped-txns), so keys cannot place them.
- A replay keeps the recording's parameters and seed; Python's only seeded
  randomness is the checkpoint-crash delay (jepsen_checkpoint.py Campaign
  `self.random`), Jepsen's seed is stagger timing only.
- Finding identity: class plus witness, or the message with digits erased when
  there is no witness. Only correctness-violation and recovery-failure count
  as defects; incomplete/invalid runs make a batch incomplete.
- Batch outcome finding > incomplete > clean; exit 1 / 4 / 0. accept(): report
  must exist, every run kept results.json, outcome must match the job
  (clean/success, finding/failure with only `Explore Jepsen` failed).
  Incomplete is never accepted.
- Re-executions first (at most 3, at most half the budget), each finding at
  most 5 attempts, ones that reproduced before first; then one baseline, then
  greedy pairwise configurations. Coverage is credited only for runs that
  kept results.
- Lane run-time estimates start as phase seconds + 120 s (not measured) and
  are replaced by the ledger's measured maximum per lane.
- GOMAXPROCS reaches Barn through the inherited environment
  (mongoose_barn_process.py OwnedBarnServer env); "runner" removes it.
- The conformance pin is read from jepsen.yml in the job, not copied.
- No execution reducer and no crash-artifact loader replay: the plan defers a
  reducer until a finding resists witness and artifact replay; the report says
  "not measured" for time to minimal reproduction.

## Surprises
- core.clj: the seed binds jepsen.random (stagger timing) only; Elle's generated
  transactions are not reproduced by a seed, so re-execution needs the recorded
  invokes fed back to Jepsen.
- Whether barn-fuzz hosts have lein/graphviz or passwordless sudo is unknown;
  the elle-cli action needs one of them. The first batch shows it.

## Next step
Open the PR, get CI green (jepsen.yml exercises the jar's replay), merge,
dispatch `fuzz.yml` dedicated-once on master, record the batch result here.
