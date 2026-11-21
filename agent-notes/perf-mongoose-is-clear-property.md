# Reduce is_clear_property receiver reads

## Goal

Reduce lookup work in the Mongoose is_clear_property path while retaining its transaction dependencies and MOO-visible results.

## State

The ordinary-property path now calls StoreTxn.FindLiveProperty and reuses its transaction for subsequent reads. The November 21 checkpoint recorded 2,171,492 entries. In the separate 45-second CPU profile the builtin accumulated 1.89 CPU seconds. Existence, inherited property lookup, and local clear-state reads accounted for 0.11, 1.15, and 0.49 seconds at their respective call lines.

## Decisions and why

Previously the builtin checked receiver existence then fetched the receiver again during inherited lookup. StoreTxn.FindLiveProperty calls existingObject once and supplies that image to findPropertyFrom on memo hits and the fallback ancestry walk. Builtin names use the existence check alone. The ordinary path translates a missing receiver's E_INVIND to E_INVARG and passes through a recycled receiver's E_INVARG and a missing property's E_PROPNF.

This removes a redundant root fetch without requiring a new store API. The independent local clear-state read remains necessary because a successfully resolved inherited property can have clear=false even when the receiver's slot is clear. It records the receiver's slot version or shape dependency. Read the programmer's wizard flag through the transaction as today.

## Surprises

PR #1097 changes property_info, set_property_info and reserved-name matching. Its diff leaves builtinIsClearProperty alone. Reuse its existing IsBuiltinPropertyName entry point so either merge order preserves the name rules.

## Next step

The focused builtin regression exercises defined, inherited, overridden, and cleared slots, a missing property, an unreadable property, builtin names, and missing/recycled receivers. Command: `go test -run '^TestIsClearPropertyLookup$' -count=1 ./builtins`. Result: `ok github.com/MongooseMoo/barn/builtins`. The existing TestFindLivePropertyMatchesObjectExistsThenFindProperty compares cold-in-transaction and repeated results and read sets, including scalar reads. The PR runs the pinned Elle workflow. Consult its healthy/fault verdicts and nonzero exercised path counts before making an isolation claim.

The final parent/candidate benchmark used identical warm snapshot transactions and a fixed million calls per case, nine runs per build. Execution order alternated by pair, with both processes pinned to the same logical CPU. The parent built the same benchmark using a Go overlay for the two changed production files. Both had zero allocations in every case.

| Case | Parent median ns/op | Candidate median ns/op | benchstat |
| --- | ---: | ---: | --- |
| Defined on receiver | 320.1 | 275.8 | -13.84%, p<0.001 |
| Inherited, clear on receiver | 276.1 | 257.8 | -6.63%, p=0.040 |
| Builtin name | 37.98 | 43.44 | Inconclusive, p=0.931 |

These are per-call microbenchmark results. An end-to-end Mongoose speedup has not been measured.

### Draft pull request

Title: Reuse the receiver lookup in is_clear_property

Use the transaction's existing combined existence and property lookup for ordinary properties. The receiver-local slot supplies the clear bit. Builtin names still require a live receiver and return zero. The November checkpoint entered this builtin 2,171,492 times. In the paired benchmark, the defined-property median fell from 320.1 to 275.8 ns/op and the inherited-property median fell from 276.1 to 257.8 ns/op.
