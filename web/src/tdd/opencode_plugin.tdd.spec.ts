// @vitest-environment node
// Red-phase plugin scenario from tdd/opencode-support.tdd.yaml (REQ-003, SC-003-b).
// The test executes the ccctl OpenCode plugin against a real Unix-socket HTTP server.
// Until the plugin template exists the dynamic import returns null, so the test
// fails at the first assertion — not at import time.

import { createServer } from 'node:http';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { afterEach, describe, expect, it } from 'vitest';

const pluginPath = fileURLToPath(new URL('../../../internal/opencode/plugin.js', import.meta.url));

async function loadPlugin() {
  return import(/* @vite-ignore */ pathToFileURL(pluginPath).href).catch(() => null);
}

describe('SC-003-b OpenCode plugin posts both events and no-ops outside ccctl', () => {
  const cleanups: Array<() => Promise<void> | void> = [];

  afterEach(async () => {
    for (const fn of cleanups.splice(0)) await fn();
  });

  it('SC-003-b posts permission.asked and session.idle, and ignores an unset slot', async () => {
    // Given the ccctl OpenCode plugin and a fake Unix socket server
    // When it receives permission.asked and session.idle with CCCTL_SLOT=1,
    //      then an event with CCCTL_SLOT unset
    // Then exactly two POSTs arrive with {slot, hook, session_id}; the unset event posts nothing.
    const mod = await loadPlugin();
    expect(mod, 'ccctl OpenCode plugin template must exist at internal/opencode/plugin.js').not.toBeNull();

    const dir = await mkdtemp(join(tmpdir(), 'ccctl-plugin-'));
    const sock = join(dir, 'ccctl.sock');
    const seen: Array<Record<string, unknown>> = [];
    const server = createServer((req, res) => {
      let body = '';
      req.on('data', (c) => (body += c));
      req.on('end', () => {
        seen.push(JSON.parse(body));
        res.writeHead(204).end();
      });
    });
    await new Promise<void>((resolve) => server.listen(sock, resolve));
    cleanups.push(async () => {
      await new Promise<void>((resolve) => server.close(() => resolve()));
      await rm(dir, { recursive: true, force: true });
    });

    process.env.CCCTL_SOCK = sock;
    process.env.CCCTL_SLOT = '1';
    const factory = (mod as any).CcctlPlugin ?? (mod as any).default;
    expect(factory, 'plugin must export CcctlPlugin').toBeTypeOf('function');
    const hooks = await factory({});
    const handle = hooks.event;
    expect(handle, 'plugin must return an event hook').toBeTypeOf('function');

    await handle({ event: { type: 'permission.asked', properties: { sessionID: 'ses_x' } } });
    await handle({ event: { type: 'session.idle', properties: { sessionID: 'ses_x' } } });
    delete process.env.CCCTL_SLOT;
    await handle({ event: { type: 'session.idle', properties: { sessionID: 'ses_x' } } });

    expect(seen).toEqual([
      { slot: 1, hook: 'permission.asked', session_id: 'ses_x' },
      { slot: 1, hook: 'session.idle', session_id: 'ses_x' },
    ]);
  });
});
