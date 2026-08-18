import { test, expect, listInstances } from './fixtures';

test.describe('web UI smoke', () => {
  test('the embedded SPA loads and reports the served repo', async ({ page, repo }) => {
    await page.goto('/');

    await expect(page.getByRole('heading', { name: 'Instances' })).toBeVisible();
    // The sidebar echoes the repo the server was pointed at.
    await expect(page.getByTitle(repo.dir)).toBeVisible();
    await expect(page.getByText(/helmdex repo · apps\//)).toBeVisible();

    // No console errors while loading the shipped bundle.
    const errors: string[] = [];
    page.on('console', (m) => m.type() === 'error' && errors.push(m.text()));
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Instances' })).toBeVisible();
    expect(errors).toEqual([]);
  });

  test('client-side routes survive a full page load', async ({ page }) => {
    // Deep-linking hits the Go server first: the SPA fallback has to serve
    // index.html for a route that is not a file.
    await page.goto('/catalog');
    await expect(page.getByRole('heading', { name: 'Catalog' })).toBeVisible();

    await page.goto('/instances/does-not-exist');
    await expect(page.getByText(/not found/i)).toBeVisible();
  });

  test('the dashboard reflects the repo on disk', async ({ page, repo }) => {
    await page.goto('/');
    await expect(page.getByRole('heading', { name: 'Instances' })).toBeVisible();

    // Whatever other specs left behind, the listing and the apps dir agree.
    const onDisk = listInstances(repo);
    if (onDisk.length === 0) {
      await expect(page.getByText(/No instances yet/)).toBeVisible();
      return;
    }
    for (const name of onDisk) {
      await expect(page.getByText(name, { exact: true })).toBeVisible();
    }
  });
});
