# Bare-metal / unikernel Barn — session notes (2026-10-09)

Q asked whether Barn can run on bare metal with no OS. Two experiments dispatched as subagents.

## Test 1: Nanos unikernel via OPS (agent `nanos-test`) — DONE

Result: Barn ran unmodified as a Nanos unikernel under QEMU (TCG emulation; no KVM at the time).

Verified by me against primary sources:
- Console log `%TEMP%\claude\C--Users-Q-code-barn\e50a945f-...\tasks\b2cgotqcs.output`: TCG fallback, `Barn MOO Server`,
  `database loaded version=4 objects=8`, `listening ... port=17777`, two `new connection` + `switched connection`
  (players #8, #9), two `CHECKPOINTING: dump_database() requested` + `checkpoint complete` (282 ms, 59 ms),
  `initiating shutdown`, `shutdown complete`, `[exited with code 0]`.
- Image `barn-nanos-exp` (`ops image tree/cat`): `/Test.db.new` 2377 bytes, v17 header, `nanos_probe` at line 48;
  `/logs/latest.jsonl` 38 lines.
- No qemu left running, nothing listening on 17777/17778, no tracked barn file changed,
  original Test.db SHA-256 `1a3f23eb...d408e` unchanged.
- Versions: OPS 0.1.47, Nanos 0.1.56, QEMU 7.2.22, Go 1.27.1, Barn 8fbf3ab,
  binary SHA-256 `815740a6...b94d`.

Agent-reported only (client transcripts not saved, not verified by me):
- evals returned (`1+2` -> `{1, 3}`, `server_version()`, `time()`, `usage()`).
- An eval containing `memory_usage()` returned E_FILE. Plausible cause: `builtins/resource_stats_linux.go:34`
  opens `/proc/self/statm`; the image's `/proc` has only `self/exe` and `sys/kernel/hostname` (I confirmed both facts,
  but the builtin was not isolated and not compared with Toast).
- 2 CPUs seen via two scheduler worker goroutines in the goroutine profile.

Not tested: signals, `os/exec`, sqlite, fileio, outbound network, load, Mongoose db, reboot from `Test.db.new`, KVM.

Scratch: `...\scratchpad\nanos\` (config.json, binary copy, db copy).
Left in WSL: apt `qemu-system-x86`, `/root/.ops/bin/ops`, Nanos 0.1.56, image `/root/.ops/images/barn-nanos-exp` (200 MB).
A 128 MB `/root/.ops/images/barn` dated Jun 24 pre-existed (earlier attempt, untouched).

## Spike 2: TamaGo `GOOS=tamago GOARCH=amd64` (agent `tamago-spike`) — RUNNING

Worktree `C:/Users/Q/code/barn-spike-tamago`, branch `spike/tamago-bare-metal` from master 8fbf3ab. Local only: no push, no PR.

Verified by me: commit `330ba25` exists, 5 files (builtins/sqlite.go, sqlite_driver.go, sqlite_driver_tamago.go,
cmd/barn/panic_exit_tamago.go, panic_exit_unix.go).

Agent-reported, NOT yet verified:
- Toolchain tamago-go1.27.1 at WSL `/opt/tamago-go-1.27.1/usr/local/tamago-go`.
- Unmodified build failed only on `modernc.org/libc` (no tamago files) and then
  `cmd/barn/panic_exit_unix.go:14:14: undefined: syscall.Exec`. After two stubs every package builds for tamago.
- Bare-metal image (board/qemu/microvm, Test.db embedded) links at 21.8 MB; Go runtime starts under QEMU TCG, then
  `panic: could not set system timer` at kvm/pvclock/pvclock.go:131 (board wants invariant TSC or KVM clock).
- Agent then began a TCG-only board copy (`internal/tamagoboard/microvmtcg`) and virtio/go-net wiring
  (`cmd/barn/net_microvm.go`, `boot_microvm.go`); uncommitted when last seen.

Still owed by the agent: the four researched facts with URLs (toolchain upstream status, amd64 SMP, net, storage),
path to raw unmodified build output, per-package table, console output under KVM.

### Spike 2 final report (TCG only) — verified by me 2026-10-10

- Three commits on 8fbf3ab: 330ba25 (stubs), 7af9e43 (boot image), 4e83c36 (network). Worktree clean.
  Own diff `8fbf3ab..HEAD`: 14 files, +597/-18. (`master..HEAD` looks huge only because master moved to 53754a0.)
- Host `go build ./...` exit 0, `go vet` on builtins/cmd/barn/internal exit 0, gofmt clean (I ran these).
- `01-unmodified-cmd-barn.txt`: only failure is `modernc.org/libc/*: build constraints exclude all Go files` x8.
- `03-qemu-console.txt`: stock board panics `could not set system timer` (pvclock.go:131) under TCG;
  spike board boots, `database loaded`; `-smp 4` prints `NumCPU=1 GOMAXPROCS=4`.
- `04-qemu-network.txt`: `-smp 1` + virtio-net: `listening`, `new connection`, `switched connection`,
  client got `*** Connected ***` and `{1, {3, "0.0.0-dev+unknown", 1}}`.
  `-smp 4` + virtio-net: stalls after first log line (60 s). `-smp 4` without virtio: loads db.
  `-smp 2` + virtio: console not captured, agent inferred stall.
  `0.0.0.0:7777` bind -> `bad local address`. Clock starts at 1970.
- Web facts checked: golang/go#73608 "proposal: all: add bare metal support" OPEN, label Proposal,
  aclements 2026-09-16 "Overall seems to be in fairly good shape"; tamago-go1.27.1 released 2026-09-02,
  1.27.2 on 2026-10-09; Compatibility wiki: "On multi-core platforms (currently only amd64)",
  "Asynchronous preemption for Goroutines is not supported".
- NOT checked by me: FAQ quotes, go-net README, Internals wiki RAM-disk quote, per-package clean list,
  the `-eval` parity claim, toolchain tarball digest.
- go.mod was regrouped by `go get` and gained tamago, go-net, gvisor, lneto. Agent used shell `cp` twice (rule breach, self-reported).
- Asked agent for one more round: stock board under KVM, -smp 1/4 with/without virtio-net -> `05-qemu-kvm.txt`.

### Spike 2 KVM round — 2026-10-10

Branch now at 27e466a (4 commits on 8fbf3ab, 15 files +671/-18, worktree clean, host build exit 0, gofmt clean — I ran these).
Log: `scratchpad\tamago\05-kvm-console.txt` (78 lines; run tallies in it are the agent's hand-written summaries, not per-run raw console).

In the log (read by me):
- Stock tamago board under KVM, -smp 1: `database loaded` at 0.093 s; with virtio-net: `listening`, `new connection`,
  client got `*** Connected ***`, `{1, {3, "0.0.0-dev+unknown", 1}}`, `{1, {500500, 16.274449148}}`.
- SMP probe: smp 2 -> 1 worker 1.48 s, 2 at once 1.57 s, APIC IDs [1 0]; smp 3 -> 1.66 s / 1.69 s, APIC IDs [2 0 1].
- Tallies: Barn image reaching `database loaded`: smp 2 10/10, smp 3 10/10, smp 4 6/12, smp 8 2/2.
  Networked image reaching `listening`: smp 2 2/3, smp 3 0/1. Probe image hung at smp 4 (2 runs) and smp 8 (1 run).
- Barn-free control (`scratchpad\tamago\hello\main.go`, I read it: only tamago board import + alloc/goroutine/GC stress):
  one-line hello 8/8 at smp 1-4; alloc stress smp 1 4/4, smp 2 0/8, smp 4 0/8, smp 8 0/8 on tamago 1.27.1;
  on 1.27.2: smp 1 8/8, smp 2 0/8, smp 4 0/8.
- Conclusion (agent's, consistent with the log): SMP + allocation/GC hangs without any Barn code. Unknown whether that is
  nested KVM on WSL2/Hyper-V (Ryzen 5950X) or tamago SMP generally. Not debugged further.

WSL now also has `/opt/tamago-go-1.27.2`. No qemu running; kvm modules still loaded.

### Spike 2 stock-board KVM round (second report) — 2026-10-10

Log: `scratchpad\tamago\05-qemu-kvm.txt` (81 lines). Commit 4e83c36, `-tags microvm`, no code changes.
- smp 1 + virtio-net: `listening`; client got `*** Connected ***`, `{1, {3, "0.0.0-dev+unknown", 1}}`, `{1, 500500}`.
- smp 4 without virtio-net: 6/12 reach `database loaded` (agent tally). smp 2 and 3: 10/10 each.
- smp 4 + virtio-net: stalled 4/4. QEMU monitor `info registers -a`, two samples per run:
  CPUs are halted in `tamago/amd64.wfi` (irq.s:78) or `amd64.apstart` (smp.s:163), or spinning in
  `runtime.findRunnable` / `runtime.beforeIdle` (idle_tamago.go:26) / `read_tsc`. No sample in Barn, gVisor or go-net.
  I re-ran `go tool addr2line` on `/tmp/tamago-out/barn-kvm` for six of the addresses: all resolve as the agent said.
- So the stall reproduces on the stock board under KVM: not TCG, not the board copy. Nested-KVM vs tamago SMP still unknown.
- Both agents done; nothing running.

## KVM in WSL

Q approved. I ran `modprobe kvm_amd` (exit 0): `/dev/kvm` exists, kvm_amd + kvm loaded, nested virtualization enabled.
Undo: `modprobe -r kvm_amd` or restart WSL. Agent told to use stock microvm board under KVM, not to unload the module.

## Tooling defect found

`~/.claude/settings.json:364` registers `claudio.exe` as a `WorktreeCreate` hook. That hook must print a worktree path;
claudio prints nothing, so Agent `isolation: worktree` fails ("hook succeeded but returned no worktree path").
Not changed. Asked Q whether to remove the entry; unanswered. Worked by creating the worktree with `git worktree add`.

## Next steps

1. When `tamago-spike` reports: verify build output, diff, and console logs before relaying anything.
2. Optional: rerun Test 1 under KVM now that `/dev/kvm` exists (needs Q's go-ahead; it is another manual launch).
3. Q's decisions outstanding: Claudio `WorktreeCreate` hook; what to do with the spike branch/worktree and the WSL leftovers.
