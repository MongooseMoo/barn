# Milestone 2: managed real-Mongoose login/control/look gate

Primary source repository: `C:/Users/Q/code/moo-conformance-tests`.
Run-record repository: `C:/Users/Q/code/barn`.

Read both repositories' current tracked state, the convergence plan, `reports/toast-oracle-wsl.md`, and `docs/reports/mongoose-phase0-conformance.md`. Use only the existing managed server, login-script environment, YAML `run`, and YAML `command` surfaces. Do not add a helper, adapter, alternate launcher, or raw-socket login implementation.

Add one focused integration suite at `src/moo_conformance/_tests/integration/mongoose_login_look.yaml`:

- `requires.config: [managed_server]`;
- one test with `permission: wizard` so the static Mongoose login is not followed by a harness user switch;
- a `run` step that exactly asserts the PROXY source IP `203.0.113.5`, one representative mixed arithmetic result, and one mixed comparison result. Do not query `server_version("options.PROMOTE_NUMBERS")`: the pinned Mongoose build compiles promotion semantics but does not publish that macro in its generated version-options tree. The verified oracle profile manifest carries the build-option assertion;
- a raw `command: look` step asserting at least two stable anchors learned from the managed WSL Mongoose Toast run, not guessed volatile state;
- an oracle-measured timeout/deadline once current Toast timing is known.

Add only direct Python tests needed to prove the YAML discovers with its exact suite/test identity and that a multi-line static login script sends the PROXY prelude, account line, and password line in order. Never put credentials in YAML, tests, commands, logs, or run records.

Oracle-first execution:

1. Freshly verify Debian WSL Mongoose Toast HEAD `72e3c7f96ce7a41fdeba793aef8818dc4408072e`, executable/version, and `mongoose.db` SHA-256 `a9d167861eab56d62e9bd12ae1d47c5e6a858530020a5dcf174a0b104fb23db9`.
2. Populate `MONGOOSE_LOGIN_SCRIPT` only inside the invoking process from the local uncommitted credential record; never echo its value.
3. Run the focused test through `uv run --project ..\moo-conformance-tests moo-conformance` with the Debian `scripts/run_toast_wsl.sh` managed command, `mongoose.db`, `--moo-login-script-env MONGOOSE_LOGIN_SCRIPT`, `--moo-skip-standard-properties`, and both oracle/target profile flags pointing to `profiles/toast/mongoose-wsl-mongoose.json`.
4. Use the first managed Toast result to select stable room anchors and measure startup/login/test duration. The finalized unchanged test must pass Toast before any Barn production diagnosis.
5. Record the exact credential-free command, identities, checksum, timing, assertions, and result under `C:/Users/Q/code/barn/reports/runs/`.

Then run the unchanged test against current Barn under the truthful Linux Mongoose outbound-on profile and record the exact result. If it passes, keep the test as coverage and do not invent a Barn source patch. If it fails, that exact assertion becomes Milestone 3's only active behavior row.

Run focused schema/discovery/transport unit tests with `uv run pytest`, `uv run ruff check` on changed Python, and `git diff --check`. Commit the conformance slice separately before any Barn production fix. Do not manually launch either server and do not use a tracked fixture in place.
