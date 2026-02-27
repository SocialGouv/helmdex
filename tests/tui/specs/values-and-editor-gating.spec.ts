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

describe('TUI values tab', () => {
  it('opens values preview modal and closes with esc', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    // Create + open instance.
    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('alpha');
    await h.press(['Enter']);
    await h.waitForText('Dependencies');

    // Go to Values tab and open preview.
    await h.press(['ArrowRight']);
    await h.waitForText('Values');
    // Preview selected file.
    await h.press(['Enter']);
    await h.waitForText('Preview values');
    await h.screenshotAndAssertIncludes('Esc close');

    await h.press(['Escape']);
    await h.waitForText('Values');
  });

  it('regen values reports status OK', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('alpha');
    await h.press(['Enter']);
    await h.waitForText('Dependencies');

    await h.press(['r']);
    await h.waitForText('Values regenerated', 30_000);
    await h.screenshotAndAssertIncludes('OK Values regenerated');
  });

  it('edit values is gated on selecting values.instance.yaml (error surface)', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('alpha');
    await h.press(['Enter']);
    await h.waitForText('Dependencies');

    // Values tab: if selection is not values.instance.yaml, `e` errors.
    await h.press(['ArrowRight']);
    await h.waitForText('Values');

    // Ensure there are multiple values files so we can select the generated one.
    await h.press(['r']);
    await h.waitForText('Values regenerated', 30_000);

    // Move selection down to values.yaml (merged output) and attempt edit.
    await h.pressMany(['ArrowDown']);
    await h.press(['e']);
    await h.waitForText('Select values.instance.yaml', 30_000);
    await h.screenshotAndAssertIncludes('ERR Select values.instance.yaml');

    // Esc clears persistent footer errors when no modal is open.
    await h.press(['Escape']);
    await h.waitStable(30_000);
    const screen = await h.screenshotText();
    if (screen.includes('Select values.instance.yaml')) {
      throw new Error('expected footer error to clear on esc');
    }
  });
});
