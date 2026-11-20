# #526 exact database header

## Goal

Replace substring header dispatch with an exact switch on the three existing complete headers, retaining outer whitespace trimming and unsupported-format context. Smallest production change is roughly ten lines; extend existing reader tests with supported fixtures and header impostors. No new version support or parser framework.

## State

Branch starts at verified origin/master c537d6c952791b2fe49bcb9d7cba90102767a36f. Exact issue read. Header dispatch now matches the complete supported literal, preserving outer TrimSpace and existing error context. #522 touches separate loader cleanup and #525 separate WAIF comments/lifetime; no files are shared across worktrees.

Focused command: `go test ./db/format -run '^TestLoadDatabaseHeader$' -count=1`.
Before repair: numeric-suffix/text-suffix/missing-frame/trailing-text cases reported `header error = <nil>, want unsupported database format`; versions 40/50 entered players decoding instead of rejecting the header.
After repair: `ok github.com/MongooseMoo/barn/db/format 4.012s`.
Diff inspected: reader.go 6 insertions/5 deletions, reader_test.go 49 added lines. No full CI or oracle/server run.

## Decisions and why

Use complete literal headers because only 4/5/17 are supported. No integer parser or regular expression is needed to add no formats. Preserve useful unsupported-header error text. Test mutations of the actual toastcore fixture without inventing a version-170 format.

## Surprises

No dedicated header test exists in reader_test.go, so add focused coverage there. Existing declared-program test supplies a minimal v4 encoding; use that known old layout for the supported-v4 case.

## Next step

Parent reviews/publishes and reads CI. No implementation remains in this worker's scope.

## PR draft

Title: fix: require an exact supported database header

Body: Numeric prefixes such as version 170 previously selected the v17 reader because dispatch used substring matches. Match complete v4/v5/v17 headers, preserving outer whitespace handling and unsupported-format diagnostics. Focused cases cover supported layouts and malformed numeric/text/framing variants of the tracked toastcore dump.

Fixes #526. Validation: `go test ./db/format -run '^TestLoadDatabaseHeader$' -count=1` failed before the change and passed after it. Full checks belong to PR CI.
