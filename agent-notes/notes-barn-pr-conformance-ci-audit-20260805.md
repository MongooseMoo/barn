# Barn PR conformance CI audit checkpoint

- Scope: read-only design audit of live Barn CI, current conformance workflow YAML/code, and live GitHub metadata.
- Barn CI currently checks out mutable conformance `main` and invokes the CLI without the canonical profile/admission/evidence path.
- Current conformance `toast-conformance-core.yml` supports `workflow_call`, but it runs Toast admission/full profiles only and derives repositories/refs from the caller event context.
- Current `paired-candidates.yml` is `repository_dispatch` only. It already pins Barn/conformance SHAs, runs canonical Barn admission and all packaged cases, and validates exact JUnit identity coverage/failure sets.
- Cross-repository dispatch would require an overpowered PAT/App bridge and is not the recommended Barn required-check path.
- Recommended direction: add a strict expected-success reusable paired entrypoint, pin its workflow ref and conformance SHA in a base-owned Barn PR workflow, pass exact fork repository/head SHA, keep candidate execution at read-only/no-secrets, and report one stable fail-closed check on the exact PR head.
- Existing scoped issue: moo-conformance-tests #18, "Add paired Barn and conformance candidate CI"; it remains open and covers the remaining automation.
- Next: finish exact workflow/API change plan, fork behavior, required-check reporting, and risk analysis.
