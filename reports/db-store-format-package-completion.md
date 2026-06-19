# DB Store/Format Package Split Completion

Date: 2026-06-19

Commits:

- `111ad7c` - Plan db store format package split
- `89210b6` - Split store implementation by domain
- `a86584e` - Move live store model into db store
- `cf83d4d` - Move database format code into db format

Final shape:

- Live store/model code is in `barn/db/store`.
- Database reader/writer/startup repair code is in `barn/db/format`.
- Root `barn/db` has no Go package.
- `db/format.Writer` consumes `store.Snapshot`.
- No root-package aliases, forwarding constructors, storage adapters, generic backends, or compatibility package were added.

Final gates:

- `rg -n "^package db$" db --glob "*.go"`: no matches.
- `rg -n '"barn/db"' --glob "*.go"`: no matches.
- `rg -n "db\.(Store|Object|Property|Verb|VerbProgram|VerbArgs|ObjectFlags|PropertyPerms|VerbPerms|NewStore|NewObject|NewWriter|LoadDatabase|CompileVerb|QueuedTask|SuspendedTask)" --glob "*.go"`: no matches.
- `rg -n "TaskSource|CheckpointManager|Backend|backend|adapter|shim" db builtins server vm --glob "*.go"`: no matches.
- `rg -n "\b(store|s\.store|vm\.Store)\.(Get|GetUnsafe|All|GetAnonymousObjects)\(" builtins server vm --glob "!**/*_test.go"`: no matches.
- `go test ./db/... ./builtins ./vm ./server`: pass.
- `go build -o barn.exe ./cmd/barn/`: pass.
- `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"`: `3871 passed, 131 skipped in 142.68s`.
- `git diff --check`: pass.
