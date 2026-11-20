# Toast CI profile audit checkpoint

- Scope: read-only audit of pinned Toast commit `aecc51e9449c6e7c95272f0f044b5ba38948459e` and current `moo-conformance-tests` issue-17 workflow/source. No server started and no conformance run performed.
- Current workflow builds one default 64-bit Toast profile with outbound enabled and runs 11,566 packaged cases, asserting 459 skips.
- Packaged declarations include 34 literal skips, 47 architecture-conditioned cases, 6 outbound-conditioned cases, 3 numeric-promotion-conditioned cases, optional-builtin conditions, and config requirements.
- Toast exposes `ONLY_32_BITS` as a CMake option. `OUTBOUND_NETWORK` and `SAVE_FINISHED_TASKS` are preprocessor settings in `src/include/options.h`; outbound also has runtime `--outbound` and `--no-outbound` switches. The default build has outbound compiled and enabled, while finished-task retention is not compiled.
- Toast's `server_version("features")` is empty at this commit because `VERSION_FEATURES` is never defined, even though the source implements the conformance features. This accounts for a large part of the default job's declared skips.
- Default optional dependencies enable curl, URL helpers, spellcheck, SQLite, PCRE2, Argon2, and TLS. `read_stdin` is behind the disabled `EXAMPLE` macro, `malloc_stats` requires jemalloc, `finished_tasks` requires `SAVE_FINISHED_TASKS`, and no `file_version` builtin exists.
- Blocker under investigation: identify the exact safe compile definitions/profile matrix and determine which literal skips cannot execute on this Toast commit.
- Next action: finish the static source mapping, compute effective skip-set unions by profile, and report exact commands/toggles to the parent agent.

## Mypy admission implementation checkpoint

- Shared worktree: `C:/Users/Q/code/.worktrees/moo-conformance-issue-17`; no commits or pushes.
- Added the maintained `types-PyYAML` dev stub and narrow mypy import overrides only for untyped `pycparser` and `moo_interp` packages.
- Declared the transport interface already required by the runner, typed parsed builtin registrations and the managed log stream, narrowed optional step text, and fixed a local tuple inference issue.
- `uv run --extra dev --frozen mypy src/moo_conformance` is green: 18 source files, no issues.
- Parent expanded the remaining scope to repository-wide Ruff admission, allowing mechanical fixes in the original owned files plus `bench/bench.py`, `tests/conftest.py`, `tests/test_profile_gate.py`, `tests/test_transport.py`, `src/moo_conformance/capabilities.py`, and `src/moo_conformance/profile_gate.py`.
- Next action: run repository-wide Ruff, apply only mechanical fixes inside that explicit scope, then run focused tests, Ruff, mypy, and inspect the final scoped diff.
