## PBT investigation checkpoint 2026-09-19
- Frozen DB copied into .tmp/mongoose-pbt-20260919; Barn @test $pbt reports 64/65 in 42.14s and 47.78s, generation exhausted in instance cleanup test.
- Canonical stock Toast lacks mixed numeric behavior: raw runner only 38/65, so its fast failure is not a valid speed comparison. Documented Mongoose Toast build reports 62/65 in 668.68ms (three cleanup failures).
- Fixture tests dominate Barn observed timings: cleanup-on-failure 1.47s, replay-cleanup 8.71s; baseline CPU capture instead shows background #1007:s_run, so do not attribute that CPU to PBT yet.
- Isolated fixture test now running with builtin debug capture; inspect create/recycler/cleanup costs and scheduler waits, then reduce uncertain behavior against canonical Toast.
- Owned Barn PID 41440 on 11505/debug11486; owned Toast runs on17910 and17911. Do not touch pre-existing Toast PID1174391. No implementation changes yet.

## Root cause checkpoint
- Current root database hash 12a3e15e differs from frozen c44051f1; current Mongoose Toast repeats three cleanup failures in 507.02ms.
- _verify_and_recycle waits up to 300 suspend(0) turns for GLOBAL queued_tasks <=10 before each recycle. Unrelated periodic world jobs dominate those waits; Toast fails quickly instead.
- Canonical managed generic recycle-with-12-future-tasks passed (2 tests including admission) on both Toast and unmodified Barn; the global wait is unnecessary for recycling.
- Next: produce a guarded offline DB repair removing only the polling block, verify fresh-world suites on both engines, preserve the original DB and document application.

## PBT repair completion
- Commit b2cb70b adds guarded offline repair, byte-preservation/rejection tests, generic recycling regression, and documentation; no Barn Go runtime changes.
- Current DB baseline Barn 62/65 in30.75s vs repaired65/65 in818.50ms; Mongoose Toast baseline62/65 in507.02ms vs repaired65/65 in1.51s. Single fresh-world observations, not statistical estimates.
- Canonical generic regression passed on both engines; three Python repair tests passed. Local root DB now repaired hash3169f9fb; original12a3e15e backed up at .tmp/mongoose-pbt-current-20260919/mongoose.before-local-repair.db.
- Disposable Barn stopped and four owned Toast processes signalled; pre-existing Toast left alone. Remote mongoose.world unchanged; a fresh remote download needs this repair again until the MOO helper is corrected there.
- Separate SQLite login error and existing scheduler responsiveness work remain; notes uncommitted, tracked tree clean.
