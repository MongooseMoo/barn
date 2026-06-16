# Investigation: conformance four failures

## Facts (verified)
- Full Barn conformance run `reports/runs/20260616_014032` had 4 failures: restart, open_network disabled, url_encode reserved, url_decode percent.
- The restart failure in that run was harness mode: pytest reported `restart_server` requires `--server-command`.
- Running the restart test with pytest-managed Barn got a real Barn result: expected `after`, got `before`.
- Barn now passes the restart target under pytest-managed Barn.
- Toast now passes the restart target through `scripts/run_toast_wsl.sh` after the wrapper writes checkpoint output to `{db}.new`.
- Toast returned success for `url_encode("a b/c?d=e&f")`: `a%20b%2Fc%3Fd%3De%26f`.
- Toast returned success for `url_decode("a%20b%2Fc%3Fd%3De%26f")`: `a b/c?d=e&f`.
- Barn returns the same URL success values; the YAML expected `E_PERM`.
- Barn reports `E_INVARG` for `open_network_connection("127.0.0.1", 1)` in the failing full run.
- The conformance skip gate for `option.OUTBOUND_NETWORK` calls `server_version("options.OUTBOUND_NETWORK")`.
- Barn now exposes `server_version("options.OUTBOUND_NETWORK")`, so disabled-network expectations are skipped for Barn's outbound-capable profile.
- The old Toast wrapper wrote checkpoint output to `/dev/null`, preventing restart tests from adopting checkpointed state.

## Classification
1. URL failures are invalid conformance expectations, not Barn bugs.
2. The open_network failure is an invalid profile-gated test: actual Toast has outbound enabled or blocks trying outbound, while the harness cannot detect the option.
3. The restart failure has two layers: Barn's documented runner bypasses managed restart, and Barn also fails to resume/persist delayed fork work once managed restart is used.

## Tests Run

| Test | Hypothesis | Result | Rules Out | Supports |
|------|------------|--------|-----------|----------|
| Toast url_encode target | URL YAML expects wrong behavior | Toast succeeded with encoded value | Barn should not be changed to E_PERM | Theory 1 |
| Toast url_decode target | URL YAML expects wrong behavior | Toast succeeded with decoded value | Barn should not be changed to E_PERM | Theory 1 |
| Barn restart via `--server-command` | Full run failure only harness | Barn restarted but returned `before` | Pure harness-only explanation | Theory 3 |
| Toast restart via old wrapper | Need Toast baseline | Toast lost property and returned E_PROPNF | Reliable Toast restart baseline | Need wrapper checkpoint fix |
| Toast open_network target | YAML E_PERM is Toast behavior | Toast timed out instead of E_PERM | Direct E_PERM expectation | Theory 2 |
| Toast restart via fixed wrapper | Need Toast restart baseline | Toast passed | Toast expects E_PROPNF or before | Theory 3 |
| Barn four-failure managed slice | Fixes close original rows | 3 passed, 1 skipped | Remaining four-row regression | Final classification |
| Barn property slice in managed mode | Property cascade from restart row | Property rows fail without restart row | Restart restore caused property cascade | Managed full-run caveat |

## Final Result
The four rows were not one Barn bug. URL rows were conformance expectation bugs and now match Toast. Open network was a profile/option-gating bug and is now skipped for Barn's outbound-capable profile. Restart was the confirmed Barn runtime bug; Barn now persists enough queued-task state and source information for the delayed fork to survive restart and execute the fork body after reload.

## Caveat
A broad `--server-command` full-suite run is not equivalent to the existing external-server script run: old stateful property/create rows fail in managed mode even without the restart test. Final verification for this work therefore used targeted managed rows for the original failures.
