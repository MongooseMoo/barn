# MOO database format and current Barn codec

## Status and authority

The database file is a durable MOO-observable contract. Freshly verified Toast
source and managed Toast behavior are authoritative. Barn's Go structs and
codec limitations describe current implementation status; they do not redefine
the format.

The verified WSL source identity, executable, profile, wrapper, and exact
managed command are recorded in
`../../banteng/docs/reports/toast-oracle-identity-2026-07-14.md`. The managed
workflow is owned by
`plans/barn-toast-mongoose-convergence-workstreams.md`. In the verified Toast
checkout:

- `src/db_file.cc` owns the top-level version dispatch, section order, object
  records, anonymous-object batches, and verb-program section;
- `src/db_io.cc` owns line, number, float, string, tagged-value, activation, and
  program IO;
- `src/db_objects.cc`, `src/db_properties.cc`, and `src/db_verbs.cc` own the
  corresponding world records and anonymous references;
- `src/waif.cc` owns WAIF creation/reference records;
- `src/eval_vm.cc` owns suspended VM read/write; and
- `src/tasks.cc` owns clocks, queued tasks, suspended tasks, interrupted tasks,
  and active-connection task state.

These paths are relative to `/root/src/toaststunt` at the recorded source SHA.
`../../../src/lambdamoo-db-py` is useful for structural inspection and
differential round trips, but it does not override verified Toast.

## 1. Supported versions

Toast's `DB_Version` enum in `src/include/version.h` is append-only and its
numeric value is written in the header. Relevant milestones are:

| Version | Toast enum milestone | Relevant format effect |
|---|---|---|
| 4 | `DBV_BFBugFixed` | Last pre-next-generation object layout; includes floats and exception-era VM data |
| 5 | `DBV_NextGen` | Next-generation object layout and locality |
| 6 | `DBV_TaskLocal` | Task-local value in suspended VM data |
| 7 | `DBV_Map` | Map values |
| 10 | `DBV_Interrupt` | Interrupted task section |
| 13 | `DBV_Anon` | Pending finalizations, early task sections, repeated object batches, anonymous objects |
| 14 | `DBV_Waif` | WAIF values |
| 15 | `DBV_Last_Move` | `last_move` object field |
| 16 | `DBV_Threaded` | Activation thread-mode persistence |
| 17 | `DBV_Bool` | Boolean values; current Toast output version |

The required Barn contract is to read LambdaMOO v4 and Toast v17 and to write
v17. Current `db/format/reader.go` also dispatches v5. Current
`db/format/writer.go` writes only v17.

## 2. Version 17 top-level order

The header is:

```text
** LambdaMOO Database, Format Version 17 **
```

The top-level data then appears in this order:

1. player count and player object IDs;
2. values pending finalization;
3. obsolete clocks;
4. queued fork tasks;
5. suspended tasks;
6. interrupted tasks;
7. formerly active connections;
8. a count followed by the permanent object records;
9. zero or more later object batches containing anonymous objects, terminated
   by a count of zero;
10. verb-program count; and
11. verb-program headers and source.

`write_task_queue()` in `src/tasks.cc` owns items 3 through 6. The repeated
object-count loop in `src/db_file.cc` owns both the first permanent batch and
later anonymous batches; the zero is the loop terminator, not a second
independent single-count section.

## 3. Tagged values

Outside enclosing record headers, `dbio_write_var()` writes a numeric type tag
line and then the type-specific payload. Containers recursively contain tagged
values. The accepted database tags are:

| Code | Type | Payload |
|---:|---|---|
| 0 | INT | integer line |
| 1 | OBJ | signed object-number line, without a required `#` prefix |
| 2 | STR | one raw newline-terminated string line |
| 3 | ERR | numeric error-code line |
| 4 | LIST | length, then that many tagged values |
| 5 | CLEAR | no payload |
| 6 | NONE | no payload |
| 7 | CATCH | numeric internal VM marker |
| 8 | FINALLY | numeric internal VM marker |
| 9 | FLOAT | textual floating-point line |
| 10 | MAP | pair count, then tagged key/value pairs |
| 11 | ITER | accepted on read as a following tagged value; current Toast output writes an iterator's current value or CLEAR instead |
| 12 | ANON | numeric anonymous-object serialization ID |
| 13 | WAIF | creation or reference record owned by `src/waif.cc` |
| 14 | BOOL | numeric 0 or 1 |

Toast writes floats with `DBL_DIG + 4` significant digits. A WAIF creation uses
`c <index>`, class, owner, indexed non-clear properties, `-1`, and `.`; a later
reference uses `r <index>` and `.`. Every reference to one index reloads as the
same WAIF identity, including after dump and managed restart; mutation through
one alias is therefore visible through every other alias. Anonymous values
allocate or reuse an above-permanent-range serialization ID and write that
number.

## 4. Next-generation object records

A live object record written by `ng_write_object()` has this sequence:

```text
#<object-id>
name
flags
owner
<tagged location value>
<tagged last_move value>
<tagged contents value>
<tagged parents value>
<tagged children value>
verb-definition count
<verb definitions>
local property-definition count
<local property names>
property-value count
<property values>
```

Each verb definition is `name`, owner object number, permissions, and
preposition code. Each property value is a tagged value, owner object number,
and permissions. The property-value sequence covers local and inherited
properties; only locally defined names appear in the preceding property-name
list.

Object flag bit positions are defined by `src/include/db.h`:

| Bit | Flag |
|---:|---|
| 0 | USER |
| 1 | PROGRAMMER |
| 2 | WIZARD |
| 3 | obsolete |
| 4 | READ |
| 5 | WRITE |
| 6 | obsolete |
| 7 | FERTILE |
| 8 | ANONYMOUS |
| 9 | INVALID anonymous object |
| 10 | RECYCLED anonymous object pending storage release |

A recycled permanent slot is a structural record of the form
`#<object-id> recycled`; it is not encoded by setting the anonymous lifecycle
flags in a normal object record.

## 5. Anonymous batches and verb programs

The object-count loop first writes all permanent slots through the current
maximum object ID. Serializing anonymous references can allocate later
serialization IDs, so the loop writes additional batches until the maximum no
longer grows, then writes `0`. A reader must continue through every batch; it
must not treat the first count after permanent objects as the whole anonymous
section.

After the zero terminator, each verb program has a zero-based header followed by
source terminated by a line containing `.`:

```text
#<object-id>:<zero-based-verb-index>
<source lines>
.
```

The verb metadata record and the later program record are distinct. A verb with
no installed program has metadata but no program-section entry.

## 6. Task persistence

Toast writes an obsolete clock header first. A queued fork record is:

```text
0 <first-line> <start-time> <task-id>
<programmer-interface activation>
<runtime environment>
<fork program source, terminated by .>
```

A suspended record is:

```text
<start-time> <task-id> <tagged resume value>
<VM>
```

`src/eval_vm.cc` writes the VM as the tagged task-local value; a header holding
top activation index, root vector, builtin continuation ID, and maximum stack
size; then every activation. Interrupted records use `<task-id> <status>` plus
the same VM. On load, Toast queues them for immediate resumption with
`E_INTRPT`.

Current Barn does not implement that full contract. `db/format/writer_task.go`
can write serializable queued forks but emits zero suspended and interrupted
tasks. `db/format/reader_task.go` reads queued forks but skips complete suspended
and interrupted task records without reconstructing either task class. Passing a
checkpoint through Barn therefore does not preserve suspended or interrupted
execution state.

## 7. Current Barn IO and checkpoint behavior

`db/format` reads through `bufio.Reader` and writes through `bufio.Writer` while
building a full in-memory `Database` or consuming a full `db/store` snapshot.
It does not transcode or validate character encoding: database string bytes are
carried in Go strings and written back as bytes. Treat the durable format as
Latin-1-compatible byte data; do not apply Unicode normalization or implicit
UTF-8 conversion.

`db/format/checkpoint.go` currently creates `<path>.tmp`, writes and flushes the
buffered v17 database, closes the file, and renames it to `<path>.new`. It never
replaces `<path>` and does not call `File.Sync()`. Therefore the historical
`os.Rename(tmp, dbPath)` snippet did not describe current Barn and did not prove
atomic replacement or crash durability.

The release contract remains functional v4/v17 input and v17 output. A round
trip must preserve MOO-visible world, source, value, topology, task, and
connection state required by the selected profile. Repeated WAIF references
must remain one identity across round trip and restart; textual byte identity
is not required. Narrow object-only or value-only round trips do not prove the
full database contract.

## 8. Verification boundary

Use the managed WSL Toast profile for bootability and MOO-observable behavior.
Use `../../../src/lambdamoo-db-py` for structural differential reads and writes.
Focused Go codec tests can prove local parsing and output mechanics, but they do
not replace Toast boot, managed restart/task rows, or full functional round
trips. Never run a tracked fixture in place.
