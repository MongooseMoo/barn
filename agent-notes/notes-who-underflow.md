# 2026-09-17 investigation checkpoint
- Reported binary revision a73ff54; isolated investigation worktree is C:/Users/Q/code/barn-who-underflow. Main checkout is concurrently changing; preserve it.
- Read-only database extraction found inherited #4:31 @who passing to #410:13 who/@who; latter chains idleflag = wizflag = " ". Root cause unconfirmed.
- Candidate control scenarios live in isolated .tmp/who-probe/assignment.yaml; no Barn implementation edits.
- Managed Toast command rejected --moo-suite-root without --candidate-root before execution. Next add the required candidate root and establish oracle behavior, then reproduce Barn.

# Control scenarios checkpoint
- Managed stock WSL Toast and Barn a73ff54 both pass admission plus chained assignment, inherited call assignment, and mapped idle_seconds/sort/yin assignment probes (4 passed on each).
- Controls use isolated .tmp/Test.db, candidate-root=isolated worktree, moo-suite-root=.tmp/who-probe; Barn executable path must use forward slashes for harness shlex parsing.
- Root cause remains unknown. Exact @who body at #410:13 calls player/formatting methods beyond these controls; #55:5 is map_builtin.
- Next expand managed discovery to actual database call path; original Mongoose login environment is absent. Asked user about preceding commands and other connected players.

# Actual database discovery checkpoint
- Disposable .tmp/mongoose-discovery.db changes only login/lifecycle adapters and replaces unused #2:ping with Test.db-style eval; original application verbs retained. Original source SHA256 30775964197ad133c07806847d52f3308680b3ee654d52a585b09ccc517b01cb.
- Single-player actual @who via explicit player-owned driver passes Toast and Barn. Two-player scenario (secondary #249) now passes Toast with admission (3 passed); next run Barn with trace.
- Multi-step tests ignore top-level setup; driver installation must be a run step. Login adapter must return 0 for empty args to avoid booting the primary when secondary opens.
- User explicitly requested fixing and proceeding. No production edits yet; exact panic not reproduced yet.

# Corrected investigation direction
- User rejected using conformance against Mongoose; those runs are withdrawn as crash evidence. No conformance repository source edits were made; stop that path permanently.
- Actual logs/latest.jsonl startup identifies C:/Users/Q/src/toastcore/toastcore.db for exact task 2054559239. Copied unchanged to isolated .tmp/toastcore-debug.db.
- Original barn.exe reproduces identical vm.go:619 stack underflow using -eval with a temporary player-owned driver invoking #2:@who. Actual verb is non-debug #100:@who, nine lines, ANSI utility #99 active=1.
- Delve stopped at panic but installed 1.24 cannot inspect Go1.26 symbols reliably. Built isolated unoptimized .tmp/barn.exe and installed Delve1.27.2 under .tmp/tools; next inspect active VM frame/IP and reduce onto Test.db only.

# Fix and verification
- Delve identified #99:show_who_listing line 5, IP36, SP0: missing args[2] becomes E_RANGE in a non-debug verb; ITER_PREP's generic error fallback supplies one value where compiler loop setup pops two.
- vm/op_iter.go now supplies an empty iterable plus flag in non-debug verbs, skipping invalid loops. Debug verbs still raise E_TYPE. vm/iteration_error_test.go failed with the original underflow before the fix and passes after it.
- Actual ToastCore CLI reproduction now returns => 0; vm/compiler/engine/server package tests pass. Minimal Test.db oracle and corrected Barn runs each pass admission and both invalid-loop scenarios.
- Changes remain uncommitted in C:/Users/Q/code/barn-who-underflow; corrected executable is .tmp/barn.exe there. Original checkout binary and conformance repository sources were not edited.

# Authorized merge checkpoint
- User requested merge to master. Committed and rebased onto 8442dfd; PR #319 head is 65663ddfc7159f973ec0fd6f6861308874d0ad6c, two files only.
- Rebased package checks and ToastCore CLI reproduction pass. PR CI run 35250265665 completed with both required checks successful.
- Normal merge requested; merge queue is testing 6a14b6538516a214c28baa4f38688e0c3d762a38 in run 35251426075, with parents current master 8442dfd and fix 65663dd.
- Waiting for queue checks and final merge. Do not stop at queued status; verify master contains the two-parent merge before reporting completion.

# Merge complete
- PR #319 merged to master as 6a14b6538516a214c28baa4f38688e0c3d762a38; both PR and merge-queue required checks completed successfully.
- Fetched origin/master and verified its two parents are 8442dfd611773dc0f748b9431bbf175d66cb030d and 65663ddfc7159f973ec0fd6f6861308874d0ad6c; fix ancestry check exited 0.
