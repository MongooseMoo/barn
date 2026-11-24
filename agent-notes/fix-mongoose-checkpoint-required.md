# Toast reads Barn's Mongoose checkpoint: green, then required

## Goal

Q 2026-11-24: "we want toast loads barn's checkpoint to be required as
well". The workflow had never passed (26 failures), so it is fixed first,
then runs on every PR and in the merge queue, then becomes a required check.

## State

- Failure 1, reproduced locally on the pinned dump with the pinned Mongoose
  Toast (0ff6fd77): one walk record, #42804's property record, 82 MB. It
  passes Toast's default max_list_value_bytes (64,537,861; Mongoose sets none),
  and both engines abort the concatenation (Toast execute.cc
  PUSH_ERROR_UNLESS_QUOTA prints `*Aborted*`; Barn `E_MAXREC`), with
  identical tick counts. Fix: the walk preamble sets max_string_concat,
  max_list_value_bytes and max_map_value_bytes to 0 (no limit on both:
  Toast options.h; Barn builtins/limits.go canonicalizeLimit). With it the
  record is identical on both engines.
- Failure 2, exposed by fix 1: Toast's task count was read from the out.db it
  saves at quit, after the walk. In the Mongoose world each add_property of a
  new $server_options property forks two tasks on Toast (measured: queued
  tasks 105, 107, 109, 111); the path is not traced (bf_add_prop,
  property.cc:207, calls no hook). Fix: Toast's counts come from a separate
  load that quits at once (lane `toast_count`).
- Local run with both fixes: outcome success, no findings, tasks 93/12 for
  Barn written, Barn read and Toast read. Private copies deleted.
- Workflow: pull_request without a path filter, and merge_group, so the
  check reports on every PR and queue entry. Runs take 10-19 minutes on the
  workstation runners.

## Next step

After this merges and the check is green on master: add "Toast loads Barn's
Mongoose checkpoint and sees the same world" to master's required status
checks.
