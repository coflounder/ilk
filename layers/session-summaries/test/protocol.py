# /// script
# requires-python = ">=3.10"
# dependencies = ["mcp==1.26.0"]
# ///
"""Exercise real stdio framing, tool discovery, persistence and tool errors."""
import asyncio
import json
import os
from pathlib import Path
import subprocess
import tempfile

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

SCRIPT = Path(__file__).resolve().parents[1] / "bin/session-summaries.py"


def result(reply):
    assert not reply.isError, reply
    return json.loads(reply.content[0].text)


async def main():
    with tempfile.TemporaryDirectory() as tmp:
        subprocess.run(["git", "init", "-q", tmp], check=True)
        binary = os.environ["ILK_TEST_BIN"]
        for args in ([binary, "init", "-y", "--test-command", "true"],
                     [binary, "add", str(SCRIPT.parents[1]), "--allow-exec", "-y"]):
            subprocess.run(args, cwd=tmp, check=True, capture_output=True)
        params = StdioServerParameters(command=binary, args=["mcp", "run", "session-summaries"], cwd=tmp)
        async with stdio_client(params) as (read, write):
            async with ClientSession(read, write) as client:
                await client.initialize()
                tools = await client.list_tools()
                assert {t.name for t in tools.tools} == {"session_summaries_list", "session_summary_read", "session_summary_save"}
                empty = result(await client.call_tool("session_summaries_list", {}))
                assert empty["summaries"] == []
                fields = ("task", "objective", "constraints", "decisions", "completed", "remaining", "verification", "questions", "next_actions")
                payload = {"harness": "direct-test", "session_id": "one", "expected_revision": 0,
                           "summary": {k: "Protocol test" if k == "task" else "None" for k in fields}}
                assert result(await client.call_tool("session_summary_save", payload))["status"] == "published"
                record = result(await client.call_tool("session_summary_read", {"harness": "direct-test", "session_id": "one"}))
                assert record["revision"] == 1
                assert (await client.call_tool("session_summary_save", payload)).isError
                hook = {"session_id": "checkpoint", "cwd": tmp, "stop_hook_active": False,
                        "last_assistant_message": "Ready to publish"}
                first = subprocess.run([binary, "hook", "run", "turn-end"], cwd=tmp,
                                       input=json.dumps(hook), text=True, capture_output=True)
                assert first.returncode == 2, first.stderr
                state = result(await client.call_tool("session_summary_read", {"harness": "claude-code", "session_id": "checkpoint"}))
                draft = {**payload, "harness": "claude-code", "session_id": "checkpoint",
                         "checkpoint_id": state["checkpoint"]["id"]}
                assert result(await client.call_tool("session_summary_save", draft))["status"] == "staged"
                hook["stop_hook_active"] = True
                subprocess.run([binary, "hook", "run", "turn-end"], cwd=tmp,
                               input=json.dumps(hook), text=True, capture_output=True, check=True)
                completed = result(await client.call_tool("session_summary_read", {"harness": "claude-code", "session_id": "checkpoint"}))
                assert completed["checkpoint"]["state"] == "published"
        # A separately invoked harness process sees the same publication.
        async with stdio_client(params) as (read, write):
            async with ClientSession(read, write) as client:
                await client.initialize()
                entries = result(await client.call_tool("session_summaries_list", {"query": "Protocol"}))
                assert len(entries["summaries"]) == 2
    print("Installed MCP stdio: discovery, explicit publication, hook checkpoint, conflict and independent-session retrieval passed")


asyncio.run(main())
