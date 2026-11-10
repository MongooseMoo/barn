# Nightly fuzz failure logs

Top-level `.log` files are GitHub-prefixed job logs. `native/*.log` files are
the original per-campaign tee logs from retained-corpus artifacts, beginning
with `go version` and native fuzz progress. The fixture check validates both
producer formats and rejects tool-truncation markers recursively.

`native/42692025227-baseline.log` is the first FuzzReadDatabase campaign from
run 42692025227's retained-corpus artifact. It records a real failed check
during baseline replay, with no mutation execution statistics.

Raw `gh run view <run> --log-failed` output captured for this change:

- `42201471629.log`: issue 610, unexpected termination/hang wording, saved input.
- `42009421620.log`: issue 606, unexpected termination/hang wording with saved input;
  issue 607, sibling FuzzSourceExecution campaign reports `signal: terminated`.
- `41882671144.log`: issue 593, recovered panic with Barn stack and saved input.
- `41680879951.log`: issue 593, unexpected termination/hang wording with saved input.

Go 1.27.1 `src/testing/fuzz.go:368-377` prints the coordinator error,
`Failing input written to`, and replay selector. `src/internal/fuzz/worker.go:141-145,
176-188` distinguishes internal failures, unexpected termination, and external
signals without a recorded crash; `sys_posix.go:106-137` explains crash signals.
The hang wording alone does not prove a timeout. These logs show no ordinary
failed-check case, so that path uses an explicit synthetic test.

The reporter reads existing committed corpus inputs where available; it does not
copy them here. Missing input files remain explicit in its JSON.

`FuzzStructuredSourceExecution/e58adbf989baa13f` and
`FuzzStructuredCompileVerifiable/0c4d4bbfffc14e70` are byte-for-byte copies of
records Go's fuzz cache held for those targets (written by Go's
`internal/fuzz/encoding.go` `marshalCorpusFile` as `[]byte(%q)`).
`test_fuzz_differential.py` decodes and renders them through
`cmd/moo-fuzz-source -corpus`.
