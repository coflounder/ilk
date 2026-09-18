// Managed by ilk. Lifecycle hooks and MCP clients belong to this Pi session.
import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { dirname, join } from "node:path";
import { createInterface } from "node:readline";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

const config: { servers: string[]; events: string[] } = {"events":["session-start","turn-end"],"servers":["session-summaries"]};
type RemoteTool = { name: string; server: string; tool: string; description: string; parameters: any };
type Reply = { content?: any[]; isError?: boolean; [key: string]: unknown };

class MCPBridge {
  private child: ChildProcessWithoutNullStreams;
  private next = 1;
  private closed = false;
  private errors = "";
  private pending = new Map<number, { resolve: (result: any) => void; reject: (error: Error) => void }>();
  readonly ready: Promise<RemoteTool[]>;

  constructor(cwd: string) {
    this.ready = new Promise((resolve, reject) => this.pending.set(0, { resolve, reject }));
    this.child = spawn("uv", ["run", "--script", join(dirname(fileURLToPath(import.meta.url)), "mcp-client.py"), ...config.servers], { cwd, stdio: "pipe" });
    this.child.stderr.on("data", (chunk) => { this.errors = (this.errors + chunk).slice(-8000); });
    const lines = createInterface({ input: this.child.stdout });
    lines.on("line", (line) => {
      try {
        const message = JSON.parse(line);
        const request = this.pending.get(message.id);
        if (!request) return;
        this.pending.delete(message.id);
        if (message.error) request.reject(new Error(message.error));
        else request.resolve(message.result);
      } catch (error) { this.fail(new Error(`Invalid MCP bridge output: ${error}`)); }
    });
    this.child.on("error", (error) => this.fail(error));
    this.child.stdin.on("error", (error) => this.fail(error));
    this.child.on("exit", (code) => {
      lines.close();
      this.fail(new Error(`MCP bridge exited (${code}): ${this.errors}`));
    });
    const timer = setTimeout(() => { this.fail(new Error("MCP bridge startup timed out")); void this.close(); }, 60000);
    this.ready.then(() => clearTimeout(timer), () => clearTimeout(timer));
  }

  private fail(error: Error) {
    this.closed = true;
    for (const request of this.pending.values()) request.reject(error);
    this.pending.clear();
  }

  async call(tool: RemoteTool, args: unknown, signal?: AbortSignal): Promise<Reply> {
    if (this.closed) throw new Error("MCP connection is closed; reload the Pi extension");
    if (signal?.aborted) throw new Error("MCP call cancelled");
    const id = this.next++;
    let abort: () => void = () => {};
    const promise = new Promise<Reply>((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      abort = () => {
        this.child.stdin.write(JSON.stringify({ cancel: id }) + "\n");
        this.pending.delete(id);
        reject(new Error("MCP call cancelled"));
      };
      signal?.addEventListener("abort", abort, { once: true });
      this.child.stdin.write(JSON.stringify({ id, server: tool.server, tool: tool.tool, arguments: args }) + "\n");
    });
    const timeout = setTimeout(abort, 60000);
    try { return await promise; }
    finally { clearTimeout(timeout); signal?.removeEventListener("abort", abort); }
  }

  async close() {
    this.fail(new Error("Pi session closed"));
    if (this.child.exitCode !== null || this.child.signalCode !== null) return;
    await new Promise<void>((resolve) => {
      const timeout = setTimeout(() => { this.child.kill("SIGTERM"); resolve(); }, 5000);
      this.child.once("exit", () => { clearTimeout(timeout); resolve(); });
      this.child.stdin.end();
    });
  }
}

async function hook(event: string, ctx: ExtensionContext, continuing: boolean, message: string) {
  return await new Promise<{ code: number; stdout: string; stderr: string }>((resolve, reject) => {
    const child = spawn("ilk", ["hook", "run", event, "--target", "pi"], { cwd: ctx.cwd, stdio: "pipe" });
    let stdout = "", stderr = "";
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    const timeout = setTimeout(() => child.kill("SIGTERM"), 30000);
    child.on("error", (error) => { clearTimeout(timeout); reject(error); });
    child.stdin.on("error", (error) => { clearTimeout(timeout); reject(error); });
    child.on("close", (code) => { clearTimeout(timeout); resolve({ code: code ?? 1, stdout, stderr }); });
    child.stdin.end(JSON.stringify({ session_id: ctx.sessionManager.getSessionId(),
      transcript_path: ctx.sessionManager.getSessionFile() ?? "", cwd: ctx.cwd,
      stop_hook_active: continuing, last_assistant_message: message }));
  });
}

export default function (pi: ExtensionAPI) {
  let bridge: MCPBridge | undefined;
  let continuing = false;
  let lastMessage = "";
  let failed = false;

  pi.on("session_start", async (_event, ctx) => {
    continuing = false;
    lastMessage = "";
    failed = false;
    await bridge?.close();
    bridge = undefined;
    if (config.servers.length) {
      const connection = new MCPBridge(ctx.cwd);
      bridge = connection;
      try {
        for (const remote of await connection.ready) {
          pi.registerTool({ name: remote.name, label: remote.tool, description: remote.description,
            parameters: remote.parameters,
            async execute(_id, args, signal) {
              const result = await connection.call(remote, args, signal);
              if (result.isError) throw new Error(JSON.stringify(result.content));
              return { content: (result.content ?? []).map((item) =>
                item.type === "text" || item.type === "image" ? item : { type: "text", text: JSON.stringify(item) }), details: result };
            },
          });
        }
      } catch (error) { await connection.close(); throw error; }
    }
    if (config.events.includes("session-start")) {
      const result = await hook("session-start", ctx, false, "");
      if (result.code !== 0) throw new Error(result.stderr);
      if (result.stdout.trim()) pi.sendMessage({ customType: "ilk-context", content: result.stdout, display: false }, { deliverAs: "nextTurn" });
    }
  });

  pi.on("input", (event) => {
    if (event.source !== "extension") { continuing = false; failed = false; }
    return { action: "continue" };
  });
  pi.on("agent_end", (event) => {
    const messages = event.messages.filter((message) => message.role === "assistant");
    const last = messages[messages.length - 1];
    failed = !last || last.stopReason === "aborted" || last.stopReason === "error";
    lastMessage = last?.content.filter((block) => block.type === "text").map((block) => block.text).join("\n") ?? "";
  });
  pi.on("agent_settled", async (_event, ctx) => {
    if (!config.events.includes("turn-end") || failed || !ctx.isIdle() || ctx.hasPendingMessages()) return;
    const result = await hook("turn-end", ctx, continuing, lastMessage);
    if (result.code === 2) {
      if (continuing) { ctx.ui.notify(result.stderr, "error"); return; }
      continuing = true;
      pi.sendMessage({ customType: "ilk-checkpoint", content: result.stderr, display: false }, { triggerTurn: true, deliverAs: "followUp" });
    } else if (result.code !== 0) {
      ctx.ui.notify(result.stderr, "error");
    }
  });
  pi.on("session_shutdown", async () => { await bridge?.close(); bridge = undefined; });
}
