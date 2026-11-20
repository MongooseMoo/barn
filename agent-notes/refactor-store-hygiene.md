# refactor/store-hygiene (db/store Wave 2)

## Goal
Wave 2 of plans/db-store-beauty.md, approved by Q ("make it so"): comments, dead
code, verb-cache counters and tests in db/store, plus `go test -race ./db/store` in
CI. No behaviour change. Brief from team-lead (2026-10-25), findings in
agent-notes/db-store-review.md (V1-V4, C1-C3, T1-T5).

## State (2026-10-25)
Done and committed on refactor/store-hygiene, rebased onto origin/master 5a61c4c8
(B5 #498 merged; no conflicts). Not pushed; no PR opened (team-lead opens it).
PR body draft: session scratchpad pr-wave2-barn.md.
Commits: db419586 comments, bd7ff769 dead code, 4ca4ce9b verb cache atomics,
36d21e37 review tests filed by area, adb1a553 race test checks the value,
7c3a0210 slices helpers, 82dd7765 CI race step (+ this note).
Final local run after rebase: go build ./... ok; go vet ./db/store ./db/format ok;
go test -count=1 ./db/store ./builtins ./engine all ok. CI is the real gate.

## Decisions and why
- builder.go:38 and :54 are `// --- section ---` headers over groups of one-line
  setters, not doc comments; left detached on purpose.
- Delete only symbols with zero callers, or whose only callers are tests of that
  symbol: HasLocalProperty, IsPropertyDefinedOnObject, ResetInheritedProperties,
  InvalidateAnonymousChildren + invalidateAnonymousChildrenLocked (0 callers);
  Verb.SetCode (its own test and duplicate table rows only). Test-only symbols used
  as helpers by other tests are listed in the PR body, not deleted (brief: don't
  rewrite unrelated tests).
- DEBUG-PROPWRITE stays: debugValidation also gates store_hot_read.go logging.
- Verb cache stats: only clears>0 and misses are ever exposed, so one atomic word
  (flag bit + miss count) replaces both fields and s.mu; keeps the mutex's
  linearizability (two separate atomics would not: a clear between Consume's two
  swaps could drop misses).
- TestCOWSameObjectCommitsSerialize now increments with retry (16x50) and checks
  the total; shown red under `BARN_ELLE_FAULT=property-reads go test -tags elle`
  (counter = 68 after 800).
- Race step checked with two concurrent copies in WSL Debian: both ok, 12.4s/12.3s
  test time, 40s wall.

## Surprises
- deadcode (x/tools v0.38.0) misses exported Store methods (reported nothing for
  InvalidateAnonymousChildren, which has zero callers); grep census used instead.
- store_cow.go's comment that coarse writers mutate published images in place is
  false: the coarse applier republishes (republishForMutation) first.
- 6 orphan comments at file ends, beyond the blank-line-detached ones.
- WSL git cannot read a Windows-created worktree's .git file; `go test` there
  needs -buildvcs=false.

## Next step
None for this agent. Team-lead: push, open the PR from the draft, read CI.
