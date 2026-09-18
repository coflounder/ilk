"""Use the actual installed layer, generated hooks and CLI in disposable repositories."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from test_store import summary

LAYER = Path(__file__).resolve().parents[1]


class InstalledTests(unittest.TestCase):
    def test_hook_publication_worktree_retrieval_and_removal(self):
        binary = Path(os.environ["ILK_TEST_BIN"])
        env = {**os.environ, "PATH": str(binary.parent) + os.pathsep + os.environ["PATH"]}
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "project"
            root.mkdir()

            def run(*args, data=None, expected=0, cwd=root):
                result = subprocess.run(args, cwd=cwd, env=env, input=data,
                                        text=True, capture_output=True)
                self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
                return result.stdout

            run("git", "init", "-q")
            run("git", "config", "user.email", "test@example.com")
            run("git", "config", "user.name", "Test")
            run("git", "commit", "--allow-empty", "-qm", "initial")
            run(str(binary), "init", "-y", "--test-command", "true")
            run(str(binary), "add", str(LAYER), "--allow-exec", "-y")
            run(str(binary), "check", "--only", "sessions.runtime")
            settings = json.loads((root / ".claude/settings.json").read_text())
            self.assertIn("Stop", settings["hooks"])
            config = json.loads((root / ".mcp.json").read_text())
            self.assertEqual(config["mcpServers"]["session-summaries"]["args"], ["mcp", "run", "session-summaries"])
            payload = {"session_id": "real-hook", "stop_hook_active": False, "cwd": str(root),
                       "transcript_path": "/native/reference-only", "last_assistant_message": "Done"}
            # Session-start coexists with record's brief; turn-end receives native JSON.
            start = run(str(binary), "hook", "run", "session-start", data=json.dumps(payload))
            self.assertIn("Shared session summaries", start)
            run(str(binary), "hook", "run", "turn-end", data=json.dumps(payload), expected=2)
            state = json.loads(run(str(binary), "session-summaries", "read", "claude-code", "real-hook"))
            save = {"harness": "claude-code", "session_id": "real-hook", "expected_revision": 0,
                    "checkpoint_id": state["checkpoint"]["id"], "summary": summary("Installed flow")}
            run(str(binary), "session-summaries", "save", "-", data=json.dumps(save))
            payload["stop_hook_active"] = True
            run(str(binary), "hook", "run", "turn-end", data=json.dumps(payload))
            published = json.loads(run(str(binary), "session-summaries", "read", "claude-code", "real-hook"))
            self.assertEqual(published["revision"], 1)
            self.assertEqual(published["checkpoint"]["state"], "published")
            run("git", "add", "-A")
            run("git", "commit", "-qm", "adopt layer")
            linked = Path(tmp) / "linked"
            run("git", "worktree", "add", "-qb", "other-task", str(linked))
            found = json.loads(run(str(binary), "session-summaries", "list", "--query", "Installed", cwd=linked))
            self.assertEqual(len(found["summaries"]), 1)
            self.assertEqual(found["summaries"][0]["worktree"], str(root))
            run(str(binary), "rm", "session-summaries", "-y")
            self.assertNotIn("Stop", json.loads((root / ".claude/settings.json").read_text())["hooks"])
            self.assertFalse((root / ".ilk/bin/session-summaries.py").exists())
            # Removing integrations never deletes the local publications.
            found = json.loads(run(str(binary), "session-summaries", "list", cwd=linked))
            self.assertEqual(len(found["summaries"]), 1)


if __name__ == "__main__":
    unittest.main()
