# TCP input reader fuzzing

## Goal
Add FuzzTCPReadInput for arbitrary pre-login TCP bytes and chunk boundaries,
and register it in the nightly fuzz matrix (approved plan item C2).

## State
Implemented the target and nightly matrix entry. Focused transport tests,
go vet, staticcheck, and golangci-lint on ./server passed. gofmt -d produced
no diff. The cap close-path test through HandleConnection also passed.
The first four-minute, two-worker WSL fuzz campaign passed (472085 executions).
The final-version campaign passed (989344 executions, 240.151s). No failing
input or production defect was found in either bounded campaign.

Local commands and results:
- `gofmt -d server/transport_fuzz_test.go`: no diff.
- `go test ./server -run '^(TestTCPReadInputFuzzCap|FuzzTCPReadInput|TestReadInput|TestReadLine)' -count=1`: exit 0, package result 2.643s.
- `go vet ./server`: exit 0.
- `staticcheck ./server`: exit 0.
- `golangci-lint run ./server`: 0 issues, exit 0.
- WSL Debian in this worktree: `timeout --kill-after=5s 300s go test ./server -run '^$' -fuzz '^FuzzTCPReadInput$' -fuzztime=4m -parallel=2`: PASS, exit 0.
- `actionlint .github/workflows/fuzz.yml`: exit 0.
- `git diff --check`: exit 0.

## Decisions and why
Use NewTCPTransport with the existing fakeConn/newTestTransport helpers.
Override only the underlying connection Read size, using the second fuzz
argument as a repeating schedule of positive sizes. Compare ordered records
(including OOB flags) and terminal errors with contiguous reads. Check CR/LF
exclusion, buffer bounds, a byte-derived call budget, and a ten-second watchdog
for a CPU spin within ReadInput.

Keep multi-megabyte cap boundaries in a focused deterministic test rather than
the mutation corpus: line and unfinished subnegotiation, below/at/above the
cap, EOF/newline, contiguous/chunked reads. Assert the existing error contract;
InputProcessor.HandleConnection closes on return (input_processor.go:107-118,
179-199); a focused test verifies Close is called for both line and telnet cap
errors before login. No production code changes. Keep the PR independent against master;
publish and watch CI, but do not merge.

## Surprises
Toast network.cc:386-467 processes telnet one byte at a time with persistent
state; pull_input:549-572 groups OOB commands per socket read. Barn returns
individual completed OOB commands (transport.go:177-227), independently of
chunking. This target checks Barn's own chunk invariance, not identical Toast
OOB grouping. Toast binary input bypasses the telnet state machine; Barn's
logged-in binary option selects ReadChunk (input_processor.go:165-170), so
BINARY telnet negotiation is seeded as OOB bytes without toggling ReadInput.

Read Toast's MAX_LINE_BYTES definition (include/options.h:247), command check
(network.cc:395), line check (network.cc:476), and merged PR #568. The existing
cap conformance YAML is absent from the local sibling checkout; no new live
Toast acceptance claim is made. Reader cap errors and Barn's connection-close
path are covered locally with in-memory connections.

Initial compilation failed because a diagnostic used %q for a slice of
structs. The existing vet check blocked it; changed the diagnostic to %v and
reran the tests and package checks successfully.

## Next step
Publish the independently based PR against master and watch its checks to
completion. CI status is held by the PR checks; this note records local
verification. Leave the PR unmerged for the parent task's review.
