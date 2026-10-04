# Recoverable checkpoint pair publication (#356)

## Reproduction and change

The first fault-injection run failed three cases: failed first database rename
left no loadable output; failed first sidecar rename changed WAIF identity; and
failed replacement sidecar rename left mismatching database/sidecar hashes.

```text
FAIL database_first: matching checkpoint pair unavailable
FAIL sidecar_first: WAIF identity 0 changed
FAIL sidecar_replace: WAIF identity sidecar does not match database
exit 1
```

The publisher now freezes previous and next generations and publishes a synced
versioned recovery journal before either output rename. The journal stores both
database and sidecar hashes, since identical portable dumps can have different
WAIF identities. Saved files are linked when possible, otherwise copied and
synced with bounded memory. Snapshot ownership and portable bytes are unchanged.

Loads and subsequent writes recover automatically under the same per-output
process lock. A complete next pair is kept; otherwise the previous pair is
restored, or the saved next pair completes first publication. Recovery preserves
its source files until the output and journal removal are synced. Ordinary and
panic checkpoint names remain separate. `RecoverCheckpoint` exposes the same
operation to library callers.

Failure paths retain publication and rollback causes. A journal rename error
also retains its saved directory, because an error need not prove that the
filesystem was unchanged. Corrupt or missing journal generations are rejected;
sidecar mismatches without journals remain errors. No recovery-directory scan or
unrelated cleanup was added.

## Verification

Windows Go 1.26.0:

```text
go test ./db/format ./cmd/barn -count=1
ok github.com/MongooseMoo/barn/db/format 4.514s
ok github.com/MongooseMoo/barn/cmd/barn 0.295s
exit 0
```

Debian WSL Go 1.24.6:

```text
go test ./... -count=1
go test -race ./db/format ./cmd/barn -count=1
go vet ./...
/root/go/bin/staticcheck ./...
ok github.com/MongooseMoo/barn/db/format 2.470s
ok github.com/MongooseMoo/barn/db/format 7.618s
ok github.com/MongooseMoo/barn/cmd/barn 1.784s
CHECKPOINT_FULL_RACE_STATIC_OK
exit 0
```

New regressions inject first/second rename failures for first and replacement
publication, all six directory-sync failure points, failed rollback, a journal
rename that reports failure after taking effect, identical dumps with different
identities, invalid/missing/corrupt recovery metadata, legacy sidecar-free
generations, and staging-name reuse. Real subprocesses exit without defers after
journal/database/sidecar publication and during database/sidecar rollback; the
parent reloads the recovered identities. Concurrent writers and readers share
one output. Long paths and panic-output isolation run on both platforms.

Existing exact-byte hashing, bounded hash allocation, identity and task-root
roundtrips, and CLI panic tests remain in the full test runs. Managed conformance
is provided by PR and merge-queue CI; no fresh baseline was requested or run.

## Durability boundaries

See [operator recovery and platform contract](../docs/checkpoint-recovery.md).
Unix directory-sync ordering is exercised by fault injection. Windows uses file
Sync and write-through replacement moves, including extended paths; this package
does not implement Unix directory fsync on Windows. Tests cover process crashes,
not physical power loss. No atomic two-file or blanket Windows durability claim
is made. One process must exclusively own each output/staging namespace;
in-process locks do not coordinate external publishers or path aliases.
