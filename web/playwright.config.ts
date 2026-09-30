import { defineConfig } from '@playwright/test';

// Acceptance runs headless in an isolated browser only (never the user's own Chrome).
export default defineConfig({
  testDir: 'e2e',
  use: { headless: true },
});
