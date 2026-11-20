# fix/reloaded-fork-debug-flag

## Goal

Make Barn agree with Toast on the debug flag of a queued fork across a
checkpoint restart: (a) the activation-line field Barn writes for a queued fork
equals what Toast writes for the same state; (b) a queued fork Barn loads runs
as Toast would run it from the same file; (c) live forks keep taking the flag at
the fork statement. Check suspended activations on the same path.

## What Toast writes and reads (verified with bytes)

Source: pinned WSL oracle checkout `/root/src/toaststunt/src/execute.cc`.

- `write_activ_as_pi` (execute.cc:3859):
  `dbio_printf("%PRIdN -7 -8 %PRIdN -9 %PRIdN %PRIdN -10 %d\n", a.recv, a.player, a.progr, a.vloc, a.debug)`.
  `a.vloc` is a `Var` (structures.h:164: 8-byte value union, then `type`), 16
  bytes, passed by value to a single `%PRIdN`. On x86-64 it takes two argument
  registers. Disassembly of the oracle binary `build-release/moo`
  (`write_activ_as_pi` at 0x60de0): `rsi`=recv, `rdx`=player, `rcx`=progr,
  `r8`=vloc value, `r9`=vloc type, and `a.debug` is pushed on the stack. The
  fourth `%PRIdN` prints `r8`; the final `%d` prints `r9`, the `Var`'s `type`.
  `a.debug` is never printed.
- So the last field is the type of the verb's definer value: `TYPE_OBJ` = 1
  for an ordinary object. For an anonymous definer it would be `TYPE_ANON` =
  12 | 0x80 = 140 (source reading only, no file captured).
- `read_activ_as_pi` (execute.cc:3892-3894) scans that last field into
  `a->debug`. `tasks.cc:2054` reads it for a queued fork; `enqueue_forked`
  keeps the activation and `do_forked_task` (execute.cc:3407) runs with
  `RUN_ACTIV = a`. Suspended activations use the same pair
  (`write_activ` execute.cc:4019, `read_activ` execute.cc:4091).
- Captured checkpoints (stock oracle, managed harness; the restart command was
  a script that copied the kept database and then exec'd Toast on it; copies
  in WSL `/root/fork-debug-capture/`):
  - fork from a `d` verb (verb flags line `172`):
    activation line `9 -7 -8 8 -9 8 9 -10 1`, file SHA-256
    `40025d3fcdd6659dc49190ed2680986a6f47a6a63f1cf4dd9f4e2e6ea0446e69`
  - fork from a verb without `d` (verb flags line `164`):
    activation line `9 -7 -8 8 -9 8 9 -10 1`, file SHA-256
    `c27c2011ee9a3db01b6f7ce7ff4a2f5380215784edc156ee501e0a3e6b7f9390`
  - Toast run: `3 passed, 6 deselected in 19.42s`.
- Every Toast-written fixture in this repo (`engine/testdata/toast-*.db`,
  `db/format/testdata/toastcore.db`) has `1` in that field on every activation
  line, forked and suspended.

Toast defect, exact statement: `write_activ_as_pi` passes the 16-byte `Var`
`a.vloc` to one integer conversion, so the checkpoint's activation line ends in
the definer's type code instead of the activation's debug flag; every
activation Toast loads from a Toast-written checkpoint therefore has debug on,
whatever its verb's `d` flag was.

## State

Done, committed on `fix/reloaded-fork-debug-flag`, not pushed.

- Before: the readers already took the field as the debug flag
  (`db/format/reader_task.go` for queued forks, `db/format/reader_vm.go` for
  suspended frames, where it overrides Barn's own metadata flag). The writers
  put the verb's flag there (`writer_task.go` from `Fork.Debug`,
  `writer_vm.go` from `frame.VerbDebug`), so a fork or suspended frame of a
  verb without `d` was written with 0 where Toast writes 1.
- Change: both writers write `activationDefinerType` (`TypeObj`, 1), the type
  code of the definer value they write. `task.ForkSnapshot.Debug` had no
  reader left and is removed. Loading is unchanged. Live forks are unchanged
  (`vm/control.go` still copies the forking frame's flag).
- Go tests, red before the writer change and green after:
  - `engine.TestQueuedForkCheckpointsDefinerTypeWhereToastReadsDebug`
    (replaces PR 945's `TestQueuedForkCheckpointsItsVerbDebugFlag`): the live
    fork holds its verb's flag; the file line ends in 1 for a d verb and a
    verb without d; a file with 1 loads and runs debug on, a file with 0 debug
    off. Red: subtest `verb_without_d` failed, `d_verb` passed.
  - `format.TestWriteSuspendedActivationLineEndsInDefinerTypeCode`: red with
    `7 -7 -8 2 -9 3 6 -10 0` for `VerbDebug` false.
  - `format.TestWriteQueuedTasksUsesTaskSnapshots` expectation
    changed from `-10 0` to `-10 1`.
  - `go test ./db/format ./task ./types ./vm ./engine -count=1`: all ok.
    `go vet ./...` clean, gofmt clean.
- Managed harness on Barn (WSL, `make conformance`, suite checkout
  `moo-conformance-tests-fork-debug`, `fork_debug_flag.yaml`, K=fork_debug_flag
  plus admission): `9 passed in 31.65s`. Barn binary SHA-256
  `dce5b85f741ad9a8d0e5be013fb61401f7c6b483dbfdf690d899a289ed980901`.
- Cross-engine (`scripts/cross-engine-checkpoint.sh` from
  `origin/ci/cross-engine-checkpoint`, scratch copy narrowed to
  `fork_debug_flag.yaml` and `fork_line_numbers_checkpoint_restart.yaml`, the
  two Barn-local checkpoint sessions removed):
  - Barn writes, Toast resumes: `6 passed, 4 deselected in 37.39s`
  - Toast writes, Barn resumes: `6 passed, 4 deselected in 37.78s`

## Decisions and why

- Write the definer's type code, not the flag: that is what Toast's bytes
  hold. Barn writes the definer as an object in both writers, so it is 1.
- Keep loading the field as the debug flag: Toast does, and a Barn file from
  before this change (0 there) runs as Toast runs it.
- Suspended frames fixed on the same path: same Toast function, same field.

## Surprises

- The harness removes its server directory; a restart command that is a small
  script (copy `{db}`, then exec Toast) keeps the checkpoint without launching
  anything by hand.
- `/mnt/c` worked; no copies under the Linux home were needed.
- Git inside WSL cannot open this Windows-made worktree, so the WSL build
  needs `GOFLAGS=-buildvcs=false`.

## Unverified

- A verb defined on an anonymous object: Toast would write 140 by source
  reading; no file captured. Barn writes the definer as an object and 1.
- No conformance test resumes a suspended task of a verb without `d` after a
  restart. The suspended-frame change rests on Toast's source, the
  disassembly, and the Toast-written fixtures; it was not run on the oracle.
- The cross-engine script's Barn-local suspended checkpoint sessions
  (`tests/mongoose-conformance/server/*checkpoint*.yaml`) were not run with
  this change; only the two fork files were.
- `go test ./...` was not run in full locally; CI runs it.

## Next step

Team lead pushes the branch and opens the PR.
