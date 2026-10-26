# #517 reader design

## Goal

Deliver source-grounded design documentation only. Smallest deliverable: one roughly 100–150-line report and this note. No codec changes, publishing or new format support. Parent checks sources and publishes. Other workers own #513/#514/#516; preserve their work.

## State

Branch research/517-format-reader-design created from current remote master c537d6c952791b2fe49bcb9d7cba90102767a36f. Report complete at docs/reports/db-format-reader-design.md. Read exact issue #517, #515 report (pinned f9f843ce955f451dff9875b02a3a0f7a753ec197), current version readers/dispatch/helpers/task reader, #516 note and subsequently its complete report in the audit worktree. Coordinated with format_513. No implementation files changed or tests run.

## Decisions and why

Recommend deleting duplicated player reader and sharing the existing body (~20 net lines removed), retaining explicit v4/v5/v17 orchestrators. The v4/v5 common sequence includes unsettled task/clock/trailing-section contracts; a general declarative table would obscure actual version differences. A larger common object-first parser is discussed but not authorized or recommended as an automatic follow-up.

## Surprises

Both v4 and v5 currently ignore trailing connection data, although only v4 calls this out in a comment. Existing shared task helpers do not establish correct historical record decoding. Player body equality was inspected directly; an initial line-slice comparison included a trailing blank from one file and not the other, so its false result was not a semantic difference.

## Next step

Parent owns source verification/publication and the Q decision before implementation. Design reflects the #516 report's unresolved historical contracts and separates count repairs from helper consolidation. No claim of runtime compatibility or performance improvement is available. Audit report citation now pins 54d1b4f343b90a2017b83dd393d19047d585d854.

## PR draft

Title: docs: propose a bounded database reader consolidation

Body:

Documents actual v4/v5/v17 sequencing and recommends sharing only the identical player-list reader while historical task/clock/trailing-section contracts are resolved. Estimates deletions and ownership, explains why a declarative registry adds no demonstrated benefit, and separates refactoring from new format support.

Design deliverable for #517; depends on #515/#516 evidence. No codec implementation or runtime validation. Q chooses the design before implementation.
