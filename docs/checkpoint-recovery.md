# Checkpoint publication and recovery

An ordinary checkpoint uses `database.new` and `database.new.waifids`.
An emergency checkpoint uses `database.new.PANIC` and
`database.new.PANIC.waifids`. The input database is never the publication target.
The portable database format and version-1 WAIF identity sidecar are unchanged.

Two file renames cannot publish a pair atomically. Barn saves matching
generations before changing either output file. For output `database.new`,
publication creates:

- `database.new.recovery-<random>/next` and `next.waifids`;
- `previous` and `previous.waifids` in that directory when an output exists;
- `database.new.publication`, a small versioned recovery journal containing
  the recovery directory name and hashes of both files in each generation.

A previous portable database without a sidecar remains a supported generation.
The sidecar hash matters independently: two identical portable dumps can carry
different stable WAIF identities. Recovery validates both hashes and the sidecar
header before changing any output. It never accepts a mismatching sidecar.

Saved files use hard links when available; otherwise Barn copies and syncs them
with bounded memory. The publisher treats every saved inode as immutable.
The journal is published only after the saved pairs and journal file are ready.
The database and sidecar are then replaced separately. After publication is
synced, Barn removes and syncs the journal before deleting the saved generations.

## After a failed or interrupted publication

`LoadDatabase(output)` and the next checkpoint write recover a pending journal
automatically. Library callers can also call `format.RecoverCheckpoint(output)`;
pass the output name, including `.new` or `.new.PANIC`, rather than the input name.

Recovery keeps a complete newly published pair. Otherwise it restores the
previous generation. If this was the first publication, it completes the saved
next generation. Recovery itself can be interrupted and retried: restoration
never moves away the only saved copy. A reported publication error can therefore
coexist with a usable newly published pair; callers must still handle the error.

If permissions or storage errors also prevent recovery, the error identifies
the saved directory and preserves both error causes. Keep the journal and saved
files together, resolve the filesystem problem, and retry loading or recovery.
Do not edit or truncate linked files, remove the pending journal, or delete its
saved directory. Invalid journals, missing saved files, or hash failures are
reported rather than silently bypassed. With no journal, ordinary strict
sidecar validation still applies.

A failed journal rename can leave an unreferenced recovery directory because
the filesystem may have changed even when the rename reported an error. A
failure after journal removal can also leave saved files. These leftovers are
deliberately retained; automatic recovery does not scan or delete unrelated
directories. Operators may remove a leftover only after establishing that no
pending journal references it and that the desired output pair loads correctly.

## Filesystem durability and ownership

On Unix, staged/copied files and the journal are synced. Barn syncs the saved
directory and its parent before journal publication, syncs the parent after
journal publication and output replacement, then syncs journal removal before
discarding backups. Filesystems must support file and directory `Sync`; errors
are returned. The protocol relies on those operations' filesystem guarantees.

On Windows, staged/copied files and the journal use file `Sync`; replacements
use `MoveFileExW` with `MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH`,
including extended paths. The package has no Unix-style directory sync on
Windows. Go documents that rename is not necessarily atomic on non-Unix
platforms, and Windows write-through moves are not a substitute for a verified
directory-fsync/power-loss guarantee. See [Go's rename contract](https://pkg.go.dev/os#Rename)
and [Microsoft's move flags](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw).

Fault injection and abrupt subprocess exits verify error handling and process
crash recovery on the tested platforms. They do not simulate hardware power
loss. Storage devices, network filesystems, and Windows directory durability
need platform-specific power-loss validation before stronger guarantees are made.

Loads, writes, and recovery of the same absolute output path are serialized
within one process; ordinary and emergency outputs have separate locks. Entries
are released when the operations finish. The journal is not an interprocess
lock: one publisher must exclusively own an output and its staging names.
External readers must use Barn recovery, and external programs must not modify
saved files in place. Different aliases of the same path are not an ownership
mechanism. Keep the containing directory under that publisher's control.
