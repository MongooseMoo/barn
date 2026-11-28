# Checkpoint memory evidence

## Goal

Retain runner memory evidence for checkpoint resume discovery job #799 through the existing corpus archive.

## State

- Run 46102207713's checkpoint job reported that a worker was killed without an input. The source resource replay was skipped for this lane, while the retained native evidence upload succeeded.
- The existing checkpoint command now records its listener cgroup memory before launch, at its highest observed five-second sample, and after exit. The existing retained log upload carries the bounded record.
- Synthetic fixtures cover increasing OOM counters, memory composition before the failure, missing cgroup data, and the same child exit result. The workflow passed the pinned actionlint and zizmor checks.

## Decisions and why

- Keep memory data scoped to the shared cgroup. Individual PID and input attribution require separate evidence, and a fast spike can fall between samples.
- Reuse the runner cgroup resolution and retained log upload. Leave command arguments, cgroup limits, corpus retention and campaign verdict rules intact.

## Surprises

- The source replay's memory copies are not on the checkpoint job path. They cannot capture this worker failure after the checkpoint child dies.

## Next step

Publish the isolated correction and verify the next eligible checkpoint job archives its samples. An actual OOM record remains unverified until another affected run.
