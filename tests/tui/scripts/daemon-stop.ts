/**
 * Stops agent-tui daemon.
 *
 * Idempotent: if not running, exits 0.
 */
import process from 'node:process';
import { resolveAgentTuiBin } from '../src/agentTui/daemon';
import { execa } from '../src/exec';

async function main() {
  const bin = resolveAgentTuiBin();

  try {
    await execa(bin, ['daemon', 'stop', '--json'], { stdio: 'inherit' });
  } catch {
    // ignore
  }
  process.stdout.write('agent-tui daemon stop requested\n');
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
