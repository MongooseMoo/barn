"""Shared observers for real gameplay in an owned, disposable Mongoose world."""
from __future__ import annotations

import time
import uuid

import bench_players as bench


class ScenarioFailure(RuntimeError):
    def __init__(self, message, evidence=None):
        super().__init__(message)
        self.evidence = evidence


def object_ref(value):
    if isinstance(value, str) and value.startswith("#"):
        value = int(value[1:])
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise ValueError("requires a valid persistent object reference")
    return f"#{int(value)}"


def actor_eval(control, actor, code, timeout=60):
    """Use an ordinary actor's identity and permissions, without a game bypass.

    This is a domain-entry diagnostic. It does not measure client/network input
    latency. Native authorization and errors remain the game's responsibility.
    """
    ref = object_ref(actor)
    admitted, _ = control.eval(
        f"if (!valid({ref})) raise(E_INVARG); endif "
        f"return {{{ref}.wizard, {ref}.programmer}};", timeout=timeout)
    if not isinstance(admitted, list) or len(admitted) != 2 or admitted[0] != 0:
        raise ScenarioFailure("scenario actor must be an ordinary non-wizard", admitted)
    return control.eval(f"player = {ref}; set_task_perms({ref}); {code}", timeout=timeout)


def require(condition, message, evidence=None):
    if not condition:
        raise ScenarioFailure(message, evidence)


def connect_actor(control, actor, options):
    """Associate a fresh private account and log in; never persist credentials."""
    ref = object_ref(actor)
    admitted, _ = control.eval(f"return {{is_player({ref}), {ref}.wizard}};")
    require(admitted == [1, 0], "requires an ordinary player before account setup", admitted)
    username = "gameplay" + uuid.uuid4().hex[:12]
    password = uuid.uuid4().hex
    original = getattr(control, "control", control)
    connection = None
    stage = "account_creation"
    try:
        bench.create_account(original, username, password, int(ref[1:]))
        # The existing account helper associates a.players but does not set
        # the new character's inherited account property. Complete only this
        # fresh fixture association, as production migration/fixture code does.
        stage = "account_association"
        bound, _ = original.eval(
            f'a = $account_manager.accounts["{username}"]; who = {ref}; '
            'if (typeof(a) != WAIF || length(a.players) != 1 || a.players[1] != who '
            '|| a.default_player != who) raise(E_PERM); endif '
            'if (who.account != $nothing && who.account != a) raise(E_PERM); endif '
            'who.account = a; return {who.account == a, (who in a.players) != 0, who.wizard};')
        require(bound == [1, 1, 0], "fresh actor account association failed", bound)
        stage = "connection"
        connection = bench.LineConn(options.host, options.port)
        proxy = f"PROXY TCP4 127.0.0.1 127.0.0.1 40001 {options.port}"
        stage = "login"
        bench.mongoose_login(connection, username, password, proxy, "immediate", 0.25)
        stage = "command_bracket"
        bench.arm_bracket(connection)
        require(bench.bracketed(connection, "look", 30) is not None,
                "ordinary actor login did not produce a command response")
        stage = "connection_verification"
        connected, _ = control.eval(f"return {ref} in connected_players();")
        require(connected, "ordinary actor did not become connected", {"actor": ref})
    except Exception as error:
        # Control failures can clip the source in the middle of a password;
        # replacing the complete password cannot sanitize that partial prefix.
        # Keep only phase/type metadata for this credential-bearing setup.
        evidence = {"stage": stage, "native_error_type": type(error).__name__}
        if connection:
            try:
                connection.close()
            except Exception as cleanup:
                evidence["cleanup_error_type"] = type(cleanup).__name__
        raise ScenarioFailure(f"ordinary actor setup failed at {stage}: {type(error).__name__}", evidence) from None
    if not hasattr(options, "actor_connections"):
        options.actor_connections = []
    options.actor_connections.append(connection)
    connection.gameplay_transcript = options.run_dir / f"actor-{ref[1:]}-commands.jsonl"
    with connection.gameplay_transcript.open("x", encoding="utf-8"):
        pass
    return connection


def actor_command(connection, command, timeout=60):
    """Submit real input; domain observers, not OUTPUTSUFFIX, prove completion."""
    if not isinstance(command, str) or not command or any(c in command for c in "\r\n"):
        raise ValueError("requires one command line")
    started = time.monotonic_ns()
    record = {"command": command, "started_monotonic_ns": started,
              "scope": "input response bracket; domain completion requires independent checks"}
    try:
        output = bench.bracketed(connection, command, timeout)
        record["output"] = output
        if output is None:
            raise ScenarioFailure("ordinary command response timed out", {"command": command})
        record["status"] = "returned"
        return record
    except Exception as error:
        record.update(status="failed", failure_type=type(error).__name__, failure=str(error))
        raise
    finally:
        record["response_wall_ns"] = time.monotonic_ns() - started
        if hasattr(connection, "gameplay_transcript"):
            import json
            with connection.gameplay_transcript.open("a", encoding="utf-8") as stream:
                stream.write(json.dumps(record) + "\n")


def wait_until(observe, predicate, timeout, *, interval=0.1, description="domain completion"):
    """A deadline is failure, never successful work. Preserve the last state."""
    if timeout <= 0 or interval <= 0:
        raise ValueError("timeout and interval must be positive")
    started = time.monotonic()
    deadline = started + timeout
    observations = 0
    while True:
        last = observe()
        observations += 1
        if predicate(last):
            return {"state": last, "observations": observations,
                    "elapsed_wall_seconds": time.monotonic() - started}
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise ScenarioFailure(f"timed out waiting for {description}",
                                  {"last_state": last, "observations": observations,
                                   "elapsed_wall_seconds": time.monotonic() - started})
        time.sleep(min(interval, remaining))
