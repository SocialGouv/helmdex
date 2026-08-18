import { afterEach, describe, it } from 'vitest';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import { createTempHelmdexRepo, rmTempRepo, instancePath } from '../src/repoHarness';
import { startHelmdexTui } from '../src/sessionHarness';
import { realHelmEnv, waitForHelmCall } from '../../shared/fakehelm';
import { expectFileContains, expectFileExists } from '../../shared/diskAssertions';

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

    // Create instance (app auto-navigates to the instance view).
    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('alpha');
    await h.waitForText('Dependencies');

    // Draft a dep so dep detail can open.
    await h.press(['a']);
    await h.waitForText('Choose source');
    await h.pressMany(['ArrowDown', 'ArrowDown']);
    await h.press(['Enter']);
    await h.waitForText('repo>');
    await h.type('https://example.invalid/charts');
    await h.press(['Enter']);
    await h.waitForAnyText(['Chart', '/ filter'], 30_000);
    await h.press(['Enter']);
    await h.waitForAnyText(['Version', 'Loading versions'], 30_000);
    // Pick a non-latest version so upgrade has something to do.
    await h.pressMany(['ArrowDown', 'ArrowDown', 'ArrowDown']);
    await h.press(['Enter']);
    await h.waitForText('alias>');
    await h.press(['Enter']);
    await h.waitForText('Dependency applied');

    // Open dep detail by pressing Enter on deps list.
    await h.press(['Enter']);
    // Wait for the detail modal footer hint (unique to the detail view).
    await h.waitForText('Esc close');
    await h.screenshotAndAssertIncludes('Esc close');

    // Tab switch.
    await h.press(['ArrowRight']);
    await h.waitStable(30_000);
    await h.screenshotAndAssertIncludes('Values');

    // Delete from detail -> confirm modal -> cancel.
    await h.press(['d']);
    await h.waitForText('Delete dependency');
    await h.screenshotAndAssertIncludes('y delete • n cancel • Esc cancel');
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

    // Create instance (app auto-navigates to the instance view).
    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('alpha');
    await h.waitForText('Dependencies');

    // Draft dep.
    await h.press(['a']);
    await h.waitForText('Choose source');
    await h.pressMany(['ArrowDown', 'ArrowDown']);
    await h.press(['Enter']);
    await h.waitForText('repo>');
    await h.type('https://example.invalid/charts');
    await h.press(['Enter']);
    await h.waitForAnyText(['Chart', '/ filter'], 30_000);
    await h.press(['Enter']);
    await h.waitForAnyText(['Version', 'Loading versions'], 30_000);
    // Pick a non-latest version so upgrade has something to do.
    await h.pressMany(['ArrowDown', 'ArrowDown', 'ArrowDown']);
    await h.press(['Enter']);
    await h.waitForText('alias>');
    await h.press(['Enter']);
    await h.waitForText('Dependency applied');

    // Trigger upgrade -> opens diff.
    await h.press(['u']);
    // Diff modal title is "Upgrade diff" in the UI, but the full-body modal may
    // not always include the title text in the screenshot output; accept either.
    await h.waitForAnyText(['Upgrade diff', 'Loading diff', 'y apply'], 30_000);
    await h.screenshotAndAssertIncludes('y apply • n/Esc cancel');

    await h.press(['n']);
    await h.waitForText('Dependencies');
  });

  // The cancellable apply overlay belongs to the dependency add+apply flow
  // (startApplyCmd). Plain `p` on an instance only raises a busy status line,
  // which cannot be cancelled — so this scenario has to go through the
  // catalog wizard to reach the overlay at all.
  it('apply overlay appears and cancel confirmation flow works', async () => {
    process.env.HELMDEX_E2E_NO_EDITOR = '1';

    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    // The relock is held open by a gate this test controls, so the overlay is
    // on screen for exactly as long as the assertions need — no timer to race.
    const gateDir = await fs.mkdtemp(path.join(os.tmpdir(), 'helmdex-helmgate-'));
    cleanup.push(() => fs.rm(gateDir, { recursive: true, force: true }));
    const logPath = path.join(gateDir, 'fakehelm.log');
    const releasePath = path.join(gateDir, 'release');

    const h = await startHelmdexTui(repo, {
      env: realHelmEnv({
        logPath,
        blockUntilPath: releasePath,
        delayCommandPrefix: 'dependency'
      })
    });
    cleanup.push(() => h.kill());

    // Create instance (app auto-navigates to the instance view).
    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('alpha');
    await h.waitForText('Dependencies');

    // Add a catalog dependency with apply: this is the flow that opens the
    // blocking, cancellable overlay.
    await h.press(['a']);
    await h.waitForText('Choose source');
    await h.press(['Enter']);

    const landed = await h.waitForAnyText(['bitnami-nginx-15.0.0', 'Catalog is empty'], 30_000);
    if (landed === 'Catalog is empty') {
      await h.press(['s']);
      await h.waitForText('bitnami-nginx-15.0.0', 60_000);
    }
    await h.selectInList('bitnami-nginx-15.0.0');
    await h.press(['Enter']);
    await h.waitForAnyText(['Sets:', 'Loading sets from preset cache…'], 30_000);
    await h.press(['Enter']);

    await h.waitForText('Applying', 30_000);
    // Wait for the gated Helm call itself, so the cancel prompt is driven
    // against a real in-flight apply rather than a race.
    await waitForHelmCall(logPath, 'dependency', 30_000);

    await h.press(['Escape']);
    await h.waitForText('Cancel apply?');
    await h.screenshotAndAssertIncludes('This is best-effort; it may still finish in the background.');

    // Declining the confirmation returns to the running apply.
    await h.press(['n']);
    await h.waitForText('Applying');

    // Releasing the gate lets the apply finish, and it really applied.
    await fs.writeFile(releasePath, '', 'utf8');
    await h.waitForText('Dependency applied', 60_000);
    await expectFileContains(repo, ['apps', 'alpha', 'Chart.yaml'], ['name: nginx', 'version: 15.0.0']);
    await expectFileExists(repo, 'apps', 'alpha', 'Chart.lock');
    // This scenario deliberately holds Helm open, so it needs more than the
    // suite's default budget.
  }, 150_000);
});
