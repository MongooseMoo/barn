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
The stock oracle is now ToastStunt 2.8.0_7, commit
`03af4fa2b53b418999f03566b646ee42bc7ea647`
(`profiles/toast/stock-wsl-testdb.json`); the `file:line` citations below are
to upstream `origin/master`, which contains that commit, and every cited line
is the same at both.
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
| 11 | `DBV_This` | Typed `this` value in saved activations |
| 13 | `DBV_Anon` | Pending finalizations, early task sections, repeated object batches, anonymous objects |
| 14 | `DBV_Waif` | WAIF values |
| 15 | `DBV_Last_Move` | `last_move` object field |
| 16 | `DBV_Threaded` | Activation thread-mode persistence |
| 17 | `DBV_Bool` | Boolean values; current Toast output version |

Toast reads the version with `scanf("** LambdaMOO Database, Format Version %u
**\n")`; a file without that header is `DBV_Prehistory`. A version
`check_db_version()` rejects logs `READ_DB_FILE: Unknown DB version number: N`
and the load fails (`src/db_file.cc:848-855`). Below `DBV_Anon` the header is
followed by four counts: objects, verb programs, an ignored number, and users.
From `DBV_Anon` on only the user count follows; object counts come later
(`src/db_file.cc:863-875`).

The required Barn contract is to read LambdaMOO v4 and Toast v17 and to write
v17. Current `db/format/reader.go` also dispatches v5. It accepts only the
three complete header lines for versions 4, 5 and 17 (outer whitespace
trimmed); any other header, including a numeric prefix such as `Version 170`,
is an unsupported format. Current `db/format/writer.go` writes only v17.

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

Toast writes floats with `DBL_DIG + 4` significant digits. A MAP's pairs are
written in the map's traversal order (`mapforeach`) and read back by
inserting each pair in file order (`src/db_io.cc:187-195`, `:379-381`). A
string payload, and every other string line read with `dbio_read_string()`,
loses only its terminating newline; a carriage return before it stays in the
value (`src/db_io.cc:113-138`). A WAIF creation uses
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
list. A property-definition name line keeps a carriage return that precedes
its newline, so a name `tail` plus CR loads as five characters and is written
back unchanged by every checkpoint
(`server/startup_propdef_trailing_cr.yaml`).

`last_move` is written with `dbio_write_var()` (`src/db_file.cc:420`), so an
anonymous object referenced only from a `last_move` value is serialized and
reloaded like one referenced from a property value; it remains a valid
anonymous object after `dump_database()` and restart
(`server/last_move_anon_checkpoint.yaml`). Before `DBV_Last_Move` the field is
absent and loads as an empty map.

How a numbered record is placed depends on the version
(`src/db_file.cc:917-929`):

- below `DBV_NextGen`, `v4_read_object()` requires the record's `#N` to be the
  last used ID plus one (`src/db_file.cc:231`). Any other label fails the
  load: Toast logs `READ_DB_FILE: Bad object #I.`, where `I` is the record's
  zero-based position, and the server exits 1
  (`server/startup_object_label_v4.yaml`);
- from `DBV_NextGen` on, `ng_read_object(0)` reads a numbered record's label
  and ignores it: the record takes the next slot by position
  (`dbpriv_new_object(-1)`, `src/db_file.cc:316-321`). A record labelled `#9`
  in position 2 loads as `#2`, and `#9` is not created
  (`server/startup_object_label_v5.yaml`). Records in the later anonymous
  batches are found by their label.

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

A recycled permanent slot is a structural record; it is not encoded by setting
the anonymous lifecycle flags in a normal object record. Toast writes it with
`printf("#% d recycled\n")`, giving `# 9 recycled` with a space before the
number (`src/db_file.cc:407-409`); the reader scans `#%d`, which accepts the
number with or without that space. A recycled slot still occupies an ID:
`dbpriv_new_recycled_object()` advances the object count
(`src/db_objects.cc:262-266`), and `max_object()` returns that count minus one
(`src/objects.cc:301-309`). Recycled slots above the highest valid object
therefore survive a load: `max_object()` still counts them, and `create()`
allocates the next ID after them (`db_create_object(-1)`,
`src/objects.cc:390`; `src/db_objects.cc:226-230`).

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

Toast writes every program in the file (verb programs, saved activations, and
queued fork bodies) as `unparse_program()` output, fully parenthesized and not
indented, followed by `.` (`src/db_io.cc:410-421`). An empty program is the
`.` line alone. On read, a `.` at the start of any line ends the program; the
character after it is skipped (`my_getc`, `src/db_io.cc:254-268`). Toast never
writes a source line that starts with `.`, because unparsed code cannot begin
with one.

The loader parses programs with a warning callback. A call to a builtin the
server does not have still compiles: the parser replaces it with
`call_function("name", @args)` and reports a warning (`src/parser.y:496-515`),
which the loader logs as `PARSER: Warning in #N:verb:` followed by
`Line L:  Unknown built-in function: name` (`src/db_io.cc:246-251`). The verb
loads, `verb_code()` shows the `call_function(...)` form, and reaching the
call raises `{E_INVARG, "Unknown built-in function", "name"}`.
`set_verb_code()` still rejects the same call
(`server/startup_unknown_builtin_load.yaml`). Current Barn applies the same
rewrite to every loaded verb before building the store, on the server,
inspection, and `moo` load paths (`db/format/unknown_builtins.go`).

## 6. Task persistence

`write_task_queue()` (`src/tasks.cc:1927-2006`) writes `0 clocks`, then
`N queued tasks` and the queued records, `N suspended tasks` and the suspended
records, and `N interrupted tasks` and the interrupted records. Within each
section it writes the waiting list, which is ordered by start time, then each
active task queue's background tasks. On load, `enqueue_waiting()` places a
task after every task with the same start time (`src/tasks.cc:1183-1205`), so
file order is the run order of tasks due at the same moment.

A queued fork record is (`write_forked_task()`, `src/tasks.cc:1904-1913`):

```text
0 <first-line> <start-time> <task-id>
<programmer-interface activation>
<N variables, then each name and tagged value>
<fork program, terminated by .>
```

- `<first-line>` is the verb line where the fork body starts
  (`find_line_number(program, f_index, 0)`); `<start-time>` is rounded to the
  second.
- The program is the unparse of the fork vector alone, fully parenthesized and
  not indented (`dbio_write_forked_program()`); no text is cut from the verb's
  source. An empty `fork ... endfork` body is the `.` line alone, and the empty
  task survives repeated dumps and restarts
  (`server/empty_fork_checkpoint_restart.yaml`). Bodies ending in a block
  statement, and bodies whose `try` has computed except codes, reload and run
  completely (`server/fork_trailing_block_checkpoint_restart.yaml`,
  `server/fork_try_computed_codes_checkpoint_restart.yaml`).
- For `fork name (...)`, the saved environment already binds `name` to the
  child's own task ID: `enqueue_forked_task2()` stores the ID before copying
  the environment (`src/tasks.cc:1283-1289`). The restored task sees that ID,
  not the parent's earlier value
  (`server/fork_variable_checkpoint_restart.yaml`).
- On load, `read_task_queue()` reorders the variables by name to the compiled
  program and sets `program->first_lineno` to `<first-line>`
  (`src/tasks.cc:2063-2071`), so line numbers in the restored task count from
  the verb line, not from 1. Nothing else sets `first_lineno`: a new program
  starts at 1 (`src/program.cc:33`), and an activation is saved as its program
  text alone (`write_activ()`, `src/execute.cc:4004-4011`) and reparsed on
  load (`src/execute.cc:4067-4068`). A restored queued fork that then suspends
  and is saved again therefore loses its first line; after the next load its
  line numbers count from 1 (`find_line_number()` starts from
  `first_lineno`, `src/decompile.cc:1071`).

The programmer-interface activation (`write_activ_as_pi()`,
`src/execute.cc:3848-3866`) is a tagged INT `-111`; typed `this` (from
`DBV_This`); typed verb location (from `DBV_Anon`); the thread mode (from
`DBV_Threaded`); the numeric row `recv -7 -8 player -9 progr vloc -10 debug`;
the four strings `No`, `More`, `Parse`, `Infos`; then the verb name and the
full verb-name string. The reader takes each typed field only from its version
on (`src/execute.cc:3869-3895`), so in a v4 file the numeric row follows the
`-111` value directly.

A suspended record is:

```text
<start-time> <task-id> <tagged resume value>
<VM>
```

A missing resume value reads as `0`. `src/eval_vm.cc:86-135` writes the VM as
the tagged task-local value (from `DBV_TaskLocal`); a header holding top
activation index, root vector, builtin continuation ID, and maximum stack size
(optional on read); then every activation, bottom first. The bottom activation
runs in the root vector, the others in the main vector. Interrupted records
use `<task-id> <status>` plus the same VM; a task waiting in `read()` or
`read_http()` is written as `<task-id> interrupted reading task`. On load,
Toast queues interrupted tasks for immediate resumption with `E_INTRPT`.

Each activation (`write_activ()`, `src/execute.cc:4004-4027`) is:

```text
language version <n>
<program, terminated by .>
<N variables, then each name and tagged value>
<N> rt_stack slots in use
<that many tagged operand values>
<programmer-interface activation>
<tagged temporary value>
<pc> <bi_func_pc> <error_pc>
[builtin name and its saved state, only when bi_func_pc is not 0]
```

The reader accepts the pc line without `<error_pc>`, which then equals `<pc>`.
`check_pc_validity()` requires `<pc>` to follow an `OP_CALL_VERB` or
`OP_BI_FUNC_CALL` in the activation's own vector (`src/execute.cc:4029-4042`).
A bad pc, an unknown builtin name, or bad builtin state fails
`read_task_queue()`, and with it the whole database load.

The saved operand stack holds Toast's own operands at that call:

- **Builtin continuations live on the callee.** When a builtin pushes a verb or
  eval activation, `OP_BI_FUNC_CALL` stores the builtin's ID, resume pc and
  data on the activation now running, which is the pushed one
  (`src/execute.cc:2285-2289`). A task suspended inside `eval()` code saves
  builtin `eval`, pc 2, no data, on the eval activation; when that activation
  returns, `bf_eval` resumes at 2 and returns `{1, value}`
  (`src/verbs.cc:604-650`). A task suspended in a `move()` hook saves builtin
  `move` on the hook verb's activation, with pc 2 for `:accept`, 3 for
  `:exitfunc` and 4 for `:enterfunc`, and the state line
  `bf_move data: what = N, where = N, position = N`
  (`src/objects.cc:117-200`, `:234-250`).
- **A finally body runs on two operands:** the reason it was entered and a
  value (`src/execute.cc:264-276`). The reasons are `FIN_FALL_THRU` 0,
  `FIN_RAISE` 1, `FIN_UNCAUGHT` 2, `FIN_RETURN` 3 and `FIN_EXIT` 5 (break or
  continue). For `FIN_UNCAUGHT` the value is `0`: the `handle_uncaught_error`
  arguments are held outside the task (`src/execute.cc:545-563`) and cleared
  whenever the interpreter starts or resumes a task
  (`src/execute.cc:3119-3120`, `:3279-3303`). A task that suspends inside a
  finally body entered for an uncaught error therefore ends, when the body
  finishes, without calling `#0:handle_uncaught_error`, live or after a
  checkpoint.
- **A range loop's lower bound is an ordinary operand** while its upper bound
  is evaluated: the code is lower bound, upper bound, `OP_FOR_RANGE`
  (`src/code_gen.cc:1030-1037`).
- **Computed except codes are saved state.** A task suspended inside a `try`
  or catch expression whose codes are variables keeps the evaluated codes and
  catches with them after restart
  (`server/suspended_computed_codes_checkpoint_restart.yaml`). A task
  suspended in a catch expression's handler resumes at the catch's entry
  stack depth: the guarded expression's result is gone and the enclosing
  operands remain (`server/catch_handler_suspend_checkpoint_restart.yaml`).

Current Barn reads and writes all three task sections:

- **Queued forks.** The saved code is the compiler's unparse of the fork body,
  fully parenthesized and not indented (`bytecode.Program.ForkSources`). It
  spells floats exactly rather than with Toast's `%.15g`, a deliberate
  difference so values survive the checkpoint. The header's first line is kept
  and applied to the compiled body (`Program.WithFirstLine`); an empty body is
  written as `.` with first line 1; the fork variable holds the child's task ID.
- **Suspended and interrupted VMs.** `engine.Runtime.TaskSnapshots()` returns
  tasks in scheduler order (ready time, enqueue order, task ID) and exports
  each frame to Toast's form (`compiler/native_export.go`): pc, error pc,
  operand stack, and fork vector, covering the eval and move continuations,
  finally reasons, range-loop bounds and catch handlers above. A task with a frame that has no Toast form is kept
  whole in Barn's own frame form, with a warning that Toast could not load the
  checkpoint. A Barn-form frame writes its raw source, variables and stack, a
  tagged list marked `__barn_vm_frame_v2` in the temporary slot naming the
  program's origin and SHA-256 fingerprint, and the pc line `<ip> 0 <ip>`.
  On load Barn recompiles each frame's source; a fingerprint mismatch drops
  that task with an ERROR log and boot continues. Toast-form frames are
  imported through `compiler/native_import.go`; a builtin continuation other
  than eval or move still fails startup.
- **Raw source lines.** Because Barn-form frames carry raw source, a source
  line starting with `.` is written with one leading space, and Barn's readers
  end a program only at a line that is exactly `.`
  (`server/dot_source_line_checkpoint_restart.yaml`).

## 7. Current Barn IO and checkpoint behavior

`db/format` reads through `bufio.Reader` and writes through `bufio.Writer` while
building a full in-memory `Database` or consuming a full `db/store` snapshot.
It does not transcode or validate character encoding: database string bytes are
carried in Go strings and written back as bytes. Treat the durable format as
Latin-1-compatible byte data; do not apply Unicode normalization or implicit
UTF-8 conversion.

Barn writes each verb's stored code, not a fresh unparse. Toast unparses verb
programs at every checkpoint and spells float literals with `%.15g`
(`src/list.cc:461-463`), so in Toast a literal that needs more digits reloads
as its 15-digit value; Barn keeps the exact value.

A loaded recycled slot is counted by Barn's object allocator
(`store.Store.AddRecycledSlot`), so `max_object()` and `create()` match Toast
after a load. An anonymous object referenced only from `last_move` is a
reachability root, so the collector keeps it for the next checkpoint; a
dangling anonymous reference in `last_move` of any numbered object the dump
writes is written as `#-1`.

`db/format/checkpoint.go` writes `<path>.tmp`, syncs it, and publishes it as
`<path>.new` together with the `.waifids` WAIF identity sidecar through a
`<path>.new.publication` recovery journal; it never replaces `<path>`. An
ordinary periodic or `dump_database()` checkpoint pauses tasks only for a short
capture at one store timestamp; the object walk, serialization and publication
run while tasks continue. Shutdown and panic dumps keep tasks paused through
publication.

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
