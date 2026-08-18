import { afterEach, describe, it } from 'vitest';
import path from 'node:path';
import os from 'node:os';
import fs from 'node:fs/promises';
import { createTempHelmdexRepo, rmTempRepo } from '../src/repoHarness';
import { startHelmdexTui } from '../src/sessionHarness';
import {
  expectChartDeps,
  expectFileContains,
  expectFileExists,
  listInstanceFiles
} from '../../shared/diskAssertions';
import { fakeHelmCalls, realHelmEnv } from '../../shared/fakehelm';

/**
 * The catalog path, end to end and against the real Helm code paths: a
 * catalog entry must land in Chart.yaml pinned, attributed to its catalog,
 * with its default sets materialised — all of which only exists on disk.
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

async function helmLogPath(): Promise<string> {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'helmdex-helmlog-'));
  cleanup.push(() => fs.rm(dir, { recursive: true, force: true }));
  return path.join(dir, 'fakehelm.log');
}

describe('TUI catalog flow', () => {
  it('adds a catalog entry pinned, attributed and with its default sets', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const logPath = await helmLogPath();
    const h = await startHelmdexTui(repo, { env: realHelmEnv({ logPath }) });
    cleanup.push(() => h.kill());

    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitForText('Dependencies');

    await h.press(['a']);
    await h.waitForText('Choose source');
    // "Predefined catalog" is the first source.
    await h.press(['Enter']);

    // The catalog syncs from the fixture source in the background; sync
    // explicitly if we got there first.
    const landed = await h.waitForAnyText(
      ['bitnami-nginx-15.0.0', 'Catalog is empty'],
      30_000
    );
    if (landed === 'Catalog is empty') {
      await h.press(['s']);
      await h.waitForText('bitnami-nginx-15.0.0', 60_000);
    }

    // Enter the entry detail, then add + apply.
    await h.selectInList('bitnami-nginx-15.0.0');
    await h.press(['Enter']);
    await h.waitForAnyText(['Sets:', 'Loading sets from preset cache…'], 30_000);
    await h.press(['Enter']);
    await h.waitForAnyText(['Dependency applied', 'Applied', 'Dependencies'], 60_000);
    await h.waitStable(30_000);

    // Pinned exactly as the catalog declares.
    await expectChartDeps(repo, 'alpha', ['nginx']);
    await expectFileContains(repo, ['apps', 'alpha', 'Chart.yaml'], [
      'name: nginx',
      'version: 15.0.0'
    ]);

    // Attributed to the catalog it came from, so detach and upgrade-from-
    // catalog have something to work with.
    await expectFileContains(repo, ['.helmdex', 'depmeta', 'alpha', 'nginx.yaml'], [
      'kind: catalog',
      'bitnami-nginx-15.0.0'
    ]);

    // The catalog entry declares defaultSets: [dev]; the marker must exist.
    const files = await listInstanceFiles(repo, 'alpha');
    if (!files.some((f) => f.startsWith('values.dep-set.nginx--dev'))) {
      throw new Error(`expected a dev set marker for nginx, got: ${files.join(', ')}`);
    }

    // Applying really relocked through Helm.
    const calls = fakeHelmCalls(logPath);
    if (!calls.some((c) => c.startsWith('dependency'))) {
      throw new Error(`expected a helm dependency call, got: ${JSON.stringify(calls)}`);
    }
    await expectFileExists(repo, 'apps', 'alpha', 'Chart.lock');
  });
});
