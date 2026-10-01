// The pane grid grows with the number of open sessions, up to six.

import { join } from 'node:path';
import type { Page } from '@playwright/test';
import { expect, openPage, paneText, test, type Controller } from './fixtures';

const visibleSlots = (page: Page) =>
  page.locator('section.pane:visible').evaluateAll((els) => els.map((e) => Number(e.getAttribute('data-slot'))));

/** Distinct columns and rows the visible panes occupy. */
async function shape(page: Page) {
  const boxes = await page.locator('section.pane:visible').evaluateAll((els) =>
    els.map((e) => {
      const r = e.getBoundingClientRect();
      return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) };
    }),
  );
  return { cols: new Set(boxes.map((b) => b.x)).size, rows: new Set(boxes.map((b) => b.y)).size, boxes };
}

async function open(c: Controller, n: number) {
  await c.launch({ task: `t${n}`, path: join(c.home, 'Documents', `repo${n}`) });
}

test.describe('Dynamic pane grid, up to six sessions', () => {
  test('The grid reflows as sessions are added and stops at six', async ({ page, controller }) => {
    await openPage(page, controller);
    // [open sessions, panes shown (open + the start card when a cell is spare), columns, rows]
    const steps: [number, number[], number, number][] = [
      [0, [1], 1, 1],
      [1, [1], 1, 1],
      [2, [1, 2], 2, 1],
      [3, [1, 2, 3, 4], 2, 2],
      [4, [1, 2, 3, 4], 2, 2],
      [5, [1, 2, 3, 4, 5, 6], 3, 2],
      [6, [1, 2, 3, 4, 5, 6], 3, 2],
    ];
    for (const [n, slots, cols, rows] of steps) {
      if (n > 0) await open(controller, n);
      await expect.poll(() => visibleSlots(page), { message: `${n} open` }).toEqual(slots);
      const s = await shape(page);
      expect({ cols: s.cols, rows: s.rows }, `${n} open`).toEqual({ cols, rows });
      for (const b of s.boxes) expect(b.w, `${n} open: pane width`).toBeGreaterThan(300);
    }
    const btn = page.getByTestId('new-session');
    await expect(btn).toBeDisabled();
    await expect(btn).toHaveAttribute('title', 'All 6 slots are in use. Stop or close a session first.');
    await expect(page.locator('section.pane--free:visible')).toHaveCount(0);
  });

  test('A seventh session is refused by the controller', async ({ controller }) => {
    for (let n = 1; n <= 6; n++) await open(controller, n);
    await expect(open(controller, 1)).rejects.toThrow(/409.*All 6 slots are in use/);
  });

  test('Each of six panes has its own terminal and Ctrl+Shift+6 types into the sixth', async ({ page, controller }) => {
    for (let n = 1; n <= 6; n++) await open(controller, n);
    await openPage(page, controller);
    for (let n = 1; n <= 6; n++) await expect.poll(() => paneText(page, n), { timeout: 8000 }).toContain('FAKE_CLAUDE');
    await page.keyboard.press('Control+Shift+Digit6');
    await page.keyboard.type('to six');
    await expect.poll(() => controller.inputLog(6).toString('utf8'), { timeout: 3000 }).toContain('to six');
    for (let n = 1; n <= 5; n++) expect(controller.inputLog(n).toString('utf8'), `slot ${n}`).not.toContain('to six');
  });

  test('Closing a session shrinks the grid back', async ({ page, controller }) => {
    for (let n = 1; n <= 5; n++) await open(controller, n);
    await openPage(page, controller);
    expect((await shape(page)).cols).toBe(3);
    await page.locator('[data-slot="5"] [data-testid="stop"]').click();
    await page.locator('[data-testid="stop-dialog"] [data-confirm]').click();
    await page.locator('[data-slot="5"]').getByRole('button', { name: 'Close slot 5' }).click();
    await expect.poll(() => visibleSlots(page)).toEqual([1, 2, 3, 4]);
    const s = await shape(page);
    expect({ cols: s.cols, rows: s.rows }).toEqual({ cols: 2, rows: 2 });
  });

  test('A maximized pane fills the grid and restoring brings the others back', async ({ page, controller }) => {
    for (let n = 1; n <= 6; n++) await open(controller, n);
    await openPage(page, controller);
    await page.locator('[data-slot="6"] [data-testid="maximize"]').click();
    await expect.poll(() => visibleSlots(page)).toEqual([6]);
    await page.locator('[data-slot="6"] [data-testid="maximize"]').click();
    await expect.poll(() => visibleSlots(page)).toEqual([1, 2, 3, 4, 5, 6]);
  });
});
