# Current paired admission JUnit checkpoint

- Main-push live run `31052068238` completed successfully through the new wrapper/core split: all three Toast profiles, exact Full suite ledger, and reusable result output job passed.
- Paired managed run `31052108970` started and completed all three Toast profiles and the exact ledger successfully. This proves the zero-job startup defect and concurrency defects are fixed.
- The paired Barn job completed successfully and produced managed evidence. Packaged Barn conformance was correctly skipped after failed admission.
- Managed admission JSON proves current Barn `aa82abf05a6a23783ba69105d0c393f926304a13` passes `option.OUTBOUND_NETWORK`, passes `feature.connectable_listener_port`, fails exactly `feature.ephemeral_listen` with `E_INVARG`, and passes `option.PROMOTE_NUMBERS` with false.
- Final verdict failed before identity comparison with: `admission JUnit does not identify the exact canonical admission test`.
- Managed JUnit classname was `controller.src.moo_conformance.test_conformance`; the trusted validator requires `src.moo_conformance.test_conformance`.
- Root cause is workflow construction: paired pytest uses `uv run --project controller` from the checkout parent, so the checkout directory leaks into JUnit identity. The canonical controller must be the step working directory.
- New branch from current conformance main: `agent/issue-43-canonical-admission-junit`.
- TDD red proof: focused workflow suite has exactly one failure because admission lacks `working-directory: controller`; 30 other focused tests pass.
- Next action: set both paired admission and packaged steps to `working-directory: controller`, use `uv run --frozen pytest`, run all gates, open/merge a narrow PR, then re-dispatch with evidence-backed required identity `admission::feature.ephemeral_listen` and close issue 43 only after the trusted verdict succeeds.
