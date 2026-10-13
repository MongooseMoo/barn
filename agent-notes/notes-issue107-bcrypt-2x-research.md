# Issue 107 bcrypt 2x implementation research checkpoint

- Scope: read-only implementation research; no Barn implementation changes.
- Current Barn: `builtins/crypto.go` recognizes `$2a$`, `$2b$`, and `$2y$`; `cryptBcrypt` computes with `github.com/go-crypt/x/bcrypt` and relabels the returned `$2a$` prefix for 2b/2y. `$2x$` is not recognized.
- Current dependencies: `github.com/go-crypt/x v0.4.12`, `golang.org/x/crypto v0.46.0`, and `github.com/digitive/crypt v0.2.0`.
- Current Toast: `src/crypto.cc` implements `bf_crypt` and calls system `crypt(password, salt)`; Openwall compatibility source exists at `dependencies/crypt/crypt_blowfish.c`.
- Working tree was already heavily dirty/untracked; no existing material has been altered.
- Blocker: none yet. Exact Openwall flags, parsing, password semantics, output, licensing, and candidate dependency behavior still need verification.
- Next action: inspect current Toast/Openwall source and current Go dependency source, then verify primary upstream licensing/documentation and synthesize a narrow file/API/test plan.

## Evidence update

- Toast `parse_prefix` accepts crypt variants 2a/2x/2y, parses a numeric cost of variable width in range 4..31, and performs the programmer strength check before `_crypt_blowfish_rn` enforces exactly two digits. This yields E_INVARG for wizard and E_PERM for programmer on the conformance one-digit 4 and three-digit 004 inputs.
- Openwall `flags_by_subtype` maps 2a to flag 2, 2x to flag 1, and 2y to flag 4. `BF_set_key` uses flag bit 0 to select the historical signed-char accumulation and flag bit 1 for the 2a anti-collision safety toggle. Therefore 2x is neither corrected bcrypt nor a prefix relabel.
- Openwall validates only 2a/2b/2x/2y via its nonzero flag table, requires exactly two decimal cost digits in 00..31 with the public entrypoint minimum effectively 04, decodes exactly 16 salt bytes from 22 bcrypt-base64 characters, emits the requested 29-character setting plus 31 hash characters, and encodes 23 of 24 final ciphertext bytes.
- Openwall password input is a C string. The key setup consumes at most 72 expansion bytes, cycles through the password including its terminating NUL, and therefore treats an embedded NUL as the end of the password and ignores bytes after byte 72.
- The current conformance source identifies the eight 2x cases: ASCII body equals 2y; raw byte 0x80 body differs; exact 04 wizard success and programmer E_PERM; one-digit 4 wizard E_INVARG/programmer E_PERM; three-digit 004 wizard E_INVARG/programmer E_PERM.
- Openwall source declares public domain with a permissive fallback allowing source and binary redistribution with or without modification and no warranty.
- Next action: inspect the Go dependency internals and upstream project/license/maintenance sources, derive exact known-answer vectors, and choose between dependency, narrow port, and x/crypto adaptation.

## Candidate and design update

- Reject `github.com/go-crypt/crypt/algorithm/bcrypt`: its decoder recognizes identifier `2x`, but `NewVariant` collapses 2a/2b/2x/2y to one standard variant, matching calls `github.com/go-crypt/x/bcrypt.Key` on the original password, and encoding uses identifier 2b. It does not compute the signed-char schedule.
- Reject `github.com/grmrgecko/go-passwd` as a production dependency despite its real bug flag: it is a low-adoption 2024 project (one star, no forks), added bcrypt in February 2026, has no 2x known-answer test, does not stop at embedded NUL like Toast C strings, and appears to reproduce substantial Go/Openwall core code under only its own MIT notice. Its maintenance and provenance risk outweigh convenience.
- `github.com/GehirnInc/crypt`, `tredoe/osutil`, the existing `github.com/digitive/crypt`, and upstream `golang.org/x/crypto/bcrypt` do not implement genuine 2x.
- Safest narrow path: adapt the existing `github.com/go-crypt/x/bcrypt` engine by synthesizing the exact 72-byte corrected key stream whose 18 big-endian words equal Openwall `BF_set_key`'s signed-char words. Passing those 72 bytes to `GenerateFromPasswordSalt` is exact: that dependency appends a NUL at index 72, but its initial and repeated Blowfish expansions each consume exactly bytes 0..71, so the appended byte is never read. Its salted setup, cost loop, 64 final encryptions, 23-byte truncation, bcrypt base64, and canonical salt/output formatting then remain vetted existing code.
- Adapter construction: treat the Toast password as a C string (truncate at the first NUL), append one terminator, cycle that sequence while generating 18 words with `word = word<<8 | uint32(int32(int8(byte)))`, and write each word big-endian into a 72-byte slice. Passwords of at least 72 non-NUL bytes naturally use only the first 72 bytes.
- For 2x, keep Toast's two validation stages distinct: outer variable-width numeric/range parse and programmer permission check first; then an exact setting parser requires two digits plus `$` and a valid 22-character bcrypt salt before hashing. This preserves E_PERM for programmer inputs 4/004 with cost value 4, while wizard reaches the exact-width rejection and gets E_INVARG.
- Scope recommendation: add a 2x-specific exact parser/hash helper so issue 107 does not absorb the separately owned 2a/2y/2b changes. `salt()` remains untouched and therefore continues rejecting 2x.
- Next action: obtain Openwall primary known-answer vectors and confirm canonical salt/output and repository license/provenance details, then deliver the file/API/test plan.

## Final research state

- Toast's `crypt_blowfish.c` blob is byte-for-byte identical to Openwall crypt_blowfish 1.3 (`9d3f3be829b330efa9eb9454c539b07d09c8b43a`; upstream tag commit `3354bb81eea489e972b0a7c63231514ab34f73a0`). This is the recommended semantic provenance.
- Openwall `wrapper.c` provides authoritative cost-05 2x vectors, including password byte A3 -> `$2x$05$/OK.fbVrR/bpIqNJ5ianF.CE5elHaaO4EbggVDjb8P19RukzXSM3e`, FF FF A3 producing the same legacy collision, and D0 C1 D2 CF CC D8 -> `$2x$05$6bNw2HLQYeqHYyBfLMsv/O9LIGgn8OMzuDoHfof8AQimSGfcSWxnS`.
- Openwall documents a 72-byte maximum, 8-bit password characters, 128-bit salt, 184-bit encoded hash, 2^cost work, 2x as the fully reintroduced bug, and intentional refusal to generate 2x salts. Output is exactly 60 bytes: `$2x$`, two cost digits, `$`, 22 canonical bcrypt-base64 salt characters, and 31 bcrypt-base64 characters encoding only the first 23 final ciphertext bytes.
- Current Barn MOO strings preserve raw high bytes; `chr(128)` is one raw byte. `chr(0)` contributes nothing because NUL cannot live in a MOO string, but the internal helper should still truncate at the first NUL to match the C-string oracle. Password bytes after the first 72 expansion bytes are ignored.
- Recommended files/API: small 2x dispatch in `builtins/crypto.go`; new `builtins/bcrypt_2x.go` with unexported `parseToastBcrypt2xCount`, `cryptBcrypt2x`, and `openwall2xSchedule`; new `builtins/bcrypt_2x_test.go`. Do not alter `builtinSalt`, `go.mod`, or `go.sum`.
- Tests: authoritative Openwall high-byte KATs; ASCII exact/2y-body equality; high-byte 2x/2y-body inequality; all six exact/one/three-digit wizard/programmer outcomes; 60-byte prefix/output; invalid salt alphabet/length; stored-hash suffix acceptance; NUL truncation and 72-byte truncation; explicit `salt("$2x$", ...)` rejection.
- License: Openwall is public domain with a permissive redistribution fallback; go-crypt/x and x/crypto are BSD-3-Clause. Barn has no detected repository license, so dependency/source terms are permissive but overall-project licensing remains unspecified. Prefer API use plus retained Openwall provenance comments over copying either implementation.
- Security/maintenance: 2x is collision-prone by design and must remain compatibility-only; keep salt generation rejected and the existing cost permission gate. The schedule adapter avoids a new dependency, CGo, copied Blowfish constants, and loss of Openwall's C runtime self-test. Pin exact KATs so any future bcrypt dependency change fails closed.
- Blocker: none. Research complete; no Barn implementation or test source was modified.
