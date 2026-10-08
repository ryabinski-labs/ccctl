// Resume by ID: paste a claude session ID (or a `claude --resume` command) into New session.
// Runs headless against the e2e server (fake Tailscale, fake claude); never in the user's own browser.

import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, openPage, paneText, test, type Controller } from './fixtures';

const ID = '18cf1881-5565-4e54-b0fd-ffafef9e86b1';

function saveTranscript(ctl: Controller, cwd: string) {
  const dir = join(ctl.home, '.claude', 'projects', cwd.replaceAll('/', '-'));
  mkdirSync(dir, { recursive: true });
  const lines = [
    { type: 'user', cwd, gitBranch: 'main', sessionId: ID },
    { type: 'ai-title', aiTitle: 'Quota issue', sessionId: ID },
  ];
  writeFileSync(join(dir, `${ID}.jsonl`), lines.map((l) => JSON.stringify(l)).join('\n') + '\n');
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
