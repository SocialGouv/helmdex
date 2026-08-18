import { spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { createTempHelmdexRepo, isolatedStateEnv } from '../../shared/repoFixture';
import { realHelmEnv } from '../../shared/fakehelm';

/**
 * Launches `helmdex ui` on a throwaway repo for the Playwright suite.
 *
 * It serves the *embedded* SPA from the built binary, not the Vite dev
 * server: that is what ships, and the point of this layer is to catch what
 * only breaks once front and back are the same artefact.
 *
 * The repo path is written to WEB_E2E_REPO_FILE so specs can assert on the
 * files the UI wrote.
 */

async function main(): Promise<void> {
  const port = process.env.WEB_E2E_PORT ?? '8119';
  const repoFile = process.env.WEB_E2E_REPO_FILE;
  if (!repoFile) throw new Error('WEB_E2E_REPO_FILE is required');

  const repo = await createTempHelmdexRepo({ agnostic: process.env.WEB_E2E_AGNOSTIC === '1' });
  fs.mkdirSync(path.dirname(repoFile), { recursive: true });
  fs.writeFileSync(repoFile, JSON.stringify(repo), 'utf8');

  const bin = process.env.HELMDEX_BIN ?? path.resolve(process.cwd(), 'bin', 'helmdex');
  if (!fs.existsSync(bin)) {
    throw new Error(`helmdex binary not found at ${bin} — run \`task build\``);
  }

  const args = ['--repo', repo.dir];
  if (repo.configPath) args.push('--config', repo.configPath);
  args.push('ui', '--port', port, '--host', '127.0.0.1');

  const child = spawn(bin, args, {
    stdio: 'inherit',
    env: {
      ...process.env,
      ...realHelmEnv({ logPath: process.env.WEB_E2E_HELM_LOG }),
      ...isolatedStateEnv(repo)
    }
  });

  const stop = () => child.kill('SIGTERM');
  process.on('SIGTERM', stop);
  process.on('SIGINT', stop);
  child.on('exit', (code) => process.exit(code ?? 0));
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
