import fs from 'node:fs/promises';
import path from 'node:path';
import { instancePath, repoPath, type TempRepo } from './repoFixture';

/**
 * Filesystem assertions for the E2E harnesses.
 *
 * Driving a UI proves a screen renders; only the repo on disk proves the
 * action did anything. Every mutating scenario should end here.
 */

export async function readRepoFile(repo: TempRepo, ...parts: string[]): Promise<string> {
  const p = repoPath(repo, ...parts);
  try {
    return await fs.readFile(p, 'utf8');
  } catch (e) {
    throw new Error(`Expected file ${p} to exist and be readable: ${(e as Error).message}`);
  }
}

export async function fileExists(repo: TempRepo, ...parts: string[]): Promise<boolean> {
  try {
    await fs.stat(repoPath(repo, ...parts));
    return true;
  } catch {
    return false;
  }
}

export async function listInstanceFiles(repo: TempRepo, instance: string): Promise<string[]> {
  const dir = instancePath(repo, instance);
  try {
    const entries = await fs.readdir(dir, { withFileTypes: true });
    return entries.map((e) => (e.isDirectory() ? `${e.name}/` : e.name)).sort();
  } catch {
    return [];
  }
}

export async function expectFileExists(repo: TempRepo, ...parts: string[]): Promise<void> {
  if (!(await fileExists(repo, ...parts))) {
    throw new Error(`Expected ${path.join(...parts)} to exist in ${repo.dir}`);
  }
}

export async function expectFileMissing(repo: TempRepo, ...parts: string[]): Promise<void> {
  if (await fileExists(repo, ...parts)) {
    throw new Error(`Expected ${path.join(...parts)} NOT to exist in ${repo.dir}`);
  }
}

export async function expectFileContains(
  repo: TempRepo,
  parts: string[],
  needles: string | string[]
): Promise<void> {
  const content = await readRepoFile(repo, ...parts);
  for (const needle of Array.isArray(needles) ? needles : [needles]) {
    if (!content.includes(needle)) {
      throw new Error(
        `Expected ${path.join(...parts)} to contain ${JSON.stringify(needle)}.\n--- FILE ---\n${content}\n--- END ---`
      );
    }
  }
}

export async function expectFileOmits(
  repo: TempRepo,
  parts: string[],
  needle: string
): Promise<void> {
  const content = await readRepoFile(repo, ...parts);
  if (content.includes(needle)) {
    throw new Error(
      `Expected ${path.join(...parts)} NOT to contain ${JSON.stringify(needle)}.\n--- FILE ---\n${content}\n--- END ---`
    );
  }
}

/** Asserts the instance's Chart.yaml declares exactly these dependency names. */
export async function expectChartDeps(
  repo: TempRepo,
  instance: string,
  wantNames: string[]
): Promise<void> {
  const chart = await readRepoFile(repo, repo.appsDir, instance, 'Chart.yaml');
  const got = [...chart.matchAll(/^\s*-?\s*name:\s*(\S+)\s*$/gm)]
    .map((m) => m[1])
    .filter((n) => n !== instance)
    .sort();
  const want = [...wantNames].sort();
  if (got.join(',') !== want.join(',')) {
    throw new Error(
      `Expected dependencies [${want}] in ${instance}/Chart.yaml, got [${got}].\n--- FILE ---\n${chart}\n--- END ---`
    );
  }
}

/**
 * Snapshots every file under a directory, to prove a flow left user-owned
 * files byte-identical.
 */
export async function snapshotTree(root: string): Promise<Map<string, string>> {
  const out = new Map<string, string>();
  async function walk(dir: string): Promise<void> {
    const entries = await fs.readdir(dir, { withFileTypes: true });
    for (const e of entries) {
      const full = path.join(dir, e.name);
      if (e.isDirectory()) {
        await walk(full);
        continue;
      }
      out.set(path.relative(root, full), await fs.readFile(full, 'utf8'));
    }
  }
  await walk(root);
  return out;
}

export function expectTreeUnchanged(before: Map<string, string>, after: Map<string, string>): void {
  const changed: string[] = [];
  for (const [rel, content] of before) {
    if (!after.has(rel)) {
      changed.push(`removed: ${rel}`);
    } else if (after.get(rel) !== content) {
      changed.push(`modified: ${rel}`);
    }
  }
  for (const rel of after.keys()) {
    if (!before.has(rel)) changed.push(`added: ${rel}`);
  }
  if (changed.length > 0) {
    throw new Error(`Expected the tree to be untouched, but:\n  ${changed.join('\n  ')}`);
  }
}
