# Binary input stream analyst

## Scope and state

Source-only analysis of the actual uncommitted binary-input and canonical-codec
change against this worktree's task HEAD. No implementation fixes, tests,
builds, benchmarks or publication by this analyst. The only Go invocation was
go env GOROOT to resolve the installed standard-library source for inspection.

The advertised legacy gauntlet role path is absent. The installed protocol
entrypoint was resolved from its existing skill root.

## Findings

No actionable correctness defect found in this selected source delta. This is
not a merge approval or performance clearance. The following owners, migrated
producers and regression boundaries were inspected.

### Actual producer and read boundary

The production ReadChunk caller is InputProcessor.HandleConnection's logged-in
binary-mode branch (server/input_processor.go:168-175). The same connection
reader goroutine selects ReadChunk, ReadInput or ReadLine, then enqueues an
immutable string (203-209); it does not launch another transport reader.
Repository-wide Go search found no UnreadByte/UnreadRune consumer. WakeInputReader
sets a deadline for TCP; it does not consume the buffered reader.

ReadChunk (server/transport.go:229-244) first Peek(1)s and returns initial errors
before mutating lineBuf. Success guarantees Buffered is positive. Peek(n) asks
for exactly that already-buffered amount, so it does not request another fill.
Encoding reads that borrowed byte slice before Discard; no slice escapes the
call. Installed Go bufio source was inspected directly: Peek only fills while
available bytes are less than requested, and Discard documents/implements
success without another read for n <= Buffered. Discard(n) is within the captured buffered count. Its error branch is
checked, but under the existing single-reader ownership it cannot need another
underlying read. No added wait for an arbitrary chunk size occurs.

lineBuf is the existing partial text owner, populated by ReadInput. Only the
new byte slice is encoded before appending, so a retained literal text tilde is
not re-escaped. String then Reset returns an immutable view while resetting the
builder to zero; subsequent reads allocate/use a new backing buffer rather than
mutating the returned string. The builder is never copied. Initial-error paths
retain the previous prefix. CR/telnet parser flags and text limits are unchanged.

### Canonical encoding and direct caller migration

AppendRawToBinary (builtins/crypto.go:152-156) loops over the original private
encodeByte (159-167). Its predicate, literal ~7E, hexadecimal formatting and
printable-byte path are unchanged. EncodeRawToBinary (172-176) has the real
fresh-string producer contract; the append API also has real existing-builder
producers in transport and mixed-value encode_binary. Integer validation and
recursive-list/quota behavior remain in encode_binary before/after those calls.
encodeAllBinaryStr retains its distinct all-bytes-escaped hash/HMAC contract.
No old encodeBinaryBytes/encodeBinaryStr Go owner or compatibility alias remains.

Every changed producer uses the same raw byte range and conversion placement:
file_read/readline/readlines (fileio.go:318,404,455), exec stdout/stderr after
the existing CRLF normalization (system.go:435-436), Unicode output before its
existing encoded-length check (unicode.go:244-246), random_bytes before its
existing encoded-length quota check (crypto.go:1438-1442), and base64 decode
after decoder error handling (96). HTTP folded/normal values, chunked body and
request/response bodies keep their previous ranges and parser completion
decisions (network.go:357,368,416,478,536). These are naming/owner migrations;
the original duplicated string encoder implements the same byte rule.

### Input admission, failure cases and immutable data

The encoded string is charged at its actual len when HandleConnection acquires
the existing per-connection input budget (input_processor.go:202-209). The budget
already permits one decoded oversized message when empty and waits only on that
connection's contexts/processed-input signal (input_buffer.go:23-47). Encoding
expansion therefore does not create a newly impossible admission size. No global
queue, shared reader lock or additional goroutine is introduced.

New tests construct the retained text via actual ReadInput returning a completed
IAC command, then assert the active prefix before calling ReadChunk. They do not
install a private builder fixture and assume it was reached. The ASCII and
literal-tilde cases check reset after success. Initial EOF checks the error and
then obtains the preserved text through ReadInput. One-byte underlying reads
bound the loop by source byte count and concatenate the actual returned chunks.
All-byte literal expected output covers every value 0-255 without copying the
production predicate; empty, ordinary and mixed strings and a nonempty existing
builder prefix are also checked (binary_input_stream_test.go:14-143).

No input byte slice is stored in a returned string. The original byte helper
copies into builder-owned storage. Existing mutable file/HTTP input buffers may
therefore be reused after encoding just as before. New strings are not mutated
after Builder.String because Reset clears the builder's backing reference, as
confirmed in the installed strings.Builder source. No unsafe conversion was
introduced by this change.

### Inspected canonical witness and current verification limits

Inspected the existing transition probe record and its actual raw XML files in
the checkpoint checkout. Observed XML cases were:

```text
toast-binary-verified-transitions.xml:
PASS capability_admission
PASS binary_mode_preserves_partial_text_input
PASS binary_mode_encodes_external_bytes
PASS binary_mode_preserves_literal_tilde_prefix

barn-binary-verified-transitions.xml (pre-correction):
PASS capability_admission
FAIL binary_mode_preserves_partial_text_input
FAIL binary_mode_encodes_external_bytes
FAIL binary_mode_preserves_literal_tilde_prefix
```

These are historical admitted contract/RED witnesses, not results from the
current corrected binary. The coder's focused six-test GREEN is recorded as
server 1.482s; this analyst did not rerun it. Parent's fresh managed candidate
run and full exact-head PR CI remain outstanding. The separate failed Toast CR
observer earns no parser-state change; this report makes no claim about all
possible CR/half-IAC/EOF mode transitions or protocol-limit combinations.

## Evidence limits

Recorded Windows timing costs remain unresolved; normalized codec instruction
equivalence is not a performance-parity verdict. Parent owns fresh managed
verification. CR/telnet state changes are excluded from this correction.

## Remaining

Parent completes fresh managed verification, current-default integration and
required PR checks/final verifier. Integration can touch the independently
changed HTTP test signatures; retain the physical IDs and canonical codec names
if that conflict occurs. No tests, builds, benchmark or production edits were
performed here. Only this report was written; no merge approval is issued.
