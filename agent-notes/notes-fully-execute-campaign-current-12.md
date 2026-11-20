# Current campaign checkpoint 12

## State

- The live paired run proved all 11,566 cases execute for both Toast admission and the explicit Barn candidate.
- Toast admission is green in 64-bit outbound on/off and 32-bit outbound on profiles, including exact union accounting.
- The Barn run retained complete evidence and the trusted verdict rejected its 79 failures.

## Common cause addressed

- Barn's generated target profile already records `runtime.arch_bits: 64`.
- Conformance ignored that canonical profile fact and fell back to Toast's implementation-specific `server_version("options/ONLY_32_BITS")` build flag.
- Current Toast source confirms `ONLY_32_BITS` is a define-only version option; a disabled Toast build reports its false object through the slash path.
- The harness now consumes `runtime.arch_bits` when present, accepts only exact integer 32 or 64, detects conflicts with `option.ONLY_32_BITS`, and retains the live Toast option fallback for profiles without architecture metadata.

## TDD proof

- New profile architecture tests failed before implementation because the harness attempted a live `ONLY_32_BITS` probe.
- After implementation, all 73 requirement tests pass.

## Blocker

- The paired Barn candidate still has non-architecture conformance failures that must be reclustered after the harness change.
- Issue #18 still lacks its strict one-failure red fixture and zero-failure green fixture pair.

## Next action

- Run the entire conformance harness quality gate, commit and PR the architecture-profile repair, then dispatch the exact paired run to obtain the reduced Barn failure set.
