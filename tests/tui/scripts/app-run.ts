/**
 * Launches `helmdex tui` under the test environment.
 *
 * This is primarily used by the E2E harness to centralize env flags.
 */
import process from 'node:process';
import { execa } from '../src/exec';
import { resolveHelmdexBin, testEnvForTui } from '../src/helmdex';

async function main() {
  const repoRoot = process.env.HELMDEX_TEST_REPO;
  if (!repoRoot) {
    throw new Error('HELMDEX_TEST_REPO is required');
  }
  const cfgPath = process.env.HELMDEX_TEST_CONFIG ?? `${repoRoot}/helmdex.yaml`;

  const helmdex = resolveHelmdexBin();
  await execa(
    helmdex,
    ['--repo', repoRoot, '--config', cfgPath, 'tui'],
    {
      stdio: 'inherit',
      env: { ...process.env, ...testEnvForTui() }
    }
  );
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});

