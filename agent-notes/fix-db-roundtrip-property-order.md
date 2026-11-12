# fix/db-roundtrip-property-order

## Goal
Fix FuzzReadDatabase "round-trip verification failed: object #0 property
definitions/order differs" (issue #722; inputs 5cf47a2623862ea6,
1f68d455afcf5ef4, 0c6e2a923f6acf73, db17a3cfac13d98f, each from the issue
comments' base64 records, sha256-checked).

## State
Done on the branch: the four inputs are in
db/format/testdata/fuzz/FuzzReadDatabase/ (all fail on master, pass here).

## Cause
Both property-definition readers (reader_v4.go and reader_object.go, the
propDefCount loop) removed the newline and then one trailing "\r". The writer
(writer_object.go writeProperties -> writer.go writeString) writes the name and
"\n". A name read from "x\r\r\n" became "x\r", was written as "x\r\n" and read
back as "x": each checkpoint dropped another trailing carriage return.

## Decisions and why
Toast's dbio_read_string (src/db_io.cc) removes only the trailing '\n';
property names come through dbio_read_string_intern (src/db_file.cc). The shared
readPropDefName (reader_helpers.go) now does the same, so the written line reads
back unchanged. The round-trip check was not changed.

## Surprises
Other Barn string readers (object names via TrimSpace, readLine and string
values via TrimRight "\n\r") also differ from Toast's newline-only rule but are
idempotent, so the fuzz check does not catch them. Out of scope here.

## Next step
Push and open the PR (Fixes #722).
