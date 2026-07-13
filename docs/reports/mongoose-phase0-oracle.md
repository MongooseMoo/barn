# Phase 0: Current WSL Toast and Mongoose fixture authority

Date: 2026-07-13

## Verdict

The managed wrapper design is feasible, but Toast verification is currently
blocked. The WSL worktrees and binaries could not be inspected from this
session: both WSL UNC forms were unavailable, and the active read-only tool
policy rejected `wsl -e` queries. Therefore the SHAs and build state below are
plan pins, not confirmed-current identities. They must be reverified before a
Toast result is authoritative.

The two local candidate database files are present, byte-identical, and exactly
100,959,239 bytes each. Their current SHA-256 and filesystem timestamps were not
available through the permitted tools, so the plan-time SHA-256 must not be
treated as revalidated.

## Oracle identities and command syntax

| Role | Required path | Required/pinned SHA | Current build state |
| --- | --- | --- | --- |
| Stock Toast | `/root/src/toaststunt/build-release/moo` | `aecc51e9449c6e7c95272f0f044b5ba38948459e` | Not currently verified |
| Mongoose Toast (`PROMOTE_NUMBERS`) | `/root/src/toaststunt-mongoose/build-release/moo` | `72e3c7f96ce7a41fdeba793aef8818dc4408072e` | Not currently verified |

Those paths and SHAs come from
`plans/barn-toast-mongoose-convergence-workstreams.md:37-49`. The untracked
`reports/toast-oracle-wsl.md:20-32` historically reports stock Toast version
`2.7.3_5` and a working release build, but that is not current verification.

Toast source defines the command as:

```text
moo [options] input-db-file output-db-file [-t|-p port-number]
```

The source consumes exactly two database arguments
(`src/db_file.cc:1330-1359`), accepts `-p PORT`
(`src/server.cc:1978-1985,2172-2177`), and also accepts a remaining numeric
positional port (`src/network.cc:1205-1236`). Therefore both of these forms are
syntactically valid:

```text
moo INPUT_DB OUTPUT_DB PORT
moo INPUT_DB OUTPUT_DB -p PORT
```

This source check was made against the nearby Windows checkout at
`C:/Users/Q/src/toaststunt`, HEAD
`e8a353665a106244f5e01edb67239c90411ae584`. That checkout is not the WSL oracle
and does not replace live verification of either required WSL worktree.

## Fixture identities

| Candidate | Current size | Current relationship | Current checksum | Timestamp |
| --- | ---: | --- | --- | --- |
| `C:/Users/Q/code/barn/mongoose.db` | 100,959,239 bytes | Byte-identical to `mongoose.db.new` | Git blob OID `e71ebfd629d8250c95d52898853208cbb073c950`; SHA-256 not reverified | Not verified |
| `C:/Users/Q/code/barn/mongoose.db.new` | 100,959,239 bytes | Byte-identical to `mongoose.db` | Same content as `mongoose.db`; SHA-256 not reverified | Not verified |

Current size came from a full-file `rg --stats` read of each file. Current
equality came from `git diff --no-index --exit-code -- mongoose.db
mongoose.db.new`, which exited 0. The plan-time SHA-256 for both was:

```text
A9D167861EAB56D62E9BD12AE1D47C5E6A858530020A5DCF174A0B104FB23DB9
```

That value is recorded at
`plans/barn-toast-mongoose-convergence-workstreams.md:105-114`; the plan itself
requires execution to re-hash the selected fixture. Select one path explicitly
and give both engines managed disposable copies of that same file.

## Managed wrapper contract

The current sibling conformance checkout is
`C:/Users/Q/code/moo-conformance-tests`, HEAD
`9300e724f7e091056a913b2cb0c7e4e8f98b9fdd`, branch `main` with no reported
tracked changes. Its managed server implementation:

- accepts `{port}`, `{db}`, `{manifest}`, and `{server_dir}` placeholders;
- requires managed mode to use `localhost`;
- copies the selected database into a Windows temporary directory;
- starts the command in that directory and waits up to 30 seconds for TCP;
- adopts common checkpoint outputs including `{db}.new` on restart;
- loads newline-separated login commands from the environment variable named by
  `--moo-login-script-env` and rejects an empty variable.

Evidence: `src/moo_conformance/plugin.py:71-208` and
`src/moo_conformance/server.py:63-138,180-228` in the sibling checkout.

From `C:/Users/Q/code/barn`, the exact Mongoose-oracle command shape is:

```powershell
uv run --project ..\moo-conformance-tests moo-conformance `
  --server-command "wsl -d Ubuntu -u root -e env TOAST_MOO=/root/src/toaststunt-mongoose/build-release/moo bash /mnt/c/Users/Q/code/barn/scripts/run_toast_wsl.sh {db} {port}" `
  --server-db C:/Users/Q/code/barn/mongoose.db `
  --moo-login-script-env MONGOOSE_LOGIN_SCRIPT
```

`MONGOOSE_LOGIN_SCRIPT` is a caller-chosen Windows environment variable
containing the uncommitted, newline-separated raw login conversation. The
credentials must not be written to the repository. The wrapper receives the
Windows temporary `{db}` path, converts it with `wslpath`, then executes:

```text
$TOAST_MOO DB_WSL DB_WSL.new PORT
```

Required environment/path facts:

- WSL distro `Ubuntu` must exist and user `root` must own/read the two oracle
  worktrees;
- `/mnt/c/Users/Q/code/barn/scripts/run_toast_wsl.sh` and `wslpath` must be
  available inside that distro;
- Windows temporary paths must be visible through `/mnt/c`;
- `TOAST_MOO` must be set explicitly to the Mongoose binary for this lane;
  otherwise the wrapper defaults to stock Toast;
- the login-script variable is consumed by the Windows Python harness, not by
  Toast or WSL.

The command is not yet a fully gated convergence command. The wrapper does not
write `{manifest}`, no stock/Mongoose Toast profile manifests exist here, and
Barn's Mongoose profiles do not declare `option.PROMOTE_NUMBERS`. Profile-gated
comparison remains Milestone 1 work.

## Active instruction drift

- `AGENTS.md:3-17` is aligned: managed conformance only, WSL release Toast only,
  and stop if Toast verification is blocked.
- `CLAUDE.md:88-118,173-224` conflicts with it by directing agents to a Windows
  Toast binary, manual server lifecycle, and older direct harness commands.
- `reports/toast-oracle-wsl.md` is untracked even though `AGENTS.md` names it as
  canonical. Its claim at lines 68-71 that `CLAUDE.md` was corrected is not true
  of the current working file.
- `scripts/wsl_oracle.sh:5-18` defaults to stale
  `~/src/toaststunt/build/moo` rather than `build-release/moo` and manually
  launches emergency-mode Toast. It is not the managed conformance path.
- `scripts/run_toast_wsl.sh` is compatible with managed lifecycle and restart
  checkpoints, but defaults to stock Toast and has no profile-manifest output.
- `profiles/barn/profiles.json:32-57,88-113` names Mongoose profiles while the
  only config files set `OUTBOUND_NETWORK`; neither config nor expected feature
  metadata enables `PROMOTE_NUMBERS`.

## Blockers before Toast verification

1. Restore read access to WSL and verify both worktree SHAs, executable presence,
   and versions directly. Do not inherit the plan pins as current facts.
2. Recompute SHA-256 and timestamps for the selected database immediately before
   the run; require SHA-256
   `A9D167861EAB56D62E9BD12AE1D47C5E6A858530020A5DCF174A0B104FB23DB9`
   only if the fresh hash actually matches.
3. Replace or explicitly supersede the Windows/manual commands in `CLAUDE.md`
   and make the WSL managed procedure a tracked active record.
4. Define truthful stock and Mongoose oracle manifests and add
   `option.PROMOTE_NUMBERS` to the Mongoose profile contract before claiming a
   profile-gated comparison.

Until blockers 1 and 2 are resolved, no Toast server should be launched and no
Barn-side behavioral diagnosis should begin.
