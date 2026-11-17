# toast-2.8-map-key-order

## Goal

Match Barn's map key ordering to stock Toast after upstream c0966e5 ("Order
waifs, anonymous objects and bools in compare()") and 7afd3e9 ("Fix compare()
truncating 64-bit integer and object map keys to 32 bits"), both in the
03af4fa oracle (Barn PR #862). Conformance side: moo-conformance-tests branch
`toast-2.8-map-key-order`.

## State

Done on branch `toast-2.8-map-key-order`. Not pushed (the orchestrator
publishes).

## Decisions and why

- `types/map.go` `toastMapCompare`: bools compare false < true; anonymous
  objects by id; waifs by the bytes of their `WaifIdentity`. Toast orders waifs
  and anonymous objects by pointer address, which is arbitrary but fixed for
  the value's lifetime. Barn has no stable addresses; the identity token and
  the anonymous id are the equivalent, and any total order consistent with
  equality gives Toast's observable behaviour (key order independent of
  insertion order, every key reachable). INT and OBJ already compared full
  width with `<`/`>`, so 7afd3e9 needed no Barn change; they now use
  `cmp.Compare`.
- `vm/op_property.go` `orderedLookupKey`: bool and waif keys are now totally
  ordered, so `maphaskey(obj.prop, key)` reads only the tested entry for them.
  Anonymous keys keep the whole-slot read: maphaskey rejects them with E_TYPE
  today (another branch is changing that builtin), and the whole-slot read is
  the safe default.
- `db/format/roundtrip.go` map comparison: compares a's tree order with b's
  insertion order. The writer emits tree order and the reader inserts in file
  order, so that pairing holds whatever the key type; b's own tree order can
  differ because a portable reload gives waifs fresh identities. The old code
  expected bool/waif/anon keys to reverse on every dump.

## Surprises

- Barn's float compare returns 0 for NaN against anything; Toast returns 1
  both ways. Unchanged here (not part of these commits).
- The old comparator's "reversal on every dump" was baked into the round-trip
  comparator, the snapshot anon-id rewrite comment and four Go tests.

## Evidence

- RED on master: new tests in `types/boolean_map_test.go`,
  `types/value_struct_test.go`, `vm/map_key_maphaskey_test.go` and
  `db/format/roundtrip_test.go` failed (compare(false,true) = 1; GetWithCase
  missed `true`; waif/anon compare not antisymmetric; bool maphaskey lost
  validation on an unrelated delete; waif-keyed round trip reported
  "property waifs value/metadata differs"). All pass after the change.
- gofmt clean; `go vet` and `go test` pass for types, vm, builtins, db/....
- Map benchmarks (`go test ./types -bench Map`), baseline 33eb9f38 vs this
  branch, interleaved test binaries, 4 rounds x 2: minima within noise
  (GetWithCase case-sensitive 100/1000/5000: 323.9/566.1/887.5 ns vs
  325.7/564.8/836.7 ns; MapBuild/2000 2.16 vs 2.13 ms). allocs/op vary
  run to run in both binaries on the loaded workstation.
- Managed conformance on the conformance branch, WSL as root:
  `make conformance-toast` and `make conformance` with
  `K="bool_map_keys_order or key_map_order_is_independent or boolean_and_equal_integer or identity_key_topology or full_width_ or int_keys_separated"`:
  Toast 03af4fa 9 passed; Barn 9 passed.

## Next step

Publish with the conformance branch; Barn CI must run against the conformance
change.
