# Repair completion handoff

## Goal

Make automatic repair report the real queue verdict, use one managed conformance proof for both providers, and complete fresh companion branches after Barn has merged.

## State

The queue now validates the evidence and exact PR head before arming. Pending checks no longer trigger removal. A native dequeue event reports the merge-group verdict and available failed check links. The existing three-engine proof runs from trusted code for both providers. The Codex publisher requests that evidence from an Actions job without its publisher credential.

The closed-repair path and Codex worker share suite completion. The code checks Barn closed or merged, validates the companion diff, refreshes a stale suite head, and waits for native auto-merge to give a terminal result.

The focused completion, worker, and scope suites returned `OK`. The managed proof shell passed `bash -n`. Actionlint found identical diagnostics on the original and changed automatic repair workflow for its existing local-action spelling. The new proof workflow passed without diagnostics.

Proof observation uses bounded API calls against one accepted run receipt. Each request can renew the installation token. Cancellation and poll-deadline messages retain the run ID and last verdict, and only a successful terminal run reaches the artifact download.

## Decisions and why

The proof is bound to the Barn base, Barn head, and exact published suite head. Running the candidate in the publisher process would expose a different trust boundary from ordinary CI. Keep execution on the existing proof runner.

Preserve the suite's strict protection and required checks. An already-current green companion uses the normal merge endpoint only when the credential is subject to protection. With an administrative or unknown credential on a branch where protection excludes administrators, report the missing non-bypassing credential explicitly.

## Surprises

GitHub exposes an exact expected head on the auto-merge mutation and the rejected merge-group commit on its dequeue timeline event. The branch update API acknowledges a request before changing the head. The companion PR list response omits the changed-file count, so fetch the individual PR before checking its files.

The manually refreshed suite companion #301 reported `MERGED` at 10:21:52 UTC. Its strict requirements and fresh CI corroborate the supported path. The suite's admin-enforcement endpoint still returns `false`; ordinary API merge requires a known non-admin credential in that configuration.

The verifier caught an installation-token lifetime mismatch before publication. The previous two-hour `gh run watch` inherited one token, including when it had less than an hour remaining. The regression failed on that watch and returned `OK` after it was replaced with bounded read calls using the refreshable publisher client.

Independent verification found that the long managed-proof watcher retains one installation credential for the entire subprocess. GitHub installation credentials can expire while the proof remains queued or running. The verification verdict is recorded in the parent workspace report.

## Next step

Commit the verified change and inspect its merge against the independent authorization branch. The parent owns publication and the exact-head CI verdict. Verify installation permissions and a real proof job before claiming runtime acceptance.
