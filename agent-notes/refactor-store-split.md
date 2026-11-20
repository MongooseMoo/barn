# refactor/store-split: Wave 3d, Store fields into named sub-structs

## Goal
Move 21 of `db/store.Store`'s 45 fields into seven named groups (`ids`, `history`,
`waifs`, `readers`, `waifRoots`, `dispatch`, `propPool`) and 4 more into existing
types (hot-read counters into `hotProperties`, `conflictTracking` into
`conflictCensus`). Methods stay on Store; only selectors change. Q said "make it so".
Design: session scratchpad `design-3d-store-split.md` (team-lead holds it). Store goes
from 45 fields to 27 (the design's "30 fields, 45 -> 24" was an arithmetic slip).

## Q's decisions
- Named sub-structs (`s.group.field`), not embedded; own PR; no allocation or
  benchmark change allowed.
- No committed layout test. The offset probe is a scratch check only.
- The `readTSShard` padding (store_core.go, "rounded clear of a line") is not part of
  3d; its layout and comment are untouched. A separate measure-then-fix PR.

## Method
- One commit per group, smallest first; the two folds share one commit.
- Edits by the Edit tool, driven by `go build`/`go vet ./db/store` errors.
  `gofmt -r` is unsafe (StoreTxn has its own `maxObjID`/`highWaterID`, db/format has
  `.propPool`); gorename cannot insert a path segment. `replace_all` was used only for
  names no other type has (`readTSShards`, `s.waifHistory...`, `s.waifDomain`).
- Group types are declared in store_core.go just above `type Store`. Each Store group
  field keeps a one-line comment naming its type; the old field comments moved onto
  the type or its fields.
- Per-commit gate, scratchpad `split3d/`:
  - layout: `zz_layout_probe_test.go.txt`, injected with `go test -overlay
    overlay.json` (never in the tree), prints all 45 fields under their pre-3d
    names; must equal the parent;
  - machine code: `regalloc.py <parent-tree> <tree>` builds db/store with `-S`,
    normalizes registers and jump targets, and reports REGALLOC-ONLY or CODE-DIFF;
  - benchmarks: `check.py ... bench` runs Q's command, `-count=3`, both sides.

## State (final)
- Branch rebased on origin/master ae0cf0c4 (#546, fix/store-b20). b20 adds no Store
  field and none of its new code names a moved field.
- Commits: 3b2a73de folds, e112bcc1 waifRoots, d356bea6 dispatch, 35596a50
  propPool, dc1e1972 ids, 1f889726 history, a501c69f readers, ba8da950 waifs, then
  this note. Every commit builds, vets and is gofmt clean on its own.
- Layout: all 45 offsets identical to master on every commit. On ae0cf0c4 Store is
  2912 bytes (b20 grew StoreTxn, which Store holds twice); on f137f23a it was 2896.
- Machine code, head vs master: 956 of 957 db/store functions byte-identical. The
  one difference is `(*Store).Recycle`: the parent-filter loop changes register
  choice and block order (one INCQ becomes LEAQ 1(R), one more JMP, two more reg-reg
  MOVQs); every memory operand is identical. It comes from e112bcc1 (Recycle inlines
  noteWaifRootsChanged) and dc1e1972. The design's probe found the same function.
- Benchmarks head vs master: allocs/op identical on all 13 rows. B/op identical on
  ten; CommitScratch/objects32, DirectDefinePropertyWide and TxnPropertyWriteCommit
  vary by a few bytes between samples on master too, and the ranges overlap.
- Gates on ba8da950: build, vet, gofmt, staticcheck (go1.27.1 build), golangci-lint
  ./db/... 0 issues; go test db/... builtins engine server vm ok; -race db/store
  engine ok; Elle Go generation seed 412 x40, healthy and ten faults, all PASS.
  Elle Java verdicts not run.
- PR body drafted to scratchpad `pr-3d-barn.md`. Not pushed; no PR opened.

## Surprises
- No db/store benchmark runs in parallel, so the benchmark gate cannot see false
  sharing; the layout probe covers it (identical offsets).
- The `readTSShard` pad comment says "rounded clear of a line" but the struct is 56
  bytes. Left alone per Q.
- The old `maxObjID`/`highWaterID` comment said allocateID/casMaxID are the only
  mutators; ResetMaxObject (store_metrics.go) stores both too. The moved comment
  names it.
- `gofmt -l` exits 0 when it lists files; gate on `test -z "$(gofmt -l ...)"`.
- Worktrees share refs: another session's fetch moved origin/master under this one
  mid-task.

## Next step
Team-lead: review, push and open the PR. Scratch worktrees
`C:/Users/Q/code/barn-store-split-base` and `-new` are removed.
