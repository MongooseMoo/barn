# Fix nightly fuzz #645: VM frame metadata decoder panics on wrong field types

## Goal
Fix the FuzzReadDatabase panic from the 2026-11-07 nightly run (issue #645,
run 42363293768) and land it. Q asked to unfuck the nightly fuzz and land the
actual fixes.

## State
db/format/reader_vm.go decodeVMFrameMetadata now calls checkVMFrameShape
before decoding. The failing input a988c15f45016ac4 is committed under
db/format/testdata/fuzz/FuzzReadDatabase and passes; it panicked at
reader_vm.go:366 before. vm_frame_shape_test.go replaces each checked field
with a wrong shape; it panics without the fix and passes with it.

## Cause
The decoder read fields of the database-supplied frame list with Get,
Elements and Str, which panic on another type (types/value.go payloadTag
index), and looped to Len, which counts a string's bytes or a map's entries.
The fuzz input made field 3 (constants) a non-list. Fields 2, 4, 5, 7, 9, 11,
14 and the recycle id lists 18.2-18.4 had the same flaw.

## Decisions and why
One shape check over every field read element by element, not a fix of field
3 alone, so the next field is not the next night's panic. Int, Obj and Truthy
do not panic on other types and are left as they were.

## Next step
CI on the PR, then merge.
