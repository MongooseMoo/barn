# Fuzz guest did not start after a host reboot

## Goal
Q 2026-11-13: "Did the machine break? Any idea what's going on here, how can
we make this more fullproof, is it still opening prs and finding bugs?"

## State
- Broken from 2026-11-12 22:34 UTC to 2026-11-14 04:28 UTC. No fuzz issue was
  opened after #805 (11-12 15:46 UTC) and no repair PR after #802.
- Cause (host journal, mongoose.world): the host rebooted at 22:34 UTC on
  11-12 and again at 19:38 UTC on 11-13. `barn-compute.service` (the KVM
  guest holding listeners mongoose-barn-fuzz-1..4) had `UnitFileState=disabled`
  and no link in `multi-user.target.wants`, so neither boot started it.
  `.github/runner/README.md:68` said `systemctl start barn-compute.service`;
  the mount and the network unit beside it use `enable --now` and both came
  back.
- Before the reboot the unit was stopped and started at 22:20 and 22:32 UTC.
  Those two restarts and the reboot ended the lanes of cycle 43317047617 that
  were running: eight lanes are red with "The self-hosted runner lost
  communication with the server". They are not Barn findings.
- Fixed live 04:28 UTC: `systemctl enable --now barn-compute.service` on the
  host; read back `UnitFileState=enabled`, active, link present.
  `fuzz_ops.py status --host` then printed "No problems seen", runners online
  and busy, four queued lanes of cycle 43317047617 started.
- This branch: README line 68 now says `enable --now`.

## Decisions and why
- Cycle 43317047617 was left to finish, not cancelled. Its acceptance will
  fail on the eight lost lanes; the cycle submits its own successor either way.

## Surprises
- Nothing reported the outage. The controller's watchdog job runs on the fuzz
  runners (`fuzz-controller.yml:61-63`), so with the guest down it cannot
  run: three scheduled controller runs sat queued until GitHub cancelled them.
  When it does run it treats a queued cycle as "chain is alive".
- `fuzz_ops.py status --host` already printed "host barn-compute.service is
  inactive" and "runner ... is offline"; nobody ran it.

## Unknown
- Who or what restarted the unit at 22:20 and 22:32 and rebooted the host
  twice. `last` is not installed on the host; the journal lines read did not
  name a login.

## Next step
- Merge this branch. Check that cycle 43317047617 ends and a successor on a
  commit after #809 starts and finishes ToastDifferential inside 45 minutes.
- Open with Q: an alarm for "fuzz runners offline or a cycle queued too long"
  that runs somewhere other than the fuzz runners.
