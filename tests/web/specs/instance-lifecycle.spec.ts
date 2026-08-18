import { test, expect, repoExists, repoRead } from './fixtures';

/**
 * The journey the web UI exists for: create an instance, give it a
 * dependency from the catalog, apply. Every step ends on the repo the server
 * is serving — the browser can only prove the screen, the filesystem proves
 * the work.
 */

const INSTANCE = 'web-e2e-alpha';

test.describe.configure({ mode: 'serial' });

test.describe('instance lifecycle', () => {
  test('creates an instance from the dashboard', async ({ page, repo }) => {
    await page.goto('/');

    await page.getByRole('button', { name: /New instance/ }).click();
    await page.getByPlaceholder(/instance name/).fill(INSTANCE);
    await page.getByRole('button', { name: 'Create' }).click();

    // The app navigates to the new instance.
    await expect(page).toHaveURL(new RegExp(`/instances/${INSTANCE}$`));
    await expect(page.getByRole('heading', { name: new RegExp(INSTANCE) })).toBeVisible();

    // A managed umbrella chart landed on disk.
    expect(repoRead(repo, 'apps', INSTANCE, 'Chart.yaml')).toContain(`name: ${INSTANCE}`);
    expect(repoExists(repo, 'apps', INSTANCE, 'values.instance.yaml')).toBe(true);
  });

  test('adds a catalog dependency, pinned and attributed', async ({ page, repo }) => {
    // The wizard offers what the local catalog cache holds, and only the
    // Catalog page can refresh it.
    await page.goto('/catalog');
    await page.getByRole('button', { name: /Sync sources/ }).click();
    await expect(page.getByText('bitnami-nginx-15.0.0')).toBeVisible({ timeout: 30_000 });

    await page.goto(`/instances/${INSTANCE}/deps`);
    await expect(page.getByText('0 dependencies in Chart.yaml')).toBeVisible();

    await page.getByRole('button', { name: /Add dependency/ }).click();
    await page.getByText('bitnami-nginx-15.0.0').first().click();
    // Picking an entry fills the draft; it is submitted explicitly.
    await page.getByRole('button', { name: 'Add to Chart.yaml' }).click();

    await expect(page.getByText('1 dependency in Chart.yaml')).toBeVisible({ timeout: 30_000 });

    const chart = repoRead(repo, 'apps', INSTANCE, 'Chart.yaml');
    expect(chart).toContain('name: nginx');
    expect(chart).toContain('version: 15.0.0');
    // Attributed to its catalog, so detach and upgrade-from-catalog work.
    expect(repoRead(repo, '.helmdex', 'depmeta', INSTANCE, 'nginx.yaml')).toContain('kind: catalog');
  });

  test('applies the instance, locking and vendoring the dependency', async ({ page, repo }) => {
    await page.goto(`/instances/${INSTANCE}/deps`);
    await expect(page.getByText('1 dependency in Chart.yaml')).toBeVisible();

    await page.getByRole('button', { name: /^Apply$/ }).click();
    // The button re-enables too fast to be a reliable signal; the server's
    // event stream says when the apply actually finished.
    await expect(page.getByText(`apply.done · ${INSTANCE}`)).toBeVisible({ timeout: 60_000 });

    expect(repoExists(repo, 'apps', INSTANCE, 'Chart.lock')).toBe(true);
    expect(repoExists(repo, 'apps', INSTANCE, 'charts', 'nginx-15.0.0.tgz')).toBe(true);
    // Managed mode: apply regenerates the merged output.
    expect(repoExists(repo, 'apps', INSTANCE, 'values.yaml')).toBe(true);
  });

  test('edits the instance values layer and regenerates', async ({ page, repo }) => {
    await page.goto(`/instances/${INSTANCE}/files`);

    await page.getByText('values.instance.yaml').first().click();
    const editor = page.locator('textarea, .monaco-editor').first();
    await expect(editor).toBeVisible({ timeout: 30_000 });

    // Write through the API-backed file editor.
    await page.evaluate(
      async ([instance]) => {
        const res = await fetch(
          `/api/instances/${encodeURIComponent(instance)}/file?path=values.instance.yaml`,
          { method: 'PUT', body: 'replicaCount: 7\n' }
        );
        if (!res.ok) throw new Error(`write failed: ${res.status}`);
      },
      [INSTANCE]
    );

    expect(repoRead(repo, 'apps', INSTANCE, 'values.instance.yaml')).toContain('replicaCount: 7');
    // Writing a managed layer regenerates the merged output.
    expect(repoRead(repo, 'apps', INSTANCE, 'values.yaml')).toContain('replicaCount: 7');
  });

  test('deletes the instance', async ({ page, repo }) => {
    await page.goto('/');
    page.on('dialog', (d) => void d.accept());

    // The delete affordance only appears on hover over the card.
    await page.getByText(INSTANCE, { exact: true }).hover();
    await page.getByTitle(`Delete ${INSTANCE}`).click();

    await expect(page.getByText(INSTANCE)).toHaveCount(0, { timeout: 30_000 });
    expect(repoExists(repo, 'apps', INSTANCE)).toBe(false);
  });
});
