# Bug discovery compute workflow

## Goal
Run retained public Barn fuzz campaigns on dedicated compute, with explicit
routing, native-to-differential artifact handoff and bounded replenishment.
Root owns host/policy provisioning, publication, CI, merge and live acceptance.
No new credentials or host changes belong to this worktree.

## State
Implementation and offline validation are complete for root review. The earlier
retention-only handoff was resumed by root. Native jobs now retain interesting
and failing records, exact provenance, logs and positive execution census.
The differential waits for native producers and downloads this run's artifacts.
The reporter stays once in fuzz.yml through a YAML anchor, preserving its
checkout-failure fallback. Its test harness reads the bounded inline block.
The compute controller has no repository-variables API request or extra token.

Exact-file validation output:

```text
python scripts/test_fuzz_retention.py -v
Ran 8 tests in 0.403s
OK
python scripts/test_fuzz_compute.py -v
Ran 6 tests in 2.465s
OK
python scripts/test_fuzz_report.py -v
Ran 21 tests in 1.496s
OK
```

Actionlint v1.7.12 emitted no errors after correcting the job-env context.
Zizmor v1.30.1 --offline emitted the following unsuppressed result (exit 1):

```text
WARN ... one or more inputs contains YAML anchors
error[dangerous-triggers]: use of fundamentally insecure workflow trigger
workflow_run is almost always used insecurely
audit confidence -> Medium
17 findings (16 suppressed): 0 informational, 0 low, 0 medium, 1 high
```

No suppression or audit configuration was added. Existing default-persona
suppressed findings are reflected in the tool's raw summary. Root will
adjudicate the explicit trust boundary before publication. The tool documents
its anchor support as beta, so this audit is not a runtime/security proof.

## Decisions and why
- Single compute-mode input: legacy default, dedicated-once, dedicated-continuous.
  The workstation's unchanged 03:17 task supplies legacy; its two-job cap and
  CI_RUNNER_LABELS hosted fallback remain. A small routing control job uses CI
  ownership, while every dedicated fuzz workload requires the dedicated group.
- Dedicated configuration requires master, barn-bug-discovery group,
  FUZZ_RUNNER_POLICY_VERIFIED=true and JSON FUZZ_RUNNER_LABELS containing
  self-hosted and barn-fuzz. FUZZ_MAX_PARALLEL accepts only two or four.
  The verified marker records root's policy proof; group policy enforces routing.
- Public-only isolated GOCACHE and cleared BARN_FUZZ_CORPUS prevent borrowing
  private runner-local seeds. Retained exact Go records and provenance are
  capped at 256 MiB per target; exhaustion fails without deleting findings.
- Successful campaign logs remain for census and artifact evidence under
  success-* names, while failure reporting consumes only run-* logs.
- One native matrix then one differential job provides a real producer barrier.
  Per-target caches bridge cycles/hosts; immutable run artifacts bridge jobs.
  Raw VM/compiler source records are consumed now. Structured choice decoding
  remains the other oracle track, without pretending string decoding handles it.
- Completed-event controller uses a new run's repository-variable snapshot.
  It requires trusted master, source event workflow_dispatch, exact continuous
  run title, matching source repository and successful conclusion. It checks
  stop/config, rejects a newer dispatch or any active/pending source dispatch,
  and submits at most one successor through GITHUB_TOKEN workflow_dispatch.
  Source jobs exclude workflow_run; controller completions cannot replenish.
- Controller checkout and code are default-branch code. It downloads no
  artifacts and never checks out source/candidate heads. Master-only workflow
  admission remains root's enforced runner-group responsibility.
- A stopped switch prevents eligible new controllers; immediate stopping also
  requires cancelling already-running controllers/campaigns. No claim of
  atomic interruption or runtime refresh inside an already-running controller.

## Surprises
Keeping the reporter in a checked-out .cjs file would lose the existing
checkout-failure producer. That untracked extraction was removed; GitHub's
documented YAML aliases share the original owner without this regression.
The original docs incorrectly said VM execution never resumes and no structured
generator exists; the source-backed descriptions are corrected.
The native real-log test initially stripped the wrong timestamp prefix; that
test adapter was corrected against the actual two-tab fixture format.

Repository-ci SKILL.md Defaults requires actionlint and zizmor and says
"Review findings rather than adding blanket suppressions." Workflow integrity
allows workflow_run with "an explicit trust boundary" and prohibits candidate
code/artifacts controlling a privileged reporter. Root read the same rule and
requested the generic trigger finding remain visible for source review.

Primary sources read:
- https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_run
- https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow#triggering-a-workflow-from-a-workflow
- https://docs.github.com/en/actions/reference/workflows-and-actions/reusing-workflow-configurations#yaml-anchors-and-aliases
- https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-variables
- https://docs.zizmor.sh/usage/#yaml-anchors

The completion docs specify default-branch execution, conclusion filtering and
a three-level workflow_run chaining limit. Token dispatch is documented as an
exception that starts a new run. Same-workflow completion delivery is not
explicitly guaranteed there; it must be proven live. Existing workstation
scheduler was inspected: it owns only the preserved interactive 03:17 dispatch,
not a fresh-variable dedicated controller. No additional credential is added.

## Next step
Root must review the diff and warning, integrate the separately merged oracle
completion/reporting change, and validate the combined exact head through PR CI.
The other worker's new panic regression must remain when updating this branch.
The PR body draft is .tmp/pr-body.md. No push/post/dispatch/merge occurred here.
Live runner policy, trigger delivery, stop-switch behavior, full dedicated
cycle, artifact/corpus restore after restart, second-cycle replay, CPU/wall
census and actual bug discoveries are unverified. GitHub artifacts expire after
30 days and caches may be evicted; confirmed findings must be committed with
their regressions before expiry.
