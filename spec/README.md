# Barn MOO specification index

This directory records MOO behavior and the current Barn implementation.
MOO-observable semantics and durable database behavior must match freshly
verified Toast source and managed Toast behavior. Barn's Go code, package maps,
and implementation notes describe current status; they do not redefine Toast.

The verified WSL source SHA, executable, fixture, profile, wrapper, and exact
managed command are recorded in
[`toast-oracle-identity-2026-07-14.md`](../../banteng/docs/reports/toast-oracle-identity-2026-07-14.md).
The managed workflow is owned by
[`barn-toast-mongoose-convergence-workstreams.md`](../plans/barn-toast-mongoose-convergence-workstreams.md).
The older `reports/toast-oracle-wsl.md` path is stale and absent.

## Documents

| Document | Subject | Phase 0 audit status |
|---|---|---|
| [grammar.md](grammar.md) | Grammar and operator precedence | Not part of this audit |
| [types.md](types.md) | Values, types, and conversions | Not part of this audit |
| [operators.md](operators.md) | Operator semantics | Not part of this audit |
| [statements.md](statements.md) | Control-flow statements | Not part of this audit |
| [errors.md](errors.md) | Error values and conditions | Not part of this audit |
| [objects.md](objects.md) | Object model and inheritance | Not part of this audit |
| [tasks.md](tasks.md) | Toast task semantics and current Barn task behavior | Corrected against current Toast and Barn |
| [vm.md](vm.md) | Toast VM authority and current Barn compiler/VM map | Corrected against current Toast and Barn |
| [database.md](database.md) | Toast v17 format and current Barn codec | Corrected against current Toast and Barn |
| [go-design.md](go-design.md) | Non-normative current Go package and ownership map | Corrected against current Barn |
| [builtins/](builtins/) | Built-in function specifications | Not part of this audit |

The presence of a document is not a completeness or conformance claim. The
current Phase 0 audit covers only `tasks.md`, `go-design.md`, `vm.md`,
`database.md`, and this index.

## Built-in specification files

| Category | File |
|---|---|
| Type conversion | [builtins/types.md](builtins/types.md) |
| Math | [builtins/math.md](builtins/math.md) |
| Strings | [builtins/strings.md](builtins/strings.md) |
| Lists | [builtins/lists.md](builtins/lists.md) |
| Maps | [builtins/maps.md](builtins/maps.md) |
| Objects | [builtins/objects.md](builtins/objects.md) |
| Properties | [builtins/properties.md](builtins/properties.md) |
| Verbs | [builtins/verbs.md](builtins/verbs.md) |
| Tasks | [builtins/tasks.md](builtins/tasks.md) |
| Time | [builtins/time.md](builtins/time.md) |
| JSON | [builtins/json.md](builtins/json.md) |
| File IO | [builtins/fileio.md](builtins/fileio.md) |
| Network | [builtins/network.md](builtins/network.md) |
| Server | [builtins/server.md](builtins/server.md) |
| Crypto | [builtins/crypto.md](builtins/crypto.md) |
| Regular expressions | [builtins/regex.md](builtins/regex.md) |
| SQLite | [builtins/sqlite.md](builtins/sqlite.md) |
| External execution | [builtins/exec.md](builtins/exec.md) |

## Verification

The shared behavioral rows live in
[`moo-conformance-tests`](../../moo-conformance-tests/). A row is authority only
after it passes against the exact managed stock-Toast command and profile named
in the current identity record. Focused Go tests and Barn-only conformance runs
can prove Barn behavior, but they do not establish Toast behavior.

Barn's managed server command templates and support classifications are owned
by [`profiles/barn/profiles.json`](../profiles/barn/profiles.json). Its Linux
profiles are supported; its Windows profiles are diagnostic until paired with a
matching Windows Toast oracle. Do not present a diagnostic Windows run as the
primary conformance gate.

Run the managed Toast command only against the disposable fixture controlled by
the harness. Never run Toast directly against the tracked fixture.

## Sources and authority order

1. Freshly verified Toast source and managed Toast behavior at the identity
   recorded above.
2. Durable `moo-conformance-tests` rows proven against that exact Toast profile.
3. Normative language and persistence statements in this directory that have
   been checked against those owners.
4. Current Barn implementation, for implementation status and divergences.
5. Structural and implementation references, for comparison only.

Additional references:

- Toast source: `/root/src/toaststunt` in the verified WSL checkout;
- [LambdaMOO Programmer's Manual](https://www.hayseed.net/MOO/manuals/ProgrammersManual.html);
- [ToastStunt documentation](https://github.com/lisdude/toaststunt-documentation);
- [`moo_interp`](../../moo_interp/), an implementation reference; and
- [`lambdamoo-db-py`](../../../src/lambdamoo-db-py/), a structural database
  reference.

If a Barn specification or reference disagrees with verified Toast, correct the
durable specification or conformance row before using it to drive implementation.
