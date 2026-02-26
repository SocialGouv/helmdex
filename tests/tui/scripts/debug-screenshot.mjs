import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const bin = './node_modules/.bin/agent-tui';

spawnSync(bin, ['daemon', 'start'], { stdio: 'inherit' });

const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'helmdex-tui-'));
const cfg = path.join(dir, 'helmdex.yaml');
const fixture = path.resolve(process.cwd(), 'fixtures/remote-source');

fs.writeFileSync(
  cfg,
  [
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
  ].join('\n'),
  'utf8'
);

const env = {
  ...process.env,
  HELMDEX_NO_TITLE: '1',
  HELMDEX_NO_ICONS: '1',
  HELMDEX_NO_LOGO: '1',
  NO_COLOR: '1'
};

const helmdex = path.resolve(process.cwd(), 'bin', 'helmdex');
const run = spawnSync(
  bin,
  ['run', '--json', '--cols', '120', '--rows', '40', helmdex, '--', '--repo', dir, '--config', cfg, 'tui'],
  { encoding: 'utf8', env }
);

console.log('run status', run.status);
console.log('run stdout', run.stdout);
console.log('run stderr', run.stderr);
if (run.status !== 0) process.exit(run.status ?? 1);

const sid = JSON.parse(run.stdout).session_id;
console.log('SID', sid);

for (const args of [
  ['screenshot', '--format', 'text', '-s', sid],
  ['screenshot', '--json', '-s', sid],
  ['screenshot', '--json', '--strip-ansi', '-s', sid],
  ['screenshot', '--json', '--include-cursor', '-s', sid]
]) {
  const ss = spawnSync(bin, args, { encoding: 'utf8' });
  console.log('---', args.join(' '), 'exit', ss.status);
  console.log('stdout head', JSON.stringify(ss.stdout.slice(0, 200)));
  console.log('stderr head', JSON.stringify(ss.stderr.slice(0, 200)));
}

spawnSync(bin, ['kill', '-s', sid, '--json'], { encoding: 'utf8' });
fs.rmSync(dir, { recursive: true, force: true });

