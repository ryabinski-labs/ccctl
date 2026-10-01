// Acceptance scenarios from tdd/repo-prefix.tdd.yaml. Scenario IDs stay in the test names so QA
// maps results back by ID. Runs headless against the e2e server (fake Tailscale, fake claude);
// never in the user's own browser. The e2e HOME holds Documents/repo1..N, Documents/folio,
// other/outside and an empty folder named empty.

import { readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import type { Page } from '@playwright/test';
import { expect, openPage, test } from './fixtures';

const TIMEOUT_MSG = 'Scanning took too long. On a Mac, allow ccctl to access this folder in the prompt on the host, then retry.';

const repoPaths = (page: Page) =>
  page.locator('#repo-list [role=option]').evaluateAll((els) => els.map((e) => e.getAttribute('data-path') ?? ''));

async function openLaunch(page: Page) {
  await page.getByTestId('new-session').click();
  return page.getByTestId('launch-form');
}

test.describe('REQ-pfx-set — The owner can set, change and clear one repository folder in Settings', () => {
  test.describe('no prefix saved', () => {
    test.use({ serverArgs: ['--slots-folders', '3', '--prefix', ''] });

    test('SC-pfx-set-f Saving a folder in Settings narrows the New session list', async ({ page, controller }) => {
      await openPage(page, controller);
      await page.getByTestId('settings-open').click();
      const dialog = page.getByTestId('settings-dialog');
      await dialog.getByLabel('Repository folder').fill('~/Documents');
      await dialog.getByRole('button', { name: 'Save folder' }).click();
      await expect(dialog.getByRole('status').filter({ hasText: 'Repository folder saved.' })).toHaveText(
        'Repository folder saved. It applies the next time you open New session.',
      );
      await dialog.getByRole('button', { name: 'Done' }).click();
      await openLaunch(page);
      await expect.poll(() => repoPaths(page)).toEqual(
        expect.arrayContaining([join(controller.home, 'Documents', 'repo1'), join(controller.home, 'Documents', 'repo3')]),
      );
      const paths = await repoPaths(page);
      expect(paths.some((p) => p.endsWith('/outside'))).toBe(false);
    });
  });

  test.describe('prefix ~/Documents saved', () => {
    test.use({ serverArgs: ['--slots-folders', '3', '--prefix', 'Documents'] });

    test('SC-pfx-set-g Settings shows the folder error under the field', async ({ page, controller }) => {
      await openPage(page, controller);
      await page.getByTestId('settings-open').click();
      const dialog = page.getByTestId('settings-dialog');
      await dialog.getByLabel('Repository folder').fill('/no/such/folder');
      await dialog.getByRole('button', { name: 'Save folder' }).click();
      await expect(dialog.getByRole('alert').filter({ hasText: 'Folder /no/such/folder' })).toHaveText(
        'Folder /no/such/folder does not exist or is not a folder.',
      );
      expect(readFileSync(join(controller.home, '.ccctl', 'config.toml'), 'utf8')).toContain('repo_prefix = "~/Documents"');
      await dialog.getByRole('button', { name: 'Done' }).click();
      await openLaunch(page);
      await expect.poll(() => repoPaths(page)).toContain(join(controller.home, 'Documents', 'repo1'));
    });
  });
});

test.describe('REQ-pfx-noscan — With no prefix set, nothing is scanned and the list offers Settings', () => {
  test.use({ serverArgs: ['--slots-folders', '3', '--prefix', ''] });

  test('SC-pfx-noscan-c The unset message offers Settings and keeps the custom path', async ({ page, controller }) => {
    await openPage(page, controller);
    const form = await openLaunch(page);
    await expect(form.locator('#repo-list')).toContainText('No repository folder is set.');
    await expect(form.getByRole('button', { name: 'Use a custom path' })).toBeVisible();
    await form.getByRole('button', { name: 'Open Settings' }).click();
    await expect(page.getByTestId('settings-dialog')).toBeVisible();
    await expect(page.locator('#repo-prefix')).toBeFocused();
  });
});

test.describe('REQ-pfx-timeout — A scan that does not finish is stopped with an explanation and Retry', () => {
  test.use({ serverArgs: ['--slots-folders', '3', '--prefix', 'Documents', '--scan-timeout', '300ms', '--block-reads', '1'] });

  test('SC-pfx-timeout-d Retry after a timed-out scan lists the repos', async ({ page, controller }) => {
    let scans = 0;
    page.on('request', (r) => {
      if (new URL(r.url()).pathname === '/api/repos') scans++;
    });
    await openPage(page, controller);
    const form = await openLaunch(page);
    await expect(form.locator('#repo-list')).toContainText(TIMEOUT_MSG);
    const before = scans;
    await form.getByRole('button', { name: 'Retry' }).click();
    await expect(form.getByRole('option', { name: /repo1/ })).toBeVisible();
    expect(scans).toBe(before + 1);
  });
});

test.describe('REQ-pfx-empty — An empty or missing prefix folder says which', () => {
  test.use({ serverArgs: ['--slots-folders', '3', '--prefix', 'empty'] });

  test('SC-pfx-empty-c The launch dialog shows the empty and missing messages', async ({ page, controller }) => {
    await openPage(page, controller);
    let form = await openLaunch(page);
    await expect(form.locator('#repo-list')).toContainText(
      'No repositories found under ~/empty. Use a custom path or change the folder in Settings.',
    );
    await form.getByText('Cancel', { exact: true }).click();
    rmSync(join(controller.home, 'empty'), { recursive: true });
    form = await openLaunch(page);
    await expect(form.locator('#repo-list')).toContainText('The repository folder ~/empty was not found. Change it in Settings.');
  });
});
