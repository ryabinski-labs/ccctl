// Resume by ID: paste a claude session ID (or a `claude --resume` command) into New session.
// Runs headless against the e2e server (fake Tailscale, fake claude); never in the user's own browser.

import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import type { Page } from '@playwright/test';
import { expect, openPage, paneText, test, type Controller } from './fixtures';

const ID = '18cf1881-5565-4e54-b0fd-ffafef9e86b1';

function saveTranscript(ctl: Controller, cwd: string, id = ID, extra: object[] = [{ type: 'ai-title', aiTitle: 'Quota issue', sessionId: id }]) {
  const dir = join(ctl.home, '.claude', 'projects', cwd.replaceAll('/', '-'));
  mkdirSync(dir, { recursive: true });
  const lines = [{ type: 'user', cwd, gitBranch: 'main', sessionId: id }, ...extra];
  writeFileSync(join(dir, `${id}.jsonl`), lines.map((l) => JSON.stringify(l)).join('\n') + '\n');
}

async function openResume(page: Page, ctl: Controller) {
  await openPage(page, ctl);
  await page.getByTestId('new-session').click();
  const form = page.getByTestId('launch-form');
  await form.getByText('Resume by ID').click();
  return form;
}

test('Resume by ID finds the session folder, names the task, and resumes it', async ({ page, controller }) => {
  const cwd = join(controller.home, 'Documents', 'repo1');
  saveTranscript(controller, cwd);
  await openPage(page, controller);
  await page.getByTestId('new-session').click();
  const form = page.getByTestId('launch-form');

  await form.getByText('Resume by ID').click();
  await expect(form.getByRole('heading')).toHaveText(/^Resume a session in slot \d$/);
  await expect(form.getByLabel('Session ID')).toBeFocused();
  await expect(form.getByText('Folder', { exact: true })).toHaveCount(0);
  await expect(form.getByTestId('launch-submit')).toBeDisabled();

  await form.getByLabel('Session ID').fill('not-an-id');
  await expect(form.getByTestId('resume-error')).toContainText('Paste a session ID');

  await form.getByLabel('Session ID').fill(`claude --resume ${ID}`);
  const found = form.getByTestId('resume-found');
  await expect(found).toContainText('Quota issue');
  await expect(found).toContainText(cwd);
  await expect(form.getByLabel('Task name')).toHaveValue('quota-issue');

  await form.getByTestId('launch-submit').click();
  await expect(form).toHaveCount(0);
  await expect.poll(() => paneText(page, 1)).toContain(`--resume ${ID}`);
  await expect.poll(() => paneText(page, 1)).toContain(`cwd=${cwd}`);

  // The same session cannot be opened twice.
  await page.getByTestId('new-session').click();
  await form.getByText('Resume by ID').click();
  await form.getByLabel('Session ID').fill(ID);
  await expect(form.getByTestId('resume-found')).toContainText('Already open in slot 1.');
  await expect(form.getByTestId('launch-submit')).toBeDisabled();
});

test('A session open in another terminal cannot be resumed', async ({ page, controller }) => {
  saveTranscript(controller, join(controller.home, 'Documents', 'repo1'));
  const reg = join(controller.home, '.claude', 'sessions');
  mkdirSync(reg, { recursive: true });
  writeFileSync(join(reg, `${process.pid}.json`), JSON.stringify({ pid: process.pid, sessionId: ID }));
  const form = await openResume(page, controller);
  await form.getByLabel('Session ID').fill(ID);
  await expect(form.getByTestId('resume-found')).toContainText(`Open in another terminal (process ${process.pid}). Quit it there first.`);
  await expect(form.getByTestId('launch-submit')).toBeDisabled();
});

test('A session whose folder was deleted is flagged before submit', async ({ page, controller }) => {
  saveTranscript(controller, join(controller.home, 'deleted-worktree'));
  const form = await openResume(page, controller);
  await form.getByLabel('Session ID').fill(ID);
  await expect(form.getByTestId('resume-found')).toContainText('This folder no longer exists, so the session cannot resume.');
  await expect(form.getByTestId('launch-submit')).toBeDisabled();
});

test('An untitled session shows its last prompt and names the task from it', async ({ page, controller }) => {
  saveTranscript(controller, join(controller.home, 'Documents', 'repo1'), ID, [
    { type: 'last-prompt', lastPrompt: 'Fix the login redirect loop on Safari please', sessionId: ID },
  ]);
  const form = await openResume(page, controller);
  await form.getByLabel('Session ID').fill(ID);
  await expect(form.getByTestId('resume-found')).toContainText('Untitled session');
  await expect(form.getByTestId('resume-found')).toContainText('Last prompt: Fix the login redirect loop on Safari please');
  await expect(form.getByLabel('Task name')).toHaveValue('fix-the-login-redirect-loop-on-safari');
});

test('Arrow keys switch modes without leaving the radio group', async ({ page, controller }) => {
  await openPage(page, controller);
  await page.getByTestId('new-session').click();
  const form = page.getByTestId('launch-form');
  await form.getByRole('radio', { name: 'Start new' }).focus();
  await page.keyboard.press('ArrowRight');
  await expect(form.getByRole('radio', { name: 'Resume by ID' })).toBeChecked();
  await expect(form.getByRole('radio', { name: 'Resume by ID' })).toBeFocused();
  await page.keyboard.press('ArrowLeft');
  await expect(form.getByRole('radio', { name: 'Start new' })).toBeFocused();
});

test('A task name filled from the session is cleared when switching back to Start new', async ({ page, controller }) => {
  saveTranscript(controller, join(controller.home, 'Documents', 'repo1'));
  const form = await openResume(page, controller);
  await form.getByLabel('Session ID').fill(ID);
  await expect(form.getByLabel('Task name')).toHaveValue('quota-issue');
  await form.getByText('Start new').click();
  await expect(form.getByLabel('Task name')).toHaveValue('');
});

test('Input without a session ID is rejected in the page, without a request', async ({ page, controller }) => {
  const lookups: string[] = [];
  page.on('request', (r) => r.url().includes('/api/transcript') && lookups.push(r.url()));
  const form = await openResume(page, controller);
  for (const bad of ['hello', ID.slice(0, 18), `${ID} 00000000-0000-4000-8000-000000000000`]) {
    await form.getByLabel('Session ID').fill(bad);
    await expect(form.getByTestId('resume-error')).toContainText('Paste a session ID');
  }
  expect(lookups).toEqual([]);
});
