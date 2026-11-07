# Nightly fuzz failure logs

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
