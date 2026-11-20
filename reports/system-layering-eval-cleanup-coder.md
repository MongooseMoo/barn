# Eval session builtin resource cleanup

## Goal

Close the builtin session owned by EvalSession after stopping its runtime, and
propagate cleanup failures through the existing expression, file and CLI owners.

## State

Read the builtin scout report, EvalSession implementation, runtime/session close
owners and all scoped production callers. The new regression opens real file
and SQLite handles through eval and confirms they remain valid after the old
EvalSession.Close. The scoped implementation now stops Runtime and delegates
to its existing builtin Session.Close, returning its error. Other in-progress
refactors remain outside this change.

RED command, inside verified Debian WSL from this worktree:

```text
go test ./internal/dbtool -run '^TestEvalSessionCloseReleasesBuiltinHandles$' -count=1
```

Raw RED excerpts:

```text
--- FAIL: TestEvalSessionCloseReleasesBuiltinHandles/file
handle after Close: found=true result={Val:"eval-cleanup" Flow:0 Error:E_NONE ...}, want E_INVARG
--- FAIL: TestEvalSessionCloseReleasesBuiltinHandles/SQLite
handle after Close: found=true result={Val:[... "path" -> ":memory:" ...] Flow:0 Error:E_NONE ...}, want E_INVARG
FAIL github.com/MongooseMoo/barn/internal/dbtool 0.039s
```

## Decisions and why

Use the existing Runtime and builtin Session owners directly. Runtime stops
before Session.Close. Preserve earlier failures by joining cleanup errors;
do not add a separate wrapper or sync.Once lifecycle.

EvalExpression and EvalFile now use named error results. Their deferred cleanup
prints the existing Error diagnostic only when Close fails and joins the close
error with any earlier error. cmd/moo.run uses a named exit status and reports
cleanup errors on stderr with exitFailed; prior failure is retained. The CLI
fixture's explicit Close now checks the returned error.

The new regression opens a real file under a disposable file sandbox and a real
in-memory SQLite handle through EvalSession.eval. It proves each handle is
valid before closure and rejected with E_INVARG through a newly created builtin
execution after closure. Sequential repeated EvalSession.Close returns nil.
Its cleanup drains the retained builtin Session even on a failing assertion.

GREEN output from the same explicit new-test selector:

```text
gofmt_exit=0
diff_check_exit=0
ok github.com/MongooseMoo/barn/internal/dbtool 0.042s
```

Formatting touched only the four verified literal Go source paths assigned to
this job. No broad local tests/build, benchmark, conformance run or fault
injection of Close errors was performed. No performance claim is made.

## Next step

Return the four-file scoped diff to the parent for review and PR CI after the
preceding independent work is merged. No git action, publication or merge was
performed by this worker.
