# Milestone 1A: fail-closed Mongoose profile comparison

Work only in `C:/Users/Q/code/moo-conformance-tests`. Before editing, read that repository's `AGENTS.md` if present, verify branch/HEAD/tracked status, and inspect the existing profile gate and tests.

Authoritative research is in:

- `C:/Users/Q/code/barn/plans/barn-toast-mongoose-convergence-workstreams.md`
- `C:/Users/Q/code/barn/docs/reports/mongoose-phase0-conformance.md`
- `C:/Users/Q/code/barn/docs/reports/mongoose-phase0-barn.md`

Implement the smallest direct change to the existing profile gate. Do not add a profile object, helper layer, adapter, interface, or new comparison surface.

Required behavior:

1. Require and compare `option.PROMOTE_NUMBERS` alongside `option.OUTBOUND_NETWORK`.
2. Require and compare `database_checksum` alongside `database_fixture` and `runtime_os`.
3. Require both option values to be JSON booleans so values such as integer `1` cannot compare equal to `true`.
4. Require both oracle and target manifests to declare an accepted, non-unsupported support status; derive the accepted vocabulary from the repository's existing manifest contract rather than inventing a new one.
5. Keep the gate optional for ordinary runs when no oracle manifest is supplied; the named Mongoose command will supply it later.

Add focused unit rows proving matching promotion is accepted; missing promotion on either side, promotion mismatch, and non-boolean option values are rejected; missing/mismatched checksum is rejected; missing or unsupported status on either side is rejected. Preserve existing outbound, fixture, runtime-OS, unsupported-target, and JSON-loading coverage.

Use `uv run pytest` for tests. Run the focused profile-gate tests, then the repository's proportionate static/full test gate if documented and reasonably bounded. Run `git diff --check`. Touch only the existing gate module and its direct test file unless a repository instruction forces another exact file. Do not stage or commit. Do not launch Barn or Toast. Report the pre-edit state, exact changed paths, tests, and any unrelated failures.
