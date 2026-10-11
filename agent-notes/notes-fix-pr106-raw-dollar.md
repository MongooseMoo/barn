# PR 106 raw bcrypt salt delimiter repair

- Assigned worktree: `C:\Users\Q\code\.worktrees\barn-fix-106-raw-dollar`.
- Branch: `agent/fix-106-bcrypt-raw-dollar`, based exactly on `origin/master` merge `911b97f`.
- Scope is limited to `builtins/crypto.go` and `builtins/crypto_bcrypt_prefix_test.go`; no push is authorized.
- Finding: `parseBcryptPrefixCost` selected the last dollar delimiter, so a supported 16-byte raw bcrypt salt containing a dollar was folded into the cost token and rejected.
- Red proof: the new table failed all six `$2a$` and `$2y$` cases. Default-cost programmer and non-default wizard cases returned `E_INVARG` instead of succeeding; non-default programmer cases returned `E_INVARG` instead of `E_PERM`.
- Fix state: select the first delimiter after the bcrypt prefix and parse the cost with `strconv.Atoi` for overflow-safe validation.
- Green proof: focused bcrypt/parser tests passed 10 times and under `-race`; the full `go test ./... -count=1`, `go build ./...`, and `go vet ./builtins` gates passed.
- Next action: inspect the final scoped diff, run formatting and diff checks, commit only the two owned files, then report the commit and clean status without pushing.
- This checkpoint note is deliberately outside the worktree and must remain uncommitted and unstaged.
