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

describe('TUI tier 2 flows (env-gated stubs)', () => {
  it('dep detail modal: open, tab switch, delete confirm cancel', async () => {
    process.env.HELMDEX_E2E_STUB_HELM = '1';
    process.env.HELMDEX_E2E_NO_EDITOR = '1';

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

    // Draft a dep so dep detail can open.
    await h.press(['a']);
    await h.waitForText('Select source');
    await h.pressMany(['ArrowDown', 'ArrowDown']);
    await h.press(['Enter']);
    await h.waitForText('repo>');
    await h.type('https://example.invalid/charts');
    await h.press(['Tab']);
    await h.type('nginx');
    await h.press(['Tab']);
    await h.type('1.0.0');
    await h.press(['Enter']);
    await h.waitForText('Dependency applied');

    // Open dep detail by pressing Enter on deps list.
    await h.press(['Enter']);
    await h.waitForText('Dependency');
    await h.screenshotAndAssertIncludes('←/→ tabs • esc close');

    // Tab switch.
    await h.press(['ArrowRight']);
    await h.waitStable(30_000);
    await h.screenshotAndAssertIncludes('Values');

    // Delete from detail -> confirm modal -> cancel.
    await h.press(['d']);
    await h.waitForText('Delete dependency');
    await h.screenshotAndAssertIncludes('y: delete • n: cancel • esc: cancel');
    await h.press(['n']);
    await h.waitForText('Dependency');
    await h.press(['Escape']);
    await h.waitForText('Dependencies');
  });

  it('upgrade diff modal appears and cancels', async () => {
    process.env.HELMDEX_E2E_STUB_HELM = '1';
    process.env.HELMDEX_E2E_NO_EDITOR = '1';

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

    // Draft dep.
    await h.press(['a']);
    await h.waitForText('Select source');
    await h.pressMany(['ArrowDown', 'ArrowDown']);
    await h.press(['Enter']);
    await h.waitForText('repo>');
    await h.type('https://example.invalid/charts');
    await h.press(['Tab']);
    await h.type('nginx');
    await h.press(['Tab']);
    await h.type('1.0.0');
    await h.press(['Enter']);
    await h.waitForText('Dependency applied');

    // Trigger upgrade -> opens diff.
    await h.press(['u']);
    await h.waitForText('Upgrade diff', 30_000);
    await h.screenshotAndAssertIncludes('y apply • n/esc cancel');

    await h.press(['n']);
    await h.waitForText('Dependencies');
  });

  it('apply overlay appears and cancel confirmation flow works', async () => {
    process.env.HELMDEX_E2E_STUB_HELM = '1';
    process.env.HELMDEX_E2E_NO_EDITOR = '1';

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

    // Start apply.
    await h.press(['p']);

    // In stub mode, apply may complete quickly (OK Applied) or briefly show the
    // overlay (Applying). Accept either, but when overlay appears, exercise the
    // cancel-confirm UX.
    await h.waitStable(30_000);
    const screen = await h.screenshotText();
    if (screen.includes('Applying')) {
      await h.press(['Escape']);
      await h.waitForText('Cancel apply?');
      await h.screenshotAndAssertIncludes('This is best-effort; it may still finish in the background.');

      // Cancel confirmation with n.
      await h.press(['n']);
      await h.waitForText('Applying');
    } else {
      await h.screenshotAndAssertIncludes('OK Applied');
    }
  });
});
