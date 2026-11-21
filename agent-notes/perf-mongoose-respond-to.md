# Reuse the canonical verb name in respond_to

## Goal

Reduce work in the native respond_to builtin on the Mongoose workload.

## State

The November checkpoint profile charged 2.98s to respond_to, including 1.78s in lookupVerbDispatchMemo, 0.15s in InternStr and 0.09s in strings.Join. In builtins/verbs.go:104 a readable match rebuilds the name from verb.Names. db/store/verb.go:287 already exposes the joined name. Renaming recomputes it at db/store/store_verbs.go:347.

## Decisions and why

The readable response reuses the canonical name up to the intern cutoff. Longer names take the original join path, preserving single-alias and multi-alias descriptor identity. The types package owns the cutoff. The added test renames a verb and exercises 64-byte and 65-byte aliases through builtin dispatch and the store's descriptor lookup.

The scoped test and benchmark command was `go test ./builtins -run '^TestRespondTo' -bench '^BenchmarkRespondToCanonicalName$' -benchmem -benchtime=150ms -count=5`. Parent implementation output for aliases was 559.7, 573.0, 505.9, 491.1 and 482.9 ns/op, each 112 B/op and 3 allocs/op. Candidate output was 369.5, 377.1, 379.4, 381.6 and 373.8 ns/op, each 96 B/op and 2 allocs/op. Both runs returned PASS. The exact, case and wildcard benchmark cases stayed at 96 B/op and 2 allocs/op. Windows amd64, AMD Ryzen 9 5950X. This measures the builtin benchmark only.

## Surprises

Raw string identity is observable. db/store/store_txn_verbs.go:304 accepts a pointer-identical names string as a descriptor even when its full multi-alias text would fail ordinary matching. InternStr copies on a short cold miss and on a hit returns the pool's owned bytes. Above 64 bytes it passes the input directly to NewStr, which retains those bytes. A blanket replacement of strings.Join by verb.NamesString changes long multi-alias descriptor behavior. The Toast source at src/verbs.cc:577 explicitly returns str_ref(db_verb_names(h)). Changing Barn's descriptor behavior deliberately requires managed Toast verification on both sides of the length cutoff. This PR can preserve the existing contract and isolate the measured optimization.

## Next step

Lead publishes the scoped branch and monitors PR CI. End-to-end Mongoose speedup remains unmeasured.

## Draft PR

**Reuse canonical names for short aliased respond_to results**

respond_to already receives the verb's canonical alias string from the store. Reuse it when constructing the readable response and retain the current string ownership for long alias lists. The November Mongoose CPU capture charged 0.09s to the repeated join. In BenchmarkRespondToCanonicalName/aliases the median fell from 505.9 to 377.1 ns/op over five runs per build. Allocation fell from 112 to 96 B/op and from 3 to 2 allocs/op.
