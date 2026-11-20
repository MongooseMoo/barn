# Current checkpoint 32

- Scope: repair draft moo-conformance-tests PR #46 after Toast checkout-topology NO-MERGE review.
- Current pushed head remains `055b993e206bedf887780e64d84dfa7fcadb2f75`; PR remains draft/unmerged.
- Root cause: Toast workflow checks the candidate out at `GITHUB_WORKSPACE` and Toast under `GITHUB_WORKSPACE/toaststunt`, so `--candidate-root=GITHUB_WORKSPACE` incorrectly treats oracle content as candidate-owned.
- Red tests added: every Toast job requires fixed `candidate-data` checkout; Toast oracle must be credentialless sibling `toast-oracle`; admission/package require exact candidate root and external build path; venv/build/reports/ledger remain outside anchor; identical primary Test.db link to sibling oracle and suite/DB links to sibling generated paths are rejected.
- Red evidence: focused workflow/inventory run collected 40; 36 passed and four topology assertions failed against current YAML. All new filesystem attack tests already pass the shared confinement implementation.
- Next: rewrite Toast quality/full-suite/ledger paths to candidate-data and sibling toast-oracle, use explicit project/working directories, move venv/build/report outputs outside anchor, rerun focused/full gates, push new commit, update PR body, and watch complete managed Toast matrix. Paired workflow remains unchanged.
