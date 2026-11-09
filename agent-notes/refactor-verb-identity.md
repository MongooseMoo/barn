# refactor/verb-identity: Wave 3a stable verb identity

## Goal
Implement the approved Wave 3a design (session scratchpad
`design-3a-verb-identity.md`): a per-verb runtime id replaces `names[0]`
(`mapKey`) as verb identity in txn read/write sets; delete `Object.verbs` and
`mapKey`; drop `Verb.name` / `VerbView.Name`. Fixes B4 (shared first alias), case C
(delete drops sibling's staged code) and B11 (staged code lost across a same-task
rename). Q: keep per-verb reads keyed by id (Q1); drop Verb.name in this PR (Q2).

## State (done, not pushed, no PR)
- Barn `C:/Users/Q/code/barn-verb-identity`, branch refactor/verb-identity, rebased
  on origin/master cd004bcc (after #500 hygiene):
  - ab4ff150 store tests (5, red on master)
  - ac950059 verbKey{objID,id}; Verb.id from nextVerbID (builder.go:76,
    store_verbs.go:382); verbByID (object.go:359); census name via
    verbConflictName (store_txn_validation.go:154)
  - 01cf514e delete Object.verbs, mapKey, 7 fallbacks; HasLocalVerb compares
    names[0] over verbList; renumber and object_bytes walk verbList
  - 2967194e drop Verb.name / VerbView.Name; NewVerb(names, ...); readers join Names
  - Production +116 -253 in 19 files; tests +340 -173 in 66 files.
- Conformance `C:/Users/Q/code/mct-verb-identity`, branch test/verb-identity,
  7ae4327 on origin/main 2a15a1f: `builtins/verb_identity_same_task.yaml` (3 rows).
- Results at the rebased heads (Test.db sha256 1a3f23eb...): WSL Toast 4 passed;
  Barn master cd004bcc 3 failed 1 passed (#140: 2 failed); branch 2967194e 4 passed
  (#140: 3 passed). Go: build, vet, gofmt clean; db/..., builtins, vm, engine,
  server, command, parser, internal/dbtool tests ok.
- Drafts: scratchpad pr-3a-barn.md, pr-3a-mct.md.
- Benchmark skipped: CPU load samples 94, 96, 88, 100, 96% (2026-10-25).

## Decisions and why
- No zero-alias conformance row: Toast's add_verb (verbs.cc:188) and set_verb_info
  (verbs.cc:339) both call validate_verb_info, which returns E_INVARG for an
  all-space names string (verbs.cc:116-119). Toast cannot make such a verb.
- Colon row follows Toast: verb_info(o, "greet") on a ":greet" verb is E_VERBNF
  on the oracle, matching verbcasecmp (utils.cc:77-110, no colon rule).
- buildImageForVerbArgs lost its `detached` memo: it only deduplicated nodes
  shared between the map and the list.
- dbtool -verb-lookup's "Name:" line removed: it would repeat "Names:".
- A zero-alias verb (only Barn's set_verb_info with all-space names, B12) now
  reads as "" instead of its pre-rename name.

## Surprises
- Rebase onto #500: hygiene split db/store/review_test.go into area files and
  deleted TestVerbSetCodeRefreshesKey; the moved NewVerb calls (renumber_test.go,
  store_verbs_test.go) were fixed in the step 4 commit.
- The Edit tool dropped a trailing space in one replacement
  (command/dispatch_order_test.go); gofmt caught it.

## Not verified
- Benchmarks (machine busy). Elle verb-reads fault, -race: CI's job.
- Design's case D attempt-0 commit cause: still not diagnosed (test now passes
  all three attempts).

## Next step
Lead checks drafts, pushes barn branch and opens PR first, then the conformance
PR, then marks #140 ready.
