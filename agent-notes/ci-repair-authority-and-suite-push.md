# Barn repair authorization and upstream conformance

## Goal

Require Barn writer authorization for human-triggered repairs. Run the managed Barn suite against each landed conformance revision, file confirmed failures, and launch the existing repair workflow.

## State

Implemented in the isolated `ci/repair-authority-and-suite-push` worktree. This change includes shared issue and queue authorization, the independent suite watcher and timer, the shared packaged run command, and the upstream run and reporter. Q authorized the commit, pull request, and merge. The live machine rollout is a separate operation.

- `python3 -m unittest test_repair_authority test_repair_worker test_conformance_watch` under Debian WSL returned `Ran 15 tests` and `OK`. The retry test kept one issue through an unconfirmed dispatch and reused the first confirmed repair run.
- The reporter read packaged CI artifacts without writes. Barn run 46129961680 returned `14408 packaged cases ran, 0 failed`. Run 45449314462 returned `14207 packaged cases ran, 5 failed` with the five exact signal-case IDs.
- Read-only GitHub checks with the local CLI returned `GitHub issue 1270 open writer dispatch authorized: True` and `GitHub open machine report (1211, True)`. GitHub Actions token behavior still needs a deployed test.
- Actionlint passed for `repair-authorize.yml`, `conformance-upstream.yml`, and `ci.yml`. ShellCheck, systemd unit parsing, and `git diff --check` passed. Zizmor reported two existing low `self-repository` advisories in the CI workflow and no medium or high findings.

## Decisions and why

- Check current GitHub collaborator roles for the opener and requester. Match automation by numeric identity and stage. Read the current label timeline for Codex queue authorization. Use current authorized comments as the issue evidence passed to providers.
- Reuse the existing App publisher and request a Barn Actions-only token from the watcher. Keep the timer separate from the repair worker. Save the suite checkpoint after a confirmed dispatch. Fetch and verify exact first-parent `main` revisions.
- Move the existing packaged conformance command into `scripts/run-packaged-conformance.sh`. Keep the CI step name and artifact paths. Keep test execution read-only and publish from a separate reporting job.

## Surprises

- Actionlint 1.7.12 rejects the existing `$/` references in the repair workflows. GitHub documents that syntax as the self-repository reference pinned to the workflow revision. Preserve it. The upstream actionlint support issue is [#760](https://github.com/rhysd/actionlint/issues/760).
- The existing packaged report has `test_capability_admission` with an `admission_context` property and `test_yaml_conformance[path.yaml::case]` names. The evidence reader checks those records and rejects setup or teardown errors, missing evidence, and zero executed packaged tests.
- App dispatch is verified historically. The always-on server-side publisher location remains unverified.

## Next step

After merge, an authorized deployment updates the running Codex repair worker to the same code. Confirm the always-on publisher host and deploy the reviewed files into its root-owned `/opt/barn-repair` checkout. The timer reads the existing `/etc/barn-repair/github-app.json` credential in place. Install the service and timer files with mode `0644`, reload systemd, and enable the timer. Confirm the initial suite tip appears in the timer journal with its Barn run ID. Verify the complete live authorization path, a green upstream run, and a controlled failure showing the issue, dispatch, and provider handoff. The workflow, worker, and timer have not been exercised together live.
