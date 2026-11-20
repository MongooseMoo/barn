# Current conformance audit checkpoint

## Findings and observations

- Barn HEAD and remote master are `aa82abf05a6a23783ba69105d0c393f926304a13`.
- The exact required WSL Toast oracle exists at `/root/src/toaststunt/build-release/moo`, is executable, and is built from Toast source commit `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
- The managed harness makes a temporary database copy and preserves checkpoint state across managed restarts; the packaged database hash matches Barn `Test_conf.db`.
- A live orphan Toast process, two Windows WSL relay processes, listening port 34046, and its `moo_conformance_*` temporary directory remain after the controlling test process exited. Current direct-child termination is insufficient for WSL process trees.
- Barn CI and `scripts/run-conformance.ps1` omit profile manifests, `--server-db-dir`, `--fail-on-unexpected-skip`, strict markers, and the exact execution ledger used by the stronger Toast CI workflow.
- The supported Barn profiles require a Linux `./barn`; no such local binary exists. Available Windows Barn binaries are diagnostic against the Linux WSL Toast profile.
- Profile registry validation is not enforced on normal Barn startup, generated manifests always claim `supported`, and the harness profile gate is opt-in and checks only a subset of identity/runtime/features.
- Retracted: a fail-open skip/cache observation came from the stale local `aa312` checkout, not current remote main. Exact remote main `d7eeee` uses fail-closed probes, parsed skip conditions, and cache reset; only exact-current source is valid evidence.
- The local harness checkout is stale and divergent (`ahead 16, behind 43`), so current remote truth is being checked through GitHub at main SHA `d7eeee341a51aa4a25ad46033ddd2c72e47946ac`.
- Current main CI run `30982938666` is conclusively red after rerun: `Toast / 32bit-outbound-on` failed twice with the same eight `slice()` expectation failures (`8 failed, 11476 passed, 90 skipped`), and `Full suite` failed. The PR run `30982355955` was green (`11484 passed, 90 skipped`) using the same tree, pinned Toast commit, runner image, and build flags.

## Blocker

The path is not ready for repeated Barn-vs-Toast issue work: required current-main CI is red, WSL teardown leaks live state, there is no single strict managed Barn-vs-Toast command, and the supported local runtime pairing is unavailable.

## Next action

Issue a `NO-MERGE` / not-ready verifier verdict with exact current-source, live-process, and CI evidence. Explicitly retract the stale fail-open finding. Do not launch Barn or Toast and do not mutate runtime or tracked files.
