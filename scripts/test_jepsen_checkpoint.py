"""Pure fixtures for the Jepsen checkpoint adapter: never launch Barn, Jepsen or Elle."""
import contextlib
import copy
import io
import json
from pathlib import Path
import re
import socket
import tempfile
import threading
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import jepsen_checkpoint as jc

# ROWS_CODE's reply: property-row objects #20-#23, keyed parent #11, holder
# #12 with its own map, holder #13 with a clear inherited slot.
LAYOUT = [20, 21, 22, 23, 11, 12, 13]
PROPS, KEYED = jc.FAMILIES["props"]["base"], jc.FAMILIES["keyed"]["base"]


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

    def test_observation_reads_every_listed_row_without_writing(self):
        layout = jc.parse_layout(LAYOUT)
        code = jc.observation_code([2, PROPS + 5, KEYED + 1], layout)
        self.assertEqual(code, jc.txn_code([["r", 2, None], ["r", PROPS + 5, None], ["r", KEYED + 1, None]], layout))
        self.assertNotIn("#0.jepsen_rows =", code)
        self.assertNotIn("add_property", code)
        self.assertNotIn("jm[20000001] =", code)

    def test_row_layout_is_seven_object_numbers(self):
        self.assertEqual(jc.parse_layout(LAYOUT), {"props": [20, 21, 22, 23], "parent": 11, "keyed": [12, 13]})
        for value in (LAYOUT[:6], LAYOUT + [1], [str(n) for n in LAYOUT], None):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    jc.parse_layout(value)

    def test_families_partition_keys_by_range(self):
        self.assertEqual([jc.family_of(key) for key in (0, 1000004, PROPS, PROPS + 1000003, KEYED + 7)],
                         ["smoke", "smoke", "props", "props", "keyed"])
        self.assertIsNone(jc.family_of(3 * jc.FAMILY_SPAN))
        with self.assertRaises(ValueError):
            jc.txn_code([["r", 3 * jc.FAMILY_SPAN, None]], jc.parse_layout(LAYOUT))
        with self.assertRaises(ValueError):
            jc.txn_code([["r", PROPS, None]])  # property rows need the generation's layout

    def test_property_rows_read_and_append_their_own_property_directly(self):
        # Keys 10000005 and 10000002: objects 5 % 4 and 2 % 4 of the layout.
        code = jc.txn_code([["r", PROPS + 5, None], ["append", PROPS + 5, 9], ["r", PROPS + 5, None],
                            ["append", PROPS + 2, 3]], jc.parse_layout(LAYOUT))
        self.assertEqual(code, " ".join([
            "out = {};",
            "out = {@out, `#21.jp10000005 ! E_PROPNF => {}'};",
            "try #21.jp10000005 = {@#21.jp10000005, 9}; "
            "except (E_PROPNF) add_property(#21, \"jp10000005\", {9}, {#0, \"r\"}); endtry",
            "out = {@out, `#21.jp10000005 ! E_PROPNF => {}'};",
            "try #22.jp10000002 = {@#22.jp10000002, 3}; "
            "except (E_PROPNF) add_property(#22, \"jp10000002\", {3}, {#0, \"r\"}); endtry",
            "return out;"]))

    def test_keyed_rows_by_mode(self):
        layout = jc.parse_layout(LAYOUT)
        txn = [["r", KEYED + 1, None], ["append", KEYED + 1, 4]]  # odd key: the inherited-slot holder #13
        self.assertEqual(jc.txn_code(txn, layout, 4), " ".join([
            "out = {};", "out = {@out, (maphaskey(#13.jm, 20000001) ? #13.jm[20000001] | {})};",
            "#13.jm[20000001] = {@(maphaskey(#13.jm, 20000001) ? #13.jm[20000001] | {}), 4};", "return out;"]))
        self.assertEqual(jc.txn_code(txn, layout, 7), jc.txn_code(txn, layout, 4))
        self.assertEqual(jc.txn_code(txn, layout, 1), " ".join([
            "out = {};", "out = {@out, `#13.jm[20000001] ! E_RANGE => {}'};",
            "#13.jm[20000001] = {@`#13.jm[(#13.jm[20000001] = {}) == {} ? 20000001 | 20000001] "
            "! E_RANGE => {}', 4};", "return out;"]))
        self.assertEqual(jc.txn_code(txn, layout, 2), " ".join([
            "out = {};", "m = #13.jm; out = {@out, (maphaskey(m, 20000001) ? m[20000001] | {})};",
            "m = #13.jm; m[20000001] = {@(maphaskey(m, 20000001) ? m[20000001] | {}), 4}; #13.jm = m;",
            "return out;"]))

    def test_keyed_fixture_is_the_generated_code(self):
        # vm/jepsen_keyed_rows_test.go runs these: the keyed pair on two entries
        # of one map both commit, the whole-map pair loses validation.
        fixture = json.loads((Path(jc.__file__).parent / "testdata" / "jepsen_keyed_txns.json").read_text())
        self.assertEqual(fixture["layout"], LAYOUT)
        layout = jc.parse_layout(fixture["layout"])
        self.assertEqual([case["name"] for case in fixture["cases"]], ["keyed", "whole-map"])
        for case in fixture["cases"]:
            for row in case["txns"]:
                with self.subTest(case=case["name"], op_id=row["op_id"]):
                    self.assertEqual(jc.txn_code(row["txn"], layout, row["op_id"]), row["code"])

    def test_keyed_modes_touch_the_map_only_through_keyed_shapes(self):
        # The compiler emits keyed access only for o.jm[K], maphaskey(o.jm, K)
        # and o.jm[K] = v (compiler/lower_expressions.go compileIndex and
        # keyedMaphaskeyProperty, compiler/lower_assignments.go
        # compileIndexAssign); any other mention of o.jm is a whole read.
        layout = jc.parse_layout(LAYOUT)
        txn = [["r", KEYED, None], ["append", KEYED, 1], ["r", KEYED + 1, None], ["append", KEYED + 3, 2]]
        for op_id in (0, 1, 3):
            code = jc.txn_code(txn, layout, op_id)
            with self.subTest(op_id=op_id):
                mentions = re.findall(r"(maphaskey\()?(#1[23]\.jm)(.)", code)
                self.assertTrue(mentions)
                self.assertTrue(all(call or after == "[" for call, _, after in mentions), mentions)
                self.assertNotIn("jepsen_rows", code)
                self.assertNotIn("jepsen_ledger", code)
        self.assertIn("m = #12.jm;", jc.txn_code(txn, layout, 2))

    def test_new_families_never_touch_the_common_rows_or_ledger(self):
        layout = jc.parse_layout(LAYOUT)
        for op_id in range(4):
            code = jc.txn_code([["append", PROPS + 1, 1], ["r", KEYED, None], ["append", KEYED, 2]], layout, op_id)
            self.assertNotIn("#0.", code)


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


class FakeCampaign:
    """What ClientSession uses of Campaign: live generation, journal, touched keys."""

    def __init__(self, live, factory):
        self.lock, self.live, self.transport_factory = threading.Lock(), live, factory
        self.events, self.touched, self.layouts = [], {}, {}

    def journal(self, row):
        self.events.append(dict(row))

    def phase(self):
        return {"fault_seq": 3, "fault": None, "paused": False}

    def touch(self, generation, keys):
        self.touched.setdefault(generation, set()).update(keys)


class SessionTests(unittest.TestCase):
    def session(self, live, script):
        made = []
        def factory(port):
            made.append(FakeTransport(port, script))
            return made[-1]
        campaign = FakeCampaign(live, factory)
        return jc.ClientSession(campaign), campaign, made

    def test_no_live_generation_is_an_unsent_failure(self):
        session, campaign, made = self.session(None, {})
        self.assertEqual(session.txn(1, 0, [["append", 1, 1]]),
                         {"type": "fail", "generation": None, "error": {"kind": "no-live-generation"}})
        self.assertEqual(made, [])
        self.assertEqual([row["event"] for row in campaign.events], ["request", "complete"])

    def test_login_failure_is_failure_because_the_transaction_was_not_sent(self):
        session, campaign, made = self.session((2, 9000), {"connect": ConnectionRefusedError("refused")})
        reply = session.txn(1, 0, [["append", 1, 1]])
        self.assertEqual((reply["type"], reply["generation"], reply["error"]["kind"]), ("fail", 2, "connect"))
        self.assertTrue(made[0].closed)
        self.assertNotIn("send", [row["event"] for row in campaign.events])

    def test_timeout_or_reset_after_send_is_unknown_and_drops_the_connection(self):
        for error in (socket.timeout("timed out"), ConnectionError("closed"), BrokenPipeError()):
            with self.subTest(error=error):
                session, campaign, made = self.session((1, 9000), {"execute": lambda port, code, e=error: e})
                reply = session.txn(4, 0, [["append", 1, 1]])
                self.assertEqual((reply["type"], reply["generation"], reply["error"]["kind"]),
                                 ("info", 1, "transport"))
                self.assertTrue(made[0].closed)
                self.assertIsNone(session.transport)
                self.assertEqual([(row["event"], row.get("type")) for row in campaign.events],
                                 [("request", None), ("send", None), ("complete", "info")])

    def test_send_is_journaled_with_generation_code_and_phase_before_execute(self):
        seen = []
        session, campaign, _ = self.session((1, 9000), {"execute": lambda port, code: (
            seen.append([row["event"] for row in campaign.events]), ok([[5]]))[1]})
        session.txn(7, 3, [["r", 2, None]])
        self.assertEqual(seen, [["request", "send"]])
        request, send, complete = campaign.events
        self.assertEqual((request["id"], request["client"], request["value"]), (7, 3, [["r", 2, None]]))
        self.assertEqual((send["id"], send["generation"], send["port"], send["code"], send["phase"]),
                         (7, 1, 9000, jc.txn_code([["r", 2, None]]),
                          {"fault_seq": 3, "fault": None, "paused": False}))
        self.assertEqual((complete["id"], complete["type"], complete["value"], complete["raw"]),
                         (7, "ok", [["r", 2, [5]]], "[[5]]"))
        self.assertEqual(campaign.touched, {1: {2}})

    def test_ok_carries_generation_and_raw_and_reconnects_on_new_generation(self):
        session, campaign, made = self.session((1, 9000), {"execute": lambda port, code: ok([[5]])})
        reply = session.txn(1, 0, [["r", 1, None]])
        self.assertEqual(reply, {"type": "ok", "value": [["r", 1, [5]]], "generation": 1, "raw": "[[5]]"})
        session.txn(2, 0, [["r", 1, None]])
        self.assertEqual(len(made), 1)
        campaign.live = (2, 9001)
        session.txn(3, 0, [["r", 1, None]])
        self.assertEqual([t.port for t in made], [9000, 9001])
        self.assertTrue(made[0].closed)

    def test_row_family_code_uses_the_live_generation_layout_and_operation_id(self):
        script = {"execute": lambda port, code: ok([[1]])}
        session, campaign, _ = self.session((2, 9000), script)
        campaign.layouts = {1: [99] * 7, 2: jc.parse_layout(LAYOUT)}
        txn = [["r", KEYED, None]]
        session.txn(6, 0, txn)
        self.assertEqual(script["codes"], [(9000, jc.txn_code(txn, jc.parse_layout(LAYOUT), 6))])
        self.assertIn("m = #12.jm;", script["codes"][0][1])


def op(process, kind, value, generation=None, error=None, op_id=None):
    row = {"process": process, "type": kind, "f": "txn", "value": value}
    if kind != "invoke":
        row["generation"] = generation
        if op_id is not None:
            row["op-id"] = op_id
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


class Run:
    """A synthetic campaign directory: Jepsen history, adapter journal, lifecycle, logs, provenance."""

    def __init__(self):
        self.history, self.journal, self.lifecycle = [], [], []
        self.logs, self.server_logs = {}, {}
        self.next_id = 0
        self.provenance = {"fixture": "synthetic"}

    def client(self, process, generation, txn, completed=None, kind="ok", error=None):
        """One client operation, journaled the way ClientSession journals it."""
        self.next_id += 1
        op_id = self.next_id
        value = completed if kind == "ok" else txn
        self.history.append(op(process, "invoke", txn))
        self.history.append(op(process, kind, value, generation, error, op_id))
        self.journal.append({"event": "request", "id": op_id, "client": process, "value": txn})
        if generation is not None:
            self.journal.append({"event": "send", "id": op_id, "generation": generation, "port": 9000,
                                 "code": jc.txn_code(txn, jc.parse_layout(LAYOUT), op_id),
                                 "phase": {"fault_seq": 0, "fault": None,
                                                                     "paused": False}})
        done = {"event": "complete", "id": op_id, "type": kind, "generation": generation}
        if kind == "ok":
            done["value"] = value
        if error:
            done["error"] = {"kind": error}
        self.journal.append(done)
        return op_id

    def event(self, **row):
        self.lifecycle.append(row)
        return row

    def admit(self, generation, kind, baseline_generation, baseline, recovered, probe=True):
        self.event(event="admit", generation=generation, kind=kind, baseline_generation=baseline_generation,
                   baseline=baseline, recovered=recovered)
        self.logs[generation] = [{"level": "INFO", "msg": "database loaded"},
                                 {"level": "WARN", "msg": "repaired object reference", "src": "startup_repair"}]
        self.server_logs[generation] = 'time=now level=INFO msg="database loaded"\n'
        if probe:
            self.probe(generation, "launch")

    def probe(self, generation, reason, ok=True):
        return self.event(event="probe", generation=generation, reason=reason, ok=ok,
                          failed_attempts=[] if ok else ["ConnectionRefusedError: refused"],
                          deadline_seconds=30.0)

    def observe(self, generation, reason, keys, value, ok=True):
        return self.event(event="observation", generation=generation, reason=reason, keys=keys,
                          value=value if ok else None, ok=ok, deadline_seconds=30.0,
                          failed_attempts=[] if ok else ["TimeoutError: timed out"])

    def crash(self, generation, exit_code=-9):
        return self.event(event="crash", generation=generation, result={"exit_code": exit_code, "forced": True})

    def checkpoint(self, generation, baseline, verified=True):
        return self.event(event="checkpoint", generation=generation, baseline=baseline,
                          result={"exit_code": 0, "final_checkpoint": {"verified": verified}})

    def write(self, root):
        root.mkdir(parents=True, exist_ok=True)
        for name, rows in (("history.jsonl", self.history), ("journal.jsonl", self.journal),
                           ("lifecycle.jsonl", self.lifecycle)):
            if rows is not None:
                (root / name).write_text("".join(json.dumps(row) + "\n" for row in rows), encoding="utf-8")
        if self.provenance is not None:
            (root / "provenance.json").write_text(json.dumps(self.provenance), encoding="utf-8")
        for generation, records in self.logs.items():
            directory = root / f"gen-{generation:03d}"
            (directory / "logs").mkdir(parents=True, exist_ok=True)
            if records is not None:
                (directory / "logs" / "latest.jsonl").write_text(
                    "".join(json.dumps(row) + "\n" for row in records), encoding="utf-8")
            if self.server_logs.get(generation) is not None:
                (directory / "server.log").write_text(self.server_logs[generation], encoding="utf-8")
        return root


def healthy_run():
    """The fixed scenario: pause/resume, checkpoint, crash, final checkpoint, each with its evidence."""
    run = Run()
    run.admit(0, "initial", None, None, {})
    run.client(0, 0, [["append", 1, 1], ["r", 1, None]], [["append", 1, 1], ["r", 1, [1]]])
    run.event(event="pause", generation=0, observed_state="T")
    run.event(event="resume", generation=0)
    run.probe(0, "resume")
    run.client(1, 0, [["append", 1, 2]], kind="info", error="transport")  # Barn never replied
    run.observe(0, "checkpoint", [1], [[1, 2]])
    run.checkpoint(0, {"1": [1, 2]})
    run.admit(1, "checkpoint", 0, {"1": [1, 2]}, {"1": [1, 2]})
    run.client(9, 1, [["append", 1, 3], ["r", 1, None]], [["append", 1, 3], ["r", 1, [3]]])
    run.observe(1, "crash", [1], [[3]])
    run.crash(1)
    run.admit(2, "crash", 0, {"1": [1, 2]}, {"1": [1, 2]})
    run.client(9, 2, [["append", 1, 4], ["r", 1, None]], [["append", 1, 4], ["r", 1, [4]]])
    run.event(event="resume", generation=2)
    run.probe(2, "resume")
    run.observe(2, "checkpoint", [1], [[4]])
    run.checkpoint(2, {"1": [1, 2, 4]})
    run.admit(3, "checkpoint", 2, {"1": [1, 2, 4]}, {"1": [1, 2, 4]})
    return run


def family_run(keyed_write_after_crash=True):
    """healthy_run with property and keyed rows selected, each read and appended in every client generation."""
    run = Run()
    run.provenance = {"fixture": "synthetic", "parameters": {"workloads": ["smoke", "props", "keyed"]}}
    for row in healthy_run().lifecycle:
        run.lifecycle.append(row)
    base = healthy_run()
    run.history, run.journal, run.logs, run.server_logs = [], [], base.logs, base.server_logs
    smoke = {0: [[["append", 1, 1], ["r", 1, None]], [["append", 1, 1], ["r", 1, [1]]]],
             1: [[["append", 1, 3], ["r", 1, None]], [["append", 1, 3], ["r", 1, [3]]]],
             2: [[["append", 1, 4], ["r", 1, None]], [["append", 1, 4], ["r", 1, [4]]]]}
    for generation in (0, 1, 2):
        run.client(0, generation, *smoke[generation])
        for process, base_key in ((20, PROPS), (21, KEYED)):
            key = base_key + generation
            if base_key == KEYED and generation == 2 and not keyed_write_after_crash:
                run.client(process, generation, [["r", key, None]], [["r", key, []]])
                continue
            run.client(process, generation, [["append", key, 7], ["r", key, None]], [["append", key, 7], ["r", key, [7]]])
    return run


class AnalyzeTests(unittest.TestCase):
    def directory(self, run):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        return run.write(Path(temporary.name))

    def elle(self, verdict="true", code=0):
        def run(command, log):
            return code, f"{command[-1]} {verdict}\n"
        return patch.object(jc.check_elle, "run", side_effect=run)

    def analyze(self, run, **elle):
        root = self.directory(run)
        with self.elle(**elle):
            results, findings = jc.analyze(root, root, Path("elle.jar"), "java")
        return results, findings, root

    def classes(self, findings):
        return {(finding["class"], finding["message"]) for finding in findings}

    def assertFinding(self, findings, kind, fragment):
        matches = [f for f in findings if f["class"] == kind and fragment in f["message"]]
        self.assertTrue(matches, f"no {kind} finding containing {fragment!r} in {findings}")

    def test_complete_healthy_run_has_no_findings_and_keeps_unknowns(self):
        results, findings, root = self.analyze(healthy_run())
        self.assertEqual(findings, [])
        self.assertEqual(jc.verdict_of(findings), "healthy")
        self.assertEqual(results["outcomes"], {"ok": 3, "info:transport": 1})
        self.assertEqual(results["missing_obligations"], [])
        self.assertEqual([(row["generation"], row["verdict"]) for row in results["elle"]],
                         [(0, "true"), (1, "true"), (2, "true")])
        self.assertEqual([len(row["violations"]) for row in results["checkpoints"]], [0, 0, 0])
        edn = (root / "gen-000.edn").read_text()
        self.assertIn(":process 1 :type :info :f :txn :value [[:append 1 2]]", edn)
        # The final observation is a read-only transaction of the same generation.
        self.assertIn(f":process {jc.OBSERVER_PROCESS} :type :ok :f :txn :value [[:r 1 [1 2]]]", edn)

    def test_selected_families_each_count_reads_and_writes_and_stay_out_of_the_ledger_check(self):
        results, findings, root = self.analyze(family_run())
        # Property and keyed tokens are not mirrored in #0.jepsen_ledger; counting
        # them would report every acknowledged one as lost.
        self.assertEqual(findings, [])
        self.assertEqual([row["acknowledged_tokens"] for row in results["checkpoints"]], [1, 1, 1])
        self.assertEqual(results["families"], {"selected": ["smoke", "props", "keyed"], "totals": {
            "keyed": {"ok_reads": 3, "ok_writes": 3}, "props": {"ok_reads": 3, "ok_writes": 3},
            "smoke": {"ok_reads": 3, "ok_writes": 3}}})
        self.assertEqual(results["generations"][1]["families"]["keyed"], {"ok_reads": 1, "ok_writes": 1})
        self.assertIn(f"[[:append {KEYED + 1} 7] [:r {KEYED + 1} [7]]]", (root / "gen-001.edn").read_text())

    def test_selected_family_without_a_write_after_a_crash_is_incomplete(self):
        results, findings, _ = self.analyze(family_run(keyed_write_after_crash=False))
        self.assertEqual(jc.verdict_of(findings), jc.INCOMPLETE)
        self.assertEqual(results["missing_obligations"], [
            "after crash of generation 1: generation 2 has no successful keyed transaction with a write",
            "before checkpoint of generation 2: generation 2 has no successful keyed transaction with a write"])
        self.assertEqual(results["families"]["totals"]["keyed"], {"ok_reads": 3, "ok_writes": 2})

    def test_unselected_family_is_not_an_obligation_and_unknown_one_is_invalid(self):
        run = family_run(keyed_write_after_crash=False)
        run.provenance["parameters"]["workloads"] = ["smoke", "props"]
        _, findings, _ = self.analyze(run)
        self.assertEqual(findings, [])
        run.provenance["parameters"]["workloads"] = ["smoke", "maps"]
        _, findings, _ = self.analyze(run)
        self.assertEqual(findings, [{"class": jc.INVALID, "message": "provenance names unknown row families ['maps']"}])

    def test_post_crash_generation_without_success_no_longer_passes(self):
        # Generation 0 alone had a successful transaction. The aggregate
        # minimum (one comparison, one Elle-checked generation) was met.
        run = Run()
        run.admit(0, "initial", None, None, {})
        run.client(0, 0, [["append", 1, 1], ["r", 1, None]], [["append", 1, 1], ["r", 1, [1]]])
        run.event(event="pause", generation=0, observed_state="T")
        run.event(event="resume", generation=0)
        run.probe(0, "resume")
        run.observe(0, "checkpoint", [1], [[1]])
        run.checkpoint(0, {"1": [1]})
        run.admit(1, "checkpoint", 0, {"1": [1]}, {"1": [1]})
        run.client(2, 1, [["append", 1, 2]], kind="info", error="transport")
        run.observe(1, "crash", [1], [[]])
        run.crash(1)
        run.admit(2, "crash", 0, {"1": [1]}, {"1": [1]})
        run.client(3, 2, [["append", 1, 3]], kind="info", error="transport")
        run.observe(2, "checkpoint", [1], [[]])
        run.checkpoint(2, {"1": [1]})
        run.admit(3, "checkpoint", 2, {"1": [1]}, {"1": [1]})
        results, findings, _ = self.analyze(run)
        self.assertTrue(results["checkpoints"])
        self.assertEqual(results["elle"][0]["verdict"], "true")
        self.assertEqual(jc.verdict_of(findings), jc.INCOMPLETE)
        self.assertFinding(findings, jc.INCOMPLETE,
                           "after crash of generation 1: generation 2 has no successful smoke transaction with a read")
        self.assertFinding(findings, jc.INCOMPLETE, "before crash of generation 1: generation 1 has no successful")
        self.assertFinding(findings, jc.INCOMPLETE, "generation 2: 1 client completions and no successful transaction")
        # Only the campaign's own observation makes generation 2 checkable.
        self.assertEqual(results["generations"][2], {"completions": 1, "ok": 0, "ok_reads": 0, "ok_writes": 0,
                                                     "families": {}})

    def test_barn_missing_reply_stays_unknown_in_its_generation(self):
        run = healthy_run()
        run.client(4, 1, [["append", 2, 9]], kind="info", error="transport")
        results, findings, root = self.analyze(run)
        self.assertEqual(findings, [])
        self.assertIn(":type :info :f :txn :value [[:append 2 9]]", (root / "gen-001.edn").read_text())

    def test_adapter_reply_missing_stays_pending_in_the_send_generation(self):
        run = healthy_run()
        op_id = run.client(4, 1, [["append", 2, 9]], kind="info", error="transport")
        run.history[-1] = op(4, "info", [["append", 2, 9]], None, "adapter-timeout", op_id)
        run.journal = [row for row in run.journal if not (row["id"] == op_id and row["event"] == "complete")]
        results, findings, root = self.analyze(run)
        self.assertEqual(results["reconciliation"]["pending"], 1)
        self.assertIn(":process 4 :type :info :f :txn :value [[:append 2 9]]", (root / "gen-001.edn").read_text())
        # The adapter hung past Jepsen's timeout: harness evidence, not a Barn failure.
        self.assertEqual(self.classes(findings), {(jc.INVALID, "1 client completions were info:adapter-timeout")})

    def test_late_reply_after_jepsen_gave_up_stays_unknown(self):
        run = healthy_run()
        op_id = run.client(4, 1, [["append", 2, 9], ["r", 2, None]], [["append", 2, 9], ["r", 2, [9]]])
        run.history[-1] = op(4, "info", [["append", 2, 9], ["r", 2, None]], None, "adapter-timeout", op_id)
        results, findings, root = self.analyze(run)
        self.assertEqual(results["reconciliation"]["late"], 1)
        self.assertIn(":process 4 :type :info :f :txn :value [[:append 2 9] [:r 2 nil]]",
                      (root / "gen-001.edn").read_text())
        self.assertFalse([f for f in findings if "contradicts" in f["message"]])

    def test_unsent_operation_is_excluded_and_a_sent_failure_is_a_contradiction(self):
        run = healthy_run()
        run.client(5, None, [["append", 3, 1]], kind="fail", error="no-live-generation")
        run.client(6, 1, [["append", 3, 2]], kind="info", error="adapter-io")
        run.history[-1]["generation"] = None
        run.journal = [row for row in run.journal if not (row["id"] == run.next_id and row["event"] != "request")]
        results, findings, _ = self.analyze(run)
        self.assertEqual((results["reconciliation"]["unsent"], results["split"]["unsent"]), (2, 2))
        self.assertEqual(self.classes(findings), {(jc.INVALID, "1 client completions were info:adapter-io")})
        run = healthy_run()
        op_id = run.client(5, None, [["append", 3, 1]], kind="fail", error="connect")
        run.journal.insert(-1, {"event": "send", "id": op_id, "generation": 1, "port": 9000, "code": "", "phase": {}})
        _, findings, _ = self.analyze(run)
        self.assertFinding(findings, jc.INVALID, "Jepsen recorded fail for an operation sent in generation 1")

    def test_mixed_generation_is_invalid_harness_evidence(self):
        run = healthy_run()
        op_id = run.client(4, 1, [["r", 1, None]], [["r", 1, [3]]])
        run.history[-1]["generation"] = 2
        _, findings, _ = self.analyze(run)
        self.assertEqual(jc.verdict_of(findings), jc.INVALID)
        self.assertFinding(findings, jc.INVALID,
                           f"operation {op_id}): Jepsen generation 2, adapter sent it in generation 1")

    def test_journal_disagreeing_with_an_ok_value_or_missing_is_invalid(self):
        run = healthy_run()
        run.journal[2]["value"] = [["append", 1, 1], ["r", 1, [99]]]
        _, findings, _ = self.analyze(run)
        self.assertFinding(findings, jc.INVALID, "journal recorded ok")
        run = healthy_run()
        run.journal = None
        root = self.directory(run)
        with self.elle():
            _, findings = jc.analyze(root, root, Path("elle.jar"), "java")
        self.assertFinding(findings, jc.INVALID, "no adapter journal.jsonl")

    def test_no_post_heal_progress_is_a_recovery_failure(self):
        run = healthy_run()
        next(row for row in run.lifecycle if row.get("reason") == "resume").update(
            ok=False, failed_attempts=["TimeoutError: timed out"])
        _, findings, _ = self.analyze(run)
        self.assertEqual(jc.verdict_of(findings), jc.RECOVERY)
        self.assertFinding(findings, jc.RECOVERY, "progress probe after resume of generation 0 did not succeed")
        run = healthy_run()
        run.lifecycle = [row for row in run.lifecycle if not (row.get("event") == "probe" and row["generation"] == 2
                                                              and row["reason"] == "launch")]
        _, findings, _ = self.analyze(run)
        self.assertFinding(findings, jc.INCOMPLETE, "missing progress probe after launch of generation 2")

    def test_zero_actual_fault_is_incomplete(self):
        run = healthy_run()
        next(row for row in run.lifecycle if row.get("event") == "crash")["result"]["exit_code"] = 0
        _, findings, _ = self.analyze(run)
        self.assertFinding(findings, jc.INCOMPLETE, "crash of generation 1: SIGKILL was not observed")
        run = healthy_run()
        next(row for row in run.lifecycle if row.get("event") == "pause")["observed_state"] = "S"
        _, findings, _ = self.analyze(run)
        self.assertFinding(findings, jc.INCOMPLETE, "pause of generation 0: owned PID was not observed stopped")
        run = healthy_run()
        run.lifecycle = [row for row in run.lifecycle if row.get("event") != "pause"]
        _, findings, _ = self.analyze(run)
        self.assertFinding(findings, jc.INCOMPLETE, "scenario: zero pause faults were applied")

    def test_missing_final_observation_is_incomplete(self):
        run = healthy_run()
        run.lifecycle = [row for row in run.lifecycle if not (row.get("event") == "observation"
                                                              and row["reason"] == "crash")]
        _, findings, _ = self.analyze(run)
        self.assertEqual(jc.verdict_of(findings), jc.INCOMPLETE)
        self.assertFinding(findings, jc.INCOMPLETE, "missing final observation before crash of generation 1")
        run = healthy_run()
        next(row for row in run.lifecycle if row.get("event") == "observation").update(
            ok=False, failed_attempts=["TimeoutError: timed out"])
        _, findings, _ = self.analyze(run)
        self.assertFinding(findings, jc.RECOVERY, "final observation before checkpoint of generation 0")

    def test_scenario_must_reach_its_final_checkpoint_from_the_expected_pair(self):
        run = healthy_run()
        run.lifecycle = run.lifecycle[:run.lifecycle.index(next(r for r in run.lifecycle
                                                                if r.get("event") == "resume"
                                                                and r["generation"] == 2))]
        _, findings, _ = self.analyze(run)
        self.assertFinding(findings, jc.INCOMPLETE, "scenario: ended after crash of generation 1")
        run = healthy_run()
        next(r for r in run.lifecycle if r.get("event") == "admit" and r["generation"] == 2)["baseline_generation"] = 1
        _, findings, _ = self.analyze(run)
        self.assertFinding(findings, jc.INCOMPLETE,
                           "crash of generation 1: generation 2 was not admitted from checkpointed generation 0")

    def test_runtime_log_records_are_classified_with_their_raw_record(self):
        cases = [({"level": "ERROR", "msg": "panic in task", "go_stack": "x"}, jc.CORRECTNESS, "panic"),
                 ({"level": "ERROR", "msg": "PANIC: out of memory"}, jc.CORRECTNESS, "panic"),
                 ({"level": "ERROR", "msg": "checkpoint failed", "err": "disk"}, jc.RECOVERY, "checkpoint failure"),
                 ({"level": "WARN", "msg": "failed to restore queued task", "task_id": 3}, jc.RECOVERY,
                  "task restore failure"),
                 ({"level": "ERROR", "msg": "uncaught exception", "handled": False}, jc.INCOMPLETE,
                  "unexamined ERROR record")]
        for record, kind, label in cases:
            with self.subTest(record=record):
                run = healthy_run()
                run.logs[1].append(record)
                _, findings, _ = self.analyze(run)
                self.assertEqual([(f["class"], f["message"]) for f in findings],
                                 [(kind, f"generation 1 logs/latest.jsonl:3: {label}: {json.dumps(record)}")])
        run = healthy_run()
        run.server_logs[2] += "panic: runtime error: index out of range\n\ngoroutine 1 [running]:\n"
        run.logs[3] = None
        _, findings, _ = self.analyze(run)
        self.assertEqual(self.classes(findings), {
            (jc.CORRECTNESS, "generation 2 server.log:2: Go runtime crash: panic: runtime error: index out of range"),
            (jc.INCOMPLETE, "generation 3: runtime log logs/latest.jsonl is missing")})

    def test_elle_rejection_is_a_violation_and_undecided_is_incomplete(self):
        _, findings, _ = self.analyze(healthy_run(), verdict="false", code=1)
        self.assertEqual(jc.verdict_of(findings), jc.CORRECTNESS)
        self.assertFinding(findings, jc.CORRECTNESS, "generation 0: Elle rejected the history (exit 1)")
        _, findings, _ = self.analyze(healthy_run(), verdict="unknown", code=0)
        self.assertFinding(findings, jc.INCOMPLETE, "generation 0: Elle could not decide")

    def test_durability_violation_native_errors_and_lifecycle_failures_are_classified(self):
        run = healthy_run()
        run.lifecycle[-2]["recovered"] = {"1": [1, 2]}  # acknowledged token 4 lost at the checkpoint
        run.client(4, 2, [["append", 2, 9]], kind="info", error="native-error")
        run.event(event="failure", operation="crash", error="crash: RuntimeError: boom")
        run.event(event="failure", operation="jepsen", error="jepsen: RuntimeError: exited 1")
        _, findings, _ = self.analyze(run)
        self.assertTrue({(jc.CORRECTNESS, "generation 3 (checkpoint): key 1: checkpointed prefix [1, 2, 4] not "
                                          "preserved; recovered [1, 2]"),
                         (jc.INCOMPLETE, "1 client completions were info:native-error"),
                         (jc.RECOVERY, "crash: RuntimeError: boom"),
                         (jc.INVALID, "jepsen: RuntimeError: exited 1")} <= self.classes(findings))
        self.assertEqual(jc.verdict_of(findings), jc.CORRECTNESS)

    def test_missing_history_or_provenance(self):
        run = healthy_run()
        run.history, run.provenance = None, None
        _, findings, _ = self.analyze(run)
        self.assertEqual(self.classes(findings), {
            (jc.INVALID, "no provenance.json: the campaign's inputs are not recorded"),
            (jc.INCOMPLETE, "Jepsen wrote no history.jsonl")})


class ProvenanceTests(unittest.TestCase):
    def test_records_every_input_hash_and_all_parameters(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ("barn", "elle.jar", "jepsen.jar", "Test.db"):
                (root / name).write_bytes(name.encode())
            args = SimpleNamespace(db=root / "Test.db", barn=root / "barn", elle_jar=root / "elle.jar",
                                   jepsen_jar=root / "jepsen.jar", java=str(root / "no-java"), phase1=1, phase3=2,
                                   rate=3, concurrency=4, seed=5, jepsen_timeout=6, workloads=["smoke", "keyed"])
            row = jc.provenance(args, root)
        sha = jc.barn_process.bench.sha256_file
        self.assertEqual(row["parameters"], {"phase1": 1, "phase3": 2, "rate": 3, "concurrency": 4, "seed": 5,
                                             "jepsen_timeout": 6, "workloads": ["smoke", "keyed"]})
        self.assertEqual(row["families"], {"smoke": jc.FAMILIES["smoke"], "keyed": jc.FAMILIES["keyed"]})
        self.assertEqual((row["barn"]["size_bytes"], row["checker_jar"]["size_bytes"],
                          row["jepsen"]["jar"]["size_bytes"], row["fixture"]["database"]["size_bytes"]),
                         (4, 8, 10, 7))
        self.assertEqual(row["fixture"]["waif_identities"]["sha256"], None)
        self.assertEqual(row["config"]["sha256"], sha(jc.CONFIG))
        self.assertTrue(row["java"].startswith("unavailable:"))
        self.assertTrue(row["fixture"]["conformance_revision"].startswith("unavailable:"))
        self.assertEqual(len(row["source_tree"]["tree"]), 40)
        for key in ("uname", "filesystem", "cpu_count", "cpu_affinity", "environment", "os_release"):
            self.assertIn(key, row["runner"])


class MainTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.jar = self.root / "elle.jar"
        self.jar.write_bytes(b"jar")

    def main(self, archive, name):
        def run(command, log):
            Path(command[-1]).exists() or self.fail("Elle input was not written")
            return 0, f"{command[-1]} true\n"
        with patch.object(jc.check_elle, "run", side_effect=run), \
             contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            code = jc.main(["--analyze", str(archive), "--output", str(self.root / name),
                            "--elle-jar", str(self.jar)])
        return code, self.root / name

    def test_workloads_name_distinct_known_families(self):
        common = ["--output", str(self.root / "out"), "--elle-jar", str(self.jar), "--barn", "b", "--db", "d",
                  "--jepsen-jar", "j"]
        for workloads in ("smoke,maps", "props,props", ""):
            with self.subTest(workloads=workloads), contextlib.redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit):
                    jc.main(common + ["--workloads", workloads])
        seen = []
        with patch.object(jc, "run_campaign", side_effect=lambda args, output: seen.append(args.workloads)), \
             patch.object(jc, "analyze", return_value=({}, [])), contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(jc.main(common + ["--workloads", "smoke,props,keyed"]), 0)
        self.assertEqual(seen, [["smoke", "props", "keyed"]])

    def test_analyze_replay_of_an_archived_result_is_exact(self):
        archive = healthy_run().write(self.root / "archive")
        replays = [self.main(archive, name) for name in ("first", "second")]
        self.assertEqual([code for code, _ in replays], [0, 0])
        texts = []
        for _, directory in replays:
            results = json.loads((directory / "results.json").read_text())
            self.assertEqual((results["verdict"], results["classes"], results["findings"]), ("healthy", [], []))
            for key in ("argv", "analyze_command"):
                results.pop(key)
            texts.append(json.dumps(results, sort_keys=True).replace(json.dumps(str(directory))[1:-1], "<OUT>"))
        self.assertEqual(texts[0], texts[1])
        for name in ("gen-000.edn", "gen-001.edn", "gen-002.edn"):
            self.assertEqual((replays[0][1] / name).read_bytes(), (replays[1][1] / name).read_bytes())
        self.assertEqual(json.loads((replays[0][1] / "results.json").read_text())["provenance"],
                         {"fixture": "synthetic"})

    def test_exit_status_names_the_most_severe_class(self):
        cases = {"healthy": (healthy_run(), 0)}
        run = healthy_run()
        next(row for row in run.lifecycle if row.get("reason") == "resume")["ok"] = False
        cases["recovery"] = (run, 3)
        run = healthy_run()
        run.lifecycle = [row for row in run.lifecycle if row.get("event") != "pause"]
        cases["incomplete"] = (run, 4)
        run = healthy_run()
        run.history[1]["generation"] = 1
        cases["invalid"] = (run, 5)
        run = copy.deepcopy(cases["recovery"][0])
        run.lifecycle[-2]["recovered"] = {"1": []}
        cases["correctness"] = (run, 1)
        for name, (run, expected) in cases.items():
            with self.subTest(name=name):
                code, directory = self.main(run.write(self.root / f"archive-{name}"), name)
                self.assertEqual(code, expected)
                results = json.loads((directory / "results.json").read_text())
                self.assertEqual(jc.EXIT_CODES[results["verdict"]], expected)

    def test_missing_lifecycle_is_invalid_harness_evidence(self):
        (self.root / "empty").mkdir()
        code, directory = self.main(self.root / "empty", "out")
        self.assertEqual(code, 5)
        self.assertIn("lifecycle.jsonl", json.loads((directory / "results.json").read_text())["findings"][0]["message"])


class FakeServer:
    def __init__(self, executable, db, run_dir, port, log):
        self.db, self.run_dir, self.port, self.log = db, run_dir, port, log
        self.paused, self.identity, self.proc = False, {"db": str(db)}, SimpleNamespace(poll=lambda: None)
        self.verified = True

    def start(self):
        self.log.append(("start", self.db.name, self.db.read_bytes(), self.port))

    def pause(self):
        self.paused = True
        return "T"

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
    def campaign(self, probe_fails=False):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "Test.db").write_bytes(b"fixture")
        log, servers, ports = [], [], iter(range(9000, 9100))
        ledger = {9000: {}}
        def evaluate(port, code):
            if code == jc.ADMIT_CODE:
                return ok(ledger.get(port, {1: [1]}))
            if code == jc.ROWS_CODE:
                return ok([port % 100 + n for n in range(7)])
            if code.startswith("#0.jepsen_probe = "):
                return ConnectionRefusedError("refused") if probe_fails else ok(int(code.split()[2][:-1]))
            if code == jc.PROBE_READ_CODE:
                return ok(1)
            if code.startswith("rows = #0.jepsen_rows; led = #0.jepsen_ledger; out = {}; out = "):
                return ok([[7]] * code.count("maphaskey"))
            return ok({1: [1]})
        script = {"execute": evaluate}
        def server_class(*args):
            servers.append(FakeServer(*args, log))
            return servers[-1]
        output = root / "run"
        output.mkdir()
        campaign = jc.Campaign(root / "barn", root / "Test.db", output,
                               lambda port: FakeTransport(port, script), server_class=server_class,
                               port_source=lambda: next(ports), probe_deadline=0)
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

    def test_every_launch_and_resume_is_probed_and_every_end_is_observed(self):
        campaign, log, servers, output = self.campaign()
        campaign.launch("initial")
        campaign.touch(0, [4, 2])
        campaign.fault("pause")
        campaign.fault("resume")
        campaign.fault("crash")
        rows = jc.load_jsonl(output / "lifecycle.jsonl")
        self.assertEqual([(row["event"], row.get("reason")) for row in rows],
                         [("admit", None), ("probe", "launch"), ("pause", None), ("resume", None),
                          ("probe", "resume"), ("observation", "crash"), ("crash", None), ("admit", None),
                          ("probe", "launch")])
        self.assertEqual(rows[2]["observed_state"], "T")
        self.assertTrue(all(row["ok"] for row in rows if row["event"] == "probe"))
        self.assertEqual((rows[1]["value"], rows[4]["value"]), ({"written": 1, "read": 1}, {"written": 2, "read": 1}))
        self.assertEqual((rows[5]["keys"], rows[5]["ok"], rows[5]["value"]), ([2, 4], True, [[7], [7]]))
        journal = jc.load_jsonl(output / "journal.jsonl")
        self.assertEqual([(row["event"], row["f"], row["fault_seq"]) for row in journal],
                         [("fault-begin", "pause", 1), ("fault-end", "pause", 1), ("fault-begin", "resume", 2),
                          ("fault-end", "resume", 2), ("fault-begin", "crash", 3), ("fault-end", "crash", 3)])

    def test_failed_probe_is_recorded_without_stopping_the_campaign(self):
        campaign, _, _, output = self.campaign(probe_fails=True)
        campaign.launch("initial")
        probe = jc.load_jsonl(output / "lifecycle.jsonl")[1]
        self.assertEqual((probe["event"], probe["ok"], probe["failed_attempts"]),
                         ("probe", False, ["ConnectionRefusedError: refused"]))
        self.assertEqual(campaign.live, (0, 9000))

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
