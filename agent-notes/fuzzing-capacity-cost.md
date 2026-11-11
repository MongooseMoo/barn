# Half-server fuzzing CPU delivery

## Goal

Deliver the approved 24-vCPU guest, 2400% host CPU quota and six workers in each
of four existing native fuzz jobs. Q explicitly approved PR publication, CI,
merge and deployment on November 11. Keep campaign duration and memory limits.

## State

PR #753 is open from `fuzzing-capacity-cost`, in
`C:/Users/Q/code/barn-ci-fuzz-half-server`. Its substantive files are
`.github/runner/barn-compute.service`, `.github/workflows/fuzz.yml` and
`.github/runner/README.md`, plus the task record in that checkout.

The first CI run passed build/unit/race tests, fuzz-script tests, workflow
audits, durability and serializable-history checks. Conformance failed on
`no_flush_notify_returns_zero_when_unread_output_queue_is_full`: expected
`[1, 1, 1]`, actual `[1, 1, 0]`. Independent PR #756 addressed this and merged
at 08:08 UTC on November 11. Incorporate master into #753 and rerun its checks.
Do not duplicate that implementation.

The checkpoint-stage job hit its 20-minute timeout after completing the
healthy schedule and mixed-pair negative control. The drop-append workload
finished and wrote 796 history operations. Its first Elle input is 16905 bytes,
but the checker log is empty and no terminal checker verdict was retained.
The artifact is preserved in the worktree at
`.tmp/fuzz-half-server-ci/jepsen-stage-evidence.tar.gz`, from run 42981299875,
artifact 13839005778. The cause is not yet proven.

Successor dispatch was stopped with `py -3.13 scripts/fuzz_ops.py stop`.
Cycle 42972636456 is draining; the latest count was 12 completed targets,
four running and one queued. All running lanes were assigned to the server's
`mongoose-barn-fuzz-*` runners. Do not restart the guest until work drains.

The reviewed service file is staged at
`/etc/barn-compute/barn-compute.service` on `root@mongoose.world` and passed
`systemd-analyze verify`. SHA-256:
`45924094cbeca8016a704d807591a888c12616efe392b93bd953155408114a7b`.
The active unit remains `/etc/systemd/system/barn-compute.service`, with
`CPUQuotaPerSecUSec=8s`. No guest restart or live CPU increase has occurred.

## Decisions and why

- Half of the host's 48 logical CPUs is 24 CPU-seconds per second; this does not
  reserve specific physical cores or establish a measured throughput ratio.
- Preserve the existing four listeners, 30-minute campaigns, 45-minute job
  timeout, 16 GiB guest RAM, 20 GiB host cap and 14 GiB shared listener slice.
- Use the existing PR checks; do not loosen timeouts, checker verdicts or gates.
- Incorporate the merged PR #756 through master into #753 and rerun CI.
- The local default Python is 32-bit and could not launch SSH. Installed
  `py -3.13` is 64-bit and successfully ran the existing operations command.
- This original-checkout note is the live operational record. Consolidate it
  into the branch's committed task record before delivery; preserve unrelated
  original-checkout modifications.

## Surprises

The rented host is a Xeon Platinum 8260 (24 cores, 48 threads), matching the
provider's Wolfhound hardware listing, not the recalled EPYC Greyhound. The
private account's product label is unverified.

The pinned Elle CLI uses Elle 0.2.7. Upstream Elle issue #32 describes a fold
thread-pool deadlock fixed by PR #33. This is a candidate explanation only;
the saved history has not yet reproduced it under a captured thread dump.
The old CI jar path from provenance was absent in the local WSL probe.

## Next step

Resolve the CI failures without weakening checks, then normally merge #753.
Let the old fuzz cycle finish, install the staged unit into systemd, reload,
restart only the compute guest, verify 24 guest CPUs and a 24-second CPU quota,
resume native-only continuous fuzzing, and inspect the six-worker processes
and memory events. Preserve corpus and findings throughout.
