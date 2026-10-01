// TDD scenarios from tdd/claude-code-controller.tdd.yaml — scenario IDs stay in the test names
// so QA maps results back by ID. Red phase is over: the markers are removed, the assertions stay.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { defineComponent, h } from 'vue';
import type { SessionState, SlotInfo } from '../lib/types';

vi.mock('../lib/api', () => {
  class ApiError extends Error {
    constructor(message: string, readonly status: number) {
      super(message);
    }
  }
  return {
    ApiError,
    api: {
      envNames: vi.fn(async () => []),
      envSet: vi.fn(async () => []),
      envRemove: vi.fn(async () => []),
      repos: vi.fn(async () => [{ name: 'notes', path: '/h/notes', display: '~/notes' }]),
      inspect: vi.fn(async (path: string) => ({
        path,
        git: false,
        worktree_allowed: false,
        note: 'Not a git repository: session runs in the folder itself.',
      })),
      launch: vi.fn(),
      stop: vi.fn(async () => ({})),
      fresh: vi.fn(),
      close: vi.fn(async () => ({})),
    },
  };
});

import { api } from '../lib/api';
import { chime } from '../lib/chime';
import { Connection } from '../lib/connection';
import { bannerText } from '../lib/backoff';
import { useSessions } from '../stores/sessions';
import AppHeader from '../components/AppHeader.vue';
import LaunchDialog from '../components/LaunchDialog.vue';
import PaneGrid from '../components/PaneGrid.vue';
import ReconnectBanner from '../components/ReconnectBanner.vue';
import SessionPane from '../components/SessionPane.vue';

// xterm needs canvas/layout; unit tests stub the terminal (it is covered by the e2e suite).
const TerminalStub = defineComponent({ name: 'TerminalView', props: ['slot'], setup: () => () => h('div') });
const stubs = { TerminalView: TerminalStub };

function info(slot: number, state: SessionState, task = `task-${slot}`): SlotInfo {
  return {
    slot,
    task,
    cwd: `/h/repo-wt-${task}`,
    repo: 'repo',
    worktree: `/h/repo-wt-${task}`,
    branch: `ctl/${task}`,
    session_id: `uuid-${slot}-${task}`,
    state,
    launched_at: '2026-09-30T00:00:00Z',
    resumed: false,
    exit_code: state === 'exited' ? 0 : null,
    message: '',
  };
}

beforeEach(() => {
  setActivePinia(createPinia());
  document.body.innerHTML = '';
  vi.clearAllMocks();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('REQ-002 — Invalid launch input is refused with the spec\'s message and no session, folder, or branch is created.', () => {
  it('SC-002-g Launch form disables worktree for non-git folders', async () => {
    const w = mount(LaunchDialog, { props: { slot: 1 }, attachTo: document.body, global: { stubs } });
    await flushPromises();
    await w.get('[role="option"][data-path="/h/notes"]').trigger('click');
    await flushPromises();
    expect(api.inspect).toHaveBeenCalledWith('/h/notes');

    const toggle = w.get('[data-testid="worktree-toggle"]').element as HTMLInputElement;
    expect(toggle.checked).toBe(false);
    expect(toggle.disabled).toBe(true);
    const note = w.get('[data-testid="worktree-note"]');
    expect(note.text()).toBe('Not a git repository: session runs in the folder itself.');
    expect(note.isVisible()).toBe(true);
    w.unmount();
  });
});

describe('REQ-004 — The owner sees every session\'s live output in its own pane and keystrokes typed in a pane reach only that session\'s PTY; a maximized pane resizes its PTY.', () => {
  it('SC-004-e Maximize and restore', async () => {
    const store = useSessions();
    store.slots = [1, 2, 3, 4].map((n) => info(n, 'running'));
    const w = mount(PaneGrid, { attachTo: document.body, global: { stubs } });
    const visible = () => w.findAll('section.pane').filter((p) => p.isVisible());

    expect(visible()).toHaveLength(4);
    await w.get('[data-slot="3"] [data-testid="maximize"]').trigger('click');
    expect(visible()).toHaveLength(1);
    expect(visible()[0].attributes('data-slot')).toBe('3');

    await w.get('[data-slot="3"] [data-testid="maximize"]').trigger('click');
    expect(visible()).toHaveLength(4);
    expect((w.get('[data-testid="grid"]').element as HTMLElement).style.gridTemplateColumns).toBe('repeat(2, 1fr)');
    w.unmount();
  });
});

describe('REQ-006 — The controller checks Tailscale every 10 seconds: when Tailscale is not Running it serves nothing and reports the spec message, keeps sessions alive, and binds again when Tailscale returns; the page shows a reconnect banner with 1, 2, 4, 8, 16, then 30 second retries.', () => {
  it('SC-006-e Reconnect backoff and banner text', async () => {
    vi.useFakeTimers();
    let constructed = 0;
    class FailingWS {
      binaryType = 'blob';
      readyState = 0;
      onopen: (() => void) | null = null;
      onclose: (() => void) | null = null;
      onerror: (() => void) | null = null;
      onmessage: ((e: MessageEvent) => void) | null = null;
      constructor() {
        constructed++;
        // Every attempt fails: the socket closes on the next tick without opening.
        setTimeout(() => this.onclose?.(), 0);
      }
      send() {}
      close() {}
    }

    const store = useSessions();
    store.host = 'mac-mini';
    const delays: number[] = [];
    let firstBanner = '';
    const w = mount(ReconnectBanner);
    const conn = new Connection(
      'ws://mac-mini:7681/ws',
      {
        onHello: () => {},
        onSlot: () => {},
        onOutput: () => {},
        onStatus: (connected, retryIn) => {
          store.connected = connected;
          store.lost = !connected;
          store.retryIn = retryIn;
        },
        onRetryScheduled: (d) => delays.push(d),
      },
      FailingWS as unknown as new (url: string) => WebSocket,
    );
    conn.connect();

    await vi.advanceTimersByTimeAsync(0); // initial connection drops
    await w.vm.$nextTick();
    firstBanner = w.get('[data-testid="reconnect-banner"]').text();

    // 7 consecutive reconnect attempts fail.
    for (let i = 0; delays.length < 7 && i < 1000; i++) await vi.advanceTimersByTimeAsync(1000);
    conn.close();

    expect(delays.slice(0, 7)).toEqual([1, 2, 4, 8, 16, 30, 30]);
    expect(constructed).toBeGreaterThanOrEqual(7);
    expect(firstBanner).toBe(
      'Lost connection to mac-mini. Check that Tailscale is on for this device and for mac-mini. Retrying in 1 s.',
    );
    expect(bannerText('mac-mini', 1)).toBe(firstBanner);
    w.unmount();
  });
});

describe('REQ-007 — At most 6 sessions are active (starting, running, needs-input); a launch takes the lowest free slot, where exited and resume-failed panes count as free and are replaced without touching their worktrees.', () => {
  it('SC-007-d New session control state', async () => {
    const store = useSessions();
    store.slots = [info(1, 'running'), info(2, 'starting'), info(3, 'needs-input'), info(4, 'running'), info(5, 'running'), info(6, 'running')];
    const w = mount(AppHeader);
    const btn = () => w.get('[data-testid="new-session"]').element as HTMLButtonElement;
    expect(btn().disabled).toBe(true);
    expect(btn().title).toBe('All 6 slots are in use. Stop or close a session first.');

    store.slots = [info(1, 'running'), info(2, 'starting'), info(3, 'needs-input'), info(4, 'running'), info(5, 'running'), info(6, 'exited')];
    await w.vm.$nextTick();
    expect(btn().disabled).toBe(false);
    w.unmount();
  });
});

describe('REQ-008 — A pane shows Needs input within 2 seconds of Claude Code\'s Notification or Stop hook, the tab title counts such panes, a single chime plays per transition unless muted, and any keystroke clears it.', () => {
  it('SC-008-d Title count and chime', () => {
    const play = vi.spyOn(chime, 'play').mockImplementation(() => {});
    const store = useSessions();
    store.setMuted(false);
    store.host = 'mac-mini';
    store.applyHello({
      type: 'hello',
      host: 'mac-mini',
      login: 'owner@example',
      slots: [info(1, 'running'), info(2, 'running'), info(3, 'running'), null],
    });
    expect(document.title.startsWith('(')).toBe(false);

    store.applySlot(1, info(1, 'needs-input'));
    store.applySlot(3, info(3, 'needs-input'));
    expect(document.title.startsWith('(2) ')).toBe(true);

    store.applySlot(1, info(1, 'needs-input')); // re-report: no new transition
    expect(document.title.startsWith('(2) ')).toBe(true);

    store.setMuted(true);
    store.applySlot(2, info(2, 'needs-input'));
    expect(document.title.startsWith('(3) ')).toBe(true);
    expect(play).toHaveBeenCalledTimes(2);

    for (const n of [1, 2, 3]) store.applySlot(n, info(n, 'running'));
    expect(document.title).toBe('ccctl · mac-mini');
  });
});

describe('REQ-010 — Stopping a session sends SIGHUP to its process group, then SIGKILL after 5 seconds; stopped or self-exited sessions show the exited message and always keep their worktree and branch.', () => {
  it('SC-010-c Stop confirmation', async () => {
    const store = useSessions();
    store.slots = [info(1, 'running', 'fix-login'), null, null, null];
    const w = mount(SessionPane, { props: { slot: 1 }, attachTo: document.body, global: { stubs } });

    await w.get('[data-testid="stop"]').trigger('click');
    expect(w.get('[data-testid="stop-dialog"] h2').text()).toBe('Stop fix-login? The worktree and branch are kept.');
    await w.get('[data-testid="stop-dialog"] [data-cancel]').trigger('click');
    expect(w.find('[data-testid="stop-dialog"]').exists()).toBe(false);
    expect(api.stop).toHaveBeenCalledTimes(0);

    await w.get('[data-testid="stop"]').trigger('click');
    await w.get('[data-testid="stop-dialog"] [data-confirm]').trigger('click');
    await flushPromises();
    expect(api.stop).toHaveBeenCalledTimes(1);
    expect(api.stop).toHaveBeenCalledWith(1);
    w.unmount();
  });
});

describe('REQ-006 — stale page after an upgrade', () => {
  it('SC-006-i Page offers a reload when ccctl was upgraded', async () => {
    setActivePinia(createPinia());
    const store = useSessions();
    const hello = (version: string) => ({ type: 'hello' as const, host: 'mac-mini', login: 'o', version, slots: [null, null, null, null] });
    store.applyHello(hello('v0.1.1'));
    store.applyHello(hello('v0.1.1')); // plain reconnect, same build
    expect(store.updated).toBe(false);
    store.applyHello(hello('v0.1.2'));
    expect(store.updated).toBe(true);
    const w = mount(ReconnectBanner);
    await flushPromises();
    expect(w.get('[data-testid="updated-banner"]').text()).toContain('ccctl on mac-mini was updated. Reload to use the new version');
  });
});

describe('REQ-015 — session environment is set from the page and is write-only', () => {
  it('SC-015-b Settings dialog never shows a saved value and confirms removal', async () => {
    setActivePinia(createPinia());
    const { default: SettingsDialog } = await import('../components/SettingsDialog.vue');
    let names: string[] = ['GEMINI_API_KEY'];
    const set = vi.spyOn(api, 'envSet').mockImplementation(async (n) => (names = [...names, n].sort()));
    vi.spyOn(api, 'envNames').mockImplementation(async () => names);
    const rm = vi.spyOn(api, 'envRemove').mockImplementation(async (n) => (names = names.filter((x) => x !== n)));
    const w = mount(SettingsDialog, { attachTo: document.body });
    await flushPromises();
    expect(w.findAll('[data-testid="env-row"]').map((r) => r.text())).toEqual([expect.stringContaining('GEMINI_API_KEY')]);
    await w.get('#env-name').setValue('CLAUDE_CODE_OAUTH_TOKEN');
    await w.get('#env-value').setValue('sk-ant-oat01-CANARY');
    expect(w.get('#env-value').attributes('type')).toBe('password');
    await w.get('form').trigger('submit');
    await flushPromises();
    expect(set).toHaveBeenCalledWith('CLAUDE_CODE_OAUTH_TOKEN', 'sk-ant-oat01-CANARY');
    expect(document.body.innerHTML).not.toContain('sk-ant-oat01-CANARY');
    expect((w.get('#env-value').element as HTMLInputElement).value).toBe('');
    expect(w.get('[data-testid="env-saved"]').text()).toContain('CLAUDE_CODE_OAUTH_TOKEN saved');
    await w.get('[aria-label="Remove GEMINI_API_KEY"]').trigger('click');
    expect(rm).not.toHaveBeenCalled();
    await w.get('.btn--danger').trigger('click');
    await flushPromises();
    expect(rm).toHaveBeenCalledWith('GEMINI_API_KEY');
    w.unmount();
  });
});
