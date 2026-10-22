// @vitest-environment node
// OpenCode plugin scenarios from tdd/opencode-support.tdd.yaml (REQ-003).
// The test executes the ccctl OpenCode plugin against a real Unix-socket HTTP
// server and the OpenCode v2 plugin contract (default { id, setup }).

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

function eventQueue() {
  const queue: unknown[] = [];
  let waiting: ((r: IteratorResult<unknown>) => void) | null = null;
  let closed = false;
  return {
    push(event: unknown) {
      if (waiting) {
        waiting({ value: event, done: false });
        waiting = null;
      } else {
        queue.push(event);
      }
    },
    stream: {
      [Symbol.asyncIterator]() {
        return {
          next(): Promise<IteratorResult<unknown>> {
            if (queue.length) return Promise.resolve({ value: queue.shift(), done: false });
            if (closed) return Promise.resolve({ value: undefined, done: true });
            return new Promise((resolve) => (waiting = resolve));
          },
          return(): Promise<IteratorResult<unknown>> {
            closed = true;
            if (waiting) {
              waiting({ value: undefined, done: true });
              waiting = null;
            }
            return Promise.resolve({ value: undefined, done: true });
          },
        };
      },
    },
  };
}

async function waitFor(cond: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (cond()) return;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error('condition not met before timeout');
}

describe('SC-003-b OpenCode plugin posts both events and no-ops outside ccctl', () => {
  const cleanups: Array<() => Promise<void> | void> = [];

  afterEach(async () => {
    for (const fn of cleanups.splice(0)) await fn();
    delete process.env.CCCTL_SOCK;
    delete process.env.CCCTL_SLOT;
  });

  it('SC-003-b posts permission.asked and session.idle, and ignores an unset slot', async () => {
    // Given the ccctl OpenCode plugin and a fake Unix socket server
    // When it receives permission.asked and session.idle with CCCTL_SLOT=1,
    //      then an event with CCCTL_SLOT unset
    // Then exactly two POSTs arrive with {slot, hook, session_id}; the unset event posts nothing.
    const mod = await loadPlugin();
    expect(mod, 'ccctl OpenCode plugin template must exist at internal/opencode/plugin.js').not.toBeNull();
    const plugin = (mod as any).default;
    expect(plugin?.id, 'plugin must default-export id ccctl').toBe('ccctl');
    expect(plugin?.setup, 'plugin must default-export a setup function').toBeTypeOf('function');

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
    const events = eventQueue();
    const cleanup = await plugin.setup({ event: { subscribe: () => events.stream } });
    cleanups.push(async () => {
      await cleanup?.();
    });

    events.push({ type: 'permission.asked', data: { sessionID: 'ses_x' } });
    events.push({ type: 'session.idle', data: { sessionID: 'ses_x' } });
    await waitFor(() => seen.length === 2);
    delete process.env.CCCTL_SLOT;
    events.push({ type: 'session.idle', data: { sessionID: 'ses_x' } });
    await new Promise((resolve) => setTimeout(resolve, 100));

    expect(seen).toEqual([
      { slot: 1, hook: 'permission.asked', session_id: 'ses_x' },
      { slot: 1, hook: 'session.idle', session_id: 'ses_x' },
    ]);
  });
});

describe('SC-003-e OpenCode plugin uses the v2 default definition', () => {
  it('SC-003-e default-exports an id and setup and accepts the v2 event data shape', async () => {
    // Given OpenCode v2 loads plugins from a default export
    // When the generated plugin module is inspected
    // Then it exports { id: 'ccctl', setup } and setup subscribes to ctx.event
    const mod = await loadPlugin();
    expect(mod).not.toBeNull();
    const plugin = (mod as any).default;
    expect(plugin).toMatchObject({ id: 'ccctl' });
    expect(typeof plugin.setup).toBe('function');
  });
});
