# Issue 14 and 16 adversarial audit

## Current state

- Audit target: full uncommitted combined diff in `C:\Users\Q\code\.worktrees\moo-conformance-issue-14`.
- Scope is read-only inspection of current source, tests, and GitHub issues #14 and #16.
- No source edits, staging, commits, pushes, or merges are authorized.

## Findings and observations

- Live issues require fail-closed requirement semantics (#14) and one canonical conformance implementation with exact entrypoint parity (#16).
- The current diff deletes `tests/test_conformance.py`, adds root `src/moo_conformance` collection, and uses full relative YAML paths plus expanded names for case IDs.
- `skip_if` now has a closed grammar for feature, builtin, and option presence/absence; requirement schema validation covers builtins, features, minimum version, and a fixed config-key set.
- Capability admission runs through a wizard-selected transport, validates returned value shapes, distinguishes exact absence from probe errors, and uses `server_version("options.ONLY_32_BITS")` with slash-path fallback for the width feature.
- Strict skip authorization compares the exact reported reason to exact reasons derived from the case and suite declarations.
- Potential review focus: admission leaves the shared transport selected as wizard. The runner appears to reselect explicit suite/test permissions before executing transport-backed setup and test bodies, but this must be verified through behavior tests rather than assumed.
- Two YAML changes remove an unsupported skip expression and make a 32-bit-only sentinel executable; their semantic scope still needs adversarial review.
- Focused verification passed 126 tests; the complete unit suite passed 174 tests; scoped Ruff passed.
- Both ordinary pytest and the packaged module collected 11,566 conformance cases, all IDs were unique, the ordered ID lists were byte-for-byte identical, and every ID included the YAML filename plus expanded case name.
- Full collection also proves the current 11,566-case YAML corpus validates under the new closed schema.
- Scoped mypy passes when the repository's untyped PyYAML import is disabled; an unqualified mypy invocation fails only because `types-PyYAML` is not installed.
- Concrete defect: requirement-name validation uses `^[A-Za-z][A-Za-z0-9_]*$`, but the supported feature model explicitly includes `64bit`. Thus `requires.features: [64bit]` is rejected even though `skip_if: feature.64bit` is accepted and `_has_feature()` has dedicated 64-bit behavior. This makes feature requirement semantics internally inconsistent.
- Strict runtime-skip authorization derives exact reasons from literal skips, `skip_if`, requirements, and implicit managed restart. Existing declared `assumes` dependency skips are not authorized; this is a possible scope gap because the capability subsystem intentionally emits those skips when providers are absent, failed, or unverified.
- Identity isolation was executed through the canonical function with an actual `YamlTestRunner`: admission switched to wizard for the dynamic probe, then the test runner switched back to programmer before executing the case body. The event sequence was wizard probe followed by programmer case execution.
- Exact strict authorization is incomplete for literal skips: `plugin.py` returns immediately when either literal skip flag is truthy, without comparing the actual runtime skip reason. An unrelated fixture/setup skip is therefore accepted on a literal-skipped case.
- The advertised root pytest command and packaged CLI converge on the sole remaining `src/moo_conformance/test_conformance.py` owner; no second non-Markdown execution owner was found.
- Diff scope is limited to entrypoint wiring, condition/schema/execution enforcement, focused regression tests, deletion of the duplicate owner, and two YAML repairs needed to eliminate an unsupported condition and make its profile sentinel executable. No whitespace errors were reported.

## Blocker

- The optional `ward set adversary` enforcement command is unavailable in this environment. The audit is continuing manually under the adversary protocol.
- The known `64bit` feature cannot be expressed in `requires.features`; this currently blocks merge.
- Literal YAML skips still authorize unrelated runtime skip reasons, so exact strict-skip admission is not fail-closed; this also blocks merge.

## Next action

- Return NO-MERGE to the parent with exact file/line evidence and preserve all source files untouched.
