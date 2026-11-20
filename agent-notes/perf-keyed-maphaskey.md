# perf/keyed-maphaskey

## Goal
Goodput campaign (Q authorised Barn changes, 2026-11-09). Mongoose's physics
engine #9655 tests presence in its ~1,300-key `pending` map with
`maphaskey(this.pending, key)` (rearm_one lines 12 and 22, rearm_falls line 9).
That call read the whole property, so the task conflicted with every writer of
any key; #9655.pending was 16-28% of lost commits, and 490 of 782 losses in one
census were at rearm_one line 22. The verbs cannot use a catch expression
(`this.pending[key] ! E_RANGE`): the object's own test bans them.

Make `maphaskey(obj.prop, key)` with a literal property name depend on the one
entry it tests, returning exactly what maphaskey returns today.

## State
Done, committed on the branch, not pushed (the caller said not to).
- compiler/lower_expressions.go: `keyedMaphaskeyProperty` picks
  `maphaskey(<expr>.<name>, key)`: exactly two arguments, neither spliced, the
  first a PropertyExpr with a static name (`obj.prop`, `this.prop`, `$x.prop`,
  any object expression, as `obj.prop[k]` accepts). `compileKeyedMaphaskey`
  emits obj, OP_GET_PROP_KEYED name, key, OP_MAPHASKEY_KEYED name func, then the
  ordinary OP_CALL_BUILTIN maphaskey 2.
- bytecode: OP_MAPHASKEY_KEYED [name:short, func_id:short], pops key, coll, obj
  and pushes coll, key; tick-free; verifier checks the name constant.
- vm/op_property.go `executeMaphaskeyKeyed`: when coll is a map and obj an
  object, ObserveMapKey (entry present/absent plus value, the keyed-read record)
  if the key passes `orderedLookupKey` (integer, object, error, string, non-NaN
  float) and `Session.MayRedirectByID` says the call cannot go to
  #0:bf_maphaskey; otherwise `StoreTxn.ReadMapKeyedSlotWhole` (one-slot degrade
  to a whole read at the first-seen version, built on #685's degradeMapKey).
- builtins/registry.go: dispatch's redirect test is `offersRedirect`, shared by
  dispatch and `MayRedirectByID`.
- Tests: vm/map_key_maphaskey_test.go (five tests); vm/map_key_test.go gained
  `runOnThisWith` (context setup hook) behind `runOnThis`. Before the production
  change, TestMaphaskeyOfPropertyDependsOnTheOneEntry failed its three commit
  cases with a whole-property conflict on state, and the equivalence test failed
  only on "compiled keyed=false". The unordered-key test fails with
  orderedLookupKey forced true ("reader commit = E_NONE; want a lost
  validation").

## Decisions and why
- The result is computed by the unchanged OP_CALL_BUILTIN, so redirect, argument
  validation, E_TYPE/E_ARGS, ticks and the builtin body are the same code as
  before. The new op only chooses which dependency is recorded.
- Equivalence of the dependency: two-argument maphaskey is
  `GetWithCase(key, false)`. For a string key that is `MapGet(key)` (keys
  compare case-folded). For other keys it walks the map's Toast-ordered tree
  with toastMapCompare, which orders integers, objects, errors and non-NaN
  floats totally and puts each type in one band, so the walk finds the key
  exactly when MapGet does. The recorded entry is MapGet's presence and value,
  and validation re-runs it on the live map. Error outcomes depend only on
  types: a non-map value is never tracked keyed (keyedSlot), and the slot is
  validated as still a map with the same owner and perms.
- Boolean, WAIF and anonymous keys and NaN read the slot whole: toastMapCompare
  calls two distinct ones unordered, so the walk depends on the other keys.
  `bin/moo`: `m = [true -> 1, false -> 2, 1 -> 3]` gives maphaskey(m, true) = 0
  with m[true] = 1. Whether Toast answers the same for that map was not checked
  (no oracle run; outside this change).
- Three-argument form left whole: a case-sensitive match depends on how the
  stored key is spelled, which the entry record does not hold.
- Protected redirect is decided at run time (Context.ServerOptions can change
  the set within a task), so the op asks at call time and reads the slot whole
  whenever dispatch would offer the redirect, even if the wrapper is missing.
- `k in mapkeys(obj.prop)` NOT done: `in` returns k's 1-based position in
  mapkeys' tree order, which depends on every key ordered before k, so it is no
  single-entry dependency.

## Surprises
- The opcode byte space was full (OP_UNWIND_JUMP is 255). OP_MAPHASKEY_KEYED
  takes the number of the dead OP_RAISE, which no compiler emitted and the
  decoder rejected, so no persisted checkpoint program can hold it. An older
  Barn loading a checkpoint suspended between this op and its call would reject
  the frame, as for any new opcode.
- A task whose work ends before the boundary validates nothing at its final
  commit: the boundary test writes after the boundary so the carried presence
  read matters.
- The recorded entry includes its value, so a concurrent change of the value at
  the tested key (presence unchanged) still loses the reader. Same as keyed
  reads; kept.

## Next step
Push and open the PR when the caller says so; CI runs the full suites. Measure
#9655.pending losses with the real-workload harness after merge.
