import concurrent.futures
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "bin/session-summaries.py"
spec = importlib.util.spec_from_file_location("summaries", SCRIPT)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def repo(path):
    path.mkdir()
    for args in (["init", "-q"], ["config", "user.email", "test@example.com"],
                 ["config", "user.name", "Test"], ["commit", "--allow-empty", "-qm", "initial"]):
        subprocess.run(["git", "-C", str(path), *args], check=True)
    return module.Store(path)


def summary(task="Resume login work"):
    return {field: task if field == "task" else "Not run" if field == "verification" else "None"
            for field in module.FIELDS}


class StoreTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.store = repo(self.root / "repo with spaces")

    def payload(self, active=False):
        return {"session_id": "native-session", "cwd": str(self.store.root),
                "stop_hook_active": active, "transcript_path": "/native/session.jsonl",
                "last_assistant_message": "Implemented the login fix."}

    def test_linked_worktree_shares_store_and_other_clone_does_not(self):
        worktree = self.root / "another worktree"
        subprocess.run(["git", "-C", str(self.store.root), "worktree", "add", "-qb", "task", str(worktree)], check=True)
        other = module.Store(worktree)
        self.assertEqual(self.store.database, other.database)
        self.store.save("pi", "one", 0, summary())
        self.assertEqual(other.read("pi", "one")["published"]["worktree"], str(self.store.root))
        isolated = repo(self.root / "other-repo")
        self.assertEqual(isolated.list()["summaries"], [])
        self.assertFalse((self.store.root / ".ilk").exists())

    def test_checkpoint_stages_then_hook_publishes(self):
        code, reason = self.store.checkpoint(self.payload())
        self.assertEqual(code, 2)
        self.assertIn("session_summary_save", reason)
        pending = self.store.read("claude-code", "native-session")
        self.assertIsNone(pending["published"])
        token = pending["checkpoint"]["id"]
        saved = self.store.save("claude-code", "native-session", 0, summary(), token)
        self.assertEqual(saved["status"], "staged")
        self.assertIsNone(self.store.read("claude-code", "native-session")["published"])
        self.assertEqual(self.store.checkpoint(self.payload(True))[0], 0)
        result = self.store.read("claude-code", "native-session")
        self.assertEqual(result["revision"], 1)
        self.assertEqual(result["published"]["source"]["checkpoint_id"], token)
        self.assertEqual(result["checkpoint"]["state"], "published")
        # Repeated Stop continuations neither republish nor restart the loop.
        self.assertEqual(self.store.checkpoint(self.payload(True))[0], 0)
        self.assertEqual(self.store.read("claude-code", "native-session")["revision"], 1)

    def test_missed_checkpoint_retains_last_publication(self):
        self.store.save("claude-code", "native-session", 0, summary("old"))
        self.store.checkpoint(self.payload())
        self.assertEqual(self.store.checkpoint(self.payload(True))[0], 0)
        result = self.store.read("claude-code", "native-session")
        self.assertEqual(result["published"]["task"], "old")
        self.assertEqual(result["checkpoint"]["state"], "missed")
        self.assertEqual(len(self.store.list()["incomplete_checkpoints"]), 1)
        self.store.save("claude-code", "native-session", 1, summary("recovered"))
        self.assertEqual(self.store.list()["incomplete_checkpoints"], [])

    def test_interruption_keeps_draft_unpublished_and_new_turn_invalidates_token(self):
        self.store.checkpoint(self.payload())
        token = self.store.read("claude-code", "native-session")["checkpoint"]["id"]
        self.store.save("claude-code", "native-session", 0, summary(), token)
        reopened = module.Store(self.store.root)
        self.assertEqual(reopened.read("claude-code", "native-session")["checkpoint"]["state"], "staged")
        reopened.checkpoint(self.payload())
        with self.assertRaisesRegex(ValueError, "no longer pending"):
            reopened.save("claude-code", "native-session", 0, summary(), token)
        self.assertIsNone(reopened.read("claude-code", "native-session")["published"])

    def test_revision_conflict_and_harness_identity(self):
        self.store.save("pi", "same", 0, summary("Pi"))
        self.store.save("claude-code", "same", 0, summary("Claude"))
        with self.assertRaisesRegex(ValueError, "Summary changed"):
            self.store.save("pi", "same", 0, summary("stale"))
        self.assertEqual(self.store.read("pi", "same")["published"]["task"], "Pi")
        self.assertEqual(len(self.store.list()["summaries"]), 2)

    def test_concurrent_writers_cannot_silently_overwrite(self):
        def write(n):
            try:
                return self.store.save("pi", "same", 0, summary(str(n)))["status"]
            except ValueError:
                return "conflict"
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(write, range(2)))
        self.assertEqual(sorted(results), ["conflict", "published"])

    def test_invalid_summary_and_checkpoint_cannot_replace_good_record(self):
        self.store.save("claude-code", "native-session", 0, summary("old"))
        self.store.checkpoint(self.payload())
        with self.assertRaisesRegex(ValueError, "checkpoint is pending"):
            self.store.save("claude-code", "native-session", 1, summary())
        token = self.store.read("claude-code", "native-session")["checkpoint"]["id"]
        for invalid in ({}, {**summary(), "verification": ""}, {**summary(), "completed": "x" * 40000}):
            with self.assertRaises(ValueError):
                self.store.save("claude-code", "native-session", 1, invalid, token)
        self.assertEqual(self.store.read("claude-code", "native-session")["published"]["task"], "old")
        self.assertEqual(self.store.read("claude-code", "native-session")["checkpoint"]["state"], "pending")

    def test_search_is_literal_and_bounded(self):
        self.store.save("pi", "one", 0, summary("login ' OR 1=1 --"))
        self.store.save("pi", "two", 0, summary("checkout"))
        self.assertEqual(len(self.store.list("' OR 1=1 --")["summaries"]), 1)
        self.assertEqual(self.store.list("%")["summaries"], [])
        for limit in (0, 101, True):
            with self.assertRaises(ValueError):
                self.store.list(limit=limit)

    def test_cli_publish_and_context(self):
        payload = {"harness": "other", "session_id": "direct", "expected_revision": 0, "summary": summary()}
        saved = subprocess.run(["python3", str(SCRIPT), "save", "-"], input=json.dumps(payload),
                               text=True, cwd=self.store.root, capture_output=True, check=True)
        self.assertEqual(json.loads(saved.stdout)["status"], "published")
        context = subprocess.run(["python3", str(SCRIPT), "context"], text=True,
                                 cwd=self.store.root, capture_output=True, check=True)
        self.assertIn("Resume login work", context.stdout)
        self.assertNotIn('"decisions"', context.stdout)

    def test_other_stop_hook_continuation_still_requests_first_checkpoint(self):
        self.assertEqual(self.store.checkpoint(self.payload(True))[0], 2)
        self.assertEqual(self.store.checkpoint(self.payload(True))[0], 0)

    def test_checkpoint_cannot_be_staged_or_published_from_another_worktree(self):
        worktree = self.root / "second"
        subprocess.run(["git", "-C", str(self.store.root), "worktree", "add", "-qb", "second", str(worktree)], check=True)
        other = module.Store(worktree)
        self.store.checkpoint(self.payload())
        token = self.store.read("claude-code", "native-session")["checkpoint"]["id"]
        with self.assertRaisesRegex(ValueError, "another worktree"):
            other.save("claude-code", "native-session", 0, summary(), token)
        self.store.save("claude-code", "native-session", 0, summary(), token)
        with self.assertRaisesRegex(ValueError, "another worktree"):
            other.checkpoint({**self.payload(True), "cwd": str(worktree)})
        self.assertEqual(self.store.read("claude-code", "native-session")["checkpoint"]["state"], "staged")

    def test_prerequisite_check_names_missing_uv(self):
        checked = subprocess.run([sys.executable, str(SCRIPT), "doctor"], cwd=self.store.root,
                                 env={**os.environ, "PATH": "/no-tools"}, text=True, capture_output=True)
        self.assertEqual(checked.returncode, 1)
        self.assertIn("uv", checked.stderr)
        self.assertFalse(self.store.database.exists())


if __name__ == "__main__":
    unittest.main()
