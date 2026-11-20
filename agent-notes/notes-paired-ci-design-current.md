# Paired candidate CI current-state audit

## Current findings

- `moo-conformance-tests` issue #18 is open and requires an explicit Barn commit plus explicit conformance commit, recorded evidence, no elevated trust for candidate code, linkable paired results, and fixture red/green proof.
- Barn `master` is `aa82abf05a6a23783ba69105d0c393f926304a13`; its only workflow is `.github/workflows/ci.yml`.
- Conformance `main` is `0615fc78544b806390906971942835e08a34d844`; its only workflow is `.github/workflows/toast-conformance.yml`.
- Barn CI checks out conformance `main`, does not record resolved candidate SHAs, does not use the profile manifests or strict unexpected-skip gate, uses floating action tags, and leaves checkout credentials enabled.
- Conformance CI is read-only at workflow level, disables persisted checkout credentials, pins actions and Toast to full SHAs, records evidence, and requires strict profile accounting.
- Both repositories currently allow all actions, do not enforce action SHA pinning, and default `GITHUB_TOKEN` permissions to write.
- Barn `master` is unprotected. Conformance `main` requires `Full suite` and `Harness, schema, duplicates, types, and lint`, enforces administrators, and requires branches to be current.

## State and blocker

- Read-only design audit is in progress. No implementation files have been changed.
- The main design constraint is that the trusted workflow definition must come from a protected default branch, while both candidate trees must be checked out by immutable commit SHA and executed with no credentials or secrets.

## Next action

- Inspect live harness CLI/profile inputs and current workflow event semantics, then report a minimal workflow/test design and fixture red/green delivery procedure to the parent agent.
