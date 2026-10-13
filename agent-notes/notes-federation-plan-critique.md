# Federation plan critique — session notes (2026-08-15)

Task: Q asked for a critical review of `plans/moo-federation.md` against how actual
MOOs work (Barn source + ToastStunt at `~/src/toaststunt`, i.e. `C:/Users/Q/src/toaststunt`).
Deliverable was the assessment (delivered in conversation); no fixes applied.

## Method

- Read the full plan (`plans/moo-federation.md`, 743 lines).
- Dispatched two Explore agents in parallel to verify every code citation:
  one against Barn (17 claims), one against Toast (12 claims). Both completed.
- Personally verified clear-property semantics (`spec/objects.md` §4.4,
  `db/store/store_properties.go:125-131`) and the connection-builtin surface
  (`spec/builtins/network.md`).

## Verification results (headline)

Nearly all citations TRUE. Corrections found:
- `task_runtime.go:404-434` → now `:412-428` (drift ~+8).
- Toast `server.cc:1934` → now `:2015`; waif_conversion is a CLI-flag migration,
  NOT a DB-version-boundary one as the plan says.
- `create()` not fully ungated: explicit-owner + fertility gates exist
  (`objects.go:188-232`); only default-owner path is free.
- Barn `$name` desugars to static `#0.name` node (`parser/parser.go:249-254`),
  not dynamic `#0.("name")`.
- Input pipeline: 256-slot shared ingress demuxes to per-connection 64-slot
  lanes (`input_processor.go:221-257`) — plan slightly overstates.
- `reader_task.go` claim understated: ALL suspend flavors round-trip to
  E_INTRPT/queued (`reader_task.go:285-292`).
- Tag 15 story confirmed sound (Toast `structures.h:99-119`, append-only comment
  :87-98, _TYPE_ITER=11, BOOL=14 last). Tag 11 reservation undocumented in Barn.
- `notify` is a PENDING EFFECT, not irreversible (`registry.go:437-440`) —
  supports the buffered-sends answer; plan missed it.

## Problems found (numbered as delivered to Q)

1. **Visitor has no connection.** notify() is a silent no-op on non-connected
   objects in BOTH engines (Barn `network.go:790-793`, Toast `server.cc:3038-3043`).
   Output relay needs a notify intercept nowhere in the plan. read() keyed to
   tqueue w/ real connection (Toast `tasks.cc:1363-1378`) — editors/pagers break
   abroad. connection_name/idle_seconds/boot_player/set_connection_option: no rulings.
2. **Visitor visibility undefined.** players() checks FlagUser
   (`objects_players.go:17-32`); connected_players() iterates connection table
   (`connection_manager.go:578-579`). Visitor appears in neither; is_player false;
   core :announce loops make them invisible. user_connected hooks unaddressed.
3. **Command parsing hidden.** "Input lines as eventual sends" — someone must run
   the command parser with player=visitor at the destination; those command tasks
   need the accountable-principal stamp or quotas/create() check are bypassed.
4. **Send ordering.** Wire has seq numbers for redelivery only; E guarantees
   ordered delivery. Barn scheduler runs delivered tasks concurrently under MVCC →
   two tells can visibly swap. Needs per-(session,target) serial rule or explicit
   "unordered" ruling.
5. **Replicas miss property VALUES.** Clear props read nearest ancestor's VALUE
   (verified both engines). Plan replicates "code + propdefs" only; parent
   property values are live data. Delta-stream question unasked.
6. **Replica ancestor chain unspecified.** Transitive subscription to foreign
   root vs re-parenting onto local core — both break stated claims (late-binding
   vs pass()/lineage). Hardest structural question, unanswered.
7. **Carriage home unrepresentable.** TYPE_REMOTE can't be in contents/location;
   "travel home as references, reconciled on return" has no mechanism. Also:
   persistent objects inside non-persistent visitor bodies at checkpoint/reap =
   dangling locations.
8. **Send transmit-time ruling missing.** Buffer-at-commit (promises can't resolve
   intra-txn; killed task silently drops fire-and-forget sends) vs send-immediately
   (IrreversibleSideEffect, leaks from aborted txns). Plan never picks.
9. **E_REMOTE gets no compat treatment.** Errors persist as bare ints
   (Toast `db_io.cc:369-374`); flag-off/Toast readers get a nameless error;
   Toast owns error numbering. Tag 15 got a paragraph; E_REMOTE got nothing.
10. **Positive VM surface understated.** prox.name / verb send on TYPE_REMOTE are
    new opcodes cases; two dispatch paths (`op_verb.go:255` AND `:474/:493`).
11. **Trust tier vs dialback contradiction.** Visits need *known*, but dialback
    retained "for stranger tier" — applies to nothing as written.
12. **Portals from abroad don't compose.** Visit requests signed by home; B can't
    sign for onward travel to C. Relay chaining/re-homing undesigned.
13. **"Green = behavioral identity proved" overclaims.** Data late-binding
    (property values, $server_options, eval/verb_code reflection) breaks it.
    Should claim "code identity" only.
14. **Replica pull = wizard-level foreign code exec**; diff tooling must show raw
    source, never normalized ASTs (normalization is a place to hide).
15. **Principal reaping vs stamped forked tasks** — dangling accountability;
    needs liveness rule.

Summary line delivered: plan is excellent below the waterline (wire/caps/types/
quotas), thin exactly where it becomes a MOO — connections, command loop, player
visibility, parent property values, containment. Phase 4 (People) is the shortest
phase and should be the longest.

## State / next steps

- Critique delivered in conversation; NOT yet incorporated into the plan.
- Next step (if Q wants): rework `plans/moo-federation.md` — biggest additions
  would be a visitor-presence design (pseudo-connection in the connection table
  looks like the natural mechanism given how notify/read/boot_player key), the
  replica value-delta + ancestry rulings, send transmit-time + ordering rulings,
  and an E_REMOTE compat paragraph mirroring the tag-15 one.
- No blockers. Toast source confirmed readable at C:/Users/Q/src/toaststunt.
