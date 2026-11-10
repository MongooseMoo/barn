# fix-jepsen-double-crash

## Goal
Find and fix why the Jepsen checkpoint harness issued two crash operations
against one already-dead Barn process generation (merge-queue Jepsen run
42862358939, `crash: RuntimeError: owned Barn identity changed; refusing to signal`).

## State
Cause established and fixed on branch fix/jepsen-double-crash, pushed to
origin; no PR yet. Q asked for the PR on 2026-11-10; the PreToolUse hook
refused `gh pr create` because the PR body cites run facts (crash time, 36 ms
gap, line numbers) that the parent session did not re-check itself in that
turn. Draft body: the parent session's scratchpad pr-jepsen-body.md. Test
scripts/test_mongoose_barn_process.py::test_startup_identity_is_checked_after_exec_finishes_writing_argv
drives the real OwnedBarnServer.start() and real verify_owned against a fake
/proc whose cmdline is empty right after Popen and full once the port listens.

Test output in WSL Debian (Python 3.11.2), fix temporarily reverted:

```
ERROR: test_startup_identity_is_checked_after_exec_finishes_writing_argv
  File ".../scripts/mongoose_barn_process.py", line 230, in start
    self.verify_owned()
  File ".../scripts/mongoose_barn_process.py", line 179, in verify_owned
    raise RuntimeError("owned Barn identity changed; refusing to signal")
Ran 24 tests in 0.202s
FAILED (errors=1)
```

With fix: `python3 -m unittest test_mongoose_barn_process test_jepsen_checkpoint`
-> `Ran 52 tests in 0.207s OK`.

## Findings (raw evidence, run 42862358939, artifact jepsen-evidence-a5c5e2e8...)

lifecycle.jsonl lines 25-31 (large fields elided):

```
25 {"event": "crash", "generation": 2, "result": {"exit_code": -9, "shutdown": "owned SIGKILL crash", ...}, "monotonic_ns": 846359584270260}
26 {"event": "admit", "generation": 3, "kind": "crash", "port": 33561, ...}
27 {"event": "pause", "generation": 3, "monotonic_ns": 846365023357401}
28 {"event": "resume", "generation": 3, "monotonic_ns": 846369025633208}
29 {"event": "crash", "generation": 3, "result": {"exit_code": -9, "shutdown": "owned SIGKILL crash", "forced": true, ...}, "monotonic_ns": 846374035457938}
30 {"event": "failure", "operation": "crash", "error": "crash: RuntimeError: owned Barn identity changed; refusing to signal", "monotonic_ns": 846374071438288}
31 {"event": "final-stop", "generation": 4, "result": {"exit_code": 0, "shutdown": "owned Barn PID stopped", "forced": false, "final_checkpoint": {"status": "verified", ...
```

1. The gap between lines 29 and 30 is 846374071438288 - 846374035457938 =
   35,980,350 ns = 36 ms, not 36 us as the brief said.
2. There is NO "admit" row for generation 4, yet gen-004/ exists and
   final-stop ran on generation 4.
3. Code path (scripts/jepsen_checkpoint.py): Campaign.crash (258-263) records
   the "crash" row only AFTER server.crash() returned, then calls
   self.launch("crash"). launch (213-240) increments generation to 4, sets
   self.server, calls server.start(), and only records "admit" after start()
   and the ADMIT evaluation succeed. fault (286-295) labels any exception with
   the fault name, so an exception in launch() is recorded as operation "crash".
4. So the "second crash" is not a second crash operation: it is the same crash
   fault failing in launch() of generation 4. The exception text comes from
   OwnedBarnServer.verify_owned (mongoose_barn_process.py:168-179); the only
   verify_owned call reachable 36 ms into launch is start() line 227, right
   after Popen + pidfd_open + process_state (214-217).
5. The gen-4 process was left running (start() line 227 is outside the
   cleanup try), fault() saw the failure, and finish() later called stop() on
   the same server object, whose verify_owned at line 365 then PASSED and the
   SIGTERM checkpoint verified (exit 0). So the identity mismatch was transient:
   true immediately after Popen, false later.

6. Jepsen side (store/.../jepsen.log) shows exactly ONE crash invocation
   for this failure, so the nemesis did not emit crash twice:

```
3331:INFO [2026-11-10 10:14:52,786] jepsen worker nemesis - jepsen.print :nemesis	:info	:crash	nil
3335:INFO [2026-11-10 10:14:52,831] jepsen worker nemesis - jepsen.print :nemesis	:info	:crash	nil	{:kind "lifecycle", :message "crash: RuntimeError: owned Barn identity changed; refusing to signal"}
```
   Later nemesis ops get {:kind "campaign-failed"} from fault() line 288-289.
   core.clj:115-123 emits one (nemesis-op :crash) per crashing cycle; no
   teardown crash exists.
7. gen-004/server.log first line is
   `time=2026-11-10T10:14:52.844 ... msg="Barn MOO Server"`, and the TCP
   listener at 10:14:52.899 -- both AFTER the failed crash reply at
   10:14:52.831. start() line 229 wait_port blocks until the port listens, so
   the verify_owned calls at lines 245/256 were not reachable before .831.
   Hence the failing call is start() line 227, before Barn's main() even logged.
   Which of the four sub-checks (exe, cwd, cmdline, start_ticks) was false is
   NOT recorded: verify_owned raises one message for all four.

8. Which sub-check: established by kernel source, runner kernel
   6.18.54.1-microsoft-standard-WSL2 (`wsl -d Debian uname -r`), CPython
   3.13.18 (job log line 395). Linux v6.18 fs/exec.c begin_new_exec():
   `set_mm_exe_file(bprm->mm, bprm->file)` then `exec_mmap(bprm->mm)` (which
   calls `exec_mm_release(tsk, old_mm)`, releasing a vfork parent) then
   `do_close_on_exec(me->files)` (which closes Popen's CLOEXEC error pipe,
   releasing a fork parent). Popen returns at that point. But
   fs/binfmt_elf.c load_elf_binary() calls begin_new_exec() BEFORE
   setup_arg_pages() (`mm->arg_start = bprm->p;`) and create_elf_tables()
   (`mm->arg_end = p;` and env_end). fs/proc/base.c get_mm_cmdline():
   `/* Check if process spawned far enough to have cmdline. */ if (!mm->env_end) return 0;`
   So for a short window after Popen returns, /proc/<pid>/cmdline is EMPTY,
   and verify_owned's `argv != command` comparison fails. The other three
   sub-checks cannot be transiently false then: exe is set before the parent
   is released, cwd is set by the child's chdir before exec, and start_ticks
   is the task start time fixed at fork. Under load (8 CI runners on one
   WSL distro) the window can outlast pidfd_open + two /proc reads.

## Decisions and why
- Fix: move the first verify_owned()/_ownership_established in start() from
  right after Popen to right after bench.wait_port(). Once Barn listens, its
  exec has completed, so the cmdline comparison is meaningful. verify_owned is
  unchanged, and it still runs before any signal (_signal_running, stop) and
  before readiness is accepted (line 256). Nothing between Popen and
  wait_port signals the process or trusts its identity (wait_port only polls
  proc.poll() and connects). No new loop, flag or retry added.
- Not a nemesis/Clojure change: the nemesis issued one crash.

## Surprises
- The brief's "two crash operations, 36 us apart" was wrong on both counts:
  one crash op, and the gap was 36 ms. The second row is fault()'s failure
  record for the same crash, raised in launch() of generation 4.
- start() line 227 failure leaves the child running with no cleanup; here
  finish() later stopped it cleanly. Not changed (out of scope).

## Unverified
- Which sub-check failed in CI is inferred from kernel source plus
  elimination, not observed: verify_owned's single message does not say.
- The fix has not run in the Jepsen workflow; only the unit test exercises it.

## Next step
Re-read the run 42862358939 evidence (lifecycle.jsonl rows 25-31,
jepsen.log lines 3331/3335) and the cited source lines, correct the PR body to
match, then open the PR against master once Q lifts the hook refusal, queue it
with `gh pr merge --auto`, and watch the Jepsen workflow.
