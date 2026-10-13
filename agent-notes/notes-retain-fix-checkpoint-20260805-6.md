# Retain/fix checkpoint

- Conformance PR 66 merged with a normal two-parent merge; Toast/Barn delete-verb boundary case retained.
- Barn PR 109 remains draft while genuine Toast `$2x$` behavior is added; issues 72 and 107 remain open.
- Barn PR 110 remains draft; delete-verb authority repair is independently approved; issue 102 remains open.
- Barn issue 111 and draft PR 112 isolate the default-profile CI repair to `.github/workflows/ci.yml`.
- PR 112 selects the same 11,584 tests as the unfiltered packaged suite, supplies `Test.db` and startup DB roots, and runs only `barn-linux-testdb-outbound-on`.
- Separate workers own cyclic-GC, bcrypt `$2x$`, and managed-runner lifecycle repairs.
- Do not merge a red Barn PR. Audit exact files/commits/closing phrases/checks and use normal merge; verify two-parent merge commits.
