# Repair completion handoff

## Goal

Make automatic repair report the real queue verdict, use one managed conformance proof for both providers, and complete fresh companion branches after Barn has merged.

## State

Both queue paths wait for existing required checks on the validated head, then call GitHub's exact-head enqueue mutation. Pending checks neither arm automatic merge nor trigger removal. A native dequeue event reports the merge-group verdict and available failed check links. The existing three-engine proof runs from trusted code for both providers. The Codex publisher requests that evidence from an Actions job without its publisher credential.

The closed-repair path and Codex worker share suite completion. They verify the added-tests-only diff on the initial and every refreshed head, then wait for protected checks on the exact fresh head. An asynchronous merge request carries that head and an explicit no-bypass flag. The code verifies the request receipt and terminal verdict.

The focused completion, worker, and scope suites returned `OK`. The managed proof shell passed `bash -n`. Actionlint found identical diagnostics on the original and changed automatic repair workflow for its existing local-action spelling. The new proof workflow passed without diagnostics.

Proof observation uses bounded API calls against one accepted run receipt. Each request can renew the installation token. Cancellation and poll-deadline messages retain the run ID and last verdict, and only a successful terminal run reaches the artifact download.

## Decisions and why

The proof is bound to the Barn base, Barn head, and exact published suite head. Running the candidate in the publisher process would expose a different trust boundary from ordinary CI. Keep execution on the existing proof runner.

Preserve the suite's strict protection and required checks. GitHub's API describes the no-bypass flag in terms of repository rules. The code additionally requires the companion's GraphQL viewer permission to confirm that the credential cannot bypass branch protection. With a bypass-capable or unknown credential, report the missing protected credential explicitly.

## Surprises

GitHub exposes an exact expected head on the auto-merge mutation and the rejected merge-group commit on its dequeue timeline event. The branch update API acknowledges a request before changing the head. The companion PR list response omits the changed-file count, so fetch the individual PR before checking its files.

The manually refreshed suite companion #301 reported `MERGED` at 10:21:52 UTC. Its strict requirements and fresh CI corroborate the branch update. The suite's admin-enforcement endpoint still returns `false`; GitHub must report no bypass capability for the merge credential.

The verifier caught an installation-token lifetime mismatch before publication. The previous two-hour `gh run watch` inherited one token, including when it had less than an hour remaining. The regression failed on that watch and returned `OK` after it was replaced with bounded read calls using the refreshable publisher client.

The verifier also found that persistent auto-merge could admit a newer head than the one proved, and that a refreshed companion could skip the allowed-file review. No automation path now persists auto-merge. An existing manual request is preserved and an armed stale branch is left for its owner to refresh. The branch-details read exposes the same required check contexts and app IDs without the administration permission needed by the dedicated protection endpoint.

Independent verification found that the long managed-proof watcher retains one installation credential for the entire subprocess. GitHub installation credentials can expire while the proof remains queued or running. The verification verdict is recorded in the parent workspace report.

## Next step

The exact-head follow-up's focused worker and completion checks returned `OK`. Inspect its merge against the independent authorization branch before publication. The parent owns publication and the exact-head CI verdict. Verify installation permissions, an actual protected merge receipt, and a real proof job before claiming runtime acceptance.
