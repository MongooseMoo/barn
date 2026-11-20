# Investigation: ordinary flight navigation in gameplay-flight-03

## Facts (verified)
- Flight03 result preserves completed_iterations=0 and the native command/output/state evidence; owned shutdown exited zero. Earlier flight01/02 controller failures remain separate cohorts.
- Commissioned ship has powered engines/thrusters, online computer, ordinary connected crew, active manning and explicit operate permission. Typed `setspeed 1.0` changes desired_speed to 1.0 with the normal confirmation message.
- Typed `heading 180 0` returns an empty output bracket. The following snapshot stays at cycle41115240, physical heading `{1,0,0}`, desired heading `{0,0,0}`, velocity0. A premature snapshot cannot yet establish a physics failure.
- The pinned checkpoint's actual helm clone inherits #3954:heading -> #4158:set_heading -> #4123::set_heading. This path assigns desired_heading through #91:deg_to_vec and announces rotation if it reaches that backend.

## Theories (plausible)
1. Another ordinary command/feature shadows heading, or the command dispatcher delivers different arguments: predicts no computer-backend assignment, while setspeed still succeeds.
2. Heading admission differs from speed (interaction/electronics/ACL): predicts a refusal before backend; current same-station fields alone do not prove action-specific admission.
3. The response bracket or snapshot precedes delayed command completion: predicts a later desired-heading change without changing permissions or commands.
4. Runtime angular conversion/backend raises or returns an unintended vector: predicts saved native error evidence or inconsistent cardinal conversion results.
5. Motion units or unused desired_heading account for physical expectations: cannot alone explain the missing synchronous desired-heading assignment; investigate only after the request path is established.

## Tests Run
| Test | Hypothesis | Result | Rules Out | Supports |
|---|---|---|---|---|
| Read saved flight03 result and exact inherited helm code with moodb | 1/2/3 | desired_speed changes, desired_heading does not; exact expected backend has an immediate assignment | blanket connection/operate failure | command-specific dispatch/admission/completion/conversion |

## Current Best Theory
The response bracket preceded a suspended heading backend. The exact source orders SQL-backed console sound before heading assignment but after the speed assignment. This explains the asymmetric saved state without permission or physics changes. A real delayed-completion probe remains necessary to confirm the cause in this cohort.

## Open Questions
- Which ordinary heading verb was selected, and what arguments reached it?
- Is there retained native error evidence from the heading request?
- Does a bounded later observer show completion without intervention?

## Next Action
Parent runs frozen flight04: after the unchanged typed heading command, boundedly observe the actual desired-heading assignment before sending speed. The independent physical position/velocity/heading/fuel and braking oracles remain unchanged; timeout still counts zero completed iterations.

## Source and controlled observer checks
- Actual helm ancestry is #19744 > #10044 > #3954 > #4158. The typed heading parser accepts two numeric arguments; #91:deg_to_vec uses north/+Y at0degrees and east/+X at90degrees. Cardinal180/0 has independent expected vector `{0,-1,0}`.
- #4158:set_heading calls `console_sound` before `computer:set_heading`; #4158:set_speed calls the setter before sound. Sound resolves its asset through #2585:pick_value -> resolve_ref -> #6128:resolve -> SQL q/val calls. Response-bracket semantics already permit suspension before domain completion.
- The shutdown dump was inspected with moodb, not raw grep. The actor's ordinary command log records heading180/0 and setspeed1.0. Its programmer flag is0; bounded traceback_log15 has no retained heading/conversion frame. That history cannot prove no error ever occurred.
- Units are source-proven: #4123::set_speed specifies km/s and passes the scalar unchanged; #4129:cycle_engines divides SI acceleration by1000, then assigns stored velocity as heading*desired_velocity. #559:cycle advances kilometre position by effective velocity*seconds. The stored impulse-velocity oracle therefore remains1.0km/s, with no guessed unit conversion.
- A deterministic observer control delays heading assignment for three observations after returning an empty bracket, and rejects speed sent before that assignment. Old module RED:1test/.004s/error1 (`speed sent before heading assignment`); changed module GREEN:10tests/.003s/OK. This proves sequencing, not actual game completion.
- Only the Python observer adds a bounded heading-assignment wait. No command syntax, MOO source, permission, velocity, fuel, scheduler or completed-work threshold changes. Module SHA2D62FD3749B493ACB678A80DDEB0CFC07E73DEC5211FBEB72BDCED44515DBBEE.

## Flight04 outcome and flight05 transport repair
- Flight04 confirms the delayed-assignment prediction: the legitimate typed heading reaches desired vector `{0,-1,0}` after four observations / 0.336373946 seconds. Natural cycle41115260 then shows changed position, partial rotation and decreased fuel. The full flight/braking oracle remains unverified and completed_iterations stays zero.
- Transcript record132 is an actual 1003-character native tagged literal ending `"velocity" ->...`. The velocity tail is absent, not an incorrectly parsed float. The former Control fallback returned a string, and the physical predicate raised TypeError; root owns the separate fail-closed parser repair. Failed cohort04 remains untouched.
- Snapshot transport now returns all original 23 fields as one fixed-order native list. Python reconstructs the dictionary after exact length and scalar/vector/component shape checks; malformed wire failures carry the wire, field order and original native output. No physical/admission/fuel/braking threshold or command changes.
- Targeted RED: 2tests / 6failures, including silent acceptance of truncated/wrong-width values. GREEN: 12tests / .003s / OK; py_compile exit0. Complete-width invalid integers, flags, vectors, NaN/Inf and component rows fail explicitly. These are observer checks, not game acceptance.
- Size replay `.tmp/flight-snapshot-wire-05/replay.json` parses the longest flight04 prefix and appends an explicitly SIMULATED high-precision velocity tail. Compact literal684characters, margin316 below1000, parse roundtrip equal. The missing actual velocity is not reconstructed or claimed. Source transcript SHA b5cb948edd65af85ebfc9b962878b4d78e2c80bbdab1470e902bbf0ca30c6077.
- Parent owns flight05 runtime on frozen module SHA85E763F58C9C834DD408D13AAA440A614D7B50470C60ABB8C42838372725BED1, tests SHA69D89CBDB7B59A00351C4BDA00589114DB8EA8494EF29EF55EBF4BD7F81360DB. Actual natural turn, target velocity, fuel debit, braking and terminal cutoff must all pass before one iteration counts.
