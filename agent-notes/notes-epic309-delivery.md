# Epic 309 delivery checkpoint - 2026-09-18
- User authorized prerequisite PR repairs/review/CI/merges (210, 247, 248, 307), then separate sequential PRs for 310-313 and epic closure.
- Isolated worktrees C:/Users/Q/code/barn-309-pr{210,247,248,307}, branches delivery/pr{number}; merged origin/master 6a14b65 locally; PR248 conflict resolved preserving absolute DB path and descriptor registry construction.
- Local gorename dry run succeeded with Git usr/bin added to process PATH; constructor rename belongs to 311 only. No remote pushes or merges yet.
- Claude review of PR210 running exec session 78981; initial invocation swallowed positional prompt, corrected by stdin. PR210 merge output session 60887 may need final poll.
- Next: managed Debian WSL Toast verification and current conformance baseline, repair actual failures with regressions, Claude reviews, exact-head CI, normal two-parent merges. PR307 has known unresolved suffix-versus-terminal-completion defect.

## Checkpoint 2
- Published master merges to existing PR branches: 210=b0ccd62, 247=1a7eced, 248=a6802f0; focused Go tests passed, full CI runs 35398526994/35398530220/35398533940 pending.
- Claude reviews: 210 no actionable findings; 247 flags OS capitalization/source metadata shape; 248 flags adding input+checkpoint sizes and stale checkpoint use (source evidence only, oracle needed before repair).
- Clean conformance dependency worktree C:/Users/Q/code/moo-conformance-309 at origin/main 9bdfd2e; original conformance checkout is dirty and untouched. Linux uv env /tmp/barn309-conformance-linux-venv.
- Managed Toast prerequisite session 32905 logs /tmp/barn309-toast-prerequisites.log, still running at last poll. Narrow disk-size oracle YAML added in PR248 server/testdata; managed invocation exited 1, exact failure must be read and reported before improvising.
- Next: read /tmp/barn309-toast-disk-size.log, establish disk-size behavior, TDD repair PR248; collect CI failures and finish reviews; PR307 terminal command completion still unresolved. No GitHub merges yet.

## Checkpoint 3
- Broad Toast managed prerequisite run completed: 520 passed, 44 skipped, 12373 deselected; log /tmp/barn309-toast-prerequisites.log. CLI custom suites require candidate-root and copied Test.db inside it (SHA 1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e, 2035 bytes).
- PR248 disk-size oracle passed 2 tests, unchanged Barn failed 2035 vs 2040, fixed Barn passed 2 tests. Uncommitted implementation selects one file using atomic ordinaryDumpStarted; checkpointWith gets ordinary bool, panic false. Go regressions fail then pass. Logs /tmp/barn309-{toast,barn}-disk-size*.log.
- PR248 additional failed-dump oracle running session 76634, /tmp/barn309-toast-disk-failure.log; PR247 metadata oracle now 5 passed in /tmp/barn309-toast-version.log. Use statement rather than code for YAML multi-statement programs (code wraps an expression).
- Claude PR307 design review session 54672 pending. PR247 production not modified yet; needs TDD OS naming/source VCS string metadata fixes and pseudo-version coverage. All three CI Go jobs passed; conformance still pending.
- Next finish narrow repairs/reviews, retain exact command/evidence records, publish only scoped changes, await terminal CI; no merges yet. Custom YAML/fixture files in PR248 server/testdata and PR247 builtins/testdata/server_version_oracle are untracked and directly inspected.

## User correction: retain Unicode indexing
- User explicitly cancelled merging PR210: current Unicode behavior is wanted for now. Exclude PR210 from all subsequent delivery/merge actions and revise epic prerequisite gate accordingly.
- PR210 had passed full CI (12517 passed, 420 skipped) and was queued, but NOT merged. Successfully removed with GraphQL dequeuePullRequest(input:{id:<PR node ID>}).
- Final live query: state OPEN, merged false, mergeCommit null, autoMergeRequest null, mergeQueueEntry null. gh pr merge --disable-auto alone does NOT remove a queued PR.
- Remaining authorized campaign: repair/review/CI/merge PR247, PR248, PR307, then sequential issues 310-313. Original checkout and existing Unicode implementation remain untouched.
- Previous tool call was interrupted; check execution state before repeating PR247 Linux build/oracle-red invocation. PR307 Claude review session 54672 may still run.

## Checkpoint 4
- PR248 repaired commit 2cc1b1c pushed; Claude re-review no actionable defects; Linux race ./server ./builtins passed; normal and failed-dump managed probes passed on Toast and fixed Barn. PR body updated. Custom probe fixtures remain untracked, do not stage accidentally.
- PR247 uncommitted metadata fixes pass Go and managed oracle tests (Toast 5 pass, old Barn 3 fail/2 pass, fixed Barn 5 pass). Claude re-review found unused builtins.boolInt (staticcheck), spec text must describe MOO not Barn, and missing builtin VCS/unknown revision cases; fix before publishing.
- PR247/248 unexpectedly appeared queued on old heads: push248 rejected GH006. Dequeued BOTH via GraphQL before retrying push; neither merged. Last query also confirmed PR210 unmerged and not queued. Watch queues so unfinished changes do not merge.
- Claude PR307 review finished: use queue_info(player)["last_input_task_id"] plus task_stack completion probe after suffix; needs Toast-first proof/fixes for duplicate resumed suffix, missing queue key, completed-task stack visibility. Full review in tool session 54672 output. Mongoose login env absent; async question asks existing fixture/setup (no credentials in chat).
- Epic309 and children310-313 explicitly updated to five-prerequisite gate excluding210. All remaining work authorized; do not merge210. Next repair247 review findings, ensure CI248 final head, then implement307 completion with managed generic fixture proofs.

## Checkpoint 5
- PR247 repaired commits 8a370f7 and doc-only 3ef8221 pushed; unused boolInt removed, generic spec fixed, VCS/unknown source cases added. Focused Go/staticcheck pass; Claude final review no blocking defects (session29415 completed). CI must follow 3ef8221, PR body still needs update.
- PR248 final head2cc1b1c CI run35400344113 pending; reviewed repairs and race pass. Neither PR210 nor247 nor248 merged at last query;210 stays excluded.
- Durable conformance branch tests/barn309-prerequisite-contracts in moo-conformance-309 has untracked version metadata YAML, two disk-size YAMLs and two isolated 2035-byte fixture copies. Managed Toast paired contract session passed6 tests in11.05s; not committed/published yet. CLAUDE.md requires durable tests in conformance repo, not Barn probes.
- Additional input_task_completion.yaml added for307; completed-fork stack oracle passed, connection test initially consumed output in send. Corrected to assert initial prefix/before/suffix at send, delayed after at read; latest Toast session7791 writes /tmp/barn309-toast-input-completion.log. Do not inspect/debug Barn implementation until corrected oracle passes.
- Remaining307 design: after suffix send nonce OOB probe, wait for last_input_task_id task_stack to disappear. Needs runtime fixes proven by oracle and Python protocol regressions. Async fixture/login setup question unanswered. Parent310-313 have not started (gate remains).

## Checkpoint 6
- PR248 final2cc1b1c now both required CI checks pass (run35400344113); ready for scope/head/review verification and normal merge queue submission. PR247 current3ef8221 CI run35400907072 pending; Claude no blockers, body updated.
- Input completion oracle now3 passed: quote YAML run strings containing whitespace before # object literals (otherwise YAML comments truncate code); send consumes output, so assert initial output at send and delayed output at read. Toast log /tmp/barn309-toast-input-completion.log.
- Old Barn input completion had2 fail/1 pass: retained completed task_stack returned[] instead of E_INVARG; resumed command repeated suffix. PR307 uncommitted runtime fixes consume suffix in shared flushTaskOutput, reject terminal task stacks, expose existing connection LastInputTaskID in queue_info; focused/full builtins-engine-server Go tests pass, managed Barn now3 passed in12.00s.
- Go suffix regression initially assumed suspend(0) state suspended; runtime queues immediate resumptions, corrected expectation to TaskQueued. Behavioral red evidence is managed unchanged-Barn failure. New builtins/task_completion_test.go directly inspected and passes.
- Next add benchmark completed_command nonce protocol plus wizard-owned helper intercepted only for reserved OOB probe (preserve existing OOB code and native command dispatch), tests including warmup timeout. No script changes yet. User fixture/login question still pending; no PRs merged,210 excluded and dequeued.

## Checkpoint 7
- User rejects any login-script dependency or further discussion of that workload. Continue #309 with managed generic conformance; no fixture/login request remains pending. #210 remains excluded; #310-313 have not started.
- #248 now MERGED at 72814a66e8fada368bff4fb4f7d3ca841c5825c1; verify two parents and issue closure. #247 exact-head CI passed and is now AWAITING_CHECKS in merge queue.
- #307 exact generated Python installer passed managed Toast (2 tests, 11.19s) and Barn (2 tests, 10.41s), including admission, delayed command completion, preserved OOB handler. Python 9 tests pass. Metrics now completed/completed_per_s instead of misleading committed/goodput.
- #307 Claude final review running session58683; Linux race and vet session64654. Changes still uncommitted. Companion conformance branch has three commits (latest c230f75), no PR yet.
- Next finish #307 review/CI/merge and companion tests, then original #310-313 sequence; do not turn live performance measurement into another gate.

## Checkpoint 8
- #248 two-parent merge verified and #241 CLOSED; #247 merge queue build passed, conformance still running (35402737104). #210 untouched. #310 source surveyed only; no implementation before current prerequisite gate.
- Claude #307 found early acknowledgement during traceback and stale input ID on unrecognized commands. Helper now always forks into scheduler before acknowledging, handles ID0; server clears ID for unknown native command. Toast 3 pass vs old Barn expected0/actual40; final generated helper now passes Toast2/Barn2 including unknown command.
- Companion conformance PR102 published draft. Claude found missing fixture hashes and vacuous task-stack check; fixed manifest, requirements, positive suspended-stack checks, failure cleanup. Full focused Toast set now8 passed20.76s. Local companion changes not yet committed.
- #307 race builtins/engine/server passed (6.992s/193.027s/4.206s), vet passed before final server one-line change. Full Linux go test only failed repository hygiene because Windows worktree .git absolute path is invalid in WSL; all packages passed. Re-run with explicit translated GIT_DIR/GIT_WORK_TREE or Windows Go; do not alter repo code for environment.
- Claude #307 re-review session72509 pending. Need final managed Barn input suite, static checks, commit/push307, follow exact CI, finish companion review/publish. Final proof logs /tmp/barn309-{toast,barn}-probe-final.log. Prevent repeat path errors by using conditional Test-Path, not merely printing False then searching.

## Checkpoint 9
- #247 merged d6ade3e1478f2729c43dcb422cbcb21ed8b51909 with two parents, #243 CLOSED. Epic309 four prerequisite checkboxes updated; only307 remains. #210 verified OPEN/unmerged/not queued.
- #307 committed/pushed 2cefd54988043e56b944e2aec009cf3576ae0589, clean worktree; PR body rewritten. Claude re-review no blockers, confirms always-fork scheduling orders acknowledgement after traceback/flush. Full Go tests/vet pass with translated GIT_DIR and GIT_WORK_TREE; races passed. CI35403572568 still running at23:01UTC (Go tests finished, cache upload pending; conformance running).
- Companion PR102 final e6da372 pushed, draft until307 merged. Claude review fixes complete incl same-connection nonzero-to-zero identity reset; Toast3 passed14.77s, Barn3 passed14.34s. Fixture validator20 pass. CI35403938914 pending; earlier profile matrix still running.
- Prepared ONLY a dry-run migration tool in main .tmp/split-store-txn.go, using Go parser and token/comment fingerprints. 135 declarations identical, source3692 lines -> lifecycle204 plus9 domain files. No StoreTxn implementation modified; wait307 prerequisite then create310 worktree from mergedmaster and apply tool -write. Tool excludes changes to existing COW/history/cache files.
- All Claude/session work complete. Original epic children310-313 remain unstarted beyond read-only survey/migration preparation. Next await307CI, normal merge queue, then310 using temporary tool, full checks/Claude/PR/merge in sequence. Companion tests are additional support, not another gate on310.

## Checkpoint 10
- #307 head2cefd54 full CI35403572568 PASSED: 12517 passed420skipped1warning; build/staticcheck/vet/Python passed. Queued with exact-head match, integration run35404609576 on3aa939dc3499c3a2f6a8902aa0dfc4b05df788c4 still running. Do not merge210.
- Companion PR102 head e6da372 full CI35403938914 PASSED all profiles: 64on12913/30skip,32on12891/52skip,64off12898/45skip; each1deselected. Remains draft until307 lands, then normal merge (no branch rules returned). Claude findings fixed; worktree clean.
- Earlier companion run35403655428 had a harness FileNotFoundError while snapshot_regular_files statted renamed Test.db.new.#1# in existing dump_database test; final-head full matrix passed without harness changes. Preserve as validation limitation, not a new #309 workstream.
- Claude310 preliminary mapping review session35679 completed: no blockers,135declarations complete, comments preserved, imports valid, existing COW/history/cache untouched. Re-run tool dry-run on actual merged base then -write only after307. Import grouping improved, projected lifecycle file205lines; main temporary tool directly inspected and gofmt formatted.
- #311 census49trackedfiles; historical reports/profile records may stay per issue; local gorename available and no BeginSnapshot collision. #312 duplicates10write-map resets and ordered4validators; #313 caller inventory surveyed read-only. Next priority actual310 once307 queue merges, not further prerequisite expansion.

## Checkpoint 11
- All five prerequisites now merged. #307 merge3aa939dc3499c3a2f6a8902aa0dfc4b05df788c4 two parents verified, integration CI35404609576 success (12517pass420skip). Epic309 checkboxes updated. #210 remains excluded.
- Companion conformance PR102 MERGED30a71eb3f472b8d77e6e1086b8359716511b67fa, two parents verified, all profile CI passed before merge. New Barn CI consumes these seven added cases.
- ACTUAL #310 implemented in C:/Users/Q/code/barn-issue310, branchissue/310-store-txn-split from3aa939d. Applied parser/token/comment-verified135declaration relocation. store_txn.go3692->205; nine new domain files; total+56header/import/spacing lines, zero deleted code complexity. Existing COW/history/cache files unchanged.
- #310 committed222f746df7561c1bc62ef4429abdad90d8ba552e and published PR320 (Closes310). Claude actual-diff review session30147 no blockers, independently all135declarations byte-identical. Full Go/vet exit0; race store4.179s,builtins7.228s,engine183.061s; diffcheck clean. PRbody has complete per-file line counts/proof/results.
- CI35405858980 on222f746 still running (conformance and Go cache post-step); wait pass then exact-head normal merge queue, verify2parents/#310closed. Only after310merged create311 worktree, localgorename BeginReadOnly->BeginSnapshot; then312helper extraction and313contract inventory. No running local/Claude sessions now.

## Checkpoint 12
- #310 PR320 head222f746 full CI35405858980 PASSED:12524pass420skip, zeroerrors/failures. Queued normally after checks; integration CI35406886932 on70a70ead8add768629e1284e3b007f213dc2b43a running. Worktree clean. Parent310 not yet closed/merged.
- #311 dry-run discovered gorename's legacy GOPATH scan only edits27files in db/store, missing cross-package callers. Installed gopls v0.17.1 dry-run succeeds on Go1.26 and edits42files across store/builtins/engine/vm. Use local gopls type-aware rename per explicit issue311 allowance, not incomplete gorename output; user informed of choice.
- Find exact UTF8 byte offset dynamically from store_txn.go raw text IndexOf('BeginReadOnly(readTS'), then Encoding.UTF8.GetByteCount(prefix), position 'db/store/store_txn.go:#'+offset. Do not guess line positions (first gopls93 guess failed harmlessly); -d confirmed no source changes. Later311 use -w only after310merges.
- Audit remaining Go comments separately (store_core/history_gc/resolve_cache and lazySet); preserve historical plan/report/experiment/profile records per311. Temporary dry diffs are $env:TEMP/barn309-311-{gorename,gopls}-dry.diff. All local/Claude sessions complete.
- Next wait320integration, verify merge2parents/issue310closed, create311frommergedmaster and apply localgopls. Then sequential312/313 with requested checks/reviews/normalmerges. No new workload or credential requirement;210 remains excluded.

## Checkpoint 13
- #310 PR320 MERGED70a70ead8add768629e1284e3b007f213dc2b43a, two parents verified, #310 CLOSED; integration CI35406886932 success. Epic309 checkbox310 updated.
- #311 implemented in C:/Users/Q/code/barn-issue311 branchissue/311-begin-snapshot from70a70ea. Local gopls -w dynamically located UTF8 constructor offset and updated42files. Patched four maintained Go comments, one test diagnostic, and active MVCC plan reference;46files total, exact string-substitution verification passed.
- Old name absent from all Go source; only four historical records remain (June24floor experiment, Sept17profile-before.txt, phase0notes/report). MVCC plan explicitly calls itself active, so its reference WAS updated (not historical).
- #311 committed/pushed9e036497acaae4d131535272cfc15a716f485785; PR321 published Closes311. Full Go/vet passed; race runsession67795 pending (store passed5.529s). Claude review session78077 pending. Worktree source unchanged after commit; update PRbody when pending checks finish.
- Next complete321CI/review/races thennormalmergequeue. Only after311merged start312 private clearStagedWrites/shared ordered validation extraction, allocation benchmarks base/head, full checks/Claude/CI/merge; then313 inventory with justified retained boundaries. #210 excluded throughout.

## Checkpoint 14
- #311 PR321 remains OPEN at9e036497acaae4d131535272cfc15a716f485785. Claude review rerun saved TEMP/barn309-311-claude-review.txt: no blockers, all lines exact rename; PRbody updated. Local race complete: store5.529s,builtins7.143s,engine185.253s; full Go/vet logs complete.
- CI35408011995 build job success, conformance tests12944 failures0 errors0 skipped420; conformance job still Post Run actions/setup-go@v5. Artifact TEMP/barn309-321-head-conformance. Await final success, queue normal exact-head merge, verify integration and issue closure before312.
- #312 baseline benchmarks on311head saved /tmp/barn309-312-base-bench.log: property35allocs/op5415-5416B/op, read17allocs/op2672B/op (five samples each); Linux Go1.24.6, AMD5950X32CPU, GOMAXPROCS unset. Same tree expected after311 merge; verify before head comparison.
- #312/313 issue bodies re-read; no implementation started. Main dirty worktree preserved; #210 remains OPEN/unmerged/autoMergeRequest null. No running local sessions. Use existing notes and isolated worktrees to continue.

## Checkpoint 15
- #311 head CI35408011995 PASSED; exact9e03649 queued normally. Integration CI35408991741 on90853788bb0aaf7457217d94c6dc20e040b2ed8e still in progress, build success, conformance cache upload pending. PR321 remains OPEN until queue finishes.
- Integration artifact TEMP/barn309-321-integration-conformance/conformance-results.xml:12944tests0failures0errors420skipped331.857s. Await job success, verify321merge/two parents/311closed, update309 then create312 worktree.
- Read-only audited shared validation/reset sites and313live-boundary callers on311tree. Full caller inventory unchanged from earlier summary; store_txn_live.go methods must remain distinct (whole-object versus facet adoption, targeted forget/renumber, unvalidated flush versus validated renewal).
- Runtime task_runtime.go preserves committed writes on task exceptions; retries discard pending effects/forks and replace/release snapshot. Pending options flush even on no-write successful completion. No implementation changes or new behavioral uncertainty investigated during wait.

## Checkpoint 16
- #311 PR321 MERGED90853788bb0aaf7457217d94c6dc20e040b2ed8e, two parents verified,311CLOSED,309checkbox updated. Integration35408991741 success. #312 created from this merge at C:/Users/Q/code/barn-issue312 branchissue/312-shared-bookkeeping.
- #312 implemented/committed90432c5dcd5181abcbbb9495c4a004b8988a6480, PR322 Closes312. Three files29added47deleted(net18): private clearStagedWrites tenfields and validateReadsLocked scalar/relationship/property/verb; callers keep locks/classification/preflight. No behavior change/new tests.
- Claude actualdiff review saved TEMP/barn309-312-claude-review.txt: no blockers. Full Go/vet and race passed: store4.830s,builtins6.932s,engine186.503s. Diffcheck clean; worktree clean. CI35410034856 running, must finish beforequeue.
- Sameenv benchmarks head /tmp/barn309-312-head-bench.log: property35allocs/op5415B/op, read17allocs/op2672B/op unchanged frombase. PRbody records command/fivesamples/Go1.24.6Linux and all localresults. No timing claim. All local sessions complete.
- Next await322headCI, normalexactheadqueue/integration/merge, update309312checkbox; onlythen313 durablepost312 contract/callerinventory. Priorreadonlyinventory covers everycaller, pendingeffects, retry/error/GC boundaries; retain distinct contracts when not equivalent. #210 excluded.

## Checkpoint 17
- #312 PR322 head CI35410034856 PASSED, fullconformance12944tests0failures0errors420skipped332.753s, artifact TEMP/barn309-322-head-conformance. PRbody updated with full local/Claude/benchmark/CI evidence.
- Admission tested PRmerge72eb14c808edad8643bd1ed5101a36f9e2556ab4; fetched refs/pull/322/merge and verified tree54ece21a02ed01e5cbe0eecabf4cef3a68e8c5e2 equals branch90432c5. Exacthead normalqueue submitted.
- Integration CI35410934149 on9923cb41fd2ef1cf3c49086aca8cd47f8fbc9420 now running: build cacheupload, conformance suite active. No source/review/test repair pending; waitfinalsuccess/merge then313.
- #313 planned durable inventory must distinguish validated renewal, carrying reads, unvalidated flush, facet-specific adoption, targeted forget and ordered renumber reconciliation; preserve pending effect ownership in engine rather than moving into StoreTxn. No313implementation yet. All local sessions complete.

## Checkpoint 18
- #312 PR322 MERGED9923cb41fd2ef1cf3c49086aca8cd47f8fbc9420,two parents verified,312CLOSED/309checkbox updated. Integration35410934149 success12944tests0errors0failures420skipped. #313 worktree C:/Users/Q/code/barn-issue313 branchissue/313-live-boundary-contracts from9923cb4.
- #313 doc plans/store-txn-live-boundaries.md197lines committedb5e929a98e90eed7f04bfb6f988d2b5f7ad7d6cc, PR323 Closes313. Comprehensive9operationcaller/contracts/candidate-retention table; noGochanges. CI35412015703 running. LocalfullGo/vet/race passed(store4.158,builtins6.986,engine186.818), diffcheck clean.
- Claude review TEMP/barn309-313-claude-review.txt: retain decision sound/no equivalent redundancy, but fix wording: alreadyescalated/previousirreversible skip renewal, unrerunnable failure can proceed; next installs anysuccess but forks/effects onlypublishedwrites; exactdeleteflush terminal; AdoptObjectpreservesownedmark, verbscan added, relationshiptombstones/partialprogress, exactbundledDBpath.
- Claude also suspected preexisting resumed-task renumber loses scalar/verbcodewrites (UNCONFIRMED). Created linked investigation #324 requiring Toast-first managed proof, noBarnrepair/diagnosis done. #313 explicitly says recordindependentissues; do NOT expand refactor. Must add324link/caveat to doc and limit renumberstatementtoproperties.
- Next applydocreviewcorrections (anchors justread), commit/push, rerunClaude focusedrecordreview; finalheadCI required (oldrunmaycancel). SourceGo unchangedso completedtests validunless newsourcechanges. Alllocal sessions complete. Thennormalmergequeue/CI, close313and309, finalreportfilemovesvs18deletedlinesvsretainedboundaries, mention324unconfirmedand210unmerged.

## Checkpoint 19
- #313 review corrections committed/pushed248e0616520fc9c33e04ebb442cffa894860aab5; doc now215lines, onlychangedfile. Linked324unconfirmedinvestigation and correctedboundarybypass/ownership/fixture statements. FinalClaude TEMP/barn309-313-claude-final-review.txt says no blockers, accurateagainst9923cb4; one optional GCwordingnit left asnonblocking.
- Finalhead CI35412305939 JUST PASSED. Fullconformance12944tests0failures0errors420skipped331.867s; artifact TEMP/barn309-323-head-conformance. AdmissionPRmerge85cae35608eb69e3466f61dcc64933ae1a2ffda8 tree de97b10578d674fe961877bd2394eb2d46627f14 equals248e061tree; verifiedfetch. Oldb5e929run35412015703 auto-cancelledbyworkflow afterpush, notmanuallyinterrupted.
- PR323bodyrecordshead,localGo/vet/race,Claude and324scope; stillneeds finalCIstatus update. PR320/321/322bodiesupdatedwithcompletedintegration/merges. Liveissues310/311/312CLOSED,313OPEN;210OPENunmergedunqueued confirmedGraphQL.
- Next queue323normalmerge withmatchhead248e061, followintegration tosuccess, verifytwo-parentmerge/313CLOSED; update309checkboxandrecordintegratedvalidation/reductions/retainedcomplexitythenclose309. Do not expand324; noGo/sourcechangesneeded. Alllocal sessions complete, worktreeclean.

## Checkpoint 20 - completed
- #313 PR323 MERGED83c3e8d7ef3546ce2ae82d550684790f77706975, parents9923cb4/248e061 verified. Integration35413215718 PASSED; final fullconformance12944tests0failures0errors420skipped332.447s, admissionpins83c3e8d. Artifact TEMP/barn309-323-integration-conformance.
- #309 CLOSED, children310-313CLOSED, allfourPR320-323MERGED. Completion evidence posted https://github.com/MongooseMoo/barn/issues/309#issuecomment-5738411437 withallhead/integrationCIids,checks,reductions/retentions. AllPRbodiesfinalized,309checkboxeschecked.
- Delivered:310core3692->205lines verified135declarationmoves(no deletedcomplexity,+56headers);311localgoplssemanticrename;312sharedhelpers18netlinesdeleted/noallocationregression;313durable215linecontractinventoryretainsnon-equivalentboundaries. AllClaude/localGo/vet/race/fullCIsuccess.
- #210 remains OPEN/unmerged/unqueued, Unicodepreserved. #324 independentUNCONFIRMEDrenumberhypothesis remainsOPEN withToast-firstrequirement, not partofcompleted309. Originaldirtymainworkpreserved,notesuncommitted; deliveryworktreesretainedclean. No active local sessions or remaining309work.
