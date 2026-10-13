# Review checkpoint: builtin dispatch/session split

- Scope: review `bb4349b6f10c5517dcd92340248bef942507bdbf` against merge base `39dd8f469f07af9e41391f4f0d4773266804e14e`.
- Worktree was already very noisy with unrelated untracked files; no tracked files were changed.
- Applicable instructions: root `AGENTS.md`; no changed files are under the scoped `parser/AGENTS.md`.
- Diff: 87 files, primarily moving mutable builtin state and host capabilities from `Registry` into `Session`, then threading the session through engine/server/VM call sites. Functions moved out of `builtins/signatures.go` are semantically unchanged after normalizing Registry-to-Session references.
- Potential issue examined: `builtinPcreReplace` now rejects a nil/missing session instead of constructing a default registry. Production dispatch always supplies a session, so this is not currently a qualifying finding.
- Current findings: none confirmed.
- Verification run: `go test ./... -count=1` completed successfully; `go test -race ./builtins ./vm ./engine ./server -count=1` completed successfully.
- Blocker: none.
- Next action: finish targeted inspection of Session/Host lifecycle and API call sites, then emit the required JSON review verdict.
