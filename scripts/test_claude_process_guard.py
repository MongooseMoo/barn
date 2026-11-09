import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
GUARD = ROOT / "scripts" / "claude_process_guard.py"


class ProcessGuardTests(unittest.TestCase):
    def invoke(self, event):
        return subprocess.run(
            [sys.executable, str(GUARD)], input=json.dumps(event),
            text=True, capture_output=True, check=False,
        )

    def test_blocks_reported_compound_call(self):
        # Exact command from the #606 session's 2026-11-05T11:10:18.382Z
        # Bash call. Treat this only as input data; never execute it.
        result = self.invoke({"tool_name": "Bash", "tool_input": {
            "command": "pkill -f 'vm.test' ; cat > vm/zz606_test.go <<'EOF'\n"
                       'package vm\nimport ("testing";"github.com/MongooseMoo/barn/bytecode";'
                       '"github.com/MongooseMoo/barn/task";dbstore "github.com/MongooseMoo/barn/db/store")\n'
                       'func TestZZ606(t *testing.T){\n'
                       ' for _, code := range [][]byte{{byte(bytecode.OP_LOOP),0,3,byte(bytecode.OP_RETURN_NONE)}} {\n'
                       '  s:=validRestoredSnapshot(); s.Frames[0].Program.Code=code\n'
                       '  c:=task.NewContext(); c.TicksRemaining=100\n'
                       '  m,err:=RestoreVMSnapshot(s,dbstore.NewStore(),nil,c); t.Log("restore err",err); if err!=nil{continue}\n'
                       '  m.Task=task.NewState(1,c.Player,100,1); m.TickLimit=100\n'
                       '  t.Log(m.Resume())\n }}\nEOF\n'
                       'timeout 20 go test ./vm -run TestZZ606 -count=1 -v 2>&1 | tail -8'
        }})
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("Process-name kills are forbidden", result.stderr)

    def test_blocks_names_through_compound_and_nested_commands(self):
        for command in (
            "true; pkill -f vm.test", "true && /usr/bin/killall vm.test",
            "bash -lc 'pkill -f vm.test'", "p'kill' -f vm.test",
            '"pkill" -f vm.test', "p\\kill -f vm.test",
            "p\\\nkill -f vm.test", "killall -9 vm.test",
            # Text references are conservatively denied too: use Read.
            "echo 'pkill -f vm.test'",
        ):
            with self.subTest(command=command):
                result = self.invoke({"tool_name": "Bash", "tool_input": {"command": command}})
                self.assertEqual(result.returncode, 2, result.stderr)

    def test_leaves_normal_permission_flow_and_owned_pid_cleanup_available(self):
        for command in (
            "go test ./vm -run '^TestRestore$'", "timeout 20 go test ./vm -run TestHang",
            'sleep 10 & owned_pid=$!; kill "$owned_pid"; wait "$owned_pid"',
            "gh issue view 606 --comments", "rg 'signal: terminated' run.log",
        ):
            with self.subTest(command=command):
                result = self.invoke({"tool_name": "Bash", "tool_input": {"command": command}})
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, "")

    def test_invalid_input_blocks(self):
        for event in (None, {}, {"tool_name": "Bash", "tool_input": {"command": 42}}):
            with self.subTest(event=event):
                self.assertEqual(self.invoke(event).returncode, 2)

    @unittest.skipUnless(os.name == "posix", "the workflow hook runs in a Linux shell")
    def test_configured_hook_blocks_execution_and_missing_script(self):
        settings = json.loads((ROOT / ".github" / "claude-fix-settings.json").read_text())
        group, = settings["hooks"]["PreToolUse"]
        self.assertEqual(group["matcher"], "Bash")
        hook, = group["hooks"]
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory, "executed")
            for workspace in (str(ROOT), directory):
                # Model the documented contract: only execute the tool call
                # if the hook succeeds. The tool text itself is harmless.
                result = subprocess.run(
                    ["bash", "-c", hook["command"] + ' && touch "$GUARD_MARKER"'],
                    input=json.dumps({"tool_name": "Bash", "tool_input": {"command": "echo pkill"}}),
                    env={**os.environ, "GITHUB_WORKSPACE": workspace, "GUARD_MARKER": str(marker)},
                    text=True, capture_output=True, check=False,
                )
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertFalse(marker.exists())


if __name__ == "__main__":
    unittest.main()
