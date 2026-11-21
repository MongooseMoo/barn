# test/fork-line-corpus

## Goal
Prove issues #910 #912 #913 #915 #920 #921 #925 #926 (FuzzCheckpointResumeEquivalence,
restored queued fork numbered its lines from 1) fixed on master after PR #922, by
adding each issue's failing input to the corpus and running it.

## State
Done. All eight inputs committed under
vm/testdata/fuzz/FuzzCheckpointResumeEquivalence/. No Barn source change.

Each input was decoded from the issue's base64 block; size and sha256 match the
values stated in the issue.

Results, `go test ./vm -run 'FuzzCheckpointResumeEquivalence/<id>$' -count=1 -v`
on the branch base (master at merge of #964):

| Issue | Input | Result |
|-------|-------|--------|
| #910 | 030ef45ff23bc9d0 | SKIP (restored fork from verb line 11 is suspended) |
| #912 | 043a998db29f0d7a | SKIP (verb line 9) |
| #913 | 04c8ca7f42b9828d | PASS |
| #915 | 09506cdbc9da7f48 | PASS |
| #920 | 11a3f5b3534a90d9 | SKIP (verb line 9) |
| #921 | 11bf0126bad1533b | SKIP (verb line 11) |
| #925 | 1ee2be5844b14cca | PASS |
| #926 | 2379cd94b28779c6 | PASS |

These results match what #922's description predicted for #910, #912, #913,
#915, #920 and #921.

## Decisions and why
- The SKIPs are the deliberate skip at vm/checkpoint_resume_test.go:435-436 that
  #922 added. That case is a queued fork, restored with Origin.FirstLine > 1, that
  is suspended and checkpointed again in Toast form. I checked Toast's source
  (origin/master) for that case:
  - first_lineno is set only by program.cc:33 (the default, 1) and tasks.cc:2069
    (read_task_queue, when a queued fork is read). A grep of every src/*.cc and *.h
    found no other assignment.
  - write_activ (execute.cc:4005) writes dbio_write_program(a.prog) and has no
    first-line field. read_activ (execute.cc:4045) is not one of the places that
    set first_lineno.
  - So Toast also numbers such a task's lines from 1 after the second reload. The
    issue's difference (line 15 vs 4 in #910) is behaviour where Barn matches
    Toast, not a Barn defect. Nothing to fix, so I kept these inputs as coverage.
- I did not run the managed WSL Toast oracle. The claim rests on reading Toast's
  source only.

## Surprises
- The skip applies to the whole case. When any suspended frame is such a fork, the
  other tasks in that input are not compared either (checkpoint_resume_test.go:435).
  So the four SKIP inputs check nothing beyond "no panic before the checkpoint".
  That is #922's design; I left it unchanged because it is outside this job.

## Next step
Orchestrator pushes branch test/fork-line-corpus and opens the PR from PR_BODY.md.
