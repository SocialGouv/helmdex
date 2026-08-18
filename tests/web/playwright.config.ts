import { defineConfig, devices } from '@playwright/test';
import path from 'node:path';
import os from 'node:os';

/**
 * Browser E2E against the real binary.
 *
 * This layer exists for what the jsdom tests structurally cannot see: the
 * embedded SPA, its routing, and the actual contract with the Go server. It
 * stays deliberately small — three critical journeys — because everything
 * cheaper to test is tested more cheaply elsewhere.
 */

const port = process.env.WEB_E2E_PORT ?? '8119';
const runDir = path.join(os.tmpdir(), 'helmdex-web-e2e');

export default defineConfig({
  testDir: './specs',
  fullyParallel: false,
  workers: 1,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure'
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'corepack pnpm exec tsx tests/web/scripts/serve.ts',
    url: `http://127.0.0.1:${port}/api/repo`,
    cwd: path.resolve(__dirname, '../..'),
    reuseExistingServer: false,
    timeout: 60_000,
    stdout: 'pipe',
    stderr: 'pipe',
    env: {
      WEB_E2E_PORT: port,
      WEB_E2E_REPO_FILE: path.join(runDir, 'repo.json'),
      WEB_E2E_HELM_LOG: path.join(runDir, 'fakehelm.log')
    }
  }
});
