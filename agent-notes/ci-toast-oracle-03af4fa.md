# Stock Toast oracle to 03af4fa

## Goal
Q 2026-11-16: the Toast DoS bug (list `+` doubling past any limit, see
`agent-notes/ci-differential-system-kill.md`) is fixed upstream; move the
differential's Toast. Q chose to move the whole stock oracle, not only the
ToastDifferential lane.

## State
- Upstream fix: lisdude/toaststunt 6505a7f "Enforce max_list_value_bytes in
  the list + operator"; tip 03af4fa also fixes 32-bit range index crashes.
- WSL `/root/src/toaststunt` fast-forwarded on `master` aecc51e -> 03af4fa,
  `build-release/moo` rebuilt (Release): ToastStunt 2.8.0_7, executable
  SHA-256 61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd,
  options.h SHA-256 e41bf849...ebd60 (new curl limit defines only).
- `profiles/toast/stock-wsl-testdb.json` ref and config checksum moved;
  `profile/manifest_test.go` pins match; plan authority lines updated.
  fuzz.yml reads the ref from that manifest, so the differential follows.
- Barn: `vm/op_arith.go` list `+` returns E_QUOTA when the result's
  ValueBytes exceeds max_list_value_bytes (strictly greater, as Toast's
  OP_ADD; the builtins' check is `>=`). Concat now uses `types.Value.Concat`,
  which carries cached byte sizes, so the check is O(1) for both forms.
  Test `TestListAddUsesInclusiveListValueByteLimit` failed before, passes.

## Decisions and why
- Historical reports and dated run records that name aecc51e stay: they
  record what ran then.

## Next step
- Managed conformance on the new Toast; any Toast-red test is a 2.8.x
  behaviour change for moo-conformance-tests. Then PR, auto-merge, watch CI.
