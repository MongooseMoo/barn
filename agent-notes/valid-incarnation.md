# valid(object, incarnation)

## Goal
Barn's `valid` accepts and honours the Mongoose ToastStunt fork's second argument, exactly as the fork does, in the Mongoose fork profiles only. In the stock profile `valid` stays exactly stock. Nothing else.

## State
- Candidate test `tests/mongoose-conformance/builtins/valid_incarnation.yaml`, committed alone first.
- `builtins/objects.go`: `valid` is registered as stock, 1 argument of any type, plus `promotedArgs(STR)`: under `PROMOTE_NUMBERS` it admits a second STR argument. After a true one-argument result it reads `incarnation_id` with `FindProperty` (ancestors, clear values), returns 0 when missing, E_PERM when unreadable (wizard, property owner, or `r`), 1 only for a nonempty exactly equal STR.
- `builtins/descriptors.go`: `Signature.PromotedArgs`, optional trailing positions admitted only under `PROMOTE_NUMBERS`. `builtins/registry.go`: each entry precomputes `promotedSig` (nil when there are none); `builtinEntry.signature(ctx)` picks it under `PROMOTE_NUMBERS`, and dispatch, the `Get`/`call_function` path and `function_info` all use it.
- `builtins/valid_signature_test.go`: stock gives `{"valid", 1, 1, {-1}}` and E_ARGS "Incorrect number of arguments (expected 1; got 2)"; `PROMOTE_NUMBERS` gives `{"valid", 1, 2, {-1, 2}}`, E_TYPE for a non-STR second argument and E_ARGS for three.
- `builtins/catalog_contract_test.go`: historical catalog hash updated (Signature gained a field; valid's is STR).
- PR CI run 43565771256 had 4 stock-suite failures, all from the 1-2 signature: `msg_args_error_names_got_two`, `msg_args_error_names_expected_and_got`, `valid_function_info_signature_matches_source`, `valid_too_many_args`.
- Managed stock lane (`outbound-on.conf`, `-k "capability_admission or msg_args_error_names or valid"`): 4087 passed, 47 skipped.
- Managed candidate lane for `valid_incarnation.yaml`: with `mongoose-outbound-on.conf` 6 passed (5 + admission); with `outbound-on.conf` 5 failed with E_ARGS, the stock behaviour.
- Toast first: 5 of 5 passed on the fork built at the live commit, through the managed harness (profile `profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`, added by the branch `managed-toast-live-commit`; its features include `option.PROMOTE_NUMBERS: true`).
- Pull request open; merged with master.

## Decisions and why
- No new per-object state and no database format change: the fork keeps incarnation as an ordinary property (`src/objects.cc` bf_valid; `git grep incarnation` in its `src/` finds only that and the connection close fact in `server.cc`).
- Gate is `PROMOTE_NUMBERS` (`RuntimeOptions.PromoteNumbers`), not the `BarnExtensions` capability. `BarnExtensions` is in `config.DefaultCapabilities()` and the stock CI profile `profiles/barn/outbound-on.conf` sets no `BUILTIN_CAPABILITIES`, so it is on in the stock run and could not keep `valid` stock; it also only includes or drops whole builtins. `PROMOTE_NUMBERS` is the one existing option that is on exactly in the fork profiles (`profiles/barn/mongoose-*.conf`) and off in the stock ones.
- Known mismatch: the fork does not tie `valid`'s second argument to `PROMOTE_NUMBERS`. It registers `register_function("valid", 1, 2, bf_valid, TYPE_ANY, TYPE_STR)` unconditionally (`src/objects.cc:1398` at 0ff6fd7); `PROMOTE_NUMBERS` is a separate `#define` (`src/include/options.h:502`) that the fork build also has on. Here the option stands for "the Mongoose fork profile".
- The candidate yaml was left unchanged: it describes the fork, and CI does not run it.

## Surprises
- The original two-argument signature also changed the stock arity message for `valid()`: `functionArgError` only formats "(expected N; got M)" when MinArgs == MaxArgs.

## Next step
None for this PR beyond green checks and merge. Separately, if more fork-only behaviour that is not number promotion needs a gate, an option that names the fork profile would be more accurate than `PROMOTE_NUMBERS`; that is a decision for Q.
