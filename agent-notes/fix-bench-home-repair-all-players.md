# fix/bench-home-repair-all-players

## Goal

Make TestMongooseRealWorkload (engine/mongoose_real_bench_test.go) valid above ~50 players.
Players #2766 and #2784 have `.home == #62` ("Generic gun"), whose inherited accept returns 0,
so their `home` command raises E_NACC in move(). Toast raises the same (objects.cc:158), so it is a
fixture defect. The harness already repaired this for #71 only. Q's scope: repair anyone's home
that refuses them (every player in players()), never a hardcoded list.

## State

Done, committed on the branch, not pushed. The harness's fixture repair block now runs a one-shot
eval as #2 over `players()`: for each player whose home is a valid object other than
$player_start, refuses them (`h:accept(p)` false, or no accept verb), and whom $player_start
accepts, set home to $player_start. It logs the count and the list. The old #71-only lines and
the home element of the sqlite polling repair are removed; the polling repair's `{1, {1,` check
is unchanged. BARN_MONGOOSE_REPAIR=0 still skips all repairs.

Verified at 64 players (warmup 2s, measure 10s, dump copy mongoose-2026-11-10T0143.db, real
sound catalog via BARN_MONGOOSE_SOUND_DB):
- before: failed=19 and failed=16 (all `home`; samples are uncaught E_NACC from #2766/#2784
  plus 2 unsettled).
- after: repaired `{4, {#71, #2766, #2784, #13139}}`; failed=3 (2 unsettled `home`, 1 `look`
  killed-before-completion), no E_NACC.

## Decisions and why

- Use `accept`, not `acceptable`: move() calls `where:accept(what)` (Toast objects.cc:135), so
  that is the refusal `home` hits. A missing accept verb counts as refusal, as in move()
  (objects.cc:143-144).
- Separate one-shot eval instead of inside the sqlite polling loop: the polling eval reruns until
  sqlite opens, and a rerun would report 0 repairs after the first pass committed them.
- `p.home ! E_PROPNF => $nothing`: #10244 (nameless child of #5, location #-1) is in players()
  but has no home property, so no home refuses it. Any other error fails the repair loudly.

## Surprises

- The diagnosis's E_TYPE from `$player_start:acceptable(...)` under `barn -eval` is a tooling
  defect, not a harness problem. #592's `_handlers()` ends with two waif exits; #1:forbids:22
  calls `respond_to(waif, ...)`. The dump sets `$server_options.protect_respond_to` and
  #1584:bf_respond_to (inherited by #0) maps a waif to its class. The harness loads protected
  builtins (mongoose_real_bench_test.go, LoadProtectedBuiltinsFromStore), so the call works there.
  `internal/dbtool/dbtool.go` NewEvalSession calls only LoadServerOptionsFromStore, never
  LoadProtectedBuiltinsFromStore (server/server.go does both), though its comment says
  protected-builtin redirection works. Not fixed here.
- Raw Barn respond_to(waif) raises E_TYPE (builtins/verbs.go isObjectRef: OBJ/ANON only). Toast
  verbs.cc bf_respond_to accepts WAIF in is_object() and then calls is_valid(), which reads the
  waif union as an objid; the Toast result was not run on the oracle (UNKNOWN).
- Running the harness from a worktree without BARN_MONGOOSE_SOUND_DB gives an empty
  files/sqlite/sound.sqlite and every `say` fails; set it to the main checkout's catalog.

## Next step

- Remaining failures at 64p are unsettled commands (completion deadline), a separate known
  problem; not addressed and timeouts not raised.
- Q to decide whether to fix the dbtool protected-builtin gap.
