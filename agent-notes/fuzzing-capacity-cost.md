# Half-server CPU allocation for continuous fuzzing

## Goal

Use up to half of the existing 48-logical-CPU host for fuzzing before buying
more compute. Q approved the prepared change with "Make those changes please."

## State

Implementation is prepared on branch `fuzzing-capacity-cost` in the isolated
`barn-ci-fuzz-half-server` worktree, based on the remote default branch.

- Compute guest: 8 to 24 virtual CPUs.
- Host service CPU-time cap: 800% to 2400%.
- Four existing concurrent fuzz jobs: two to six workers each.
- Runner documentation updated to describe the CPU-time cap accurately.

The inspected diff contains those settings and this note. PR CI and deployment
are pending. The live continuous flag was true and FUZZ_MAX_PARALLEL was 4
when read during implementation. CI_RUNNER_LABELS was
`["self-hosted","Linux","X64","barn"]`.

## Decisions and why

The existing host reports a single Xeon Platinum 8260, 24 physical cores and
48 logical CPUs. Systemd CPUQuota percentages are relative to one logical CPU;
2400% permits 24 CPU-seconds per second across the guest service. It does not
reserve a particular set of physical cores.

Use the existing four listeners and increase each campaign's Go `-parallel`
from 2 to 6. This needs no additional runner slots, selector values, flags, or
orchestration. Keep the existing 30-minute campaign, 45-minute job timeout,
16 GiB guest RAM, 20 GiB host memory cap, and low CPU/IO weights.

Verification belongs to the existing PR CI: its benchmark-driver test command
covers the fuzz scripts, and Audit discovery workflows runs pinned actionlint
and offline zizmor. No new test that merely repeats the numeric settings was
added. The production service definition will also be checked by systemd
before the compute guest is restarted.

## Surprises

The host identifies as an Intel 8260, not the EPYC Q recalled. Prior read-only
inspection matched its hardware to the provider's Wolfhound listing; the
private account product label remains unknown. This does not affect the
half-of-48 CPU-time calculation.

Unrelated modifications in the original checkout are outside this task. The
earlier discussion note has been consolidated into this implementation note.

## Next step

Obtain the explicit PR-post authorization required by Q's publishing rule,
then publish the concrete change and run the existing PR checks. Once checks
and review requirements are satisfied, normally merge the change. Stop
successor dispatch through the existing fuzz-ops command and let active work
finish; preserve its findings and corpus. Deploy the reviewed service file,
restart only the compute guest, verify 24 guest CPUs and the 24-second CPU
quota, and resume the existing campaign. Inspect worker counts and completed
campaign evidence. A threefold quota increase is not itself a measured
threefold throughput gain.
