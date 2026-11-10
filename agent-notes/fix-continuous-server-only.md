# Continuous server-only discovery

## Goal

Restore continuous Barn discovery, prevent repeated cached-seed failure, and
exclude the workstation from every discovery job.

## State

Q authorized the presented repair plan with "Make it so!". Work is isolated
on fix/continuous-server-only. Primary checkout dirt is preserved. The existing
workstation task "Barn - Nightly fuzz" was disabled and read back as Disabled.

Failed run 42692025227 and controller 42715997349 are the live producers.
Native workers used mongoose-barn-fuzz-1 through -4. Routing and controller used
Q-barn-wsl-x64-05 and -04, respectively. The controller stopped on acceptance
failure, not a verified server crash. Group 4 currently allows only fuzz.yml
on master; its four runners were online and idle when inspected.

Downloaded failing-input and retained-corpus artifacts. Four exact saved inputs
are now regression corpus files. The original first campaign log is preserved
in scripts/fixtures/fuzz/native/42692025227-baseline.log. Focused replay on current
master passed: go test ./db/format -run with the four saved input names,
output "ok github.com/MongooseMoo/barn/db/format 1.731s".

Implementation complete, awaiting PR CI: remove legacy routing and workstation installer;
pin routing/controller to the dedicated group; disable input minimization in
the campaign so exact cached originals can be removed; extend the existing
retention owner with quarantine that preserves findings before removing only
identical disposable replay copies. A repeated finding stops promptly rather
than spinning. Acceptance and real mutation execution requirements stay intact.

## Decisions and why

The failed run found the database check 75 times during baseline replay and
never produced mutation execution statistics. Current master rejects/handles
all four saved cases, so no database implementation change is warranted.
The durable workflow fix must prevent a cached failing original being replayed
after its minimized descendant is moved aside. Preserve raw inputs and logs;
never relabel baseline coverage as mutation executions or bypass acceptance.

Every discovery job must use barn-bug-discovery. Its trusted master controller
must be added to the existing selected-workflow policy before activation.
Ordinary PR CI stays on its existing runners. No second scheduler is planned.
No promise of immunity to all future defects or provider outages is possible.

repository-ci skill read. Source read includes both workflows, the compute and
retention owners and tests, reader_fuzz_test.go, reader_vm.go, writer_vm.go,
task persistence owners and comparison, and docs/elle-isolation.md. No runtime
task/suspension implementation is being changed; PR checks include Elle.

## Surprises

Master has advanced since the failed run and already handles its saved database
inputs. The earlier legacy-overlap fix does not address cached baseline replay.
No existing actionlint/zizmor job was found; their verified current releases are
v1.7.12 and v1.32.0. Add focused workflow auditing through existing PR CI.

The edited compute and retention test files passed explicitly. Their raw output
ended with OK; quarantine reported removed_replay_copies=2 while preserving the
finding and unrelated cached record. The real baseline log still fails the
zero-mutation-work check, as required. git diff --check produced no output.
Three artifact inputs were already present on master; only the missing
6b8992e5372061cb input is a new regression corpus file.

PR 682 is open. Server group policy now admits exactly the trusted master
fuzz.yml and fuzz-controller.yml paths. Host barn-compute.service is active,
running, Restart=on-failure, NRestarts=0. All four guest listener services are
active/running. No workstation Debian Go fuzz commands were found.

First PR build job failed in test_raw_fixtures_are_not_tool_truncations: the new
native tee log was placed among GitHub-prefixed logs. Moved it into native/,
documented the producer formats, and extended the existing check recursively
with explicit native go-version/progress assertions. Existing GitHub-prefix
and truncation checks remain. Other PR checks were still running.

## Next step

Run the edited fixture tests, publish the fixture-format repair, watch exact-head
CI, fix red checks, normal merge, dispatch continuous native-only discovery, verify all
actual runner assignments and a completed cycle's successor. Update this note
with actual evidence before committing substantive work and before completion.
