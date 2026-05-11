# Barn/ToastStunt/Mongoose Convergence Workstream

Goal: make Barn and ToastStunt converge enough that both can run Mongoose, using
`../moo-conformance-tests` as the authority for durable behavior and using
Mongoose only to discover behavior that the suite does not cover yet.

Barn currently passes the existing conformance suite. That is the baseline, not
the finish line. The work is to use Mongoose to find missing ToastStunt behavior,
promote each real behavior into the conformance suite, and then make Barn pass
the expanded suite at a named matching profile.

## Control Surface

- `../moo-conformance-tests` is the conformance authority.
- `../mongoose/bridge` is read-only reference material for how an AI agent can
  log into a MOO. It is not the target implementation.
- Barn already has `cmd/moo_client`, a simple TCP MOO command client. Extend or
  wrap that existing command surface when a MOO command bridge is needed.
- Do not invent a separate transcript/artifact architecture as the work product.
  Normal process logs are acceptable diagnostics; durable progress is committed
  source, configuration, profile metadata, or conformance tests.
- Do not create a repo-local `.tmp` workstream surface. Managed conformance uses
  its own lifecycle and disposable database copies.
- Discovery is not proof. A Mongoose observation becomes actionable only after a
  smallest useful conformance test is added to `../moo-conformance-tests` and
  verified against ToastStunt.
- Barn source changes happen only after the same conformance test has passed on
  ToastStunt and failed on Barn at the matching profile, unless Barn already
  passes and the change is pure conformance coverage.

## Definition Of Profile

A profile is the named runtime identity used to decide whether two results are
comparable. It must include:

- implementation: `barn` or `toaststunt`
- implementation reference: commit/build identity and dirty tracked status
- executable command and platform
- database fixture: `Test.db`, Mongoose DB, or a reduced fixture
- configuration file or explicit option set
- feature map: option values, optional builtins, platform facts, extensions
- support status: `supported`, `unsupported`, or `diagnostic`

Unsupported or diagnostic profiles are visible debt. They are not passing
conformance, and they are not Barn semantic failures.

## Recoverable Prior Work

The branch `exp/barn-options-profile-workstream` contains useful profile work
that must be inspected and either cherry-picked or reimplemented cleanly:

- `8327538` added `config/options.go`, `config/parser.go`, and option tests.
- `ab0ebd4` wired runtime options through Barn execution.
- `9ff5357` added profile manifest emission.
- `fbf1155` added a managed Barn profile registry and profile config files.

Only recover profile/config/runtime-option work after review. Do not recover the
invented `cmd/mongoose_bridge` path, managed transcript artifacts, or `.tmp`
run layout from that branch.

## Execution Order

1. Baseline the repos.
   - Record Barn `master` commit, ToastStunt commit/build identity, and
     `../moo-conformance-tests` commit.
   - Verify Barn still passes the existing managed conformance command.
   - Read the conformance managed-server documentation and the current
     `cmd/moo_client` implementation before designing any command bridge.

2. Restore profile/config support.
   - Reintroduce Barn `.conf` parsing for Toast-sensitive runtime options.
   - Start with `OUTBOUND_NETWORK`, because it already exists in the lost branch
     and affects observable Toast-compatible behavior.
   - Reintroduce profile manifests and a registry that names supported,
     unsupported, and diagnostic Barn profiles.
   - Wire options through Barn execution so the profile changes actual behavior,
     not only metadata.
   - Commit this slice atomically with focused tests.

3. Teach the conformance harness about profiles only where needed.
   - Use `../moo-conformance-tests` managed server mode for lifecycle.
   - Add profile metadata checks, feature requirements, or matrix expectations
     only when a real Toast-verified test requires them.
   - Unknown or mismatched feature metadata must fail closed instead of silently
     comparing unlike profiles.
   - Commit conformance-suite changes in `../moo-conformance-tests`, not in
     Barn.

4. Build the MOO command bridge on the existing Barn command surface.
   - Inspect and extend `cmd/moo_client` or add an adjacent command that shares
     its purpose: connect to a MOO over TCP, log in, send commands, and print
     tagged output.
   - Support Barn-only, Toast-only, and send-both command execution.
   - Support local uncommitted credentials/config for Mongoose login; never
     commit secrets.
   - Preserve enough raw output for debugging in process output or normal logs,
     but do not make generated transcripts the work product.
   - The bridge is for discovery and operator control. It is not a replacement
     for conformance tests.

5. Use Mongoose to discover uncovered behavior.
   - Run the same MOO commands against Barn/Mongoose and ToastStunt/Mongoose
     profiles using disposable database copies.
   - Start with simple, high-signal commands: login, character selection,
     `look`, movement, `@who`, `@display`, `@props`, `@verbs`, parser shortcuts,
     task-visible commands, and persistence-sensitive workflows.
   - Classify differences as server behavior, database content, profile
     mismatch, platform behavior, optional feature behavior, or harness/client
     behavior.
   - A difference is only a candidate until reduced to a conformance test.

6. Promote each real candidate into `../moo-conformance-tests`.
   - Write the smallest YAML test or fixture-backed test that expresses the
     ToastStunt behavior.
   - Prefer `Test.db` or a reduced fixture. Use a Mongoose fixture only when the
     behavior cannot be reduced.
   - Run the new test against ToastStunt first through the managed conformance
     command and record the exact profile.
   - Run the unchanged test against Barn at the matching profile.
   - If Barn passes, keep the test as coverage. If Barn fails, move to the Barn
     fix loop.

7. Fix Barn only from Toast-pass/Barn-fail evidence.
   - Read the failing conformance test and profile metadata.
   - Make the smallest Barn production change for that behavior.
   - Run the focused Barn test, then the relevant managed conformance suite.
   - Commit the Barn fix atomically and reference the conformance behavior it
     closes.

8. Repeat by behavior family.
   - Work one family at a time: parser, command output, object inspection,
     properties/verbs, task scheduling, networking/listeners, persistence, then
     Mongoose-specific reduced fixtures.
   - Do not switch families just because setup or diagnostics are interesting.
   - A slice ends with a kept conformance test, a kept Barn fix, an explicit
     unsupported profile record, or a documented invalid comparison.

## Done Criteria

- Barn has committed profile/config support for the Toast-sensitive options
  needed by the expanded suite.
- The conformance suite can express the profile requirements needed for those
  tests.
- The existing MOO command client surface can drive Barn and ToastStunt against
  Mongoose well enough to discover candidate differences.
- Every accepted Mongoose-discovered behavior has been promoted into
  `../moo-conformance-tests` or closed as database content, profile mismatch,
  unsupported profile, diagnostic-only behavior, or invalid comparison.
- Barn passes the expanded managed conformance suite for every supported
  matching profile.
