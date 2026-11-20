# Oracle bytecode bounds checkpoint

## Current state

- Assigned read-only managed Toast oracle verification for live Barn issues #39, #69, and #70.
- Scope: 255 versus 256 distinct constants in static property get/set/verb-call; oversized catch expression, try/except, try/finally, and fork body; MOO-visible `set_verb_code` rejection result/diagnostic and preservation of existing verb code.
- Required oracle: `MOO=/root/src/toaststunt/build-release/moo` through current `scripts/wsl_oracle.sh` only.

## Findings and observations

- Historical Markdown was excluded. Read inputs were the live GitHub bodies for open issues #39, #69, and #70; current Barn Go source/tests; and current `scripts/wsl_oracle.sh`.
- Every oracle process used `MOO=/root/src/toaststunt/build-release/moo` and was launched only by `/mnt/c/Users/Q/code/barn/scripts/wsl_oracle.sh`.
- Static-name boundary, strict form: source used 255 or 256 distinct static property/verb names directly in GET, SET, or CALL statements. All six candidates returned `{}` from `set_verb_code`; the known-old-code equality returned `0`. Toast therefore accepts both 255 and 256 for all three operand kinds.
- Static-name boundary, executable form: 254 or 255 filler string constants plus one static name (255/256 total distinct constants) also returned `{}` and executed correctly: property get `123`, property set `456`, verb call `789`.
- Oversized caught expression: a helper verb generated one 120,025-source-byte caught expression with 60,000 variable elements. Helper installation returned `{}`, candidate `set_verb_code` returned `{}`, and known-old-code equality returned `0`.
- Oversized try/except: a helper generated 20,000 `x = x + 1;` statements in the try body. Candidate returned `{}` and replaced old code (`0`).
- Oversized try/finally: the same 20,000-statement body returned `{}` and replaced old code (`0`).
- Oversized fork: the same 20,000-statement fork body returned `{}` and replaced old code (`0`).
- No requested boundary case was rejected by Toast, so there is no boundary diagnostic to copy. A rejection control proved the visible failure contract: unknown builtin returned `{"Line 1:  Unknown built-in function: foo"}`, old-code equality returned `1`, and invoking the verb still returned `9001`.
- Direct transport limitation: a generated command containing a 140,025-byte caught expression failed before Toast with Bash `Argument list too long` and exit 126. A 120,068-byte emergency input produced no result line and left the target unchanged. Those are excluded as oracle behavior; the source was then generated inside a small helper verb while preserving the managed launcher.

## Exact command patterns and outputs

- Direct managed command form: `wsl -d Debian -u root -e env MOO=/root/src/toaststunt/build-release/moo bash /mnt/c/Users/Q/code/barn/scripts/wsl_oracle.sh <emergency-command>...` from `C:\Users\Q\code\barn`.
- Large cases used `wsl -d Debian -u root -e bash -s`; that small Bash program exported the same `MOO` and invoked only `/mnt/c/Users/Q/code/barn/scripts/wsl_oracle.sh`. It installed `zz_oracle` plus `zz_builder`; `zz_builder` generated the large source in the scratch database and returned `set_verb_code(#2, "zz_oracle", code)`.
- Strict constant outputs per case: `=> 35`, `=> {}`, `=> {}`, `=> 0` (add target verb, install known body, install candidate, compare candidate with known body).
- Large-case outputs per case: `=> 35`, `=> 36`, `=> {}`, `=> {}`, `=> {}`, `=> 0` (add target, add helper, known target body, helper body, helper invocation/candidate install, old-body comparison).
- Rejection control exact command appended these managed arguments: `;add_verb(#2,{#2,"rxd","zz_oracle"},{"this","none","this"})`, `;set_verb_code(#2,"zz_oracle",{"return 9001;"})`, `;set_verb_code(#2,"zz_oracle",{"x = foo(1,2,3);"})`, `;verb_code(#2,"zz_oracle") == {"return 9001;"}`, `;#2:zz_oracle()`. Exact output: `=> 35`, `=> {}`, `=> {"Line 1:  Unknown built-in function: foo"}`, `=> 1`, `=> 9001`.

## Blocker

- No behavioral blocker. Direct huge emergency-command transport is unsuitable, but managed helper generation verified all required behavior.

## Next action

- #39 implementation is now assigned TDD-first in isolated worktree `C:\Users\Q\code\.worktrees\barn-fix-39-bytecode-static-names`, branch `agent/fix-39-bytecode-static-names`, from exact live master `b3d730e8bd3579517124fd011126f0d514547ce2`.
- Current worktree head is still the exact base; no commit, push, or PR yet.
- Relevant baseline passed: `go test ./bytecode ./vm ./builtins`.
- Audit finding: suspended VM frames persist raw `Program.Code` opcode bytes. New opcode values must therefore be appended rather than inserted. Existing `OP_GET_PROP` / `OP_SET_PROP` / `OP_CALL_VERB` with a `0xFF` name operand must remain decodable for already-saved dynamic-name tasks.
- Planned representation: append dynamic-name opcode variants for newly compiled dynamic operations and append wide-static variants for constant index 255. This removes the new-program collision, keeps all existing opcode numbers, and preserves legacy sentinel decoding.
- Test-only changes are written but not yet executed or committed: `vm/static_name_constant_boundary_test.go` proves executable static get/set/call at index 255 and dynamic-name behavior; `builtins/verbs_set_code_constant_boundary_test.go` proves `set_verb_code` accepts and stores all three boundary forms.
- Immediate next action: run focused tests to capture exact red failures, inspect/gofmt the test-only diff, commit the red phase separately, then implement the appended opcode representation.
- Red phase completed and committed: `4af56b70a41cc3dd2000ba265d50dba0389f6bd4` (`test: expose static name constant 255 collision`). Exact focused result: all three static index-255 cases compile with 256 constants and `edge` at `Constants[255]`, then return `E_TYPE`; dynamic get/set/call controls pass; all three `set_verb_code` installation cases pass.
- Partial green implementation now exists only in the isolated worktree and is uncommitted: appended explicit dynamic and wide-static opcodes in `bytecode/opcodes.go`, operand ownership in `bytecode/disassemble.go`, shared name-resolution work in new `vm/op_name.go`, and property decoder refactoring in `vm/op_property.go`.
- The partial implementation has not been formatted or compiled yet. One known mechanical correction remains: the VM short reader is named `ReadShort`, not `FetchShort`; `op_name.go` should compare against `types.TYPE_STR` directly.
- No architectural blocker. Next action is to finish verb-call decoding/dispatch and compiler emission helpers, make the two mechanical corrections, add persisted legacy-sentinel coverage, then run focused green tests.
- #39 implementation completed in the isolated worktree with a clean status and two commits:
  - red `4af56b70a41cc3dd2000ba265d50dba0389f6bd4` — `test: expose static name constant 255 collision`
  - green `e5252a3ef71e0d359fb7c1b4512ff186d4c5c717` — `fix: separate dynamic names from constant 255`
- Final diff from exact base is nine files: new builtins boundary test; compiler, disassembler, and opcode updates; new shared VM name decoder; property/verb/dispatch updates; new VM boundary/persistence test.
- Green architecture: all original opcode values are preserved; six name opcodes are appended. New programs use explicit dynamic-name opcodes and wide static-name opcodes for index 255. Original name opcodes retain legacy `0xFF` dynamic decoding for raw Programs restored from suspended-task persistence.
- Verification green: focused boundary matrix x20; focused race; full `go test ./...`; full `go test -race ./...` (scheduler 275.420s); `go vet ./...`; `go build ./...`; gofmt; `git diff --check`; post-commit focused smoke.
- No push or PR was performed. No known blocker remains. Parent should independently review exact head, merge current master normally into the branch if it has advanced, rerun gates, then handle PR workflow.

## 2026-08-08 issue #39 PR execution checkpoint

### Current findings and observations

- Live issue #39 still requires all 255 and 256 distinct static-name get, set,
  and call programs, plus unchanged dynamic-name behavior and successful
  `set_verb_code` installation.
- A fresh managed WSL invocation used only
  `MOO=/root/src/toaststunt/build-release/moo` through
  `/mnt/c/Users/Q/code/barn/scripts/wsl_oracle.sh`. Windows-to-WSL argument
  transport produced only three of seven expected result lines: verb id `35`
  and `{}` for the 255/256 static-get programs. Exit was zero. The missing
  set/call outputs are transport-incomplete evidence, not Toast failures; the
  live issue body's prior managed six-case result remains authoritative.
- The original red commit `4af56b7` was replayed in a disposable detached
  worktree. The focused VM test exited 1 with `E_TYPE` for static get, set, and
  call at constant index 255. The disposable worktree was then removed after
  an exact path and clean-status check.
- Current master `598df03` was merged normally into
  `agent/fix-39-bytecode-static-names`. The merge commit is `7ce22f3` with
  parents `e5252a3` and `598df03`. The sole content conflict was
  `bytecode/disassemble.go`; the resolution retained both master's
  `OP_END_FINALLY` operand and issue #39's wide-name operands. New issue files
  were also updated to master's GitHub module path. Focused bytecode, VM, and
  builtins tests passed after that resolution.
- Literal 255/256 distinct-name tests are committed as red commit `b31d313`.
  Get and call reach the intended compact/wide boundary. Static property-set
  programs currently fail first with `too many local variables (max 255)`
  because `compileAssign` allocates a unique object temporary for every
  sequential property assignment.

### Current state

- Isolated worktree:
  `C:\Users\Q\code\.worktrees\barn-fix-39-bytecode-static-names`.
- Branch: `agent/fix-39-bytecode-static-names`.
- Head: `b31d313` (`test: cover exact static name boundaries`).
- Root checkout's large untracked set remains untouched. This note remains
  untracked and must not enter the PR.

### Blocker

- No external blocker. The exact static-set regression requires a narrow
  compiler temporary-lifetime repair before the branch can be green.

### Next action

- Make property-assignment object/name temporaries reusable by nesting depth so
  sequential assignments share slots while nested assignments retain distinct
  live slots. Then run the exact boundary matrix, full local gates, managed
  conformance, push, open a PR containing `Closes #39`, watch required CI, audit
  files/commits/scope/checks/closing phrase, and normal-merge only when green.

### Final outcome

- Property-assignment temporaries now reuse slots by nesting depth; exact
  255/256 static get, set, and call tests, dynamic-name tests, legacy persisted
  sentinel tests, and nested-assignment lifetime tests all pass.
- Full local `go test ./... -count=1`, `go vet ./...`, `go build ./...`, focused
  race tests, formatting, and diff checks passed.
- PR #127 contained exactly nine issue-scoped files and five intended commits,
  with `Closes #39` recognized by GitHub.
- Required GitHub checks passed: `Build & unit tests` in 51 seconds and
  `MOO conformance suite` in 4 minutes 28 seconds.
- Normal merge command was `gh pr merge 127 --merge`. Merge commit
  `98dbd3b3bea1274c0edbb2d4114e4d508ac228ae` has two parents:
  base `598df037b511384ac468e520de0e3e600d3e1cdc` and feature
  `535060bc52591e2955eb724bf2e7c35e79abbea4`.
- Issue #39 is closed by PR #127. This note remains untracked and was not part
  of the PR.
