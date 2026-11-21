# Mongoose mapvalues

## Goal

Reduce the measured cost of the single-map form of mapvalues while preserving tree order and keyed lookup behavior.

## State

The comparator now checks folded equality before validating UTF-8. In the November 21 baseline profile, filtering to builtinMapvalues produces 410ms under #9655:set_wakeup and 370ms under #9655:wake. Reading the matching checkpoint with moodb identifies the call in #9655:wake at line 21. It enumerates engine.pending, finds due transitions, and at line 29 deletes the string key formed from the cell and fire verb. #9655:set_wakeup forks a call to $physics:wake at line 18. Root labels identify task ancestry.

## Decisions and why

Single-argument mapvalues called Pairs 691 times in the count run. In the independent CPU sample, Pairs took 780ms, toastRoot 750ms, toastLookupInsert 450ms and compareFoldedASCII 260ms. Filtering stacks to mapvalues gives 220ms in utf8.ValidString and 20ms in strings.EqualFold. These cumulative times overlap.

The old comparator fully validated both strings before checking whether they folded to the same text. With distinct ledger keys the equality test can reject at the first unequal folded character. All three predicates are pure. Keeping both validations in the equality branch preserves the bytewise ASCII-fold fallback for invalid encodings.

The benchmark reads all 2,043 string keys from the November 21 17:55 checkpoint's #9655.pending, in the order printed by moodb prop --full. It inserts the keys in an empty map with integer placeholder values. The cold operation replaces the first value and enumerates Pairs on the resulting indexed map, as mapvalues does. The warm operation enumerates a map whose ordered tree is already populated. Go 1.27.1 on Windows/amd64, one logical CPU, six independent runs per binary in interleaved order, 350ms target per subbenchmark. Raw inputs, benchmark source, overlay and outputs are local in .tmp/mapvalues.

| Operation | Baseline median | Candidate median | benchstat |
|---|---:|---:|---|
| Cold | 2.532 ms/op | 1.467 ms/op | −42.06%, p=0.002 |
| Warm | 88.10 µs/op | 83.14 µs/op | p=0.485 |

Allocation count and bytes showed no significant difference at either temperature. TestCompareFoldedMapKeys passed on representative ledger strings, ASCII and Unicode fold-equivalent strings, and unequal invalid UTF-8 encodings.

The separate Linux/amd64 capture used the same checkpoint and catalog hashes, loopback namespace, PGO build, 15-second post-ready delay and 45-second sampling window as the baseline. The sampled candidate used 50.20 total CPU seconds, with 28.16s under #9655:wake and 16.14s under #0:server_started. Mapvalues received 190ms (120ms under #9655:set_wakeup and 70ms under #9655:wake), including 10ms in UTF-8 validation. The baseline used 58.94s total and spent 780ms under mapvalues. The two windows ran different amounts of background work. The fixed-key result above measures the speedup. Raw candidate profile and server log are in .tmp/mapvalues/runtime.

## Surprises

types/map.go:477 inserts into the mutable tree owned by one sync.Once builder. It does rotations in place. A redesign to remove persistent path copies would target work this function does not perform. types/map.go:692 materializes key/value pairs, but most sampled CPU occurs before that traversal. Open PR #1090 exports the map comparator and changes VM range-bound checks. Its files omit types/str.go and its purpose is unrelated to comparison cost.

## Next step

The parent publishes the implementation and follows PR CI.

## Draft PR text

**Avoid validating unequal string keys while building the ordered map tree**

The Mongoose physics engine enumerates its pending transition ledger with mapvalues. In the baseline capture, CPU samples under mapvalues included 220ms in UTF-8 validation out of 780ms total. Test folded equality before validating both strings. Distinct folded keys bypass the full validity scans, while fold-equal strings still require both checks. Cold enumeration with the checkpoint's 2,043 keys fell from 2.532 to 1.467 ms/op (−42.06%, p=0.002, six samples per build). Warm enumeration and allocations showed no significant difference.
