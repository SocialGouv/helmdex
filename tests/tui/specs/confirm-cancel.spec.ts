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

describe('TUI confirm modal', () => {
  it('opens delete-instance confirm and cancels with n', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    // Create an instance so delete confirm is available.
    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('alpha');

    // From dashboard: 'd' opens confirm delete.
    await h.press(['Escape']);
    await h.waitForText('Instances');
    await h.press(['d']);
    await h.waitForText('Delete instance');
    await h.screenshotAndAssertIncludes(['y delete • n cancel • Esc cancel', 'Delete instance']);

    await h.press(['n']);
    // Wait for the dashboard footer hint that is only visible after the confirm modal closes.
    await h.waitForText('/ filter');
    await h.screenshotAndAssertIncludes('/ filter');
  });
});
