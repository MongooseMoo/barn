## Measurement checkpoint
- User requested repeatable representative interpreter, Mongoose/PBT, and MVCC comparisons; no runtime optimization in this task.
- Linux Barn built from b2cb70b; both engines will run in Debian using the repaired 3169f9fb database and copied sound.sqlite.
- Added Linux-native alternating fresh-world matrix driver; Toast harness smoke run underway, results checked before counting timings.
- Existing unrelated Toast on port 17890 remains untouched; matrix owns port 17920 and its subprocess groups only.
- Next: validate concurrency fixture, repeat multiple fresh worlds, summarize correctness and timings; leave this note uncommitted.

## Harness validation and map assignment
- Toast smoke confirmed exact numeric/list/map results; parentless recycling goes through Mongoose's protected recycle wrapper and is not a valid shared lifecycle case.
- PBT passes before lifecycle churn; now measure its terminal report directly, without the older OOB polling completion helper.
- Temporary benchmark accounts hit a swallowed welcome exception; added optional disposable login traceback instrumentation to identify it.
- User explicitly requested map optimization; delegated Go map implementation and benchmarks to /root/map_performance, keeping baseline executable unchanged.
- Next: finish account setup diagnosis, run repeated matrix, review map correctness and before/after results separately.

## Concurrent harness acceptance
- Disposable welcome tracing proved E_PROPNF on #106.cloaked; setting the preference per selected player allowed all eight account logins.
- Toast smoke passed read-only/disjoint/hot-counter workloads at one and four clients with exact committed totals.
- Eight-client commands exposed missing eval command routing on an existing player ("I don't understand that"), not a measured MVCC speed result; correcting fixture features next.
- Map worker reports preliminary persistent-index construction improvement from ~700ms to ~1.6ms in package benchmarks; correctness/read/iteration checks still pending.
- Baseline binary remains frozen; full repeated matrix waits for successful harness acceptance, and invalid smoke results stay excluded.

## Baseline and isolated map verification
- Nine baseline worlds finished: all PBT and concurrent-counter checks passed; one g4 map eval timed out and its aggregate stays INVALID. Raw matrix copied under .tmp/performance-matrix-20260919.
- Repeated median PBT reported time: Toast2.020s, Barn g32 1.170s; empty range loop0.134ms vs0.511ms; g32 map construction187.024ms vsToast17.169ms.
- Map worker's final bulk builder removes the initial constructor slowdown; Windows full Go suite/types race passed. Unrelated admission/server/vm edits are preserved and excluded from map-only build.
- Isolated b2cb70b archive plus map files built; all Linux Go tests pass after supplying a tracked-file Git index required by repository hygiene. Initial archive-only root test failed solely for absent .git.
- Canonical managed Toast map suite is collecting; next run unchanged map suite on isolated Barn, then repeat live matrix with the map-only binary and commit owned files only.

## Oracle result and map-after matrix
- Focused managed canonical Toast completed168pass/1fail: existing wide-int map assertion expects two entries, but Toast collapses0 and2^32; leave assertion unchanged and report mismatch.
- After matrix running nine fresh worlds with frozen map-only binary; observed map insertion drops from~187ms to~3ms and all observed PBT/counter results remain correct.
- Early after-world concurrency is slower than baseline; investigating attribution before treating it as acceptable. Windows snapshot shows unrelated python PID51700 consuming6.62CPU-seconds in~1second.
- Map worker doing read-only live-heap/GC review; no concurrent agent benchmark work. Need discriminate host load from representation cost using controlled follow-up if repeated results remain worse.
- Next: finish matrix, managed Barn map check, compare full results and resolve performance concerns before committing owned files.

## Compact map candidate and paired follow-up
- First after matrix completed9worlds/0errors: g32 map2.725ms, but concurrency lower than baseline; retaining all results.
- Worker removed finalization temporary arrays and compacted trie leaves; frozen files copied into isolated b2cb70b source and final build/tests running.
- Added documented alternate candidate lanes and GC counters for concurrency-only paired runs, skipping unequal-duration preceding workloads.
- Managed Barn map check running; canonical Toast remains168pass/1existingwide-integer expectation failure. No assertions changed.
- Next: finish validation, run paired concurrency and final workload samples, update report, commit only owned map/tooling files; leave unrelated runtime edits and notes untouched.

## Final candidate verification
- Isolated Linux full Go suite and types race passed; candidate files match the five owned working-tree map files exactly.
- Managed final Barn maps168pass/1fail(first_last_index); focused frozen baseline reproduces identical mismatch(1pass/1fail). Canonical Toast passed that case but failed its separate wide-int expectation.
- Final compact representation retains~539KB per2000entrymap versus baseline524KB; tiny8 remains slower/larger and is documented.
- Four-round alternating baseline/final g32 concurrency-only matrix now running(session38254), with ten-second settle and GC counters; no competing owned benchmarks.
- Map files staged only, not committed; next finish paired/final corpus measurements, replace report placeholder, and commit owned work while leaving notes/unrelated edits.

## Paired result and map commit
- Eight paired worlds completed0errors/6240validatedrequests; g32 eight-client baseline→final ops/s: read1730.3→1638.0,disjoint1565.9→1544.1,hot617.0→641.2.
- Large first-candidate slowdown absent after compacting leaves/removing scan arrays and controlling phase order; residual5.3%read reduction remains documented, not waved away.
- Committed only five map files as4f05fd0 on fix/mongoose-workload; unrelated runtime/admission edits remain unstaged.
- Three final g32 interpreter/PBT worlds running(session10247); first world's map sample~2.07ms, PBT65/65, create/recycle still~28ms versus baseline16ms.
- Next finish final corpus, record full tradeoffs and validation limits, commit reusable scripts/docs separately, leave all notes uncommitted.

## Completed workload report
- Final corpus completed3worlds/0errors/360interpreter samples/9PBT suites65of65; map median2.276ms versus187.024ms baseline(~82x), PBT622ms versus1170ms.
- Lifecycle remains28.115ms versus16.032ms(~75%slower); paired8client reads5.3%lower; report explicitly records these tradeoffs and does not claim universal improvement.
- Map commit4f05fd0; reusable runner/report/docs commit7c7c39d, both on fix/mongoose-workload. Full isolated Go/race,7harness tests and script syntax passed; managed map failures are documented baseline/oracle differences.
- All raw matrices/reports retained in .tmp/performance-matrix-20260919 and /tmp/barn-perf-matrix-20260919; all owned server processes exited through harness cleanup.
- Notes remain uncommitted and unrelated runtime/admission changes untouched; next performance work is lifecycle/tiny-map profiling and short-command response latency.
