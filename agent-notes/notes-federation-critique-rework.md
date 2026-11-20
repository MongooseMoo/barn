# Federation proposal rework — session notes (2026-08-12)

## Task
Q delivered a 12-point critique (+2 minor) of `plans/moo-federation.md` (Grange
protocol). We are to fix the proposal, including fundamental rethinks, grounded
in Barn and ToastStunt source. Deliverable: revised plan document, after
resolving open design forks with Q (per precompilation principle).

## Critique summary (in threat order)
1. Promise pipelining contradicts auto-suspend surface (`prox:tell()` suspends ⇒ chained calls are sequential round trips; pipelining needs promises as receivers, undefined).
2. Auto-suspend on verb calls is inconsistent with explicit-reads rationale (implicit atomicity shatter in existing code handed a proxy).
3. Principal-object "fail-closed, zero call sites" misstates MOO perms: verbs run with *programmer's* perms; task Programmer doesn't govern inside wizard-owned verbs; confused deputy untouched; phase-3 exit test doesn't test deputy case.
4. Local OBJ appearing in eventual-send args: undefined (auto-export = capability leak; E_TYPE = breaks code). Must be specified.
5. `typeof(prox)==OBJ` requires ~30 unwritten builtin rulings (parent, move, valid, .location, tostr, `in`, scatter...).
6. No availability/resource-exhaustion story (flood deliver, pipelined speculative work, fork, unbounded signed logs, deposit-gift disk abuse). No quotas/backpressure/log truncation.
7. `grange://key@host:port/swissnum` conflates key identity and location; no server-key rotation; host move breaks all refs. OCapN separates locator hints from identity.
8. Leases renew-on-use ⇒ quiet portals silently die. Portals need distinct keepalive/refresh policy.
9. Causal-hash event log has no consumer (idempotency only needs session seqnums; cross-peer causality = Matrix DAG we swore off). Cargo cult — drop or justify.
10. Versioned class imports hand-wave dependencies ($string_utils etc. ambient refs). Research project, not a phase-6 feature.
11. Ownership abroad undefined: principal "owns nothing but itself" vs acquiring items abroad. Either principals can own local objects or visiting is read-and-talk; must say which.
12. Moderation binds to identity; authority is bearer (sturdyrefs). Ban ≠ revocation; must couple the two systems.
Minor: dialback proves "did you send this," not key identity — specify authoritative check. Keep "virtual world" qualifier on "never deployed" claim.

## Direction sketched (told to Q, awaiting research + Q's calls)
- #1/#2/#5 root cause = `typeof==OBJ` + implicit suspend. Fix: new TYPE_PROXY
  (WAIF precedent: non-OBJ value that receives verb calls); `prox:tell()` =
  eventual send returning a promise IMMEDIATELY (never suspends; E semantics);
  explicit `federation_await(promise)` suspends; verb call on a promise value
  pipelines. OBJ builtins raise E_TYPE by existing rule — no 30-ruling matrix.
- #3: rewrite honestly — fail-closed at boundary/top frames only; deputy risk
  inside exported wizard-owned facet verbs named as facet author's burden;
  extend phase-3 exit test with deputy scenario.
- #4: raw local OBJ in send args ⇒ E_TYPE; export always explicit. Harmless
  once sends themselves are explicit.
- Open forks to bring to Q after research: causal log drop vs seqnums-only;
  cut class imports to research track?; principals owning locally-minted
  objects?; promise type representation details; resource-quota shape.

## State
- Read: `plans/moo-federation.md` (340 ln), `docs/reports/federation-barn-groundwork.md` (225 ln).
- Groundwork §4 already half-confirms critique #3 (per-frame Programmer, set_task_perms semantics).
- 3 background researchers running, writing to:
  - `notes-fed-research-perms.md` (barn-perms): verb-frame perms, E_PERM sites, type tags, non-OBJ dispatch, anonObjects.
  - `notes-fed-research-tasks.md` (barn-tasks): suspend flavors, task serialization, resume(value), existing resource limits, MVCC irreversible effects, network panic status.
  - `notes-fed-research-toast.md` (toast-source): waif.cc dispatch, primitive-type verb dispatch, execute.cc frame perms, Toast resource controls, type-tag numbering. WSL access via `wsl bash -c ...`.

## Research results — barn-tasks (arrived)
Full report: notes-fed-research-tasks.md. Key facts for the rework:
- Suspend commits txn + fresh RO snapshot (engine/task_runtime.go:404-434);
  commit failure at boundary KILLS task. Confirms suspend = hard atomicity
  boundary ⇒ critique #2's hazard is real and severe.
- New suspend flavor checklist exists (discriminator field, completer, Kill()
  clearing, snapshot field, IsInterruptedForPersistence decision, irreversible
  list entry). Wrinkle: flavor flags NOT restored across checkpoint — read/
  exec/http tasks come back as plain suspended. Federation suspends must define
  restart semantics explicitly (probably resolve to E_REMOTE on restart).
- resume(task_id, value) injects arbitrary values; error values raise unless
  WakeErrorAsValue. Good substrate for promise resolution.
- Resource limits: queued_task_limit and MAX_QUEUED_OUTPUT DO NOT EXIST in
  Barn (unbounded outputBuffer!). Only ticks/seconds, input-queue backpressure
  (256 global/64 per-conn), 1MiB ws frame cap, connect_timeout. No rate
  limits, no max connections. Critique #6 is even more right than stated —
  quotas must be built, nothing to lean on.
- Irreversible-effect classifier = static name list set BEFORE builtin runs
  (registry.go:441-453). Federation builtins must be added there.
- network.go:399 panic appears ALREADY FIXED (bounds check at network.go:337-
  346); stale citation in plan. Real open items: zero recover() in server/
  goroutines (process-killing panics), websocket InsecureSkipVerify Origin.
  Phase 0 should be rewritten around these two.

## Research results — barn-perms (arrived)
Full report: notes-fed-research-perms.md. Key facts:
- Critique #3 CONFIRMED in code: vm/op_verb.go:255 unconditionally sets
  ctx.Programmer = verb.Owner on every frame push; restored on pop. Principal-
  as-task-perms only governs top-level frames. All ~150-250 checks (not 345;
  plan's count wrong) compare live ctx.Programmer.
- move() delegates permission to db (`where:accept`) — no programmer check.
- Type tag 11 is FREE in both language and DB format → natural slot for a new
  proxy/federated type. Full add-a-type checklist enumerated in report (~40
  files reference WAIF/ANON; 9 heavily).
- vm/op_verb.go:81-111 startVerbCall is the single receiver-dispatch point,
  already switching on WAIF (dispatch to class with ":"-prefixed verb name,
  this=waif) and primitive protos ($int_proto etc). A TYPE_PROXY case slots in
  exactly here. Precedent complete for "non-OBJ value receiving verb calls."
- valid() hard-codes waifs invalid; OBJ/ANON share path. A proxy type would
  need its own ruling (probably: valid(prox) = E_TYPE or connection-liveness).
- anonObjects: side registry keyed by real allocated ids, resolver falls back
  to it; checkpoint assigns serialization ids > maxObj and rewrites refs.
  Precedent for proxy registry + checkpoint re-enlivening.
- Synthetic principal ObjIDs that don't name real objects: flag lookups fail
  soft to "not a wizard" — fail-closed shape holds for the frame it governs.

## Research results — toast-source (arrived)
Full report: notes-fed-research-toast.md. Key facts:
- WAIF dispatch: OP_CALL_VERB routes waif receiver to class object with ":"
  prefix; `this` = waif value; call_verb2 splits _this from recv — that split
  is what makes non-OBJ receivers possible. Barn mirrors this exactly.
- Type enum strictly append-only; TYPE_COMPLEX_FLAG in-memory only; Toast has
  ITER at 11 (not persisted/user-visible) — Barn left 11 free. A new tag slots
  at 15+ (or Barn's free 11 — but Toast interop argues for 15+, past BOOL 14).
- call_verb2 line 734: RUN_ACTIV.progr = db_verb_owner(h), unconditional —
  Toast confirms critique #3. eval() is the one frame push inheriting perms.
  set_task_perms sticky within frame, propagates into fork and suspend.
- Toast resource precedent: check_user_task_limit keyed by TASK PERMS (progr),
  not connected player — so per-principal quotas fall out naturally if inbound
  tasks run with Programmer=principal. Also MAX_QUEUED_OUTPUT (65536, floor
  2048) with oldest-line flush + "lines lost" report; INPUT_HIWAT/LOWAT
  backpressure; MAX_LINE_BYTES 5MB closes conn; ownership_quota; fg/bg ticks.
  Barn lacks most of these (see barn-tasks) — federation quota section should
  ALSO backfill queued_task_limit/max_queued_output as Toast-conformance work.

## Synthesis — critique verdicts against code
1,2,5: root cause confirmed. Fix via new value type + eventual-send-returns-
   promise semantics. WAIF/primitive-proto dispatch is complete precedent;
   type-tag slot free; add-a-type checklist in perms report.
3: confirmed (both Barn vm/op_verb.go:255 and Toast execute.cc:734). Rewrite
   security section honestly: boundary control + top-frame fail-closed only;
   deputy risk owned by facet authors; extend phase 3 exit test.
4: rule E_TYPE for raw OBJ in send args; explicit federation_export only.
6: confirmed worse than critique says: Barn has NO queued task limit and
   unbounded output buffers. Build per-principal quotas (Toast progr-keyed
   precedent), per-peer rate/inflight caps, bounded gift store, bounded logs.
7: split identity (key) from locator (hints list); add key rotation
   (successor cert signed by old key); sturdyref = key-id + swissnum + hints.
8: portals = pinned exports with keepalive refresh, distinct from leases.
9: drop causal hashes; per-session seqnums for idempotency; bounded local
   audit log as operator feature (fork for Q).
10: recommend cutting class imports to separate research workstream (fork).
11: resolve via possession≠ownership: visitors never own foreign objects;
    carrying = containment; acquisitions travel as refs. No building abroad v1.
12: couple systems: capability check AND identity/trust check conjunctive at
    delivery; ban ⇒ deny regardless of ref possession + revoke issued caps.
Minor: dialback = anti-spoof liveness only, session sig is authoritative;
    keep "virtual world" qualifier.

## DECISIONS LOCKED (Q, 2026-08-12/15)
1. E-style eventual sends: prox:verb()/prox.name return promises immediately;
   federation_await = only suspend; send-to-promise pipelines.
2. One new type TYPE_REMOTE (tag 15) for far refs + promises.
3. Raw OBJ in send args = E_TYPE; export explicit.
4. Seqnums + bounded local audit log; causal hashes dropped.
5. Class imports REPLACED by replica model (Q's key insight: versioned
   bundles are not a MOO concept; a parent is a live mutable object).
   Remote parent = single-writer replica, pull-to-advance, content-hash
   pins, fingerprint manifests, tri-state link check (green=proved identical,
   yellow=diverged+property-tests, red=absent), advance knob
   strict/optimistic/vendored. Refused: runtime origin fetch, version
   solver, auto transitive fetch. Migration problem dissolves (language
   already defines live propdef propagation); "who signs v8" dissolves
   (home signs its own edits).
6. Possession ≠ ownership abroad; no building abroad v1.
7. Conjunctive capability+trust checks; bans effective vs bearer refs.
8. Honest deputy story + deputy exit test in phase 3.
9. P6 availability principle; quotas keyed by principal-as-task-perms;
   Barn backfill of queued_task_limit/max_queued_output as conformance work.
10. Identity: key-id+swissnum, mutable hints, successor-cert rotation.
11. Portals pinned + keepalive, distinct from leases.
12. Dialback = stranger-tier liveness only; session sig authoritative.

## Deliverable status
plans/moo-federation.md REWRITTEN in full (2026-08-15) incorporating all of
the above.

## Adversary review results (plan-adversary, 2026-08-15)
Verdict: 11/14 critique points resolved; 3 HIGH, 9 MEDIUM, 8 LOW new findings.
HIGH: H1 quota-keying self-contradiction (frame perms reset ⇒ progr-keyed
quotas charge the wizard, not principal ⇒ need explicit accountable-principal
field on task); H2 quota backfill called prerequisite but unscheduled ⇒ add
to phase 0; H3 auto-advance for trusted tier contradicts pull-to-advance
rationale ⇒ remove auto-advance everywhere.
MEDIUM: M4 missing type rulings (Truthy/Len/value_bytes/index/json) +
remote_state absent from surface; M5 principal/visitor creation = unbounded
peer-driven id allocation ⇒ quota + reap; M6 create()/move() outliers unnamed
(create needs explicit principal check = enforcement for no-building-abroad;
move stays accept-delegated); M7 pipelining round-trip claim needs same-vat
caveat + exit test fix; M8 restart⇒broken needs positive mechanism (persist
flavor, inject E_REMOTE wake); M9 replica deltas unbounded ⇒ bound pending-
delta queue + per-peer ceilings; M10 rotation = ban shedding ⇒ bans bind to
successor-chain root; M11 isa lineage must follow chain too; M12 trust tiers
grant on identity alone vs P3 ⇒ tiers gate, never grant.
LOW: L13 wrong kernel/context.go cite; L14 owns-nothing wording; L15 flag-off
changes need honesty note; L16 tag-15 forward compat (waif_conversion
precedent); L17 audit surface = facet list + call closure; L18 await inherits
suspend's commit-failure-kills edge; L19 revoke needs 4-granularity surface;
L20 fairness exit test unpassable unless fed netlayer bypasses global
256-slot input queue.

## Fix state (applying now)
DONE: H1 (accountable-principal keying, both sites), H3 (auto-advance
removed everywhere), M4 (type rulings + remote_state in surface), M5
(identity-churn quota bullet), M6 (create/move outlier rulings + possession
enforcement pointer), M7 (same-vat caveat in surface), M8 (restart mechanism
stated), M9 (replica delta bounds), M10 (bans bind to chain root), M11 (isa
via successor-chain identity), M12 (tiers gate never grant), L13, L14, L15
(conformance bullet honesty note), L16 (tag-15 forward compat), L17, L18
(await commit-failure edge), L19 (4-granularity revoke), L20 (netlayer
isolation bullet).
DONE (final batch): H2 (phase 0 = Hygiene & backfill, quota conformance
tests Toast-green-first in exit), M7 exit test (same-vat chain + cross-vat
pays one extra trip), phase 6 sub-phased 6a/6b/6c with vendored mode
deferred-until-demanded.

ALL 20 ADVERSARY FINDINGS RESOLVED (3 high, 9 medium, 8 low) as of
2026-08-15.

## Adversary second pass: 16 RESOLVED, 4 PARTIAL, 10 NEW findings
N1 HIGH: H1 claim survived in P6 (94-97) + Verdict — FIXED (both now say
  shape-precedent-not-key / accountable-principal).
N2 HIGH fact error: I ruled TYPE_REMOTE "always truthy (like objects and
  waifs)" — BACKWARDS: objects/waifs/anons/errors are FALSY in both engines
  (types/value.go:138-141). FIXED: ruled falsy, consistent with references.
N3 MED-HIGH: create() check key unstated; frame-perms key = dead through
  deputy; accountable-principal key = breaks visit flow. FIXED: reads
  accountable principal; visitor/principal instantiation is server's own
  unstamped act, so check can't fire on visit flow.
N4: wall row 3 vs churn bullet — FIXED (remote *references* never enter
  object space; stand-ins quota'd).
N5: isolation vs shared-wss — FIXED (shares listener only; own upgrade
  path; never constructs player Connection/EnqueueInput).
N6: successor-chain trust inheritance + fork adjudication — IN PROGRESS
  (rule: first-seen-pins; same-generation fork freezes identity + operator
  alert; known/trusted tiers confirm successor acceptance out of band).
N7: wall row 4 deputy surface — FIXED (+ call closure).
N8: allocator cite 130-133→133-136 — FIXED.
N9: create() check missing from phase 3 scope/exit — TODO.
N10: "zero new rulings" → "zero new OBJ-builtin rulings" — FIXED.

N6 FIXED (first-seen-pins; same-generation fork freezes identity + operator
alert; known+ tiers require operator ack before honoring a successor —
"rotation stays routine; inheritance of trust gets the ceremony").
N9 FIXED (phase 3 scope gains accountable-principal stamp + create() check;
exit test gains (d): create() through deputy frame gets E_PERM).

ALL 30 FINDINGS ACROSS BOTH ADVERSARY PASSES RESOLVED (2026-08-15).
Document final pending Q review.

## Next steps
1. Collect researcher reports when notifications arrive.
2. Synthesize; confirm/refute each critique point against code facts.
3. Present decision list (forks + recommendations) to Q.
4. Rewrite `plans/moo-federation.md` per decisions; keep principles P1–P5 and
   phase structure where sound; add availability/threat-model section, identity
   redesign (key vs locator), portal lease policy, ban↔revocation coupling.

## Blockers
None — waiting on background agents.
