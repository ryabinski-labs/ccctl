// Page-level basics found in QA: one h1, file-like paths that do not exist are 404s,
// and the shell fits the window so no pane is clipped below the fold.

import { join } from 'node:path';
import { expect, openPage, paneText, test } from './fixtures';

test.describe('Page basics', () => {
  test('The page has exactly one level-one heading', async ({ page, controller }) => {
    await openPage(page, controller);
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('ccctl');
  });

  test('Missing file-like paths are 404, not the page', async ({ page, controller }) => {
    for (const p of ['/robots.txt', '/sitemap.xml']) {
      const res = await page.request.get(controller.url + p);
      expect(res.status(), p).toBe(404);
    }
  });

  test('SC-004-h A short window fits the page: no pane bottom is clipped', async ({ page, controller }) => {
    await controller.launch({ task: 'short-window', path: join(controller.home, 'Documents', 'repo1') });
    // 560 px is shorter than the shell's old 600 px minimum, which made the page
    // scroll and pushed the bottom of the terminal below the fold.
    await page.setViewportSize({ width: 1024, height: 560 });
    await openPage(page, controller);
    const layout = await page.evaluate(() => {
      const vw = window.innerWidth;
      const vh = window.innerHeight;
      const bottoms = Array.from(document.querySelectorAll<HTMLElement>('section.pane'))
        .filter((el) => el.offsetParent !== null)
        .map((el) => el.getBoundingClientRect().bottom);
      return {
        vw,
        vh,
        paneBottom: Math.max(0, ...bottoms),
        docWidth: document.documentElement.scrollWidth,
        docHeight: document.documentElement.scrollHeight,
        bodyHeight: document.body.scrollHeight,
      };
    });
    expect(layout.paneBottom, 'pane bottom').toBeLessThanOrEqual(layout.vh);
    expect(layout.docHeight, 'document height').toBeLessThanOrEqual(layout.vh);
    expect(layout.bodyHeight, 'body height').toBeLessThanOrEqual(layout.vh);
    expect(layout.docWidth, 'document width').toBeLessThanOrEqual(layout.vw);
  });

  test('SC-004-i Refresh re-fits the terminals without losing their output or input', async ({ page, controller }) => {
    await controller.launch({ task: 'refresh-view', path: join(controller.home, 'Documents', 'repo1') });
    await openPage(page, controller);
    await expect.poll(() => paneText(page, 1), { timeout: 8000 }).toContain('FAKE_CLAUDE');
    await page.getByTestId('refresh-view').click();
    await expect.poll(() => paneText(page, 1)).toContain('FAKE_CLAUDE');
    await page.keyboard.press('Control+Shift+Digit1');
    await page.keyboard.type('after refresh');
    await expect.poll(() => controller.inputLog(1).toString('utf8'), { timeout: 3000 }).toContain('after refresh');
  });
});
