# PR 48 adversary review checkpoint

## Scope and state

- Read-only review target: `C:\Users\Q\code\.worktrees\moo-conformance-all-toast-zero-skips`.
- Exact local and PR head: `fc85ab4f5d33d67f731a1352597d8ed91212e2b4`.
- No reviewed-worktree files have been edited, staged, committed, or pushed.
- PR 48 remains open. Managed run `31018225886`; job `92347843179` (`Toast / 64bit-example-outbound-on`) failed while building the pinned Toast source after `#define EXAMPLE 0` was changed to `1`.

## Current findings

- NO-MERGE at this head: required managed Toast CI is not green.
- The new example profile mutates `toaststunt@aecc51e.../src/extensions.cc` before building. The pinned source itself describes that block as example code, disabled because it is not generally useful, and the live job proves the example block no longer compiles against the pinned tree (`panic` undeclared and two invalid `void *` conversions).
- Further source patching would create a conformance-owned Toast fork rather than test the pinned Toast oracle. Preferred direction is to remove the example profile and treat `read_stdin` as absent from the unmodified pinned Toast runtime: remove positive `read_stdin` conformance identities or replace the one useful surface claim with an exact absence assertion. Do not retain positive-named identities that assert the opposite behavior merely to preserve counts.
- `builtins/math.yaml::minint_modulus_edge_case` is a false green: lines 1179-1183 now skip on 32-bit and execute `-2147483648 % -1` only on 64-bit, where the operand is not minimum integer. The identity/comment still claim a 32-bit minint edge.
- `language/promote_numbers.yaml::list_membership_promotes_integer_to_float` now expects `0`; its identity says promotion while its assertion proves non-promotion. Rename/remove/correct rather than preserve an inverted identity.
- Provenance gap: `toast-conformance.yml` records the pinned commit in `ADMISSION_CONTEXT`, but the example source patch is not hashed into that context/profile identity. The profile manifest calls the result `toaststunt` and `supported` based on environment declarations, not a source/build attestation.
- Exact-union ledger logic is directionally aligned: each report must have the exact candidate identity surface, failures/errors fail, and every identity must pass in at least one profile. However the Toast workflow recomputes the expected surface from the candidate itself and does not bind the quality collection output to the ledger; selective identity deletion can shrink both sides unless intentionally governed as a reviewed semantic deletion.

## Observations still being checked

- Remaining YAML changes mostly appear to repair fixtures while preserving the stated behaviors (GC inherited-property setup, anonymous stack verbs on an anonymous child, verb wildcard code assignment by index, and increased tick budget for `tostr_limit`).
- Need final status/log refresh after run completion and exact line citations for all final findings.

## Blocker and next action

- Blocker: example profile cannot build pinned Toast, so it cannot supply JUnit evidence and the required `Full suite` gate cannot pass.
- Next: finish the false-green/provenance audit, refresh live check status, and report concise NO-MERGE findings to the parent agent.
