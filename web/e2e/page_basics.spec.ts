// Page-level basics found in QA: one h1, and file-like paths that do not exist are 404s.

import { expect, openPage, test } from './fixtures';

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
});
