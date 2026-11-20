# Retain/fix checkpoint

- PR 112 first draft used `--checkpoint-interval 0`; this disables Barn's final shutdown checkpoint and caused six missing `AnonN.db.new` outputs.
- PR 112 head `54298f4` removes that flag while retaining one default profile, canonical admission, correct `Test.db`/startup fixture roots, strict markers, skip accounting, and evidence upload.
- Managed current-master admission plus Anon1-6 with normal checkpointing creates every output file: Anon1/3/4/5/6 pass; only Anon2 fails semantically (`0` pending vs Toast `1`).
- Existing draft PR 96 is the scoped Anon2/#95 repair; it is being audited/rebased instead of duplicated.
- PR 109 head `716a27f` has bcrypt boundary tests; code review approves semantics, but asks for an exact empty-password known-answer assertion and still blocks merge on the red legacy CI gate.
- PR 114 is the staged anonymous-GC repair for #113; independent review is pending.
- Conformance lifecycle repair has 467 unit tests green; its first full Toast command was non-canonical and is being rerun with the bundled startup fixtures and default profile metadata.
- Next: add exact empty bcrypt vector; audit/update PR96; verify PR112 rerun exposes only real semantic gaps; independently review PR114 and lifecycle repair; resolve green merge dependency without weakening checks.
