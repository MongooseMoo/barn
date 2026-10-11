# Issue 107 bcrypt 2x implementation status

- Isolated worktree: `C:\Users\Q\code\.worktrees\barn-issue-107-bcrypt-2x`
- Branch: `agent/issue-107-bcrypt-2x`, based on `origin/master` at `aa82abf05a6a23783ba69105d0c393f926304a13`
- Head: `275063916473a18361e153ea9b6b574407538568`
- Draft PR: https://github.com/MongooseMoo/barn/pull/108 with literal `Closes #107`
- Scope: `builtins/bcrypt_2x.go`, `builtins/bcrypt_2x_test.go`, and minimal `builtins/crypto.go` dispatch/parser changes; no module-file changes
- Implementation: Openwall `$2x$` signed-char schedule adapter synthesizing 18 big-endian words / 72 bytes for `github.com/go-crypt/x/bcrypt`; C-string NUL and 72-byte limits preserved
- Openwall provenance: `CRYPT_BLOWFISH_1_3`, commit `3354bb81eea489e972b0a7c63231514ab34f73a0`
- Red: focused tests returned `*0` for all `$2x$` vectors before implementation
- Green: focused count 10, focused race, full `go test ./... -count=1`, `go build ./...`, `go vet ./builtins`, gofmt, and diff checks pass
- Known baseline gate: full `go vet ./...` reports `vm/stack.go:49:15: method ReadByte() byte should have signature ReadByte() (byte, error)` on both the branch and untouched master
- CI run 31001408582: Build and unit tests passed; MOO conformance failed before test execution with 11,566 capability-probe errors because `OUTBOUND_NETWORK` returned `1` while the live harness expects `ON`, `OFF`, or `#-1`
- Baseline evidence: unrelated CI run 30995033791 has the same `OUTBOUND_NETWORK` capability-probe failure
- Current blocker: draft PR is red only on the repository-wide moving-harness capability probe, unrelated to the three-file bcrypt diff
- Next action: finish exact PR/status audit and report the draft PR without changing PR 106, the integration worktree, or launching Barn/Toast manually

## Stack repair

- Parent review verdict for head `2750639`: CODE NO-MERGE because PR 108 duplicated parser ownership from PR 106
- Required stack base: `agent/issue-72-bcrypt-prefixes` at exact head `08a2de3498c8b888e0d0244cc12700bce3bd0830`
- A normal `--no-ff` merge was started from that exact remote head; the only conflict is `builtins/crypto.go`
- Resolution keeps PR 106's `parseBcryptPrefixCost` and Toast validation/permission ordering, adds `$2x$` to the canonical bcrypt branch, and selects only the 2x backend after permission
- The private leading-digit 2x pre-parser has been removed
- Added malformed-setting rows: `$2x$04x$...` programmer expects `E_INVARG`; `$2x$05` programmer expects `E_PERM`
- Current state: merge is not yet committed; focused and full gates still need to run
- Next action: format, resolve the index, run all requested gates, commit the merge resolution, push only PR 108, retarget its base to PR 106, and audit the displayed delta

## Stack repair complete

- New exact head: `34d2ff589bf6b37d0c77b1c7829abc1fa457e04d`
- Merge parents: `275063916473a18361e153ea9b6b574407538568` and exact PR 106 head `08a2de3498c8b888e0d0244cc12700bce3bd0830`
- PR 108 base is now `agent/issue-72-bcrypt-prefixes`; it remains draft and retains literal `Closes #107`
- Displayed PR delta: only `builtins/bcrypt_2x.go`, `builtins/bcrypt_2x_test.go`, and `builtins/crypto.go`; 331 additions and 3 deletions
- Displayed commits over PR 106: `2750639` implementation plus `34d2ff5` two-parent merge resolution
- Gates pass: focused count 10, focused race, full `go test ./... -count=1`, `go build ./...`, `go vet ./builtins`, gofmt, and stacked diff check
- Replacement CI run `31002569182`: Build and unit tests passed in 51 seconds; MOO conformance failed with 21 skipped and 11,566 capability-probe errors because `OUTBOUND_NETWORK` returned `1` instead of `ON`, `OFF`, or `#-1`
- Final state: branch pushed and clean; no merge, readiness change, PR 106 push, integration-worktree change, or manual Barn/Toast launch
