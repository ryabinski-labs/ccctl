// Getting text and files from the viewing device into a session: paste text, paste or
// attach a file. Runs headless against the e2e server (fake claude, real PTYs).

import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import type { Page } from '@playwright/test';
import { expect, openPage, paneText, test, type Controller } from './fixtures';

async function start(page: Page, controller: Controller) {
  await controller.launch({ task: 'paste', path: join(controller.home, 'Documents', 'repo1') });
  await openPage(page, controller);
  await expect.poll(() => paneText(page, 1), { timeout: 5000 }).toContain('FAKE_CLAUDE');
  await page.locator('[data-slot="1"] .xterm').click();
}

function uploads(c: Controller): string[] {
  try {
    return readdirSync(join(c.home, '.ccctl', 'uploads'));
  } catch {
    return [];
  }
}

test.describe('Paste and upload into a session', () => {
  test('Pasted text reaches the session', async ({ page, controller }) => {
    await start(page, controller);
    await page.evaluate(() => {
      const dt = new DataTransfer();
      dt.setData('text/plain', 'from my laptop');
      document.querySelector('[data-slot="1"] .xterm-helper-textarea')!.dispatchEvent(
        new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }),
      );
    });
    await page.keyboard.press('Enter');
    await expect.poll(() => paneText(page, 1), { timeout: 3000 }).toContain('ECHO from my laptop');
  });

  test('A pasted file is stored on the host and its path typed into the session', async ({ page, controller }) => {
    await start(page, controller);
    await page.evaluate(() => {
      const dt = new DataTransfer();
      dt.items.add(new File(['PNGDATA'], 'my shot.png', { type: 'image/png' }));
      document.querySelector('[data-slot="1"] .xterm-helper-textarea')!.dispatchEvent(
        new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }),
      );
    });
    await expect.poll(() => uploads(controller).length, { timeout: 5000 }).toBe(1);
    const name = uploads(controller)[0];
    expect(name).toMatch(/-my_shot\.png$/);
    const full = join(controller.home, '.ccctl', 'uploads', name);
    expect(readFileSync(full, 'utf8')).toBe('PNGDATA');
    await expect
      .poll(() => controller.inputLog(1).toString('utf8'), { timeout: 3000 })
      .toContain(`${full.replace(/[^A-Za-z0-9_\-./~@%+=:,]/g, '\\$&')} `);
  });

  test('The attach button sends a chosen file', async ({ page, controller }) => {
    await start(page, controller);
    await page.locator('[data-slot="1"] [data-testid="attach-input"]').setInputFiles({
      name: 'notes.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('hello'),
    });
    await expect.poll(() => uploads(controller).length, { timeout: 5000 }).toBe(1);
    await expect.poll(() => controller.inputLog(1).toString('utf8'), { timeout: 3000 }).toContain('-notes.txt ');
  });
});
