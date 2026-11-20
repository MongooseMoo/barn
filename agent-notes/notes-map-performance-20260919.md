# Map performance investigation

- Parent supplied same-Linux 2000 insert timings: Toast 0.016240s; Barn 0.704816s.
- Source finding: every MapSet clones the entire hash table and insertion-order slice.
- Investigating a persistent representation preserving aliases and exact lookup topology.
- No implementation changes yet; next establish benchmark and bound patch scope.

- Implemented immutable hash-trie path copying and persistent insertion order; Toast lookup reconstruction retained.
- Windows 2000 insert singles: before 635-856ms/~453MB, initial after 1.33-1.83ms/~1.93MB; not a live Linux claim.
- Snapshot/order/delete/reinsert/collision/concurrent-alias tests added; types race and full Go suite passed.
- Delete still copies the newer order prefix; direct reads measured about 41-53ns before/after.
- Parent owns live Linux verification; holding CPU benchmarks and commits for coordination.

- Bulk benchmark exposed 31-44% constructor slowdown and 8002 allocations for 2000 pairs versus baseline33.
- Added unpublished block builder for leaves/order; branches stay separate to avoid retaining obsolete whole tries.
- Bulk512+ pair alias/duplicate/order regression passes; published nodes remain immutable.
- Parent requested quiet CPU during live matrix, so final builder benchmarks and full suite await its signal.
- Scope remains five types implementation/test files; no commits or binary changes by this agent.

- Final bulk medians:16pairs8801 to7038ns,2000pairs1209483 to1058989ns; fresh five-run Windows comparisons.
- Bulk2000 bytes fell955KB to751KB; allocations789 versus baseline33 and initial prototype8002.
- Leaf blocks trade fewer allocations for up to31 neighboring entries retained by a surviving leaf; branches remain separate.
- Final types race passes1.715s; full Go suite currently running while parent prepares isolated map-only live build.
- Performance sampling stopped; no binaries or commits produced here.

- Compact leaves now omit16 unused child pointers; branch mutations clone owned child arrays in a single branch allocation.
- Finalization scans now allocate zero and stop early; insertion-order output no longer builds a second array.
- Retained2000-map footprint baseline524KB versus candidate539KB;8-key case remains larger/slower, recorded for parent.
- Compact candidate constructor2000 median667us versus baseline1487us in three short Windows samples; types race passed1.941s.
- Files frozen for paired live workload comparison; latest full suite still running, no source edits planned.
