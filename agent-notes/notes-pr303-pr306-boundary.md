Checkpoint 2026-09-16: User authorized reviewing #306, fixing #303 integration, and full CI for both PRs and merge queue.
#301 and #302 are queued; #303 inline SQL must use the before-effect boundary regardless of SQL prefix.
Isolated C:/Users/Q/code/barn-review-306 merges origin/master and #301 into #306; conflicts resolved in cb8aa53, package tests running.
Reviewing read-carrying renewal against newly merged property-shape/memo dependencies; managed SQLite oracle setup next.
Next: finish review/tests, push #306, stack repaired #303, run full CI and merge-queue checks. Leave this checkpoint uncommitted.

Checkpoint: #301/#302 merged; #306 at 1bd44e9 fixes dropped shape/memo dependencies; full CI 35181628618 passed.
Managed WSL Toast inline CTE/PRAGMA scenario plus admission: 2 passed, 12936 deselected.
#303 isolated worktree is stacked on #306 at 24147f2; SQLite guard/regression still to implement.
#306 became conflicting after #301/#302 landed, so queue attempt did not create an entry; merge latest master and rerun full CI.
Next: resolve that overlap, implement and verify SQLite retry guard, publish updates and finish CI/queue validation.

Checkpoint: #306 head 02491f3 and #303 head 67af036 both passed complete CI (35182320836 and 35182388300).
SQLite forced-conflict regression reproduced duplicate CTE inserts before guard; guard fixes both builtins, PRAGMA, and SELECT; full local Go tests and focused race checks passed.
Unchanged managed inline CTE/PRAGMA scenario passed on both WSL Toast and Windows Barn with admission.
#306 queue integration run 35182881929 is active; #303 was removed by the queue with reason invalid_merge_commit (not a test failure).
Next: let #306 merge, integrate resulting master into #303 if needed, rerun full CI, then queue #303 and verify final master CI.

Checkpoint: #306 merged as 8fa1f1b5 after full PR/queue CI; its master CI 35183510316 passed.
#303 refreshed onto that master as 08b8113 without tree changes, then passed full PR CI 35183537710 and queue CI 35183550596.
#303 merged as 6d1a6d33 with two verified parents; primary checkout fast-forwarded to this master, tracked files clean.
Final master full CI 35184189344 is running for 6d1a6d33; no remaining implementation work.
Next: wait for final master build/conformance completion and report both PRs merged with CI evidence.

Final checkpoint: #306 and #303 are merged; master is 6d1a6d33 and tracked files are clean.
Full CI passed for both updated PRs and both queue integrations; final master run 35184189344 is success on attempt 2.
Master attempt 1 passed conformance but vet could not import sync (unexpected EOF) following failed cache extraction.
Unchanged-SHA failed-job rerun passed build/unit tests; conformance success was retained. No source change was needed for the rerun.
Requested work is complete; local oracle scenario and these notes remain uncommitted evidence.
