## Rebase checkpoint 2026-09-18
- User requested rebase onto latest origin/master; fetched tip 83c3e8d and preserved old HEAD abb32eb on backup/mongoose-before-rebase-20260918.
- Preserved two colliding untracked files in .tmp/rebase-local-files-20260918; other untracked work remains untouched.
- Rebase is at shared-admission commit b4120d8, with conflicts from upstream transaction split and BeginSnapshot rename.
- Earlier cache changes were relocated to snapshot/verbs files; retain upstream command-suffix and database disk-size behavior.
- Next: finish capability/wait-accounting relocation, complete rebase, build and run Go tests. Leave this note uncommitted.

## Rebase completion checkpoint 2026-09-18
- Rebased fix/mongoose-workload onto fetched origin/master 83c3e8d; final HEAD d3cb057 includes API integration repairs.
- Build succeeded; go test ./... exited 0; race tests passed for db/store (3.897s) and server (2.235s).
- Restored master and backup refs after Git rebase.updateRefs moved them automatically: master f1b5aa2, backup abb32eb.
- Both preserved untracked files differ from upstream and remain in .tmp/rebase-local-files-20260918; tracked working tree is clean.
- No conformance or live Mongoose rerun performed; next work remains checkpoint/login diagnosis and admission responsiveness measurement.
