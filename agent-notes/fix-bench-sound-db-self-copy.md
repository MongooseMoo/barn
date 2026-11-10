# fix/bench-sound-db-self-copy

## Goal

Stop the real-workload harness from truncating the sound catalog when
`BARN_MONGOOSE_SOUND_DB` names the same file as its copy target
`files/sqlite/sound.sqlite` (relative to the repo root the harness chdirs to).

## State

Done, committed, not pushed. `copyBenchFile` in
`engine/mongoose_real_bench_test.go` stats src (through the open handle) and
dst; when dst exists and `os.SameFile` says they are the same file it returns
`copied=false` without opening dst. The one caller (the sound catalog install)
logs "already in place ... not copied" in that case. New test
`engine/copy_bench_file_test.go` `TestCopyBenchFile` covers the ordinary copy
and the same-file case reached through an absolute and a relative spelling.

## Decisions and why

- Return a `copied` bool rather than an error: the same-file case is a valid
  setup (running from the main checkout), not a failure.
- `os.SameFile` on Stat results instead of comparing path strings: two
  spellings (absolute vs relative, slash direction) of one file must match.

## Surprises

The previous code opened src, then `os.Create(dst)`; on the same file that
truncated it to zero before `io.Copy` read anything, so the 481 MB catalog
was destroyed and every `say` failed with "no such table: AssetFiles".

## Next step

None for this fix. The destroyed main-checkout `files/sqlite/sound.sqlite`
must be restored from its original source separately.
