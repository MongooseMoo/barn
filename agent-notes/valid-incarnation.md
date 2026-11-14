# valid(object, incarnation)

## Goal
Barn's `valid` accepts and honours the Mongoose ToastStunt fork's second argument, exactly as the fork does, only when the `VALID_INCARNATION` runtime option is on (the Mongoose fork profiles). Otherwise `valid` stays exactly stock. Nothing else.

## State
- Candidate test `tests/mongoose-conformance/builtins/valid_incarnation.yaml`, committed alone first. Each test now declares `skip_if: "not option.VALID_INCARNATION"`, as `float_math_integer_promotion.yaml` declares PROMOTE_NUMBERS.
- `config`: `Options.ValidIncarnation`, default off, config key `VALID_INCARNATION`, feature `option.VALID_INCARNATION` in `FeatureMap` (profile manifests). It is set to 1 in `profiles/barn/mongoose-outbound-on.conf` and `mongoose-outbound-off.conf`, and `profiles/barn/profiles.json` expects it on the four profiles that use those confs (linux and windows; `profile/registry_test.go` checks all four).
- `builtins/objects.go`: `valid` is registered as stock, 1 argument of any type, plus `incarnationArgs(STR)`. After a true one-argument result it reads `incarnation_id` with `FindProperty` (ancestors, clear values), returns 0 when missing, E_PERM when unreadable (wizard, property owner, or `r`), 1 only for a nonempty exactly equal STR.
- `builtins/descriptors.go`: one option-signature mechanism shared with #825's `PromotedFloat`. `Signature.IncarnationArgs` are optional trailing positions; `optionSignature()` builds the option signature (FLOAT positions -2 when PromotedFloat, IncarnationArgs appended); `optionEnabled(options)` is PromotedFloat with PROMOTE_NUMBERS, or IncarnationArgs with VALID_INCARNATION. Descriptor validation forbids both on one signature. `builtins/registry.go`: each entry precomputes `optionSig`; `builtinEntry.signature(ctx)` picks it while its option is on; dispatch, the `Get`/`call_function` path and `function_info` all use it. #825's per-call `argType`/`promote` parameters are gone. PROMOTE_NUMBERS no longer affects `valid`.
- `builtins/valid_signature_test.go`: with PROMOTE_NUMBERS on and VALID_INCARNATION off, `{"valid", 1, 1, {-1}}` and E_ARGS "Incorrect number of arguments (expected 1; got 2)"; with VALID_INCARNATION, `{"valid", 1, 2, {-1, 2}}`, E_TYPE for a non-STR second argument, E_ARGS for three.
- `builtins/catalog_contract_test.go`: historical catalog hash updated, one comment line per change (PromotedFloat from #825, IncarnationArgs here).
- PR CI run 43565771256 had 4 stock-suite failures, all from the 1-2 signature: `msg_args_error_names_got_two`, `msg_args_error_names_expected_and_got`, `valid_function_info_signature_matches_source`, `valid_too_many_args`.
- Managed candidate lane, fork conf (`mongoose-outbound-on.conf`, profile label `barn-linux-testdb-fork`, oracle manifest = #816's `mongoose-0ff6fd7-wsl-testdb.json`): `valid_incarnation.yaml` + `float_math_integer_promotion.yaml` 11 passed, 2 skipped (stock-only float cases). Stock conf with the stock oracle manifest: `valid_incarnation.yaml` 5 skipped "Requires option: VALID_INCARNATION", admission passed.
- Toast first: 5 of 5 passed on the fork built at the live commit, through the managed harness (profile `profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`, branch `managed-toast-live-commit`), before the yaml gained its skip_if.
- Pull request open; merged with master (including #825).

## Decisions and why
- No new per-object state and no database format change: the fork keeps incarnation as an ordinary property (`src/objects.cc` bf_valid; `git grep incarnation` in its `src/` finds only that and the connection close fact in `server.cc`).
- Q 2026-11-14 chose a dedicated `VALID_INCARNATION` option over PROMOTE_NUMBERS, since the fork registers `register_function("valid", 1, 2, bf_valid, TYPE_ANY, TYPE_STR)` unconditionally (`src/objects.cc:1398` at 0ff6fd7), not under PROMOTE_NUMBERS (a separate `#define`, `src/include/options.h:502`). `BarnExtensions` was ruled out: it is in `config.DefaultCapabilities()` and on in the stock CI profile.
- The option is reported in the feature map only, as Q directed; it is not added to `server_version()` output, which mirrors Toast's compile options.

## Surprises
- The original two-argument signature also changed the stock arity message for `valid()`: `functionArgError` only formats "(expected N; got M)" when MinArgs == MaxArgs.
- The harness reads `skip_if` options from the target manifest only when `--oracle-profile-manifest` is given. Without it, it probes `server_version("options.VALID_INCARNATION")`, which neither Barn nor Toast answers, and the test errors. A Barn profile id containing `-mongoose-` also forces `database_fixture` to `mongoose` (`profile/manifest.go` fixtureID), so pairing the fork conf with a Test.db oracle needs a `-testdb-` label.

## Next step
- The fork Toast manifest from #816 (`profiles/toast/mongoose-0ff6fd7-wsl-testdb.json`) lacks `option.VALID_INCARNATION`; a Toast run of this yaml with `--target-profile-manifest` set to it falls back to the server_version probe, so the tests error or skip instead of running (which one is not verified). Adding `"option.VALID_INCARNATION": true` there belongs to #816.
