# Barn Mongoose Phase-Zero Research

## Scope and checkout state

This report answers only the Barn side of Milestones 0 and 1 in
`plans/barn-toast-mongoose-convergence-workstreams.md`.

Research snapshot:

- branch: `master`, at `7aac0750456e415c8e4fce4e3da69d9426ed2dc4`;
- branch relation: `master...origin/master [ahead 7]`;
- the tracked files inspected for this report were clean;
- `reports/toast-oracle-wsl.md` is present but untracked;
- unrelated untracked files were ignored, and no server was launched.

The plan makes Milestone 0 the first execution action and forbids a Barn source
patch before the managed Toast-first login gate
(`plans/barn-toast-mongoose-convergence-workstreams.md:354-356`).

## Verified current facts

### The durable WSL control surface is incomplete

- `AGENTS.md` correctly requires the managed conformance command and forbids
  manual Barn or Toast launches unless the user explicitly requests one
  (`AGENTS.md:5-10`). It also correctly identifies the ordinary oracle as the
  WSL binary `/root/src/toaststunt/build-release/moo` and rejects Windows Toast
  substitution (`AGENTS.md:14-17`).
- That active instruction points to `reports/toast-oracle-wsl.md`
  (`AGENTS.md:15`), but Git reports that file as untracked. Therefore a fresh
  checkout cannot follow the referenced procedure from tracked files alone.
- The tracked wrapper already supplies the correct managed-process shape: it
  translates the harness-provided Windows database path with `wslpath`, writes
  restart output to `{db}.new`, stays in the foreground, and permits oracle
  selection through `TOAST_MOO` (`scripts/run_toast_wsl.sh:2-19`). No new
  launcher/helper is needed.
- The tracked plan pins stock Toast at
  `aecc51e9449c6e7c95272f0f044b5ba38948459e` and Mongoose Toast at
  `72e3c7f96ce7a41fdeba793aef8818dc4408072e`
  (`plans/barn-toast-mongoose-convergence-workstreams.md:37-45`). It also
  requires each run record to carry the exact Barn SHA, both applicable Toast
  SHA(s), fixture checksum, connection contract, and the credential-free login
  script mechanism (`plans/barn-toast-mongoose-convergence-workstreams.md:160-168`).
- The harness mechanism is a user-selected environment variable named by
  `--moo-login-script-env`; its value is newline-separated raw login commands.
  The documentation should standardize one non-secret name, for example
  `MONGOOSE_LOGIN_SCRIPT`, and show the flag without committing its value.
  Commands may use `{user}` when tests switch users. This mechanism is owned by
  `moo-conformance-tests`, not Barn.

### Active instructions conflict

`CLAUDE.md` is the stale active surface that Milestone 0 must repair:

- it tells agents to launch a Windows Toast binary directly
  (`CLAUDE.md:88-118`), contrary to `AGENTS.md:14-17` and the plan's WSL-only
  authority (`plans/barn-toast-mongoose-convergence-workstreams.md:28-49`);
- its reference table still identifies Toast by a Windows `test/moo.exe`
  (`CLAUDE.md:173-181`);
- it presents attached-port/manual commands as the ordinary test workflow
  (`CLAUDE.md:194-224`) and has a manual-server section
  (`CLAUDE.md:246-264`), while `AGENTS.md:5-10` requires managed lifecycle;
- it calls `moo-conformance-tests` read-only and says every failure must only
  produce a Barn fix (`CLAUDE.md:28-42`). That conflicts with the convergence
  plan's required durable test commits and its rule that a candidate Barn
  already passes should be kept as coverage without inventing a Barn change
  (`plans/barn-toast-mongoose-convergence-workstreams.md:9-22,140-153`);
- the untracked oracle report says its stale `CLAUDE.md` paths were already
  fixed (`reports/toast-oracle-wsl.md:68-71`), but the current tracked
  `CLAUDE.md` proves that claim false.

### Mongoose profiles currently run strict semantics

- Barn's option model and parser are already sufficient. `PROMOTE_NUMBERS`
  defaults false (`config/options.go:15-20`), is parsed from a config assignment
  (`config/parser.go:50-61`), and is published as
  `option.PROMOTE_NUMBERS` in every generated manifest feature map
  (`config/options.go:28-35`, `profile/manifest.go:70-88`).
- Both committed config files contain only `OUTBOUND_NETWORK`; neither sets
  `PROMOTE_NUMBERS` (`profiles/barn/outbound-on.conf:1`,
  `profiles/barn/outbound-off.conf:1`).
- All four Mongoose entries reuse those strict config files and declare only
  outbound networking plus builtin presence. This affects Linux on/off
  (`profiles/barn/profiles.json:31-57`) and Windows on/off
  (`profiles/barn/profiles.json:87-113`). Their command templates contain
  neither a promotion-enabled config nor `--promote-numbers`.
- Consequently a managed Mongoose command produces a manifest with
  `option.PROMOTE_NUMBERS: false`, even though its profile ID and fixture say
  `mongoose`. This is not a missing Barn runtime capability; it is false profile
  wiring and metadata.
- `ValidateManifestAgainstProfile` already rejects any expected feature that is
  absent or unequal (`profile/registry.go:107-131`), but the Mongoose registry
  entries do not expect the promotion feature. Existing tests exercise only an
  outbound mismatch (`profile/registry_test.go:53-98`).
- `BuildManifest` already records the actual runtime OS, database checksum,
  config checksum, and full runtime feature map
  (`profile/manifest.go:64-89`). Existing manifest tests check checksums and
  outbound metadata but do not assert promotion metadata
  (`profile/manifest_test.go:11-48`).
- The server's `--profile-id` is currently a label used to build a manifest;
  normal startup does not load or validate that manifest against the registry.
  Registry loading is used for `--list-profiles` only
  (`cmd/barn/main.go:117-120,162-168,307-321`). The comparison gate must
  therefore remain fail-closed in the conformance harness.

## Smallest coherent Barn change set

### Milestone 0: documentation-only commit

Change exactly these two Barn paths:

1. `CLAUDE.md`
   - remove/supersede the Windows Toast server commands, attached-port/manual
     default workflow, stale binary table entry, and read-only-conformance rule;
   - point to the managed WSL workflow and the plan without duplicating a second
     contradictory procedure.
2. `reports/toast-oracle-wsl.md`
   - add it as a tracked file and rewrite it as the canonical managed procedure;
   - document both pinned WSL binaries/SHAs, `scripts/run_toast_wsl.sh`, the exact
     managed command shape, disposable `{db}`/`{db}.new` handling, and selection
     of the Mongoose binary via `TOAST_MOO`;
   - standardize `MONGOOSE_LOGIN_SCRIPT` as the uncommitted newline-separated
     value passed through `--moo-login-script-env MONGOOSE_LOGIN_SCRIPT`;
   - require a run record containing Barn HEAD/tracked-dirty state, selected
     Toast SHA, selected fixture path/size/SHA-256, profile manifest, connection
     contract, command, and result;
   - delete the false claim that `CLAUDE.md` was already repaired.

No change is required to `AGENTS.md`, the convergence plan, or
`scripts/run_toast_wsl.sh`: their current policy, pinned identities, and wrapper
capability are already the surfaces the corrected docs should expose.

### Milestone 1: profile/manifest commit

Change exactly these Barn paths:

1. Add `profiles/barn/mongoose-outbound-on.conf` containing explicit
   `OUTBOUND_NETWORK = 1` and `PROMOTE_NUMBERS = 1`.
2. Add `profiles/barn/mongoose-outbound-off.conf` containing explicit
   `OUTBOUND_NETWORK = 0` and `PROMOTE_NUMBERS = 1`.
3. Update `profiles/barn/profiles.json` so all four Linux/Windows Mongoose
   entries use those files and declare `option.PROMOTE_NUMBERS: true`; keep
   strict/Test.db profiles on the existing configs.
4. Add `profiles/toast/stock-wsl-mongoose.json` and
   `profiles/toast/mongoose-wsl-mongoose.json` as immutable oracle manifests for
   the same selected Mongoose fixture. Each must include its exact implementation
   SHA, `runtime_os: linux`, `database_fixture: mongoose`, the freshly verified
   database checksum, explicit `option.OUTBOUND_NETWORK`, and
   `option.PROMOTE_NUMBERS` (`false` for stock, `true` for Mongoose Toast).
5. Extend `profile/registry_test.go` with a table over all four Mongoose entries:
   load each referenced config, require promotion true and the declared outbound
   value, require expected promotion metadata true, build a manifest, and run
   `ValidateManifestAgainstProfile`. Add missing-promotion and false-promotion
   rejection cases.
6. Extend `profile/manifest_test.go` to assert that generated manifests contain
   `option.PROMOTE_NUMBERS` with both true and false values and retain database
   and config checksums.

No production change is required in `config`, `profile/manifest.go`,
`profile/registry.go`, or `cmd/barn/main.go`: the parser, manifest feature map,
checksum emission, and generic mismatch rejection already implement the needed
Barn behavior. The defect is the committed profile data and its missing tests.

## Tests and acceptance gates

Milestone 0:

- fixed-point search of active tracked instructions finds no Windows Toast
  command or ordinary manual-launch direction;
- a fresh-checkout reader can construct both stock and Mongoose managed WSL
  commands without an untracked note;
- the example records identities and the fixture checksum but never the login
  script value;
- `git diff --check`, followed by a separate documentation-only commit.

Milestone 1:

- `go test ./config ./profile ./cmd/barn`;
- `go test ./...` and `git diff --check`;
- generate manifests through every Mongoose command template and verify the
  declared profile, actual runtime OS, database checksum, config checksum,
  outbound value, and promotion value;
- in `moo-conformance-tests`, add `option.PROMOTE_NUMBERS` to the required
  feature keys and prove missing, false/true mismatch, and true/false mismatch
  all fail before behavioral tests run;
- WSL stock Toast compares only with a Linux Barn strict manifest, and WSL
  Mongoose Toast compares only with a Linux Barn Mongoose manifest;
- the Windows Barn deployment lane uses the same test and freshly hashed
  fixture, but must not claim an OS-matched Toast comparison.

## Blockers and risks

- Fail-closed comparison is cross-repository. The current conformance gate
  requires only `option.OUTBOUND_NETWORK`; Barn profile edits alone cannot make
  missing/mismatched promotion metadata stop a run.
- The conformance gate also requires equal `runtime_os`. A WSL oracle manifest
  cannot be paired directly with a Windows Barn manifest. The Windows lane must
  be recorded as deployment proof or the harness must define a separate policy;
  Barn must not falsify `runtime_os`.
- The plan's recorded Mongoose checksum is evidence from plan-writing time
  (`plans/barn-toast-mongoose-convergence-workstreams.md:105-114`). Milestone 1
  must freshly hash the selected disposable-input source before freezing oracle
  manifests; a nearby Mongoose database is not a substitute.
- Do not commit credentials or reuse the credential-bearing historical notes as
  active instructions. Only the environment-variable name and command shape
  belong in the tracked procedure.
