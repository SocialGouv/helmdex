import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

/**
 * Throwaway helmdex workspaces shared by the TUI and web E2E harnesses.
 *
 * The Go suites build the same shapes through internal/testutil; keeping the
 * two in step matters less than keeping each honest, so this module stays
 * deliberately small: a repo on disk, and the paths needed to assert on it.
 */

export type TempRepo = {
  /** Repo root passed to helmdex as --repo. */
  dir: string;
  /** helmdex.yaml path; empty string for agnostic repos. */
  configPath: string;
  /** Instances directory, relative to dir. */
  appsDir: string;
  /**
   * Where helmdex keeps state for a repo that did not opt in. It sits beside
   * the repo, never inside it: an agnostic repo must stay byte-identical, and
   * tests compare its whole tree.
   */
  stateDir: string;
};

export type RepoFixtureOptions = {
  /**
   * Seed from fixtures/agnostic-gitops and write no helmdex.yaml, so helmdex
   * must adapt to a repo that never opted in.
   */
  agnostic?: boolean;
  /** Presets layer to import (values.platform.<name>.yaml). Defaults to eks. */
  platform?: string;
};

export function repoRoot(): string {
  // Harnesses run from the repo root (vitest/playwright are launched there).
  return path.resolve(process.cwd());
}

export function fixturePath(...parts: string[]): string {
  return path.join(repoRoot(), 'fixtures', ...parts);
}

export async function createTempHelmdexRepo(opts: RepoFixtureOptions = {}): Promise<TempRepo> {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'helmdex-e2e-'));
  const dir = path.join(root, 'repo');
  const stateDir = path.join(root, 'state');
  await fs.mkdir(dir, { recursive: true });
  await fs.mkdir(stateDir, { recursive: true });

  if (opts.agnostic) {
    await copyDir(fixturePath('agnostic-gitops'), dir);
    return { dir, configPath: '', appsDir: 'apps', stateDir };
  }

  await fs.mkdir(path.join(dir, 'apps'), { recursive: true });
  const configPath = path.join(dir, 'helmdex.yaml');
  const yaml = [
    'apiVersion: helmdex.io/v1alpha1',
    'kind: HelmdexConfig',
    'repo:',
    '  appsDir: apps',
    'platform:',
    `  name: ${opts.platform ?? 'eks'}`,
    'sources:',
    '  - name: Example',
    '    git:',
    `      url: ${fixturePath('remote-source')}`,
    '    presets:',
    '      enabled: true',
    '      chartsPath: charts',
    '    catalog:',
    '      enabled: true',
    '      path: catalog.yaml',
    'artifactHub:',
    '  enabled: false',
    ''
  ].join('\n');

  await fs.writeFile(configPath, yaml, 'utf8');
  return { dir, configPath, appsDir: 'apps', stateDir };
}

export async function rmTempRepo(repo: TempRepo): Promise<void> {
  // Both live under the same mkdtemp root.
  await fs.rm(path.dirname(repo.dir), { recursive: true, force: true });
}

/**
 * Environment that keeps helmdex state and config out of the developer's
 * home. An agnostic repo resolves both from there, so without this a suite
 * writes to the real ~/.cache and reads the developer's own config.
 */
export function isolatedStateEnv(repo: TempRepo): Record<string, string> {
  return {
    HELMDEX_CACHE_DIR: repo.stateDir,
    HELMDEX_USER_CONFIG: path.join(repo.stateDir, 'absent-user-config.yaml')
  };
}

/** Absolute path of a repo-relative file. */
export function repoPath(repo: TempRepo, ...parts: string[]): string {
  return path.join(repo.dir, ...parts);
}

/** Absolute path of an instance directory. */
export function instancePath(repo: TempRepo, name: string): string {
  return path.join(repo.dir, repo.appsDir, name);
}

async function copyDir(src: string, dst: string): Promise<void> {
  await fs.cp(src, dst, { recursive: true });
}
