import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

export type TempRepo = {
  dir: string;
  configPath: string;
};

export async function createTempHelmdexRepo(): Promise<TempRepo> {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'helmdex-tui-'));
  const configPath = path.join(dir, 'helmdex.yaml');

  const fixture = path.resolve(process.cwd(), 'fixtures/remote-source');

  const yaml = [
    'apiVersion: helmdex.io/v1alpha1',
    'kind: HelmdexConfig',
    'repo:',
    '  appsDir: apps',
    'platform:',
    '  name: test',
    'sources:',
    '  - name: Example',
    '    git:',
    `      url: ${fixture}`,
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
  return { dir, configPath };
}

export async function rmTempRepo(repo: TempRepo): Promise<void> {
  await fs.rm(repo.dir, { recursive: true, force: true });
}

