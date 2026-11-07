"""Offline execution of the workflow's real routing and replenishment owner."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import unittest


ROOT = Path(__file__).resolve().parents[1]
CONFIG = {"FUZZ_RUNNER_LABELS": '["self-hosted","Linux","X64","barn-fuzz"]',
          "FUZZ_RUNNER_POLICY_VERIFIED": "true", "FUZZ_MAX_PARALLEL": "2",
          "FUZZ_CONTINUOUS_ENABLED": "true"}
SOURCE = {"id": 10, "event": "workflow_dispatch", "head_branch": "master",
          "path": ".github/workflows/fuzz.yml", "conclusion": "success",
          "display_title": "Nightly fuzz / dedicated-continuous",
          "head_repository": {"full_name": "test/barn"}}


class ComputeTests(unittest.TestCase):
    def execute(self, method="replenish", env=None, source=None, active=(), latest=10, ref="refs/heads/master"):
        harness = r"""
const input = JSON.parse(require('node:fs').readFileSync(0, 'utf8'));
for (const key of Object.keys(process.env)) if (key.startsWith('FUZZ_') || key === 'COMPUTE_MODE' || key === 'CI_RUNNER_LABELS') delete process.env[key];
Object.assign(process.env, input.env);
const calls = [], output = {};
const core = {setOutput: (k,v) => output[k] = v, info: () => {}};
const context = {repo: {owner:'test',repo:'barn'}, ref:input.ref, runId:20,
 eventName: input.method === 'configure' ? 'workflow_dispatch' : 'workflow_run', payload: {workflow_run:input.source}};
const github = {paginate: async (method,args) => {calls.push(args); return input.active;}, rest: {actions: {
 listWorkflowRuns: async args => {calls.push(args); return {data:{workflow_runs:[{id:input.latest}]}};},
 createWorkflowDispatch: async args => calls.push({dispatch:args})
}}};
Promise.resolve().then(() => require('./scripts/fuzz_compute.cjs')[input.method]({github,context,core}))
 .then(() => console.log(JSON.stringify({calls,output})))
 .catch(error => console.log(JSON.stringify({calls,output,error:error.message})));
"""
        node = shutil.which("node")
        if node is None:
            node = str(Path(os.environ["RUNNER_TEMP"]).parents[1] / "externals/node24/bin/node")
        result = subprocess.run([node, "-e", harness], input=json.dumps({
            "method": method, "env": CONFIG if env is None else env,
            "source": SOURCE if source is None else source, "active": list(active),
            "latest": latest, "ref": ref}), text=True, capture_output=True, check=True, cwd=ROOT)
        return json.loads(result.stdout)

    def test_legacy_fallback_and_cap_are_preserved(self):
        result = self.execute("configure", {"COMPUTE_MODE": "legacy"})
        self.assertEqual(result["output"], {"runner": '["ubuntu-latest"]', "parallel": 2})
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

    def test_controller_dispatches_one_successor_without_variable_api(self):
        result = self.execute()
        self.assertNotIn("error", result)
        dispatches = [call["dispatch"] for call in result["calls"] if "dispatch" in call]
        self.assertEqual(len(dispatches), 1)
        self.assertEqual(dispatches[0]["inputs"], {"compute-mode": "dedicated-continuous"})
        self.assertTrue(all("workflow_id" in call or "dispatch" in call for call in result["calls"]))

    def test_stop_switch_and_existing_or_newer_cycle_prevent_dispatch(self):
        results = [self.execute(env={**CONFIG, "FUZZ_CONTINUOUS_ENABLED": "false"}),
                   self.execute(active=[{"id": 11}]), self.execute(latest=11)]
        self.assertEqual(results[0]["calls"], [])
        for result in results:
            self.assertFalse(any("dispatch" in call for call in result["calls"]))

    def test_controller_completion_failure_wrong_mode_and_foreign_repo_cannot_replenish(self):
        for field, value in [("event", "workflow_run"), ("conclusion", "failure"),
                             ("head_branch", "topic"), ("display_title", "Nightly fuzz / dedicated-once"),
                             ("head_repository", {"full_name": "foreign/barn"})]:
            with self.subTest(field=field):
                result = self.execute(source={**SOURCE, field: value})
                self.assertIn("error", result)
                self.assertEqual(result["calls"], [])

    def test_changed_dedicated_configuration_prevents_replenishment(self):
        result = self.execute(env={**CONFIG, "FUZZ_RUNNER_LABELS": ""})
        self.assertIn("error", result)
        self.assertEqual(result["calls"], [])


if __name__ == "__main__":
    unittest.main()
