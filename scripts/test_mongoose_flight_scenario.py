"""Completion controls; these do not launch or substitute for a Mongoose run."""
import copy
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import mongoose_flight_scenario as flight
from mongoose_scenario_support import ScenarioFailure


def state(**changes):
    value = {"cycle": 10, "position": [0.0, 0.0, 0.0],
             "velocity": [0.0, 0.0, 0.0], "heading": [0.0, 1.0, 0.0],
             "desired_speed": 0.0, "engines_cut": False,
             "tanks": [[1, 4147, "", 100.0, 100.0]]}
    value.update(changes)
    return value


class FlightCompletionTests(unittest.TestCase):
    def test_compact_snapshot_retains_all_fields_in_one_native_read(self):
        wire = [1, 0, 1, 45648, 1, 1, 1, 45645, 59, 41115260,
                [1000000000000.5, 999999999999.134, 1e12],
                [0.499909530400231, -0.866077630131976, 0.0],
                [0.499909530400231, -0.866077630131976, 0.0],
                [-0.0, -1.0, 0.0], 1.0, False, True, 1, 6103.6,
                [[3, 4147, "", 999.99985067065, 1000.0]],
                [[1, 1066, "750 millicochrane impulse engine", 1e9, 1e9, 1.0]],
                [[6, 4153, "RCS Thruster quad", 46200.0, 46200.0, 1.0]],
                [[5, 4152, 17e9, 17e9, 1.0]]]

        class CompactControl:
            calls = 0

            def eval(self, code, **kwargs):
                self.calls += 1
                self.code = code
                return copy.deepcopy(wire), ["complete native tagged output"]

        control = CompactControl()
        observed = flight.snapshot(control, {"ship": 45645, "actor": 277801, "console": 45649})
        self.assertIsInstance(observed, dict)
        self.assertEqual(len(observed), 23)
        self.assertEqual(observed["universe_member"], 59)
        self.assertEqual(observed["velocity"], wire[11])
        self.assertEqual(observed["tanks"], wire[19])
        self.assertEqual(observed["reactors"], wire[22])
        self.assertEqual(control.calls, 1)
        self.assertIn("return {is_player(a)", control.code)
        # Complete-length corruption must fail before any physical predicate.
        for index, bad in ((2, "1"), (9, True), (11, [1.0, 0.0]),
                           (11, [float("nan"), 0.0, 0.0]), (11, [True, 0.0, 0.0]),
                           (15, "false"), (18, float("inf")),
                           (19, [[3, 4147, "", "999", 1000.0]]),
                           (20, [[1, 1066, "engine", 1e9]]), (22, "not rows")):
            with self.subTest(index=index, bad=bad):
                damaged = copy.deepcopy(wire)
                damaged[index] = bad
                with self.assertRaises(ScenarioFailure) as raised:
                    flight._decode_snapshot(damaged, ["native corrupt wire"])
                self.assertEqual(raised.exception.evidence["native_output"], ["native corrupt wire"])

    def test_invalid_snapshot_wire_keeps_native_failure_evidence(self):
        truncated = '{"bp136", ["velocity" ->...'
        for value in (truncated, [], [0] * 22, [0] * 24, {"cycle": 1}):
            with self.subTest(value=value):
                class InvalidControl:
                    def eval(self, *args, **kwargs):
                        return value, [truncated]

                with self.assertRaises(ScenarioFailure) as raised:
                    flight.snapshot(InvalidControl(), {"ship": 1, "actor": 2, "console": 3})
                self.assertEqual(raised.exception.evidence["snapshot_wire"], value)
                self.assertEqual(raised.exception.evidence["native_output"], [truncated])

    def test_real_turn_motion_and_fuel_are_all_required(self):
        before = state()
        moved = state(cycle=11, position=[1.0, 0.0, 0.0],
                      velocity=[1.0, 0.0, 0.0], heading=[1.0, 0.0, 0.0],
                      tanks=[[1, 4147, "", 99.0, 100.0]])
        self.assertTrue(flight._motion_completed(before, moved, 1.0, [1.0, 0.0, 0.0]))
        for key in ("cycle", "position", "velocity", "heading", "tanks"):
            with self.subTest(missing=key):
                noop = copy.deepcopy(moved)
                noop[key] = before[key]
                self.assertFalse(flight._motion_completed(before, noop, 1.0, [1.0, 0.0, 0.0]))

    def test_desired_request_without_physical_progress_is_not_work(self):
        before = state()
        requested = state(desired_speed=1.0, desired_heading=[1.0, 0.0, 0.0])
        self.assertFalse(flight._motion_completed(before, requested, 1.0, [1.0, 0.0, 0.0]))

    def test_wrong_speed_or_heading_is_not_completed_flight(self):
        before = state()
        moved = state(cycle=11, position=[1.0, 0.0, 0.0],
                      velocity=[0.5, 0.0, 0.0], heading=[1.0, 0.0, 0.0],
                      tanks=[[1, 4147, "", 99.0, 100.0]])
        self.assertFalse(flight._motion_completed(before, moved, 1.0, [1.0, 0.0, 0.0]))
        moved["velocity"] = [1.0, 0.0, 0.0]
        self.assertFalse(flight._motion_completed(before, moved, 1.0, [0.0, -1.0, 0.0]))

    def test_braking_requires_actual_zero_velocity_and_fuel(self):
        moving = state(cycle=11, velocity=[1.0, 0.0, 0.0])
        stopped = state(cycle=12, tanks=[[1, 4147, "", 99.0, 100.0]])
        self.assertTrue(flight._brake_completed(moving, stopped))
        for key, value in (("cycle", 11), ("velocity", [0.1, 0.0, 0.0]),
                           ("desired_speed", 1.0), ("engines_cut", True),
                           ("tanks", moving["tanks"])):
            with self.subTest(missing=key):
                refused = copy.deepcopy(stopped)
                refused[key] = value
                self.assertFalse(flight._brake_completed(moving, refused))

    def test_already_stationary_is_not_braking_work(self):
        self.assertFalse(flight._brake_completed(state(), state(cycle=11,
                         tanks=[[1, 4147, "", 99.0, 100.0]])))

    def test_wizard_disconnected_or_unmanned_actor_fails(self):
        fixture = {"room": 20, "ship": 10}
        admitted = {"actor_is_player": 1, "actor_wizard": 0,
                    "actor_connected": 1, "actor_location": 20,
                    "actor_manning": 1, "console_feature": 1,
                    "operate_allowed": 1, "console_ship": 10,
                    "universe_member": 1}
        flight._require_station(admitted, fixture)
        for key, value in (("actor_wizard", 1), ("actor_connected", 0),
                           ("actor_manning", 0), ("console_feature", 0),
                           ("operate_allowed", 0), ("actor_location", 21),
                           ("console_ship", 11), ("universe_member", 0)):
            with self.subTest(refusal=key), self.assertRaises(ScenarioFailure):
                flight._require_station(dict(admitted, **{key: value}), fixture)

    def test_invalid_physical_vectors_fail_explicitly(self):
        for vector in ([], [1.0], [float("nan"), 0.0, 0.0], [float("inf"), 0.0, 0.0]):
            with self.subTest(vector=vector), self.assertRaises(ScenarioFailure):
                flight._length(vector)

    def test_actual_membership_59_and_later_list_positions_are_admitted(self):
        # Saved flight02 proved that the new ship is universe member 59.
        # MOO `in` returns an index, not a normalized boolean.
        admitted = {"actor_is_player": 1, "actor_wizard": 0,
                    "actor_connected": 1, "actor_location": 20,
                    "actor_manning": 1, "console_feature": 1,
                    "operate_allowed": 1, "console_ship": 10,
                    "universe_member": 59}
        flight._require_station(admitted, {"room": 20, "ship": 10})
        for key in ("actor_connected", "actor_manning", "console_feature", "universe_member"):
            with self.subTest(later_position=key):
                flight._require_station(dict(admitted, **{key: 2}), {"room": 20, "ship": 10})
            with self.subTest(absent=key), self.assertRaises(ScenarioFailure):
                flight._require_station(dict(admitted, **{key: 0}), {"room": 20, "ship": 10})

    def test_setup_preserves_original_failure_evidence(self):
        original = {"native_state": {"universe_member": 59}, "observations": 3}

        class FailedControl:
            def eval(self, *args, **kwargs):
                raise ScenarioFailure("native control failed", original)

        with self.assertRaises(ScenarioFailure) as raised:
            flight.setup(FailedControl(), SimpleNamespace(deadline_seconds=2))
        self.assertEqual(raised.exception.evidence["primary_failure_evidence"], original)

    def test_suspended_heading_finishes_before_speed_is_sent(self):
        # A returned input bracket can precede the SQL-backed sound and
        # heading assignment. Speed must not be sent into that pending phase.
        fixture = {"actor": 42, "room": 20, "ship": 10}
        world = state(heading=[1.0, 0.0, 0.0], desired_heading=[0.0, 0.0, 0.0],
                      actor_is_player=1, actor_wizard=0, actor_connected=1,
                      actor_location=20, actor_manning=1, console_feature=1,
                      operate_allowed=1, console_ship=10, universe_member=59)
        pending = 0
        commands = []

        def command(conn, text, **kwargs):
            nonlocal pending
            commands.append(text)
            if text == "heading 180 0":
                pending = 3
            elif text == "setspeed 1.0":
                self.assertEqual(pending, 0, "speed sent before heading assignment")
                world.update(cycle=11, heading=[0.0, -1.0, 0.0],
                             desired_speed=1.0, position=[0.0, -1.0, 0.0],
                             velocity=[0.0, -1.0, 0.0],
                             tanks=[[1, 4147, "", 99.0, 100.0]])
            elif text == "setspeed 0":
                world.update(cycle=12, desired_speed=0.0, velocity=[0.0, 0.0, 0.0],
                             tanks=[[1, 4147, "", 98.0, 100.0]])
            elif text == "cutengines":
                world["engines_cut"] = True
            else:
                self.fail("unexpected command " + text)
            return {"command": text, "output": []}

        def observe(*args):
            nonlocal pending
            if pending:
                pending -= 1
                if not pending:
                    world["desired_heading"] = [0.0, -1.0, 0.0]
            return copy.deepcopy(world)

        def bounded_wait(observe, predicate, timeout, **kwargs):
            for n in range(8):
                last = observe()
                if predicate(last):
                    return {"state": last, "observations": n + 1}
            raise ScenarioFailure("bounded observer did not finish", last)

        with patch.dict(flight._connections, {42: object()}, clear=True), \
                patch.object(flight, "snapshot", side_effect=observe), \
                patch.object(flight, "actor_command", side_effect=command), \
                patch.object(flight, "wait_until", side_effect=bounded_wait):
            result = flight.run(object(), fixture, SimpleNamespace(iterations=1, deadline_seconds=1))
        self.assertEqual(result["completed_iterations"], 1)
        self.assertEqual(commands, ["heading 180 0", "setspeed 1.0", "setspeed 0", "cutengines"])


if __name__ == "__main__":
    unittest.main()
