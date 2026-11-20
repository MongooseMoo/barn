# Current Toast conformance checks investigation

## State

- Live `MongooseMoo/moo-conformance-tests` default branch: `main` at `7df5d1891a9849a10e3b5ec4f80235f1b5e03714`, the two-parent merge of PR #46.
- Current main push run `31016667522` is in progress.
- Remaining open PRs: #45 at `5c5132bf8f6f563ee236a1249aa42677c3bda1e5` and #22 at `13722f5492bce5c0bcfba63d225188ffd81dd76c`; both currently have green Toast workflows.
- PR #46's green run collected 11,588 items: one admission item and 11,587 packaged conformance items. Its profile union passed only 11,546 of 11,587 packaged cases; 41 were skipped by every Toast profile.

## Findings

- Current main stages `-m admission` and `-m conformance`; the marker deselections partition the full collection and do not themselves lose cases.
- The false-green mechanism is `ci/toast-never-executed.json`, passed to `moo_conformance.execution_ledger` by `.github/workflows/toast-conformance.yml`.
- The ledger treats only `passed` as executed, but explicitly accepts every never-passed case named in that 41-entry baseline if all profiles skip it with the exact recorded reason.
- `--fail-on-unexpected-skip` also accepts YAML `skip`, `skip_if`, suite `requires`, and managed-restart skip reasons, despite its help text claiming literal YAML skips only.
- The runner checks those skip mechanisms before `runner.run_test`, so the 41 cases never exercise Toast behavior.
- Baseline groups: 24 anonymous local-property GC, 5 missing `read_stdin`, 3 anonymous verb-stack, 3 disabled `PROMOTE_NUMBERS`, 3 wildcard verb matching, and one each for 32-bit minint modulus crash, missing `file_version`, and `tostr_limit`.
- PR #45: union 11,552/11,593, 41 reviewed skips. PR #22: union 11,549/11,590, 41 reviewed skips.

## Blocker

- A zero-skip fail-closed gate will immediately expose 41 existing Toast incompatibilities/crash cases; making the gate truthful and making all cases pass are separate pieces of work.

## Next action

- Report the smallest fail-closed fix: remove baseline authorization from the workflow/ledger and require `never_executed` to be empty, then make each of the 41 cases executable through suitable Toast profiles or explicit executable assertions instead of pre-run skips.
