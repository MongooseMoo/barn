# Investigation: magic prerequisite observer

## Facts

- Failed cohort `.tmp/gameplay-magic-01` used checkpoint FAB6957ACFFA1DB8BC1C4B911FB53A8E8D6E605F4DE2A31B9F8E4FA874C3200D and accepted f96 executable. Manifest records owned shutdown exit0 and both files unchanged.
- Ordinary player #277801, private room #45668, tutor #45669 and fitted wand #45677 were created through the production factory and domain entry paths. The typed command log contains successful fitting, wielding, Lumos lesson and Nox lesson replies.
- Tagged read bp13 returned actual wand and actor lifetimes. Prerequisite read bp14 had no tagged result. Retained output reports existing news #2478 SQLite errors, not a prerequisite task frame.
- `quality_for` really exists with `{actor,skill}` and delegates to effective proficiency. Effective proficiency may differ from raw training seed; raw `$skills:proficiency` is the correct observer of the 60 seed.
- RecordingControl returns native parser values. MooObj subclasses int. The module compared raw snapshot locations against string fixture references; this is a separate observer identity mismatch.

## Competing hypotheses

1. A prerequisite expression/API/property access raises natively. Predicts a matching bp14 eval frame in shutdown `$wiz_utils.traceback_log`.
2. Teaching replies precede incomplete or different stored lessons/skills. Predicts missing lessons/ranks/raw skill seeds in shutdown actor properties.
3. The eval succeeds but transport loses its tagged output. Predicts no corresponding native eval frame and valid prerequisite state; requires fresh bounded split observers to distinguish further.

## Tests and limitations

- Read original source with moodb: training authorizes teacher, seeds raw proficiency through existing setter, and persists lessons/ranks before returning. This supports normal completed lessons but does not prove actual actor state.
- Read controller eval source: an empty failed bracket can be followed by unrelated background errors during the bounded tag wait. The news error does not identify the prerequisite root cause.
- Initial inspection accidentally supplied bare object and verb as separate moodb arguments, causing full-object source and an invalid-reference diagnostic. Correct syntax is `code '#16355:quality_for'`; no world mutation occurred.
- Shutdown `prop '#277801.hp_state' '#277801.skills'` returned both wand lessons1, charms rank1 and all four raw skills60. This rules out missing stored training for magic01.
- Shutdown `prop --full '$wiz_utils.traceback_log'` returned the bounded fifteen rows, including existing news SQLite/background errors, but no prerequisite eval frame. A frame may be absent for compilation or lost from bounded history; this does not prove transport failure.
- Actual parser regression before edits: `Ran 11 tests ... FAILED (failures=1, errors=1)`, raw `.tmp/magic-observer-red-01.txt`. After module-only observer/error preservation: `Ran 11 tests in 0.014s OK`, raw `.tmp/magic-observer-green-01.txt`.
- Module now normalizes only typed MooObj references recursively, retains ordinary integer quantities, and preserves primary native output/partial fixture through a secondary cleanup failure. No game code, training, RNG or command behavior changed.
- Next diagnostic splits prerequisite expressions into individually journaled reads; observes raw proficiency rather than effective/settling quality to test the actual training seed. All existing required lessons/rank/seed/catalogue/account assertions remain.
- WSL git diff check failed because the worktree .git contains a Windows absolute path; Windows `git -C ... diff --check` then completed exit0. Unit verification is separate from this infrastructure failure.

## Diagnosis from magic02

- Source hypothesis tested: account login might authenticate without assigning the player's `.account`. Existing `bench.create_account` only sets account players/default/email_verified; real `#3842:login_as_player` validates membership and switches connection, without assigning the character property.
- Owned magic01 shutdown `prop --full '#277801.account'` returns inherited clear value `#-1`. This is a fixture association defect, independent of legitimate fitting/training.
- Magic02 first split observer is `a=#277801; return string_hash(a.account.username);`. Its full preserved native output begins `#-1:Input to EVAL (this == #-1), line 68: Invalid indirection`, followed by normal eval frames. Later news errors had concealed the relevant first diagnostic in magic01.
- This source/state/native-output agreement explains the prerequisite failure: dereferencing the unassociated account. The transport correctly retains the complete error after its diagnostic repair.
- Parent owns the shared support repair: associate only the fresh ordinary actor with the freshly created private account in privileged fixture setup, as production account migration and delete-character fixtures do; verify identity/membership before login. Do not change game account/login code or fabricate ordinary authorization.
- Module remains frozen at 37e259125c2d0241f1e03c581df63c93632a6d6c0ad0cdf96bfb788b62e9e71f during the owned run. Actual Lumos/Nox and successful native cleanup remain unverified.

## Next action

Await magic02 terminal owned shutdown, then parent repairs only the private account fixture association and serializes magic03. Require real casting/resource/light/cleanup evidence before declaring gameplay valid.

## Magic03 observer wording correction

- Parent completed magic02 owned shutdown0, added exactfresh account association with rejection of unrelated existing bindings, and serialized magic03. Module core37e2591 stayed frozen for that run.
- Magic03 completed setup: actual account identity/membership, both ordinary lessons/rank/raw60, original catalogue and dark/idle snapshot all returned. Actual Nox-dark refusal left receipts/resources/recovery/light unchanged.
- Actual `weave lumos with 0 focus` returned `Choose between zero and 10000 focus, subject to the spell's smaller limit.` Before/after observers show receipt/debit counts0, joules100000, ready_at0, wand emission/light0 unchanged. The expected string was mistakenly the later backend preflight refusal.
- Original `#16980:weave` returns the parser reason immediately; `$hp_magic:parse` lines46-47 reject amount<=0 with that exact string. This is an observer contract mismatch, not a game failure.
- Native cleanup returned actor/wand/tutor/room removed1 and private account retained1. Parent reports owned shutdown0/allfive integrity checks true. No positive cast ran or was counted in magic03.
- Minimal module change: only that expected string, preserving every no-effect/resource/receipt/recovery/light assertion. Saved actual refusal rows are in `scripts/fixtures/magic03-refusals.json`, used by independent replay through the complete module negative-control path.
- Raw replay before correction `.tmp/magic-refusal-red-01.txt`: Ran12tests FAILED(errors1), expected ordinary refusal absent. After `.tmp/magic-refusal-green-01.txt`: Ran12tests0.022s OK. Windows diffcheck exit0.
- Freeze module3b041fdf5df88292405f2d83f1d8c9c80c549bcd53ac6309384b1cc4410bc1da for parent magic04. Actual casting/physical effects still await that run; no performance claim.

## Compact coherent observer before magic04

- Parent's actual flight04 proves eval output capped at1003 characters. Prior memory of6000 was wrong for this instance. Original magic idle map fits, but full formed receipt adds duplicate target metadata and result maps.
- Original start/complete/terminal/wand.apply source proves the receipt fields and native result shape. Source-derived tagged estimates: full Lumos1180/Nox1179, compact655/654; actual native lengths remain unknown until parent run. Raw `.tmp/magic-wire-budget-02.json` explicitly marks constructed terminal cases source-derived.
- Targeted fix keeps one native observation:21 positional fields; sparse index/value receipt projection preserving absent versus zero/empty; complete native result/pending/matching debit maps retained. All accepted oracle assertions stay unchanged. Native returns `length(toliteral(wire))`; Python rejects malformed/duplicate fields and payload>900 without truncation or normalization of quantities.
- Actual magic03 refusal rows are now copied literally, preserving integer-valued floats. Earlier PowerShell/JavaScript reconstruction collapsed those to JSON integers; original raw cohort remains intact. `.tmp/magic-wire-budget-01.json` and first compact unit log remain preserved; corrected fixture SHA2ae2545b287f5ee2f8f3ca7c1dd8ddebe2dfcec8eed35c6699ef68529fb0c41d and second logs supersede their native-type replay claim.
- No snapshot FileIO, extra observation task, gameplay/spec/proficiency/RNG change. Receipt selection is explicit in returned evidence, rather than claiming all internal preparation metadata was fetched.
- Raw `.tmp/magic-compact-green-02.txt`: Ran15tests0.023s OK, including actual idle/native parser, source-derived formed/fizzle oracle and malformed/oversized/presence checks. Windows diffcheck0.
- Parent magic04 and independent final source audit use frozen module90c96717e47f5d6cd2ff307dad2d6753b7d8cdd09cd6f7fa1ef46dbaf301eba7, tests3abff17b9ff7965eb7bd7eeeb50f17a295714d48187b532fa71ee29cabf035ea. No actual casting or performance outcome claimed yet.

## Magic04 actual correctness acceptance

- Read-only review of actual `.tmp/gameplay-magic-04/magic-actions.jsonl`, `magic/result.json` and `manifest-final.json` confirms statuscomplete, owned shutdownexit0, no finalization errors, unchanged source database/executable/controller sources/sound catalog.
- Six terminal validated attempts: four Lumos fizzles (roll97.3612,60.9819,88.0489,92.2508 against quality60), then Lumos applied(roll46.0025,Q60) and Nox applied(roll1.1953,Q60.05). The small later quality increase is native learning; fixture never sets it.
- Fizzles each pay0.5 of quote1.0, preserve dark physical state, and have exactly one matching debit; successful spells each pay1.0 with exact matching ledger identity. Raw reservoirs and natural refill satisfy the bounded independent resource oracle; shared recovery starts and pending clears for all six.
- Actual roomlight0 -> 0.238732414637843 ->0; real owned wand lights and extinguishes. Exactly one productive Lumos/Nox iteration; fizzles are reported separately rather than counted as productive work.
- Native compact payload lengths547,554,556,555,767,749, all<=900. The earlier terminal budget was a source-derived estimate; these are actual native `toliteral` lengths from the run.
- Native cleanup returns actor/wand/tutor/room removed1, worker idle after one observation, private account retained1 with explicit lack of account-registry deletion API. Background/connection errors remain visible in raw transcript; no claim that this imported world has no native errors.
- Module remains SHA90c96717e47f5d6cd2ff307dad2d6753b7d8cdd09cd6f7fa1ef46dbaf301eba7. No commits, engine/game fixes, live calls or worker world launches. Parent can reuse it frozen in the combined real gameplay run.
- This accepts gameplay correctness for this cohort only. Natural RNG/learning makes attempted work vary; no equal-work speedup, Barn parity, peak-memory reduction or live Mongoose acceptance follows from one successful offline run.
