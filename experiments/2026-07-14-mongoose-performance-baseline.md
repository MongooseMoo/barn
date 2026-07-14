# Mongoose deployment performance baseline

## Fixed workload

- Fixture SHA-256: `b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`
- Toast executable: `/root/src/toaststunt-mongoose/build-release/moo`
- Toast executable SHA-256: `a748a93644fe2b973cc85dfed902454a0a56c8b368afdc8104161ec76154d098`
- Runner: `scripts/benchmark-mongoose.ps1`
- Client: existing `cmd/moo_client`, with optional timestamped events and a
  deterministic maximum-duration boundary
- Login input: exactly three newline-separated commands supplied through the
  uncommitted `MONGOOSE_LOGIN_SCRIPT` environment variable
- Post-login commands, identical on both engines: `look`, `west`, `@who`, task
  and connection liveness query, then `dump_database()`
- Client timing: 3000 ms banner wait, 2500 ms between commands, 15 seconds idle
  timeout, and 40 seconds maximum duration
- Resource sample: after a fixed 180-second post-workload settle period
- Checkpoint completion: observed from creation of the disposable
  `<run.db>.new`, not from the command reply

Raw run artifacts stay under `.tmp/mongoose-convergence/` and are not committed
because the client transcript is deployment-local. The summary below contains
no login commands or credentials.

## Pinned WSL Mongoose Toast baseline

Run directory: `.tmp/mongoose-convergence/perf-toast-20260714-03`

| Metric | Toast baseline |
|---|---:|
| Database load to listening | 6392 ms |
| Connect to first banner | 143 ms |
| PROXY send to first output | 3 ms |
| Complete login from PROXY send | 5015 ms |
| Startup `@who` response | 2 ms |
| Explicit `look` render | 3 ms |
| Open-exit movement response | 6 ms |
| Liveness query response | 1 ms |
| Checkpoint command reply | 2 ms |
| Checkpoint file completion | 9429 ms |
| Post-settle CPU | 3.6% |
| Post-settle RSS | 311640064 bytes |

The liveness query returned `{3, 1}`: three queued tasks and one connected
player. The Toast transcript independently reported checkpoint completion in
9.37 seconds, consistent with the runner's 9.429-second file observation.

## Barn acceptance thresholds

These thresholds were fixed from the Toast baseline before measuring Barn:

- database load to listening: at most 12784 ms (2x Toast);
- complete login from PROXY send: at most 10030 ms (2x Toast);
- PROXY-to-first-output, startup command, `look`, movement, and liveness response: at most
  100 ms each (Toast is 1-6 ms; the floor avoids making scheduler jitter the
  acceptance boundary);
- checkpoint file completion: at most 18858 ms (2x Toast);
- post-settle CPU: at most 7.2% (2x Toast);
- post-settle RSS: at most 467460096 bytes (1.5x Toast);
- liveness: one connected player, a nonnegative queued-task count, successful
  `look` and west movement, and a completed checkpoint file.

Barn must be measured with the unchanged runner, fixture, commands, client
timings, and settle period. A failed threshold names the first performance
target; it does not authorize widening to another metric.

Connect-to-first-banner remains an informational measurement, not an acceptance
threshold. The required causal metric in the plan is PROXY prelude to first
banner/output. The client deliberately waits 3000 ms before sending PROXY;
Barn's banner follows that prelude, while Toast emits its banner before it.
Treating Barn's 3001 ms connect-to-banner value as a performance failure would
measure protocol ordering plus the intentional wait, not latency after PROXY.

## Windows Barn baseline

Run directory: `.tmp/mongoose-convergence/perf-barn-20260714-01`

| Metric | Barn | Threshold | Result |
|---|---:|---:|---|
| Database load to listening | 5380 ms | 12784 ms | pass |
| PROXY send to first output | 1 ms | 100 ms | pass |
| Complete login from PROXY send | 5483 ms | 10030 ms | pass |
| Startup `@who` response | 4 ms | 100 ms | pass |
| Explicit `look` render | 11 ms | 100 ms | pass |
| Open-exit movement response | 4 ms | 100 ms | pass |
| Liveness query response | 2 ms | 100 ms | pass |
| Checkpoint file completion | 2341 ms | 18858 ms | pass |
| Post-settle CPU | 0.46875% | 7.2% | pass |
| Post-settle RSS | 1882996736 bytes | 467460096 bytes | **fail** |

The liveness query returned `{2, 1}` and the checkpoint file completed. Barn's
saved `/debug/vars` proves that the RSS failure belongs to the Go heap rather
than an external process-accounting artifact: `HeapAlloc=847925488`,
`HeapInuse=1089699840`, `HeapSys=1945698304`, and `Sys=2008639992` bytes after
12 garbage collections.

The sole active performance target is post-settle RSS. The first hypothesis is
that the loaded database's in-memory object/value representation dominates the
retained heap. The next evidence is a heap profile from the same fixed Barn
workload; no other metric or source surface is active.

## Heap profile and first slice

The unchanged profile-bearing repeat is under
`.tmp/mongoose-convergence/perf-barn-20260714-02`. It reproduced the failure at
2010251264 bytes RSS with all non-memory gates still passing. Forced-GC
`inuse_space` accounted for 814.77 MB, of which database load retained 803.25
MB. The leading flat owners were:

| Owner | Retained bytes | Heap share |
|---|---:|---:|
| `types.NewMap` | 247.06 MB | 30.32% |
| `ObjectBuilder.ResetProperties` | 222.61 MB | 27.32% |
| `types.NewStr` | 131.01 MB | 16.08% |
| `Database.resolvePropertyNames` cumulative | 256.68 MB | 31.50% |
| `Database.readValue` cumulative | 508.04 MB | 62.35% |

This confirms the database-representation hypothesis. The first and only
active source slice is `types.NewMap`: its current `goMap` retains an insertion
order slice of key hashes plus a Go hash map whose values duplicate the full
key/value entries. The slice will delete redundant retained storage for small
maps while preserving exact typed key identity, insertion order, copy-on-write,
and indexed lookup for larger maps. The same deployment benchmark and managed
promotion/conformance gate decide whether the slice is kept.

### Slice 1 result: rejected and restored

The adaptive small-map representation reduced a three-entry `NewMap` from 10
allocations to at most 3. `go test ./types` passed. The managed map/promotion
gate passed 3163 tests with 8 established skips after excluding the unrelated,
pre-slice `toliteral(-0.0)` failure. That failure was initially misread as a
map-key mismatch; the mistaken key-preservation change and test were removed,
and the committed pre-slice formatter proves the row was already red.

The unchanged deployment benchmark under
`.tmp/mongoose-convergence/perf-barn-map-slice-01` measured 2133549056 bytes
RSS, versus 2010251264 bytes in the profile-bearing baseline repeat: an increase
of 123297792 bytes (6.13%). All latency, liveness, checkpoint, and CPU gates
still passed, but RSS is the sole keep/revert metric. The entire source slice
was restored and no source commit was made.

This is one consecutive slice with no kept improvement. The next retained-byte
owner on the same RSS target is `ObjectBuilder.ResetProperties` at 222.61 MB;
it must be inspected before naming the second slice.

## Pinned slice 2: property value storage

Inspection confirms that the loader's inherited-name resolution already builds
the complete final `map[string]Property`, but `ObjectBuilder.ResetProperties`
then allocates a second map and one separately allocated `*Property` for every
entry. Those pointers remain in every loaded object's property map and account
for the profile's 222.61 MB flat owner.

Slice 2 will store final object properties as `map[string]Property`, assign the
resolver-owned map directly in `ResetProperties`, and write changed values back
at the existing `db/store` mutation sites. It will not add an interface,
adapter, parallel property representation, or loader pass. A focused allocation
regression must fail before the change and pass afterward, the affected store
and format tests must pass, and the unchanged deployment benchmark is the sole
keep/reject decision. Any RSS decrease is kept because the acceptance target is
exact convergence; no decrease is a rejection and triggers the required stop
after two consecutive unsuccessful slices.

### Slice 2 result: kept

`TestResetPropertiesReusesResolvedValueMap` established the pre-change cost at
5 allocations for three resolved properties and passes at 0 allocations after
the conversion. The complete `db/store` and `db/format` suites pass. The full Go
suite has no new failures; its only failure remains the independently recorded
`TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent` scheduler
collision.

The unchanged deployment benchmark under
`.tmp/mongoose-convergence/perf-barn-property-slice-02` measured 1837264896
bytes RSS, a reduction of 172986368 bytes (8.61%) from the 2010251264-byte
profile-bearing repeat. Database load, PROXY response, login, commands,
liveness, checkpoint, and CPU all remain within their pinned gates. The slice
is kept and resets the consecutive-no-improvement count, but RSS remains above
the 467460096-byte acceptance threshold. The saved heap profile from this kept
run is the authority for selecting the next representation slice.

## Pinned slice 3: delete duplicated property names

The kept run's forced-GC profile retains 890.78 MB. Its largest flat owner is
now `Database.resolvePropertyNames` at 317.05 MB, with 292.50 MB attributed to
inserting final values into each object's `map[string]Property`. Every map entry
stores the property name twice: once as the map key and again as the `name`
field in the map value.

Slice 3 will delete `Property.name`, use the existing map key wherever a name is
needed, and pass names explicitly through the existing definition and view
boundaries. It will not add a replacement type, index, adapter, or parallel
storage. A red size regression must prove the map value shrinks, the complete
Go package tests must show no new failure, and the unchanged deployment
benchmark remains the sole keep/reject decision.

### Slice 3 result: kept

`TestPropertyFitsCompactMapValue` failed at 56 bytes before the deletion and
passes within the 40-byte budget afterward. The affected store, format,
builtins, VM, scheduler, and server packages have no new failures; the complete
Go suite still reports only the independently recorded scheduler task-ID
collision.

The unchanged deployment benchmark under
`.tmp/mongoose-convergence/perf-barn-property-slice-03` measured 1295065088
bytes RSS, a reduction of 542199808 bytes (29.51%) from slice 2's 1837264896
bytes. All latency, liveness, checkpoint, and CPU gates pass. The slice is kept,
but RSS remains above the 467460096-byte acceptance threshold. The saved heap
profile from this kept run selects the next slice on the same metric.

## Pinned slice 4: preallocate resolved property maps

The slice-3 forced-GC profile retains 831.56 MB. Property resolution remains
the largest flat owner at 263.18 MB: 223 MB at final map insertion and 39.68 MB
for property order. The resolver knows `len(oldOrder)` before allocating the
final property map but currently creates it with no capacity and grows it one
entry at a time.

Slice 4 will give that existing map the exact known capacity. It will not
change representation or touch another owner. A focused resolver allocation
regression, the affected package suites, and the unchanged deployment benchmark
decide keep or full restore.
