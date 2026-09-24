# Fable review: checkpoint consistency under admission (INCOMPLETE)

Read-only; no tests/conformance run. Notes only — final replaces.

Author annotation: the reviewer stopped before a verdict because the external CLI returned HTTP 429, `model_requires_usage_credits`, and "You're out of usage credits." Session a9ab3ce1-a743-4eff-81c0-8ddac1dbab95. This draft is not an acceptance; the earlier admission ACCEPT does not cover this checkpoint follow-up.

## Questions
1. Is the explanation (TaskSnapshots before SnapshotWithRoots gate => torn checkpoint) supported by source?
2. Is Pause(ctx)+SweepMu/VMStartMu+snapshot+write sound at cap 1? cycles? shutdown/panic checkpoints?
3. Can a suspended Eval mutate state outside admission/sweep protection?
4. Follow-up: taskCompletion pins result.Val until OnComplete returns; Scope.Finish 1ns floor.
