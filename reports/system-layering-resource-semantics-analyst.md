# Resource semantics analyst review

## Scope and state

Reviewing actual open Barn PR #639, Correct file scan cursors and cached output
capacity. The published head matches the clean detached verification worktree.
Future uncommitted frame/checkpoint/hash/startup changes in the campaign tree
are excluded. No source edits, local tests/builds or publication are performed.
The legacy gauntlet instruction path is absent; the installed gauntlet skill
was resolved from its advertised root and read instead.

## Source inspection

Published Go patches and both bounded coder reports have been read. The two
scan builtins now use the existing irreversible-effect policy; grep restores
unread buffered bytes before returning its first match, and line counting
retains a pending-line bit across buffer fragments. Capacity follows the
existing pending snapshot/session cache and converts to signed 32 bits before
positive minimum clamping. The completed bounded review found the catalog-pin
CI blocker below; no other actionable source defect was established.

## Finding: required catalog contract check remains stale

The two intentional descriptor changes in builtins/fileio.go make the required
unit job fail. TestNativeCatalogPreservesHistoricalContract pins every catalog
policy column, but its expected digest/comment have not been reconciled with
file_count_lines and file_grep becoming Irreversible. The published diff changes
those two Effect values without changing descriptor order or signatures.

Raw completed job log (job 128380122006):

```text
--- FAIL: TestNativeCatalogPreservesHistoricalContract (0.00s)
catalog_contract_test.go:42: historical native catalog changed: 1c65ff23ae88787b06c0a2fdb3baaf7d8bdd89d252e10b2f39ff20313e47d4e3
FAIL github.com/MongooseMoo/barn/builtins 10.737s
```

This blocks merge. Verify the complete catalog delta is only the two intended
effect policies before updating the existing pin and its explanation. Keep the
check and the new refused-boundary regressions; do not bypass or loosen them.
No other unit failure appeared in the completed job log. No source repair was
performed by this analyst.

The exact complete fileioDescriptors function was compared between the
published base and head. Removing only the two new annotations from the head
makes the whole function byte-identical, preserving names, signatures, order,
visibility and capabilities. Catalog aggregation, metadata builders, registry
installation and EffectPolicy enum are unchanged in the published diff.

```text
descriptor_function_identical_after_only_two_effect_removals=True
metadata_builder_diff_exit=0
```

This is an exact source-delta proof, not a locally executed descriptor dump.

## Remaining source inspection

No other concrete implementation defect has been established. Scan operations
retain the position mutex and native E_FILE outcomes; first-match seek uses the
reader's unread byte count before returning. Count treats buffer fragments as
one pending line and counts the final unterminated line only at EOF.
The effect wrapper refuses before invoking cursor work. Capacity defaults,
initial session state, collected snapshots and publication all carry the new
field; the existing pending snapshot takes precedence over the locked cache.
Manual partial snapshots occur only in tests in the inspected source inventory.

Raw oracle XML inspected directly:

```text
toast-file-scan-v3.xml: tests=5 failures=0 errors=0 skipped=0
toast-acceptance-v2.xml: tests=4 failures=0 errors=0 skipped=0
toast-capacity-width.xml: tests=2 failures=0 errors=0 skipped=0
barn-file-scan-before.xml: tests=4 failures=1 errors=0 skipped=0
barn-acceptance-before.xml: tests=3 failures=2 errors=0 skipped=0
```

## Bounded result and next step

One actionable blocker: reconcile the existing full-catalog pin/comment with
the verified two-policy correction. No other production correctness, race or
error-path defect was found in this reviewed change. Parent owns repair and
new exact-head CI. Full conformance and pinned isolation results were pending
at the captured check snapshot. The exact-head managed comparison being run
by the parent was not claimed as completed by this analyst.

No local tests, builds, benchmark or race instrumentation were run. No measured
performance or isolation claim is made. No source edits or publication occurred;
only this report and the required shared task-note checkpoint were written.
