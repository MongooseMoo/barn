# Milestone 1B: truthful Barn and WSL Toast profiles

Work only in `C:/Users/Q/code/barn`. Read `AGENTS.md`, the convergence plan, and the three `docs/reports/mongoose-phase0-*.md` reports. Verify branch/HEAD/tracked status before editing. Preserve all unrelated untracked files.

Implement the profile-data and direct-test slice only. Do not change production Go code, scripts, conformance code, or active documentation. Do not add a helper, interface, adapter, or alternate profile surface.

Current verified identities for this slice (2026-07-13):

- Debian WSL stock Toast: `/root/src/toaststunt/build-release/moo`, HEAD `aecc51e9449c6e7c95272f0f044b5ba38948459e`, ToastStunt `2.7.3_5`, ELF x86-64.
- Debian WSL Mongoose Toast: `/root/src/toaststunt-mongoose/build-release/moo`, HEAD `72e3c7f96ce7a41fdeba793aef8818dc4408072e`, ToastStunt `2.7.3_5`, ELF x86-64.
- Stock options file: `/root/src/toaststunt/src/include/options.h`, SHA-256 `a88a8c6c37b66ca65a08a318988361827f131421edeff25e5b4af83fb3fa8036`, `OUTBOUND_NETWORK=1`, no `PROMOTE_NUMBERS` definition.
- Mongoose options file: `/root/src/toaststunt-mongoose/src/include/options.h`, SHA-256 `6c855f6b1f2dd584ba949d42891018ca68eccd34bd75b7e2300428b9246724a9`, `OUTBOUND_NETWORK=1`, `PROMOTE_NUMBERS` defined.
- Mongoose fixture: `C:/Users/Q/code/barn/mongoose_fresh2.db`, 101,244,108 bytes, SHA-256 `33201970097d3d2d2bfc0d5f875f087d587601bf8255ef31ef19b416d65ac925`.
- Canonical strict fixture: `C:/Users/Q/code/moo-conformance-tests/src/moo_conformance/_db/Test.db`, 2,035 bytes, SHA-256 `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.

Required files and behavior:

1. Add `profiles/barn/mongoose-outbound-on.conf` with explicit `OUTBOUND_NETWORK = 1` and `PROMOTE_NUMBERS = 1`.
2. Add `profiles/barn/mongoose-outbound-off.conf` with explicit `OUTBOUND_NETWORK = 0` and `PROMOTE_NUMBERS = 1`.
3. Update all four Linux/Windows Mongoose entries in `profiles/barn/profiles.json` to reference the matching new config, use it in the command template, and declare `option.PROMOTE_NUMBERS: true`. Do not alter strict/Test.db entries.
4. Add immutable oracle manifests under `profiles/toast/` using the existing Barn manifest field vocabulary:
   - `stock-wsl-testdb.json` for the canonical strict Test.db fixture, stock implementation/options identity, `runtime_os: linux`, `arch_bits: 64`, `option.OUTBOUND_NETWORK: true`, and `option.PROMOTE_NUMBERS: false`.
   - `mongoose-wsl-mongoose.json` for `mongoose_fresh2.db`, the Mongoose implementation/options identity, `runtime_os: linux`, `arch_bits: 64`, `option.OUTBOUND_NETWORK: true`, and `option.PROMOTE_NUMBERS: true`.
   Use lowercase checksums, `support_status: supported`, exact binary/config paths, `build_system: cmake`, and no invented dirty-state claim.
5. Extend `profile/registry_test.go` to table-test all four Mongoose entries: the referenced config parses with promotion true and the declared outbound value, expected promotion metadata is true, a built manifest validates against the entry, and missing/false promotion is rejected.
6. Extend `profile/manifest_test.go` so generated manifests explicitly prove `option.PROMOTE_NUMBERS` false and true while retaining database/config checksum coverage.

Run `gofmt` only on changed Go tests. Run `go test ./config ./profile ./cmd/barn`, then `go test ./...`, then `git diff --check`. Do not launch Barn or Toast. Do not stage or commit. Report exact files, test results, and any unrelated failures.
