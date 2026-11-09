# #524: untrusted section and record counts

## Goal
Return parse errors for invalid counts and avoid allocating an input's unproven claimed size while preserving legitimate large databases.

## State
Base c537d6c952791b2fe49bcb9d7cba90102767a36f, branch fix/524-format-counts. Implemented locally; parent publishes. Separate from #522/#523 branches. Production diff is 111 added/35 removed lines across existing count owners and the renamed allocation bound; existing list test changes only follow the symbol rename. New reader_counts_test.go holds focused issue evidence.

## Decisions and why
Smallest repair: validate negative counts in their existing reader owners; reject top==MaxInt before top+1; cap initial slice/map capacity at the existing 4096 collection bound and append decoded items. Generalize the existing bound's name rather than add a second policy. Existing property slot capacity remains bounded by maxTrustedSlotCount. Roughly 100-150 production lines across existing readers, no new format/parser framework and no maximum database size.
Evidence will extend focused count coverage: negative transformations of tracked toastcore; direct record readers with negative and max-int truncated counts; successful records beyond initial capacity. No huge allocations, server startup or local CI suite.

## Surprises
The first focused run failed because I used an LF anchor for the tracked CRLF v4 fixture: `reader_counts_test.go:137: v4 fixture property-definition anchor missing`. Read-only byte probe returned `CRLF=True LFAnchor=False CRLFAnchor=True`. Corrected the test anchor/substitution to retain CRLF bytes. No production behavior was changed to accommodate the test.

## Next step
Parent and independent reviewer inspect the commit, then parent publishes and reads PR CI. No local CI suite, native oracle, or deployment performed.

## PR draft
Title: fix(db/format): validate counts before allocating reader records

Corrupt section counts could panic during slice allocation or request enormous capacity before any record was read. Reject negative section/record counts at their owners, reject frame tops whose top+1 would overflow, and bound initial allocation using the existing collection limit while appending decoded records. The limit is initial capacity, not a maximum database size. Existing list/map behavior is unchanged apart from naming the shared bound maxTrustedCount.

Focused coverage mutates tracked v17/v4 fixtures and writer-produced object/VM records, exercises negative and truncated max-int counts, and decodes 4,097 player/environment records without losing order. Complete v4/v5/v17 fixtures remain readable in those focused tests. Closes #524.

## Evidence and limits

```text
go test ./db/format -run '^(TestReadDatabaseRejectsNegativeSectionCounts|TestReadRecordCountsRejectNegativeAndTruncatedHugeCounts|TestReadVMCountsRejectOverflowAndUntrustedLengths|TestReadObjectPropertyCountsRejectInvalidLengths|TestReadRecordsGrowBeyondInitialCapacity)$' -v
read players: invalid player count -1
read finalizations: invalid pending finalization count -1
read queued tasks: invalid queued task count -1
read suspended tasks: invalid suspended task count -1
read interrupted tasks: invalid interrupted task count -1
read active connections: invalid active connection count -1
--- PASS: TestReadDatabaseRejectsNegativeSectionCounts (0.04s)
--- PASS: TestReadRecordCountsRejectNegativeAndTruncatedHugeCounts (0.00s)
--- PASS: TestReadVMCountsRejectOverflowAndUntrustedLengths (0.01s)
--- PASS: TestReadObjectPropertyCountsRejectInvalidLengths (0.00s)
--- PASS: TestReadRecordsGrowBeyondInitialCapacity (0.00s)
PASS
ok github.com/MongooseMoo/barn/db/format 1.361s
```

Then extended the positive-growth case to the ordered environment and strengthened negative-record assertions, and ran only the two changed tests:

```text
go test ./db/format -run '^(TestReadRecordCountsRejectNegativeAndTruncatedHugeCounts|TestReadRecordsGrowBeyondInitialCapacity)$' -v
--- PASS: TestReadRecordsGrowBeyondInitialCapacity (0.01s)
PASS
ok github.com/MongooseMoo/barn/db/format 2.201s
```

The max-int cases use 9223372036854775807 on this windows/amd64 host, plus 9223372036854775806 for frame/stack/env truncation. Frame snapshots are produced for parsing evidence only, not claimed executable native continuations. The full-fixture reads are toastcore.db, Test_fresh2.db and Broken1.db; their existing startup repairs still log warnings. This fix does not add a recursive-depth/global byte limit, change historic clocks/tasks, certify all malformed files, or promise any benchmark improvement. Allocation owners driven by decoded metadata lengths stay unchanged; NumLocals already has its existing bytecode.MaxLocals validation.
