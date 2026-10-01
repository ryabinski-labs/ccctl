// Acceptance scenarios from tdd/claude-code-controller.tdd.yaml. Scenario IDs stay in the test
// names so QA maps results back by ID. Runs headless against the e2e server (fake Tailscale,
// fake claude, real PTYs); never in the user's own browser.

import { join } from 'node:path';
import { expect, openPage, paneText, test } from './fixtures';

test.describe('REQ-001 — The owner can launch a Claude Code session into a free slot for a named task; it starts within 5 seconds in a new git worktree on branch ctl/TASK with --dangerously-skip-permissions and a recorded --session-id.', () => {
  test('SC-001-a Launch form starts a live session in a new worktree', async ({ page, controller }) => {
    await openPage(page, controller);
    await page.getByTestId('new-session').click();
    const form = page.getByTestId('launch-form');
    await form.getByLabel('Task name').fill('fix-login');
    await form.getByLabel('Search repositories').fill('folio');
    await form.getByRole('option', { name: /folio/ }).first().click();
    await expect(form.getByTestId('worktree-toggle')).toBeChecked();
    await form.getByLabel(/First prompt/).fill('hello world');
    await form.getByTestId('launch-submit').click();

    let text = '';
    await expect
      .poll(async () => (text = await paneText(page, 1)), { timeout: 5000 })
      .toMatch(/FAKE_CLAUDE cwd=\S*\/folio-wt-fix-login/);
    expect(text).toContain('--dangerously-skip-permissions');
    expect(text).toContain('prompt=hello world');
    const m = /--session-id ([0-9a-f-]{36})/.exec(text);
    expect(m, 'banner shows --session-id UUID').not.toBeNull();
    expect(controller.stateFile().slots[0]?.session_id).toBe(m![1]);
  });
});

test.describe('REQ-004 — The owner sees every session\'s live output in its own pane and keystrokes typed in a pane reach only that session\'s PTY; a maximized pane resizes its PTY.', () => {
  test('SC-004-a Typing in pane 2 reaches only session 2', async ({ page, controller }) => {
    for (const n of [1, 2, 3, 4]) {
      await controller.launch({ task: `t${n}`, path: join(controller.home, 'Documents', `repo${n}`) });
    }
    await openPage(page, controller);
    for (const n of [1, 2, 3, 4]) {
      await expect.poll(() => paneText(page, n), { timeout: 5000 }).toContain('FAKE_CLAUDE');
    }
    await page.locator('[data-slot="2"] .xterm').click();
    await page.keyboard.type('hello');
    await page.keyboard.press('Enter');

    await expect.poll(() => paneText(page, 2), { timeout: 2000 }).toContain('ECHO hello');
    for (const n of [1, 3, 4]) expect(controller.inputLog(n).length, `slot ${n} input bytes`).toBe(0);
  });
});

test.describe('REQ-006 — The controller checks Tailscale every 10 seconds: when Tailscale is not Running it serves nothing and reports the spec message, keeps sessions alive, and binds again when Tailscale returns; the page shows a reconnect banner with 1, 2, 4, 8, 16, then 30 second retries.', () => {
  test('SC-006-f Banner appears when the controller goes away', async ({ page, controller }) => {
    await openPage(page, controller);
    await expect(page.getByTestId('reconnect-banner')).toHaveCount(0);
    controller.kill();
    const banner = page.getByTestId('reconnect-banner');
    await expect(banner).toBeVisible({ timeout: 3000 });
    await expect(banner).toHaveText(/^Lost connection to .*Check that Tailscale is on for this device/);
  });
});

test.describe('REQ-008 — A pane shows Needs input within 2 seconds of Claude Code\'s Notification or Stop hook, the tab title counts such panes, a single chime plays per transition unless muted, and any keystroke clears it.', () => {
  test('SC-008-e Needs-input badge appears', async ({ page, controller }) => {
    await controller.launch({ task: 'waiter', path: join(controller.home, 'Documents', 'repo1') });
    await openPage(page, controller);
    await expect.poll(() => paneText(page, 1), { timeout: 5000 }).toContain('FAKE_CLAUDE');
    await page.locator('[data-slot="1"] .xterm').click();
    await page.keyboard.type('!stop');
    await page.keyboard.press('Enter');
    await expect(page.locator('[data-slot="1"] [data-testid="pane-header"]')).toContainText('Needs input', {
      timeout: 2000,
    });
  });
});
