import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { test as base, expect } from '@playwright/test';
import type { TempRepo } from '../../shared/repoFixture';

/**
 * Gives every spec the repo the server is actually serving, so a browser
 * journey can end on an assertion about files on disk.
 */

const runDir = path.join(os.tmpdir(), 'helmdex-web-e2e');

export function servedRepo(): TempRepo {
  const file = path.join(runDir, 'repo.json');
  if (!fs.existsSync(file)) {
    throw new Error(`served repo descriptor missing at ${file} — is the web server running?`);
  }
  return JSON.parse(fs.readFileSync(file, 'utf8')) as TempRepo;
}

export function repoRead(repo: TempRepo, ...parts: string[]): string {
  return fs.readFileSync(path.join(repo.dir, ...parts), 'utf8');
}

export function repoExists(repo: TempRepo, ...parts: string[]): boolean {
  return fs.existsSync(path.join(repo.dir, ...parts));
}

/** Instance directories currently on disk, i.e. what the API should list. */
export function listInstances(repo: TempRepo): string[] {
  const dir = path.join(repo.dir, repo.appsDir);
  if (!fs.existsSync(dir)) return [];
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .filter((e) => e.isDirectory() && fs.existsSync(path.join(dir, e.name, 'Chart.yaml')))
    .map((e) => e.name)
    .sort();
}

export const test = base.extend<{ repo: TempRepo }>({
  repo: async ({}, use) => {
    await use(servedRepo());
  }
});

export { expect };
