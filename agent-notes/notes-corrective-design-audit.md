# Corrective workstream design audit checkpoint

- Scope: current master `911b97f`, issue 102 atomic delete authority correction, and issue 72 bcrypt raw-dollar parser correction.
- Constraints: read-only audit, no historical Markdown, no manual Barn or Toast launch.
- Issue 102 current defect: `builtinDeleteVerb` resolves and checks transaction-snapshot authority, then performs irreversible `Store.DeleteResolvedVerb`, marks the transaction live-mutated, and adopts live verbs. A concurrent target owner/flag change or later transaction validation can therefore fail the task after the deletion is already live.
- Canonical authority storage: `Object.owner` and `Object.flags`, both protected by `Object.scalarVersion`; transaction `ObjectOwner`/`ObjectFlags`/`HasObjectFlag` record that version in `scalarReads`.
- Canonical exact-verb identity: opaque `ResolvedVerb{store,objID,index,listVersion}`; object verb topology is protected by `Object.verbVersion` and transaction `verbScans`.
- Canonical commit order: `commitGate`, then store locks; scalar, relationship, property, and verb read validation; only then staged apply.
- Design constraint: a live Store wrapper that checks authority and deletes under `s.mu` is insufficient because later transaction validation can still fail. The deletion must be transaction-staged and published only during the existing validation/apply window, or an equivalent operation must atomically finalize the entire transaction.
- Prefer a transaction-owned API that checks authority in the transaction view, updates a private verb topology, and stages the resulting topology against its base `verbVersion`. Replaying raw resolved indices is unsafe across multiple deletes because earlier deletes shift later indices.
- Preserve builtin error order: argument/type/object validation, descriptor resolution and E_VERBNF, authority and E_PERM, then staging. Preserve commit validator order so authority conflicts retry before any mutation.
- Deterministic issue 102 seams: a commit-validation barrier for concurrent owner/flag revocation; unrelated-read conflict with no deletion; concurrent verb-list mutation without retargeting; two deletes in one transaction; staged survivor code plus target deletion; overlapping aliases and index/string descriptors.
- Issue 72 current defect: `parseBcryptPrefixCost` uses `LastIndexByte` after the prefix, so a valid 16-byte raw bcrypt salt containing `$` becomes part of the cost token and is rejected.
- Parser constraint: use the first `$` after byte 4, parse only that token with overflow-safe arithmetic, and preserve validation, permission, then exact-two-digit ordering. Do not reject token length early because long leading-zero tokens can have values 4 through 31.
- Deterministic issue 72 seams: `$` at first/middle/last raw-salt positions, multiple `$`, NUL/high bytes, empty/missing delimiter, nondigit, costs 03/32, one/three digits, huge digits, and long leading zeros.
- Next: inspect the two corrective worktrees without editing, compare any implementation already present against these constraints, and send final blockers/actionable constraints.
