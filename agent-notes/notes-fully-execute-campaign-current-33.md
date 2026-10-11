# Current checkpoint 33

- Scope: Toast checkout-topology repair for draft moo-conformance-tests PR #46.
- Current pushed head remains `055b993e206bedf887780e64d84dfa7fcadb2f75`; new repair is local and uncommitted in three files.
- Workflow now checks every conformance candidate into fixed `candidate-data`; pinned Toast source is separate credentialless sibling `toast-oracle`.
- Quality/full-suite/ledger use explicit candidate working directories or `--project` arguments. Virtual environments, Toast build, harness JUnit, profile reports, and execution ledger are under runner temp; admission/package anchor is exactly `${GITHUB_WORKSPACE}/candidate-data`.
- Toast build uses sibling source and runner-temp build directory. Fixture verification/checksum run from candidate-data. Admission/package DB fixture roots are candidate-data absolute paths. Artifact upload and union paths are outside the candidate anchor.
- Regressions cover identical-byte primary Test.db symlink to sibling Toast oracle, suite and DB links to sibling generated paths, fixed credentialless sibling checkouts, explicit candidate project paths, and external generated state.
- Gates green: focused workflow/inventory 40 passed; full harness 335 passed; collect-only zero exit with unchanged 11,588 surface; duplicate lint exact; Ruff; mypy 22 source files; diff check.
- Next: stage only the three intended files, commit and push a new commit, update PR body, watch all managed Toast jobs and union, inspect exact counts/PR state, keep draft/unmerged.
