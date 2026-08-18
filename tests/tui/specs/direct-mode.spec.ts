import { afterEach, describe, it } from 'vitest';
import { createTempHelmdexRepo, rmTempRepo, instancePath } from '../src/repoHarness';
import { startHelmdexTui } from '../src/sessionHarness';
import {
  expectFileExists,
  expectFileMissing,
  snapshotTree,
  expectTreeUnchanged
} from '../../shared/diskAssertions';
import { realHelmEnv } from '../../shared/fakehelm';

/**
 * A helmdex-agnostic repo (no helmdex.yaml) is not helmdex's to rewrite:
 * every values file is user-owned, edited in place, never generated. This is
 * the invariant easiest to break by accident, so it gets a byte-level test.
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

describe('TUI on a helmdex-agnostic repo', () => {
  it('browsing leaves every file byte-identical', async () => {
    const repo = await createTempHelmdexRepo({ agnostic: true });
    cleanup.push(() => rmTempRepo(repo));

    const before = await snapshotTree(repo.dir);

    const h = await startHelmdexTui(repo, { env: realHelmEnv() });
    cleanup.push(() => h.kill());

    // The discovered instance is visible without any config file.
    await h.waitForText('demo-preprod', 30_000);
    await h.press(['Enter']);
    await h.waitForText('Dependencies');

    // Walk the tabs, including Values.
    await h.press(['ArrowRight']);
    await h.waitForText('Values');
    await h.press(['ArrowRight']);
    await h.waitStable(30_000);

    await h.press(['Escape']);
    await h.waitForText('Instances');

    expectTreeUnchanged(before, await snapshotTree(repo.dir));
  });

  it('regenerating values is a no-op: helmdex owns nothing here', async () => {
    const repo = await createTempHelmdexRepo({ agnostic: true });
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo, { env: realHelmEnv() });
    cleanup.push(() => h.kill());

    await h.waitForText('demo-preprod', 30_000);
    await h.press(['Enter']);
    await h.waitForText('Dependencies');

    const before = await snapshotTree(instancePath(repo, 'demo-preprod'));

    await h.press(['r']);
    await h.waitStable(30_000);

    expectTreeUnchanged(before, await snapshotTree(instancePath(repo, 'demo-preprod')));
    await expectFileMissing(repo, 'apps', 'demo-preprod', 'values.instance.yaml');
    await expectFileMissing(repo, '.helmdex');
  });

  it('creating an instance stays direct-mode and does not opt the repo in', async () => {
    const repo = await createTempHelmdexRepo({ agnostic: true });
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo, { env: realHelmEnv() });
    cleanup.push(() => h.kill());

    await h.waitForText('demo-preprod', 30_000);
    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('demo-review-42');
    await h.press(['Enter']);
    await h.waitForText('Dependencies');

    await expectFileExists(repo, 'apps', 'demo-review-42', 'values.yaml');
    await expectFileMissing(repo, 'apps', 'demo-review-42', 'values.instance.yaml');
    await expectFileMissing(repo, 'helmdex.yaml');
    await expectFileMissing(repo, '.helmdex');
  });
});
