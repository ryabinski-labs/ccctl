import { spawn, type ChildProcess } from 'node:child_process';
import { readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { createInterface } from 'node:readline';
import { test as base, expect, type Page } from '@playwright/test';

export interface Controller {
  url: string;
  dir: string;
  home: string;
  proc: ChildProcess;
  /** Launch a session through the REST API (the scenario's Given). */
  launch(req: { task: string; path: string; worktree?: boolean; prompt?: string }): Promise<{ slot: number }>;
  /** Bytes the fake claude for `slot` read from its PTY. */
  inputLog(slot: number): Buffer;
  stateFile(): { version: number; slots: ({ slot: number; session_id: string } | null)[] };
  kill(): void;
}

async function startController(args: string[]): Promise<Controller> {
  const bin = join(process.env.CCCTL_E2E_BIN!, 'e2eserver');
  const proc = spawn(bin, args, { stdio: ['ignore', 'pipe', 'inherit'] });
  const first = await new Promise<string>((resolveLine, reject) => {
    const rl = createInterface({ input: proc.stdout! });
    const timer = setTimeout(() => reject(new Error('e2e server did not report its address within 15 s')), 15_000);
    rl.once('line', (line) => {
      clearTimeout(timer);
      resolveLine(line);
    });
    proc.once('exit', (code) => reject(new Error(`e2e server exited early with code ${code}`)));
  });
  // Keep draining stdout so the server never blocks on a full pipe.
  proc.stdout!.resume();
  const { url, dir } = JSON.parse(first) as { url: string; dir: string };
  const home = join(dir, 'home');
  const ctl: Controller = {
    url,
    dir,
    home,
    proc,
    async launch(req) {
      const res = await fetch(`${url}/api/sessions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Origin: url },
        body: JSON.stringify({ worktree: false, prompt: '', ...req }),
      });
      const body = await res.json();
      if (res.status !== 201) throw new Error(`launch ${req.task} failed: HTTP ${res.status} ${JSON.stringify(body)}`);
      return body;
    },
    inputLog(slot) {
      const p = join(dir, 'fakeclaude', `input-${slot}.bin`);
      return existsSync(p) ? readFileSync(p) : Buffer.alloc(0);
    },
    stateFile() {
      return JSON.parse(readFileSync(join(home, '.ccctl', 'state.json'), 'utf8'));
    },
    kill() {
      if (proc.exitCode === null && proc.signalCode === null) proc.kill('SIGKILL');
    },
  };
  return ctl;
}

export const test = base.extend<{ controller: Controller; serverArgs: string[] }>({
  serverArgs: [['--slots-folders', '4'], { option: true }],
  controller: async ({ serverArgs }, use) => {
    const ctl = await startController(serverArgs);
    await use(ctl);
    ctl.kill();
  },
});

export { expect };

/** Text of a pane's terminal buffer, via the page's read-only test hook. */
export function paneText(page: Page, slot: number): Promise<string> {
  return page.evaluate((n) => window.__ccctl?.text(n) ?? '', slot);
}

export async function openPage(page: Page, ctl: Controller) {
  await page.goto(ctl.url);
  await expect(page.getByTestId('host')).not.toHaveText('');
}
