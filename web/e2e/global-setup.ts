import { execFileSync } from 'node:child_process';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const repoRoot = resolve(webDir, '..');

export default function globalSetup() {
  const bin = process.env.CCCTL_E2E_BIN;
  if (!bin) throw new Error('CCCTL_E2E_BIN is not set (playwright.config.ts sets it).');
  // The e2e server embeds web/dist, so build the frontend first.
  execFileSync('npm', ['run', 'build'], { cwd: webDir, stdio: 'inherit' });
  // The e2e server finds ccctl (hook entry point) and the fake claude next to itself.
  for (const [out, pkg] of [
    ['e2eserver', './internal/e2e/cmd/e2eserver'],
    ['ccctl', './cmd/ccctl'],
    ['fakeclaude', './internal/testutil/fakeclaude'],
  ]) {
    execFileSync('go', ['build', '-o', join(bin, out), pkg], { cwd: repoRoot, stdio: 'inherit' });
  }
}
