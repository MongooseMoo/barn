# Current campaign state

- PR 105 was normal-merged as `f5596133f3e1d11eacfb81a9d7d8e28f7d838c62`; its merge has two parents and issue 102 is closed.
- PR 106 is updated from that exact master at head `cc2407b7f38df988a6ece89abcb2e5b15adedd10` with a normal two-parent branch merge.
- PR 106 scope remains two bcrypt files and seven owned failing identities. Focused race tests, full Go tests, build, formatting, and diff checks pass.
- `go vet ./...` stops on the existing unrelated `vm/stack.go:49` warning that `ReadByte() byte` should have the standard `(byte, error)` signature. The bcrypt diff does not touch it; keep it separate.
- Managed paired run `31066742002` is running PR 106 against conformance `acc2718cbc3093c7527448faec0f6c162dac9fe6`. It expects the exact 19-case post-PR105 baseline minus seven PR 106 bcrypt identities, leaving 12 required failures.
- Independent audit approves PR 106 scope/semantics and the stacked PR 108 delta. PR 108 must wait for PR 106, then merge newest master into its branch and retarget its GitHub base to master before exact verification.
- PR 96 worker commit 1 remains `34aae2b`; commit 2 scheduler/store shutdown publication work is in progress with explicit admission-gap guidance.
- Barn legacy-CI audit found conformance issue 18 as the scoped tracker. Proposed direction is a strict reusable paired workflow plus a base-owned trusted reporter, replacing Barn's mutable/unprofiled duplicate job.
- Next actions: finish PR 106 pair and normal-merge after exact audit; update/retarget PR 108; review PR 96 worker implementation; continue issue 18 workflow design.

