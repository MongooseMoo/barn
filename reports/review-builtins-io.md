# I/O & System Builtins Review

**Files reviewed:** `network.go`, `network_test.go`, `network_http_test.go`,
`fileio.go`, `sqlite.go`, `compat_sqlite_test.go`, `curl.go`, `crypto.go`,
`crypto_nocgo.go`, `crypto_unix.go`, `crypto_windows.go`, `crypto_test.go`,
`argon2.go`, `tasks.go`, `system.go`, `gc.go`

**Baseline:** `go test ./builtins/... -count=1` → PASS (0 failures)

**Red tests written:** `builtins/review_io_test.go` — 4 tests, all fail.

---

## Architecture Summary

The builtins package is organized as a flat collection of Go files, each covering a
subsystem (fileio, sqlite, network, tasks, crypto, system, gc).  All builtins share
a common calling convention (`func(ctx *kernel.TaskContext, args []types.Value) types.Result`)
registered in `signatures.go`.

**Resource ownership:** Two distinct singletons manage open handles —
`fileState` (file handles) and `sqliteState` (SQLite connections).  Both use the
same pattern: a mutex-protected `map[int64]*handle` with a monotonic ID counter.
Neither handle type has its own mutex, which creates races described below.

**Sandbox model:** `fileio.go` enforces a `files/` directory root via
`sanitizeFilePath()` (rejects traversal) + `resolveFilePath()` (prepends `files/`).
All 16 fileio builtins call both functions in sequence.  `sqlite.go` calls only
`sanitizeFilePath()` and omits `resolveFilePath()`, breaking the sandbox for SQLite.

**Permission model:** Consistent wizard-only guard (`ctx.IsWizard`) on all I/O
builtins.  `curl.go` additionally gates on `ctx.RuntimeOptions.OutboundNetwork`.

**Crypto:** DES crypt delegates to `crypt(3)` on Unix (via CGo) or a pure-Go library
on Windows — correct.  MD5/SHA256/SHA512 crypt are home-grown, non-standard
implementations.

---

## CONFIRMED Bugs (red tests in `builtins/review_io_test.go`)

### C3 — SQLite files open outside the `files/` sandbox — CRITICAL

`builtinSqliteOpen` (`sqlite.go:307`) calls `sanitizeFilePath()` but never calls
`resolveFilePath()` (which prepends `"files/"`).  Every fileio builtin calls both;
sqlite calls only the first.  A wizard can create or open `.db` files in the process
working directory, bypassing the sandbox entirely.

**Red test:** `TestReview_IO_SqliteSandboxEscape`
```
review_io_test.go:46: CONFIRMED BUG (C3): sqlite_open("review_sandbox_escape_test.db")
    created the database in CWD (sandbox escape).
    Expected path: files/review_sandbox_escape_test.db
```

**Fix:** Change `sqlite.go:315` from `sql.Open("sqlite", path)` to
`sql.Open("sqlite", resolveFilePath(path))`.

---

### C1 — `crypt()` SHA256/SHA512 rounds silently capped and wrong algorithm — CRITICAL

`cryptSHA256` and `cryptSHA512` (`crypto.go:455–533`) cap `actualRounds` at 1000
regardless of the salt's `rounds=N` parameter, but write the original `N` into the
output prefix.  A call with `rounds=5000` and `rounds=2000` produce identical hash
bytes — the output is a lie.  Separately, neither function implements the actual
SHA-crypt algorithm (akkadia.org/docs/sha-crypt.html) used by glibc/Toast; they use
a simplified feedback loop incompatible with any standard verifier.

**Red test:** `TestReview_IO_CryptSHA256RoundsSilentlyCapped`
```
review_io_test.go:235: CONFIRMED BUG (C1): crypt() SHA256 rounds are silently capped.
    rounds=5000 and rounds=2000 produced identical hash bytes
    "VcxYHa0Ghm0Nr8YQOrg9Vwe8wImV4cXV8.OLmotOBn5"
    Full results:
      rounds=5000: $5$rounds=5000$testsalt$VcxYHa0Ghm0Nr8YQOrg9Vwe8wImV4cXV8.OLmotOBn5
      rounds=2000: $5$rounds=2000$testsalt$VcxYHa0Ghm0Nr8YQOrg9Vwe8wImV4cXV8.OLmotOBn5
```

**Scope:** MD5 crypt (`$1$`) has the same algorithmic problem (naive hash instead of
md5crypt) but no rounds cap, making it harder to demonstrate with a self-contained
test. Marked SUSPECTED C2 below.

**Fix:** Replace `cryptSHA256`/`cryptSHA512`/`cryptMD5` with conformant implementations
of SHA-crypt and md5crypt.

---

### H4 — `queued_tasks()` returns tasks in reverse chronological order — HIGH

`builtinQueuedTasks` (`tasks.go:46`) sorts with `StartTime.After()`, producing
descending order (newest first).  Toast returns tasks ascending (oldest first).
MOO code that relies on queue position — e.g., to find the oldest pending task —
will see the wrong task at index 1.

**Red test:** `TestReview_IO_QueuedTasksSortOrder`
```
review_io_test.go:192: CONFIRMED BUG (H4): queued_tasks() returned newer task at
    index 1 before older task at index 2.
    Cause: builtinQueuedTasks sorts with StartTime.After() (descending).
```

**Fix:** `tasks.go:46` — change `tasks[i].StartTime.After(tasks[j].StartTime)` to
`tasks[i].StartTime.Before(tasks[j].StartTime)`.

---

### M1 — `file_readlines()` ignores the binary flag — MEDIUM

`builtinFileReadlines` (`fileio.go:349`) uses `bufio.Scanner` unconditionally and
returns `scanner.Text()` without calling `filterTextMode` (text mode) or
`encodeBinaryBytes` (binary mode).  When a handle is opened in binary mode, line
content containing non-printable bytes must be `~XX`-encoded; currently it is not.

**Red test:** `TestReview_IO_FileReadlinesBinaryMode`
```
review_io_test.go:112: CONFIRMED BUG (M1): file_readlines in binary mode returned
    "ab\x01c", want "ab~01c".
    Cause: builtinFileReadlines ignores h.binary.
```

**Fix:** After `scanner.Text()`, branch on `h.binary` — call `encodeBinaryBytes`
or `filterTextMode` matching the behavior of `builtinFileReadline` and `builtinFileRead`.

---

## SUSPECTED Bugs (static analysis only, no oracle available)

### C2 — `crypt()` MD5 (`$1$`) wrong algorithm — CRITICAL (SUSPECTED)

`cryptMD5` (`crypto.go:439`) computes `md5(password + saltValue)` and base64-encodes
the result.  The actual md5crypt algorithm (OpenWall) is a multi-step construction
completely different from a single MD5 call.  Output will not match Toast/glibc for
any `$1$` salt.  Marked SUSPECTED only because no oracle run was possible; the code
error is unambiguous.

---

### H1 — File handle use-after-close race — HIGH (SUSPECTED)

`builtinFileClose` (`fileio.go:226`) acquires the handle via `getFileHandle()` (which
takes and releases `fileState.mu`), then calls `h.file.Close()` **without holding any
lock**.  A concurrent task that also obtained the same handle pointer (also after
releasing `fileState.mu`) can issue `h.file.Read()` or `h.file.Write()` on a closed
`*os.File`, producing `EBADF` or worse on a recycled file descriptor.  `mooFileHandle`
has no per-handle mutex.

A reliable race-detector test requires goroutines and would be non-deterministic.
The structural gap is confirmed by code inspection.

---

### H2 — `exec()` string form is silently dead — HIGH (SUSPECTED)

`builtinExec` (`system.go:219`) sets `program = "sh"` for the string form, then
`validateAndResolvePath("sh")` searches only subdirectories of `builtins/testdata/exec`
— never the system `PATH`.  Since `sh` is never present in `testdata/exec`, the
string form always returns `E_INVARG`.  The list form has the same constraint: only
programs pre-placed in `testdata/exec` can be executed.

---

### H3 — `resolveConnection()` fallback sends to wrong player — HIGH (SUSPECTED)

`resolveConnection` (`network.go:141–158`): when `player == ctx.Player` and the direct
`GetConnection(player)` lookup returns `nil`, the fallback iterates all connected players
and returns the **first** connection found.  If the calling player is temporarily
disconnected but other players are connected, `notify(self, msg)` silently delivers
to a different player's connection.

---

### M2 — `crypt()` non-wizard bcrypt restricted to cost 5 only — MEDIUM (SUSPECTED)

`cryptPasswordWithPerm` (`crypto.go:368`): non-wizards are rejected for any bcrypt
cost other than exactly 5.  Toast's restriction is "cost must be within allowed
range" but does not hard-code cost 5 as the single allowed non-wizard cost.
Behavior unverified without oracle.

---

### M3 — `argon2()` salt not decoded as binary string — MEDIUM (SUSPECTED)

`builtinArgon2` (`argon2.go:32`): `salt := []byte(s.Value())` treats the salt as
raw string bytes.  The rest of the crypto API (e.g., `binary_hmac`, `encode_base64`)
decode `~XX` escapes before use.  A caller who passes the output of `random_bytes()`
as the salt will have literal `~XX` ASCII sequences used as salt bytes, not the
intended binary values.

---

### M4 — `filterTextMode` strips tab (0x09) — MEDIUM (SUSPECTED)

`filterTextMode` (`fileio.go:173`) keeps only bytes `0x20–0x7E`.  Tab (`0x09`) is
stripped in text-mode reads and readlines.  Whether Toast strips or preserves tab
could not be verified without oracle.

---

### L1 — `curl()` cannot set request headers — LOW

`builtinCurl` (`curl.go:13`) accepts `(url, method, body)` only.  No way to set
`Content-Type`, `Authorization`, or any other request header.  Toast's curl supports
a headers map argument.

---

### L2 — `gc_stats()` returns all zeros — LOW

`builtinGCStats` (`gc.go:46`) returns a map with all zero values.  There is no actual
tri-color GC tracking.  MOO code that inspects GC stats for memory pressure decisions
receives no useful signal.

---

## Coverage Gaps in Existing Tests

| Gap | File |
|-----|------|
| No test for SHA256/SHA512/MD5 `crypt()` correctness | `crypto_test.go` (only DES tested) |
| No test for `sqlite_open()` with file paths (only `:memory:`) | `compat_sqlite_test.go` |
| No test for `exec()` string form or list form | (none) |
| No test for `resolveConnection()` fallback path | `network_test.go` |
| No test for `file_readlines()` in binary mode | (none) |
| No test for `file_read()` / `file_readline()` binary encoding | (none) |
| No test for file handle concurrent close+read | (none) |
| Global `fileState` singleton not reset between tests | (none) |
