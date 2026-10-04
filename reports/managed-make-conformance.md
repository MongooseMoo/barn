# Managed Makefile conformance (#224)

The four `conformance*` recipes attached to a manually started server on port
9300 and invoked retired `cow_py` pytest tests. They now use the documented
`uv run --project ... --frozen moo-conformance` entrypoint. A shared command
supplies the packaged database/startup directory, stock oracle manifest,
admission/conformance markers, and strict skip/marker policy. Barn targets
verify the main package and build only Barn before the harness owns its server
and disposable database lifecycle.

Quiet, verbose, stop-on-first-failure, and selected modes retain their respective
output flags. Any nonempty `K` selection automatically includes canonical
`capability_admission`. `conformance-toast` uses the canonical WSL Toast
executable and explicit stock target profile. Linux/WSL is the documented
execution platform; its uv environment is outside the checkout.

The fixed-port `run-test` and background-server `quick-test` helpers were
deleted. `clean` removes only the build directory and no longer deletes root
logs by wildcard. Help and CLAUDE.md describe the replacement commands.

## Verification

Debian WSL ran all five managed targets on `cache_stats_authority.yaml`, with
canonical admission in every session:

```text
conformance:       3 passed in 6.06s
conformance-v:     3 passed in 6.07s
conformance-x:     3 passed in 6.01s
conformance-k:     3 passed in 6.12s
conformance-toast: 3 passed in 6.13s
MANAGED_MAKE_FIVE_TARGETS_OK
```

After correcting absolute `BIN` resolution, final `conformance-k` passed in
6.13s. `go test . -run 'Repository|Hygiene' -count=1` passed in 2.085s and
printed `MANAGED_MAKE_FINAL_HYGIENE_OK`. `make help`, dry runs of all targets,
an absolute-BIN dry run, `make -n clean`, and `git diff --check` completed.
The absence probe printed `MAKE_OBSOLETE_SURFACES_ABSENT` for retired pytest,
cow_py, port 9300, quick-test, and root-log wildcard deletion in Makefile.
Clean itself was not executed.

The packaged Test.db SHA-256 remained
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
These were focused command-path checks; no fresh full-suite baseline or manual
server was launched. Existing CI supplies the full managed regression gate.
