# Real offline flight and magic

Raw `.tmp` links below refer to local retained recordings, not files shipped in
a fresh checkout. The [tracked acceptance summary](reports/mongoose-gameplay-acceptance-20261013.md)
preserves input hashes, outcomes and measurement limits; use fresh output paths
to reproduce those checks.

`scripts/run_mongoose_gameplay.py` runs an owned copy of a SHA-256-pinned Mongoose
checkpoint inside a loopback-only Linux network namespace. It creates private
accounts and ordinary player characters through existing game APIs. Production
credentials are unnecessary. Setup is outside the timed gameplay phase.

The flight scenario constructs a real Type 6 shuttle with installed systems,
grants its crew legitimate access, and drives connected console commands. Only
natural universe cycles advance flight. Position, heading, velocity, fuel,
station membership and actual braking are independent completion checks.

The magic scenario uses actual catalogue Lumos/Nox, ordinary wand fitting,
wielding and lessons. It counts completed light/extinguish sequences, checks
effects and resource receipts, and records natural fizzles separately. No copied
enabled spells, artificial proficiency or wizard casting substitute for work.

From PowerShell, verify the installed distribution and choose a fresh output:

```powershell
wsl --list --quiet
wsl -d Debian --exec unshare --net -- bash -lc 'set -euo pipefail; ip link set lo up; python3 /mnt/c/Users/Q/code/barn-gameplay-workloads/scripts/run_mongoose_gameplay.py --db /mnt/c/Users/Q/projects/mongoose/db/mongoose.db.new.2026-10-12T2349 --db-sha256 FAB6957ACFFA1DB8BC1C4B911FB53A8E8D6E605F4DE2A31B9F8E4FA874C3200D --moo /root/build/toaststunt-profiler-retention-refresh-order-20261013/moo --moo-sha256 f96c79de3ac701557c7fe71e2a9be42816c188d6d085afba1a7b4b3c4e7c7a01 --sound-db /mnt/c/Users/Q/projects/mongoose/db/sound.sqlite.barn395396-20261011T0348 --sound-db-sha256 BA15182E25C60E9795C10F8F9479095FF4D22EA9BF2A03D7D061446CFCA241E4 --out /mnt/c/Users/Q/code/barn-gameplay-workloads/.tmp/gameplay-next --scenario both --iterations 1 --warmup-seconds 30 --deadline-seconds 120'
```

`--scenario flight|magic` selects one scenario. `--iterations` counts complete
sequences, not submitted commands. Seed records controller choices; it does not
claim to reset engine randomness. Normal character permissions, station admission,
training and resource/recovery rules remain active. A refusal, native failure,
missing effect or expired deadline cannot count as productive work.

Default `--profile off --native-profile off` disables both diagnostic profilers.
It still includes controller/observer work and naturally scheduled world work;
whole-main-thread CPU deltas are never assigned exclusively to a single action.
Loopback input response brackets are separate from domain completion and from
human/client/network latency.

For a separate diagnostic cohort, add `--profile definitions` and optionally
`--native-profile required`. The native collector targets only the owned main
thread, proves local perf recording capability and preserves raw data/report.
`auto` records explicit unavailability; `required` invalidates the cohort when
collection fails. Native user-space CPU excludes kernel and worker CPU. These
diagnostic runs are not uninstrumented performance comparisons.

Manifests preserve perf's raw exit code. Perf's requested SIGINT shutdown can
return `-2` after flushing; the collector accepts it only with its verified own
stop, acknowledged disable and valid nonempty capture. Unexpected exits and
forced stops remain failures. Both sampling rates must be 10..1000Hz.

On this WSL installation perf lacks unwind support, so the collector records
flat instruction samples and reports that limitation. It cannot supply inclusive
native call stacks. Raw recording uses a private Linux filesystem directory;
only the collector's artifacts are copied into the output directory.

Artifacts include initial/final manifests, source/binary/controller hashes,
ordinary command transcripts, setup/actions/drain phase boundaries, before/after
domain state, failed and unfinished work, process CPU/RSS chronology, cleanup
and owned shutdown. Failed cohorts are retained. Profile export and its request
duration remain separate; source-line resolution is bounded and unresolved
positions stay visible. RSS is not a heap/retained-allocation profile.

Every run checks source checkpoint/catalog hashes again and stops only its owned
PID. A final successful status requires completed scenarios, requested collectors,
unchanged input/controller hashes and a successful owned shutdown. Read the raw
results; an implementation or unit-test pass alone does not prove gameplay.

This runner implements the first two scenarios in the
[campaign plan](../plans/mongoose-gameplay-performance-campaign.md). The same
domain outcomes must be proved on Barn before comparing engines. Allocation
attribution, mixed fleet/caster load, scheduled arrivals and latency percentiles
need separate subsequent cohorts; they are not implied by a single successful run.

Validated local evidence from October 13, 2026 is retained in
[both01](../.tmp/gameplay-both-01/manifest-final.json). One command completed one
flight sequence and one actual Lumos/Nox pair; magic included one natural fizzle
and three total attempts. The final manifest and scenario results record:

```text
status: complete
scenarios: [flight, magic]
completed_iterations: 1 each
shutdown.exit_code: 0
source_database/executable/controller_sources/sound_catalog/owned_shutdown: true
```

Both diagnostic profilers were disabled in this cohort. The recorded action
windows were flight 3.180644918s wall / 3.182204075s main CPU and magic
3.082636184s wall / 3.077598100s main CPU. Whole-process high-water RSS was
604636KiB. These windows include observer requests and scheduled background
work. They are one functional cohort, not repeat measurements, exclusive action
costs, a pristine-stock benchmark or evidence of faster execution/lower memory.
Setup, drain, cleanup and shutdown have separate evidence.

The isolated world retains inherited background tasks and their native errors.
For example, both01's bounded retained history reported `count=15, other=15`;
this is neither a complete error inventory nor an error arrival rate. Full
external-service parity and steady background behavior still need separate
validation before treating this workload as a production performance baseline.

[Flight05](../.tmp/gameplay-flight-05/flight/result.json) and
[magic04](../.tmp/gameplay-magic-04/magic/result.json) preserve the earlier
individual successful runs. Failed attempts remain alongside them. Tooling
regressions ran in WSL: `Ran 94 tests in 0.152s / OK`.

The separate [diagnostic02](../.tmp/gameplay-diagnostic-02/manifest-final.json)
also completed both scenarios, with native profiles collected (588 flight /
510 magic samples) and valid MOO profiles (747 / 664 accounted CPU periods,
unknown or missing 0 / 1). It ended with owned shutdown exit0, all five final
checks true and no finalization errors. Read the
[flight MOO report](../.tmp/gameplay-diagnostic-02/flight-cpu-report.md) and
[magic MOO report](../.tmp/gameplay-diagnostic-02/magic-cpu-report.md).

When perf lacks ELF support, resolve saved captures offline with
`scripts/symbolize_mongoose_native.py`. It verifies the captured executable SHA
and build ID, converts recorded mappings through ELF load segments, and checks
finite symbol bounds. It preserves every sample/period, including unresolved
shared libraries, and never attaches to or changes a running MOO. Use a fresh
output directory:

```powershell
wsl -d Debian --exec python3 /mnt/c/Users/Q/code/barn-gameplay-workloads/scripts/symbolize_mongoose_native.py /mnt/c/Users/Q/code/barn-gameplay-workloads/.tmp/gameplay-diagnostic-02/flight/native --output /mnt/c/Users/Q/code/barn-gameplay-workloads/.tmp/native-symbol-flight-next
```

Saved [flight symbols](../.tmp/native-symbol-flight-20261013-02/symbols.json) and
[magic symbols](../.tmp/native-symbol-magic-20261013-02/symbols.json) resolve
476/588 and 413/510 samples, with the remaining weights explicitly unresolved.
`db_fixup_owners(long)` accounts for estimated native self-IP shares 17.86% /
20.00%; identified profiler-specific functions account for 19.56% / 20.59%.
These are instrumented whole-main-thread user CPU estimates. Exact source
lines, native call graphs and total off/on overhead are unproven. Native and
MOO sampling boundaries differ. See
[the updated diagnosis](../why-mongoose-slow.md) for interpretation.
