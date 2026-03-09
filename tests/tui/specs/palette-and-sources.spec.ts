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
    // UX: palette no longer renders its own hint footer; global context help is authoritative.
    await h.screenshotAndAssertIncludes('type to search');

    await h.press(['Escape']);
    // Wait for the dashboard footer hint that is only visible when the palette is closed.
    await h.waitForText('/ filter');
    await h.screenshotAndAssertIncludes('/ filter');
  });

  it('opens sources modal from palette and closes with esc', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.press(['m']);
    await h.waitForText('Command palette');

    // Filter to sources; wait for results before selecting.
    await h.type('sources');
    await h.waitForText('2 items');
    await h.press(['Enter']);
    // Wait for the sources modal to appear before asserting.
    await h.waitForText('Tab next field');
    // UX: sources modal footer hints are normalized.
    await h.screenshotAndAssertIncludes('Tab next field');

    await h.press(['Escape']);
    // Wait for the dashboard footer hint that is only visible when the sources modal is closed.
    await h.waitForText('/ filter');
    await h.screenshotAndAssertIncludes('/ filter');
  });
});
