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

  test('SC-004-j Refresh heals a host that missed a resize frame', async ({ page, controller }) => {
    await controller.launch({ task: 'missed-resize', path: join(controller.home, 'Documents', 'repo1') });
    await page.setViewportSize({ width: 1440, height: 900 });
    // Proxy the socket so the test can drop resize frames, as a sleeping laptop
    // does while its connection is down. Everything else forwards untouched.
    let blockResize = false;
    await page.routeWebSocket(/\/ws$/, (ws) => {
      const server = ws.connectToServer();
      ws.onMessage((message) => {
        if (blockResize && typeof message === 'string' && message.includes('"type":"resize"')) return;
        server.send(message);
      });
    });
    await openPage(page, controller);
    await expect.poll(() => paneText(page, 1), { timeout: 8000 }).toContain('FAKE_CLAUDE');
    await page.waitForTimeout(300); // let the first fit reach the PTY

    const cols = () => page.evaluate(() => window.__ccctl?.size(1)?.cols ?? 0);
    const askSize = async () => {
      await page.keyboard.press('Control+Shift+Digit1');
      await page.keyboard.type('!size\r');
      await page.waitForTimeout(150);
      const lines = (await paneText(page, 1)).split('\n').filter((line) => line.startsWith('SIZE '));
      return lines[lines.length - 1] ?? '';
    };

    const before = await cols();
    blockResize = true;
    await page.setViewportSize({ width: 1024, height: 700 });
    await expect.poll(cols, { timeout: 3000 }).not.toBe(before);
    const after = await cols();
    // The browser has the new grid; the host still paints the old one, which is
    // the blank right and bottom of the pane in the bug report.
    expect(await askSize()).toContain(`SIZE ${before} `);

    blockResize = false;
    await page.getByTestId('refresh-view').click();
    await expect
      .poll(
        async () => {
          await page.waitForTimeout(150);
          return askSize();
        },
        { timeout: 5000 },
      )
      .toContain(`SIZE ${after} `);
  });
});
