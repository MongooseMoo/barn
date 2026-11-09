"""Independent completion-oracle controls; no MOO or fixture world is launched."""
import copy
import json
from pathlib import Path
import unittest
from unittest.mock import Mock, patch
from types import SimpleNamespace

import bench_players as bench

import mongoose_magic_scenario as magic
from mongoose_scenario_support import ScenarioFailure


def states(spell="lumos", outcome="applied"):
    before = {"actor": "#102", "wizard": 0, "is_player": 1, "location": "#100",
              "wand": "#101", "wand_location": "#102", "lit": int(spell == "nox"),
              "wand_emission": 20.0 if spell == "nox" else 0.0,
              "room_light": 0.2 if spell == "nox" else 0.0,
              "receipt": {}, "pending": {}, "debit_count": 0, "ready_at": 0.0,
              "capacity_j": 100000.0, "refill_w": 250.0, "joules": 100000.0,
              "settled_at": 100.0, "now": 100.1}
    after = copy.deepcopy(before)
    applied = outcome == "applied"
    paid = 1.0 if applied else 0.5
    after.update(receipt={"id": "actual-cast-1", "phase": "terminal", "spell": magic.SPELLS[spell],
                          "outcome": outcome, "quoted_focus": 1.0, "paid_focus": paid,
                          "formed": int(applied), "mastered_utility": 0,
                          "roll": 10.0 if applied else 70.0, "quality": 60.0,
                          "result": {"effect_applied": int(applied),
                                     "targets": [{"wands": ["#101"]}] if applied else []}},
                 matching_debits=[{"id": "actual-cast-1", "focus": paid}], debit_count=1,
                 ready_at=101.5, settled_at=100.5, now=100.6, joules_per_focus=1000.0,
                 joules=99000.0 if applied else 99500.0)
    if applied:
        after.update(lit=int(spell == "lumos"), wand_emission=20.0 if spell == "lumos" else 0.0,
                     room_light=0.2 if spell == "lumos" else 0.0)
    return before, after


def wire_literal(value):
    """MOO literal for source-derived offline wire-budget/parse round trips."""
    if isinstance(value, bench.MooObj):
        return f"#{int(value)}"
    if isinstance(value, str):
        if value.startswith("#") and value[1:].isdigit():
            return value
        return json.dumps(value)
    if isinstance(value, dict):
        return "[" + ", ".join(wire_literal(k) + " -> " + wire_literal(v) for k, v in value.items()) + "]"
    if isinstance(value, list):
        return "{" + ", ".join(wire_literal(v) for v in value) + "}"
    return repr(value)


def encoded_snapshot(state):
    values = [state.get(key, 0) for key in magic.SNAPSHOT_FIELDS]
    values[magic.SNAPSHOT_FIELDS.index("receipt")] = [
        [index, state["receipt"][key]] for index, key in enumerate(magic.RECEIPT_FIELDS, 1)
        if key in state["receipt"]]
    literal = wire_literal(values)
    return [len(literal), values]


class MagicOracleTests(unittest.TestCase):
    def test_compact_single_query_replays_actual_idle_native_values(self):
        state = json.loads((Path(__file__).parent / "fixtures" / "magic03-refusals.json").read_text())["refusals"][0]["before"]
        wire = encoded_snapshot(state)
        control = Mock()
        control.eval.return_value = (bench.parse_moo(wire_literal(wire)), [])
        fixture = {"actor": state["actor"], "wand": state["wand"], "room": state["location"],
                   "actor_generation": "actor-lifetime", "wand_generation": "wand-lifetime",
                   "room_generation": "room-lifetime"}
        observed = magic.snapshot(control, fixture)
        self.assertEqual({key: observed[key] for key in magic.SNAPSHOT_FIELDS}, state)
        self.assertEqual(control.eval.call_count, 1)
        self.assertLess(len('=> {"bp99999999999999999999", ' + wire_literal(wire) + '}'), 1000)

    def test_compact_terminal_and_fizzle_preserve_complete_result_and_oracle(self):
        for spell, outcome in (("lumos", "applied"), ("nox", "applied"), ("lumos", "fizzle")):
            with self.subTest(spell=spell, outcome=outcome):
                before, after = states(spell, outcome)
                after["receipt"].update(id="#277801:103176:1000000000:1000000000", at=1791945111.56178, worker=2147483647)
                after["receipt"]["result"].update(outcome=outcome, paid_focus=after["receipt"]["paid_focus"], delivered=0.0)
                for target in after["receipt"]["result"]["targets"]:
                    target.update(outcome="applied", delivered=0.0, effect_applied=1, target="#277801")
                after["matching_debits"][0].update(id=after["receipt"]["id"], at=1791945111.56178)
                wire = encoded_snapshot(after)
                decoded = magic._decode_snapshot(magic._references(bench.parse_moo(wire_literal(wire))))
                self.assertEqual(decoded["receipt"], after["receipt"])
                magic.validate_terminal(spell, before, decoded, set())
                self.assertLess(wire[0], 900)
                self.assertLess(len('=> {"bp99999999999999999999", ' + wire_literal(wire) + '}'), 1000)

    def test_compact_receipt_preserves_missing_fields_and_rejects_corruption(self):
        state, _ = states()
        state["receipt"] = {"id": "preparing", "phase": "prepared", "paid_focus": 0.0}
        state["pending"] = {"id": "preparing", "phase": "prepared", "task": 2147483647, "deadline": 1791945112.9}
        wire = encoded_snapshot(state)
        decoded = magic._decode_snapshot(wire)
        self.assertNotIn("roll", decoded["receipt"])
        self.assertNotIn("outcome", decoded["receipt"])
        self.assertEqual(decoded["receipt"]["paid_focus"], 0.0)
        self.assertIs(type(decoded["receipt"]["paid_focus"]), float)
        self.assertEqual(decoded["pending"], state["pending"])
        for corrupt in ([901, wire[1]], [True, wire[1]], [wire[0], wire[1][:-1]]):
            with self.assertRaises(ScenarioFailure):
                magic._decode_snapshot(corrupt)
        for row in ([0, "bad"], [True, "bad"], [999, "bad"], ["1", "bad"]):
            corrupt = copy.deepcopy(wire)
            corrupt[1][magic.SNAPSHOT_FIELDS.index("receipt")].append(row)
            with self.assertRaises(ScenarioFailure):
                magic._decode_snapshot(corrupt)
        corrupt = copy.deepcopy(wire)
        corrupt[1][magic.SNAPSHOT_FIELDS.index("receipt")].append([1, "duplicate"])
        with self.assertRaisesRegex(ScenarioFailure, "duplicate"):
            magic._decode_snapshot(corrupt)

    def test_negative_controls_replay_actual_ordinary_magic03_response_and_state(self):
        rows = json.loads((Path(__file__).parent / "fixtures" / "magic03-refusals.json").read_text())["refusals"]
        options = SimpleNamespace(iterations=0, deadline_seconds=10, magic_connection=Mock())
        fixture = {"initial_state": rows[0]["before"]}
        observed = [state for row in rows for state in (row["before"], row["after"])] + [rows[-1]["after"]]
        with patch.object(magic, "snapshot", side_effect=observed), \
             patch.object(magic, "actor_command", side_effect=[row["response"] for row in rows]), \
             patch.object(magic, "_record"):
            evidence = magic.run(Mock(), fixture, options)
        self.assertEqual(len(evidence["negative_controls"]), 2)
        self.assertTrue(all(row["no_effect_verified"] for row in evidence["negative_controls"]))
        self.assertEqual(evidence["attempt_count"], 0)
        self.assertEqual(evidence["completed_iterations"], 0)

    def test_actual_native_parser_object_types_normalize_without_changing_integers(self):
        value = bench.parse_moo('["actor" -> #102, "count" -> 102, "receipt" -> ["spell" -> #20835, "targets" -> {{#101}}]]')
        control = Mock()
        control.eval.return_value = (value, [])
        observed = magic._read(control, "return 1;")
        self.assertEqual(observed["actor"], "#102")
        self.assertEqual(observed["count"], 102)
        self.assertEqual(observed["receipt"]["spell"], "#20835")
        self.assertEqual(observed["receipt"]["targets"], [["#101"]])

    def test_partial_setup_cleanup_preserves_primary_native_failure(self):
        options = SimpleNamespace(magic_fixture={"actor": "#102"})
        primary = bench.ControlEvalError("primary prerequisite error", ["native compile diagnostic"])
        with patch.object(magic, "_setup", create=True, side_effect=primary), \
             patch.object(magic, "cleanup", side_effect=RuntimeError("secondary cleanup error")) as clean:
            with self.assertRaises(ScenarioFailure) as captured:
                magic.setup(Mock(), options)
        clean.assert_called_once()
        evidence = captured.exception.evidence
        self.assertEqual(evidence["primary_failure"], "primary prerequisite error")
        self.assertEqual(evidence["native_output"], ["native compile diagnostic"])
        self.assertEqual(evidence["partial_fixture"], {"actor": "#102"})
        self.assertEqual(evidence["cleanup_failure"], "secondary cleanup error")

    def test_real_light_and_extinguish_require_independent_physical_state(self):
        for spell in ("lumos", "nox"):
            with self.subTest(spell=spell):
                before, after = states(spell)
                result = magic.validate_terminal(spell, before, after, set())
                self.assertEqual(result["outcome"], "applied")
                self.assertTrue(result["physical_effect_verified"])

    def test_applied_ack_with_no_light_delta_is_not_productive_work(self):
        before, after = states()
        after.update(lit=0, room_light=0.0, wand_emission=0.0)
        with self.assertRaisesRegex(ScenarioFailure, "did not light"):
            magic.validate_terminal("lumos", before, after, set())

    def test_pending_ack_is_not_terminal_completion(self):
        before, after = states()
        after["receipt"]["phase"] = "prepared"
        after["pending"] = {"id": "actual-cast-1"}
        with self.assertRaisesRegex(ScenarioFailure, "terminal receipt"):
            magic.validate_terminal("lumos", before, after, set())

    def test_fizzle_keeps_effect_and_pays_honest_half_quote(self):
        before, after = states(outcome="fizzle")
        self.assertEqual(magic.validate_terminal("lumos", before, after, set())["paid_focus"], 0.5)
        after["lit"] = 1
        with self.assertRaisesRegex(ScenarioFailure, "fizzle changed"):
            magic.validate_terminal("lumos", before, after, set())

    def test_reused_identity_cannot_count_again(self):
        before, after = states()
        with self.assertRaisesRegex(ScenarioFailure, "reused cast"):
            magic.validate_terminal("lumos", before, after, {"actual-cast-1"})

    def test_duplicate_debits_and_wrong_payment_are_visible_failures(self):
        before, after = states()
        after["matching_debits"].append(copy.deepcopy(after["matching_debits"][0]))
        with self.assertRaisesRegex(ScenarioFailure, "one matching"):
            magic.validate_terminal("lumos", before, after, set())
        before, after = states()
        after["receipt"]["paid_focus"] = 0.5
        with self.assertRaisesRegex(ScenarioFailure, "payment"):
            magic.validate_terminal("lumos", before, after, set())

    def test_resource_oracle_keeps_natural_refill_instead_of_exact_wall_delta(self):
        before, after = states()
        before["joules"] = 98000.0
        after["joules"] = 97120.0  # 1000 spent; native refill happened before settlement.
        magic.validate_terminal("lumos", before, after, set())
        after["joules"] = 98120.0  # No spend, despite a forged receipt/debit.
        with self.assertRaisesRegex(ScenarioFailure, "raw reservoir"):
            magic.validate_terminal("lumos", before, after, set())

    def test_recovery_and_reported_wand_must_be_real(self):
        before, after = states()
        after["ready_at"] = 0.0
        with self.assertRaisesRegex(ScenarioFailure, "recovery"):
            magic.validate_terminal("lumos", before, after, set())
        before, after = states()
        after["receipt"]["result"]["targets"] = [{"wands": ["#999"]}]
        with self.assertRaisesRegex(ScenarioFailure, "actual owned wand"):
            magic.validate_terminal("lumos", before, after, set())

    def test_native_roll_and_ordinary_actor_cannot_be_forged(self):
        before, after = states()
        after["receipt"]["roll"] = 99.0
        with self.assertRaisesRegex(ScenarioFailure, "roll mismatch"):
            magic.validate_terminal("lumos", before, after, set())
        before, after = states()
        after["wizard"] = 1
        with self.assertRaisesRegex(ScenarioFailure, "ordinary actor"):
            magic.validate_terminal("lumos", before, after, set())


if __name__ == "__main__":
    unittest.main()
