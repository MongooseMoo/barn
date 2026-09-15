#!/usr/bin/env python3
"""Multi-player closed-loop TCP benchmark: Toast vs Barn on the real Mongoose db.

Issue #265. Drives N logged-in TCP connections through the same weighted
command mix the in-process Barn harness uses (engine/mongoose_real_bench_test.go:
look 35 / say 30 / i 10 / @who 10 / home 15), closed-loop per connection
(each connection waits for its command to finish before sending the next),
2 s warm-up then 8 s measurement per player level, and reports goodput/s,
p50, p99, p99.9 and max latency. The same code path targets either engine so
the two numbers are apples to apples.

Engines
-------
toast  The pinned WSL Mongoose ToastStunt oracle
       (/root/src/toaststunt-mongoose/build-release/moo, PROMOTE_NUMBERS build),
       launched through the tracked wrapper scripts/run_toast_wsl.sh exactly as
       plans/barn-toast-mongoose-convergence-workstreams.md and
       scripts/benchmark-mongoose.ps1 do. Clients dial the WSL NAT IP
       (`wsl hostname -I`); localhost forwarding into WSL is unreliable.
barn   A Windows barn.exe (build with `go build -o barn.exe ./cmd/barn/`)
       started with the Mongoose profile config (PROMOTE_NUMBERS=1,
       OUTBOUND_NETWORK=1), listening on 127.0.0.1.

Both engines run on a DISPOSABLE copy of the fixture inside the run directory;
the source fixture is never opened by a server. Path, size and SHA-256 of the
source and of the copy are recorded in run.json. The default port is 7777 =
$network.port so that #0:server_started sees the production shape ($prod()
is "#0 in listeners($network.port)") and activates $sql_utils, exactly like
the in-process harness's stub listener does.

Login (the hard part)
---------------------
Mongoose players have unknown passwords, and the login is a read()-driven
account conversation ($account_login:login). This script needs ONE real
wizard login, supplied through the environment variable MONGOOSE_LOGIN_SCRIPT
(same contract as scripts/benchmark-mongoose.ps1: three newline-separated
lines -- the trusted-proxy prelude with a literal {port}, the account
username, the password; credentials are never written to disk by this
script). With that control connection it mutates the DISPOSABLE copy:

  * computes the roster the Barn harness would pick: players() whose
    .location is > #0, sorted ascending, first N;
  * creates one fresh account per roster player through the database's own
    $account_manager:create_account (argon2 password), sets the account's
    players/default_player to that one player and marks the email verified,
    so `benchp<i>` + password logs straight into player i with no menu;
  * creates a `benchctl` account bound to a wizard OUTSIDE the roster prefix,
    used to re-verify connected_players() after the bench logins;
  * optionally (default on) applies the same two one-line wizard repairs the
    Barn harness applies to this snapshot (re-point #2585.sql at the open
    $sql_utils sound database + create its `assets` table; add #36.cloaked),
    so `say` and `@who` do not become a traceback storm on either engine.
    --no-repair measures the dump as-is.

Trusted-proxy prelude (--proxy):
  immediate  (default) the PROXY line is the first thing on the wire, as a
             real proxy would send it. Verified on the Toast oracle from
             inside WSL (127.0.0.1 is in $server_options.trusted_proxies) and
             on Barn on 127.0.0.1. On an UNTRUSTED connection (Toast reached
             from Windows via the WSL NAT gateway) the server hands the line
             to read() as a bogus username, which costs one "Access Denied";
             the client tolerates exactly one such denial when it sent PROXY.
  auto       wait --banner-wait for the username prompt; send PROXY only if
             none appears (trusted servers ignore the connect-time blank).
             NOTE: Barn currently breaks on a PROXY line that arrives after a
             delay on a trusted connection (#1414:_read -> read() E_INVARG);
             Toast accepts it. Recorded as a Barn delta, so `auto` is not the
             default.
  never      no PROXY line (untrusted connections only).

Per-command completion uses the server-intrinsic OUTPUTPREFIX/OUTPUTSUFFIX
bracket, which both engines emit around every command line regardless of the
verb's own output, so the round trip is timed from write to suffix echo.

Usage
-----
  set MONGOOSE_LOGIN_SCRIPT (3 lines) in the environment, then

  python scripts/bench_players.py --engine toast --players 1,16
  python scripts/bench_players.py --engine barn  --players 1,16 --barn-exe barn.exe
  python scripts/bench_players.py --engine toast --players 1 --warmup 1 --measure 3   # smoke

Outputs <out>/run.json, <out>/report.md, <out>/server.log; default --out is
.tmp/bench_players/<engine>-<UTC timestamp>/ under the repo root.
Stdlib only.
"""

from __future__ import annotations

import argparse
import datetime as _dt
import hashlib
import json
import os
import random
import re
import shutil
import socket
import statistics
import subprocess
import sys
import threading
import time
from dataclasses import dataclass, field
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
DEFAULT_DB = REPO_ROOT / "mongoose.db.new"
TOAST_MOO = "/root/src/toaststunt-mongoose/build-release/moo"
TOAST_SRC = "/root/src/toaststunt-mongoose"
TOAST_WRAPPER_WSL = "/mnt/c/Users/Q/code/barn/scripts/run_toast_wsl.sh"
WSL_DISTRO = "Debian"
BARN_CONFIG = REPO_ROOT / "profiles" / "barn" / "mongoose-outbound-on.conf"
BARN_PROFILE_ID = "barn-windows-mongoose-outbound-on"

# Same mix and seeds as engine/mongoose_real_bench_test.go.
SHAPES: list[tuple[str, str, int]] = [
    ("look", "look", 35),
    ("say", "say Hello there, this is a benchmark message!", 30),
    ("inventory", "i", 10),
    ("who", "@who", 10),
    ("home", "home", 15),
]
SEED_BASE = 0xBEEF

PREFIX_TAG = "===BP-PREFIX==="
SUFFIX_TAG = "===BP-SUFFIX==="

# Mongoose $account_login prompts (read from the fixture; see run notes).
USERNAME_PROMPT = "Enter your username or email:"
PASSWORD_PROMPT = "Password"
LOGGED_IN_MSG = "Welcome!"
ACCESS_DENIED = "Access Denied"

BENCH_PASSWORD = "bench-pass-265"
TRACEBACK_RE = re.compile(r"^#-?\d+:\S.*\(this == #-?\d+\), line \d+")


def utc_stamp() -> str:
    return _dt.datetime.now(_dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ")


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def log(msg: str) -> None:
    print(f"[{time.strftime('%H:%M:%S')}] {msg}", flush=True)


# --------------------------------------------------------------------------
# MOO literal parsing (enough for the values this script reads back)
# --------------------------------------------------------------------------


class MooObj(int):
    """An object number (#N) distinct from a plain int."""

    def __repr__(self) -> str:  # pragma: no cover - debugging aid
        return f"#{int(self)}"


class _MooParser:
    def __init__(self, text: str):
        self.s = text
        self.i = 0

    def peek(self) -> str:
        return self.s[self.i] if self.i < len(self.s) else ""

    def ws(self) -> None:
        while self.i < len(self.s) and self.s[self.i] in " \t":
            self.i += 1

    def expect(self, ch: str) -> None:
        self.ws()
        if self.peek() != ch:
            raise ValueError(f"expected {ch!r} at {self.i} in {self.s!r}")
        self.i += 1

    def value(self):
        self.ws()
        c = self.peek()
        if c == "{":
            self.i += 1
            items = []
            self.ws()
            if self.peek() == "}":
                self.i += 1
                return items
            while True:
                items.append(self.value())
                self.ws()
                if self.peek() == ",":
                    self.i += 1
                    continue
                self.expect("}")
                return items
        if c == "[":
            # map [k -> v, ...] or waif [[class = #n, owner = #m]]
            self.i += 1
            self.ws()
            if self.peek() == "[":
                start = self.i - 1  # the outer '[' already consumed
                depth = 1
                while self.i < len(self.s):
                    ch = self.s[self.i]
                    self.i += 1
                    if ch == "[":
                        depth += 1
                    elif ch == "]":
                        depth -= 1
                        if depth == 0:
                            break
                return ("waif", self.s[start : self.i])
            result = {}
            if self.peek() == "]":
                self.i += 1
                return result
            while True:
                k = self.value()
                self.ws()
                if self.s.startswith("->", self.i):
                    self.i += 2
                v = self.value()
                result[k if not isinstance(k, list) else json.dumps(k)] = v
                self.ws()
                if self.peek() == ",":
                    self.i += 1
                    continue
                self.expect("]")
                return result
        if c == '"':
            self.i += 1
            out = []
            while self.i < len(self.s):
                ch = self.s[self.i]
                if ch == "\\" and self.i + 1 < len(self.s):
                    out.append(self.s[self.i + 1])
                    self.i += 2
                    continue
                if ch == '"':
                    self.i += 1
                    return "".join(out)
                out.append(ch)
                self.i += 1
            raise ValueError("unterminated string")
        if c == "#":
            m = re.match(r"#(-?\d+)", self.s[self.i :])
            if not m:
                raise ValueError("bad objnum")
            self.i += m.end()
            return MooObj(int(m.group(1)))
        m = re.match(r"-?\d+\.\d*(?:e[+-]?\d+)?|-?\d+e[+-]?\d+", self.s[self.i :])
        if m:
            self.i += m.end()
            return float(m.group(0))
        m = re.match(r"-?\d+", self.s[self.i :])
        if m:
            self.i += m.end()
            return int(m.group(0))
        m = re.match(r"[A-Za-z_][A-Za-z_0-9]*", self.s[self.i :])
        if m:
            self.i += m.end()
            word = m.group(0)
            if word == "true":
                return True
            if word == "false":
                return False
            return word  # E_PERM etc.
        raise ValueError(f"unexpected {c!r} at {self.i} in {self.s!r}")


def parse_moo(text: str):
    p = _MooParser(text.strip())
    v = p.value()
    p.ws()
    if p.i != len(p.s):
        raise ValueError(f"trailing data in {text!r}")
    return v


# --------------------------------------------------------------------------
# Telnet-stripping line socket
# --------------------------------------------------------------------------

IAC, DONT, DO, WONT, WILL, SB, SE = 255, 254, 253, 252, 251, 250, 240


class LineConn:
    def __init__(self, host: str, port: int, connect_timeout: float = 10.0):
        self.host, self.port = host, port
        self.sock = socket.create_connection((host, port), timeout=connect_timeout)
        self.sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
        self.raw = b""
        self.lines: list[str] = []
        self.closed = False
        self._telnet_state = 0  # 0 normal, 1 after IAC, 2 after IAC+option-cmd, 3 in SB

    def close(self) -> None:
        self.closed = True
        try:
            self.sock.close()
        except OSError:
            pass

    def send_line(self, line: str) -> None:
        self.sock.sendall(line.encode("utf-8", "replace") + b"\r\n")

    def _strip_telnet(self, data: bytes) -> bytes:
        out = bytearray()
        st = self._telnet_state
        for b in data:
            if st == 0:
                if b == IAC:
                    st = 1
                else:
                    out.append(b)
            elif st == 1:
                if b == IAC:
                    out.append(b)
                    st = 0
                elif b == SB:
                    st = 3
                elif b in (DO, DONT, WILL, WONT):
                    st = 2
                else:
                    st = 0  # two-byte command
            elif st == 2:
                st = 0  # option byte
            elif st == 3:
                if b == IAC:
                    st = 4
            elif st == 4:
                st = 0 if b == SE else 3
        self._telnet_state = st
        return bytes(out)

    def _fill(self, timeout: float) -> bool:
        """Read one chunk; return False on timeout, raise on EOF."""
        self.sock.settimeout(timeout)
        try:
            chunk = self.sock.recv(65536)
        except socket.timeout:
            return False
        if not chunk:
            raise ConnectionError("server closed connection")
        self.raw += self._strip_telnet(chunk)
        while b"\n" in self.raw:
            line, self.raw = self.raw.split(b"\n", 1)
            self.lines.append(line.rstrip(b"\r").decode("utf-8", "replace"))
        return True

    def read_line(self, deadline: float) -> str | None:
        """Next line, or None once the deadline passes with no complete line."""
        while not self.lines:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                return None
            self._fill(min(remaining, 5.0))
        return self.lines.pop(0)

    def wait_for(self, needles: list[str], timeout: float) -> tuple[str | None, list[str]]:
        """Consume lines until one contains a needle; return (needle, seen)."""
        deadline = time.monotonic() + timeout
        seen: list[str] = []
        while True:
            line = self.read_line(deadline)
            if line is None:
                return None, seen
            seen.append(line)
            for n in needles:
                if n in line:
                    return n, seen

    def drain(self, quiet: float) -> list[str]:
        """Read until nothing arrives for `quiet` seconds."""
        seen: list[str] = []
        while True:
            line = self.read_line(time.monotonic() + quiet)
            if line is None:
                return seen
            seen.append(line)


# --------------------------------------------------------------------------
# Mongoose login and command bracket
# --------------------------------------------------------------------------


@dataclass
class LoginResult:
    player_hint: str
    proxy_sent: bool
    seconds: float
    transcript: list[str] = field(default_factory=list)


def mongoose_login(
    conn: LineConn,
    username: str,
    password: str,
    proxy_line: str | None,
    proxy_mode: str,
    banner_wait: float,
    timeout: float = 45.0,
) -> LoginResult:
    """Drive the $account_login conversation. See the module docstring for
    the three --proxy modes. In `immediate` mode on an UNTRUSTED connection
    the server hands the PROXY line to read() as a username, which costs one
    "Access Denied" before the real attempt; exactly one such denial is
    tolerated when a PROXY line was sent."""
    t0 = time.monotonic()
    transcript: list[str] = []
    proxy_sent = False
    if proxy_mode == "immediate" and proxy_line:
        conn.send_line(proxy_line)
        proxy_sent = True
        hit, seen = conn.wait_for([USERNAME_PROMPT], timeout)
    elif proxy_mode == "auto":
        hit, seen = conn.wait_for([USERNAME_PROMPT], banner_wait)
        transcript += seen
        if hit is None and proxy_line:
            conn.send_line(proxy_line)
            proxy_sent = True
            hit, seen = conn.wait_for([USERNAME_PROMPT], timeout)
    else:  # never
        hit, seen = conn.wait_for([USERNAME_PROMPT], timeout)
    transcript += seen
    if hit is None:
        raise TimeoutError(f"no username prompt for {username}: {transcript[-10:]}")
    conn.send_line(username)
    denials = 0
    while True:
        hit, seen = conn.wait_for([PASSWORD_PROMPT, ACCESS_DENIED], timeout)
        transcript += seen
        if hit == PASSWORD_PROMPT:
            break
        if hit == ACCESS_DENIED and proxy_sent and denials == 0:
            denials += 1  # the PROXY line was consumed as a bogus username
            continue
        raise RuntimeError(f"no password prompt for {username}: {transcript[-10:]}")
    conn.send_line(password)
    hit, seen = conn.wait_for([LOGGED_IN_MSG, ACCESS_DENIED, USERNAME_PROMPT], timeout)
    transcript += seen
    if hit != LOGGED_IN_MSG:
        raise RuntimeError(f"login failed for {username}: {transcript[-10:]}")
    return LoginResult(username, proxy_sent, time.monotonic() - t0, transcript)


def arm_bracket(conn: LineConn) -> None:
    conn.send_line(f"OUTPUTPREFIX {PREFIX_TAG}")
    conn.send_line(f"OUTPUTSUFFIX {SUFFIX_TAG}")


def bracketed(conn: LineConn, line: str, timeout: float) -> list[str] | None:
    """Send one command line; return the lines between prefix and suffix
    (None on timeout). Lines outside the bracket (async output) are dropped."""
    conn.send_line(line)
    deadline = time.monotonic() + timeout
    inside = False
    body: list[str] = []
    while True:
        got = conn.read_line(deadline)
        if got is None:
            return None
        if got == PREFIX_TAG:
            inside = True
            body = []
            continue
        if got == SUFFIX_TAG:
            return body
        if inside:
            body.append(got)


class Control:
    """A logged-in wizard connection used for eval."""

    def __init__(self, conn: LineConn):
        self.conn = conn
        self._seq = 0
        arm_bracket(conn)
        # Sync: the first bracketed command flushes any pre-bracket output.
        if bracketed(conn, "look", 30.0) is None:
            raise TimeoutError("control connection did not bracket `look`")

    def eval(self, code: str, timeout: float = 60.0, allow_no_result: bool = False):
        """Evaluate MOO statements whose LAST statement is `return EXPR;`.

        Uses `;;` (statement mode; a single `;` wraps only the first statement
        in `return ...;`). The returned value is tagged with a per-call nonce
        because a builtin that suspends the task (Toast's sqlite_execute does)
        makes the server print the OUTPUTSUFFIX at suspension and the `=> `
        result line afterwards, outside the bracket; the nonce lets us wait for
        exactly this call's result and ignore stale ones. Eval output lines are
        truncated by the server around 6000 chars, so keep results compact.
        Returns (value, lines) where value is None only when allow_no_result
        and the eval raised (Mongoose's #0:handle_uncaught_error swallows the
        traceback, so a failed eval is an empty bracket).
        """
        flat = " ".join(ln.strip() for ln in code.strip().splitlines() if ln.strip())
        head, sep, tail = flat.rpartition("return ")
        if not sep or not tail.endswith(";"):
            raise ValueError(f"eval code must end with `return EXPR;`: {flat[:80]!r}")
        self._seq += 1
        nonce = f"bp{self._seq}"
        cmd = f';; {head}return {{"{nonce}", {tail[:-1]}}};'
        marker = f'=> {{"{nonce}", '
        body = bracketed(self.conn, cmd, timeout)
        if body is None:
            raise TimeoutError(f"eval timed out: {flat[:80]}")
        result_line = next((ln for ln in body if ln.startswith(marker)), None)
        if result_line is None:
            # Either the task suspended (result still to come) or it raised
            # (no result ever). Wait a bounded time for the tagged line.
            deadline = time.monotonic() + (timeout if allow_no_result is False else 3.0)
            while result_line is None:
                ln = self.conn.read_line(deadline)
                if ln is None:
                    break
                body.append(ln)
                if ln.startswith(marker):
                    result_line = ln
        if result_line is None:
            if allow_no_result:
                return None, body
            raise RuntimeError(f"eval produced no result: {flat[:80]!r} -> {body[-8:]}")
        try:
            tagged = parse_moo(result_line[3:])
            return tagged[1], body
        except (ValueError, IndexError, TypeError):
            return result_line[3:], body

    def close(self) -> None:
        self.conn.close()


# --------------------------------------------------------------------------
# Servers
# --------------------------------------------------------------------------


def wait_port(host: str, port: int, timeout: float, proc: subprocess.Popen) -> float:
    t0 = time.monotonic()
    while time.monotonic() - t0 < timeout:
        if proc.poll() is not None:
            raise RuntimeError(f"server exited early with code {proc.returncode}")
        try:
            with socket.create_connection((host, port), timeout=1.0):
                return time.monotonic() - t0
        except OSError:
            time.sleep(0.25)
    raise TimeoutError(f"server did not listen on {host}:{port} within {timeout}s")


def wsl(*args: str, timeout: float = 60.0) -> str:
    cmd = ["wsl.exe", "-d", WSL_DISTRO, "-u", "root", "-e", *args]
    return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout).stdout


class Server:
    engine: str
    host: str
    identity: dict

    def start(self) -> float:  # returns seconds until listening
        raise NotImplementedError

    def stop(self) -> None:
        raise NotImplementedError


class ToastServer(Server):
    engine = "toast"

    def __init__(self, run_dir: Path, db_copy: Path, port: int):
        self.run_dir, self.db_copy, self.port = run_dir, db_copy, port
        self.proc: subprocess.Popen | None = None
        self.log_file = None
        self.host = wsl("hostname", "-I").strip().split()[0]
        sha_line = wsl("sha256sum", TOAST_MOO).strip()
        commit = wsl("git", "-C", TOAST_SRC, "rev-parse", "HEAD").strip()
        self.identity = {
            "executable": TOAST_MOO,
            "executable_sha256": sha_line.split()[0] if sha_line else None,
            "source_commit": commit,
            "wrapper": TOAST_WRAPPER_WSL,
            "wsl_distro": WSL_DISTRO,
            "host": self.host,
        }

    def start(self) -> float:
        self.log_file = open(self.run_dir / "server.log", "ab")
        db_win = str(self.db_copy).replace("\\", "/")
        cmd = [
            "wsl.exe", "-d", WSL_DISTRO, "-u", "root", "-e",
            "env", f"TOAST_MOO={TOAST_MOO}",
            "bash", TOAST_WRAPPER_WSL, db_win, str(self.port),
        ]
        self.identity["command"] = cmd
        self.proc = subprocess.Popen(
            cmd, cwd=str(self.run_dir), stdin=subprocess.DEVNULL,
            stdout=self.log_file, stderr=subprocess.STDOUT,
        )
        return wait_port(self.host, self.port, 180.0, self.proc)

    def _listener_pid(self) -> str | None:
        out = wsl("bash", "-c", f"ss -H -ltnp 'sport = :{self.port}'")
        m = re.search(r"pid=(\d+)", out)
        return m.group(1) if m else None

    def stop(self) -> None:
        if self.proc is not None:
            self.proc.terminate()
            try:
                self.proc.wait(10)
            except subprocess.TimeoutExpired:
                self.proc.kill()
        # The wsl.exe relay usually takes the foreground moo down with it. If a
        # listener on OUR port survives, kill only that pid, only if its command
        # line names our disposable database (other Toast servers may be running).
        pid = self._listener_pid()
        if pid:
            args = wsl("bash", "-c", f"tr '\\0' ' ' < /proc/{pid}/cmdline").strip()
            if self.db_copy.name in args:
                wsl("kill", pid)
                log(f"killed leftover Toast pid {pid} ({args})")
            else:
                log(f"WARNING: listener on port {self.port} (pid {pid}) is not ours: {args}")
        if self.log_file:
            self.log_file.close()


class BarnServer(Server):
    engine = "barn"

    def __init__(self, run_dir: Path, db_copy: Path, port: int, exe: Path):
        self.run_dir, self.db_copy, self.port, self.exe = run_dir, db_copy, port, exe.resolve()
        self.proc: subprocess.Popen | None = None
        self.log_file = None
        self.host = "127.0.0.1"
        head = subprocess.run(["git", "-C", str(REPO_ROOT), "rev-parse", "HEAD"],
                              capture_output=True, text=True).stdout.strip()
        dirty = subprocess.run(["git", "-C", str(REPO_ROOT), "status", "--porcelain",
                                "--untracked-files=no"], capture_output=True, text=True).stdout
        self.identity = {
            "executable": str(self.exe),
            "executable_sha256": sha256_file(self.exe),
            "git_head": head,
            "git_tracked_dirty": bool(dirty.strip()),
            "config": str(BARN_CONFIG),
            "host": self.host,
        }

    def start(self) -> float:
        self.log_file = open(self.run_dir / "server.log", "ab")
        cmd = [
            str(self.exe),
            "--db", str(self.db_copy),
            "--listen", f"tcp://{self.host}:{self.port}",
            "--checkpoint-interval", "0",
            "--config", str(BARN_CONFIG),
            "--promote-numbers",
            "--profile-id", BARN_PROFILE_ID,
            "--profile-manifest", str(self.run_dir / "profile.json"),
            "--log-dir", str(self.run_dir / "logs"),
            "--debug-addr", "127.0.0.1:0",
        ]
        self.identity["command"] = cmd
        self.proc = subprocess.Popen(
            cmd, cwd=str(self.run_dir), stdin=subprocess.DEVNULL,
            stdout=self.log_file, stderr=subprocess.STDOUT,
        )
        return wait_port(self.host, self.port, 180.0, self.proc)

    def stop(self) -> None:
        if self.proc is not None and self.proc.poll() is None:
            self.proc.terminate()
            try:
                self.proc.wait(10)
            except subprocess.TimeoutExpired:
                self.proc.kill()
        if self.log_file:
            self.log_file.close()


# --------------------------------------------------------------------------
# Snapshot repair (mirrors engine/mongoose_real_bench_test.go)
# --------------------------------------------------------------------------

REPAIR_CODE = """
sql = #2585.sql;
for x in ($sql_utils.databases)
  if (x.name == sql.name && x:is_open())
    #2585.sql = x;
  endif
endfor
cloaked = `property_info(#36, "cloaked") ! E_PROPNF => add_property(#36, "cloaked", 0, {#36, "r"})';
if (#2585.sql:is_open())
  sqlite_execute(#2585.sql.handle, "CREATE TABLE IF NOT EXISTS assets (asset_location TEXT, duration REAL);", {});
endif
return {#2585.sql:is_open() != 0, length($sql_utils.databases), sqlite_handles(), $prod(), cloaked};
"""


def apply_repair(ctl: Control) -> dict:
    """Poll until $sql_utils has opened the sound database (it opens from forked
    tasks after #0:server_started), then apply the two wizard repairs."""
    last = None
    for attempt in range(60):
        # During the first seconds after boot the registry waifs are still
        # being opened by forked tasks and the eval can raise; keep polling.
        value, body = ctl.eval(REPAIR_CODE, allow_no_result=True)
        last = {"attempt": attempt + 1,
                "value": value if not isinstance(value, list) else [str(v) if isinstance(v, MooObj) else v for v in value],
                "raw": body[-3:]}
        if isinstance(value, list) and value and value[0] == 1:
            return last
        time.sleep(0.5)
    return last or {}


ROSTER_PAGE = 150  # players per eval; results must stay under the ~6000-char output cap


def compute_roster(ctl: Control) -> list[dict]:
    """Players with location > #0 (the Barn harness rule), ascending by objnum.
    Fetched compactly ({obj, wizard} pairs, paged); names and locations are
    filled in later only for the players the run actually uses."""
    count, _ = ctl.eval("return length(players());")
    entries: list[dict] = []
    for start in range(1, int(count) + 1, ROSTER_PAGE):
        stop = min(start + ROSTER_PAGE - 1, int(count))
        code = f"""
        r = {{}};
        for p in (players()[{start}..{stop}])
          loc = `p.location ! ANY => #-1';
          if (toint(loc) > 0)
            r = {{@r, {{p, p.wizard}}}};
          endif
        endfor
        return r;
        """
        value, _ = ctl.eval(code, timeout=120.0)
        if not isinstance(value, list):
            raise RuntimeError(f"roster eval returned {value!r}")
        for p, wiz in value:
            entries.append({"obj": int(p), "wizard": bool(wiz)})
    entries.sort(key=lambda e: e["obj"])
    return entries


def describe_players(ctl: Control, entries: list[dict]) -> None:
    """Fill name/location/programmer for the given roster entries in place."""
    for start in range(0, len(entries), 40):
        chunk = entries[start : start + 40]
        objs = ", ".join(f"#{e['obj']}" for e in chunk)
        code = f"""
        r = {{}};
        for p in ({{{objs}}})
          r = {{@r, {{p.name, `p.location ! ANY => #-1', p.programmer}}}};
        endfor
        return r;
        """
        value, _ = ctl.eval(code)
        if not isinstance(value, list) or len(value) != len(chunk):
            raise RuntimeError(f"describe eval returned {value!r}")
        for e, (name, loc, prog) in zip(chunk, value):
            e["name"], e["location"], e["programmer"] = name, int(loc), bool(prog)


def create_account(ctl: Control, username: str, password: str, player: int) -> dict:
    code = f"""
    am = $account_manager;
    a = am:create_account("", "{username}", "{password}");
    a.players = {{#{player}}};
    a.default_player = #{player};
    a.email_verified = 1;
    return {{a.username, a.players, a:default_player(), a.email_verified, a:validate_password("{password}")}};
    """
    value, body = ctl.eval(code, timeout=60.0)
    ok = isinstance(value, list) and len(value) == 5 and int(value[2]) == player and value[4] in (1, True)
    if not ok:
        raise RuntimeError(f"account {username} for #{player} not usable: {value!r} {body[-5:]}")
    return {"username": username, "player": player}


# --------------------------------------------------------------------------
# Load generation
# --------------------------------------------------------------------------


@dataclass
class ShapeStat:
    ok: int = 0
    fail: int = 0
    total_lat: float = 0.0
    traceback_lines: int = 0


@dataclass
class PlayerStat:
    shapes: list[ShapeStat]
    lats: list[float] = field(default_factory=list)
    broken: str | None = None
    fail_samples: list[str] = field(default_factory=list)


def pick_shape(rng: random.Random, total_weight: int) -> int:
    x = rng.randrange(total_weight)
    for j, (_, _, w) in enumerate(SHAPES):
        if x < w:
            return j
        x -= w
    return len(SHAPES) - 1


def run_window(conns: list[LineConn], rngs: list[random.Random], stats: list[PlayerStat],
               duration: float, record: bool, cmd_timeout: float) -> float:
    total_weight = sum(w for _, _, w in SHAPES)
    deadline = time.monotonic() + duration

    def worker(idx: int) -> None:
        conn, rng, st = conns[idx], rngs[idx], stats[idx]
        while time.monotonic() < deadline and st.broken is None:
            sh = pick_shape(rng, total_weight)
            line = SHAPES[sh][1]
            t0 = time.perf_counter()
            try:
                body = bracketed(conn, line, cmd_timeout)
            except (OSError, ConnectionError) as exc:
                st.broken = f"{SHAPES[sh][0]}: {exc}"
                if record:
                    st.shapes[sh].fail += 1
                break
            lat = time.perf_counter() - t0
            if not record:
                continue
            ss = st.shapes[sh]
            if body is None:
                ss.fail += 1
                st.broken = f"{SHAPES[sh][0]}: no suffix within {cmd_timeout}s"
                if len(st.fail_samples) < 5:
                    st.fail_samples.append(st.broken)
                break
            ss.ok += 1
            ss.total_lat += lat
            st.lats.append(lat)
            ss.traceback_lines += sum(1 for ln in body if TRACEBACK_RE.match(ln))

    threads = [threading.Thread(target=worker, args=(i,), daemon=True) for i in range(len(conns))]
    t_start = time.monotonic()
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    return time.monotonic() - t_start


def percentile(sorted_vals: list[float], q: float) -> float:
    if not sorted_vals:
        return 0.0
    idx = min(int(q * len(sorted_vals)), len(sorted_vals) - 1)
    return sorted_vals[idx]


def ms(x: float) -> str:
    return f"{x * 1000:.1f} ms"


# --------------------------------------------------------------------------
# Main flow
# --------------------------------------------------------------------------


def load_login_script(port: int) -> tuple[str, str, str]:
    raw = os.environ.get("MONGOOSE_LOGIN_SCRIPT", "")
    lines = [ln for ln in raw.replace("\r", "").split("\n") if ln.strip()]
    if len(lines) != 3:
        sys.exit("MONGOOSE_LOGIN_SCRIPT must hold exactly three non-empty lines: "
                 "PROXY prelude (with {port}), wizard account username, password")
    return lines[0].replace("{port}", str(port)), lines[1], lines[2]


def connect_control(server: Server, proxy: str, user: str, password: str, proxy_mode: str,
                    banner_wait: float, label: str) -> tuple[Control, LoginResult]:
    conn = LineConn(server.host, server.port)
    lr = mongoose_login(conn, user, password, proxy, proxy_mode, banner_wait)
    log(f"{label}: logged in as account {lr.player_hint!r} in {lr.seconds:.2f}s (proxy_sent={lr.proxy_sent})")
    return Control(conn), lr


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--engine", choices=["toast", "barn"], required=True)
    ap.add_argument("--players", default="1,16", help="comma list of concurrency levels (default 1,16)")
    ap.add_argument("--warmup", type=float, default=2.0, help="warm-up seconds per level (default 2)")
    ap.add_argument("--measure", type=float, default=8.0, help="measure seconds per level (default 8)")
    ap.add_argument("--db", type=Path, default=DEFAULT_DB, help=f"source fixture (default {DEFAULT_DB})")
    ap.add_argument("--port", type=int, default=7777, help="listen port; 7777 = $network.port makes $prod() true")
    ap.add_argument("--out", type=Path, default=None, help="run directory (default .tmp/bench_players/<engine>-<ts>)")
    ap.add_argument("--barn-exe", type=Path, default=REPO_ROOT / "barn.exe")
    ap.add_argument("--no-repair", action="store_true", help="skip the two harness snapshot repairs")
    ap.add_argument("--proxy", choices=["immediate", "auto", "never"], default="immediate",
                    help="when to send the PROXY prelude: immediate (default; first bytes on the wire), "
                         "auto (only if no login prompt appears within --banner-wait), never")
    ap.add_argument("--banner-wait", type=float, default=1.5,
                    help="--proxy auto: seconds to wait for the username prompt before sending the PROXY prelude")
    ap.add_argument("--cmd-timeout", type=float, default=30.0, help="per-command completion timeout")
    ap.add_argument("--settle", type=float, default=2.0, help="seconds to let post-login forks settle before warm-up")
    ap.add_argument("--keep-db", action="store_true", help="keep the disposable database copy after the run")
    args = ap.parse_args()

    levels = sorted({int(x) for x in args.players.split(",") if x.strip()})
    if not levels:
        sys.exit("--players needs at least one level")
    max_level = max(levels)
    proxy_line, ctl_user, ctl_pass = load_login_script(args.port)

    # --- fixture -----------------------------------------------------------
    src = args.db.resolve()
    if not src.is_file():
        sys.exit(f"fixture not found: {src}")
    stamp = utc_stamp()
    run_dir = (args.out or (REPO_ROOT / ".tmp" / "bench_players" / f"{args.engine}-{stamp}")).resolve()
    run_dir.mkdir(parents=True, exist_ok=True)
    (run_dir / "files" / "sqlite").mkdir(parents=True, exist_ok=True)
    src_sha = sha256_file(src)
    db_copy = run_dir / "run.db"
    shutil.copy2(src, db_copy)
    copy_sha = sha256_file(db_copy)
    if copy_sha != src_sha:
        sys.exit("disposable copy hash differs from source")
    fixture = {"source_path": str(src), "size_bytes": src.stat().st_size, "sha256": src_sha,
               "copy_path": str(db_copy), "copy_sha256": copy_sha}
    log(f"fixture {src} size={fixture['size_bytes']} sha256={src_sha}")
    log(f"run dir {run_dir}")

    record: dict = {
        "issue": 265, "engine": args.engine, "started_utc": stamp, "fixture": fixture,
        "mix": [{"name": n, "line": l, "weight": w} for n, l, w in SHAPES],
        "seed_base": SEED_BASE, "warmup_s": args.warmup, "measure_s": args.measure,
        "levels": levels, "port": args.port, "repair": not args.no_repair, "proxy_mode": args.proxy,
        "python": sys.version,
    }

    if args.engine == "toast":
        server: Server = ToastServer(run_dir, db_copy, args.port)
    else:
        if not args.barn_exe.is_file():
            sys.exit(f"barn executable not found: {args.barn_exe} (go build -o barn.exe ./cmd/barn/)")
        server = BarnServer(run_dir, db_copy, args.port, args.barn_exe)
    record["server"] = server.identity
    bench_conns: list[LineConn] = []
    exit_code = 0
    try:
        t_listen = server.start()
        record["listen_after_s"] = round(t_listen, 3)
        log(f"{args.engine} listening on {server.host}:{args.port} after {t_listen:.1f}s")

        # --- control session 1: repair, roster, accounts --------------------
        ctl, lr = connect_control(server, proxy_line, ctl_user, ctl_pass, args.proxy, args.banner_wait, "control")
        record["control_login"] = {"seconds": round(lr.seconds, 3), "proxy_sent": lr.proxy_sent}
        who, _ = ctl.eval("return {player, player.name, player.wizard, $prod(), server_version()};")
        record["control_identity"] = [str(v) if isinstance(v, MooObj) else v for v in who] if isinstance(who, list) else who
        log(f"control identity {record['control_identity']}")
        if not args.no_repair:
            rep = apply_repair(ctl)
            record["repair_result"] = rep
            log(f"snapshot repair {{sound db open, registry size, handles, prod, cloaked}}: {rep.get('value')} (attempt {rep.get('attempt')})")

        roster_all = compute_roster(ctl)
        record["roster_candidates"] = len(roster_all)
        if len(roster_all) < max_level:
            raise RuntimeError(f"only {len(roster_all)} candidate players, need {max_level}")
        roster = roster_all[:max_level]
        roster_objs = {e["obj"] for e in roster}
        ctl_wizard = next((e for e in roster_all if e["wizard"] and e["obj"] not in roster_objs), None)
        if ctl_wizard is None:
            raise RuntimeError("no wizard outside the roster prefix for the benchctl account")
        describe_players(ctl, roster + [ctl_wizard])
        record["roster"] = roster
        log(f"roster ({len(roster_all)} candidates): " + " ".join(f"#{e['obj']}" for e in roster))
        accounts = []
        for i, e in enumerate(roster, start=1):
            accounts.append(create_account(ctl, f"benchp{i}", BENCH_PASSWORD, e["obj"]))
        create_account(ctl, "benchctl", BENCH_PASSWORD, ctl_wizard["obj"])
        record["accounts"] = {"players": accounts, "control_wizard": ctl_wizard}
        log(f"created {len(accounts)} bench accounts + benchctl -> #{ctl_wizard['obj']} ({ctl_wizard['name']})")
        ctl.close()
        time.sleep(1.0)

        # --- per level --------------------------------------------------------
        results = []
        for active in levels:
            log(f"=== level {active} players ===")
            bench_conns = []
            logins = []
            for i in range(active):
                conn = LineConn(server.host, args.port)
                lr = mongoose_login(conn, f"benchp{i + 1}", BENCH_PASSWORD, proxy_line, args.proxy, args.banner_wait)
                arm_bracket(conn)
                if bracketed(conn, "look", args.cmd_timeout) is None:
                    raise TimeoutError(f"benchp{i + 1} did not bracket `look`")
                bench_conns.append(conn)
                logins.append({"account": f"benchp{i + 1}", "player": roster[i]["obj"],
                               "seconds": round(lr.seconds, 3), "proxy_sent": lr.proxy_sent})
            log(f"{active} bench connections logged in (avg {statistics.mean(l['seconds'] for l in logins):.2f}s each)")

            # Verify identities from a wizard that is not in the roster.
            ctl, _ = connect_control(server, proxy_line, "benchctl", BENCH_PASSWORD, args.proxy, args.banner_wait, "verify")
            connected, _ = ctl.eval("return connected_players();")
            connected_set = {int(v) for v in connected} if isinstance(connected, list) else set()
            expected = {e["obj"] for e in roster[:active]}
            missing = sorted(expected - connected_set)
            extra = sorted(connected_set - expected - {ctl_wizard["obj"]})
            ctl.close()
            if missing:
                raise RuntimeError(f"roster players not connected after login: {missing}")
            log(f"connected_players() covers roster (extra non-roster players: {extra})")
            time.sleep(args.settle)

            rngs = [random.Random(SEED_BASE + i) for i in range(active)]
            stats = [PlayerStat([ShapeStat() for _ in SHAPES]) for _ in range(active)]
            run_window(bench_conns, rngs, stats, args.warmup, False, args.cmd_timeout)
            elapsed = run_window(bench_conns, rngs, stats, args.measure, True, args.cmd_timeout)

            committed = sum(s.ok for st in stats for s in st.shapes)
            failed = sum(s.fail for st in stats for s in st.shapes)
            lats = sorted(l for st in stats for l in st.lats)
            shape_rows = []
            for j, (name, _, _) in enumerate(SHAPES):
                ok = sum(st.shapes[j].ok for st in stats)
                fl = sum(st.shapes[j].fail for st in stats)
                tl = sum(st.shapes[j].total_lat for st in stats)
                tb = sum(st.shapes[j].traceback_lines for st in stats)
                shape_rows.append({"shape": name, "ok": ok, "fail": fl,
                                   "avg_ms": round(tl / ok * 1000, 2) if ok else None,
                                   "traceback_lines": tb})
            level_result = {
                "players": active, "elapsed_s": round(elapsed, 3),
                "committed": committed, "failed": failed,
                "goodput_per_s": round(committed / elapsed, 1) if elapsed else 0,
                "p50_ms": round(percentile(lats, 0.50) * 1000, 2),
                "p99_ms": round(percentile(lats, 0.99) * 1000, 2),
                "p999_ms": round(percentile(lats, 0.999) * 1000, 2),
                "max_ms": round(lats[-1] * 1000, 2) if lats else None,
                "shapes": shape_rows, "logins": logins,
                "extra_connected": extra,
                "broken_connections": [st.broken for st in stats if st.broken],
            }
            results.append(level_result)
            log(f"players={active} goodput={level_result['goodput_per_s']}/s committed={committed} failed={failed} "
                f"p50={level_result['p50_ms']}ms p99={level_result['p99_ms']}ms p99.9={level_result['p999_ms']}ms max={level_result['max_ms']}ms")
            for row in shape_rows:
                log(f"  shape {row['shape']:<10} ok={row['ok']:<6} fail={row['fail']:<4} avg={row['avg_ms']} ms tracebacks={row['traceback_lines']}")
            for b in level_result["broken_connections"]:
                log(f"  BROKEN: {b}")
            for c in bench_conns:
                c.close()
            bench_conns = []
            time.sleep(1.0)
        record["results"] = results
    except Exception as exc:  # noqa: BLE001 - report and keep the run record
        record["error"] = f"{type(exc).__name__}: {exc}"
        log(f"ERROR {record['error']}")
        exit_code = 1
    finally:
        for c in bench_conns:
            c.close()
        server.stop()
        record["finished_utc"] = utc_stamp()
        (run_dir / "run.json").write_text(json.dumps(record, indent=2, default=str), encoding="utf-8")
        write_report(run_dir / "report.md", record)
        if not args.keep_db:
            for p in (db_copy, Path(str(db_copy) + ".new"), Path(str(db_copy) + ".new.PANIC")):
                try:
                    p.unlink()
                except FileNotFoundError:
                    pass
        log(f"wrote {run_dir / 'run.json'} and {run_dir / 'report.md'}")
    return exit_code


def write_report(path: Path, rec: dict) -> None:
    lines = [f"# bench_players: {rec['engine']} on Mongoose ({rec['started_utc']})", ""]
    fx = rec["fixture"]
    lines += [f"- fixture: `{fx['source_path']}` size={fx['size_bytes']} sha256=`{fx['sha256']}`",
              f"- engine: `{json.dumps(rec.get('server', {}), default=str)}`",
              f"- mix: " + ", ".join(f"{m['name']} {m['weight']}" for m in rec["mix"]) +
              f"; warmup {rec['warmup_s']}s, measure {rec['measure_s']}s, seeds 0x{rec['seed_base']:X}+idx",
              f"- repair applied: {rec.get('repair')} -> {rec.get('repair_result', {}).get('value')}",
              f"- roster: " + " ".join(f"#{e['obj']}" for e in rec.get("roster", [])), ""]
    if rec.get("error"):
        lines += [f"**ERROR:** {rec['error']}", ""]
    lines += ["| players | goodput/s | committed | failed | p50 | p99 | p99.9 | max |",
              "|---:|---:|---:|---:|---:|---:|---:|---:|"]
    for r in rec.get("results", []):
        lines.append(f"| {r['players']} | {r['goodput_per_s']} | {r['committed']} | {r['failed']} | "
                     f"{r['p50_ms']} ms | {r['p99_ms']} ms | {r['p999_ms']} ms | {r['max_ms']} ms |")
    for r in rec.get("results", []):
        lines += ["", f"Per shape at {r['players']} players:", "",
                  "| shape | ok | fail | avg | traceback lines |", "|---|---:|---:|---:|---:|"]
        for s in r["shapes"]:
            lines.append(f"| {s['shape']} | {s['ok']} | {s['fail']} | {s['avg_ms']} ms | {s['traceback_lines']} |")
        if r.get("broken_connections"):
            lines += ["", "Broken connections: " + "; ".join(r["broken_connections"])]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


if __name__ == "__main__":
    sys.exit(main())
