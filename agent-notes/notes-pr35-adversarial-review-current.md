# PR 35 adversarial review checkpoint

- Scope: read-only review of `moo-conformance-tests` PR #35 at exact head `a51dfdf9474583e81490812888d481aa47626d15`.
- Constraint: current code and live PR/diff only; no historical repository Markdown read.
- Diff: one file, `src/moo_conformance/_tests/builtins/slice_call_shapes.yaml`, with 17 additions and 36 deletions.
- Pinned oracle: workflow builds Toast commit `aecc51e9449c6e7c95272f0f044b5ba38948459e` in 64-bit outbound-on/off and `ONLY_32_BITS=ON` outbound-on profiles.
- Source finding: `bf_slice` accepts string indexes, then unconditionally evaluates `index.v.num <= 0` outside the list branch before reaching string map lookup. `Var.v` is a union of `const char *str` and `Num num`; `ONLY_32_BITS` makes `Num` `int32_t` while CI remains a 64-bit process. Thus the low 32 pointer bits drive success versus `E_RANGE`, which varies with allocation/ASLR.
- Replacement coverage: the 64-bit no-default case checks matching map lookup, duplicate matches, and omission of a missing key. Seven default cases check a matched value plus a missing key for int, float, object, string, error, list, and map defaults. Test identities are preserved for the 64-bit cases.
- Current concern under review: the schema supports only a single expected result, so the removed 32-bit assertions cannot directly encode the oracle's two accepted outcomes. Need finish checking whether omission creates a hidden execution-ledger or duplicate-baseline regression, and check final CI state.
- Current PR state at first inspection: draft; quality and three Toast profile checks in progress.
- Next action: inspect exact-head schema/runner/ledger behavior and completed CI, then issue PASS/FAIL with concrete findings.
