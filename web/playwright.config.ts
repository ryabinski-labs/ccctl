import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { defineConfig } from '@playwright/test';

// Built binaries live in one temp dir shared by global setup and the workers.
process.env.CCCTL_E2E_BIN ??= mkdtempSync(join(tmpdir(), 'ccctl-e2e-bin-'));

// Acceptance runs headless in an isolated browser only (never the user's own Chrome).
export default defineConfig({
  testDir: 'e2e',
  globalSetup: './e2e/global-setup.ts',
  timeout: 30_000,
  fullyParallel: true,
  reporter: [['list']],
  use: { headless: true, viewport: { width: 1440, height: 900 } },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
});
