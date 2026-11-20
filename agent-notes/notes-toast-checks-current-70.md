# Current Toast checks live-proof checkpoint

- PR `MongooseMoo/moo-conformance-tests#60` merged normally as `0e9ac7db76d8f6e06372516ae5452313aef5de4d`.
- Verified merge parents: prior main `09056a9daea918b0b851d81f66c7025be38e7985` and PR head `c640edc07fdc339e73467567936b36eefa531e46`; both are ancestors.
- PR scope was one commit and exactly four intended paths: paired workflow, new read-only Toast core, public Toast wrapper, and workflow regression tests.
- PR checks were fully green: trusted/untrusted controller gates, all three Toast profiles, exact `Full suite` ledger, and both required exact-head checks.
- Local proof was green: 415 tests, actionlint v1.7.7, Ruff, mypy, and 11,581 collected tests (one admission plus 11,580 packaged cases).
- The original paired default-branch run `31050391689` had `startup_failure` with zero jobs because the reusable workflow contained a skipped job requesting write authority above the read-only caller ceiling.
- The fix separates the read-only reusable core from the sole write-capable base-owned PR reporter wrapper. Core and paired workflows have no concurrency; only the public event wrapper owns PR/push cancellation.
- Main-push live proof run `31052068238` is in progress at the merge SHA; its three Toast profiles are actively running.
- Managed paired live proof run `31052108970` is in progress with Barn `aa82abf05a6a23783ba69105d0c393f926304a13` and conformance `0e9ac7db76d8f6e06372516ae5452313aef5de4d`. It expects exact admission failure `admission::option.OUTBOUND_NETWORK`; unlike the old failure, it started real jobs and all three Toast profiles are actively running.
- Issue 43 remains open intentionally until the paired run emits exact admission evidence and a successful trusted verdict.
- Next action: wait for both managed runs, audit full Toast execution/ledger and paired evidence, close issue 43 only on exact successful proof, and verify final state.
