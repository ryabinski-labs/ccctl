// Acceptance scenarios from tdd/opencode-support.tdd.yaml. Scenario IDs stay in
// the test names so QA maps results back by ID. Runs headless against the e2e
// server (fake Tailscale, fake Claude/OpenCode, real PTYs).

import { expect, openPage, paneText, test } from './fixtures';

test.describe('REQ-001 — Launch either agent with its own argv and record the agent', () => {
  test('SC-001-a Launch form starts an OpenCode session in a new worktree', async ({ page, controller }) => {
    await openPage(page, controller);
    await page.getByTestId('new-session').click();
    const form = page.getByTestId('launch-form');
    await form.getByTestId('agent-opencode').check();
    await form.getByLabel('Task name').fill('fix-login');
    await form.getByLabel('Search repositories').fill('folio');
    await form.getByRole('option', { name: /folio/ }).first().click();
    await expect(form.getByTestId('worktree-toggle')).toBeChecked();
    await form.getByLabel(/First prompt/).fill('hello world');
    await form.getByTestId('launch-submit').click();

    let text = '';
    await expect
      .poll(async () => (text = await paneText(page, 1)), { timeout: 5000 })
      .toMatch(/FAKE_OPENCODE cwd=\S*\/folio-wt-fix-login/);
    expect(text).toContain('--auto');
    expect(text).toContain('prompt=hello world');
    const m = /--session (ses_[A-Za-z0-9]+)/.exec(text);
    expect(m, 'banner shows --session ses_ ID').not.toBeNull();
    expect(controller.stateFile().slots[0]?.agent).toBe('opencode');
    expect(controller.stateFile().slots[0]?.session_id).toBe(m![1]);
  });
});

test.describe('REQ-002 — Both agents share the six slots', () => {
  test('SC-002-a Claude and OpenCode run side by side in the grid', async ({ page, controller }) => {
    await openPage(page, controller);
    await page.getByTestId('new-session').click();
    let form = page.getByTestId('launch-form');
    await form.getByLabel('Task name').fill('claude-task');
    await form.getByLabel('Search repositories').fill('repo1');
    await form.getByRole('option', { name: /repo1/ }).first().click();
    await form.getByTestId('launch-submit').click();
    await expect.poll(() => paneText(page, 1), { timeout: 5000 }).toContain('FAKE_CLAUDE');

    await page.getByTestId('new-session').click();
    form = page.getByTestId('launch-form');
    await expect(form.locator('h2')).toContainText('slot 2');
    await form.getByTestId('agent-opencode').check();
    await form.getByLabel('Task name').fill('opencode-task');
    await form.getByLabel('Search repositories').fill('repo2');
    await form.getByRole('option', { name: /repo2/ }).first().click();
    await form.getByTestId('launch-submit').click();
    await expect.poll(() => paneText(page, 2), { timeout: 5000 }).toContain('FAKE_OPENCODE');

    await expect(page.locator('[data-slot="1"] [data-testid="pane-header"]')).toContainText('Claude Code');
    await expect(page.locator('[data-slot="2"] [data-testid="pane-header"]')).toContainText('OpenCode');
    expect(controller.stateFile().slots[0]?.agent).toBe('claude');
    expect(controller.stateFile().slots[1]?.agent).toBe('opencode');
  });
});

test.describe('REQ-003 — OpenCode needs-input through the plugin', () => {
  test('SC-003-a OpenCode permission event turns the pane amber', async ({ page, controller }) => {
    await controller.launch({ task: 'waiter', path: `${controller.home}/Documents/repo1`, agent: 'opencode' });
    await openPage(page, controller);
    await expect.poll(() => paneText(page, 1), { timeout: 5000 }).toContain('FAKE_OPENCODE');
    await page.locator('[data-slot="1"] .xterm').click();
    await page.keyboard.type('!notify');
    await page.keyboard.press('Enter');

    await expect(page.locator('[data-slot="1"] [data-testid="pane-header"]')).toContainText('Needs input', { timeout: 2000 });
    await expect(page).toHaveTitle(/^\(1\)/);
    await expect.poll(() => controller.log(), { timeout: 2000 }).toContain('"source":"opencode-plugin"');
    expect(controller.log()).toContain('"agent":"opencode"');
  });
});

test.describe('REQ-004 — Pane headers name each agent', () => {
  test('SC-004-a Pane headers name each agent', async ({ page, controller }) => {
    await controller.launch({ task: 'claude-one', path: `${controller.home}/Documents/repo1` });
    await controller.launch({ task: 'opencode-one', path: `${controller.home}/Documents/repo2`, agent: 'opencode' });
    await openPage(page, controller);
    await expect(page.locator('[data-slot="1"] [data-testid="pane-header"]')).toContainText('Claude Code');
    await expect(page.locator('[data-slot="2"] [data-testid="pane-header"]')).toContainText('OpenCode');
    expect(await page.locator('[data-slot="1"]').getAttribute('aria-label')).toContain('Claude Code');
    expect(await page.locator('[data-slot="2"]').getAttribute('aria-label')).toContain('OpenCode');
  });
});

test.describe('REQ-007 — Agent selector, memory, and Resume by ID limit', () => {
  test('SC-007-a OpenCode selection explains the Resume by ID limit', async ({ page, controller }) => {
    await openPage(page, controller);
    await page.getByTestId('new-session').click();
    const form = page.getByTestId('launch-form');
    await form.getByTestId('agent-opencode').check();
    await form.getByTestId('mode-resume').check();
    await expect(form.getByTestId('resume-agent-note')).toHaveText('Resume by ID is Claude Code only in this release.');
    await expect(form.getByTestId('launch-submit')).toBeDisabled();
    await form.getByTestId('agent-claude').check();
    await expect(form.getByTestId('resume-agent-note')).toHaveCount(0);
    await expect(form.getByLabel('Session ID')).toBeEnabled();
  });

  test('SC-007-b Last chosen agent is remembered', async ({ page, browser, controller }) => {
    await openPage(page, controller);
    await page.getByTestId('new-session').click();
    let form = page.getByTestId('launch-form');
    await expect(form.getByTestId('agent-claude')).toBeChecked();
    await form.getByTestId('agent-opencode').check();
    await form.locator('footer').getByRole('button', { name: 'Cancel' }).click();

    await page.getByTestId('new-session').click();
    form = page.getByTestId('launch-form');
    await expect(form.getByTestId('agent-opencode')).toBeChecked();

    const ctx = await browser.newContext();
    const fresh = await ctx.newPage();
    await openPage(fresh, controller);
    await fresh.getByTestId('new-session').click();
    await expect(fresh.getByTestId('launch-form').getByTestId('agent-claude')).toBeChecked();
    await ctx.close();
  });
});
