// Load the generated extension with Pi's real loader, then exercise native events
// and registered tools against the installed project's actual MCP servers.
import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { execFileSync } from 'node:child_process';

const sdk = process.env.ILK_TEST_PI_ROOT;
const { loadExtensions } = await import(pathToFileURL(join(sdk, 'dist/core/extensions/loader.js')));
const root = await mkdtemp(join(tmpdir(), 'ilk-pi-'));
const binary = process.env.ILK_TEST_BIN;
const env = { ...process.env, PATH: `${resolve(binary, '..')}:${process.env.PATH}` };
process.env.PATH = env.PATH;
const run = (...args) => execFileSync(args[0], args.slice(1), { cwd: root, env, encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'] });
const layer = resolve(import.meta.dirname, '..');
let loaded;
let ctx;
async function emit(type, fields = {}) {
  for (const ext of loaded.extensions) {
    for (const handler of ext.handlers.get(type) ?? []) await handler({ type, ...fields }, ctx);
  }
}
const fields = ['task', 'objective', 'constraints', 'decisions', 'completed', 'remaining', 'verification', 'questions', 'next_actions'];
const summary = Object.fromEntries(fields.map(key => [key, key === 'task' ? 'Pi integration' : 'None']));
try {
  run('git', 'init', '-q');
  run(binary, 'init', '-y', '--test-command', 'true');
  run(binary, 'add', layer, '--allow-exec', '-y');
  run(binary, 'agents', 'add', 'pi', '-y');
  const extension = join(root, '.pi/extensions/ilk/index.ts');
  loaded = await loadExtensions([extension], root);
  assert.deepEqual(loaded.errors, []);
  const messages = [];
  loaded.runtime.sendMessage = (message, options) => messages.push({ message, options });
  const errors = [];
  ctx = { cwd: root, sessionManager: { getSessionId: () => 'pi-native', getSessionFile: () => '/pi/source.jsonl' },
    isIdle: () => true, hasPendingMessages: () => false,
    ui: { notify: (message, level) => { if (level === 'error') errors.push(message); } } };
  await emit('session_start', { reason: 'startup' });
  assert(messages.some(entry => entry.message.customType === 'ilk-context'));
  const tool = (name) => loaded.extensions[0].tools.get(`ilk_session_summaries__${name}`).definition;
  const call = async (name, args) => {
    const result = await tool(name).execute('call', args, new AbortController().signal, undefined, ctx);
    return JSON.parse(result.content[0].text);
  };
  assert.equal((await call('session_summaries_list', {})).summaries.length, 0);
  const end = () => emit('agent_end', { messages: [{ role: 'assistant', stopReason: 'stop', content: [{ type: 'text', text: 'Ready' }] }] });
  await emit('input', { source: 'interactive' });
  await end();
  await emit('agent_settled');
  const request = messages.at(-1);
  assert.equal(request.message.customType, 'ilk-checkpoint');
  assert.equal(request.options.triggerTurn, true);
  assert.equal(request.options.deliverAs, 'followUp');
  const pending = await call('session_summary_read', { harness: 'pi', session_id: 'pi-native' });
  assert.equal(pending.checkpoint.state, 'pending');
  assert.equal((await call('session_summary_save', { harness: 'pi', session_id: 'pi-native', expected_revision: 0,
    checkpoint_id: pending.checkpoint.id, summary })).status, 'staged');
  await end();
  await emit('agent_settled');
  assert.equal((await call('session_summary_read', { harness: 'pi', session_id: 'pi-native' })).revision, 1);
  assert.equal(messages.filter(entry => entry.message.customType === 'ilk-checkpoint').length, 1);
  // A second user turn resets continuation state. A missed draft stops the loop.
  await emit('input', { source: 'rpc' });
  await end(); await emit('agent_settled');
  await end(); await emit('agent_settled');
  assert.equal((await call('session_summary_read', { harness: 'pi', session_id: 'pi-native' })).checkpoint.state, 'missed');
  assert.equal(messages.filter(entry => entry.message.customType === 'ilk-checkpoint').length, 2);
  // Aborted runs must not trigger a summary continuation.
  await emit('input', { source: 'interactive' });
  await emit('agent_end', { messages: [{ role: 'assistant', stopReason: 'aborted', content: [] }] });
  await emit('agent_settled');
  assert.equal(messages.filter(entry => entry.message.customType === 'ilk-checkpoint').length, 2);
  assert.deepEqual(errors, []);
  await emit('session_shutdown', { reason: 'reload' });
  // Fresh load/reload replaces the MCP connection and retrieves prior publications.
  loaded = await loadExtensions([extension], root);
  loaded.runtime.sendMessage = () => {};
  await emit('session_start', { reason: 'reload' });
  assert.equal((await call('session_summaries_list', {})).summaries.length, 1);
  await emit('session_shutdown', { reason: 'exit' });
  loaded = undefined;
  run(binary, 'agents', 'remove', 'pi', '-y');
  await assert.rejects(readFile(extension));
  console.log('Pi: real extension loader, persistent MCP tools, checkpoint continuation, missed/aborted turns, reload and removal passed');
} finally {
  if (loaded && ctx) await emit('session_shutdown', { reason: 'exit' });
  await rm(root, { recursive: true, force: true });
}
