# Managed Toast Oracle on WSL

This is Barn's canonical procedure for Toast-oracle conformance work. Run Toast
only through the local `moo-conformance-tests` managed server lifecycle. Do not
substitute a Windows Toast binary, attach the harness to a separately launched
server, or manually manage the Toast process.

## Pinned oracle identities

| Role | WSL binary | Required source SHA |
| --- | --- | --- |
| Stock Toast | `/root/src/toaststunt/build-release/moo` | `aecc51e9449c6e7c95272f0f044b5ba38948459e` |
| Mongoose Toast with `PROMOTE_NUMBERS` | `/root/src/toaststunt-mongoose/build-release/moo` | `72e3c7f96ce7a41fdeba793aef8818dc4408072e` |

These are pins, not permanent proof of current WSL state. Before every
authoritative run, freshly verify both worktree HEADs and both executable paths:

```powershell
wsl -d Debian -u root -e git -C /root/src/toaststunt rev-parse HEAD
wsl -d Debian -u root -e test -x /root/src/toaststunt/build-release/moo
wsl -d Debian -u root -e /root/src/toaststunt/build-release/moo --version

wsl -d Debian -u root -e git -C /root/src/toaststunt-mongoose rev-parse HEAD
wsl -d Debian -u root -e test -x /root/src/toaststunt-mongoose/build-release/moo
wsl -d Debian -u root -e /root/src/toaststunt-mongoose/build-release/moo --version
```

The observed HEAD must equal the required SHA for the selected binary. Record
the command outputs. If an identity or executable cannot be verified, stop: the
run is not authoritative.

## Fresh fixture identity

Select one input database explicitly. Immediately before each authoritative
run, record its absolute path, byte size, last-write time, and fresh SHA-256:

```powershell
Get-Item -LiteralPath C:/Users/Q/code/barn/mongoose.db | Select-Object FullName,Length,LastWriteTimeUtc
Get-FileHash -Algorithm SHA256 -LiteralPath C:/Users/Q/code/barn/mongoose.db
```

At plan-writing time, `mongoose.db` was 100,959,239 bytes with SHA-256
`A9D167861EAB56D62E9BD12AE1D47C5E6A858530020A5DCF174A0B104FB23DB9`.
That historical value is not current verification. Barn and Toast comparisons
must use equivalent managed copies of the same freshly hashed input; a nearby
or older database is not a substitute.

## Managed disposable lifecycle

The harness copies `--server-db` into a temporary server directory and
substitutes that disposable Windows path for `{db}` and an allocated local port
for `{port}`. Barn's tracked `scripts/run_toast_wsl.sh` converts `{db}` with
`wslpath`, keeps Toast in the foreground under the harness, and executes:

```text
$TOAST_MOO DB_WSL DB_WSL.new PORT
```

Both `{db}` and checkpoint output `{db}.new` therefore remain disposable
harness files; the selected source fixture is not run in place. The harness can
adopt `{db}.new` across `restart_server` steps. Do not manually launch Toast or
point a manual process at a tracked fixture.

`TOAST_MOO` selects the WSL binary. The wrapper defaults to stock Toast at
`$HOME/src/toaststunt/build-release/moo`, but authoritative command records
should set it explicitly. The wrapper is mounted in WSL at
`/mnt/c/Users/Q/code/barn/scripts/run_toast_wsl.sh`.

## Managed command shapes

Run these from `C:/Users/Q/code/barn`. Replace the selected database only after
freshly recording its identity. Add reviewed test-selection and profile flags
for the particular run, and preserve the resulting exact command in the run
record.

Stock Toast:

```powershell
uv run --project ..\moo-conformance-tests moo-conformance `
  --server-command "wsl -d Debian -u root -e env TOAST_MOO=/root/src/toaststunt/build-release/moo bash /mnt/c/Users/Q/code/barn/scripts/run_toast_wsl.sh {db} {port}" `
  --server-db C:/Users/Q/code/barn/mongoose.db
```

Mongoose Toast with number promotion and the real Mongoose login contract:

```powershell
uv run --project ..\moo-conformance-tests moo-conformance `
  --server-command "wsl -d Debian -u root -e env TOAST_MOO=/root/src/toaststunt-mongoose/build-release/moo bash /mnt/c/Users/Q/code/barn/scripts/run_toast_wsl.sh {db} {port}" `
  --server-db C:/Users/Q/code/barn/mongoose.db `
  --moo-login-script-env MONGOOSE_LOGIN_SCRIPT `
  --moo-skip-standard-properties
```

`MONGOOSE_LOGIN_SCRIPT` is a Windows environment variable set by the caller to
the uncommitted, newline-separated raw login conversation, including any
trusted-PROXY prelude and account-selection commands required by the run. The
Windows Python harness consumes it; Toast and WSL do not. Never echo, print,
write, commit, or include its value in a run record. Record only the variable
name and the non-secret connection contract.

The wrapper does not emit a `{manifest}`. Stock and Mongoose Toast oracle
manifests and the fail-closed `option.PROMOTE_NUMBERS` profile gate are
Milestone 1 work. Until those artifacts exist and are supplied through the
reviewed profile flags, a run may prove managed behavior against a freshly
verified engine and fixture, but it must not be described as a profile-gated
convergence result.

## Required run record

Create a repository-local run record for every result that will be used as
authority. It must contain:

1. timestamp, working directory, Barn branch and exact HEAD, plus tracked-dirty
   state from `git status --short --untracked-files=no`;
2. selected oracle role, binary path, pinned SHA, freshly observed WSL HEAD,
   executable check, and version output;
3. selected source fixture's absolute path, byte size, last-write time, and
   freshly computed SHA-256;
4. applicable oracle and target profile-manifest paths, identities, checksums,
   and validation result. While Milestone 1 artifacts are unavailable, record
   that fact explicitly and label the result as not profile-gated;
5. the connection contract: managed `localhost` transport, selected test(s),
   logical permission/user, whether standard-property setup is skipped, any
   non-secret trusted-PROXY address asserted by the test, and the login-script
   environment variable name only;
6. the exact command after all test-selection and profile flags are added;
7. exit status, pass/fail/skip counts, focused assertion result, relevant
   artifact/log paths, and any observed startup or transport failure.

Do not record credentials or the value of `MONGOOSE_LOGIN_SCRIPT`. A passing
process-start check, nearby fixture, unverified WSL identity, or omitted profile
status is not a substitute for the required evidence.

## Failure handling

If the managed harness fails to start or connect, report the exact command,
working directory, expected result, and actual failure before improvising. Do
not guess at startup, transport, IPv4/IPv6, or harness behavior. If fresh WSL
identity or fixture verification is blocked, stop before Toast-side behavioral
claims or Barn-side diagnosis.

The complete cross-repository execution order and acceptance gates remain in
`plans/barn-toast-mongoose-convergence-workstreams.md`.
