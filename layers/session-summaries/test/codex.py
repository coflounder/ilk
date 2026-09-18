"""Exercise installed Codex config parsing and its documented native hook payload."""
import asyncio
import json
import os
from pathlib import Path
import subprocess
import tempfile

binary = os.environ["ILK_TEST_BIN"]
codex = os.environ["ILK_TEST_CODEX_BIN"]
layer = Path(__file__).resolve().parents[1]
env = {**os.environ, "PATH": str(Path(binary).parent) + os.pathsep + os.environ["PATH"]}

with tempfile.TemporaryDirectory() as tmp:
    root = Path(tmp)

    def run(*args, data=None, code=0):
        result = subprocess.run(args, cwd=root, env=env, input=data, text=True, capture_output=True)
        assert result.returncode == code, result.stdout + result.stderr
        return result.stdout

    run("git", "init", "-q")
    run(binary, "init", "-y", "--test-command", "true")
    run(binary, "add", str(layer), "--allow-exec", "-y")
    (root / ".codex").mkdir()
    original_config = '# User setting and comment\nweb_search = "disabled"\n'
    original_hooks = {"description": "user hook", "hooks": {"SessionEnd": [{"hooks": [{"type": "command", "command": "true"}]}]}}
    (root / ".codex/config.toml").write_text(original_config)
    (root / ".codex/hooks.json").write_text(json.dumps(original_hooks))
    run(binary, "agents", "add", "codex", "-y")
    # Use Codex's config-directory setting for this child process only. The
    # developer's real config and hook trust remain untouched.
    test_codex_dir = root / "test-codex-config"
    test_codex_dir.mkdir()
    (test_codex_dir / "config.toml").write_text(f'[projects.{json.dumps(str(root))}]\ntrust_level="trusted"\n')
    harness_environment = {**env, "CODEX_HOME": str(test_codex_dir)}
    async def inspect_native_config():
        # ConfigRead includes project layers; `codex mcp get` manages user-level
        # config only. No model turn or external provider call is started here.
        process = await asyncio.create_subprocess_exec(codex, "app-server", "--stdio",
            cwd=root, env=harness_environment, stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.DEVNULL)
        async def request(number, method, params):
            process.stdin.write((json.dumps({"id": number, "method": method, "params": params}) + "\n").encode())
            await process.stdin.drain()
            while True:
                line = await asyncio.wait_for(process.stdout.readline(), timeout=30)
                assert line, "Codex app-server closed before replying"
                response = json.loads(line)
                if response.get("id") == number:
                    assert "error" not in response, response.get("error")
                    return response["result"]
        try:
            await request(1, "initialize", {"clientInfo": {"name": "ilk-adapter-test", "version": "1.0"},
                                           "capabilities": {"experimentalApi": True}})
            process.stdin.write(b'{"method":"initialized"}\n')
            config = await request(2, "config/read", {"cwd": str(root), "includeLayers": True})
            server = config["config"]["mcp_servers"]["session-summaries"]
            assert server["command"] == "ilk"
            assert server["args"] == ["mcp", "run", "session-summaries"]
            discovered = await request(3, "hooks/list", {"cwds": [str(root)]})
            assert "ilk hook run turn-end --target codex" in json.dumps(discovered)
            assert "ilk hook run session-start --target codex" in json.dumps(discovered)
        finally:
            process.stdin.close()
            try:
                await asyncio.wait_for(process.wait(), timeout=5)
            except asyncio.TimeoutError:
                process.kill()
                await process.wait()
    asyncio.run(inspect_native_config())
    hooks = json.loads((root / ".codex/hooks.json").read_text())["hooks"]
    assert set(hooks) >= {"SessionStart", "Stop"}
    payload = {"session_id": "codex-fixture", "cwd": str(root), "hook_event_name": "Stop",
               "turn_id": "turn-1", "stop_hook_active": False, "transcript_path": None,
               "last_assistant_message": None}
    stop = hooks["Stop"][0]["hooks"][0]["command"]
    run("sh", "-c", stop, data=json.dumps(payload), code=2)
    record = json.loads(run(binary, "session-summaries", "read", "codex", "codex-fixture"))
    fields = ("task", "objective", "constraints", "decisions", "completed", "remaining", "verification", "questions", "next_actions")
    draft = {"harness": "codex", "session_id": "codex-fixture", "expected_revision": 0,
             "checkpoint_id": record["checkpoint"]["id"], "summary": {k: "None" for k in fields}}
    run(binary, "session-summaries", "save", "-", data=json.dumps(draft))
    payload["stop_hook_active"] = True
    run("sh", "-c", stop, data=json.dumps(payload))
    published = json.loads(run(binary, "session-summaries", "read", "codex", "codex-fixture"))
    assert published["revision"] == 1
    assert published["published"]["harness"] == "codex"
    # Another harness reads the same publication through the project CLI.
    assert len(json.loads(run(binary, "session-summaries", "list"))["summaries"]) == 1
    run(binary, "agents", "remove", "codex", "-y")
    assert (root / ".codex/config.toml").read_text() == original_config
    assert json.loads((root / ".codex/hooks.json").read_text()) == original_hooks
print("Codex: real Codex discovers project MCP config and native hooks; native hook fixtures publish under Codex identity; removal passed")
