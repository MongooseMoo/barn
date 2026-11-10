# fix/exec-gate-after-checks

## Goal
exec() must cross the irreversible-effect boundary (exclusive commit gate plus
commit-and-renew) only when it is about to start a process. Before this change
exec was registered Irreversible, so the registry wrapper
(builtins/registry.go install) took the exclusive gate before builtinExec
checked anything. A failing exec (E_PERM, E_TYPE, E_INVARG for a bad path, a
missing program, bad input or a bad environment) started no process, yet held
the gate until its slice committed. In the Mongoose harness, with no
`executables/curl`, the #9655 physics fork children hit this on every heat
transition (1,521 holds, about 17 s of each 30 s window, census-p16).

## State
- builtins/system.go: exec's descriptor is ImplementationOwned. builtinExec
  calls beginIrreversible itself after its last check (the environment list)
  and before both process starts (the no-task synchronous fallback and the
  suspend-and-goroutine path), using the builtins/curl.go pattern. Error codes,
  their order, and the success path (suspend, gate released at the suspend's
  commit) are unchanged.
- builtins/catalog_contract_test.go: pinned catalog hash updated, with a
  comment line naming the exec effect change.
- builtins/system_exec_test.go: three new tests.
  - TestBuiltinExecFailedChecksDoNotCrossIrreversibleBoundary (10 cases): same
    error as before, zero boundary crossings, IrreversibleSideEffect false,
    task still running. Red on master (every case crossed once).
  - TestBuiltinExecCrossesBoundaryBeforeStartingProcess: one crossing, observed
    before the task is exec-suspended; the child's output arrives on resume.
  - TestBuiltinExecStaleAttemptAbortsBeforeStartingProcess: a refused boundary
    returns FlowAbortAttempt with no process and the task still running.
- Committed on the branch; not pushed, no PR.

## Decisions and why
- ImplementationOwned rather than a new effect class: curl and sqlite_query
  already use it for "the builtin crosses the boundary itself".
- The boundary sits before the no-task synchronous fallback too, because that
  path also runs the process.

## Surprises
- Toast's exec.cc checks the wizard bit late (after path, environment and
  input checks); Barn checks it first. Not changed here: the brief was to keep
  Barn's error order.
- Every other Irreversible descriptor (file_*, sqlite_open/close/limit/interrupt,
  listen, unlisten, open_network_connection, flush_input, force_input,
  set_connection_option, read_http, reseed_random, read_stdin, server_log,
  reset_max_object) also takes the gate before its own checks. flush_input has
  no effect at all. Not changed; listed with file:line in the campaign's
  fix-exec-gate progress file.

## Next step
Push, open the PR, and measure TestMongooseRealWorkload 1p/16p plus managed
conformance against master (exclusive-gate holds row, goodput, #9655.pending
losses). Owner of that step is the lead.
