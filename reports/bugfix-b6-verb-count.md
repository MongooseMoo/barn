# B6: verb-program count off-by-one (1949 vs Toast 1950)

## Status: FIXED (committed on fix/b6-verb-count, NOT merged)

## Unit test
db/format/verb_program_roundtrip_test.go: TestRoundTripPreservesEmptyVerbProgram.
Builds 3 verbs (real code / empty program via set_verb_code / never programmed),
round-trips, asserts the empty program keeps its entry and the never-programmed
verb does NOT gain one. PASSES with fix; verified FAILS with the old
len(Code)>0 gate ("HasProgram = false, want true").

## Setup
- Worktree: C:/Users/Q/code/barn-fix-b6, branch fix/b6-verb-count off e67f970
- Canonical copied to _b6_canonical.db
- Barn v17 output: _b6_barn_out.db (via db_roundtrip.exe)

## RULE ZERO — Toast confirmed (1950)
WSL: `~/src/toaststunt/build-release/moo toastcore.db /tmp/out.db -p PORT`
Log /tmp/toast_load.log:
```
LOADING: Reading 1950 MOO verb programs ...
LOADING: Done reading 1950 verb programs ...
PANIC-DUMPING: Writing 1950 MOO verb programs ...
```
=> Toast reads 1950 verb programs from canonical. CONFIRMED.

## Barn round-trip
db_roundtrip on _b6_canonical.db: SUCCESS (maxObj=126, players=6, objects=127).
Round-trip object/verb-count comparison passes. But that does NOT count verb
PROGRAMS in the v17 text section — need to load Barn's output in Toast to count.

## NEXT
- Load _b6_barn_out.db in WSL Toast, expect "Reading 1949 MOO verb programs".
- Diff the #obj:verbidx program headers in Toast's canonical-dump vs Barn's output.
- Find which verb program is dropped + why.
- Likely db/format/writer_object.go writeVerbPrograms / len(verb.Code)>0 gate.

## Barn output loaded in Toast — CONFIRMED 1949
/tmp/toast_barn_load.log:
```
LOADING: Reading 1949 MOO verb programs ...
VALIDATE Phase 1/2/3 clean (PANIC is just pkill signal 1, not a load error)
```

## MISSING VERB IDENTIFIED: #10:17
Header diff (grep `^#N:N$` from each v17 dump, comm):
- in Toast not Barn: `#10:17`
- in Barn not Toast: (none)

Toast writes `#10:17` followed immediately by bare `.` => EMPTY PROGRAM.
The verb #10:17 EXISTS and HAS a program slot, but its program is empty
(compiles to no-op). Toast still emits a verb-program entry (counts it).

## ROOT CAUSE
- Writer: db/format/writer_object.go:292 `if len(view.Code) > 0` — skips verbs
  whose code slice is empty/nil, dropping the empty-program entry #10:17.
- Loader: db/format/reader_object.go:347-364 readVerbCode reads header then
  lines until `.`; for an empty program codeLines stays nil and
  SetVerbCodeByIndex(idx, nil) is called. Barn loses the distinction between
  "verb has an (empty) program" vs "verb has no program at all".

The verb metadata count (per-object VerbList) does NOT tell which verbs have
programs — Toast emits one program entry per verb that has a program. In the
canonical DB, EVERY verb in the verb-metadata has a program entry (even empty
ones). So Barn must emit a program entry for every verb, OR track which verbs
had a program on load.

## FIX PLAN (verifying)
Need to confirm: in canonical, is #programs == total #verbs across all objects?
If yes -> Toast emits a program per verb unconditionally; writer should drop the
len()>0 gate (emit all verbs). Need to verify count matches 1950 = total verbs.

## Counts (verified via Barn loader on canonical)
total verbs=1954, withCode=1949, emptyCode=5.
Toast emits 1950 programs. So NOT one-per-verb.
The 5 empty-code verbs: #46:54, #10:17, #75:2, #122:7, #76:6.
Toast emits a program entry for ONLY #10:17 (special_action). The other 4 have
NO program at all (Toast omits them too). Confirmed by grep of Toast dump:
  #10:17 -> 1, others -> 0.

=> The distinction is "verb has a program (even empty source)" vs "verb never
   had a program". Barn's loader collapsed both to Code=nil. Must track a
   separate hasProgram bit.

## FIX (implemented, in progress)
Add Verb.hasProgram bool + VerbView.HasProgram + Snapshot.
- NewVerb: hasProgram = len(code)>0  (add_verb passes [] -> false; loader passes nil -> false)
- builder SetVerbCodeByIndex (loader): hasProgram=true (program entry existed in DB)
- store SetVerbCode / SetVerbCodeByIndex (runtime set_verb_code): hasProgram=true
- writer gate writer_object.go:292: `if view.HasProgram` instead of len(Code)>0
DONE: object.go Verb.hasProgram field + VerbView.HasProgram added.
TODO: NewVerb, View(), builder.SetVerbCodeByIndex, store setters, snapshot, writer.

## FIX APPLIED — files
- db/store/object.go: Verb.hasProgram field; VerbView.HasProgram; NewVerb sets
  hasProgram=len(code)>0; SetCode sets hasProgram=true; View() carries it.
- db/store/builder.go: SetVerbCodeByIndex sets hasProgram=true (loader path).
- db/store/store_verbs.go: SetVerbCode + SetVerbCodeByIndex set hasProgram=true.
- db/format/writer_object.go:~292: gate `if view.HasProgram` (was len(Code)>0).

## RESULT after fix (db_roundtrip on canonical)
- Barn v17 output header count: 1950 (was 1949).
- #10:17 present: yes (count 1).
- comm diff Toast-canonical vs Barn-output: BOTH directions EMPTY. Exact match.

## NEXT GATES
- Load Barn output in WSL Toast -> expect "Reading 1950 MOO verb programs" + VALIDATE clean.
- go vet, go test, go list -deps parser-free, conformance 3871/0/131.
- Add unit test for empty-program round-trip. Commit.

## GATES — RESULTS
- go build ./...: OK
- go vet ./...: 2 known (moo_client IPv6 fmt, vm/stack ReadByte sig). No new.
- go list -deps ./db/store | grep parser: EMPTY (parser-free preserved).
- go test ./...: db/store OK, db/format OK (with mongoose fixture copied in;
  TestLoadMongooseSnapshot only fails when untracked fixture absent in worktree —
  not a code regression; baseline master tree passes it w/ the file present).
  conformance test pkg fails = needs cow_py dir (known fixture/path, same on master).
- B6 GATE: WSL Toast loads Barn output -> "Reading 1950 MOO verb programs",
  VALIDATE Phase 1/2/3 clean. PASSED.
- Conformance suite (managed barn.exe): 3871 passed, 0 failed, 131 skipped. EXACT.
- No persistence regression: VALIDATE clean; db_roundtrip canonical SUCCESS.

## TODO
- Add Go unit test for empty-program round-trip.
- Commit on fix/b6-verb-count. Do NOT merge.

## Blocker: none.
