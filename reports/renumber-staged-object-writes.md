# Staged writes across renumber (#324)

Stock WSL Toast preserves name, owner, read/write/fertile flags, property values
and verb code when a task changes them immediately before renumber. Barn lost
the scalar and verb-code updates after an irreversible effect had already
crossed the task boundary; the property update survived.

The test-owned driver first executes `server_log`, then stages the updates and
renumbers into an actually lower free identity. Separate first-boundary and
resume-only controls prevent a topology flush from concealing the failure.
Each boundary is covered with another owner and a self-owned target. Checks in
the driver and a subsequent task prove both read-your-writes and persistence.

## Red evidence

The strengthened four-case suite used the same managed disposable Test.db on
both engines, always with canonical capability admission:

```text
Stock Toast: 5 passed, 1 warning in 6.91s; exit 0
Unmodified Barn 71301246ba06e0ccb9d626428a2f18e4e7edaf94:
  expected [1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
  actual   [1, 1, 1, 0, 0, 0, 0, 0, 1, 0]
  after_irreversible_boundary: failed
  after_resume_and_irreversible_boundary: failed
  2 failed, 3 passed, 1 warning in 8.20s; exit 1
```

The original recycle-based boundary was insufficient: staged topology can flush
the later updates before renumber. It was replaced before diagnosis and repair.
An initial managed Barn startup timed out before admission; the identical
command passed on retry. The timeout's cause remains unknown. Its output was
`Managed server did not accept connections on port 37935 within 30.0s`, ending
with `1 error in 36.95s`; no semantic result was inferred from that attempt.

## Repair

Renumber transfers the target's scalar and verb-code staging maps alongside
its existing property operations before forgetting the old identity. It
reapplies those writes to the adopted private image, preserving alias selection,
compiled code keys and read-your-writes. A staged self-owner follows the new
identity. Commit retains the transferred writes for publication.

The property-only transfer/apply methods were replaced with object-write
methods, and their callers updated. Existing authority checks, coarse topology
publication, error paths and anonymous relationship adoption remain in place.
The transaction regression checks committed results, an unrelated staged write,
verb aliases, unchanged live values before commit and the immutable prior
snapshot.

## Managed verification context

Oracle source: `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
Oracle executable: `/root/src/toaststunt/build-release/moo`.
Executable SHA-256:
`72fb1cf96cb303647a8ee72808e7c1ff62a491ecf44f547e6757e71ba2402bde`.
Test.db SHA-256:
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
Initial managed controller: `9b4163e2d1171428c65351a98c59ac1e0a26d849`.
Final packaged controller base: `fe028c4fdfeeabfe8ed4498e6ec1ec7c7494ee14`.
Linux Go: 1.24.6. Windows Go: 1.26.0.

The generic YAML belongs in the companion conformance repository at
`src/moo_conformance/_tests/builtins/renumber_staged_writes.yaml`. Barn's CI uses
that repository's main branch. The final focused commands, from this checkout
inside verified Debian WSL, are:

```bash
set -euo pipefail
suite=/mnt/c/Users/Q/code/moo-conformance-issue-324
for target in conformance-toast conformance; do
  make "$target" CONFORMANCE_SUITE="$suite" \
    CONFORMANCE_ENV=/root/.cache/barn-324-packaged-linux \
    CONFORMANCE_PATHS="$suite/src/moo_conformance/_tests/builtins/renumber_staged_writes.yaml" \
    K=renumber_staged_writes
done
```

The managed make targets retain admission, verified profile metadata, strict
markers and unexpected-skip checking. No fresh full conformance baseline was
collected; PR and merge-queue CI supply the complete suite.

```text
Final packaged stock Toast: 9 passed in 7.73s; exit 0
Final packaged repaired Barn: 9 passed in 15.53s; exit 0
Windows go test ./db/store ./builtins: ok (1.372s, 6.683s)
Linux go test ./...: exit 0
Linux go test -race ./db/store ./builtins ./engine:
  ok (3.836s, 64.956s, 205.365s)
go vet ./...; staticcheck ./...:
  RENUMBER_FULL_RACE_STATIC_OK; exit 0
```
