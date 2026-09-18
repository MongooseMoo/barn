# Adversarial second round: make the gate-aware recommendation concrete

Continue your existing Fable session and read your completed first report at
reports/fable-scheduler-gate-20260918.md. Own ONLY the new file
reports/fable-scheduler-gate-followup-20260918.md. Keep the first report intact.
No runtime edits, conformance runs, server restarts, databases or credentials.

We agree the gate must be part of the policy. Now challenge the design with
specific implementation traps and choose a concrete first deliverable:

1. Fair worker admission and fair gate admission can create two unrelated
   priority orders. Describe who owns the next dispatch decision and who owns
   the exclusive grant. What prevents handing the gate to a task that cannot
   obtain execution capacity? Does the currently active holder always retain a
   way to finish? Give a step-by-step schedule, including direct connection
   lanes and ordinary shared committers, not just scheduler workers.
2. A resumed task currently takes exclusive access before executing ANY opcode,
   even if that particular slice would be read-only. Do NOT simply remove it:
   demonstrate how future writes, irreversible effects, and no retry snapshot
   make that unsafe. Is early exclusive admission the right first implementation
   until a separate continuation/retry design exists?
3. An effect boundary is inside a Go builtin call. VM builtin operands/arguments
   have already been consumed (vm/op_misc.go), and FlowSuspend supplies a return
   value and advances execution. The current bool hook means retry abort or
   proceed. If recommending parking here, specify continuation state, exactly-once
   effects, read-set revalidation, and safe GC roots. Is a parked goroutine with
   a retained physical execution lease a defensible bounded intermediate design?
   Would it prevent GC indefinitely? Which option is smallest and actually safe?
4. Go RWMutex writer preference can block new shared committers behind an
   exclusive waiter. A fair arbiter cannot govern only exclusive Lock calls
   while letting untracked shared commits/checkpoints bypass it. Give a migration
   boundary that accounts for every access without a half-migrated fairness claim.
5. After first effect, A owns the gate and calls slow curl. A queued login B has
   a 50ms SOFT target. Explain why donation/weights cannot meet that target; what
   must change and how it matches Toast's actual existing async builtin semantics.
   Parent source check: Toast src/curl.cc:91 uses background_thread; Barn
   builtins/curl.go:51 calls client.Do synchronously. Do not claim this caused our
   observed Mongoose delay without a trace. Include plain long MOO computation
   after an effect, where async I/O fixes would not help.
6. Retryability for scheduling differs from runtime retryability: first-run
   forks remain solo batches but forkFirstRunRebuilder permits internal retry.
   Separate current production concurrency from hypothetical all-worker blocking.
7. Model cancellation at wait/grant/start and nested engine calls under an
   inherited lease. Include a checkpoint or GC request waiting behind a holder.
   State whether quotas count waiting, speculative retries, execution occupancy,
   and gate occupancy; priority donation must not confer free future service.

End with a decision table: safe now / needs representation work / changes MOO
semantics. Recommend ONE coherent first slice, with owned components and exit
criteria, followed by at most three subsequent phases. Identify disagreements
with our first proposal. Be willing to recommend a simpler serialized scheduling
lane if the resource model supports it; also say what throughput it sacrifices.

The user wants a design discussion, not premature implementation. Return a
source-grounded recommendation with residual limits, not a list of possibilities.
Use the source context already gathered; do not repeat the broad first-round
audit. Keep this second report to roughly 1,500 words, rereading only a specific
source seam needed to resolve a disagreement.
