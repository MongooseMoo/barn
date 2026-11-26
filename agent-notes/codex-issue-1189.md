# Barn #1189

## Goal

Fix pre-login HTTP input with a failing Go regression first, open the Barn PR, then open a separate moo-conformance-tests PR after the fix lands in Barn.

## State

The issue reports a suspended `read_http("request", player)` and zero response bytes from a real Mongoose listener. Barn `server/input_processor.go:174` requires both login and the `binary` option before it calls the binary transport reader. The text path removes HTTP line endings before `builtins/network.go:694-702` decodes and appends the input to the HTTP buffer. The installed stock Toast source uses `if (h->binary)` at `src/network.cc:547-549`.

The Go regression reached `read_http()` before login and timed out receiving the result on unchanged Barn. It returned `GET /` after the fix. The test asserts that the connection is anonymous before sending the socket request.

Managed stock Toast passed the new socket case and capability admission. The command reported `2 passed, 1 warning in 11.24s`. Evidence is in `/tmp/barn-1189-toast-regression.bYizrS` in Debian WSL.

Managed fixed Barn passed the same case and admission, `2 passed, 1 warning in 13.54s`. Evidence is in `/tmp/barn-1189-fixed.5HElAI`. The final Go regression passed ten runs.

## Decisions and why

The Go test uses the real `InputProcessor`, runtime, and TCP transport over `net.Pipe`. It waits for the wizard-owned login hook to suspend in `read_http()` before sending the request and checks the returned method and path.

`HandleConnection` now selects binary input from the `binary` option. The existing transport already handles byte framing and reconsiders the option when a blocked read wakes. The pre-login idle deadline still applies.

The conformance case uses `allocate_port`, `new_connection`, and `send_bytes`. The hook publishes its parsed result on the handler so the wizard control connection can verify the connection identity, method, URI, and Host header. Cleanup closes the client and removes the listener and handler.

## Surprises

The Go test waits for the login task to finish before closing the pipe. Closing immediately after the client reads the notification can race the server's write deadline reset.

## Next step

Open the Barn PR and follow required checks through the merge queue. Open the separate suite PR after the fix reaches Barn master.
