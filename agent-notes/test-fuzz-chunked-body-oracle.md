# test/fuzz-chunked-body-oracle

## Goal
Give `FuzzParseHTTPChunkedBody` (builtins/network_http_test.go) a real
correctness oracle: generated valid chunked encodings must decode to the
original body with correct consumed/complete status; every strict prefix must
report incomplete. Raw-bytes target stays as panic/termination coverage.

## State
Structured target `FuzzParseHTTPChunkedBodyValidEncoding` added in
builtins/network_http_test.go; seeds pass
(`go test ./builtins -run 'FuzzParseHTTPChunkedBody' -count=1`: both targets,
3 + 4 seeds PASS).
- Campaign `timeout 300 go test ./builtins -run '^$' -fuzz
  '^FuzzParseHTTPChunkedBodyValidEncoding$' -fuzztime=3m -parallel=2`:
  `fuzz: elapsed: 3m0s, execs: 962914 (0/sec), new interesting: 78 (total: 82)`
  then `PASS` / `ok github.com/MongooseMoo/barn/builtins 181.296s`. No
  decoding bug found.
- Oracle fires: temporary mutation in network.go after the chunk append,
  `if chunkSize >= 16 { body = body[:len(body)-1] }` (no seed has a chunk of
  16+ bytes). Seeds still passed; the fuzz run failed at once:
  `network_http_test.go:333: encoding "0010;ext=v\r\n0000000000000000\r\n000;last\r\n\r\n" decoded to "000000000000000", want "0000000000000000"`.
  The raw target under the same mutation: 30 s, 200237 execs, PASS (it cannot
  see wrong decoding). Mutation reverted, generated testdata file deleted.
- gofmt -l builtins: clean; go vet ./builtins: ok; go test ./builtins: ok.
- Committed locally on test/fuzz-chunked-body-oracle; not pushed.

## Contract (builtins/network.go on origin/master)
- `parseHTTPChunkedBody(data, start) (string, int, bool)` at network.go:403.
  Parsing begins at `start`; the int is an absolute offset into `data`
  (one past the final CRLF of the trailer section), not a length from `start`.
- Complete: returns `EncodeRawToBinary(concatenated chunk data)`, end offset,
  true (network.go:435).
- Incomplete and malformed both return `("", 0, false)` (network.go:410, 418,
  423, 431, 445, 450). There is no separate error result, so "incomplete, not
  error" cannot be distinguished at this layer; the oracle asserts only
  `complete == false` for strict prefixes.
- Size line: text before the first `;` is the size (extension ignored,
  network.go:413-415), `strings.TrimSpace`'d (416), parsed as hex int64 (421).
  Upper/lower hex and leading zeros accepted.
- Trailers: after the 0-size line, CRLF lines are skipped until an empty line
  (network.go:428-437). Trailer content is not validated or returned.
- No size limit beyond the overflow guard (network.go:443-446).
- Callers: parseHTTPRequest (network.go:486-492) and parseHTTPResponse
  (544-550) pass `header.bodyStart`, use the body only when complete and set
  `consumed = next`; collectHTTPWakeupsLocked (network.go:581-585) drops
  exactly `consumed` bytes from the held buffer, keeping later bytes for the
  next read. So consumed must stop at the end of the encoding even when more
  bytes follow.
- Toast (WSL /root/src/toaststunt/src/http_parser.c:1600-1698): chunk size is
  hex digits, then CR, or `;`/space starting ignored parameters until CR; data
  goes to on_body (tasks.cc:1548-1553, concatenated); size 0 enters trailer
  header parsing (F_TRAILING). So for valid encodings with `;ext` extensions and
  trailer fields, Toast's body is the chunk-data concatenation too.

## Decisions and why
- Separate structured target `FuzzParseHTTPChunkedBodyValidEncoding` rather
  than a mode byte on the raw target: keeps the raw target unchanged and the
  generator readable.
- Generator inputs: arbitrary prefix (start = len(prefix), checks the absolute
  offset contract), body, chunk-length bytes, arbitrary suffix after the
  encoding (pipelined bytes; consumed must ignore them), style bits (hex case,
  leading zeros, `;ext` extensions, trailer field). Only forms both Barn's
  contract and Toast's parser accept are generated (no leading whitespace in
  the size line, which Barn tolerates but Toast rejects).
- Body capped at 1024 bytes because the prefix check is quadratic.

## Surprises
- Divergence from source reading (not run on Toast): on a malformed chunk size
  Toast sets HPE_INVALID_CHUNK_SIZE and read_http returns a map with an
  `error` key (tasks.cc:1713-1724); Barn's parseHTTPChunkedBody returns
  incomplete for the same bytes, so parseHTTPRequest/Response return
  incomplete (network.go:488-489, 546-547) and read_http keeps waiting.
  Out of scope for this task; reported, not changed.

## Next step
Push and PR when the coordinator says so. Separately (needs Q): the
malformed-chunk divergence above should first be confirmed on the managed WSL
Toast oracle as a conformance test before any Barn change.
