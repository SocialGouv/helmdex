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

describe('TUI add-dependency wizard (tier 1)', () => {
  it('catalog list flow: enters catalog, opens detail, then backs out', async () => {
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

    // Open add-dep wizard.
    await h.press(['a']);
    await h.waitForText('Add dependency');
    await h.screenshotAndAssertIncludes('Select source');

    // Choose predefined catalog.
    // Catalog sync can race with our assertions (a sync-done message can
    // temporarily reset the wizard back to Select source).
    // Strategy: press Enter until we actually land in the Catalog step.
    for (let i = 0; i < 5; i++) {
      await h.press(['Enter']);
      await h.waitStable(30_000);
      const s = await h.screenshotText();
      if (s.includes('Catalog is empty') || s.includes('/ filter • ↑/↓ select • enter: next • esc back')) break;
    }
    await h.screenshotAndAssertIncludes('Add dependency');

    // If catalog is empty in this run, exercise recovery hints and back out.
    // Otherwise enter detail and back out.
    const screen = await h.screenshotText();
    if (screen.includes('Catalog is empty')) {
      await h.screenshotAndAssertIncludes('s: sync catalog • c: configure sources • esc: back');
      await h.press(['Escape']);
      await h.waitForText('Select source');
      await h.press(['Escape']);
      await h.waitForText('Dependencies');
      return;
    }

    await h.screenshotAndAssertIncludes('/ filter • ↑/↓ select • enter: next • esc back');

    // Enter detail.
    await h.press(['Enter']);
    await h.waitForAnyText(['Sets:', 'Loading sets from preset cache…'], 30_000);
    await h.screenshotAndAssertIncludes('enter: add+apply');

    // Back to source chooser (Esc from Catalog detail returns to Select source),
    // then close wizard.
    await h.press(['Escape']);
    await h.waitForText('Select source');
    await h.press(['Escape']);
    await h.waitForText('Dependencies');
  });

  it('arbitrary dep draft flow: fills repo/name/version and drafts dependency', async () => {
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

    await h.press(['a']);
    await h.waitForText('Select source');

    // Move to Arbitrary (third item).
    await h.pressMany(['ArrowDown', 'ArrowDown']);
    await h.press(['Enter']);

    await h.waitForText('repo>');
    await h.type('https://example.invalid/charts');
    await h.press(['Tab']);
    await h.type('nginx');
    await h.press(['Tab']);
    await h.type('1.2.3');
    await h.press(['Enter']);

    await h.waitForText('Dependency applied');
    await h.screenshotAndAssertIncludes('OK Dependency applied');

    // Deps list should include nginx.
    await h.screenshotAndAssertIncludes('nginx');
  });


  it('artifact hub detail flow: auto-loads README/Values for latest + keeps header visible on Versions tab', async () => {
    // Enable deterministic stubs:
    // - Artifact Hub search results/versions
    // - Helm previews (README + values)
    process.env.HELMDEX_E2E_STUB_ARTIFACTHUB = '1';
    process.env.HELMDEX_E2E_STUB_HELM = '1';

    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo, { cols: 120, rows: 40 });
    cleanup.push(() => h.kill());

    // Create + open instance.
    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('alpha');
    await h.press(['Enter']);
    await h.waitForText('Dependencies');

    // Open add-dep wizard.
    await h.press(['a']);
    await h.waitForText('Add dependency');
    await h.waitForText('Select source');

    // Move to Artifact Hub (second item).
    await h.press(['ArrowDown']);
    await h.press(['Enter']);
    await h.waitForText('Artifact Hub search');

    // Search for nginx.
    await h.type('nginx');
    await h.press(['Enter']);
    await h.waitForText('Artifact Hub results');

    // Enter detail.
    await h.press(['Enter']);
    await h.waitForText('README');

    // README should auto-load using the selected latest version.
    await h.waitForText('Stub README');

    // Switch to Values tab (→). Note: tab navigation is bound to `l`/`h` (and
    // left/right arrows) but arrow keys can be swallowed by the versions list
    // depending on focus; `l` is more reliable in E2E.
    await h.press(['l']);
    await h.waitForText('Values');
    await h.waitForText('replicaCount: 1');

    // Switch to Versions tab (→) and ensure the global header remains visible.
    await h.press(['l']);
    await h.waitForText('Versions');
    await h.screenshotAndAssertIncludes(['Add dependency', 'README', 'Values', 'Versions']);
  });
});
