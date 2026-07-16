# Barn Agent Notes

## Conformance Discipline Rules

- For conformance work, use only the documented managed conformance command unless the user explicitly approves a different path.
- Do not manually launch Barn or Toast for conformance work unless the user explicitly asks for a manual repro.
- Do not manually run against a tracked database file. If a manual repro is explicitly required, use a disposable copy outside tracked fixtures.
- Do not create ad hoc debug executables with arbitrary names for conformance work. Use the managed server flow, or one clearly temporary disposable binary under `.tmp` only when the user explicitly approves a manual repro.
- If the managed harness command fails to start or connect, report the exact command, working directory, expected result, and actual failure before improvising.
- Do not guess at startup failures, transport failures, IPv4/IPv6 issues, or harness behavior. Prove them or report them as unknown.

## Conformance Verification Rule

- Before debugging, implementing, or changing Barn for a conformance-test behavior that is uncertain, surprising, order-dependent, or poorly understood, first verify the exact behavior against Toast.
- For Barn Toast-oracle and conformance work, Toast means the WSL oracle documented in `plans/barn-toast-mongoose-convergence-workstreams.md`: `/root/src/toaststunt/build-release/moo` (`~/src/toaststunt/build-release/moo` inside WSL). Do not use Windows Toast binaries for this workflow unless the user explicitly asks for a Windows Toast repro.
- Do not spend time on Barn-side root-cause analysis until the expected Toast behavior has been confirmed.
- If Toast verification is blocked, stop and say so plainly before proceeding further.
