# Managed Toast at the live fork commit

## Goal

Give Barn's managed conformance harness a Toast built from the Mongoose fork at
the commit the live game runs (`mongoosemoo/mongoose` 0ff6fd7), runnable on the
small conformance `Test.db`, beside the existing oracles and without changing
any of them. Then run the fork-only candidate tests of the `valid-incarnation`,
`validate-string-lines-waif-owner-count` and `float-math-promote-ints` branches
on it as their Toast-first proof.

## State

- Build: `/root/src/toaststunt-mongoose-0ff6fd7` in WSL Debian, cloned from the
  Windows checkout `C:\Users\Q\src\toaststunt` at 0ff6fd7 with no network,
  `cmake -S . -B build-release -DCMAKE_BUILD_TYPE=Release -DUSE_JEMALLOC=OFF`,
  `cmake --build build-release -j 8`. All dependencies were already installed.
- Profile: `profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`, checked by
  `TestCommittedToastOracleManifests`; usage in `docs/mongoose-conformance.md`.
- Results on that Toast through the managed candidate lane: valid_incarnation
  5/5, validate_string_lines 11/11, waif_owner_count 8/8, float math 5 passed
  and 2 skipped by their own `skip_if: option.PROMOTE_NUMBERS`.
- Pull request open.

## Decisions and why

- A new manifest instead of reusing `stock-wsl-testdb.json`: the profile gate
  in moo-conformance-tests requires oracle and target to agree on
  `option.PROMOTE_NUMBERS`, and the fork has promotion compiled in.
- The new manifest is passed as both oracle and target for a Toast run; Toast
  writes no `{manifest}` of its own.
- Build options follow `ops/executables/stage-release` in mongoose-ops, recorded
  there as what was done by hand for the live 0ff6fd7 build. The live binary
  itself was not inspected.

## Surprises

- None in the build. Test files from other worktrees were copied into this
  worktree's gitignored `.tmp/` for the run, because candidate mode confines
  the suite root to the candidate root and those worktrees were not to be
  written to.

## Next step

- The two non-promotion float-math cases have no Toast proof from this build
  (they skip here by design); they belong on the stock Test.db oracle.
- Whether the new profile should be wired into CI or the Makefile is a decision
  for Q; nothing shared was changed.
