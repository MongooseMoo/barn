Checkpoint 2026-09-16: PR #300 merged as 46c29cf; duplicate first aliases caused warm dispatch to select the wrong definition.
Commit 398bfc2 preserves the definition index, checks execute permission, and adds cold/warm, edit, and deletion regressions.
Store, builtins, and VM race tests passed; engine race run timed out at 180 seconds; plain engine tests passed.
Direct master push was rejected by merge-queue and required-check rules; commit pushed to fix/dispatch-memo-identity.
Work complete for the requested push; merge requires the repository workflow. Leave this checkpoint uncommitted.
