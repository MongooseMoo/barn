# Deletion exploration: builtin and server surfaces

## Goal

Explore production-code deletion without removing functionality or slowing execution. Scope: builtins, server, command, cmd, moo. Exploration only; no implementation, tests, servers, publication or commits. Parent owns the campaign note; this is the delegated evidence appendix requested by the parent.

## State

Static source exploration on 2026-10-25. No runtime or performance result is claimed. Estimated credible opportunity in this scope: roughly **450–850 production lines**, not 5,000. Estimates below are deliberately discounted and overlap is identified. No test, generated, documentation or blank-line removal is counted as implementation savings.

## Decisions and why

### 1. Remove argument checks already performed by existing descriptor admission

Smallest change: delete duplicate raw-implementation arity/type checks where the existing descriptor proves exactly the same admission; preserve nested-value, semantic, permission and variadic checks. No new framework, flags or validation layer. Estimated gross deleted 300–650 production lines, new 0, net 300–650. Tests may need direct-call fixtures routed through their existing admission helper; test changes are not counted as savings.

Evidence:

- `builtins/registry.go:139-145` constructs the validated callable returned by `Registry.Get`; `registry.go:245-248` validates before invoking an entry on ID/name dispatch.
- `builtins/signatures.go:48-84` validates arity, positional arguments, alternatives and an explicitly configured variadic tail. `signatures.go:155-170` resolves `call_function` through `Get` and invokes that validated callable.
- Real VM producer: `vm/op_misc.go:79` calls `CallByIDWithExecution`.
- `builtins/strings.go:312-314`, `328-330`, `344-346` each check optional trim characters are strings; the three descriptors at `strings.go:805-807` already require string arguments. Their implementation has no non-test direct caller found by the source search below.
- `builtins/crypto.go:962-985` repeats string argument checks for `string_hash`; `crypto.go:1509` already declares its string argument types. Similar duplication occurs in five related hash/HMAC implementations.
- `builtins/fileio.go:709-779` repeats one-argument checks in size/mode/modification/stat implementations, all fixed-arity in `fileio.go:1100-1105`.

Raw reconnaissance output:

```text
rg -n 'return types.Err\(types.E_TYPE\)|return types.Err\(types.E_ARGS\)' builtins --glob '*.go' --glob '!**/*_test.go' | Measure-Object
Count : 349
```

A read-only PowerShell regex restricted to three-line `if args[N].Type() ...` / `if len(args) ...` blocks returning E_TYPE/E_ARGS found:

```text
Count Name
42 builtins\fileio.go
15 builtins\sqlite.go
13 builtins\math.go
11 builtins\system.go
11 builtins\objects_hierarchy.go
10 builtins\strings.go
9 builtins\connection.go
9 builtins\lists.go
8 builtins\network.go
7 builtins\tasks.go
4 builtins\signatures.go
3 builtins\verbs.go
3 builtins\objects_players.go
2 builtins\argon2.go
2 builtins\objects_movement.go
2 builtins\url.go
2 builtins\maps.go
2 builtins\objects.go
1 builtins\curl.go
1 builtins\objects_misc.go
Candidate blocks: 157
Physical matched lines: 504
```

This count is a candidate inventory, **not** proof all 504 lines are removable. The regexp includes whitespace and misses checks using local variables. It does not inspect each corresponding descriptor.

An exact inspected subset is **132 production lines, no replacements**: 41 three-line checks in fileio (123 lines), plus the three trim optional-string checks (9). Fileio's 42 regexp matches start at lines `198,201,236,254,268,282,292,365,431,441,492,502,529,539,565,607,614,639,659,713,727,741,769,793,813,816,833,836,860,863,875,890,893,910,913,946,949,989,1010,1040,1047,1050`. Exclude `875`: the descriptor accepts only one mkdir argument, so this optional second-argument check is inside an unreachable branch, not independently redundant admission. All other listed argument positions/types and arities are admitted more narrowly or identically by `fileio.go:1087-1115`. The three trim descriptors at `strings.go:805-807` cover their checks. No `getFileHandle` check is included because it accepts an arbitrary value and has independent callers. Repository-wide non-test call search found only the two timestamp wrappers directly invoking a file builtin; both pass through their own identically fixed-arity descriptors before calling modification-time. This subset remains subject to the exported-Descriptor direct-call contract caveat below; it is source-proved for the observed in-repository dispatch routes, not an external API audit.

Important exclusions and uncertainty:

- `Signature.ArgTypes` may deliberately use -1; descriptor admission then cannot replace implementation-specific checks.
- `min`/`max` inspect homogeneous variadic values (`math.go:35-102`); those checks express more than generic numeric admission.
- `substitute` checks nested list shape (`strings.go:565-622`); keep those.
- Protected redirection occurs before argument validation (`registry.go:231-248`); preserve that ordering.
- Direct builtin-to-builtin calls exist: `fileio.go:755,762` call modification-time builtin; `objects.go:482` calls recycle. Audit each affected direct path instead of assuming the registry owns all entrypoints.
- `descriptors.go:41-49` exports implementation functions as construction data. External direct use of `Descriptor.Implementation` is not established here. Whether it promises independent admission is an open API-contract question; all in-repository production dispatch observed is validated.

Existing verification to preserve: `TestDescriptorDispatchAdmissionParity` (`descriptor_dispatch_test.go:10`) covers ID/name/dynamic admission and error payload agreement, but swaps implementations for stubs and therefore is not sufficient alone. `TestConnectionOptionPreservesSemanticErrorOrder` (`:90`) explicitly checks an implementation-sensitive order. No tests were run.

Performance expectation: deleting redundant conditions adds no work. This is an inference, not measured performance evidence.

### 2. One owner for hash result formatting and algorithm selection

Smallest change: retain six public builtin implementations and their preparation/error order, share only the repeated final digest formatting, and use the existing constructor switch for ordinary hashes too. Avoid a mode-driven universal crypto dispatcher.

`crypto.go:939-959` selects an already-instantiated hash; `crypto.go:1247-1267` selects constructors for the exact same listed algorithms. `string_hash`, `binary_hash`, `value_hash`, `string_hmac`, `binary_hmac`, `value_hmac` repeat uppercase hex versus all-bytes ~XX formatting at `:991-996`, `:1037-1042`, `:1077-1082`, `:1136-1141`, `:1193-1198`, `:1237-1242`.

Estimated gross deleted 90–140, new 20–35, net 60–110 production lines **excluding** admission deletions counted in item 1. Optional algo/output parsing could add savings, but coupling error ordering makes it less attractive; do not count another 100 by folding everything together.

Preserve observed differences: `string_hmac` rejects MD5 at `crypto.go:1118-1120`; binary/value HMAC do not have that explicit rejection. `value_hash` replaces anonymous values with `*anonymous*` at `:1068-1071`; `value_hmac` uses `args[0].String()` at `:1234`. These are current-source differences, not claims of correct oracle behavior. A refactor must retain them until separately verified, not silently normalize them.

Risk: new helper/constructor calls might affect inlining/allocation. Do not promise no slowdown without applicable performance evidence. Do not replace password crypt algorithms with a general library merely to transfer their code elsewhere.

### 3. Remove the second binary encoder

`crypto.go:1447-1459` loops over bytes and applies the exact branches already implemented by `encodeByte` at `:165-173`. `fileio.go:177-183` loops through `encodeByte` and `EncodeRawToBinary` at `:190-192` exposes it. Repoint internal users to the existing single owner, remove the other implementation; no alias wrapper needed. Estimated gross deleted 15–20, new 0, net 15–20 production lines.

Real users found by `rg -n 'encodeBinaryStr\(|encodeBinaryBytes\(' . --glob '*.go'` include HTTP header/body conversion (`network.go:357,368,416,478,536`), random bytes (`crypto.go:1435`), Unicode encoding (`unicode.go:243`), file reads (`fileio.go:335,421,472`), and exec output (`system.go:434-435`). They must continue encoding printable ASCII literally and escaping tilde/nonprintable bytes. Do not unify `encodeAllBinaryStr`: hashes intentionally encode every byte.

Existing `TestFileConversionAllBytes` (`file_conversion_bench_test.go:40-65`) independently constructs the expected mapping for all 256 byte values. `BenchmarkFileConversion` (`:9-38`) covers several shapes; no measurements were run. Tiny helper-call overhead/inlining remains a performance consideration; retain whichever single loop preserves current performance.

### 4. Register equivalent file timestamp builtin implementations directly

`fileio.go:751-763` defines two wrappers, each repeating the wizard check and then calling `builtinFileLastModify`, which checks wizard itself at `:738-740`. The descriptors at `:1102-1104` have identical signatures/effects. Point the access/change descriptors directly to `builtinFileLastModify` and delete their wrapper functions. Gross deleted 14, new 0, net 14 production lines; descriptor replacements do not add lines. Public names remain separate and available.

Source search of all non-test Go files found only these wrapper definitions and descriptor references. Current behavior already reports modification time for all three; this proposal preserves that behavior and makes no claim about oracle correctness or filesystem access/change timestamp semantics.

### 5. Share the exact HTTP body-completion tail

`network.go:463-481` and `:521-539` implement the same chunked/content-length completion and map body insertion. Smallest change is one helper accepting existing header parse result and pairs; keep request/status parsing separate. Gross deleted roughly 40, new 22–28, net 12–18 production lines. This is a minor complexity improvement, not a large target.

Do not replace the parser wholesale with `net/http`. Current source accepts a request with no version (`:443`) and emits specific error-map codes; the existing test at `network_http_test.go:159` asserts the version key is absent. Folded header behavior at test `:185` joins continuation text and binary-escapes a tab. Compatibility must be demonstrated before any parser substitution. Current parser tests cover content length (`:133`), absent version (`:159`), folded headers (`:185`), and chunked responses (`:205`). No tests were run.

### 6. Consolidate three identical verb-selector branches, preserve distinct readers

`verbs.go:222-244`, `:275-297`, and `:694-717` implement name-or-index selection for `verb_args`, `verb_code`, and `set_verb_code`. All use object-local name lookup, 1-based to 0-based index conversion, E_VERBNF for name failures, and E_RANGE from index selection. Existing owners are `store_reads.go:187-200`, which directly delegate to transaction FindVerbOnObject/VerbByIndex/Resolve variants. A small builtin-level descriptor selector could share these three branches while keeping object validity and permission checks at callers. Estimated gross deleted 65–75, new 25–35, net **30–50 production lines**, separate from the argument-check subset above because descriptor signatures admit any selector and these switch branches still need validation.

Do not automatically include `verb_info`: `verbs.go:171-180` rejects nonpositive indices with E_INVARG and maps E_RANGE to E_VERBNF. Do not automatically include `disassemble`: `:889-896` uses `findVerbForRead`, an ancestry lookup, rather than object-local lookup. Do not merge delete's resolved-identity path (`:497-523`) with view-only reads. These differences make a broad single selector with flags unnecessary and risky. The source supports a small three-caller helper, not replacing every verb operation. Transaction behavior and isolation workflow would require the documented isolation guidance before implementation; this note makes no isolation claim.

## Rejected or deferred ideas

- Remove the 348-line builtin catalog/layout: rejected. `catalog.go:5-6` documents historical bytecode ABI; the literal layout is stable builtin ID ordering, while `descriptors.go:166-193` deliberately leaves holes for disabled capabilities. Reordering to save metadata would change persisted program interpretation. No claim made about all persistence consumers having been traced.
- Merge command wildcard matching with store matching blindly: rejected. `command/verbs.go:22-56` lowercases and matches prefixes; `db/store/store_verbs.go:13-33` additionally handles waif colon names and `:58-73` uses pre-lowered cached names. A shared lower-level wildcard primitive might save 20–30 lines, but the cold/hot ownership and colon differences must stay explicit. Not counted.
- Remove Registry's funcs map/closure and funnel Get/call_function through dispatch: rejected as a deletion recommendation. `registry.go:231-248` redirects protected functions; `signatures.go:157-170` invokes Get directly. Replacing it with ordinary name dispatch changes this observed difference. A smaller internal validated invocation refactor may exist but net savings are small and new call overhead is uncertain.
- Delete the `call_function(max_object)` integer conversion as unreachable: deferred. Base builtin returns an object (`objects.go:619-623`), but descriptor construction accepts supplied implementations (`descriptors.go:166`) and tests intentionally substitute them. The base implementation alone does not prove this compatibility branch unreachable for all registries.
- Collapse `cmd/moo` into `cmd/moo_client`: rejected. `cmd/moo/main.go:79-95` loads a local store/eval session; `cmd/moo_client/main.go:1-100` implements TCP-client configuration/event logging. Similar names are not duplicated functionality. Local eval already delegates to `internal/dbtool`, so deleting either command removes a surface.
- Remove bespoke password crypt algorithms: rejected without proving compatibility, platform support and speed for a replacement. `crypto.go:568-776` implements MD5/SHA crypt including rounds; existing bcrypt tests explicitly cover prefix/cost/permission ordering and signed-byte behavior. Outsourcing code is not automatically removing complexity.
- No server lifecycle/listener/input concurrency deletion candidate established. These large files were indexed and sampled, not exhaustively audited. Do not treat this as proof they are minimal.

## Surprises

The highest-value surface candidate is not a new abstraction: signature admission already exists and several old checks remain. Conversely, many superficially duplicate branches carry real compatibility differences. Large apparent savings from replacing entire HTTP/crypto implementations would need a new compatibility investigation and cannot be counted now.

## Next step

Parent should verify the cited source before relaying candidates. If Q later authorizes implementation, begin with descriptor-backed duplicate admission removal and the two tiny direct-ownership deletions; preserve semantic checks and all public names. Keep crypto formatting separate so any performance movement has a narrow cause. Follow project PR CI and managed conformance rules; this exploration did not execute either and makes no pass claim.
