"""Offline tests of the exploratory batch driver (scripts/jepsen_explore.py).

Campaign subprocesses are replaced by a fake that writes the files
jepsen_checkpoint.py writes (results.json, history.jsonl); the commands it
receives are parsed by jepsen_checkpoint.py's own argument parser.
"""
import contextlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import jepsen_checkpoint as jc
import jepsen_explore as je

DEFECT = {"class": jc.CORRECTNESS, "message": "generation 2: Elle rejected the history with G2-item "
                                             "(graphs-gen-002/G2-item.txt)", "witness": "elle:G2-item"}
INVOKES = [{"process": 0, "type": "invoke", "f": "txn", "phase": 0, "value": [["append", 1, 1]]},
           {"process": 0, "type": "ok", "f": "txn", "value": [["append", 1, 1]], "generation": 0},
           {"process": "nemesis", "type": "info", "f": "crash", "value": None},
           {"process": 1, "type": "invoke", "f": "txn", "phase": 1000000, "value": [["r", 1000001, None]]},
           {"process": 1, "type": "info", "f": "txn", "value": [["r", 1000001, None]], "generation": 1,
            "error": {"kind": "transport"}}]


class FakeCampaigns:
    """subprocess.run for jepsen_checkpoint.py: each campaign gets the next
    scripted outcome ("healthy", "defect", "incomplete" or "missing"), then
    "healthy"."""

    def __init__(self, outcomes):
        self.outcomes, self.commands, self.environments = list(outcomes), [], []

    def __call__(self, command, env=None, stdout=None, stderr=None, timeout=None, check=False):
        self.commands.append(command)
        self.environments.append(env)
        output = Path(command[command.index("--output") + 1])
        if "--analyze" in command:
            source = Path(command[command.index("--analyze") + 1])
            output.mkdir(parents=True)
            (output / "results.json").write_text((source / "results.json").read_text())
            return subprocess.CompletedProcess(command, 1)
        outcome = self.outcomes.pop(0) if self.outcomes else "healthy"
        output.mkdir(parents=True)
        if outcome == "missing":
            return subprocess.CompletedProcess(command, 5)
        (output / "history.jsonl").write_text("".join(json.dumps(op) + "\n" for op in INVOKES))
        findings = {"healthy": [], "defect": [DEFECT],
                    "incomplete": [{"class": jc.INCOMPLETE, "message": "Jepsen wrote no history.jsonl"}]}[outcome]
        verdict = jc.verdict_of(findings)
        (output / "results.json").write_text(json.dumps({
            "verdict": verdict, "classes": sorted({f["class"] for f in findings}), "findings": findings,
            "outcomes": {"ok": 1, "info:transport": 1}}))
        return subprocess.CompletedProcess(command, jc.EXIT_CODES[verdict])


class Fixture(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.batches = 0

    def batch(self, outcomes, budget=600, seed=7):
        self.batches += 1
        fake = FakeCampaigns(outcomes)
        args = ["batch", "--out", str(self.root / f"batch-{self.batches}"), "--ledger", str(self.root / "ledger"),
                "--barn", "bin/barn", "--barn-elle", "bin/barn-elle", "--db", "Test.db", "--jepsen-jar", "j.jar",
                "--elle-jar", "e.jar", "--budget", str(budget), "--seed", str(seed)]
        with patch.object(je.subprocess, "run", side_effect=fake), contextlib.redirect_stdout(io.StringIO()):
            code = je.main(args)
        report = json.loads((self.root / f"batch-{self.batches}" / "report.json").read_text())
        return code, report, fake


class CoveringScheduleTests(unittest.TestCase):
    def test_greedy_schedule_covers_every_pair_in_few_runs(self):
        # The two largest factors alone need 5 x 4 = 20 runs.
        self.assertEqual(len(je.ALL_PAIRS), sum(len(je.FACTORS[a]) * len(je.FACTORS[b])
                                                for a, b in je.itertools.combinations(sorted(je.FACTORS), 2)))
        rng, covered, runs = je.random.Random(3), set(je.pairs_of(je.BASELINE)), 1
        while covered != je.ALL_PAIRS:
            config, gain = je.next_config(covered, rng)
            self.assertGreater(gain, 0)
            self.assertEqual(gain, len(je.pairs_of(config) - covered))
            self.assertEqual(set(config), set(je.FACTORS))
            covered |= je.pairs_of(config)
            runs += 1
        self.assertLessEqual(runs, 30)

    def test_plan_fills_the_budget_with_the_baseline_first_and_new_pairs(self):
        entries = je.plan(je.empty_ledger(), 1800, 1)
        self.assertEqual(entries[0]["kind"], "baseline")
        self.assertEqual(entries[0]["config"], je.BASELINE)
        self.assertTrue(all(entry["kind"] == "explore" and entry["new_pairs"] > 0 for entry in entries[1:]))
        spent = [entry["estimate_seconds"] for entry in entries]
        self.assertLess(sum(spent[:-1]), 1800)
        self.assertGreaterEqual(sum(spent), 1800)
        # Same ledger and seed, same plan.
        self.assertEqual(je.plan(je.empty_ledger(), 1800, 1), entries)

    def test_plan_starts_from_the_ledger_coverage_and_its_measured_times(self):
        ledger = je.empty_ledger()
        ledger["covered"] = sorted(je.ALL_PAIRS - {je.pair_key("lane", "stages", "tasks", "on")})
        ledger["elapsed"] = {"fixed": [50, 70]}
        entries = je.plan(ledger, 400, 1)
        self.assertEqual(entries[0]["estimate_seconds"], 70)
        self.assertEqual((entries[1]["config"]["lane"], entries[1]["config"]["tasks"], entries[1]["new_pairs"]),
                         ("stages", "on", 1))
        # Every pair now covered: the next configuration opens a new round.
        self.assertEqual(entries[2]["new_pairs"], len(je.pairs_of(entries[2]["config"])))

    def test_open_findings_are_reexecuted_first_within_half_the_budget(self):
        ledger = je.empty_ledger()
        for index, (attempts, reproductions) in enumerate([(1, 0), (2, 2), (5, 1), (0, 0), (0, 0), (1, 0)]):
            ledger["findings"][f"f{index}"] = {
                "id": f"f{index}", "key": f"k{index}", "replayable": index != 4, "attempts": attempts,
                "reproductions": reproductions, "first_seen": {"batch": "1", "run": "r"}, "estimate_seconds": 100,
                "parameters": {"seed": index}}
        entries = je.plan(ledger, 1000, 1)
        # f2 has its five attempts and f4 kept no transactions; f1 reproduced, so it leads.
        self.assertEqual([entry["finding"] for entry in entries if entry["kind"] == "reexecute"], ["f1", "f3", "f0"])
        self.assertEqual(entries[3]["kind"], "baseline")
        entries = je.plan(ledger, 500, 1)
        self.assertEqual([entry.get("finding") for entry in entries[:3]], ["f1", "f3", None])


class CommandTests(unittest.TestCase):
    PATHS = {"barn": "bin/barn", "barn_elle": "bin/barn-elle", "db": "Test.db", "jepsen_jar": "j.jar",
             "elle_jar": "e.jar", "java": "java"}

    def parse(self, command):
        """The command's arguments as jepsen_checkpoint.main parses them."""
        seen = {}

        def campaign(args, output):
            seen.update(vars(args))
        with tempfile.TemporaryDirectory() as directory:
            jar = Path(directory) / "e.jar"
            jar.write_bytes(b"jar")
            argv = command[2:]
            argv[argv.index("--elle-jar") + 1] = str(jar)
            argv[argv.index("--output") + 1] = str(Path(directory) / "out")
            with patch.object(jc, "run_campaign", side_effect=campaign), \
                    patch.object(jc, "analyze", return_value=({}, [])), contextlib.redirect_stdout(io.StringIO()):
                jc.main(argv)
        return seen

    def test_every_lane_builds_a_command_the_campaign_accepts(self):
        for lane in je.FACTORS["lane"]:
            with self.subTest(lane=lane):
                config = dict(je.BASELINE, lane=lane, tasks="on", hot_keys="2", gomaxprocs="1",
                              families="effects")
                parameters = je.parameters_of(config, 99, 300)
                command, environment = je.campaign_command(parameters, self.PATHS, "out")
                args = self.parse(command)
                self.assertEqual(command[1], str(je.CAMPAIGN))
                self.assertEqual(environment["GOMAXPROCS"], "1")
                self.assertEqual((args["workloads"], args["seed"], args["tasks"], args["hot_keys"],
                                  args["jepsen_timeout"]), (["effects"], 99, True, 2, 300))
                self.assertEqual(args["barn"], Path("bin/barn-elle" if lane == "stages" else "bin/barn"))
                self.assertEqual(args["stages"], list(jc.STAGE_SCHEDULE) if lane == "stages" else [])
                self.assertEqual(args["phase_transport"], 40 if lane == "transport" else 0)
                self.assertEqual(args["phase_live"], 40 if lane == "live" else 0)

    def test_runner_gomaxprocs_is_left_to_go_and_replay_is_passed(self):
        parameters = je.parameters_of(je.BASELINE, 1, 100)
        with patch.dict(je.os.environ, {"GOMAXPROCS": "8"}):
            command, environment = je.campaign_command(parameters, self.PATHS, "out", replay=Path("r.jsonl"))
        self.assertNotIn("GOMAXPROCS", environment)
        self.assertEqual(command[-2:], ["--replay", "r.jsonl"])
        self.assertNotIn("--hot-keys", command)

    def test_recorded_parameters_rebuild_the_recorded_command(self):
        parameters = je.parameters_of(dict(je.BASELINE, lane="transport", gomaxprocs="2"), 5, 200)
        provenance = {"parameters": {key: value for key, value in parameters.items() if key != "gomaxprocs"},
                      "runner": {"environment": {"GOMAXPROCS": "2"}}}
        self.assertEqual(je.parameters_from_provenance(provenance), parameters)
        # Campaigns recorded before --hot-keys ran each family's own key count.
        del provenance["parameters"]["hot_keys"]
        self.assertEqual(je.parameters_from_provenance(provenance, timeout=9)["hot_keys"], 0)
        self.assertEqual(je.parameters_from_provenance(provenance, timeout=9)["jepsen_timeout"], 9)


class FindingKeyTests(unittest.TestCase):
    def test_witness_names_a_defect_and_numbers_do_not(self):
        self.assertEqual(je.finding_key(DEFECT), "correctness-violation|elle:G2-item")
        first = {"class": jc.RECOVERY, "message": "generation 3: token 1000004:7 lost after recovery"}
        second = {"class": jc.RECOVERY, "message": "generation 5: token 2000001:9 lost after recovery"}
        self.assertEqual(je.finding_key(first), je.finding_key(second))
        self.assertEqual(je.finding_key(first), "recovery-failure|generation N: token N:N lost after recovery")


class BatchTests(Fixture):
    def test_clean_batch_is_complete_and_records_coverage_and_times(self):
        code, report, fake = self.batch(["healthy"] * 20)
        self.assertEqual((code, report["outcome"], report["complete"]), (0, "clean", True))
        self.assertEqual(report["runs"][0]["kind"], "baseline")
        ledger = je.load_ledger(self.root / "ledger")
        exercised = set().union(*(je.pairs_of(row["config"]) for row in report["runs"]))
        self.assertEqual(set(ledger["covered"]), exercised)
        self.assertEqual(report["yield"]["combinations"]["covered_pairs"], len(exercised))
        self.assertEqual(report["yield"]["unknown_outcome_fraction"], 0.5)
        self.assertEqual(report["yield"]["incomplete_run_fraction"], 0)
        self.assertEqual(report["yield"]["time_to_first_witness_seconds"], None)
        self.assertEqual(ledger["batches"], 1)
        self.assertTrue(ledger["elapsed"]["fixed"])
        self.assertFalse((self.root / "batch-1" / "drafts").exists())
        # Each run has its own new evidence directory.
        outputs = [command[command.index("--output") + 1] for command in fake.commands]
        self.assertEqual(len(outputs), len(set(outputs)))

    def test_defect_is_red_but_complete_kept_for_reexecution_and_drafted(self):
        code, report, fake = self.batch(["healthy", "defect"] + ["healthy"] * 20)
        self.assertEqual((code, report["outcome"], report["complete"]), (1, "finding", True))
        key = je.finding_key(DEFECT)
        self.assertEqual(report["yield"]["distinct_defects"], [key])
        self.assertIsNotNone(report["yield"]["time_to_first_witness_seconds"])
        defect_run = report["runs"][1]
        # The offline re-analysis wrote a new directory and found the same defect.
        self.assertEqual(defect_run["offline"]["defects"], [key])
        self.assertTrue((self.root / "batch-1" / defect_run["offline"]["evidence"] / "results.json").exists())
        self.assertIn("--analyze", fake.commands[2])
        ledger = je.load_ledger(self.root / "ledger")
        finding = ledger["findings"][je.finding_id(key)]
        self.assertEqual((finding["replayable"], finding["attempts"], finding["sightings"]), (True, 0, 1))
        replay = self.root / "ledger" / "findings" / finding["id"] / "replay.jsonl"
        self.assertEqual(jc.load_jsonl(replay), jc.replay_invokes(INVOKES))
        draft = json.loads((self.root / "batch-1" / "drafts" / f"{finding['id']}.json").read_text())
        self.assertEqual((draft["status"], draft["key"], draft["offline_reanalysis"]),
                         ("draft (not posted)", key, [{"run": defect_run["evidence"], "reproduced": True}]))

        # The next batch re-executes it first, from the kept transactions.
        code, report, fake = self.batch(["defect"] + ["healthy"] * 20, seed=8)
        self.assertEqual(code, 1)
        first = report["runs"][0]
        self.assertEqual((first["kind"], first["finding"], first["reproduced"]), ("reexecute", finding["id"], True))
        self.assertEqual(fake.commands[0][-2:], ["--replay", str(replay)])
        self.assertEqual(fake.commands[0][fake.commands[0].index("--seed") + 1], str(finding["parameters"]["seed"]))
        ledger = je.load_ledger(self.root / "ledger")
        finding = ledger["findings"][finding["id"]]
        self.assertEqual((finding["attempts"], finding["reproductions"], finding["sightings"]), (1, 1, 2))
        self.assertEqual(report["yield"]["reproducible_defects"], [key])

        # A re-execution that does not reproduce counts as an attempt only.
        code, report, _ = self.batch(["healthy"] * 20, seed=9)
        self.assertEqual((code, report["runs"][0]["reproduced"]), (0, False))
        finding = je.load_ledger(self.root / "ledger")["findings"][finding["id"]]
        self.assertEqual((finding["attempts"], finding["reproductions"]), (2, 1))

    def test_missing_results_or_incomplete_run_is_never_clean(self):
        for outcome, complete in (("missing", False), ("incomplete", True)):
            with self.subTest(outcome=outcome):
                code, report, _ = self.batch(["healthy", outcome] + ["healthy"] * 20)
                self.assertEqual((code, report["outcome"], report["complete"]), (4, "incomplete", complete))
                self.assertEqual(report["yield"]["incomplete_runs"], [report["runs"][1]["name"]])
                self.assertGreater(report["yield"]["incomplete_run_fraction"], 0)

    def test_run_without_results_adds_no_coverage(self):
        code, report, _ = self.batch(["missing"] + ["healthy"] * 20)
        ledger = je.load_ledger(self.root / "ledger")
        rest = set().union(*(je.pairs_of(row["config"]) for row in report["runs"][1:]))
        self.assertEqual(set(ledger["covered"]), rest)
        self.assertEqual(code, 4)

    def test_runs_that_do_not_fit_the_remaining_budget_are_deferred(self):
        code, report, fake = self.batch(["healthy"], budget=1)
        self.assertEqual((code, report["outcome"], report["runs"]), (4, "incomplete", []))
        self.assertEqual(len(report["deferred"]), 1)
        self.assertEqual(fake.commands, [])

    def test_existing_output_or_foreign_ledger_version_is_refused(self):
        self.batch(["healthy"] * 20)
        with self.assertRaises(FileExistsError):
            self.batches -= 1
            self.batch(["healthy"] * 20)
        (self.root / "ledger" / "ledger.json").write_text(json.dumps({"version": 99}))
        with self.assertRaisesRegex(ValueError, "ledger version"):
            je.load_ledger(self.root / "ledger")


if __name__ == "__main__":
    unittest.main()
