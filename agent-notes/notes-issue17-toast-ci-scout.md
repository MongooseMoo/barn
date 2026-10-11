# Issue 17 Toast CI scout checkpoint

## Scope

- Read-only design review for making GitHub Actions execute every current packaged conformance case against pinned Toast.
- Inspected current code/config/live GitHub state only; did not read repository Markdown or manually launch Toast/Barn.

## Current findings

- Current `origin/main` is `a031527` (PR #21 merge); active worktree is `C:\Users\Q\code\.worktrees\moo-conformance-issue-17` on `agent/issue-17-run-all-toast` with no branch changes.
- `.github/workflows/toast-conformance.yml` has one Ubuntu job and one pinned Toast build (`aecc51e9449c6e7c95272f0f044b5ba38948459e`).
- The job runs `pytest --pyargs moo_conformance` once with Toast `--outbound`, then asserts only aggregate JUnit counts: 11,566 tests and 138 skips. It does not prove that every node ID executed.
- The harness explicitly allows literal YAML skips and matching `skip_if` skips under `--fail-on-unexpected-skip`; therefore a green aggregate count is not full execution proof.
- Profile-sensitive cases exist for `option.OUTBOUND_NETWORK` (enabled and disabled cases) and `option.PROMOTE_NUMBERS`; current workflow only exercises outbound-enabled runtime state and provides no profile manifest.
- Toast pinned source compiles `OUTBOUND_NETWORK` in (`#define OUTBOUND_NETWORK 1`), allowing runtime `--outbound` / `--no-outbound` profiles. No `PROMOTE_NUMBERS` symbol was found in pinned source grep so far; exact behavior still needs confirmation from current code/test metadata and live CI evidence.
- Pytest node IDs are deterministic from `relative_path.stem::test.name`, embedded in `test_yaml_conformance[...]`; duplicate filename stems across directories need checking because IDs omit directories.
- Capability `provides` / `assumes` reorder within one session and can create runtime skips if providers fail; naive sharding could separate providers from consumers and invalidate coverage.

## State / blocker

- No external blocker. A mandatory checkpoint required this uncommitted note before further inspection.

## Next action

- Enumerate exact collected node IDs and metadata safely via collection/static parsing, classify literal skips and profile-conditional cases, inspect live Actions artifacts/logs, then recommend the minimal matrix and fail-closed set-union accounting while preserving capability dependencies.
