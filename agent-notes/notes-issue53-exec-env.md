# Issue 53 exec environment checkpoint

## Current findings

- GitHub issue #53 reports that `builtinExec` accepts and type-checks the third `env` list but never forwards it to the child process.
- Barn is on branch `agent/issue-53-exec-env`, created from `master` at `5cdc373`.
- The Barn worktree already contained extensive unrelated untracked material; no pre-existing file has been modified or staged.
- `builtins/system.go` owns both the synchronous fallback and suspended-task execution paths through `execCommandWithContext`.
- The current helper accepts only program, args, and input; the subprocess `exec.Cmd.Env` is never assigned.

## Toast oracle observations

- The required oracle is WSL Toast at `/root/src/toaststunt/build-release/moo`, driven by the managed `moo-conformance` server flow.
- The first external suite-path attempt failed closed during collection because packaged suites cannot escape `src/moo_conformance/_tests`; no server behavior was observed.
- A temporary untracked suite now exists at `../moo-conformance-tests/src/moo_conformance/_tests/issue53_exec_env_oracle.yaml`.
- The first packaged selection failed closed because `-k` excluded canonical capability admission; rerunning with `capability_admission` selected satisfied the trust gate.
- The suite's inherited `exec_support` feature requirement caused a skip, so the temporary direct probe now requires only the demonstrably present `exec` builtin.
- With the managed server inheriting `BASH_ENV=/mnt/c/Users/Q/code/barn/.tmp/issue53-bash-env.sh`, Toast returned `{0, "one", ""}` for `exec({"test_args", "one"}, "", {"ISSUE53_CHILD=child"})`. This shows the explicit child environment replaces rather than appends to the server environment.
- A complementary row passes both `BASH_ENV` and `ISSUE53_CHILD` explicitly and expects `{0, "child one", ""}` to prove the supplied values reach the child rather than being ignored.

## Current state and blocker

- The final two-row managed Toast probe passed together with canonical capability admission: 3 passed in 4.97s.
- The live discriminator used a parent-only `LD_PRELOAD=/issue53-parent-not-found` and a child-supplied `LD_PRELOAD=/issue53-child-not-found`. The parent value was absent from child stderr while the supplied value produced the expected loader diagnostic.
- Toast source at `/root/src/toaststunt/src/exec.cc` confirms the exact mechanism: construct a fresh environment, put `PATH=/bin:/usr/bin` first, append supplied strings, then call `execve`.
- TDD red proof: `go test ./builtins -run '^TestExecCommandWithContextUsesToastEnvironmentSemantics$' -count=1` failed to compile because `execCommandWithContext` accepted no environment argument.
- Barn now builds the Toast-compatible environment in `builtinExec`, rejects non-string entries with `E_INVARG`, threads it through synchronous and suspended paths, and assigns `cmd.Env`.
- Focused green proof: the low-level replacement-semantics test, end-to-end builtin child-environment test, and non-string rejection test all passed (`ok github.com/MongooseMoo/barn/builtins 4.586s`).
- Temporary local probe files are `.tmp/issue53-toast-env.yaml`, `.tmp/issue53-bash-env.sh`, and the sibling packaged YAML named above. They are not intended PR content and must be removed after proof is captured.
- Intended PR files are only `builtins/system.go` and `builtins/system_exec_test.go`.
- The first full local CI attempt stopped at formatting because `gofmt -l .` traversed unrelated untracked nested worktrees under `.claude` and `.tmp` and reported files inside them. Vet, build, and full tests were not run in that attempt. The PR diff itself passes `git diff --check`.

## Next action

1. Run formatting against tracked Go files only, then run `go vet ./...`, `go build ./...`, and `go test ./... -count=1` with native exit-code checks.
2. Remove only the temporary oracle probe files after validating their exact paths; leave this checkpoint note and all unrelated untracked material uncommitted.
3. Stage only `builtins/system.go` and `builtins/system_exec_test.go`, inspect the staged diff, and commit.
4. Open a PR with literal `Closes #53`, audit it, watch required CI, normal-merge with `gh pr merge <number> --merge`, verify two-parent merge commit, and confirm issue closure.
