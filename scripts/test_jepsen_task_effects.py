"""Fixtures for the Jepsen task lane and effects lane (plan section 4): never
launch Barn, Jepsen or Elle. Each defect fixture must give its own witness."""
import copy
import unittest

import jepsen_checkpoint as jc
from test_jepsen_checkpoint import LAYOUT, CampaignTests, ok

EFFECTS = jc.FAMILIES["effects"]["base"]
PROGRAMMER, OBJECT = 40, 41
SETUP = {"programmer": PROGRAMMER, "object": OBJECT, "anon_parent": 42, "waif_class": 43}
TASKS = [{"kind": kind, "token": 1000100 + index, "tid": 500 + index} for index, kind in enumerate(jc.TASK_KINDS)]


def layout_with_effects():
    layout = jc.parse_layout(LAYOUT)
    layout["effects"] = {"object": 30, "receiver": 31, "option": 32}
    return layout


class EffectsCodeTests(unittest.TestCase):
    def test_boundary_transactions_cross_set_connection_option_before_the_rows(self):
        code = jc.effects_code([["append", EFFECTS + 1, 5], ["r", EFFECTS + 2, None]], layout_with_effects(), 7)
        order = [code.index(part) for part in ("h = #30.hot;", 'notify(#31, "jepsen-effect 7 pre");', "#30.pre = 7;",
                                               'set_connection_option(#32, "flush-command", "jepsen-effect-7");',
                                               f"#30.je{EFFECTS + 1}", f"#30.je{EFFECTS + 2}", "#30.hot = h + 1;",
                                               'notify(#31, "jepsen-effect 7 post");', "return out;")]
        self.assertEqual(order, sorted(order))

    def test_even_operations_only_notify_around_the_rows(self):
        code = jc.effects_code([["append", EFFECTS, 1]], layout_with_effects(), 8)
        self.assertNotIn("set_connection_option", code)
        self.assertNotIn(".pre =", code)
        self.assertIn('"jepsen-effect 8 pre"', code)
        self.assertIn('"jepsen-effect 8 post"', code)

    def test_txn_code_routes_effects_and_refuses_mixed_or_missing_objects(self):
        self.assertEqual(jc.txn_code([["append", EFFECTS, 1]], layout_with_effects(), 3),
                         jc.effects_code([["append", EFFECTS, 1]], layout_with_effects(), 3))
        with self.assertRaises(ValueError):
            jc.effects_code([["append", EFFECTS, 1], ["r", 0, None]], layout_with_effects(), 3)
        with self.assertRaises(ValueError):
            jc.txn_code([["append", EFFECTS, 1]], jc.parse_layout(LAYOUT), 3)

    def test_drain_code(self):
        self.assertEqual(jc.drain_code({"receiver": 31, "option": 32}, "2 5"),
                         'notify(#31, "jepsen-drain 2 5"); return connection_option(#32, "flush-command");')


class TaskCodeTests(unittest.TestCase):
    def test_setup_creates_every_verb_with_its_owner(self):
        code = jc.task_setup_code()
        for name, (owner, _) in jc.TASK_VERBS.items():
            who = "p" if owner == "p" else "player"
            self.assertIn(f'add_verb(o, {{{who}, "xd", "{name}"}}', code)
        # task_local and notify to another player are wizard-only builtins.
        self.assertEqual({name for name, (owner, _) in jc.TASK_VERBS.items() if owner == "wizard"},
                         {"set_local", "get_local", "effect_body"})
        self.assertIn("set_player_flag(p, 1);", code)
        self.assertIn("wc = create($waif, p);", code)

    def test_holds_are_timed_never_indefinite(self):
        for name, (_, lines) in jc.TASK_VERBS.items():
            for line in lines:
                self.assertNotIn("suspend()", line, name)

    def test_spawn_and_release_code(self):
        self.assertEqual(jc.task_spawn_code(SETUP, "fork", 9), "return #41:fork_hold(9);")
        self.assertEqual(jc.task_spawn_code(SETUP, "values", 9), 'return #41:spawn("values_body", 9, #42, #43);')
        self.assertEqual(jc.task_spawn_code(SETUP, "recycle", 9, 77), 'return #41:spawn("recycle_body", 9, #77);')
        release = jc.task_release_code({"setup": SETUP, "tasks": TASKS}, 31)
        self.assertTrue(release.startswith("#41.receiver = #31; #41.gate = 1;"))
        cancel = next(task for task in TASKS if task["kind"] == "cancel")
        self.assertIn(f"kill_task({cancel['tid']});", release)
        for task in TASKS:
            if task["kind"] in jc.RESUMED_KINDS:
                self.assertIn(f'resume({task["tid"]}, {{{task["token"]}, "wake"}});', release)
        self.assertNotIn("resume(500,", release)  # the fork passes its gate

    def test_task_state_parsing(self):
        self.assertEqual(jc.parse_task_state([[1, 2], {"7": [["x"]]}]), {"ids": [1, 2], "results": {7: [["x"]]}})
        self.assertEqual(jc.parse_task_state([[1], 0]), {"ids": [1], "results": None})
        for value in ([1, 2], [[1], "x"], [["a"], {}]):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    jc.parse_task_state(value)


def healthy_records(tasks=TASKS, programmer=PROGRAMMER):
    return {str(task["token"]): [jc.expected_task_record(task, programmer)] for task in tasks
            if task["kind"] != "cancel"}


class TaskLane:
    """A lifecycle fixture: plant in generation 0, graceful checkpoint, generation 1
    finds it held and releases it."""

    def __init__(self):
        self.plant = {"event": "task-plant", "seq": 1, "point": "graceful-checkpoint", "generation": 0,
                      "setup": dict(SETUP), "tasks": copy.deepcopy(TASKS), "recycled": 77}
        self.held_ids = [task["tid"] for task in TASKS]
        self.records = healthy_records()
        self.after_ids = []
        self.receipts = [{"generation": 1, "line": f"jepsen-task {TASKS[-1]['token']}"}]
        self.complete = True

    def lifecycle(self):
        return [self.plant,
                {"event": "admit", "generation": 1, "kind": "checkpoint", "baseline_generation": 0},
                {"event": "task-recovery", "generation": 1, "plant": 1, "ok": True,
                 "state": {"ids": self.held_ids, "results": {}},
                 "present": [tid for tid in self.held_ids if tid in {t["tid"] for t in TASKS}]},
                {"event": "task-release", "generation": 1, "plant": 1, "complete": self.complete,
                 "state": {"ids": self.after_ids, "results": self.records}, "deadline_seconds": 45}]

    def check(self, recoveries=None, live=False, lifecycle=None):
        return jc.task_checks(lifecycle or self.lifecycle(), self.receipts, recoveries or [], live)


def witnesses(findings, kind=jc.CORRECTNESS):
    return sorted({witness for found, witness, _ in findings if found == kind})


def token_of(kind):
    return str(next(task["token"] for task in TASKS if task["kind"] == kind))


class TaskCheckTests(unittest.TestCase):
    def test_held_then_released_once_with_expected_records_is_healthy(self):
        rows, findings = TaskLane().check()
        self.assertEqual(findings, [])
        self.assertEqual(rows[0]["launches"], [{"generation": 1, "status": "held"}])

    def test_wrong_wake_value(self):
        lane = TaskLane()
        lane.records[token_of("suspend")][0][3][0] = 0  # woke with 0, as an indefinite suspend would at load
        self.assertEqual(witnesses(lane.check()[1]), ["wrong-wake-value"])

    def test_lost_task_missing_at_recovery_or_never_completing(self):
        lane = TaskLane()
        lane.held_ids = lane.held_ids[1:]
        self.assertEqual(witnesses(lane.check()[1]), ["lost-task"])
        lane = TaskLane()
        del lane.records[token_of("values")]
        lane.after_ids, lane.complete = [TASKS[2]["tid"]], False
        _, findings = lane.check()
        self.assertEqual(witnesses(findings), ["lost-task"])
        self.assertEqual(len([1 for _, witness, _ in findings if witness == "lost-task"]), 2)

    def test_duplicated_continuation(self):
        lane = TaskLane()
        lane.records[token_of("edit")].append(lane.records[token_of("edit")][0])
        self.assertEqual(witnesses(lane.check()[1]), ["duplicated-continuation"])
        lane = TaskLane()
        lane.held_ids = lane.held_ids + [lane.held_ids[0]]
        self.assertEqual(witnesses(lane.check()[1]), ["duplicated-continuation"])

    def test_wrong_alias(self):
        for index in (2, 4, 5, 6, 7, 9, 10, 11):
            with self.subTest(field=index):
                lane = TaskLane()
                record = lane.records[token_of("values")][0]
                record[index] = 0 if record[index] != 0 else 1
                self.assertEqual(witnesses(lane.check()[1]), ["wrong-alias"])

    def test_wrong_task_perms(self):
        lane = TaskLane()
        lane.records[token_of("fork")][0][2] = 3
        self.assertEqual(witnesses(lane.check()[1]), ["wrong-task-perms"])

    def test_killed_task_ran(self):
        lane = TaskLane()
        lane.records[token_of("cancel")] = [["cancel", "ran", 0]]
        self.assertEqual(witnesses(lane.check()[1]), ["killed-task-ran"])

    def test_edited_verb_and_recycled_reference(self):
        lane = TaskLane()
        lane.records[token_of("edit")] = [["edit", "new", [int(token_of("edit"))]]]
        self.assertEqual(witnesses(lane.check()[1]), ["wrong-program-after-edit"])
        lane = TaskLane()
        lane.records[token_of("recycle")][0][1] = 1
        self.assertEqual(witnesses(lane.check()[1]), ["recycled-reference-valid"])

    def test_a_result_before_release(self):
        lane = TaskLane()
        lifecycle = lane.lifecycle()
        lifecycle[2]["state"]["results"] = {token_of("suspend"): [["suspend"]]}
        self.assertEqual(witnesses(lane.check(lifecycle=lifecycle)[1]), ["ran-before-release"])

    def test_effect_task_delivers_once_after_release_and_never_before(self):
        lane = TaskLane()
        lane.receipts = lane.receipts * 2
        self.assertEqual(witnesses(lane.check()[1]), ["duplicated-effect"])
        lane = TaskLane()
        lane.receipts = []
        self.assertEqual(witnesses(lane.check()[1]), ["lost-effect"])
        lane = TaskLane()
        lane.receipts.append({"generation": 0, "line": f"jepsen-task {token_of('effect')}"})
        self.assertEqual(witnesses(lane.check()[1]), ["effect-before-release"])

    def test_graceful_plant_must_be_held_after_its_checkpoint(self):
        lane = TaskLane()
        lifecycle = lane.lifecycle()
        lifecycle[2]["state"] = {"ids": [], "results": None}
        lifecycle[2]["present"] = []
        del lifecycle[3]
        lane.receipts = []
        self.assertEqual(witnesses(lane.check(lifecycle=lifecycle)[1]), ["lost-task"])

    def live_lane(self, cuts, status_held=True):
        lane = TaskLane()
        lifecycle = lane.lifecycle()
        lifecycle[0]["point"] = "live-checkpoint"
        lifecycle[1] = {"event": "admit", "generation": 1, "kind": "checkpoint-crash", "restart": {"generation": 0}}
        if not status_held:
            lifecycle[2]["state"] = {"ids": [], "results": None}
            lifecycle[2]["present"] = []
            del lifecycle[3]
            lane.receipts = []
        return lane, lifecycle, [{"generation": 0, "candidate_cuts": cuts}]

    def test_live_plant_presence_follows_the_selected_cut(self):
        lane, lifecycle, recoveries = self.live_lane(["completed checkpoint 2"])
        _, findings = lane.check(recoveries, live=True, lifecycle=lifecycle)
        self.assertEqual(witnesses(findings), [])
        self.assertEqual([message for _, _, message in findings],  # this fixture has no graceful plant
                         ["task lane: no graceful-checkpoint plant was released after its restart"])
        lane, lifecycle, recoveries = self.live_lane(["completed checkpoint 2"], status_held=False)
        self.assertEqual(witnesses(lane.check(recoveries, lifecycle=lifecycle)[1]), ["lost-task"])
        lane, lifecycle, recoveries = self.live_lane(["unchanged input"], status_held=False)
        self.assertEqual(witnesses(lane.check(recoveries, lifecycle=lifecycle)[1]), [])
        lane, lifecycle, recoveries = self.live_lane(["unchanged input"])
        self.assertEqual(witnesses(lane.check(recoveries, lifecycle=lifecycle)[1]), ["unexpected-task"])

    def test_coverage(self):
        lane = TaskLane()
        lane.complete = False
        self.assertIn("task-coverage", witnesses(lane.check()[1], jc.INCOMPLETE))
        _, findings = TaskLane().check(live=True)
        self.assertEqual(witnesses(findings, jc.INCOMPLETE), ["task-coverage"])
        self.assertIn("no live plant", findings[0][2])


def effect_send(op_id, generation=1, kind="ok", complete_ns=100):
    value = [["append", EFFECTS, op_id]]
    return [{"event": "request", "id": op_id, "value": value},
            {"event": "send", "id": op_id, "generation": generation, "monotonic_ns": 10},
            {"event": "complete", "id": op_id, "type": kind, "monotonic_ns": complete_ns}]


class EffectLane:
    def __init__(self):
        self.journal = effect_send(7) + effect_send(8)
        self.receipts = [{"generation": 1, "line": f"jepsen-effect {op} {part}"} for op in (7, 8)
                         for part in ("pre", "post")]
        self.drain = {"event": "effect-drain", "generation": 1, "ok": True, "sent_ns": 200, "received_ns": 210,
                      "option": "jepsen-effect-7", "commit_retries": 3}

    def check(self):
        return jc.effect_checks([self.drain], self.journal, self.receipts)


class EffectCheckTests(unittest.TestCase):
    def test_each_completed_operation_delivers_pre_and_post_once(self):
        results, findings = EffectLane().check()
        self.assertEqual(findings, [])
        self.assertEqual((results["ok"], results["ok_boundary"], results["max_commit_retries"]), (2, 1, 3))

    def test_duplicated_live_retry_effect(self):
        lane = EffectLane()
        lane.receipts.append({"generation": 1, "line": "jepsen-effect 8 pre"})
        self.assertEqual(witnesses(lane.check()[1]), ["duplicated-live-retry-effect"])

    def test_invented_effect_and_option(self):
        lane = EffectLane()
        lane.receipts.append({"generation": 1, "line": "jepsen-effect 9 pre"})
        self.assertEqual(witnesses(lane.check()[1]), ["invented-effect"])
        lane = EffectLane()
        lane.drain["option"] = "jepsen-effect-8"  # 8 never crossed the boundary
        self.assertEqual(witnesses(lane.check()[1]), ["invented-effect"])

    def test_lost_effect_before_the_drain_but_not_after_it(self):
        lane = EffectLane()
        lane.receipts = [receipt for receipt in lane.receipts if receipt["line"] != "jepsen-effect 8 pre"]
        self.assertEqual(witnesses(lane.check()[1]), ["lost-effect"])
        lane = EffectLane()
        lane.journal = effect_send(7) + effect_send(8, complete_ns=300)
        lane.receipts = [receipt for receipt in lane.receipts if not receipt["line"].startswith("jepsen-effect 8")]
        self.assertEqual(lane.check()[1], [])
        lane.journal[-1]["type"] = "info"
        self.assertEqual(lane.check()[1], [])

    def test_coverage(self):
        lane = EffectLane()
        lane.drain["commit_retries"] = 0
        self.assertEqual(witnesses(lane.check()[1], jc.INCOMPLETE), ["effect-coverage"])
        lane = EffectLane()
        lane.journal = effect_send(8)
        lane.receipts = lane.receipts[2:]
        lane.drain["option"] = ""
        _, findings = lane.check()
        self.assertEqual([message for _, _, message in findings],
                         ["effects: no successful transaction crossed the set_connection_option boundary"])


class StagelessControlTests(unittest.TestCase):
    def test_effect_control_passes_only_with_its_witness_in_the_findings(self):
        results = {"recoveries": [], "findings": [{"class": jc.CORRECTNESS, "message": "m",
                                                   "witness": "duplicated-live-retry-effect"}]}
        passed = jc.control_result("pending-effect-retry", results)
        self.assertEqual((passed["passed"], passed["stage"]), (True, None))
        results["findings"][0]["witness"] = "lost-effect"
        self.assertFalse(jc.control_result("pending-effect-retry", results)["passed"])
        self.assertFalse(jc.control_result("pending-effect-retry", {"recoveries": [], "findings": []})["passed"])


class FakeReceiver:
    players = iter(range(300, 400))

    def __init__(self, factory, port, on_line):
        self.player, self.on_line, self.closed = next(self.players), on_line, False

    def start(self):
        return self

    def close(self):
        self.closed = True


class TaskCampaignTests(CampaignTests):
    """The task lane's order: plant before the graceful checkpoint's stop; at the
    next launch read every plant's held state after the recovery observation
    and before any release; release, wait, then admit."""

    def lane_campaign(self):
        campaign, log, servers, output = self.campaign(tasks=True, receiver_class=FakeReceiver, release_deadline=0)
        state = {"released": False, "spawned": []}
        evaluate = self.script_evaluate

        def execute(port, code):
            if code == jc.task_setup_code():
                return ok([PROGRAMMER, OBJECT, 42, 43])
            if code == "return toint(create($nothing));":
                return ok(77)
            if ":fork_hold(" in code or ':spawn("' in code:
                state["spawned"].append(code)
                return ok(500 + len(state["spawned"]) - 1)
            if code.startswith("ids = {}; for t in (queued_tasks())"):
                if state["released"]:
                    return ok([[], healthy_records()])
                return ok([[500 + n for n in range(len(jc.TASK_KINDS))], {}])
            if code.startswith(f"#{OBJECT}.receiver = "):
                state["released"] = True
                return ok(1)
            return evaluate(port, code)

        self.script["execute"] = execute
        return campaign, output, state

    def campaign(self, probe_fails=False, **options):
        result = super().campaign(probe_fails, **options)
        # CampaignTests keeps its evaluate in a closure; reach it through the script.
        campaign = result[0]
        self.script = campaign.transport_factory(0).script
        self.script_evaluate = self.script["execute"]
        return result

    def test_plant_hold_recover_release_order(self):
        campaign, output, state = self.lane_campaign()
        campaign.launch("initial")
        campaign.checkpoint()
        codes = [code for _, code in self.script_codes()]
        lifecycle = jc.load_jsonl(output / "lifecycle.jsonl")
        events = [row["event"] for row in lifecycle]
        self.assertLess(events.index("task-plant"), events.index("checkpoint"))
        plant = lifecycle[events.index("task-plant")]
        self.assertEqual([task["kind"] for task in plant["tasks"]], list(jc.TASK_KINDS))
        second = [index for index, row in enumerate(lifecycle) if row.get("generation") == 1]
        order = [lifecycle[index]["event"] for index in second]
        self.assertEqual(order[:4], ["recovery-observation", "task-recovery", "effect-connections", "task-release"])
        self.assertLess(order.index("task-release"), order.index("admit"))
        recovery = lifecycle[second[1]]
        self.assertEqual((recovery["status"], len(recovery["present"])), ("held", len(jc.TASK_KINDS)))
        release_at = next(i for i, code in enumerate(codes) if code.startswith(f"#{OBJECT}.receiver = "))
        admit_at = max(i for i, code in enumerate(codes) if code == jc.ADMIT_CODE)
        self.assertLess(release_at, admit_at)
        self.assertTrue(codes[release_at].startswith(f"#{OBJECT}.receiver = #"))
        rows, findings = jc.task_checks(lifecycle, [{"generation": 1, "line": "jepsen-task 1000106"}], [])
        self.assertEqual(findings, [])
        self.assertEqual(rows[0]["releases"], [{"generation": 1, "complete": True}])

    def test_a_plant_that_is_not_held_fails_the_campaign(self):
        campaign, output, state = self.lane_campaign()
        state["released"] = True  # tasks gone and results written already
        campaign.launch("initial")
        reply = campaign.fault("checkpoint")
        self.assertEqual(reply["type"], "info")
        self.assertIn("is not held", reply["error"]["message"])


if __name__ == "__main__":
    unittest.main()
