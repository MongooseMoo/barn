# Investigation: issue 279 workload validity

## Facts

- Original checkpoint and catalog hashes are recorded in experiments/issue-279/README.md.
- Initial run: native read ENOMEM while copying the catalog from a Windows-backed path. Later copies succeeded. Underlying allocation failure is unknown.
- Full-mix runs contain home E_NACC for player 71 and one unsettled say continuation; raw frames and diagnostics remain in the private evidence directory named in the task note.
- Repaired control run 42661518475 has three valid uninstrumented cohorts and one valid diagnostic cohort (zero failed/unsettled commands). The original failures remain excluded from performance comparisons. Paired owning-intern comparison run 42665898830 completed successfully: twelve uninstrumented cohorts plus two diagnostic cohorts, all zero failed/unsettled. Private raw outputs, profiles, inputs and both executables are saved beneath C:/Users/Q/code/barn/.tmp/issue-279-ci/42665898830-1.

## Theories

1. The home failure is a world-state rejection by the destination accept hook, reproduced by Toast; alternatively Barn's movement behavior differs.
2. Unsettled say is delayed external/readmission work under load; alternatively a continuation or scheduling defect prevents completion.
3. Catalog ENOMEM comes from Windows-backed filesystem transport; alternatively process/kernel allocation pressure is the cause.

## Tests run

| Test | Hypothesis | Result | Meaning |
| --- | --- | --- | --- |
| Read #1174:home, #9501:move_as_owner, #1:accept and #1:acceptable from the original checkpoint | Home failure is native world rejection | #71.home is #62; #62 inherits acceptable returning literal 0; movement runs with #71 owner permissions | A captured bad home, not evidence to change Barn permissions |
| Managed stock Toast candidate plus capability admission | Nonwizard owner with false accept should receive E_NACC | 2 passed; XML/admission retained under .tmp/conformance | Expected MOO behavior confirmed before any Barn movement analysis |
| Same authored witness plus capability admission through managed Barn | The candidate preserves the confirmed MOO behavior | 2 passed, zero XML failures/errors/skips; barn-move XML/admission retained under .tmp/conformance | Confirms the fixture repair does not require changing Barn movement semantics; final PR CI remains required |
| Evaluate designated player start on original checkpoint | There is an existing valid replacement home | {$player_start, $player_start:acceptable(#71)} = {#592, 1} | Extend existing explicit snapshot repairs for this captured defect |
| Inspect continuation #1886 with exact source | It is stuck versus repeatedly yielding | #1:loc_enter recursively visits contents then suspend(0); #249 has 166 contents; trace shows many executing slices before cancellation | Progress exists; preserve 10-second deadline and test clean uninstrumented workload |
| Three strace-controlled 32 KiB reads of original catalog | Native source read failure is currently reproducible | All reads completed; failed-read traces empty; memory pressure avg10/60/300 zero | Do not invent a current allocator failure or retry policy. Stage verified inputs onto the native runner filesystem before boot, separating the Windows/9p transport from the workload process |

## Next action

The validity defect is repaired in the harness and confirmed by complete cohorts. See experiments/issue-279/README.md for the full paired and GC results. No scheduler/permission change or catalog-copy retry was introduced. The old native ENOMEM remains an unreproduced setup error; the new verified staging separates that transport from the workload process. Final PR CI selects the managed false-accept witness alongside the existing local regressions.
