# Recurring native bug discovery

## Goal

Q authorized the written follow-up with "Make it so!": add native-only campaigns
to the existing compute workflow, preserve that selection through replenishment,
deliver an independent PR through normal merge, and activate recurring native
discovery after live acceptance. Preserve the full native-plus-Toast mode and its
comparison requirement, the private disclosure boundary, and unrelated work.

## State

Worktree `C:/Users/Q/code/barn-native-discovery`, branch `ci/native-discovery`,
starts from fetched master. Primary checkout has unrelated modified notes and
differential sources; none are included. No new campaign or setting change yet.
Live API returned two online idle dedicated runners, restricted to the master
fuzz workflow. Host service is active with CPU quota 8s, memory limit 20 GiB and
no swap. Continuous mode is unset.

## Decisions and why

1. Add `campaign` choice `native-and-toast` (existing default) or `native-only`.
   Keep the default run title compatible; give native-only a distinct suffix.
2. Validate the choice in the existing compute helper before routing. Native-only
   deliberately skips the differential job and its artifact download. Acceptance
   retains native execution, census, uploads, cache and reporting checks; it must
   reject an unexpectedly run comparison and explicitly log no comparison.
3. The controller recognizes trusted continuous titles and dispatches the same
   campaign selection. Stop switch, master-only/group restrictions, no duplicate
   dispatch and accepted-cycle requirements remain intact.
4. Extend the existing helper tests for selection, native-only findings,
   incomplete native work, full-mode differential failures and selection-preserving
   replenishment. Run only this test file locally. Audit changed workflows with
   actionlint and zizmor. PR CI supplies the full verification.
5. Merge normally after exact-head green checks and review-state inspection.
   Execute dedicated native-only finite campaigns, preserve evidence, verify
   guest isolation and retained corpus across the documented idle guest restart
   before enabling recurring mode. The guest restart is acceptance, not a fix for
   any discovered defect. No production service restart.

## Surprises

The repository-level runner API returned zero because these are organization
runners; the dedicated organization group API returned both actual listeners.
Prior assessment did not reverify guest SSH host-key trust or restart survival.
Those remain live acceptance work, not facts inferred from online status.

Guest management now works with strict host-key checking. The ED25519 key from
the trusted host's guest console matches the separately recorded local
known-hosts entry. Raw guest evidence: hostname `barn-compute`, virtualization
`kvm`, `nproc` 8, memory 16004 MiB, swap 0, both runner services active, runner
sudo denied. Mount inventory contains no host shares. Full network checks and
retained-corpus restart acceptance remain outstanding.

GitHub documents job conditions before matrix expansion. Removed the differential
job's one-entry matrix so its skipped name is explicitly ToastDifferential too;
the existing shared reporter takes target/package from each job's environment.

Focused validation:

```text
python -m unittest scripts.test_fuzz_compute -v
Ran 17 tests
OK
actionlint v1.7.12: exit 0
zizmor v1.30.1: 1 high dangerous-triggers (existing workflow_run); exit 1
```

The trigger finding is adjudicated from the controller source: checkout is from
master, no campaign artifact or candidate source is loaded, trusted master source
identity and accepted-cycle status are checked before dispatch, and only the
existing actions write permission is used. No suppression or threshold change.

## Next step

Publish the implemented workflow/helper/tests/docs as the single scoped PR.
Collect exact-head CI and review state, merge normally, then execute live guest
and native campaign acceptance before enabling recurring discovery.
