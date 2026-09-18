# /// script
# requires-python = ">=3.10"
# dependencies = ["mcp==1.26.0"]
# ///
"""Pi adapter transport. Each server keeps one official MCP client session open."""
import asyncio
from contextlib import AsyncExitStack
import json
import os
import re
import sys

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client


def emit(message):
    print(json.dumps(message), flush=True)


async def main():
    pending = {}
    async with AsyncExitStack() as stack:
        sessions, tools, names = {}, [], set()
        for name in sys.argv[1:]:
            streams = await stack.enter_async_context(stdio_client(StdioServerParameters(
                command="ilk", args=["mcp", "run", name], env=dict(os.environ))))
            session = await stack.enter_async_context(ClientSession(*streams))
            await session.initialize()
            sessions[name] = session
            cursor = None
            while True:
                page = await session.list_tools(cursor=cursor)
                for tool in page.tools:
                    alias = "ilk_" + re.sub(r"[^a-zA-Z0-9_]", "_", name) + "__" + tool.name
                    if alias in names:
                        raise ValueError("MCP tool name collision: " + alias)
                    names.add(alias)
                    tools.append({"name": alias, "server": name, "tool": tool.name,
                                  "description": tool.description or tool.name,
                                  "parameters": tool.inputSchema})
                cursor = page.nextCursor
                if not cursor:
                    break
        emit({"id": 0, "result": tools})

        async def call(request):
            try:
                result = await sessions[request["server"]].call_tool(request["tool"], request.get("arguments", {}))
                emit({"id": request["id"], "result": result.model_dump(mode="json", by_alias=True)})
            except asyncio.CancelledError:
                emit({"id": request["id"], "error": "MCP call cancelled"})
            except Exception as exc:
                emit({"id": request["id"], "error": str(exc)})
            finally:
                pending.pop(request["id"], None)

        try:
            while line := await asyncio.to_thread(sys.stdin.readline):
                request = json.loads(line)
                if "cancel" in request:
                    task = pending.get(request["cancel"])
                    if task:
                        task.cancel()
                else:
                    pending[request["id"]] = asyncio.create_task(call(request))
        finally:
            tasks = list(pending.values())
            for task in tasks:
                task.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)


if __name__ == "__main__":
    asyncio.run(main())
