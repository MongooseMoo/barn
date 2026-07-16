# Barn/Toast/Mongoose Convergence Plan
## Goal
- Make Barn run the real Mongoose workload with exact Toast-compatible behavior.
- Convert every accepted semantic difference into a durable cross-engine test.
- Finish functional convergence before measuring or optimizing performance.
## Authority
- `C:/Users/Q/code/barn` owns implementation, profiles, diagnostics, and deployment verification.
- `C:/Users/Q/code/moo-conformance-tests` owns generic behavioral truth.
- `C:/Users/Q/code/mongoose` supplies the real workload but never conformance fixtures or credentials.
- Stock behavior authority is `/root/src/toaststunt/build-release/moo` at commit `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
- Mongoose behavior authority is `/root/src/toaststunt-mongoose/build-release/moo` at commit `72e3c7f96ce7a41fdeba793aef8818dc4408072e`.
- Use WSL Toast only; never substitute a Windows Toast binary.
- Direct Toast observation precedes conformance reduction whenever a concrete theory exists.
## Fixed inputs
- Current convergence fixture: `.tmp/mongoose-refresh-20260713/mongoose.db.new`, SHA-256 `b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`.
- Fixed login control: `mongoose_fresh2.db`, SHA-256 `33201970097d3d2d2bfc0d5f875f087d587601bf8255ef31ef19b416d65ac925`.
- Re-hash the selected fixture immediately before each control run.
- Run each engine only against a disposable copy, never the source fixture.
- Mongoose profiles must explicitly enable `PROMOTE_NUMBERS = 1` and declare outbound-network policy.
- Use built `moo_client.exe` for live login, connection, and room-render behavior.
- Use `-banner-wait 3000 -inter-cmd 2500 -timeout 15` with the existing trusted-PROXY/account/password interaction.
- Keep credentials in the existing uncommitted mechanism and never print or commit them.
- Use `scripts/run_toast_wsl.sh` for managed stock-Toast `Test.db` runs.
## Non-negotiable boundaries
- `moo-conformance-tests` must contain no Mongoose names, fixtures, profiles, hooks, accounts, passwords, or environment details.
- Keep live Mongoose discovery evidence inside Barn only.
- Do not reconstruct the login flow or replace `moo_client.exe` with an ad hoc client.
- Do not change conformance transport unless an independent Toast/Barn row proves a transport defect.
- Work on exactly one target behavior or family at a time.
- Diagnostics, setup, and understanding are not convergence progress.
- If two consecutive slices produce no kept divergence reduction, stop instead of widening scope.
- Git is the ledger: every kept slice is committed and every rejected slice is fully reverted.
- Do not append execution reports, observations, conclusions, or status narratives to this plan.
## Direct-oracle loop
1. Inspect the existing run-local structured JSON ledger before adding diagnostics.
2. Direct every new run's structured log to a unique stable run directory.
3. Correlate timestamps, `conn_id`, `task_id`, player, verb, traceback, frames, and source.
4. State one precise behavioral theory in implementation-independent terms.
5. State the observation that would prove or disprove that theory.
6. If an action cannot change that decision, do not perform it.
7. If logging lacks the dynamic target, instrument the existing choke point temporarily and nowhere else.
8. Start pinned WSL Mongoose Toast on a disposable copy of the selected fixture.
9. Drive the exact existing interaction with `moo_client.exe`.
10. Test the concrete theory directly on Toast immediately.
11. Reproduce the same smallest event on unchanged Barn.
12. Compare direct output, structured logs, metrics, and resulting state.
13. If Toast and Barn agree, reject the theory and fully revert its experimental slice.
14. Proceed to conformance reduction only after a direct Toast/Barn difference is proven.
## Conformance-reduction loop
15. Describe the proven difference without Mongoose-specific names or data.
16. Preserve the triggering workload, ordering, state, and protocol boundary in bundled `Test.db`.
17. Add exactly one generic conformance row for the active behavior.
18. Run that unchanged row first on the managed stock WSL Toast oracle.
19. If Toast fails, correct or reject the reduction; do not run Barn.
20. If Toast passes, run the unchanged row on pre-fix Barn.
21. If Barn passes, reject it as a divergence reduction and revert the row unless the user requests coverage.
22. If Barn fails, record only the exact differing assertion as the active row.
23. Remove all temporary Barn diagnostics before any kept production fix.
24. Commit the Toast-green/Barn-red conformance row in `moo-conformance-tests`.
25. Do not place live fixture evidence or credentials in that commit.
26. Do not modify Barn production code before Toast-green and Barn-red proof exists.
## Barn-fix loop
27. Add a focused Go regression when the failing Barn ownership boundary can express one.
28. Observe the focused regression fail before implementing the fix.
29. Change the smallest real production owner of the divergent behavior.
30. Add no wrapper, shim, helper layer, sender, adapter, fallback, or dual path.
31. Run the focused Go regression.
32. Run the unchanged focused conformance row on Barn.
33. Run the relevant managed conformance family on Toast and Barn.
34. Run the relevant Go package gates and `git diff --check`.
35. Rebuild Barn and repeat the exact live Mongoose interaction on a fresh disposable copy.
36. If the live gate remains open, select its next proven direct difference without broadening families.
37. Commit the Barn fix and exact required run record, staging only named files.
## Behavior-family order
38. Close startup responsiveness under restored background work.
39. Close trusted-PROXY metadata rewrite and connection identity.
40. Close the account-login `read()` sequence and prompt delivery.
41. Close burst input, `hold-input`, and queued-read behavior.
42. Close reconnect, disconnect, and connection-hook behavior.
43. Close room render, contents, exits, `look`, and movement.
44. Close command dispatch, telnet boundaries, MCP, GMCP, and out-of-band traffic.
45. Close scheduling, suspended tasks, restart, persistence, and promotion-sensitive semantics.
## Performance
46. Begin performance work only after all functional deployment gates pass.
47. Run the same fixture and client script on WSL Mongoose Toast and Windows Barn.
48. Record load-to-listen, banner, login, command, movement, CPU, memory, task, connection, and checkpoint metrics.
49. Name one metric and one falsifiable hypothesis per performance slice.
50. Capture the before measurement and profile before editing production code.
51. Change one production surface and rerun the same benchmark plus conformance gate.
52. Commit a measured improvement or fully revert the slice.
## Completion gates
53. Pinned WSL Mongoose Toast passes the exact live deployment interaction.
54. Windows Barn passes the same fixture, client interaction, and observable anchors.
55. Every accepted semantic difference has a Toast-passing/Barn-passing generic conformance row.
56. Every conformance row uses bundled `Test.db` and contains no Mongoose knowledge.
57. Truthful strict and Mongoose profiles pass their complete managed suites.
58. Every behavior family above has no open accepted row.
59. Every kept source slice and required evidence artifact is committed in its owning repository.
60. Every run record identifies fixture hash, engine identity, profile, command, and result without credentials.
61. Audit every line above against current evidence; do not report completion while any gate is unproven.
