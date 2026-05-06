# Testdata reorganization report

Date: 2026-05-05
Workstream prompt: `prompts/testdata-reorg.md`

## Step 1: Survey results

### Tracked vs untracked

- `executables/` (8 files): all 8 `.bat` files were tracked in git.
  - `echo.bat`, `sleep.bat`, `test_args.bat`, `test_echo.bat`, `test_exit_status.bat`, `test_io.bat`, `test_with_sleep.bat`, `true.bat`
- `files/` (4 files + `sqlite/` subdir): everything UNTRACKED.
  - `__test_closed.txt` (empty), `__test_empty_b.tmp` (empty), `__test_empty_t.tmp` (empty), `test_fileio.tmp` (24 bytes — content "one\ntwo\nthree\nfour\nfive\n"), `sqlite/log.sqlite` (20480 bytes), `sqlite/sound.sqlite` (empty).
- `.gitignore` does not list these paths — they are simply not staged.

### Path references in Go source

Only two non-test files reference the paths, both in `builtins/`:

| File | Lines | Reference |
|------|-------|-----------|
| `builtins/system.go` | 352, 356, 357 | `execDirs := []string{"executables"}` and exe-relative variants joining `"executables"` |
| `builtins/compat_fileio.go` | 55, 59 | `filepath.Join("files", rel)` and `os.MkdirAll("files", 0o755)` |

Both `builtins/*.go` non-test source files. Per the workstream constraints I did NOT modify them — codex is mid-refactor in this directory.

### Path references in Go test files

None. Grep across `**/*_test.go` for `executables`, `"files`, or related patterns returned zero matches. So no test code needed editing for this move.

## Step 2: Files moved

| From | To | Mechanism |
|------|----|----|
| `executables/echo.bat` | `builtins/testdata/exec/echo.bat` | `git mv` (rename preserved) |
| `executables/sleep.bat` | `builtins/testdata/exec/sleep.bat` | `git mv` |
| `executables/test_args.bat` | `builtins/testdata/exec/test_args.bat` | `git mv` |
| `executables/test_echo.bat` | `builtins/testdata/exec/test_echo.bat` | `git mv` |
| `executables/test_exit_status.bat` | `builtins/testdata/exec/test_exit_status.bat` | `git mv` |
| `executables/test_io.bat` | `builtins/testdata/exec/test_io.bat` | `git mv` |
| `executables/test_with_sleep.bat` | `builtins/testdata/exec/test_with_sleep.bat` | `git mv` |
| `executables/true.bat` | `builtins/testdata/exec/true.bat` | `git mv` |
| `files/test_fileio.tmp` | `builtins/testdata/fileio/test_fileio.tmp` | plain `mv` then `git add` (was untracked, has real fixture content) |
| `files/__test_closed.txt`, `__test_empty_*.tmp`, `sqlite/*` | `builtins/testdata/fileio/...` | plain `mv`, NOT staged (runtime test artifacts, not source-controlled) |

Total: 8 `.bat` files + 1 fixture (`test_fileio.tmp`) committed at new locations. 5 untracked runtime artifacts (`__test_closed.txt`, two empty `.tmp` files, `sqlite/log.sqlite`, `sqlite/sound.sqlite`) physically moved to the new directory but left untracked, matching their previous state.

`git diff --cached -M` confirms rename detection on all 8 `.bat` files (similarity index 100%).

## Step 3: Files edited

None. Survey found zero path references in test files. Non-test source files that hardcode `"executables"` and `"files"` were left alone per constraints.

## Step 4: Verification

Build and test cannot establish a clean baseline at master HEAD because of pre-existing failures unrelated to this workstream:

```
$ go build ./...
# barn/db
db\reader_v17.go:10:21: method Database.parseV17 already declared at db\reader.go:275:21
db\reader_v17.go:89:21: method Database.readPlayersV17 already declared at db\reader.go:373:21
db\reader_v4.go:12:21: method Database.parseV4 already declared at db\reader.go:129:21
db\reader_v4.go:81:21: method Database.readPlayersV4 already declared at db\reader.go:354:21
db\reader_v4.go:100:21: method Database.readObjectV4 already declared at db\reader.go:750:21
db\reader_v5.go:12:21: method Database.parseV5 already declared at db\reader.go:201:21
db\reader_v5.go:77:21: method Database.readObjectV5 already declared at db\reader.go:266:21
```

```
$ go test ./builtins/...
... same db build errors ...
FAIL    barn/builtins [build failed]
FAIL
```

These errors are in `db/reader_*.go` — duplicate method declarations from a database-reader split that codex appears to have in progress. They are not caused by, nor related to, the directory move performed here. Per the workstream prompt: "If tests fail for unrelated reasons (e.g. codex's lift broke something temporarily), note it in the report and do NOT try to fix the unrelated issue."

The earlier `builtins/objects.go` build errors observed during initial survey (about extra arguments to `builtinCreate` etc.) are also part of codex's mid-flight registry lift, also unrelated.

## Step 5: Commit

Commit hash: `91b6b35` — "Move test fixtures under builtins/testdata/"

Commit content: 10 files changed, 123 insertions(+).

```
rename {executables => builtins/testdata/exec}/echo.bat (100%)
rename {executables => builtins/testdata/exec}/sleep.bat (100%)
rename {executables => builtins/testdata/exec}/test_args.bat (100%)
rename {executables => builtins/testdata/exec}/test_echo.bat (100%)
rename {executables => builtins/testdata/exec}/test_exit_status.bat (100%)
rename {executables => builtins/testdata/exec}/test_io.bat (100%)
rename {executables => builtins/testdata/exec}/test_with_sleep.bat (100%)
rename {executables => builtins/testdata/exec}/true.bat (100%)
create mode 100644 builtins/testdata/fileio/test_fileio.tmp
create mode 100644 reports/testdata-reorg-report.md
```

All 8 `.bat` renames recorded at similarity index 100% — git history is preserved.

**Note on staging:** During this workstream, three concurrent commits from the codex registry-lift workstream landed on master between staging and committing (`9c64bb0`, `66763dc`, `266e04b`). The first commit attempt failed because codex's intervening operations had cleared my index. I re-staged and re-committed; resulting commit `91b6b35` is on top of codex's concurrent work and contains only the directory move + this report.

## References that COULD NOT be updated (handoff to codex)

These hardcoded paths in `builtins/*.go` non-test source must be updated by codex (or in a follow-up commit once codex's lift lands), to point to the new `builtins/testdata/` locations:

1. **`builtins/system.go:352-357`** — `builtinExec` resolves programs by joining a literal `"executables"` directory name with the program name. Currently:
   ```go
   execDirs := []string{"executables"}
   if exePath, err := os.Executable(); err == nil {
       exeDir := filepath.Dir(exePath)
       execDirs = append(execDirs,
           filepath.Join(exeDir, "executables"),
           filepath.Clean(filepath.Join(exeDir, "..", "executables")),
       )
   }
   ```
   Needs to point at `builtins/testdata/exec` (likely with both repo-relative and exe-relative variants), or — better — be made configurable so production code does not depend on a `testdata/` path. This was the existing layout's coupling problem; the move surfaces it.

2. **`builtins/compat_fileio.go:55,59`** — `resolveFilePath` and `ensureFilesRoot` hardcode `"files"`:
   ```go
   func resolveFilePath(rel string) string {
       return filepath.Join("files", rel)
   }
   func ensureFilesRoot() error {
       return os.MkdirAll("files", 0o755)
   }
   ```
   For tests this should resolve to `builtins/testdata/fileio`. For production this should be configurable (probably from `ctx.Store` or a server config), as the file IO builtin is a runtime feature, not a test-only thing.

These two references mean that with this commit alone, anything that exercises `exec()` or the file IO builtins at runtime will continue to look at `./executables/` and `./files/` relative to CWD — which after this move no longer exist as source-controlled directories. Test runs that depend on those paths will fail until codex's work updates the runtime resolution (or until `files/` and `executables/` get recreated at runtime via `ensureFilesRoot()` and the new test setup).

## Notes for follow-up

- Once codex's registry lift settles and `go build ./...` is green again, run `go test ./builtins/...` to confirm the move plus path updates is clean.
- Consider parameterizing both base paths via a `BuiltinConfig` struct (or pulling them from `ctx.Store`/server config). Hardcoded relative paths in production code is the underlying smell that made this reorg awkward.
