# fix/http-read-syntax-error

## Goal
Find why Barn intermittently fails read_http conformance tests with "Line 1:  syntax error" and fix it in Barn.

## State
Fixed and committed on fix/http-read-syntax-error (not pushed). Cause: the per-connection input reader (server/input_processor.go) chose binary mode before any bytes arrived. A binary read already past Go's deadline check returns bytes that arrive after set_connection_option(player, "binary", 0) and its SetReadDeadline(now) wake, so the next harness statement was read binary-escaped (`...;~0A`) and failed to compile. Fix: the reader waits for bytes (BinaryTransport.WaitInput), re-reads the binary option, and reads text if binary went off. This matches Toast network.cc pull_input, which tests h->binary on bytes already read.

## Decisions and why
- Decide the mode after bytes are buffered, not before the read: it is Toast's order, and no deadline-based wake can stop a read that has already passed its deadline check.
- Kept the existing WakeInputReader deadline wake: it still moves a blocked reader to the new mode when no bytes come.
- Deterministic Go test (deadlinePassedConn ignores read deadlines) instead of the 200-iteration stress test used to find the cause; the stress test failed only at GOMAXPROCS>=2 and was deleted.

## Surprises
- The suspected double-hold in HandleHeldInput (builtins/network.go) is not reachable here: awaitingReadier (task/task.go:822) keeps a fork(0) child queued until the parent's slice ends.
- Line-mode reads have the mirror-image race (bytes sent after binary is turned on read as text). Nothing in the suite sends input while binary is on; not addressed.

## Next step
Push the branch, open the PR, watch CI.

## Findings log
- CI run 44565393650 attempt 1: only failure builtins/http.yaml::read_http_chopped_up_requests_work, ExecutionResult error_message "['Line 1:  syntax error']" (13702 passed).
- Harness (moo-conformance-tests src/moo_conformance/transport.py:444-460) flattens the statement to one line and sends "; <code>\n" on the player socket, then reads PREFIX/SUFFIX framed reply. So the syntax error is a compile of a line Barn treated as the eval command.
- Toast tasks.cc:1075-1125 enqueue_input_task: force_input lines go into the tqueue input queue (TASK_INBAND); with hold_input and no reader they wait. tasks.cc:1675-1760 run_ready_tasks: a reader with parsing set feeds each input line (binary_to_raw_bytes) into http_parser; reader without parsing gets the line; otherwise do_command_task. One input task at a time, strictly queued, single threaded.
- Barn reader loop server/input_processor.go:150-225: each iteration clears the read deadline (logged in), then checks the connection's "binary" option and reads with TCPTransport.ReadChunk (server/transport.go:229, all buffered bytes, binary-escaped, "\n" becomes "~0A") or line-mode ReadInput. set_connection_option("binary") (builtins/network.go:1400-1407) stores the option immediately (Session.setConnectionOption, network.go:253) then WakeInputReader (server/connection.go:248) sets read deadline to now so the reader reselects its mode.
- Working hypothesis: a "; ..." line read by ReadChunk ends in "~0A", which fails to compile with "Line 1:  syntax error". Need to find how the reader can still be in ReadChunk when the next harness line arrives. Pure deadline ordering analysis found no lost-wake window yet.
- WSL clones for reproduction: /root/http-flake/barn (origin/master df0a1546), /root/http-flake/suite (093c83b, submodules initialized).
- Strong candidate cause: builtins/network.go HandleHeldInput (636-723). With hold-input on and no read_http waiter registered yet, a forced line is appended to the HTTP buffer AND to heldCommands. If the forked force_input task runs before the parent reaches prepareHTTPRead (Barn runs fork(0) concurrently; Toast cannot, single thread), read_http later parses the buffer, but the same pieces stay in heldCommands; set_connection_option("hold-input", 0) (network.go:1408-1411) replays them as commands ("G", "E", ...). Toast holds a line once in the tqueue input queue; read_http dequeues it (tasks.cc:1675).
- Checking reachability: is a fork(0) child runnable before its parent's slice reaches read_http's prepareHTTPRead (builtins/network.go:1521)?
- engine/task_factory.go:307 SetReadier(parent) + task/task.go:822 awaitingReadier: a fork(0) child is not selected until the parent's slice stops executing, so the parent registers its read_http waiter (prepareHTTPRead) and marks itself suspended (SuspendSeconds(-1)) before the child can force_input. The double-hold path above needs that order broken; not yet shown reachable.
- Deadline-wake ordering in the reader loop (clear deadline, read option, read) looks correct for a single wake path.
- CI artifact conformance-results keeps only the last 10 server logs; none cover http.yaml.
- Barn reads input ahead (commit 670cf24d): the per-connection reader goroutine reads the next line while the previous command still runs; WakeInputReader + server/binary_input_transition_test.go cover the binary-off reselect. The flake shape (eval line compiled with a binary "~0A" tail or split) fits a read done in binary mode after the previous test turned binary off.
- Repro attempt 1: GOMAXPROCS=1, http.yaml x15 on WSL clone: 0 failures. Attempt 2 running: 40 busy loops + GOMAXPROCS=2, x30.
- REPRODUCED on managed harness (WSL clone, Barn df0a1546): 40 busy loops + GOMAXPROCS=2, `make conformance-k K=http.yaml` x30 -> 2 failing runs: read_http_bad_request_invalid_method and read_http_no_reason_phrase_response_works, both "Line 1:  syntax error". Each failing test follows another binary read_http test.
- REPRODUCED in a Go test: server/read_http_statement_stress_test.go (real TCP loopback, real InputProcessor reader, 200 chunked read_http statements on one connection). GOMAXPROCS=1 x10 passed; GOMAXPROCS=2 and 4 failed within 52-87 iterations with `{0, {"Parse error: syntax error"}}`.
- Debug print in the reader (temporary) on failing iterations: the statement line was returned by ReadChunk (len 586 vs 555, tail `...};~0A`) and binary was already false when the read returned. So the reader sat in a binary read across the binary-off; the next statement was escaped and failed to compile.
- Timestamped debug (Windows): failing iteration = `DEBUGCHUNK start`, `DEBUGSET binary 0`, `DEBUGSET woke`, `DEBUGCHUNK end <nil>` with the next statement as data. The wake (SetReadDeadline(now)) did not make the pending binary read fail before the next statement arrived. Checking whether Linux shows the same.
- Linux reproduces the same (WSL clone, GOMAXPROCS=4, failed at iteration 51): CHUNK start 632694, binary 0 632916, woke 632922, CHUNK end <nil> 633060 with the next statement. Cause: a read already past Go's deadline check (prepareRead) returns bytes that arrive after SetReadDeadline(now); the wake only stops reads that would block. The mode is chosen before the bytes exist, so bytes sent after binary-off can be consumed by the binary read.
- Toast network.cc:544-549 (pull_input): read() the bytes first, then test h->binary on the bytes in hand. Barn decides before the bytes exist.
- Fix (server/transport.go, server/input_processor.go): BinaryTransport gains WaitInput (Peek(1), no consume); the reader waits for bytes, re-reads the binary option, and `continue`s to text mode if binary went off, so bytes are decoded in the mode current when they are in hand (Toast's order). ReadChunk then consumes only bytes buffered before that check.
- Replacing the probabilistic stress test with a deterministic one: a conn whose SetReadDeadline is a no-op stands in for a read already past its deadline check.
- TestStatementAfterBinaryOffIsTextWhenBinaryReadReturnsIt (server/binary_input_transition_test.go): without the fix 3/3 fail with `{0, {"Parse error: syntax error"}}`; with the fix 20/20 pass (with TestDisablingBinaryModeWakesBlockedReader).
- Server package tests pass (go test ./server, 21s); gofmt and go vet clean on server and builtins. Managed-harness confirmation (http.yaml x30, 40 busy loops, GOMAXPROCS=2, WSL clone with the fix) running.
- Managed-harness confirmation with the fix was started before commit (WSL /root/http-flake, http.yaml x30 under load, output in the orchestrating session's background task b87bk4t3z). Result not recorded here; unverified until it is read.
