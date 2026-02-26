import { afterEach, describe, it } from 'vitest';
import { createTempHelmdexRepo, rmTempRepo } from '../src/repoHarness';
import { startHelmdexTui } from '../src/sessionHarness';
import { expectIncludes } from '../src/assertions';

let cleanup: Array<() => Promise<void>> = [];
afterEach(async () => {
  for (const fn of cleanup.reverse()) {
    try {
      await fn();
    } catch {
      // ignore
    }
  }
  cleanup = [];
});

describe('TUI smoke', () => {
  it('app launches and main dashboard UI is visible', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.waitStable(30_000);
    const screen = await h.screenshotText();

    expectIncludes(screen, 'Instances', 'expected dashboard list title');
    expectIncludes(screen, '/ filter', 'expected dashboard footer help');
  });
});

