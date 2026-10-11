# Issue 25 parser-depth contract audit

- Scope: read-only verification against `origin/master` `b3d730e8bd3579517124fd011126f0d514547ce2`; no historical project Markdown, source edits, GitHub mutations, or tracked-database execution/mutation.
- Current defect: `parser.Parser` has no depth field; `ParseExpression`, `parseStatement`, and nested-body helpers recurse without an immutable ceiling. Formatter and compiler also recursively consume the resulting semantic IR.
- Oracle constraint: use only `scripts/wsl_oracle.sh` with `MOO=/root/src/toaststunt/build-release/moo`, which copies the database to scratch files.
- Database scan constraint: `cmd/dump_verb` loads through `db/format.LoadDatabase` but requires a named object/verb. Current search has not yet established a documented enumerating scan command; if none exists, report the stored-verb census blocked rather than author an ad hoc executable.
- Current source/tool observations: `Store.Snapshot()` exposes `AllObjects` and each `SnapshotObject.VerbList`, while current `cmd/barn` has an existing `-list-verbs` path that may provide safe enumeration. This must be verified before use.
- Next action: inspect the existing `cmd/barn -list-verbs` behavior, run bounded managed Toast probes at representative 256/512/1024 expression and statement depths, then derive a justified ceiling and at/over-limit matrix including compiler and formatter flat-chain exposure.
- This note must remain uncommitted and unstaged.

## Oracle evidence and blockers

- Managed command family: `wsl -d Debian -u root -e env MOO=/root/src/toaststunt/build-release/moo bash /mnt/c/Users/Q/code/barn/scripts/wsl_oracle.sh ...`; every successful invocation used the script's disposable database copies.
- Positive results: prefix `!`, parentheses, and nested list braces all compiled/evaluated at depths 256, 512, and 1024. Right-associative power compiled/evaluated at 256 and 512; depth 1024 produced no success result. Whole-program nested `if` via documented `;;CODE` compiled/evaluated at depth 256; 512 and 1024 produced no success result.
- A grouped `while` probe never started: Windows rejected the `wsl.exe` invocation with `The filename or extension is too long`. Per `AGENTS.md`, no altered invocation or manual launch was attempted.
- Full stored-verb nesting census is blocked. `cmd/barn -verb-code`, `cmd/barn -list-verbs`, and `cmd/dump_verb` require named objects/verbs; the one existing formatter test scans only the first 50 parseable verbs from `db/format/testdata/toastcore.db` and does not compute nesting. Although `Store.All`/`VerbByIndex` are read-only APIs, using them for a census requires a new ad hoc scanner, explicitly disallowed by the task.

## Ready contract

- Immutable maximum semantic/source nesting: 256 nested constructs on any root-to-leaf path. Depth 256 is accepted; the 257th construct returns an ordinary syntax diagnostic. Sequential/sibling nodes do not accumulate depth.
- MOO-facing diagnostic remains exactly `Line N:  syntax error`; internal detail is `maximum nesting depth exceeded (max 256)` and the position is the first over-limit token/node.
- Guard active recursive descent before descending, then validate the completed semantic IR iteratively. The second layer is mandatory because flat left-associative binary/postfix chains and flat `elseif` lowering create deep IR without deep active parser calls.
- Validate semantic IR iteratively at `bytecode.Compiler.Compile` and `CompileProgram` entry so non-parser frontends/direct IR cannot crash compilation. Formatter must validate before recursive unparsing and return an error rather than panic or partial output.
- Boundary matrix per recursive prefix, parentheses, lists/maps, right-associative power/assignment, nested `if`/`while`/`try`, flat additive/postfix chains, and flat `elseif`: depth 255 success, 256 success and compile/format/reparse round trip, 257 one syntax diagnostic/no program, 2560 one syntax diagnostic/no panic. Add direct semantic-IR compiler/formatter tests at 256/257/2560 and a wide-sequential control proving the budget is depth rather than total node count.
