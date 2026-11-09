# Managed Mongoose diagnostic witnesses

These investigation probes have recorded stock/Barn outcomes. Run them through
the [managed candidate command](../../docs/mongoose-conformance.md), using this
directory as `--moo-suite-root`, and include `capability_admission` in the same
session.

`server/uncaught_error_task_identity.yaml` currently passes stock Toast and fails
Barn's handler/origin task-ID equality predicate. The
[evidence report](../../docs/reports/mongoose-error-task-identity-20261014.md)
records the exact source, binary, commands and outcomes. This explicit diagnostic
suite is separate from the existing `mongoose-conformance` candidate selection.
