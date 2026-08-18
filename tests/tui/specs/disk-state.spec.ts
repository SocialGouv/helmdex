import { afterEach, describe, expect, it } from 'vitest';
import { createTempHelmdexRepo, rmTempRepo, instancePath } from '../src/repoHarness';
import { startHelmdexTui } from '../src/sessionHarness';
import {
  expectChartDeps,
  expectFileContains,
  expectFileExists,
  expectFileMissing,
  fileExists,
  listInstanceFiles
} from '../../shared/diskAssertions';
import { realHelmEnv } from '../../shared/fakehelm';

/**
 * Screen assertions prove the TUI rendered something. These prove it did
 * something: every flow here ends on the repo as it exists on disk.
 */

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

describe('TUI writes what it says it writes', () => {
  it('creating an instance writes a managed umbrella chart', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('alpha');
    await h.waitForText('Dependencies');

    await expectFileContains(repo, ['apps', 'alpha', 'Chart.yaml'], [
      'name: alpha',
      'apiVersion: v2'
    ]);
    // Managed mode: the user-owned layer exists and helmdex owns values.yaml.
    await expectFileExists(repo, 'apps', 'alpha', 'values.instance.yaml');
  });

  it('renaming an instance moves the directory and rewrites the chart name', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('Dependencies');
    await expectFileExists(repo, 'apps', 'alpha', 'Chart.yaml');

    // Rename lives on the Instance tab (Deps -> Values -> Instance).
    await h.press(['ArrowRight', 'ArrowRight']);
    await h.waitForText('r: rename');
    await h.press(['r']);
    await h.waitStable(30_000);
    // The prompt is prefilled with the current name.
    await h.press(['Backspace', 'Backspace', 'Backspace', 'Backspace', 'Backspace']);
    await h.type('beta');
    await h.press(['Enter']);
    await h.waitForText('Name: beta', 30_000);

    await expectFileMissing(repo, 'apps', 'alpha');
    await expectFileContains(repo, ['apps', 'beta', 'Chart.yaml'], 'name: beta');
  });

  it('regenerating values produces a merged values.yaml on disk', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('Dependencies');

    // A value in the user-owned layer must show up in the generated output.
    const layer = instancePath(repo, 'alpha');
    const fs = await import('node:fs/promises');
    await fs.writeFile(`${layer}/values.instance.yaml`, 'replicaCount: 6\n', 'utf8');

    await h.press(['r']);
    await h.waitForText('Values regenerated', 30_000);

    await expectFileContains(repo, ['apps', 'alpha', 'values.yaml'], 'replicaCount: 6');
  });

  it('confirming the delete prompt removes the instance directory', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('doomed');
    await h.press(['Enter']);
    await h.waitForText('Dependencies');
    await expectFileExists(repo, 'apps', 'doomed', 'Chart.yaml');

    await h.press(['Escape']);
    await h.waitForText('Instances');

    await h.press(['d']);
    await h.waitForText('y delete • n cancel • Esc cancel');
    await h.press(['y']);
    await h.waitStable(30_000);

    await expectFileMissing(repo, 'apps', 'doomed');
    expect(await listInstanceFiles(repo, 'doomed')).toEqual([]);
  });

  it('drafting an arbitrary dependency records it in Chart.yaml', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    // Real Helm code paths, fake registry: the dependency the wizard records
    // is the one the registry actually publishes.
    const h = await startHelmdexTui(repo, { env: realHelmEnv() });
    cleanup.push(() => h.kill());

    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('Dependencies');

    await h.press(['a']);
    await h.waitForText('Choose source');
    await h.pressMany(['ArrowDown', 'ArrowDown']);
    await h.press(['Enter']);

    await h.waitForText('repo>');
    await h.type('https://example.invalid/charts');
    await h.press(['Enter']);

    // Wait for the chart list to be populated, not merely titled, then pick
    // a named chart rather than whatever happens to be selected.
    await h.selectInList('nginx');
    await h.press(['Enter']);

    // Same for versions: wait for a concrete version to exist.
    await h.selectInList('15.2.0');
    await h.press(['Enter']);

    await h.waitForText('alias>');
    await h.press(['Enter']);
    await h.waitForText('Dependency applied', 30_000);

    await expectChartDeps(repo, 'alpha', ['nginx']);
    await expectFileContains(repo, ['apps', 'alpha', 'Chart.yaml'], [
      'repository: https://example.invalid/charts',
      'version: 15.2.0'
    ]);
    if (await fileExists(repo, '.helmdex', 'depmeta', 'alpha', 'nginx.yaml')) {
      await expectFileContains(repo, ['.helmdex', 'depmeta', 'alpha', 'nginx.yaml'], 'kind: arbitrary');
    }
  });
});
