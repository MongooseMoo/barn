# Capture actual offline Mongoose CPU work

Linked `.tmp` artifacts and sibling worktrees refer to the recording workspace.
They are excluded from source commits. See the [tracked acceptance summary](reports/mongoose-gameplay-acceptance-20261013.md)
for portable input hashes and the recorded gameplay results.

`scripts/profile_offline_mongoose.py` owns a disposable checkpoint copy, a fresh
wizard account, the Toast process and every capture artifact. It runs in a Linux
network namespace with loopback only. It verifies that namespace, database hash,
executable hash and `$prod()==0` before starting a diagnostic window. The source
checkpoint is copied and hash checked; it is never opened by a server for writing.

From PowerShell, verify the installed WSL distribution first. For Debian:

```powershell
wsl --list --quiet
wsl -d Debian --exec unshare --net -- bash -lc 'set -euo pipefail; ip link set lo up; python3 /mnt/c/Users/Q/code/barn-gameplay-workloads/scripts/profile_offline_mongoose.py --db /mnt/c/Users/Q/projects/mongoose/db/mongoose.db.new.2026-10-12T2349 --db-sha256 FAB6957ACFFA1DB8BC1C4B911FB53A8E8D6E605F4DE2A31B9F8E4FA874C3200D --moo /root/build/toaststunt-profiler-retention-refresh-order-20261013/moo --moo-sha256 f96c79de3ac701557c7fe71e2a9be42816c188d6d085afba1a7b4b3c4e7c7a01 --source-parent d80a17ca1d51f91a1938a9e4c1e88d52457deac7 --source-commit 45f4f613c3ab64378f292ef835e1c21c5cfb67ac --out /mnt/c/Users/Q/code/barn-gameplay-workloads/.tmp/offline-mongoose-cpu-next --seconds 10 --hz 100 --metadata-mode definitions --warmup-seconds 30 --sound-db /mnt/c/Users/Q/projects/mongoose/db/sound.sqlite.barn395396-20261011T0348 --sound-db-sha256 BA15182E25C60E9795C10F8F9479095FF4D22EA9BF2A03D7D061446CFCA241E4'
```

Use a fresh `--out` directory for every cohort. Supply the actual candidate hash;
the example hash identifies the accepted frozen profiler executable and will
change when it is rebuilt. `--source-parent` records the candidate's parent,
without claiming uncommitted source is a published commit. Optional
`--source-commit` records a known build revision; executable SHA-256 remains
authoritative. `--metadata-mode definitions` requests the explicit verb-focused
mode; default `detailed` also collects execution context. Definitions exports
must mark context unavailable and omit receiver/player/programmer/task fields;
the report validator rejects contradictory attribution. `--bootstrap-only`
validates account setup and nonproduction control without collecting CPU shares.

For representative sound work, supply `--sound-db PATH --sound-db-sha256 HASH`
using a complete local catalog snapshot. The controller verifies and copies it
to owned `files/sqlite/sound.sqlite` before bootstrap; only that copy is opened
with SQLite. A nonempty source WAL is rejected rather than discarded. The
manifest records source/copy hashes, SQLite quick_check and Sounds row count.
This supplies data through the existing MOO database path; no MOO sound repair
or error catch is installed. An older catalog can still lack newer sound IDs.

Snapshots include the size of the sound history map and a bounded census of
existing retained errors: total, cannot-resolve messages, other messages and the
history limit. These are retained records, not new-error counts; overwrites can
hide arrivals. Error arguments, command text and traceback payloads are excluded.
The report also groups attributed root paths and source-line self CPU, keeping
unknown/observer periods in the denominator. Root paths count each semantic
sample once, including builtin work.

Optional `--warmup-seconds N` lets background work run without sampling before
the capture. Default 0 preserves the original immediate-start cohorts; allowed
range is 0..120 seconds. Saved warmup snapshots show progress and error history,
without claiming the world has reached steady state. This separates restored
startup/rearming from later workload windows. Before/after snapshots and real
foreground actions remain necessary to establish the workload being measured.

The existing `bench_players` bootstrap helper creates a random-password account.
This controller splits its generated account creation into two emergency evals:
the expected missing-connection error propagates normally, and a second eval
completes the stored account. No MOO exception catch is added. The temporary
password-bearing script is removed and the saved stdout redacts that password.

The natural-background window collects the entire server main thread, including
existing scheduled work and the small start/stop control tasks. It saves a full
native JSON profile through wizard FileIO in the owned `files/` directory, so
control output truncation cannot discard it. A validated Markdown report and
folded stacks preserve all CPU periods, including unknown and missing samples.
External `/proc/PID/task/PID/schedstat` and RSS snapshots provide independent
whole-thread CPU/memory context. Before/after cycle, electronics and bounded
scheduler snapshots show workload progress; scheduler inspection is limited to
500 entries and overdue entries are not runnable-task or CPU attribution counts.

Initial and final manifests, native logs and failed cohorts remain in the run
directory. Shutdown opens a pidfd, verifies the executable, database arguments
and process start ticks, and signals only the owned process. No production SSH,
API calls, production credentials or fixture repairs are involved.
The run has no external network and uses empty owned `executables/`; `files/`
starts empty unless a pinned sound catalog is supplied. Dependency failures
remain visible in saved evidence. This is an
instrumented offline diagnostic, not a live Mongoose profile or a gameplay
speedup comparison.

The initial 2026-10-13 cohort is retained in
`.tmp/offline-mongoose-cpu-01`. Its native CPU clock measured 10.007086 seconds
and accounted for 1001 periods, without drops, overruns or unobserved expirations.
An independent process window measured 9.998210 seconds of main-thread CPU across
10.000222 wall seconds. Universe cycle advanced 41115234 to 41115239, electronics
time advanced, and `updating` was zero before and after. RSS samples ranged from
593532 to 600056 KiB; this range describes this instrumented interval, not a heap
leak or production memory claim. Owned shutdown exited zero and the source hash
remained unchanged.

That cohort exposes a profiler limit: 76.82% of periods were unknown, 13.99%
were published metadata/control states and 5.69% were transitions. Only two
deliveries were late. The dictionary reached 8192 states and 8388586 of its
8388608 budgeted bytes, with 38728400 dictionary rejections. Thus its small
attributed verb rows cannot establish the dominant Mongoose CPU consumer.
Reducing dictionary churn or retaining sampled states efficiently is required
before this workload yields useful complete rankings. The raw profile SHA-256 is
`1310ad1f360fa0b20af850f50ac5250250ddca5d0939136cd2409ffd25475b98`.

The first definitions cohort02 failed during source-line export with
`Missing END_CATCH in DECOMPILE`; its panic dump, server log and failed manifest
are preserved. A deterministic real compiled-Program test reproduced the
short-circuit pointer increment bug, and the minimal fix passed catch, nested
catch and fork-vector lookups. No source checkpoint or production server changed.

Cohort03 exported successfully after that fix, accounting for 1050 periods over
10.498787 main-thread CPU seconds with zero drops, overruns, unobserved periods
or late deliveries. It still reached 8192 states: unknown 45.62%, published
metadata 43.33%, semantic coverage 4.48%, 1212 frames and 6716382 budgeted bytes.
Zero dictionary rejections did not establish complete metadata because the
state-cap branch lacks a counter increment. Retain the incomplete ranking rather
than normalizing away its unknown or observer weights. Its profile SHA-256 is
`9f35c4f4c081bbc0af6aa4118206946184b351237de21c4e53a2496c4bec6a00`.

External request-window CPU of 9.991234s stops before sending the stop control;
the native timer continues until that control actually executes. Future captures
also retain start and stop/export control round-trip wall times, including
queueing, MOO wrappers, JSON serialization and FileIO. These are distinct from
native export wall time and from verb CPU shares. Progress snapshots bracket
start through completed export, not exactly the native sampling window.

Sampled-state retention cohort04 completed on executable SHA-256
`5baeb578320dc7c0b9eacb9caf6857c8e1b27bac46e8cdde68ba694578abfb4e`.
The dictionary recycled 92536 unsampled states while preserving sampled IDs.
It accounts for 1000 periods over 9.996166 main-thread CPU seconds: unknown 0.30%,
metadata/control 33.70%, transition 4.90%. Attributed self CPU includes
`#2585:pick_value` 16.20%, `#9777::validate` 3.50%, and builtin `recycle` 5.70%.
These are observations of one instrumented restored-world interval, not live
shares or an accepted speedup. Metadata observer cost still requires reduction
and paired fixed-work measurement before treating the ranking as representative.

Capacity rejections were 0; dictionary rejections 3375; budgeted structural metadata
6974276 bytes, retained Program estimate 1305031 bytes. There were no buffer drops,
one timer overrun and one late observation. Universe cycle advanced 41115234 to
41115238 and electronics time advanced. Owned shutdown exited 0; source hash was
unchanged. Full reports, folded stacks and manifests are retained in
`.tmp/offline-mongoose-cpu-04`; raw profile SHA-256
`69521bead2825256cbb981c11b71cd0881177820266eef5cdb52f4bfaf32c89e`.

Catalog-backed cohort05 uses the same frozen retention binary and a snapshot
with 229142 Sounds rows (SHA-256 `ba15182e25c60e9795c10f8f9479095ff4d22ea9bf2a03d7d061446cfca241e4`).
It accounts for 1007 periods with zero unknown, drop, overrun, unobserved or late
weights; metadata/control 44.69%. Error-handler root has 2 periods, scheduler root
generations 212 and 56, physics start_rearm 117; pick_value self 11 and recycle 58.
Sound history has 18023 keys before/after. Retained errors stay 15, including one
cannot-resolve record; these can predate the cohort and are not arrival counts.
Source/catalog hashes remained unchanged and owned shutdown exited 0. Raw profile
SHA-256 `ab8654be18264174ec9d7919a1b79ae2734369aab0f3c4a9e6843c6c23a09038`,
with all evidence in `.tmp/offline-mongoose-cpu-05`. It includes startup/catch-up
and inherited tests; these observations are not a causal speedup or live ranking.

Cohort06 supplies the same catalog, uses the accepted sampler executable
`f96c79de3ac701557c7fe71e2a9be42816c188d6d085afba1a7b4b3c4e7c7a01`,
and runs 30s of background warmup with sampling inactive. Cycle advances
41115234 to 41115249 during warmup; overdue entries among the inspected 500 fall
301 to 21. This is progress, not proof of steady state. The following instrumented
window advances cycle to 41115252, with overdue 2 and queued work 190 to 292.

Its native timer accounts for 1132 periods over 11.313072s main-thread CPU:
unknown, buffer drops, overruns, unobserved and late deliveries are all zero.
Published metadata/control is 44.70%, transitions 7.07%. Root paths include
start_rearm 211 (18.64%), #10566:_fork 187 (16.52%) and #1007:s_run 134 (11.84%).
Self CPU includes #9777::validate 123 (10.87%), bf_respond_to 47 (4.15%) and
#313:atmosphere_owner 42 (3.71%). Unsampled-state retirements 100171; capacity and
dictionary rejections 0. Structural metadata 6988912B and retained Program
estimate 902227B remain separate from process memory.

The external request window measures 10.002905s main-thread CPU across 10.002764s
wall. Start RPC is 2.265ms; stop/export RPC 1329.275ms, while native export itself
is 9.934ms. The timer continues until stop actually executes; these boundaries
are deliberately distinct and do not assign request delay to a particular verb.
RSS observations across capture/control range 594680..600768 KiB, with process
high-water 604412 KiB. These are instrumented process observations, not heap
allocation attribution, a leak finding or a memory reduction.

Published metadata/control states exclude sampler-handler and export cost.
Total observer overhead requires a paired benchmark; work skipped by the fast
guard can also be sampled under the MOO category.

Retained errors remain 15, including one cannot-resolve record; sound history
remains 18023 keys. Source/catalog hashes are unchanged and owned shutdown exits 0.
Raw SHA-256 `ba75fad8092b10b10541c02f22b9c66ed4250041319b85053471ebfdc73be6fc`;
[report](../.tmp/offline-mongoose-cpu-06/natural-background-cpu-report.md),
[manifest](../.tmp/offline-mongoose-cpu-06/manifest-final.json),
[warmup](../.tmp/offline-mongoose-cpu-06/warmup.json).
This restores a more complete world and yields attributed verbs, but still
contains rearming and inherited tasks. The [gameplay runner](mongoose-gameplay-runner.md)
now has successful flight and Lumos/Nox cohorts with independently checked
outcomes, action-window CPU/RSS and separate diagnostic captures. See the
[tracked acceptance summary](reports/mongoose-gameplay-acceptance-20261013.md).
Repeated equal-work comparisons, client latency percentiles and complete
external-service parity remain separate work.
