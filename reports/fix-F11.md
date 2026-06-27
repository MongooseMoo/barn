# Fix F11 — crypt() rounds silently capped + non-standard SHA-crypt

## The finding
Barn's `crypt()` for `$5$` (sha256), `$6$` (sha512) and `$1$` (md5) did not
implement the standard glibc algorithms:

- `cryptSHA256`/`cryptSHA512` (`builtins/crypto.go`) silently **capped rounds at
  1000** (`actualRounds > 1000 { actualRounds = 1000 }`) while emitting a prefix
  that advertised the requested round count, and used a naive
  `H(pw||salt) then loop H(H())` construction unrelated to the SHA-crypt spec.
- `cryptMD5` was naive `base64(md5(pw||salt))`, not Poul-Henning Kamp md5crypt.

Result: hashes ignored the requested rounds and were **incompatible with any
real password database** produced by glibc/Toast.

Red test (was FAILING, now passes):
`builtins/review_io_test.go:210 TestReview_IO_CryptSHA256RoundsSilentlyCapped`.

## What ToastStunt does (authority)
`toaststunt/src/crypto.cc`:
- `bf_crypt` (line 309). For BCRYPT it calls `_crypt_blowfish_rn` (line 361);
  for **everything else** (`$1$`/`$5$`/`$6$`) it delegates to the **system
  `crypt(string, salt)`** (line 373) — i.e. glibc `crypt(3)`, the STANDARD
  md5crypt / sha256crypt / sha512crypt.
- `parse_prefix` (line 87) parses `rounds=` and enforces the spec range
  **1000..999999999** for SHA-crypt (lines 119-124) and cost 4..31 for bcrypt
  (lines 147-152). There is **no 1000 cap**.
- Non-wizards may not specify non-default strength (line 349-355): any nonzero
  `rounds=` for `$1/$5/$6` raises `E_PERM` — Barn already enforces this in
  `cryptPasswordWithPerm`, left intact.

glibc's SHA-crypt is Ulrich Drepper's spec (http://www.akkadia.org/drepper/
SHA-crypt.txt); md5crypt is PHK's FreeBSD algorithm.

## What I implemented (`builtins/crypto.go`)
Replaced the three broken functions with spec-correct implementations, ported
from the well-known pure-Go reference `github.com/GehirnInc/crypt` (BSD), which
is itself verified against the published glibc/Drepper vectors. No new module
dependency was added (GehirnInc is not in `go.sum`; adding it risked an offline
sumdb fetch). The algorithm is vendored as a small spec-faithful implementation:

- `parseCryptSalt` — parses `$id$[rounds=N$]salt`, honors `rounds=` with
  **clamping only to 1000..999999999** (default 5000), terminates salt at the
  next `$`, truncates salt to 16 (sha) / 8 (md5) chars.
- `shaCryptDigest` — the full Drepper key-derivation (sumA/sumB/seqP/seqS and the
  rounds loop), parameterized by `sha256.New`/`sha512.New`.
- `cryptBase64_24Bit` — the GNU/crypt base64 variant (`./0-9A-Za-z`,
  little-endian 6-bit groups, no padding).
- `cryptMD5` — PHK md5crypt (1000-round inner loop, `$1$` magic).
- `cryptSHA256` / `cryptSHA512` — emit `$5$`/`$6$` with the correct output byte
  permutation; `rounds=` segment emitted only when explicitly requested
  (matching glibc, which omits it for the 5000 default).
- bcrypt (`$2a/$2b/$2y`) and traditional DES paths and all permission gating are
  preserved unchanged.

## Known-answer (parity) vectors — proof of real-DB compatibility
Added `TestCryptGlibcKnownAnswers` (and `TestCryptRoundsHonored`) in
`builtins/crypto_test.go`. Vectors are the published Drepper SHA-crypt.txt KATs
and PHK md5crypt KATs, carried verbatim in `github.com/GehirnInc/crypt`'s
`{sha256,sha512,md5}_crypt/*_test.go`:

- `$5$` `crypt("Hello world!", "$5$saltstring")`
  = `$5$saltstring$5B8vYYiY.CVt1RlTTf8KbXBH3hsxY/GNooZaBBGWEc5`
- `$5$` rounds=10000 `"Hello world!"`/`"$5$rounds=10000$saltstringsaltstring"`
  = `$5$rounds=10000$saltstringsaltst$3xv.VbSHBb41AL9AvLeujZkZRBAwqFMz2.opqey6IcA`
- `$6$` `crypt("Hello world!", "$6$saltstring")`
  = `$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1`
- `$6$` rounds=10000 `"Hello world!"`/`"$6$rounds=10000$saltstringsaltstring"`
  = `$6$rounds=10000$saltstringsaltst$OW1/...y3RnOaw5v.`
- `$1$` `crypt("abcdefghijk", "$1$$")` = `$1$$pL/BYSxMXs.jVuSV1lynn1`
- `$1$` `crypt("password", "$1$deadbeef$")` = `$1$deadbeef$Q7g0UO4hRC0mgQUQ/qkjZ0`

All reproduced EXACTLY.

## Test output
```
go test ./builtins/ -run 'TestCryptGlibcKnownAnswers|TestCryptRoundsHonored|TestReview_IO_Crypt|TestCryptDES' -v
--- PASS: TestCryptDES
--- PASS: TestCryptGlibcKnownAnswers (all 7 subtests: sha256 x3, sha512 x2, md5 x2)
--- PASS: TestCryptRoundsHonored
--- PASS: TestReview_IO_CryptSHA256RoundsSilentlyCapped
```

## Before/after failure list (`go test ./builtins/`)
The crypt red test `TestReview_IO_CryptSHA256RoundsSilentlyCapped` went from
FAIL -> PASS. No new failures introduced. The remaining failures are the
pre-existing, unrelated intentionally-red review findings in files I did not
touch:
- `review_data_test.go`: AbsMinInt64Overflow, UniqueStrCaseInsensitive,
  IsMemberStrCaseSensitiveBug, SetaddUniqueConsistency, SortReverseIgnored,
  PcreMatchEmptySubject, CapitalizeDeprecatedTitle
- `review_io_test.go`: FileReadlinesBinaryMode (M1), QueuedTasksSortOrder (H4)
- `review_test.go`: VerbCodeAllowsOwnerWithoutReadBit, AddVerbUsesProgNotPlayerForPerm

## Scheme status
- `$1$` md5crypt — IMPLEMENTED (PHK, glibc-exact).
- `$5$` sha256crypt — IMPLEMENTED (Drepper, glibc-exact, rounds honored).
- `$6$` sha512crypt — IMPLEMENTED (Drepper, glibc-exact, rounds honored).
- `$2a/$2b/$2y` bcrypt — preserved (existing `go-crypt/x/bcrypt` path).
- traditional DES — preserved.

## Commit
dc21daa297c60ca967ec191ff5ea330a14618e1e (branch review/branch-stocktake-2026-06-25)
