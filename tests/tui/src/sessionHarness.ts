import path from 'node:path';
import fs from 'node:fs/promises';
import { agentTuiCli } from './agentTui/cli';
import { normalizeScreenText } from './assertions';
import { testEnvArgsForTui, resolveHelmdexBin } from './helmdex';
import { isolatedStateEnv, type TempRepo } from './repoHarness';

export type Harness = {
  sessionId: string;
  screenshotText(): Promise<string>;
  screenshotAndAssertIncludes(needles: string | string[], hint?: string): Promise<void>;
  waitForAnyText(texts: string[], timeoutMs?: number): Promise<string>;
  press(keys: string[]): Promise<void>;
  pressMany(keys: string[]): Promise<void>;
  type(text: string): Promise<void>;
  waitForText(text: string, timeoutMs?: number): Promise<void>;
  waitStable(timeoutMs?: number): Promise<void>;
  selectInList(label: string, timeoutMs?: number): Promise<void>;
  kill(): Promise<void>;
};

// The list delegate marks the selected row with a leading gutter. A modal's
// rounded border draws the same glyph down its left edge, so the gutter alone
// is not enough: inside a bordered panel every row carries it.
const GUTTER = /^\s*│/;

function isSelected(rows: string[], index: number): boolean {
  if (!GUTTER.test(rows[index])) return false;
  const body = rows.filter((l) => l.trim() !== '');
  return body.some((l) => !GUTTER.test(l));
}

export async function startHelmdexTui(
  repo: TempRepo,
  opts?: { cols?: number; rows?: number; env?: Record<string, string> }
): Promise<Harness> {
  const cli = agentTuiCli();

  await cli.daemonStart();
  const health = await cli.health();
  if (!health?.running) throw new Error(`agent-tui daemon not running: ${JSON.stringify(health)}`);

  const helmdex = resolveHelmdexBin();
  // agent-tui does not currently provide a way to set env vars for the child
  // process directly, so we prefix with `env`.
  const helmdexArgs = ['--repo', repo.dir];
  if (repo.configPath) helmdexArgs.push('--config', repo.configPath);

  const sessionId = await cli.run(
    'env',
    [...testEnvArgsForTui({ ...isolatedStateEnv(repo), ...opts?.env }), helmdex, ...helmdexArgs, 'tui'],
    {
      cwd: path.resolve(process.cwd()),
      cols: opts?.cols ?? 120,
      rows: opts?.rows ?? 40
    }
  );

  // Don't rely on --stable early; some apps render blank frames first.
  await cli.waitText(sessionId, 'Instances', 30_000);

  const screenshotText = async (): Promise<string> => {
    const shot = await cli.screenshot(sessionId, { json: true, stripAnsi: true });
    return normalizeScreenText(String(shot?.screenshot ?? ''));
  };

  return {
    sessionId,
    screenshotText,
    async screenshotAndAssertIncludes(needles: string | string[], hint?: string) {
      const txt = await screenshotText();
      const list = Array.isArray(needles) ? needles : [needles];
      for (const needle of list) {
        if (!txt.includes(needle)) {
          const name = `failure-${Date.now()}-${sessionId}-assert.txt`;
          await writeArtifact(name, txt);
          const msg = hint ? `\nHint: ${hint}` : '';
          throw new Error(
            `Expected screen to include ${JSON.stringify(needle)} but it did not.${msg}\n--- SCREEN ---\n${txt}\n--- END ---\nArtifact: tests/tui/artifacts/${name}`
          );
        }
      }
    },
    async waitForAnyText(texts: string[], timeoutMs = 30_000) {
      const want = texts.filter((t) => String(t ?? '').trim() !== '');
      if (want.length === 0) throw new Error('waitForAnyText: no texts provided');
      const deadline = Date.now() + timeoutMs;
      let last = '';
      while (Date.now() < deadline) {
        // eslint-disable-next-line no-await-in-loop
        last = await screenshotText();
        for (const t of want) {
          if (last.includes(t)) return t;
        }
        // eslint-disable-next-line no-await-in-loop
        await new Promise((r) => setTimeout(r, 250));
      }
      const name = `failure-${Date.now()}-${sessionId}-waitForAnyText.txt`;
      await writeArtifact(name, last);
      throw new Error(`Timed out waiting for any of: ${want.map((t) => JSON.stringify(t)).join(', ')}\nArtifact: tests/tui/artifacts/${name}`);
    },
    async press(keys: string[]) {
      await withFailureArtifact(`press-${keys.join('+')}`, async () => {
        await cli.press(sessionId, keys);
      }, screenshotText);
    },
    async pressMany(keys: string[]) {
      // agent-tui accepts multiple keys in one call, but multi-step sequences are
      // sometimes more reliable when sent as discrete presses.
      for (const k of keys) {
        // eslint-disable-next-line no-await-in-loop
        await withFailureArtifact(`press-${k}`, async () => {
          await cli.press(sessionId, [k]);
        }, screenshotText);
      }
    },
    async type(text: string) {
      await withFailureArtifact(`type-${text.slice(0, 24)}`, async () => {
        await cli.type(sessionId, text);
      }, screenshotText);
    },
    async waitForText(text: string, timeoutMs?: number) {
      await withFailureArtifact(`waitForText-${text.slice(0, 24)}`, async () => {
        await cli.waitText(sessionId, text, timeoutMs);
      }, screenshotText);
    },
    async waitStable(timeoutMs?: number) {
      await withFailureArtifact(`waitStable`, async () => {
        await cli.waitStable(sessionId, timeoutMs);
      }, screenshotText);
    },
    /**
     * Moves the list selection onto the row containing `label`.
     *
     * Filtering would be shorter, but it needs two Enters — one to apply the
     * filter, one to act — and under load the second can land while the
     * filter input still has focus. Stepping the selection and verifying the
     * screen after each step has no such window.
     */
    async selectInList(label: string, timeoutMs = 30_000) {
      const deadline = Date.now() + timeoutMs;
      let screen = '';
      let everRendered = false;
      let steps = 0;
      while (Date.now() < deadline) {
        // eslint-disable-next-line no-await-in-loop
        screen = await screenshotText();
        const rows = screen.split('\n');
        // The last match wins: an item's description repeats words from its
        // title, and the title is what carries the selection.
        const target = rows.map((l) => l.includes(label)).lastIndexOf(true);
        if (target < 0) {
          // Not rendered yet — wait rather than move a selection blindly.
          // eslint-disable-next-line no-await-in-loop
          await new Promise((r) => setTimeout(r, 200));
          continue;
        }
        everRendered = true;
        if (isSelected(rows, target)) return;
        // Stepping past the end would loop forever on an unreachable row.
        if (steps > rows.length) break;
        steps++;
        // eslint-disable-next-line no-await-in-loop
        await cli.press(sessionId, ['ArrowDown']);
        // eslint-disable-next-line no-await-in-loop
        await new Promise((r) => setTimeout(r, 150));
      }
      const name = `failure-${Date.now()}-${sessionId}-selectInList.txt`;
      await writeArtifact(name, screen);
      throw new Error(
        everRendered
          ? `Could not move the selection onto ${JSON.stringify(label)} — is a modal covering the list?\nArtifact: tests/tui/artifacts/${name}`
          : `${JSON.stringify(label)} never appeared in the list.\nArtifact: tests/tui/artifacts/${name}`
      );
    },
    async kill() {
      await cli.kill(sessionId);
    }
  };
}

async function withFailureArtifact<T>(
  op: string,
  fn: () => Promise<T>,
  screenshot: () => Promise<string>
): Promise<T> {
  try {
    return await fn();
  } catch (e) {
    try {
      const txt = await screenshot();
      const name = `failure-${Date.now()}-${op.replace(/[^a-zA-Z0-9_.-]+/g, '_')}.txt`;
      await writeArtifact(name, txt);
    } catch {
      // ignore secondary failures
    }
    throw e;
  }
}

export async function writeArtifact(name: string, content: string): Promise<void> {
  const dir = path.resolve(process.cwd(), 'tests/tui/artifacts');
  await fs.mkdir(dir, { recursive: true });
  await fs.writeFile(path.join(dir, name), content, 'utf8');
}
