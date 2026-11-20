# Builtin external resource lifetime survey

## State

P5 scout is tracing actual file, SQL, HTTP, subprocess and stdin owners.
Findings below are source observations, not measured workload rankings.

## Keep existing SQL and curl operation owners

SQLite holds its operation slot through execution and row conversion, releasing
before diagnostic I/O. Its operation context is handle-owned. Explicit interrupt
cancels the current operation or records one pending interrupt; Session.Close
closes, cancels and waits. Threaded SQL captures immutable inputs and stages its
start until commit, then resumes the captured suspension generation. Curl
captures URL/options, closes its response body and limits reading to 1 MiB.
No basis was found to blanket-thread these or attach shared handles to task kill.

## File cursor descriptor inconsistency

file_count_lines and file_grep acquire the shared position mutex, seek to zero
and scan without restoring the cursor (builtins/fileio.go:1020-1033,1056-1082),
but their public descriptors omit the irreversible effect policy (:1114-1115).
Other cursor-changing file builtins use it. Public registration supplies a real
call path, and toastcore examples call both. Before correction, verify cursor
semantics with managed Toast and add a deterministic retry-boundary regression.
Do not change cursor behavior or invent file workers to conceal the mismatch.

## Subprocess shutdown ownership remains unresolved

exec creates an independent 30-second background context, stores cancellation
on the task and launches an unjoined worker. Explicit task Kill cancels it.
Runtime.Stop cancels runtime contexts and joins scheduler/maintenance; no link
to exec's independent context was found. Session.Close drains file/SQL/HTTP
resources but no child-process contexts. Establish the required shutdown
behavior with an embedded regression before selecting the owner correction.
ProcessStdin has a single host-owned blocking reader and waiters, with no Close;
keep process stdin separate from session file handles.

## EvalSession teardown omission

internal/dbtool.EvalSession.Close calls only Runtime.Stop. The production server
explicitly stops its runtime and closes the builtin Session. EvalSession runs
arbitrary MOO, and EvalExpression/EvalFile defer Close, so file/SQLite handles
opened by an eval have no path to Session.Close's actual drain. The scout
finished the caller check. Minimum correction: EvalSession.Close returns an
error after Runtime.Stop and Session.Close; EvalExpression, EvalFile and
cmd/moo.run carry cleanup errors through their existing diagnostics. A regression
must open a real handle through eval, close the owner, then prove file_name on
that session rejects the former handle. Do not rely on OS unlink behavior.

## Completed dispositions

Keep per-handle SQL slots through conversion, commit-deferred starts, generation
checks, SQL close versus session cancellation distinctions, curl input/body
ownership, file-position serialization and task-owned exec cancellation.
Exec shutdown and stdin blocking lifetimes remain explicitly unverified.
The scout completed without running tests or changing source.
