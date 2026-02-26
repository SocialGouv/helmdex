import { afterEach, describe, it } from 'vitest';
import { createTempHelmdexRepo, rmTempRepo } from '../src/repoHarness';
import { startHelmdexTui, writeArtifact } from '../src/sessionHarness';
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

describe('TUI navigation', () => {
  it('toggles help overlay with ? and closes with esc', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.press(['?']);
    await h.waitStable(30_000);
    let screen = await h.screenshotText();
    if (!screen.includes('Help')) await writeArtifact('navigation-help-open.txt', screen);
    expectIncludes(screen, 'Help', 'help overlay heading should render');
    expectIncludes(screen, 'Global:', 'help overlay body should render');

    await h.press(['Escape']);
    await h.waitStable(30_000);
    screen = await h.screenshotText();
    expectIncludes(screen, 'Instances', 'should return to dashboard');
    expectIncludes(screen, '/ filter', 'dashboard footer should be visible');
  });
});

