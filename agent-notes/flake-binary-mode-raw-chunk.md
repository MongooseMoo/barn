# binary mode raw chunk flake

## Goal
Find and fix the cause of the intermittent failure of
`audit/gap_followups_toast_oracle.yaml::audit_binary_mode_dispatches_raw_chunk_without_newline`
in Barn CI (merge-queue run 44941866885, job 136964223627, runner Q-barn-wsl-x64-08).

## Evidence
- CI failure: the final poll step (`for attempt in [1..100] ... suspend(0.01)`)
  returned `''` instead of `'auditbin'`; 1 failed, 13855 passed.
- JUnit time for the case: 1.741 s. The harness `send_bytes` step
  (moo_conformance/transport.py:160) waits 0.5 s of silence
  (QUIET_BEFORE_DATA) after sending, then the poll runs >= 1.0 s. So Barn had
  not run `do_command` for "auditbin" ~1.5 s after the bytes were sent, while
  the wizard connection's evals were served at normal speed. That points at
  input stuck/lost, not general slowness.

## Code read
- `server/input_processor.go:146-235` reader loop: chooses binary
  (`WaitInput`/`ReadChunk`) only when `conn.IsLoggedIn()` and the `binary`
  option are both true at the top of an iteration; otherwise line mode
  `ReadInput`, which buffers "auditbin" (no CR/LF) forever.
- `builtins/network.go:1400-1407` set_connection_option writes the option
  immediately (`Session.setConnectionOption`, network.go:253) then calls
  `WakeInputReader` (server/connection.go:248), which sets a TCP read
  deadline of now.
- Login is asynchronous: the initial "" event (input_processor.go:135) is
  dispatched to `dispatchLoginCommand` (input_processor.go:680), which starts a
  login task; `loginPlayer` runs from its onComplete. The reader therefore
  enters its loop before login and blocks in line-mode ReadInput with the
  connect-timeout deadline.
- `server/input_login.go:253` SetPlayer and playerConns update under cm.mu together.
- `server/transport.go:231-251` WaitInput = bufio Peek(1); ReadChunk drains buffered bytes.

## Reproduction setup
- Barn: `GOOS=linux go build -o bin/barn-linux ./cmd/barn` from this worktree (origin/master a41f6088).
- Suite: worktree `C:/Users/Q/code/mct-flake-binary-chunk` (origin/main 54009bc),
  rsynced to `/root/mct-binchunk` in WSL (full collection on /mnt/c takes 8 min).
- Loop script: `.flake-bin/binloop.sh toast|barn N` (untracked), runs the managed
  harness with selector `_tests/audit/gap_followups_toast_oracle.yaml` and
  `-k "capability_admission or (audit_binary_mode_dispatches_raw_chunk_without_newline)"`.
- WSL load average during runs: 12-19 on 8 CPUs (CI runners busy), no added load.

## Runs
- Toast, full collection, 1 run: 2 passed (487 s, mostly collection on /mnt/c).
- Toast, narrowed, 1 run: 2 passed.
- Toast, narrowed, 100 runs: 100 pass, 0 fail (load 10.8-15.7).
- Barn master a41f6088, narrowed, 200 runs: 199 pass, 1 fail (run 167:
  `expected value 'auditbin', but got ''`, same as CI; load ~21).
- Now running a temporary debug build (DBGFLAKE slog lines in the reader loop,
  WakeInputReader and processInput; not committed) to see the event order
  in a failing run.

- Debug build, 400-run loop: failures at run 23 and (with buffer logging) run 12.
  Event order for the binary connection (conn 8) in the failing run:
  1. `top in=true bin=false buf line="" buffered=0` -> blocks in text `ReadInput`.
  2. `wake` (set_connection_option binary=1 sets the read deadline to now).
  3. `read err=i/o timeout buf line="auditbin" buffered=0` -- the text read
     returned the 8 bytes before the timeout and put them in `lineBuf`.
  4. `top in=true bin=true buf line="auditbin"` -> binary `WaitInput`
     (`Peek(1)`) waits for NEW socket bytes; "auditbin" sits in `lineBuf`
     until the cleanup closes the connection 1.7 s later. `do_command` never runs.

## Findings
### The race (Barn bug, Barn diverges from Toast)
Toast picks text or binary per `read()` call: `pull_input` (toaststunt
src/network.cc:546-565) reads the socket, then tests `h->binary` on those
bytes. Bytes read after `set_connection_option(p, "binary", 1)` are binary.

Barn's text read can consume bytes sent after binary was turned on:
- server/input_processor.go:174 picks text mode (binary off) and calls
  `TCPTransport.ReadInput` (server/transport.go:108), which loops
  `reader.ReadByte()`; each refill is a blocking socket read.
- set_connection_option (builtins/network.go:1400-1406) sets the option, then
  `WakeInputReader` sets a read deadline of now (server/connection.go:256).
- The harness gets the eval result and sends "auditbin". Under CPU load the
  reader goroutine is woken by the deadline but not yet run when the bytes
  arrive; Go's poller then reports the fd readable and the pending `Read`
  returns the bytes instead of the timeout. ReadInput appends them to
  `lineBuf` (no CR/LF, so no line), and the next refill returns the timeout.
- server/input_processor.go:180 then waits in binary mode for new socket
  bytes; the binary path drains `lineBuf` only after a new read, so the bytes
  are never dispatched.

This is the text->binary mirror of fa00a946 (binary->text), which fixed the
binary path by waiting for bytes and rechecking the mode before consuming.

### Fix (Barn, branch fix/flake-binary-mode-raw-chunk)
- `TCPTransport.ReadBufferedInput` (server/transport.go): text interpretation
  of only the bytes already buffered; with none buffered it waits
  (`Peek(1)`) and returns complete=false without interpreting, keeping
  ReadInput's EOF partial-line rule. ReadInput is unchanged (shares `readInput`).
- server/input_processor.go: for a BinaryTransport in text mode the reader
  calls ReadBufferedInput and loops back to choose the mode again when it
  returns no complete input. Each socket read is therefore interpreted in the
  mode current after the bytes arrived, as in Toast.
- Red test first: `TestChunkAfterBinaryOnIsBinaryWhenTextReadReturnsIt`
  (server/binary_input_transition_test.go), using the existing
  `deadlinePassedConn` model of a read that returns bytes despite the wake
  deadline. Before the fix: FAIL "chunk sent after binary on was not
  dispatched". After: pass, 20/20 with the other two transition tests.
- `go test ./server` and `go test -race ./server`: ok.
- No conformance change: the YAML asserts Toast behaviour Barn violated.
- staticcheck ./server and golangci-lint ./server/...: clean.

### Runs after the fix
- Fixed Barn, narrowed case, 400 runs: 400 pass, 0 fail (load 23-34 on 8 CPUs).
- Fixed Barn, full managed suite once (selector `_tests`, `-k test_`):
  13854 passed, 327 skipped, 0 failed.

## State
Committed locally on fix/flake-binary-mode-raw-chunk; not pushed (the lead publishes).
`.flake-bin/binloop.sh` (untracked) is the loop script used for the runs.

## Next step
Push and open the PR against master.
