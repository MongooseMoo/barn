# valid(object, incarnation)

## Goal
Barn's `valid` accepts and honours the Mongoose ToastStunt fork's second argument, exactly as the fork does. Nothing else.

## State
- Candidate test `tests/mongoose-conformance/builtins/valid_incarnation.yaml`, committed alone first.
- `builtins/objects.go`: `valid` is 1-2 args, second must be STR; after a true one-argument result it reads `incarnation_id` with `FindProperty` (ancestors, clear values), returns 0 when missing, E_PERM when unreadable (wizard, property owner, or `r`), 1 only for a nonempty exactly equal STR.
- `builtins/catalog_contract_test.go`: historical catalog hash updated for the new signature.
- Managed candidate lane: unchanged Barn 5 failed (E_ARGS), changed Barn 6 passed (5 + admission).

## Decisions and why
- No new per-object state and no database format change: the fork keeps incarnation as an ordinary property (`src/objects.cc` bf_valid; `git grep incarnation` in its `src/` finds only that and the connection close fact in `server.cc`).

## Surprises
- No managed Toast build has the feature. The pinned Mongoose Toast (`profiles/toast/mongoose-wsl-mongoose.json`) and stock Toast both predate the fork commit that added it, and the Mongoose profile is pinned to the Mongoose database, not Test.db. Stock Toast raises E_ARGS on every two-argument case. The Toast-first proof is missing.

## Next step
A managed WSL build of the fork at or after the commit "Validate a captured object incarnation in one permission-checked native call", with a Test.db profile, so this file can be shown green on Toast. Not done here: it needs a new oracle build and manifest.
