# v4 bad object label wording

## Goal
Make Barn's v4 numbered-object label rejection log Toast's text,
"READ_DB_FILE: Bad object #<record>.", so the moo-conformance-tests suite
startup_object_label_v4 (branch test/db-object-labels) passes on Barn.

## State
Branch fix/db-v4-bad-object-wording off origin/master (after PR #624).

## Decisions and why
- Toast db_file.cc:920 (v4 object loop) logs
  `errlog("READ_DB_FILE: Bad object #%" PRIdN ".\n", i - 1)`; i counts from 1,
  so i - 1 is the 0-based record index, which is readObjectV4's slot.
- Only the label-mismatch error changes; other object read errors keep their
  text (not asked, not asserted by any suite).

## Surprises
staticcheck ST1005 rejects an error string ending in "."; Toast's text ends
"#2.\n". The error is "READ_DB_FILE: Bad object #2. (label #9)": Toast's text
is a substring (the suite asserts contains), and the check is not bypassed.

Toast also logs "DB_LOAD: Cannot load database!" (db_file.cc:1249) before
server.cc:2304-2305 exit(1). Barn does not; no suite asserts it, so not added.

## Results
go test ./db/format -count=1: ok. gofmt, go vet, staticcheck, golangci-lint on
./db/format: clean (0 issues).

## Next step
Edit, go test ./db/format, rerun the suites on Toast and this branch, commit
locally. No push/PR.
