# PR 109 bcrypt 2x boundary checkpoint

## Findings and observations

- Added focused `$2x$` password-boundary regressions in the dedicated bcrypt test file.
- Empty passwords succeed.
- An embedded NUL hashes identically to the prefix before the NUL.
- A 72-byte password hashes identically to the same password with a 73rd byte.
- The 71-byte and 72-byte boundary passwords differ.
- High bytes at positions 71 and 72 preserve `$2x$` signed-char divergence from `$2y$` while the high-byte 73rd byte is ignored.

## State

- Focused `$2x$` tests repeated ten times: pass.
- Focused race run: pass.
- Full `go test ./... -count=1`: pass.
- `go build ./...`: pass.
- `go vet ./builtins`: pass.
- Formatting and diff checks: pass.

## Blocker

- Full `go vet ./...` still reports the unchanged baseline warning in `vm/stack.go:49` about the `ReadByte` method signature. The file has no PR diff.

## Next action

- Commit only `builtins/bcrypt_2x_test.go`, push PR 109, update its verification summary if needed, and verify the remote head and checks without merging.
