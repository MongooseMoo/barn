"""Fixtures for the authority and topology families (plan section 3 rows 3
and 4): never launch Barn, Jepsen or Elle. Each defect fixture must give its
own witness."""
import copy
import json
from pathlib import Path
import tempfile
import unittest

import jepsen_checkpoint as jc
from test_jepsen_checkpoint import FakeCampaign, FakeTransport, native, ok

AUTH, TOPO = jc.FAMILIES["authority"]["base"], jc.FAMILIES["topology"]["base"]
AR, TR = jc.AUTHORITY_ROLES, jc.TOPOLOGY_ROLES

# Object numbers Barn's moo interpreter gave the authority and topology setup
# code on a fresh pinned Test.db (authority_setup_code / topology_setup_code).
AUTHORITY_OBJECTS = dict(zip(jc.AUTHORITY_LAYOUT, [11, 8, 9, 10, 12, 13, 14, 15, 16, 17, 18]))
TOPOLOGY_OBJECTS = dict(zip(jc.TOPOLOGY_LAYOUT, [10, 8, 9, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21]))
LAYOUT = {"authority": AUTHORITY_OBJECTS, "topology": TOPOLOGY_OBJECTS}

# Raw replies of authority verbs, in run order (a few V-machine reads left
# out), from Barn's moo interpreter (bin/moo -db Test.db) running
# authority_setup_code and then each call as a separate task: the expected
# errors, slot inheritance and dispatch that the replay must accept. The values agree with Toast's property.cc and
# db_properties.cc rules the expectations encode.
AUTHORITY_RUN = [
    ("p_read()", [[[], []], [], ["E_PERM"]]),
    ("p_write(101)", [[[], []], [1], []]),
    ("p_read()", [[[], [101]], [], ["E_PERM"]]),
    ("p_flags(100)", [[[]], [1], ["ok"]]),
    ("p_read()", [[[100], "E_PERM"], [], ["E_PERM"]]),
    ("p_write(102)", [[[100], "E_PERM"], ["E_PERM"], []]),
    ("p_flags(103)", [[[100]], [1], ["ok"]]),
    ("p_write(106)", [[[100, 103], [101]], [1], []]),
    ("p_chown(104)", [[[100, 103]], [1], [10]]),
    ("p_write(107)", [[[100, 103, 104], [101, 106]], [1], []]),
    ("p_flags(108)", [[[100, 103, 104]], [1], ["E_PERM"]]),
    ("p_read()", [[[100, 103, 104, 108], [101, 106, 107]], [], ["E_PERM"]]),
    ("p_chown(109)", [[[100, 103, 104, 108]], [1], [8]]),
    ("p_flags(111)", [[[100, 103, 104, 108, 109]], [1], ["ok"]]),
    ("f_observe()", [[[]], [], [1, [0], [-1], 1, 13, "E_PERM", "E_PERM"]]),
    ("f_mutate(300)", [[[]], [1], ["deleted"]]),
    ("f_observe()", [[[300]], [], [0, "E_PROPNF", [-1], 1, 13, "E_PERM", "E_PROPNF"]]),
    ("f_mutate(301)", [[[300]], [1], ["set"]]),
    ("f_observe()", [[[300, 301]], [], [0, "E_PROPNF", [301], 0, 13, "E_PERM", "E_PROPNF"]]),
    ("f_mutate(302)", [[[300, 301]], [1], [14]]),
    ("f_observe()", [[[300, 301, 302]], [], [0, "E_PROPNF", [-2], 1, 14, "E_PERM", "E_PROPNF"]]),
    ("f_mutate(303)", [[[300, 301, 302]], [1], ["added"]]),
    ("f_observe()", [[[300, 301, 302, 303]], [], [1, "E_PROPNF", [-2], 1, 14, "E_PERM", "E_PROPNF"]]),
    ("f_mutate(304)", [[[300, 301, 302, 303]], [1], ["set"]]),
    ("f_observe()", [[[300, 301, 302, 303, 304]], [], [1, "E_PROPNF", [304], 0, 14, "E_PERM", "E_PROPNF"]]),
    ("f_mutate(305)", [[[300, 301, 302, 303, 304]], [1], [13]]),
    ("f_observe()", [[[300, 301, 302, 303, 304, 305]], [], [1, [303], [-1], 1, 13, "E_PERM", "E_PERM"]]),
    ("v_read()", [[[], []], [], []]),
    ("v_mutate(600)", [[[], []], [1], [0]]),
    ("v_mutate(601)", [[[], [600]], [1], [1]]),
    ("v_observe(700)", [[[600, 601]], [1], []]),
    ("v_read()", [[[600, 601], [700]], [], []]),
    ("v_mutate(602)", [[[700], [600, 601]], [1], [2]]),
    ("f_mutate(307)", [[[300, 301, 302, 303, 304, 305]], [1], ["set"]]),
    ("f_observe()", [[[300, 301, 302, 303, 304, 305, 307]], [], [1, [303], [307], 0, 13, "E_PERM", "E_PERM"]]),
    ("f_mutate(310)", [[[300, 301, 302, 303, 304, 305, 307]], [1], ["cleared"]]),
    ("f_observe()", [[[300, 301, 302, 303, 304, 305, 307, 310]], [], [1, [303], [-1], 1, 13, "E_PERM", "E_PERM"]]),
]
# The c_observe replies of the same kind of run of topology_setup_code.
TOPOLOGY_OBSERVATIONS = [
    ([[10, 11, 12], [], [], []],
     [[[[26, 1, 10, 14, 20, 3], [27, 1, 11, 14, 20, 3], [28, 1, 12, 14, 20, 3]], [], []],
      [[[26, 1, 10, 20], [27, 1, 11, 20], [28, 1, 12, 20]], []], []]),
    ([[10, 11, 12], [], [13, 14], [15]],
     [[[[28, 1, 12, 14, 21, 4]], [[26, 1, 10, 15, 20, 3]], [[27, 1, 11, 16, 20, 3]]],
      [[[26, 1, 10, 20], [27, 1, 11, 20]], [[28, 1, 12, 21]]], []]),
    ([[10, 11, 12], [16], [13, 14], [15]],
     [[[], [[26, 1, 10, 15, 20, 3]], [[27, 1, 11, 16, 20, 3]]], [[[26, 1, 10, 20], [27, 1, 11, 20]], []],
      [[16, 12, 28, 0]]]),
    ([[10, 11, 12], [16, 17, 18, 19], [13, 14], [15]],
     [[[], [], []], [[], []], [[16, 12, 28, 0], [17, 10, 26, 0], [18, 11, 27, 0], [19, 0, -1, 0]]]),
]

SHAPE_OF_VERB = {"p_read": "p-read", "p_write": "p-write", "p_flags": "p-mutate", "p_chown": "p-mutate",
                 "f_observe": "f-observe", "f_mutate": "f-mutate", "v_read": "v-read", "v_mutate": "v-mutate",
                 "v_observe": "v-observe"}


def shaped_txn(family, shape, v=None):
    """The Jepsen invocation value of one shape, as core.clj shaped-txns builds it."""
    roles, shapes = jc.SHAPED[family]
    base = jc.FAMILIES[family]["base"]
    return [[f, base + roles[role], v if f == "append" else None] for f, role in shapes[shape][1]]


def completed(family, shape, v, reply, generation=0, op_id=1):
    """One ok shaped transaction record (shaped_transactions' form) from a raw verb reply."""
    txn = shaped_txn(family, shape, v)
    done = jc.complete_shaped(family, txn, ok(reply))
    return {"id": op_id, "generation": generation, "request": txn, "value": done["value"], "denied": done["denied"],
            "shape": done["shape"], "observations": done["observations"]}


def authority_run():
    txns = []
    for index, (call, reply) in enumerate(AUTHORITY_RUN):
        verb, _, rest = call.partition("(")
        v = int(rest[:-1]) if rest[:-1] else None
        txns.append(completed("authority", SHAPE_OF_VERB[verb], v, reply, op_id=index + 1))
    return txns


def topology_observation(ledgers, observations):
    reply = [ledgers, [], observations]
    return completed("topology", "c-observe", None, reply)


def violations(txn):
    return [(witness, expected, observed) for witness, _, expected, observed
            in jc.authority_expectations(txn, AUTHORITY_OBJECTS) if expected != observed]


class ShapeTests(unittest.TestCase):
    def test_every_shape_is_identified_from_its_invocation(self):
        for family, (_, shapes) in jc.SHAPED.items():
            for shape in shapes:
                with self.subTest(shape=shape):
                    self.assertEqual(jc.identify_shape(family, shaped_txn(family, shape, 5)), shape)

    def test_jepsen_gets_every_shape_with_integer_roles(self):
        table = jc.FAMILIES["authority"]["shapes"]
        self.assertEqual({row["name"] for row in table}, set(jc.AUTHORITY_SHAPES))
        self.assertIn({"name": "p-write", "weight": 3, "mops": [["r", 0], ["r", 1], ["append", 1]]}, table)
        json.dumps(jc.FAMILIES)  # passed to core.clj as --families JSON

    def test_unknown_shape_mixed_family_or_two_values_are_refused(self):
        for txn in ([["r", AUTH + AR["d"], None]],
                    [["r", AUTH + AR["mp"], None], ["r", TOPO, None]],
                    [["append", TOPO + TR["cr1"], 1], ["append", TOPO + TR["ch1"], 2]],
                    [["r", AUTH + AR["mp"], 3]]):
            with self.subTest(txn=txn), self.assertRaises(ValueError):
                jc.identify_shape(jc.family_of(txn[0][1]), txn)

    def test_each_shape_is_one_verb_call_and_p_mutate_kind_4_is_the_chown_control(self):
        self.assertEqual(jc.shaped_code("authority", shaped_txn("authority", "p-mutate", 9), LAYOUT),
                         "return #11:p_chown(9);")
        self.assertEqual(jc.shaped_code("authority", shaped_txn("authority", "p-mutate", 7), LAYOUT),
                         "return #11:p_flags(7);")
        self.assertEqual(jc.shaped_code("authority", shaped_txn("authority", "f-observe"), LAYOUT),
                         "return #11:f_observe();")
        self.assertEqual(jc.shaped_code("topology", shaped_txn("topology", "t-cross-2", 4), LAYOUT),
                         "return #10:t_cross(2, 1, 4);")
        with self.assertRaises(ValueError):
            jc.shaped_code("topology", shaped_txn("topology", "t-read"), {})

    def test_observations_read_shaped_rows_as_the_wizard_and_clients_never_use_txn_code(self):
        keys = [AUTH + AR["mp"], AUTH + AR["d"], AUTH + AR["dsp"], TOPO + TR["cr2"], TOPO + TR["ch1"],
                TOPO + TR["moves"]]
        code = jc.observation_code(keys, LAYOUT)
        for part in ("#11.mp", "#12.x", "#18:probe()", "#10:tokens(#13.contents)", "#10:tokens(children(#18))",
                     "#10.moves"):
            self.assertIn(part, code)
        with self.assertRaises(ValueError):
            jc.txn_code([["r", AUTH + AR["mp"], None], ["append", AUTH + AR["mp"], 1]], LAYOUT)

    def test_setup_code_builds_the_layout_and_each_verb_with_its_identity(self):
        code = jc.authority_setup_code(dwell=7)
        self.assertIn('add_verb(o, {player, "xd", "p_chown"}', code)  # the wizard control verb
        for verb, identity in (("p_flags", "own"), ("p_read", "rd"), ("p_write", "wr"), ("f_observe", "rd"),
                               ("v_mutate", "own"), ("v_observe", "wr")):
            self.assertIn(f'add_verb(o, {{{identity}, "xd", "{verb}"}}', code)
        self.assertIn("for dwell_i in [1..7]", code)
        self.assertNotIn("dwell_i", jc.authority_setup_code())
        self.assertEqual(jc.parse_shaped_layout("topology", list(range(14)))["tq4"], 13)
        with self.assertRaises(ValueError):
            jc.parse_shaped_layout("authority", [1, 2])


class CompletionTests(unittest.TestCase):
    def test_denied_read_and_append_are_listed_and_left_out_of_the_effective_value(self):
        txn = shaped_txn("authority", "p-write", 5)
        done = jc.complete_shaped("authority", txn, ok([[[1], "E_PERM"], ["E_PERM"], []]))
        self.assertEqual(done["type"], "ok")
        self.assertEqual(done["value"], [["r", AUTH, [1]]])
        self.assertEqual(done["denied"], [["r", AUTH + 1, "E_PERM"], ["append", AUTH + 1, "E_PERM"]])
        self.assertEqual(done["shape"], "p-write")
        done = jc.complete_shaped("authority", txn, ok([[[1], [2]], [1], []]))
        self.assertEqual(done["value"], [["r", AUTH, [1]], ["r", AUTH + 1, [2]], ["append", AUTH + 1, 5]])
        self.assertEqual(done["denied"], [])

    def test_native_error_or_malformed_reply_is_unknown(self):
        txn = shaped_txn("topology", "c-move", 3)
        self.assertEqual(jc.complete_shaped("topology", txn, native("E_TICKS"))["type"], "info")
        for value in ([[[1]], [1]], [[[1], [2]], [1], []], [[[1]], [2], []], [[1], [1], []], "x"):
            with self.subTest(value=value):
                done = jc.complete_shaped("topology", txn, ok(value))
                self.assertEqual((done["type"], done["error"]["kind"]), ("info", "malformed-response"))


class SessionTests(unittest.TestCase):
    def test_shaped_transactions_use_a_programmer_connection_and_the_verb_call(self):
        users = []

        class Recording(FakeTransport):
            def connect(self, user):
                users.append(user)

        script = {"execute": lambda port, code: ok([[[4]], [1], ["ok"]]) if "p_flags" in code else ok([[1]])}
        campaign = FakeCampaign((2, 9000), lambda port: Recording(port, script))
        campaign.layouts = {2: LAYOUT}
        session = jc.ClientSession(campaign)
        reply = session.txn(6, 0, shaped_txn("authority", "p-mutate", 3))
        self.assertEqual((reply["type"], reply["value"], reply["shape"], reply["observations"]),
                         ("ok", [["r", AUTH, [4]], ["append", AUTH, 3]], "p-mutate", ["ok"]))
        self.assertEqual(script["codes"], [(9000, "return #11:p_flags(3);")])
        session.txn(7, 0, [["r", 1, None]])
        self.assertEqual(users, ["programmer", "wizard"])
        self.assertEqual(campaign.events[-1]["value"], [["r", 1, [1]]])


class AuthorityTests(unittest.TestCase):
    def test_the_raw_barn_run_satisfies_every_expectation(self):
        for txn in authority_run():
            with self.subTest(operation=txn["id"], shape=txn["shape"]):
                self.assertEqual(violations(txn), [])

    def test_replays(self):
        self.assertEqual(jc.replay_permissions([]), ("owner", "rw"))
        self.assertEqual(jc.replay_permissions([100, 104, 108]), ("writer", ""))  # flags frozen while chowned
        self.assertEqual(jc.replay_permissions([104, 109, 111]), ("owner", "r"))
        self.assertEqual(jc.replay_definitions([]), {"y": True, "y_value": [0], "z_local": None, "parent": "fa"})
        self.assertEqual(jc.replay_definitions([300, 301, 302, 303]),
                         {"y": True, "y_value": [303], "z_local": None, "parent": "fb"})

    def mutate(self, index, **changes):
        txn = copy.deepcopy(authority_run()[index])
        txn.update(changes)
        return violations(txn)

    def test_permitted_read_after_revocation_is_stale_permission(self):
        # p_read after p_flags(100) revoked r: reading the list instead of E_PERM.
        self.assertEqual(self.mutate(4, value=[["r", AUTH, [100]], ["r", AUTH + 1, []]], denied=[]),
                         [("stale-permission", "E_PERM", "list")])

    def test_denied_read_while_readable_is_stale_permission(self):
        self.assertEqual(self.mutate(11, value=[["r", AUTH, [100, 103, 104, 108]]],
                                     denied=[["r", AUTH + 1, "E_PERM"]]),
                         [("stale-permission", "list", "E_PERM")])

    def test_write_through_a_revoked_write_flag_is_stale_permission(self):
        # p_write(102) with flags "": the append must be denied.
        self.assertEqual(self.mutate(5, value=[["r", AUTH, [100]], ["r", AUTH + 1, [101]], ["append", AUTH + 1, 102]],
                                     denied=[]),
                         [("stale-permission", "E_PERM", "list"), ("stale-permission", "E_PERM", "written")])

    def test_wrong_error_code_is_error_precedence(self):
        self.assertEqual(self.mutate(2, observations=["E_INVARG"]), [("error-precedence", ["E_PERM"], ["E_INVARG"])])
        self.assertEqual(self.mutate(5, denied=[["r", AUTH + 1, "E_PROPNF"], ["append", AUTH + 1, "E_PROPNF"]]),
                         [("error-precedence", "E_PERM", "E_PROPNF"), ("error-precedence", "E_PERM", "E_PROPNF")])
        # delete_property must check object write before PROPNF; clear_property PROPNF before PERM.
        observe = AUTHORITY_RUN[16][1][2]
        self.assertEqual(self.mutate(16, observations=observe[:5] + ["E_PROPNF", "E_PERM"]),
                         [("error-precedence", "E_PERM", "E_PROPNF"), ("error-precedence", "E_PROPNF", "E_PERM")])

    def test_flag_change_by_a_former_owner_and_chown_results_are_mutation_outcomes(self):
        self.assertEqual(self.mutate(10, observations=["ok"]), [("mutation-outcome", ["E_PERM"], ["ok"])])
        self.assertEqual(self.mutate(8, observations=[8]), [("mutation-outcome", [10], [8])])
        self.assertEqual(self.mutate(15, observations=["added"]), [("mutation-outcome", ["deleted"], ["added"])])

    def test_stale_definitions_and_inheritance(self):
        observe = AUTHORITY_RUN[26][1][2]  # after y re-added and FC back under FA
        self.assertEqual(self.mutate(26, observations=[0] + observe[1:]), [("stale-definition", 1, 0)])
        self.assertEqual(self.mutate(26, observations=observe[:1] + ["E_PROPNF"] + observe[2:]),
                         [("stale-definition", [303], "E_PROPNF")])
        self.assertEqual(self.mutate(26, observations=observe[:2] + [[-2]] + observe[3:]),
                         [("stale-inheritance", [-1], [-2])])
        self.assertEqual(self.mutate(26, observations=observe[:3] + [0] + observe[4:]),
                         [("stale-inheritance", 1, 0)])
        self.assertEqual(self.mutate(26, observations=observe[:4] + [14] + observe[5:]),
                         [("stale-inheritance", 13, 14)])

    def test_dispatch_rows_are_left_to_elle(self):
        self.assertEqual(jc.authority_expectations(authority_run()[28], AUTHORITY_OBJECTS), [])

    def test_coverage_labels(self):
        labels = {label for txn in authority_run() for label in jc.authority_coverage(txn)}
        self.assertEqual(set(jc.AUTHORITY_COVERAGE) - labels, set())


def grave(v, token, obj, back=0):
    return [v, token, obj, back]


class TopologyTests(unittest.TestCase):
    def snapshot(self, index):
        ledgers, observations = copy.deepcopy(TOPOLOGY_OBSERVATIONS[index])
        return ledgers, observations

    def problems(self, ledgers, observations):
        return [witness for witness, _ in jc.topology_violations(topology_observation(ledgers, observations),
                                                                 TOPOLOGY_OBJECTS)]

    def test_the_raw_barn_snapshots_are_consistent(self):
        for index in range(len(TOPOLOGY_OBSERVATIONS)):
            with self.subTest(snapshot=index):
                self.assertEqual(self.problems(*self.snapshot(index)), [])

    def test_object_in_two_rooms_is_reciprocal_and_not_conserved(self):
        ledgers, observations = self.snapshot(1)
        observations[0][0].append([26, 1, 10, 15, 20, 3])  # #26 also listed in T3, located in T4
        self.assertEqual(self.problems(ledgers, observations), ["reciprocal-topology", "token-not-conserved"])

    def test_child_with_another_parent_is_reciprocal(self):
        ledgers, observations = self.snapshot(1)
        observations[1][0][0][3] = 21
        self.assertEqual(self.problems(ledgers, observations), ["reciprocal-topology"])

    def test_lost_or_resurrected_token_is_not_conserved(self):
        ledgers, observations = self.snapshot(1)
        observations[0][1] = []  # #26 (token 10) vanished from every room
        self.assertEqual(self.problems(ledgers, observations), ["token-not-conserved"])
        ledgers, observations = self.snapshot(2)
        observations[0][0] = [[28, 1, 12, 14, 21, 4]]  # recycled token 12 still in T3
        self.assertEqual(self.problems(ledgers, observations), ["token-not-conserved"])

    def test_invalid_member_or_recycled_object_back_with_its_token_is_an_invalid_reference(self):
        ledgers, observations = self.snapshot(2)
        observations[0][0] = [[28, 0]]
        self.assertEqual(self.problems(ledgers, observations), ["invalid-reference"])
        ledgers, observations = self.snapshot(2)
        observations[2] = [grave(16, 12, 28, 1)]
        self.assertEqual(self.problems(ledgers, observations), ["invalid-reference"])
        ledgers, observations = self.snapshot(2)
        observations[1][1] = [[28, 0]]
        self.assertEqual(self.problems(ledgers, observations), ["invalid-reference"])

    def test_dispatch_through_the_old_parent_is_stale(self):
        ledgers, observations = self.snapshot(1)
        observations[0][0][0][5] = 3  # parent TQ4 but kind() answered TQ3's 3
        self.assertEqual(self.problems(ledgers, observations), ["stale-dispatch"])

    def test_ledger_and_graves_disagreeing_or_a_double_recycle(self):
        ledgers, observations = self.snapshot(2)
        ledgers[1] = [16, 17]
        self.assertEqual(self.problems(ledgers, observations), ["token-not-conserved"])
        ledgers, observations = self.snapshot(3)
        observations[2][1] = grave(17, 12, 26)
        problems = self.problems(ledgers, observations)
        self.assertIn("duplicate-recycle", problems)


def journal_for(family, txns):
    journal = []
    for txn in txns:
        journal += [{"event": "request", "id": txn["id"], "value": txn["request"]},
                    {"event": "send", "id": txn["id"], "generation": txn["generation"]},
                    {"event": "complete", "id": txn["id"], "type": "ok", "value": txn["value"],
                     "denied": txn["denied"], "shape": txn["shape"], "observations": txn["observations"]}]
    return journal


class ShapedChecksTests(unittest.TestCase):
    lifecycle = [{"event": "admit", "generation": 0, "rows": LAYOUT}]

    def test_a_healthy_run_has_no_correctness_finding_and_reports_missing_coverage(self):
        txns = authority_run()
        results, findings = jc.shaped_checks(self.lifecycle, journal_for("authority", txns), ["authority"])
        self.assertEqual([item for item in findings if item[0] != jc.INCOMPLETE], [])
        self.assertEqual(results["authority"]["checked"], len(txns))
        self.assertEqual(results["authority"]["denied"], 3)  # one p_read read, one p_write read and its append

        self.assertEqual(results["authority"]["missing"], [])
        results, findings = jc.shaped_checks(self.lifecycle, journal_for("authority", txns[:3]), ["authority"])
        self.assertIn((jc.INCOMPLETE, "authority-coverage", "authority: no successful transaction showed p-chown"),
                      findings)

    def test_violation_is_a_correctness_finding_with_its_witness(self):
        txns = authority_run()
        txns[2]["observations"] = ["ok"]
        _, findings = jc.shaped_checks(self.lifecycle, journal_for("authority", txns), ["authority"])
        self.assertEqual([(kind, witness) for kind, witness, _ in findings],
                         [(jc.CORRECTNESS, "error-precedence")])

    def test_topology_coverage_and_missing_layout(self):
        observe = topology_observation(*copy.deepcopy(TOPOLOGY_OBSERVATIONS[0]))
        results, findings = jc.shaped_checks(self.lifecycle, journal_for("topology", [observe]), ["topology"])
        self.assertEqual(results["topology"]["coverage"], {"c-observe": 1})
        self.assertIn("c-recycle:applied", results["topology"]["missing"])
        observe["generation"] = 5
        _, findings = jc.shaped_checks(self.lifecycle, journal_for("topology", [observe]), ["topology"])
        self.assertEqual(findings[0][:2], (jc.INVALID, "topology-layout"))


class ElleAndOracleTests(unittest.TestCase):
    def test_elle_sees_only_effective_micro_ops(self):
        denied = shaped_txn("authority", "p-write", 9)
        ops = [{"process": 1, "type": "invoke", "f": "txn", "value": denied},
               {"process": 1, "type": "ok", "f": "txn", "value": [["r", AUTH, [3]]]},
               {"process": 2, "type": "invoke", "f": "txn", "value": [["r", AUTH + 1, None]]},
               {"process": 2, "type": "ok", "f": "txn", "value": []},
               {"process": 3, "type": "invoke", "f": "txn", "value": [["append", 5, 1]]},
               {"process": 3, "type": "info", "f": "txn", "value": [["append", 5, 1]]}]
        ready = jc.elle_ops(ops)
        self.assertEqual([op["value"] for op in ready],
                         [[["r", AUTH, None]], [["r", AUTH, [3]]], [["append", 5, 1]], [["append", 5, 1]]])

    def test_snapshot_oracle_counts_only_effective_writes(self):
        journal = [{"event": "request", "id": 1, "value": shaped_txn("authority", "p-write", 9)},
                   {"event": "send", "id": 1, "generation": 0, "monotonic_ns": 1},
                   {"event": "complete", "id": 1, "type": "ok", "value": [["r", AUTH, [3]], ["r", AUTH + 1, []]],
                    "monotonic_ns": 2}]
        self.assertEqual(jc.epoch_transactions(journal, 0)[0]["writes"], [])
        journal[2]["type"] = "info"
        self.assertEqual(jc.epoch_transactions(journal, 0)[0]["writes"], [(AUTH + 1, 9)])

    def test_elle_anomaly_names_come_from_its_witness_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.assertEqual(jc.elle_anomalies(root / "absent"), [])
            (root / "G2-item").mkdir()
            for name in ("G2-item.txt", "G-single-item.txt"):
                (root / name).write_text("witness")
            (root / "sccs").mkdir()
            self.assertEqual(jc.elle_anomalies(root), ["G-single-item", "G2-item"])

    def test_isolation_controls_pass_only_on_their_elle_witness(self):
        for control in ("relationship-reads", "verb-memo", "hot-memo-guard"):
            with self.subTest(control=control):
                spec = jc.CONTROLS[control]
                self.assertNotIn("stage", spec)
                hit = {"findings": [{"class": jc.CORRECTNESS, "message": "m", "witness": "elle:G2-item"}]}
                miss = {"findings": [{"class": jc.CORRECTNESS, "message": "m", "witness": "elle:G-single-item"}]}
                self.assertTrue(jc.control_result(control, hit)["passed"])
                self.assertFalse(jc.control_result(control, miss)["passed"])
                self.assertFalse(jc.control_result(control, {})["passed"])


if __name__ == "__main__":
    unittest.main()
