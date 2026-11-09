"""Offline execution of the workflow's real routing and replenishment owner."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
CONFIG = {"FUZZ_RUNNER_LABELS": '["self-hosted","Linux","X64","barn-fuzz"]',
          "FUZZ_RUNNER_POLICY_VERIFIED": "true", "FUZZ_MAX_PARALLEL": "2",
          "FUZZ_CONTINUOUS_ENABLED": "true"}
SOURCE = {"id": 10, "event": "workflow_dispatch", "head_branch": "master",
          "path": ".github/workflows/fuzz.yml", "conclusion": "success",
          "display_title": "Nightly fuzz / dedicated-continuous",
          "head_repository": {"full_name": "test/barn"}}
ACCEPTED = [{"name": "Accept discovery cycle", "conclusion": "success"}]
COMPARE = "Compare retained public source on Barn and Toast"


def job(name, conclusion="success", failed=None):
    steps = [{"name": "Upload retained corpus and native evidence", "conclusion": "success"}]
    if failed:
        steps.append({"name": failed, "conclusion": "failure"})
    return {"name": name, "conclusion": conclusion, "steps": steps}


class ComputeTests(unittest.TestCase):
    def execute(self, method="replenish", env=None, source=None, active=(), latest=10, ref="refs/heads/master",
                jobs=ACCEPTED, cwd=ROOT):
        harness = r"""
const input = JSON.parse(require('node:fs').readFileSync(0, 'utf8'));
for (const key of Object.keys(process.env)) if (key.startsWith('FUZZ_') || key === 'COMPUTE_MODE' || key === 'CI_RUNNER_LABELS') delete process.env[key];
Object.assign(process.env, input.env);
const calls = [], output = {}, info = [];
const core = {setOutput: (k,v) => output[k] = v, info: message => info.push(message)};
const context = {repo: {owner:'test',repo:'barn'}, ref:input.ref, runId:20,
 eventName: input.method === 'configure' ? 'workflow_dispatch' : 'workflow_run', payload: {workflow_run:input.source}};
const github = {paginate: async (method,args) => {calls.push(args); return args.run_id ? input.jobs : input.active;}, rest: {actions: {
 listJobsForWorkflowRun: async () => ({}),
 listWorkflowRuns: async args => {calls.push(args); return {data:{workflow_runs:[{id:input.latest}]}};},
 createWorkflowDispatch: async args => calls.push({dispatch:args})
}}};
Promise.resolve().then(() => require(input.module)[input.method]({github,context,core}))
 .then(() => console.log(JSON.stringify({calls,output,info})))
 .catch(error => console.log(JSON.stringify({calls,output,info,error:error.message})));
"""
        node = shutil.which("node")
        if node is None:
            node = str(Path(os.environ["RUNNER_TEMP"]).parents[1] / "externals/node24/bin/node")
        result = subprocess.run([node, "-e", harness], input=json.dumps({
            "method": method, "env": CONFIG if env is None else env,
            "source": SOURCE if source is None else source, "active": list(active),
            "latest": latest, "ref": ref, "jobs": jobs, "module": str(ROOT / "scripts/fuzz_compute.cjs")}),
            text=True, capture_output=True, check=True, cwd=cwd)
        return json.loads(result.stdout)

    def test_legacy_fallback_and_cap_are_preserved(self):
        result = self.execute("configure", {"COMPUTE_MODE": "legacy"})
        self.assertEqual(result["output"], {"runner": '["ubuntu-latest"]', "parallel": 2,
                                            "campaign": "native-and-toast"})
        result = self.execute("configure", {"COMPUTE_MODE": "legacy", "CI_RUNNER_LABELS": '["existing-wsl"]'})
        self.assertEqual(json.loads(result["output"]["runner"]), ["existing-wsl"])

    def test_dedicated_requires_explicit_policy_labels_and_master(self):
        env = {**CONFIG, "COMPUTE_MODE": "dedicated-once"}
        result = self.execute("configure", env)
        self.assertEqual(json.loads(result["output"]["runner"])["group"], "barn-bug-discovery")
        for key, value in [("FUZZ_RUNNER_LABELS", ""), ("FUZZ_RUNNER_LABELS", '["self-hosted","barn"]'),
                           ("FUZZ_RUNNER_POLICY_VERIFIED", "false"), ("FUZZ_MAX_PARALLEL", "8")]:
            with self.subTest(key=key, value=value):
                result = self.execute("configure", {**env, key: value})
                self.assertIn("error", result)
                self.assertNotIn("runner", result["output"])
        self.assertIn("error", self.execute("configure", env, ref="refs/heads/topic"))

    def test_campaign_selection_is_validated_before_routing(self):
        for selected in ("native-and-toast", "native-only"):
            result = self.execute("configure", {"COMPUTE_MODE": "legacy", "FUZZ_CAMPAIGN": selected})
            self.assertEqual(result["output"]["campaign"], selected)
        result = self.execute("configure", {"COMPUTE_MODE": "legacy", "FUZZ_CAMPAIGN": "unknown"})
        self.assertIn("error", result)
        self.assertEqual(result["output"], {})

    def test_controller_dispatches_one_successor_without_variable_api(self):
        result = self.execute()
        self.assertNotIn("error", result)
        dispatches = [call["dispatch"] for call in result["calls"] if "dispatch" in call]
        self.assertEqual(len(dispatches), 1)
        self.assertEqual(dispatches[0]["inputs"], {"compute-mode": "dedicated-continuous",
                                                  "campaign": "native-and-toast"})
        self.assertTrue(all("workflow_id" in call or "dispatch" in call or call.get("run_id") == SOURCE["id"]
                            for call in result["calls"]))

    def test_controller_preserves_native_only_including_after_a_finding(self):
        source = {**SOURCE, "display_title": "Nightly fuzz / dedicated-continuous / native-only"}
        for conclusion in ("success", "failure"):
            with self.subTest(conclusion=conclusion):
                result = self.execute(source={**source, "conclusion": conclusion})
                self.assertNotIn("error", result)
                dispatches = [call["dispatch"] for call in result["calls"] if "dispatch" in call]
                self.assertEqual([dispatch["inputs"] for dispatch in dispatches], [
                    {"compute-mode": "dedicated-continuous", "campaign": "native-only"}])
        for kwargs in ({"env": {**CONFIG, "FUZZ_CONTINUOUS_ENABLED": "false"}},
                       {"active": [{"id": 11}]}, {"latest": 11}, {"jobs": []}):
            result = self.execute(source=source, **kwargs)
            self.assertFalse(any("dispatch" in call for call in result["calls"]))

    def test_stop_switch_and_existing_or_newer_cycle_prevent_dispatch(self):
        results = [self.execute(env={**CONFIG, "FUZZ_CONTINUOUS_ENABLED": "false"}),
                   self.execute(active=[{"id": 11}]), self.execute(latest=11)]
        self.assertEqual(results[0]["calls"], [])
        for result in results:
            self.assertFalse(any("dispatch" in call for call in result["calls"]))

    def test_accepted_cycle_with_a_finding_replenishes(self):
        result = self.execute(source={**SOURCE, "conclusion": "failure"})
        self.assertNotIn("error", result)
        self.assertEqual(len([call for call in result["calls"] if "dispatch" in call]), 1)

    def test_unaccepted_cycle_stops_the_chain(self):
        for jobs in ([], [{"name": "Accept discovery cycle", "conclusion": "failure"}],
                     [{"name": "Accept discovery cycle", "conclusion": "skipped"}]):
            with self.subTest(jobs=jobs):
                result = self.execute(source={**SOURCE, "conclusion": "failure"}, jobs=jobs)
                self.assertIn("error", result)
                self.assertFalse(any("dispatch" in call for call in result["calls"]))

    def accept(self, jobs, lanes, outcome="success", campaign="native-and-toast"):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for target, census in lanes.items():
                lane = root / ".fuzz/retained/vm" / target
                lane.mkdir(parents=True)
                (lane / "census.json").write_text(json.dumps({"target": target, **census}))
            if outcome is not None:
                (root / "fuzz-differential").mkdir()
                (root / "fuzz-differential/results.json").write_text(json.dumps({"summary": {"outcome": outcome}}))
            return self.execute("accept", env={**CONFIG, "FUZZ_CAMPAIGN": campaign}, jobs=jobs, cwd=root)

    def test_native_only_accepts_measured_clean_work_and_saved_findings(self):
        for conclusion, failed, campaign in (("success", None, "clean"),
                                             ("failure", "Fuzz FuzzA", "finding")):
            with self.subTest(conclusion=conclusion):
                result = self.accept([job("FuzzA", conclusion, failed), job("ToastDifferential", "skipped")],
                                     {"FuzzA": {"completed_executions": 7, "campaign": campaign}},
                                     outcome=None, campaign="native-only")
                self.assertNotIn("error", result)
                report = json.loads(result["info"][-1])
                self.assertEqual(report["campaign"], "native-only")
                self.assertEqual(report["differential"], "not-run-native-only")

    def test_native_only_rejects_missing_work_evidence_and_unexpected_comparison(self):
        good = {"FuzzA": {"completed_executions": 7, "campaign": "clean"}}
        skipped = job("ToastDifferential", "skipped")
        cases = [([job("FuzzA"), skipped], {}, None),
                 ([job("FuzzA"), skipped], {"FuzzA": {"completed_executions": 0, "campaign": "clean"}}, None),
                 ([job("FuzzA", "failure", "Upload retained corpus and native evidence"), skipped], good, None),
                 ([job("FuzzA", "failure", "Open failure issues"), skipped], good, None),
                 ([job("FuzzA", "cancelled"), skipped], good, None),
                 ([job("FuzzA", "failure", "Fuzz FuzzA"), skipped],
                  {"FuzzA": {"completed_executions": 7, "campaign": "infrastructure"}}, None),
                 ([skipped], {}, None),
                 ([job("FuzzA")], good, None),
                 ([job("FuzzA"), job("ToastDifferential")], good, "success"),
                 ([job("FuzzA"), skipped], good, "success")]
        for jobs, lanes, outcome in cases:
            with self.subTest(jobs=jobs, lanes=lanes, outcome=outcome):
                self.assertIn("error", self.accept(jobs, lanes, outcome, campaign="native-only"))

    def test_cycle_with_findings_and_divergence_is_accepted(self):
        jobs = [job("Validate compute routing"), job("FuzzA"), job("FuzzB", "failure", "Fuzz FuzzB"),
                job("ToastDifferential", "failure", COMPARE), {"name": "Accept discovery cycle", "conclusion": None}]
        lanes = {"FuzzA": {"completed_executions": 5, "campaign": "clean"},
                 "FuzzB": {"completed_executions": 9, "campaign": "finding"}}
        self.assertNotIn("error", self.accept(jobs, lanes, "divergence"))
        self.assertNotIn("error", self.accept(jobs[:3] + [job("ToastDifferential")], lanes))

    def test_incomplete_or_unmeasured_cycle_is_rejected(self):
        good = {"completed_executions": 5, "campaign": "clean"}
        finding = {"completed_executions": 5, "campaign": "finding"}
        cases = {
            "infrastructure": ([job("FuzzA", "failure", "Fuzz FuzzA"), job("ToastDifferential")],
                               {"FuzzA": {"completed_executions": 5, "campaign": "infrastructure"}}, "success"),
            "zero work": ([job("FuzzA"), job("ToastDifferential")],
                          {"FuzzA": {"completed_executions": 0, "campaign": "clean"}}, "success"),
            "missing census": ([job("FuzzA"), job("ToastDifferential")], {}, "success"),
            "cancelled lane": ([job("FuzzA", "cancelled"), job("ToastDifferential")], {"FuzzA": good}, "success"),
            "failed upload": ([job("FuzzA", "failure", "Upload retained corpus and native evidence"),
                               job("ToastDifferential")], {"FuzzA": finding}, "success"),
            "failed reporter": ([{"name": "FuzzA", "conclusion": "failure", "steps": [
                                    {"name": "Fuzz FuzzA", "conclusion": "failure"},
                                    {"name": "Open failure issues", "conclusion": "failure"}]},
                                 job("ToastDifferential")], {"FuzzA": finding}, "success"),
            "no lanes": ([job("ToastDifferential")], {}, "success"),
            "inconclusive differential": ([job("FuzzA"), job("ToastDifferential", "failure", COMPARE)],
                                          {"FuzzA": good}, "inconclusive"),
            "differential setup": ([job("FuzzA"), job("ToastDifferential", "failure", "Build Toast oracle and Barn")],
                                   {"FuzzA": good}, None),
            "skipped differential": ([job("FuzzA"), job("ToastDifferential", "skipped")], {"FuzzA": good}, None),
        }
        for name, (jobs, lanes, outcome) in cases.items():
            with self.subTest(name):
                self.assertIn("error", self.accept(jobs, lanes, outcome))

    def test_controller_completion_failure_wrong_mode_and_foreign_repo_cannot_replenish(self):
        for field, value in [("event", "workflow_run"), ("conclusion", "cancelled"),
                             ("head_branch", "topic"), ("display_title", "Nightly fuzz / dedicated-once"),
                             ("display_title", "Nightly fuzz / dedicated-continuous / unknown"),
                             ("head_repository", {"full_name": "foreign/barn"})]:
            with self.subTest(field=field):
                result = self.execute(source={**SOURCE, field: value})
                self.assertIn("error", result)
                self.assertEqual(result["calls"], [])

    def test_fuzz_workflow_does_not_trigger_on_itself(self):
        # GitHub refuses to dispatch a workflow whose workflow_run trigger
        # names itself ("cannot listen to itself"); the controller is separate.
        fuzz = (ROOT / ".github/workflows/fuzz.yml").read_text()
        controller = (ROOT / ".github/workflows/fuzz-controller.yml").read_text()
        self.assertNotIn("workflow_run:", fuzz)
        self.assertIn("name: Nightly fuzz\n", fuzz)
        self.assertIn("workflows: [Nightly fuzz]", controller)
        self.assertNotIn("name: Nightly fuzz\n", controller)

    def test_fuzz_corpus_cache_paths_have_no_dot_segment(self):
        # Every matrix package starts with ./; actions/cache rejected
        # .fuzz/retained/./vm/... and saved nothing ("Cache save failed").
        fuzz = (ROOT / ".github/workflows/fuzz.yml").read_text()
        self.assertIn("package: ./vm,", fuzz)
        self.assertNotIn(".fuzz/retained/${{ matrix.package }}", fuzz)
        self.assertEqual(fuzz.count(".fuzz/retained/**/${{ matrix.target }}/inputs"), 2)

    def test_differential_job_name_is_exactly_its_target(self):
        # Run 42424236943 named the job "ToastDifferential (./scripts,
        # ToastDifferential)", so accept counted it as a lane and missed it.
        fuzz = (ROOT / ".github/workflows/fuzz.yml").read_text()
        differential = fuzz.split("\n  differential:\n")[1].split("\n  accept:\n")[0]
        self.assertIn("\n    name: ToastDifferential\n", differential)
        self.assertNotIn("\n    strategy:", differential)
        self.assertIn("FUZZ_TARGET: ToastDifferential", differential)
        self.assertIn("FUZZ_PACKAGE: ./scripts", differential)

    def test_changed_dedicated_configuration_prevents_replenishment(self):
        result = self.execute(env={**CONFIG, "FUZZ_RUNNER_LABELS": ""})
        self.assertIn("error", result)
        self.assertEqual(result["calls"], [])


if __name__ == "__main__":
    unittest.main()
