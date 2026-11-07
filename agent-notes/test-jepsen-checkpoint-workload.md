# Jepsen checkpoint workload

## Goal
Run real concurrent Barn clients through an actual pinned Jepsen workload and
process faults against one disposable server. Check only the explicit completed
checkpoint durability boundary; retain native outcomes, histories and replay data.

## State
Implemented and committed locally on test/jepsen-checkpoint-workload; not
pushed (the coordinator owns push/PR/merge; PR body drafted at .tmp/pr-body.md).
Pure-fixture tests pass (WSL python3 3.11: test_jepsen_checkpoint 28,
test_mongoose_barn_process 23, test_mongoose_gameplay_runner 27, all OK;
Windows py -3: test_jepsen_checkpoint 28 OK). The Clojure project compiles and
its uberjar ran against AdapterServer with an in-memory MOO stand-in (scratch
smoke, no Barn). The real Jepsen-against-Barn campaign has never run: its first
execution is the `Checkpoint durability campaign` job in
.github/workflows/jepsen.yml on the PR. Unverified until then: Test.db login
through SocketTransport under concurrency, Barn's replies to the generated MOO,
checkpoint pair verification on Test.db, Elle CLI acceptance of the exported
EDN, and the 20-minute bound.

Pre-implementation history (superseded): the design below was traced before
the lifecycle-owner boundary was settled by the coordinator.

2026-11-07 (implementer): coordinator settled the owner boundary: extend
scripts/mongoose_barn_process.py OwnedBarnServer with a registered public Test.db
profile and owned pause/resume/crash methods; no second process owner and no
ManagedServer change. Implementation started on test/jepsen-checkpoint-workload.

Implementation findings (read before building):
- profiles/barn/profiles.json already registers barn-linux-testdb-outbound-on
  (outbound-on.conf, expected_features OUTBOUND_NETWORK + open_network_connection).
  OwnedBarnServer hardcodes PROFILE_ID and checks two option features; the
  extension is a profile_id parameter whose expected features come from the
  registry entry (same rule as profile/registry.go:123 Validate).
- Barn loads `<db>.waifids` beside `--db` and requires its header to equal the
  database SHA-256 (db/format/waif_identity_sidecar.go:54-80); a generation
  launched from `gen/input.db` + `gen/input.db.waifids` copies of a verified
  pair therefore loads that exact pair.
- Jepsen 0.3.15 has no JSON library (project.clj deps); the Clojure side adds
  pinned org.clojure/data.json for the adapter protocol and history export.
  Python does generation splitting, EDN export, Elle invocation, durability.
- CI `go` job runs `python3 -m unittest discover -s scripts -p 'test_*.py'`
  without moo-conformance-tests, so the adapter imports SocketTransport lazily.
- Owner extension done: `profile_id=` keyword (default unchanged Mongoose),
  registry-driven expected features, `pause()`/`resume()`/`crash()` signalling
  only the verified pidfd. scripts/test_mongoose_barn_process.py: 23 tests OK
  in WSL Debian python3 3.11 (pidfd/SIGSTOP do not exist on Windows Python).
- Read io.jepsen/generator 0.1.6 jar source: phases (synchronizes), clients,
  nemesis, cycle, sleep, time-limit, stagger (= stagger-exp, total rate), once;
  jepsen.random/with-seed binds only jepsen.random. Elle's la/gen (elle 0.2.8
  list_append.clj:1000, txn.clj wr-txns) uses its own randomness; writes are
  unique per key across the whole run, so (key, value) is a durable token.
- SocketTransport._receive (transport.py:521-560) returns partial/None on EOF;
  the recording override detects EOF so a closed socket is :info, never :ok.
- Written (untested yet): tests/jepsen-checkpoint/{project.clj,src/barn_checkpoint/core.clj}
  (Client + AdapterNemesis via nemesis/compose, phases: clients+pause/resume,
  resume, checkpoint, clients+pause/resume/crash, resume, checkpoint; history
  exported as history.jsonl) and scripts/jepsen_checkpoint.py (Campaign owner,
  ClientSession, AdapterServer, split_generations, elle_history EDN,
  durability_violations, analyze with check_elle.run/verdict reuse).
  Design detail: a native MOO error or malformed reply is :info and fails the
  campaign (the workload code is fixed, so any native error is a finding);
  fault-window timeouts/disconnects are :info and only counted.

## Decisions and why
- Reuse managed server and checkpoint subprocess owners; no generic framework.
- Pin Jepsen v0.3.15, bound its JVM to 2 GiB, and keep the pinned Elle checker.
- A command acknowledgment is not a checkpoint durability acknowledgment.
- Disconnects and fault-window timeouts remain unknown outcomes until resolved.
- Use primary Jepsen source/docs and paper-reader only for any new literature.
- Parent owns CI/publication/merge; no SSH, deployment or posting by this worker.

## Surprises
The conformance ManagedServer restart adopts only the portable database file,
not Barn's identity sidecar. Its process owner cannot establish the required
database/WAIF-pair boundary unchanged. The existing mongoose_barn_process owner
already checks pidfd, start ticks, command/cwd identity and a freshly published
intact database/sidecar pair, but hardcodes the Mongoose runtime profile. Root was
asked whether the minimal registered Test.db profile extension and explicit owned
pause/resume/crash methods belong in this task, rather than a companion managed
conformance owner change. No implementation is being built on that unsettled owner.

2026-11-07 smoke (scratch, no Barn): the real uberjar against AdapterServer
with an in-memory MOO stand-in produced 1668 history ops over 3 generations,
all 16 nemesis ops, and split cleanly. It exposed that each `la/gen` call
restarts keys and values, so phase 1 and phase 3 reused (key, value) tokens and
the durability check reported duplicates. Fixed by giving each client phase a
disjoint key offset (gen/map). Local compile needed a scratch Temurin 21 JDK in
WSL /tmp: Jepsen's bifurcan dependency is class file 65 (Java 21); WSL has 17.

One read-only rg invocation used an invalid wildcard Windows path and failed;
subsequent source searches use the verified directory with --glob. The prior
campaign already identified the need for a parent-owned command guard.

## Precise workload and oracle plan

- Primary APIs read: Jepsen v0.3.15 project/client/nemesis/core/store and its
  core_test list-append example; io.jepsen/generator 0.1.6 API documentation.
  Pin released [jepsen "0.3.15"] and project JVM -Xmx2g rather than upstream's
  -Xmx32g. Use dummy SSH and one local logical node, no remote host actions.
- Small Clojure project under test tooling implements Jepsen Client and Nemesis,
  gen/phases, gen/clients, gen/nemesis, gen/stagger, gen/time-limit and nemesis/compose.
  Jepsen owns scheduling, concurrent histories and seed binding. Python is solely
  the lifecycle/MOO transport adapter because those owners already exist here.
- Read the real MCT SocketTransport and public Test.db login implementation:
  connect wizard creates a fresh wizard player for each connection. Concurrent
  clients therefore do not replace a shared login. Reuse SocketTransport parser
  and preserve its raw response via a small recording override, not a new parser.
- Each successful native transaction appends a unique token to a per-boot append
  row and a separate persistent ledger in one MOO invocation, and returns the row
  observations. Use bounded keys/operations so native responses stay small.
  Native MOO errors retain code/message/raw response; disconnect/timeout outcomes
  are :info. Error/unknown counts are explicit and cannot yield an unqualified
  healthy-workload pass. Never infer that a native exception rolled back writes.
- Phase 1: concurrent real client append/read operations with pause/resume faults;
  no crash yet. Phase barrier waits for outstanding operations to drain.
- Phase 2: a nemesis checkpoint operation reads the ledger, performs the owned
  graceful signal checkpoint, requires the existing intact-pair verification,
  and restarts from that exact pair. The recorded frozen ledger is the durability
  baseline. A command reply or dump_database request alone is not that boundary.
- Phase 3: concurrent appends/reads with composed pause/resume and crash/restart
  nemeses. Restart always uses the most recent explicitly verified pair. The
  acknowledged suffix after that checkpoint may disappear legitimately. Retain
  every unknown fault-window operation, generation and native result.
- Reset only the per-boot checker rows when a new boot is admitted; never reset
  the durable ledger. Check append histories separately per process generation,
  so Elle is never asked to treat uncheckpointed command replies as durable.
  This also avoids inventing initialization writes for recovered ledger tokens.
  Export real per-generation list-append EDN observations for the established
  pinned Elle CLI with serializable explicitly selected. Do not replace it with
  Jepsen's transitive Elle version. Zero comparisons are an incomplete campaign.
- After healing, all checkpoint-baseline ledger prefixes must survive. Additional
  pre-crash suffix tokens are permitted. Preserve pair hashes, boot/log manifests,
  raw native outcomes, Jepsen history/results, each Elle input/verdict/witness,
  seed/config/source identity, and concrete rerun/analyze commands.
- Tests first exercise adapter parsing/outcome categorization, durable-prefix
  corruption rejection, generation splitting, unknown outcomes and lifecycle
  ownership using pure fixtures. The actual Jepsen/Barn run is a separately
  requested CI invocation; no production or manually improvised MOO process.

No new MOO conformance assertion is needed for the documented Barn checkpoint
contract. If unknown semantics emerge, use managed pinned WSL Toast plus admission
before changing runtime behavior. Production server code is outside this track.

## Decisions made while implementing
- The pinned elle-cli build moved from elle.yml into the shared composite
  action .github/actions/elle-cli (same commit, jar and invocation), used by
  elle.yml and jepsen.yml, so the pin exists once. Composite steps cannot set
  timeout-minutes, so the 3-minute apt bound is `timeout 180` around both apt
  commands.
- The campaign runs the uberjar with `java -Xmx2g -jar` from the evidence
  directory, so Jepsen's store/ lands in the evidence.
- jepsen.yml mirrors elle.yml's triggers and tested-tree skip/record.

## Next step
Coordinator: push, open the PR, and read the first `Checkpoint durability
campaign` run (artifact jepsen-evidence-<sha>: results.json, lifecycle.jsonl,
history.jsonl, gen-*/server.log). A red first run is diagnosed from that
evidence before any change; a native MOO error there is a finding, not a test
to loosen.
