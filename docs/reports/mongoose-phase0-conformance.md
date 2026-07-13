# Mongoose phase-zero conformance research

## Scope and current state

This is a source-only report. No Barn or Toast process was launched. The inspected
`moo-conformance-tests` checkout is `main` at
`9300e724f7e091056a913b2cb0c7e4e8f98b9fdd`; it has unrelated untracked files, which
were not touched.

The functional Mongoose gate does **not** need a new adapter/helper. The current
managed server, environment-backed login script, `run` step, and raw `command` step
already compose into the required login/control/`look` scenario. The profile gate
does need to become truthful first, and the current timeout surface cannot yet
express the plan's oracle-derived startup/login deadline.

## Verified facts

- Managed mode already copies the selected `--server-db` into a temporary directory,
  substitutes `{port}`, `{db}`, `{manifest}`, and `{server_dir}`, starts the child in
  that directory, waits for the port, and removes the directory on final stop
  (`../moo-conformance-tests/src/moo_conformance/server.py:63-138`, `:140-164`). This
  is the required disposable-fixture lifecycle.
- `--server-command`, `--server-db`, both profile-manifest options,
  `--moo-login-script-env`, and `--moo-skip-standard-properties` already exist
  (`../moo-conformance-tests/src/moo_conformance/plugin.py:84-135`). The login script
  is read from the named environment variable as nonempty newline-separated commands,
  so credentials need not enter YAML or Git (`plugin.py:138-154`).
- The session transport starts the managed server, loads that script, and connects
  once as its logical `wizard` user (`plugin.py:157-208`, `:259-291`). Each script
  line is sent in order; `{user}` substitution is supported (`transport.py:348-356`).
  A static Mongoose script cannot survive an attempted logical user switch
  (`transport.py:305-346`), so the focused YAML must use `permission: wizard` (or the
  harness would try the default `programmer` switch before the test).
- The real fixture must use `--moo-skip-standard-properties`: otherwise transport
  startup attempts five `add_property()` mutations on `#0`
  (`transport.py:235-303`).
- YAML already supports sequential `run` and raw `command` steps with output
  assertions (`runner.py:255-450`). A raw command result can use exact lines, a regex,
  or a substring (`schema.py:172-180`, `:549-562`), which is sufficient for stable
  `look` anchors.
- Secondary raw sockets already support `new_connection`, `send`, `send_bytes`,
  `read_connection`, and `close_connection` (`schema.py:304-344`;
  `runner.py:281-352`). They are useful for reduced lifecycle rows, but they cannot
  consume the secret environment login conversation and are therefore not a
  substitute for the session transport in the first real-fixture gate.
- The profile gate currently requires only `option.OUTBOUND_NETWORK`,
  `database_fixture`, and `runtime_os` (`profile_gate.py:14-15`, `:31-56`). Its tests
  cover outbound mismatch, unsupported target, missing outbound, fixture mismatch,
  and JSON loading, but nothing about number promotion or exact fixture identity
  (`tests/test_profile_gate.py:8-58`).
- Barn's current Mongoose-named profiles still point at configs containing only
  `OUTBOUND_NETWORK`; their expected features omit `option.PROMOTE_NUMBERS`, and their
  command templates omit `--promote-numbers`
  (`profiles/barn/profiles.json:32-57`, `:88-113`;
  `profiles/barn/outbound-on.conf:1`; `profiles/barn/outbound-off.conf:1`). Barn can
  already parse `PROMOTE_NUMBERS` and emits it in runtime feature maps
  (`config/parser.go:58-59`; `config/options.go:6`; `profile/manifest.go:70-87`).

## Exact profile-gate gaps

1. `option.PROMOTE_NUMBERS` is not required or compared. A strict Barn run can match
   a Mongoose Toast manifest as long as outbound networking, fixture label, and OS
   match.
2. `database_checksum` is neither required nor compared. Two different files both
   labelled `mongoose` pass the gate. This violates the plan's exact-fixture rule.
3. Feature values are untyped `Any`; Python considers JSON `1 == true`. Promotion and
   outbound values therefore need explicit boolean validation, not equality alone.
4. Only `target.support_status == "unsupported"` is rejected. A missing target
   status, an unsupported oracle, or a missing oracle status is accepted. The pair
   is not fail-closed.
5. The gate is optional when `--oracle-profile-manifest` is absent
   (`plugin.py:189-208`). That is appropriate for ordinary conformance, but the named
   Mongoose command must always supply it.
6. `runtime_os` equality correctly prevents a Linux oracle manifest from masquerading
   as Windows proof. Consequently the Windows deployment lane cannot be represented
   as a Linux-oracle comparison; it must run the unchanged focused test and record its
   Windows target manifest separately.

Smallest change: add `option.PROMOTE_NUMBERS` to `REQUIRED_FEATURE_KEYS`, add
`database_checksum` to required top-level equality, validate both option fields as
JSON booleans, and require both manifests to declare an allowed non-unsupported
support status. Do not introduce a new profile object or comparison adapter.

Required unit rows are: matching promotion accepted; promotion missing from either
side rejected; promotion mismatch rejected; integer/string promotion rejected;
checksum missing from either side rejected; checksum mismatch rejected; missing or
unsupported oracle status rejected; missing target status rejected. Existing outbound,
fixture, and path-loading rows remain.

## Existing coverage versus the missing real gate

| Behavior | Current durable coverage | Missing first-gate proof |
|---|---|---|
| Login dispatch | Listener-handler dispatch and original `args`/`argstr` are covered (`connection_lifecycle_toast_oracle.yaml:31-90`, `:1172-1244`). | The real Mongoose account conversation from the environment reaches an authenticated player. |
| Trusted PROXY | A reduced row proves trusted `PROXY` clears login input (`connection_lifecycle_toast_oracle.yaml:251-339`). | The real login connection adopts the announced source address, proven by `connection_name(player, 1)`. |
| Post-login lifecycle | Reduced rows cover command dispatch, disconnect/reconnect hooks, redirect, timeout, and creation hooks (`connection_lifecycle_toast_oracle.yaml:92-250`, `:341-637`, `:967-1112`, `:1405-1518`). | Post-login control on the real fixture and a stable room render. |
| Held input | Reduced rows cover held input release, OOB interaction, and flush behavior (`connection_lifecycle_toast_oracle.yaml:638-748`, `:1246-1404`). | Burst input while the real Mongoose world is busy; this is a follow-on row, not required to overload the first login/`look` test. |
| Connection name | Shape and generic value coverage exists (`builtins/connection_name_semantics.yaml:15-61`). | Equality to the address announced in this run's PROXY prelude. |
| Promotion | Current general arithmetic tests assert strict `E_TYPE` behavior (`basic/arithmetic.yaml:41-50`; `basic/types.yaml:595-670`). | A Mongoose-only positive assertion that mixed arithmetic/comparison promotes. Full Mongoose-profile suite work must later gate strict rows and add promoted counterparts. |

## Smallest first real-Mongoose YAML change

Add one integration suite, for example
`src/moo_conformance/_tests/integration/mongoose_login_look.yaml`, with one focused
test. Declare `requires.config: [managed_server]`, set `permission: wizard`, and do
not put the PROXY line, account name, password, or selection in YAML; those remain in
the environment login script.

The test should contain only:

1. one `run` returning and exactly asserting:
   `connection_name(player, 1)` equals the documented source IP in the PROXY prelude;
   `server_version("options.PROMOTE_NUMBERS") == 1`; representative mixed arithmetic
   equals the Toast-observed float; and representative mixed comparison equals the
   Toast-observed boolean;
2. one `command: look` whose `expect.output.match` names two or more stable anchors
   selected from the Toast run (room title plus a stable render label, not dynamic
   occupants/time/state).

This proves login success indirectly but decisively: failed authentication cannot
execute the eval or framed `look`, while the connection-name assertion proves the
PROXY metadata survived into the authenticated connection. No schema, runner, raw
socket, or login-script helper change is needed for this functional row.

Add schema/discovery tests only for the new YAML's validity and collection identity;
add a focused transport test confirming a multi-line static login script sends the
PROXY prelude and subsequent lines in order. Keep the existing static-user-switch
test (`tests/test_managed_server.py:179-196`).

## Managed command shapes

Set the secret once in the invoking environment as newline-separated raw commands,
including the trusted PROXY prelude and account conversation; never print or commit
its value. Then run the unchanged focused node first against pinned WSL Mongoose
Toast:

```powershell
uv run --project ..\moo-conformance-tests moo-conformance -k mongoose_login_look --server-command "wsl -e env TOAST_MOO=/root/src/toaststunt-mongoose/build-release/moo bash /mnt/c/Users/Q/code/barn/scripts/run_toast_wsl.sh {db} {port}" --server-db C:/Users/Q/code/barn/<selected-mongoose-db> --moo-login-script-env MOO_MONGOOSE_LOGIN --moo-skip-standard-properties --oracle-profile-manifest <wsl-mongoose-toast-manifest.json> --target-profile-manifest <wsl-mongoose-toast-manifest.json> -v
```

The explicit target manifest is required because the current Toast wrapper accepts
only `{db}` and `{port}` and does not emit `{manifest}`
(`scripts/run_toast_wsl.sh:8-20`). Record the pinned engine SHA and selected database
checksum beside that manifest before treating the self-comparison as authoritative.

Run the unchanged node against WSL Barn with its truthful Mongoose config and an
auto-written target manifest:

```powershell
uv run --project ..\moo-conformance-tests moo-conformance -k mongoose_login_look --server-command "wsl -e <wsl-barn> --db {db} --listen tcp://127.0.0.1:{port} --checkpoint-interval 0 --config <mongoose.conf> --profile-id barn-linux-mongoose-outbound-off --profile-manifest {manifest}" --server-db C:/Users/Q/code/barn/<same-selected-mongoose-db> --moo-login-script-env MOO_MONGOOSE_LOGIN --moo-skip-standard-properties --oracle-profile-manifest <wsl-mongoose-toast-manifest.json> -v
```

Finally run the same node and checksum against Windows Barn as the separate deployment
lane. Do not pair its Windows manifest with the Linux oracle manifest; `runtime_os`
must remain truthful.

## Blockers and risks

- Stable `look` anchors and deadline values are not known from source inspection.
  They must be captured from the managed WSL Mongoose Toast pass before the YAML is
  finalized; guessing them would substitute a hunch for the oracle.
- `MooTestCase.timeout_ms` is parsed but never consumed anywhere outside the schema
  (`schema.py:375`, `:915`; repository search has no runner use). Startup waits a
  fixed 30 seconds (`server.py:137-138`), socket connect uses 3 seconds, and every
  login-script line drains with a fixed 2-second timeout (`transport.py:213-280`).
  These are safety limits, not measured oracle-derived assertions. After measuring
  Toast, wire the existing timeout field into request execution; login/startup timing
  needs the smallest session-level timing assertion that covers the already-existing
  lifecycle, not a new transport abstraction.
- `_consume_login_output()` treats timeout as completion and discards the transcript
  (`transport.py:240-280`). The proposed post-login eval, exact promoted results,
  connection-name equality, and `look` anchors are therefore all required; a mere
  process-start or no-exception assertion is not a login proof.
- `requires.config: [managed_server]` skips when omitted (`test_conformance.py:135-147`).
  The canonical gate command must therefore be repository-recorded and reviewed for
  an actual pass, not accepted as green solely from pytest exit status when the row
  could have skipped.
