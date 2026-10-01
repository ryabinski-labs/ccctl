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

  test('Several pasted files arrive in order, each path escaped and followed by a space', async ({ page, controller }) => {
    await start(page, controller);
    await page.evaluate(() => {
      const dt = new DataTransfer();
      for (const n of ['one (1).png', 'résumé #2.pdf', 'three.txt']) dt.items.add(new File([n], n));
      document.querySelector('[data-slot="1"] .xterm-helper-textarea')!.dispatchEvent(
        new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }),
      );
    });
    await expect.poll(() => uploads(controller).length, { timeout: 5000 }).toBe(3);
    await expect
      .poll(() => controller.inputLog(1).toString('utf8'), { timeout: 3000 })
      .toMatch(/-one_1_\.png \S+-r_sum_2\.pdf \S+-three\.txt $/);
    const typed = controller.inputLog(1).toString('utf8');
    // No stored name keeps a space, bracket, or non-ASCII character.
    for (const n of uploads(controller)) expect(n).toMatch(/^[A-Za-z0-9._-]+$/);
    expect(typed.indexOf('one_1')).toBeLessThan(typed.indexOf('three.txt'));
  });

  test('A file over 25 MB shows an error and types nothing', async ({ page, controller }) => {
    await start(page, controller);
    await page.locator('[data-slot="1"] [data-testid="attach-input"]').setInputFiles({
      name: 'big.bin',
      mimeType: 'application/octet-stream',
      buffer: Buffer.alloc(25 * 1024 * 1024 + 1),
    });
    await expect(page.locator('[data-slot="1"] .perror')).toHaveText('That file is over 25 MB.', { timeout: 15_000 });
    expect(uploads(controller)).toEqual([]);
    expect(controller.inputLog(1).toString('utf8')).not.toContain('big.bin');
  });

  test('Files pasted in two panes go to their own session', async ({ page, controller }) => {
    await controller.launch({ task: 'a', path: join(controller.home, 'Documents', 'repo1') });
    await controller.launch({ task: 'b', path: join(controller.home, 'Documents', 'repo2') });
    await openPage(page, controller);
    for (const n of [1, 2]) await expect.poll(() => paneText(page, n), { timeout: 5000 }).toContain('FAKE_CLAUDE');
    for (const [slot, name] of [
      [1, 'for-one.txt'],
      [2, 'for-two.txt'],
    ] as const) {
      await page.evaluate(
        ([sl, nm]) => {
          const dt = new DataTransfer();
          dt.items.add(new File(['x'], nm));
          document.querySelector(`[data-slot="${sl}"] .xterm-helper-textarea`)!.dispatchEvent(
            new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }),
          );
        },
        [slot, name] as const,
      );
    }
    await expect.poll(() => uploads(controller).length, { timeout: 5000 }).toBe(2);
    await expect.poll(() => controller.inputLog(1).toString('utf8'), { timeout: 3000 }).toContain('for-one.txt ');
    await expect.poll(() => controller.inputLog(2).toString('utf8'), { timeout: 3000 }).toContain('for-two.txt ');
    expect(controller.inputLog(1).toString('utf8')).not.toContain('for-two');
    expect(controller.inputLog(2).toString('utf8')).not.toContain('for-one');
  });

  test('A dropped file is uploaded like a pasted one', async ({ page, controller }) => {
    await start(page, controller);
    await page.evaluate(() => {
      const dt = new DataTransfer();
      dt.items.add(new File(['dropped'], 'dropped.txt'));
      document.querySelector('[data-slot="1"] .term')!.dispatchEvent(
        new DragEvent('drop', { dataTransfer: dt, bubbles: true, cancelable: true }),
      );
    });
    await expect.poll(() => uploads(controller).length, { timeout: 5000 }).toBe(1);
    await expect.poll(() => controller.inputLog(1).toString('utf8'), { timeout: 3000 }).toContain('-dropped.txt ');
  });

  test('A failed upload shows an error and types nothing', async ({ page, controller }) => {
    await start(page, controller);
    await page.route('**/api/sessions/1/upload*', (r) => r.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"disk on fire"}' }));
    await page.locator('[data-slot="1"] [data-testid="attach-input"]').setInputFiles({ name: 'a.txt', mimeType: 'text/plain', buffer: Buffer.from('x') });
    await expect(page.locator('[data-slot="1"] .perror')).toHaveText('disk on fire');
    expect(controller.inputLog(1).toString('utf8')).not.toContain('a.txt');
  });

  test('Multi-line pasted text reaches the session as lines', async ({ page, controller }) => {
    await start(page, controller);
    await page.evaluate(() => {
      const dt = new DataTransfer();
      dt.setData('text/plain', 'first line\nsecond line');
      document.querySelector('[data-slot="1"] .xterm-helper-textarea')!.dispatchEvent(
        new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }),
      );
    });
    await expect.poll(() => paneText(page, 1), { timeout: 3000 }).toContain('ECHO first line');
    await page.keyboard.press('Enter');
    await expect.poll(() => paneText(page, 1), { timeout: 3000 }).toContain('ECHO second line');
  });
});

test.describe('Copy out of a session (OSC 52)', () => {
  test('A clipboard write from the session reaches the browser clipboard', async ({ page, controller, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await start(page, controller);
    await page.keyboard.type('!copy copied from claude');
    await page.keyboard.press('Enter');
    await expect.poll(() => paneText(page, 1), { timeout: 3000 }).toContain('COPIED');
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText()), { timeout: 3000 }).toBe('copied from claude');
  });

  test('Without a secure context (plain HTTP on a Tailscale IP) it falls back to execCommand', async ({ page, controller }) => {
    await page.addInitScript(() => {
      Object.defineProperty(window, 'isSecureContext', { value: false });
      (window as unknown as { __copied: string[] }).__copied = [];
      document.execCommand = (cmd: string) => {
        const el = document.activeElement as HTMLTextAreaElement | null;
        if (cmd === 'copy' && el?.value) (window as unknown as { __copied: string[] }).__copied.push(el.value);
        return cmd === 'copy';
      };
    });
    await start(page, controller);
    await page.keyboard.type('!copy plain http copy');
    await page.keyboard.press('Enter');
    await expect
      .poll(() => page.evaluate(() => (window as unknown as { __copied: string[] }).__copied), { timeout: 3000 })
      .toEqual(['plain http copy']);
  });
});
