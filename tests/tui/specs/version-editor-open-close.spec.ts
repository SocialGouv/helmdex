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

describe('TUI dependency version editor (tier 1)', () => {
  it('opens version editor and cancels with esc (OCI manual mode)', async () => {
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

    // Draft an OCI dep (no helm/network needed).
    await h.press(['a']);
    await h.waitForText('Choose source');
    await h.pressMany(['ArrowDown', 'ArrowDown']);
    await h.press(['Enter']);
    await h.waitForText('repo>');
    await h.type('oci://example.invalid/charts');
    // OCI flow skips chart list and goes straight to manual version input.
    await h.press(['Enter']);
    await h.waitForText('Version');
    await h.waitForText('version>');
    await h.type('1.2.3');
    await h.press(['Enter']);
    await h.waitForText('alias>');
    await h.press(['Enter']);
    await h.waitForText('Dependency applied');

    // Open version editor.
    await h.press(['v']);
    await h.waitForText('Change dependency version');
    await h.screenshotAndAssertIncludes('Enter an exact version:');

    await h.press(['Escape']);
    await h.waitForText('Dependencies');
    await h.screenshotAndAssertIncludes('v version • u upgrade');
  });
});
