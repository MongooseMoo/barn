# Live campaign checkpoint 29

## PR #46 second rejection

- Corrected staged-admission head `a8756eef330ba686f898db3eb7ddd283492c7c3a` passed ordinary analyst review and full managed Toast CI, but adversarial review found two deeper trust violations.
- A fake `@admission` item or a hookwrapper removing the canonical item after the trylast gate can still authorize packaged execution.
- The paired workflow lets the conformance candidate generate the expected packaged inventory and admission evidence, so a candidate can shrink its own purported complete surface or self-attest probes.
- Active correction requires a runtime guard set only by the exact canonical admission execution or validated external evidence, malicious-plugin tests, controller-owned harness execution, trusted-main-superset inventory validation, JUnit/JSON cross-check, and nonce/context binding.

## Barn repair rejection

- Barn local head `dc342649225a871277b36f9dd5cc63bb436f322d` is CODE NO-MERGE.
- Remaining blockers: late completed-task roots after shutdown-ready; non-atomic suspended state/VM publication; checkpoint reads of running mutable VMs; real frame-pop recursively promotes nested WAIFs; no startup consumer invokes loaded pending `:recycle` hooks.
- Active correction must add the actual deterministic startup pending-finalization executor and unify producer/sweep/snapshot lifecycle ownership.

## PR #45 hardening

- Managed Toast regeneration is now locally green for all three checked-in startup fixtures and byte-compares `.new` outputs to targets.
- Canonical natural-exit wait and exact file equality are implemented locally; exact serialized suspended-task payload assertions and full gates/matrix remain in progress.

## Trusted integration evidence

- Run `31003847397` remains the current accepted characterization of Barn integration `d528f5499f00b36817e3bfbe0ef38650ed14f00b`: 11,590 packaged, 11,146 passed, exactly one failed, zero errors, 443 skipped.
- The sole failure is the pending-finalization Anon2 identity; all eight bcrypt 2x failures are repaired.

## Next action

- Await new exact heads, then repeat analyst and adversary reviews from zero.
- Merge no conformance controller or oracle tests on green CI alone.
- Compose only independently accepted Barn commits and require trusted paired empty-set success before delivery.

This file is a local untracked checkpoint only. Do not stage, commit, or push it.
