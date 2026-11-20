# Current campaign checkpoint 39

## State

- Draft PR #48 at first commit `f512248` removes the permanent never-executed baseline and makes the exact union fail on any case skipped by every Toast profile.
- First managed CI run `31017076556` has quality, 64-bit outbound-off, and 32-bit outbound-on green; 64-bit outbound-on is still running, after which the ledger is expected to provide red proof for the 41 old exceptions.
- Current uncommitted repair set preserves existing identities and fixes root causes:
  - 32-bit crashing modulus identity runs on 64-bit Toast.
  - Three PROMOTE_NUMBERS identities now assert pinned Toast strict numeric behavior without invalid gates.
  - `tostr_limit` raises the task tick budget so it isolates the configured string limit.
  - Three anonymous stack identities define verbs on a permanent class and invoke them through an anonymous child.
  - Three wildcard identities use the numeric verb descriptor when installing code.
  - `file_version_public` asserts the builtin is absent from the pinned Toast registry.
  - A fourth deterministic pinned-source example-extension profile enables the five `read_stdin` identities and is included in exact union accounting.
  - The GC worker is rewriting the 24 invalid direct-anonymous-property cases using inherited properties; its file is currently modified but not yet handed off.
- Managed WSL Toast proof passed all 15 currently repaired non-GC identities with a profile manifest: 15 passed, zero skipped/failures/errors.
- Focused workflow, ledger, and schema tests: 100 passed.

## Blocker

- The 24 GC rewrites still need worker handoff and managed Toast proof.
- The source-enabled example-extension profile must be proved by managed GitHub CI; the canonical WSL binary is the stock build and cannot exercise it without an unauthorized manual rebuild.

## Next action

Wait for the GC worker, review its exact diff, run managed WSL Toast on all 24 GC identities, then commit the behavior/profile repair as a separate commit and push PR #48 for the full four-profile zero-exception run.
