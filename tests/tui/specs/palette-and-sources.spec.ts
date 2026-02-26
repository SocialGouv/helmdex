import { afterEach, describe, it } from 'vitest';
import { createTempHelmdexRepo, rmTempRepo } from '../src/repoHarness';
import { startHelmdexTui } from '../src/sessionHarness';

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

describe('TUI command palette + sources', () => {
  it('opens command palette and closes with esc', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.press(['m']);
    await h.waitForText('Command palette');
    await h.screenshotAndAssertIncludes('type to search • ↑/↓ select • enter run • esc close');

    await h.press(['Escape']);
    await h.waitForText('Instances');
    await h.screenshotAndAssertIncludes('/ filter');
  });

  it('opens sources modal from palette and closes with esc', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.press(['m']);
    await h.waitForText('Command palette');

    // Filter to sources.
    await h.type('sources');
    await h.press(['Enter']);

    await h.waitForText('Configure sources');
    await h.screenshotAndAssertIncludes('tab: next field • shift+tab: prev field • enter: save • esc: close');

    await h.press(['Escape']);
    await h.waitForText('Instances');
    await h.screenshotAndAssertIncludes('/ filter');
  });
});

