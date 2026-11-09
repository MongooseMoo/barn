"""Pure fixtures for the Jepsen checkpoint adapter: never launch Barn, Jepsen or Elle."""
import json
from pathlib import Path
import socket
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import jepsen_checkpoint as jc


def ok(value):
    return SimpleNamespace(success=True, value=value, error=None, error_message=None)


def native(code, message=None):
    return SimpleNamespace(success=False, value=None, error=SimpleNamespace(value=code), error_message=message)


class TxnCodeTests(unittest.TestCase):
    def test_appends_write_rows_and_ledger_in_one_task_and_reads_rows(self):
        code = jc.txn_code([["append", 3, 7], ["r", 3, None]])
        self.assertEqual(code, " ".join([
            "rows = #0.jepsen_rows;", "led = #0.jepsen_ledger;", "out = {};",
            "rows[3] = {@(maphaskey(rows, 3) ? rows[3] | {}), 7};",
            "led[3] = {@(maphaskey(led, 3) ? led[3] | {}), 7};",
            "out = {@out, (maphaskey(rows, 3) ? rows[3] | {})};",
            "#0.jepsen_rows = rows;", "#0.jepsen_ledger = led;", "return out;"]))

    def test_read_only_transaction_writes_nothing(self):
        code = jc.txn_code([["r", 1, None]])
        self.assertNotIn("#0.jepsen_rows =", code)
        self.assertNotIn("#0.jepsen_ledger =", code)

    def test_malformed_micro_operations_are_refused(self):
        for txn in ([["append", True, 1]], [["append", 1, "x"]], [["w", 1, 2]], [["r", 1, 5]], [["r", 1]]):
            with self.subTest(txn=txn):
                with self.assertRaises(ValueError):
                    jc.txn_code(txn)


class OutcomeTests(unittest.TestCase):
    def test_reads_fill_in_order_and_appends_echo(self):
        txn = [["r", 1, None], ["append", 1, 4], ["r", 2, None]]
        self.assertEqual(jc.complete_txn(txn, ok([[1, 2], []])),
                         {"type": "ok", "value": [["r", 1, [1, 2]], ["append", 1, 4], ["r", 2, []]]})

    def test_native_error_is_unknown_with_code_and_message(self):
        reply = jc.complete_txn([["append", 1, 2]], native("E_PERM", "Permission denied"))
        self.assertEqual(reply, {"type": "info", "error": {"kind": "native-error", "code": "E_PERM",
                                                           "message": "Permission denied"}})

    def test_reply_shape_mismatch_is_unknown_not_ok(self):
        txn = [["r", 1, None]]
        for value in ([], [[1], [2]], [["1"]], [[True]], "E_TYPE", None, [1]):
            with self.subTest(value=value):
                reply = jc.complete_txn(txn, ok(value))
                self.assertEqual(reply["type"], "info")
                self.assertEqual(reply["error"]["kind"], "malformed-response")

    def test_ledger_must_be_integer_lists_by_integer_key(self):
        self.assertEqual(jc.parse_ledger({1: [2, 3], 4: []}), {1: [2, 3], 4: []})
        for value in ([], {"1": [2]}, {1: [True]}, {1: 2}, None):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    jc.parse_ledger(value)
        self.assertEqual(jc.ledger_from_json(jc.ledger_json({2: [1], 1: [5]})), {1: [5], 2: [1]})


class FakeSocket:
    def __init__(self, chunks):
        self.chunks = list(chunks)

    def recv(self, size):
        return self.chunks.pop(0) if self.chunks else b""


class FakeSocketTransport:
    """The parts of SocketTransport._receive the recording override relies on."""

    def __init__(self, chunks):
        self.sock = FakeSocket(chunks)

    def _receive(self):
        lines = []
        while True:
            data = self.sock.recv(4096)
            if not data:
                break
            lines.append(data.decode())
            if data.endswith(b"SUFFIX"):
                break
        return "".join(lines) or None


class RecordingTransportTests(unittest.TestCase):
    def test_complete_frame_is_returned_and_recorded(self):
        transport = jc.recording_transport_class(FakeSocketTransport)([b"{1, 2}", b"SUFFIX"])
        sock = transport.sock
        self.assertEqual(transport._receive(), "{1, 2}SUFFIX")
        self.assertEqual(transport.last_raw, "{1, 2}SUFFIX")
        self.assertIs(transport.sock, sock)

    def test_peer_close_before_frame_end_is_never_a_reply(self):
        for chunks in ([], [b"{1, "]):
            with self.subTest(chunks=chunks):
                transport = jc.recording_transport_class(FakeSocketTransport)(chunks)
                sock = transport.sock
                with self.assertRaises(ConnectionError):
                    transport._receive()
                self.assertEqual(transport.last_raw, "{1, " if chunks else None)
                self.assertIs(transport.sock, sock)


class FakeTransport:
    def __init__(self, port, script):
        self.port, self.script, self.last_raw, self.closed = port, script, None, False

    def connect(self, user):
        if self.script.get("connect"):
            raise self.script["connect"]

    def execute(self, code):
        self.script.setdefault("codes", []).append((self.port, code))
        outcome = self.script["execute"](self.port, code)
        if isinstance(outcome, Exception):
            raise outcome
        self.last_raw = repr(outcome.value)
        return outcome

    def disconnect(self):
        self.closed = True


class SessionTests(unittest.TestCase):
    def session(self, live, script):
        made = []
        def factory(port):
            made.append(FakeTransport(port, script))
            return made[-1]
        campaign = SimpleNamespace(lock=jc.threading.Lock(), live=live, transport_factory=factory)
        return jc.ClientSession(campaign), campaign, made

    def test_no_live_generation_is_an_unsent_failure(self):
        session, _, made = self.session(None, {})
        self.assertEqual(session.txn([["append", 1, 1]]),
                         {"type": "fail", "generation": None, "error": {"kind": "no-live-generation"}})
        self.assertEqual(made, [])

    def test_login_failure_is_failure_because_the_transaction_was_not_sent(self):
        session, _, made = self.session((2, 9000), {"connect": ConnectionRefusedError("refused")})
        reply = session.txn([["append", 1, 1]])
        self.assertEqual((reply["type"], reply["generation"], reply["error"]["kind"]), ("fail", 2, "connect"))
        self.assertTrue(made[0].closed)

    def test_timeout_or_reset_after_send_is_unknown_and_drops_the_connection(self):
        for error in (socket.timeout("timed out"), ConnectionError("closed"), BrokenPipeError()):
            with self.subTest(error=error):
                session, _, made = self.session((1, 9000), {"execute": lambda port, code, e=error: e})
                reply = session.txn([["append", 1, 1]])
                self.assertEqual((reply["type"], reply["generation"], reply["error"]["kind"]),
                                 ("info", 1, "transport"))
                self.assertTrue(made[0].closed)
                self.assertIsNone(session.transport)

    def test_ok_carries_generation_and_raw_and_reconnects_on_new_generation(self):
        session, campaign, made = self.session((1, 9000), {"execute": lambda port, code: ok([[5]])})
        reply = session.txn([["r", 1, None]])
        self.assertEqual(reply, {"type": "ok", "value": [["r", 1, [5]]], "generation": 1, "raw": "[[5]]"})
        session.txn([["r", 1, None]])
        self.assertEqual(len(made), 1)
        campaign.live = (2, 9001)
        session.txn([["r", 1, None]])
        self.assertEqual([t.port for t in made], [9000, 9001])
        self.assertTrue(made[0].closed)


def op(process, kind, value, generation=None, error=None):
    row = {"process": process, "type": kind, "f": "txn", "value": value}
    if kind != "invoke":
        row["generation"] = generation
    if error:
        row["error"] = {"kind": error}
    return row


class SplitTests(unittest.TestCase):
    def test_pairs_follow_their_completion_generation_in_history_order(self):
        history = [op(0, "invoke", [["append", 1, 1]]), op(1, "invoke", [["append", 1, 2]]),
                   {"process": "nemesis", "type": "info", "f": "crash", "value": None},
                   op(0, "ok", [["append", 1, 1]], 0), op(1, "info", [["append", 1, 2]], 0, "transport"),
                   op(2, "invoke", [["r", 1, None]]), op(2, "fail", [["r", 1, None]], None, "no-live-generation"),
                   op(2, "invoke", [["r", 1, None]]), op(2, "ok", [["r", 1, []]], 1)]
        buckets, counts = jc.split_generations(history)
        self.assertEqual(buckets[0], [history[0], history[1], history[3], history[4]])
        self.assertEqual(buckets[1], [history[7], history[8]])
        self.assertEqual(counts, {"nemesis": 1, "unsent": 1})

    def test_unplaceable_or_unpaired_operations_refuse_to_split(self):
        cases = {"no generation": [op(0, "invoke", []), op(0, "info", [], None, "adapter-timeout")],
                 "unpaired": [op(0, "invoke", [])],
                 "orphan": [op(0, "ok", [], 0)],
                 "double": [op(0, "invoke", []), op(0, "invoke", [])]}
        for name, history in cases.items():
            with self.subTest(name=name):
                with self.assertRaises(ValueError):
                    jc.split_generations(history)

    def test_elle_history_matches_the_recorder_edn_form(self):
        ops = [op(3, "invoke", [["append", 1, 2], ["r", 1, None]]),
               op(3, "ok", [["append", 1, 2], ["r", 1, [2]]], 0)]
        self.assertEqual(jc.elle_history(ops),
                         "{:index 0 :process 3 :type :invoke :f :txn :value [[:append 1 2] [:r 1 nil]]}\n"
                         "{:index 1 :process 3 :type :ok :f :txn :value [[:append 1 2] [:r 1 [2]]]}\n")


class DurabilityTests(unittest.TestCase):
    def test_baseline_prefix_and_attempted_suffix_pass(self):
        self.assertEqual(jc.durability_violations({1: [1, 2]}, {1: [1, 2, 3], 2: [9]}, {(1, 2)},
                                                  {(1, 3), (2, 9)}), [])

    def test_corrupted_lost_reordered_resurrected_or_duplicated_tokens_fail(self):
        baseline, acked, attempted = {1: [1, 2], 2: [5]}, {(1, 1), (1, 2), (2, 5)}, {(1, 3)}
        cases = {"lost key": {1: [1, 2]},
                 "truncated": {1: [1], 2: [5]},
                 "reordered": {1: [2, 1], 2: [5]},
                 "corrupt token": {1: [1, 2, 99], 2: [5]},
                 "duplicate": {1: [1, 2, 3, 3], 2: [5]},
                 "foreign key": {1: [1, 2], 2: [5], 7: [1]}}
        for name, recovered in cases.items():
            with self.subTest(name=name):
                self.assertTrue(jc.durability_violations(baseline, recovered, acked, attempted))

    def test_acknowledged_token_missing_fails_even_without_a_baseline_entry(self):
        problems = jc.durability_violations({}, {}, {(4, 8)}, set())
        self.assertEqual(problems, ["key 4: token 8 acknowledged before the checkpoint is missing"])

    def test_comparisons_use_only_generations_since_the_checkpoint(self):
        buckets = {0: [op(0, "ok", [["append", 1, 1]], 0), op(1, "info", [["append", 1, 7]], 0)],
                   1: [op(0, "ok", [["append", 1, 2]], 1)],
                   2: [op(0, "info", [["append", 1, 3]], 2)]}
        lifecycle = [{"event": "admit", "generation": 0, "kind": "initial", "baseline": None, "recovered": {}},
                     {"event": "admit", "generation": 2, "kind": "checkpoint", "baseline_generation": 1,
                      "baseline": {"1": [1, 2]}, "recovered": {"1": [1, 2]}},
                     {"event": "admit", "generation": 3, "kind": "crash", "baseline_generation": 1,
                      "baseline": {"1": [1, 2]}, "recovered": {"1": [1, 2, 3, 7]}}]
        rows = jc.checkpoint_comparisons(buckets, lifecycle)
        self.assertEqual([(row["generation"], row["kind"]) for row in rows], [(2, "checkpoint"), (3, "crash")])
        self.assertEqual(rows[0]["violations"], [])
        self.assertEqual(rows[0]["acknowledged_tokens"], 1)
        # Token 3 was attempted after the checkpoint; token 7 only before it.
        self.assertEqual(rows[1]["violations"],
                         ["key 1: recovered token 7 was never attempted after the checkpoint"])


class AnalyzeTests(unittest.TestCase):
    def directory(self, history, lifecycle):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        for name, rows in (("history.jsonl", history), ("lifecycle.jsonl", lifecycle)):
            if rows is not None:
                (root / name).write_text("".join(json.dumps(row) + "\n" for row in rows))
        return root

    def elle(self, verdict="true", code=0):
        def run(command, log):
            return code, f"{command[-1]} {verdict}\n"
        return patch.object(jc.check_elle, "run", side_effect=run)

    def test_zero_checkpoint_comparisons_is_incomplete(self):
        root = self.directory([op(0, "invoke", [["append", 1, 1]]), op(0, "ok", [["append", 1, 1]], 0)],
                              [{"event": "admit", "generation": 0, "kind": "initial", "baseline": None,
                                "recovered": {}}])
        with self.elle():
            results, errors = jc.analyze(root, root, Path("elle.jar"), "java")
        self.assertIn("incomplete campaign: zero completed checkpoint comparisons", errors)
        self.assertEqual(results["elle"][0]["verdict"], "true")
        self.assertTrue((root / "gen-000.edn").is_file())

    def complete_run(self):
        history = [op(0, "invoke", [["append", 1, 1]]), op(0, "ok", [["append", 1, 1]], 0),
                   op(1, "invoke", [["append", 1, 2]]), op(1, "info", [["append", 1, 2]], 1, "transport")]
        lifecycle = [{"event": "admit", "generation": 0, "kind": "initial", "baseline": None, "recovered": {}},
                     {"event": "admit", "generation": 1, "kind": "checkpoint", "baseline_generation": 0,
                      "baseline": {"1": [1]}, "recovered": {"1": [1]}}]
        return history, lifecycle

    def test_complete_healthy_run_has_no_errors_and_counts_unknowns(self):
        root = self.directory(*self.complete_run())
        with self.elle():
            results, errors = jc.analyze(root, root, Path("elle.jar"), "java")
        self.assertEqual(errors, [])
        self.assertEqual(results["outcomes"], {"ok": 1, "info:transport": 1})
        self.assertEqual(results["elle"][1], {"generation": 1, "skipped": "no :ok transactions"})

    def test_elle_rejection_native_errors_and_lifecycle_failures_fail(self):
        history, lifecycle = self.complete_run()
        root = self.directory(history, lifecycle)
        with self.elle("false", 1):
            _, errors = jc.analyze(root, root, Path("elle.jar"), "java")
        self.assertIn("generation 0: Elle exit=1, verdict=false", errors)
        history[-1] = op(1, "info", [["append", 1, 2]], 1, "native-error")
        root = self.directory(history, lifecycle + [{"event": "failure", "error": "crash: boom"}])
        with self.elle():
            _, errors = jc.analyze(root, root, Path("elle.jar"), "java")
        self.assertEqual(errors, ["crash: boom", "1 client completions were info:native-error"])

    def test_missing_history_is_incomplete(self):
        root = self.directory(None, [{"event": "failure", "error": "jepsen: exited 1"}])
        self.assertEqual(jc.analyze(root, root, Path("elle.jar"), "java"),
                         ({}, ["jepsen: exited 1", "incomplete campaign: Jepsen wrote no history.jsonl"]))


class FakeServer:
    def __init__(self, executable, db, run_dir, port, log):
        self.db, self.run_dir, self.port, self.log = db, run_dir, port, log
        self.paused, self.identity, self.proc = False, {"db": str(db)}, SimpleNamespace(poll=lambda: None)
        self.verified = True

    def start(self):
        self.log.append(("start", self.db.name, self.db.read_bytes(), self.port))

    def pause(self):
        self.paused = True

    def resume(self):
        self.log.append(("resume", self.port))
        self.paused = False

    def crash(self):
        self.log.append(("crash", self.port))
        self.proc = SimpleNamespace(poll=lambda: -9)
        return {"exit_code": -9, "forced": True}

    def stop(self):
        self.log.append(("stop", self.port))
        self.proc = SimpleNamespace(poll=lambda: 0)
        if not self.verified:
            return {"exit_code": 0, "final_checkpoint": {"verified": False, "reason": "bad pair"}}
        database, sidecar = Path(str(self.db) + ".new"), Path(str(self.db) + ".new.waifids")
        database.write_bytes(b"checkpoint of " + self.db.read_bytes())
        sidecar.write_bytes(b"sidecar")
        return {"exit_code": 0, "final_checkpoint": {"verified": True, "outputs": {
            "database": {"path": str(database), "sha256": jc.barn_process.bench.sha256_file(database)},
            "waif_identities": {"path": str(sidecar), "sha256": jc.barn_process.bench.sha256_file(sidecar)}}}}


class CampaignTests(unittest.TestCase):
    def campaign(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "Test.db").write_bytes(b"fixture")
        log, servers, ports = [], [], iter(range(9000, 9100))
        ledger = {9000: {}}
        def evaluate(port, code):
            if code == jc.ADMIT_CODE:
                return ok(ledger.get(port, {1: [1]}))
            return ok({1: [1]})
        script = {"execute": evaluate}
        def server_class(*args):
            servers.append(FakeServer(*args, log))
            return servers[-1]
        output = root / "run"
        output.mkdir()
        campaign = jc.Campaign(root / "barn", root / "Test.db", output,
                               lambda port: FakeTransport(port, script), server_class=server_class,
                               port_source=lambda: next(ports))
        return campaign, log, servers, output

    def test_generations_launch_from_the_fixture_then_the_latest_verified_pair(self):
        campaign, log, servers, output = self.campaign()
        campaign.launch("initial")
        self.assertEqual(campaign.live, (0, 9000))
        servers[0].paused = True
        self.assertEqual(campaign.fault("checkpoint")["value"], {"checkpointed": 0, "generation": 1})
        self.assertEqual(log[1], ("resume", 9000))  # SIGTERM never goes to a frozen process
        self.assertEqual(campaign.fault("crash")["value"], {"crashed": 1, "generation": 2})
        starts = [entry for entry in log if entry[0] == "start"]
        self.assertEqual([entry[2] for entry in starts],
                         [b"fixture", b"checkpoint of fixture", b"checkpoint of fixture"])
        self.assertEqual((output / "gen-002" / "input.db.waifids").read_bytes(), b"sidecar")
        rows = jc.load_jsonl(output / "lifecycle.jsonl")
        admits = [row for row in rows if row["event"] == "admit"]
        self.assertEqual([(row["kind"], row["baseline_generation"], row["baseline"]) for row in admits],
                         [("initial", None, None), ("checkpoint", 0, {"1": [1]}), ("crash", 0, {"1": [1]})])
        self.assertEqual(admits[1]["recovered"], {"1": [1]})

    def test_unverified_checkpoint_fails_the_campaign_without_relaunch(self):
        campaign, log, servers, _ = self.campaign()
        campaign.launch("initial")
        servers[0].verified = False
        reply = campaign.fault("checkpoint")
        self.assertEqual((reply["type"], reply["error"]["kind"]), ("info", "lifecycle"))
        self.assertIsNone(campaign.live)
        self.assertEqual(len(servers), 1)
        self.assertEqual(campaign.fault("pause")["error"]["kind"], "campaign-failed")
        self.assertIn("not verified", campaign.failures[0])

    def test_tampered_verified_pair_is_refused_before_launch(self):
        campaign, log, servers, _ = self.campaign()
        campaign.launch("initial")
        campaign.fault("checkpoint")
        Path(campaign.source["database"]).write_bytes(b"tampered")
        reply = campaign.fault("crash")
        self.assertEqual(reply["type"], "info")
        self.assertIn("does not match its verified source", campaign.failures[0])
        self.assertEqual(len(servers), 2)

    def test_finish_resumes_and_stops_only_a_live_owned_server(self):
        campaign, log, servers, _ = self.campaign()
        campaign.launch("initial")
        servers[0].paused = True
        campaign.finish()
        self.assertEqual(log[-2:], [("resume", 9000), ("stop", 9000)])
        campaign.finish()  # already exited: nothing is signalled
        self.assertEqual(log[-1], ("stop", 9000))


if __name__ == "__main__":
    unittest.main()
