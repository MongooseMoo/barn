## Look-me checkpoint 2026-09-18
- Canonical WSL Toast renders q's look me with yourself/he/his; rebased Barn renders Verb not found for those pronouns on identical frozen Mongoose DB.
- Barn properties player.ps/pp/pr are correct; $string_utils:pronoun_sub("%s %p %r", player) returns three Verb not found strings.
- #59:verb_or_property uses a catch expression with a nested property fallback; next reduce and verify that expression against canonical Toast before fixing.
- Evidence: .tmp/mongoose-look-me-20260918. Disposable Barn runs on 11505, debug 11506 (exec session 86697); owned Toast on 7777 stopped to remove WSL forwarding overlap.
- Separate known startup errors remain; stock Toast numeric mode also breaks spatial room rendering. No implementation change yet.

## Parser reduction checkpoint 2026-09-18
- #1:ps*c exists and is executable, but compiling its source fails; its catch default contains a conditional expression.
- Generic Test.db reduction is tests/mongoose-conformance/builtins/catch_ternary_fallback.yaml; canonical WSL Toast passed all three cases plus admission, Barn failed all three with syntax error.
- Parser currently reads catch defaults at PREC_CATCH, excluding lower-precedence conditional expressions; property fallback itself works.
- Next: fix parsing with a regression, rebuild, rerun unchanged managed tests and live look me, then commit owned changes.

## Pronoun fix completion 2026-09-18
- Commit 62d767e lowers catch-default parse threshold to PREC_TERNARY, adds AST and three generic behavioral regressions, documents reproduction.
- Canonical Toast: 4 passed in 7.68s; original Barn: 3 failed/1 passed; rebuilt Barn: 4 passed in 1.13s; go test ./... exit 0 and added parser test passed.
- Fresh Barn look me now prints yourself/He/his and explicit substitution returns he his himself. Separate SQLite Confunc failure remains.
- Root barn.exe rebuilt after commit (exit 0); disposable Barn PID 113736 stopped. Notes remain uncommitted; tracked working tree clean.
- Evidence is under .tmp/mongoose-look-me-20260918, particularly look-me-fixed.txt and conformance/20260918_234756.
