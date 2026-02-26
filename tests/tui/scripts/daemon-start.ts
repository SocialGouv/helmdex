/**
 * Starts the agent-tui daemon.
 *
 * This script is intentionally idempotent:
 * - `agent-tui daemon start` is safe to call multiple times
 */
import { spawn } from 'node:child_process';
import process from 'node:process';
import { resolveAgentTuiBin } from '../src/agentTui/daemon';

async function main() {
  const bin = resolveAgentTuiBin();

  const child = spawn(bin, ['daemon', 'start', '--json'], {
    stdio: ['ignore', 'inherit', 'inherit'],
    env: process.env
  });
  const code: number = await new Promise((r) => child.on('close', (c) => r(c ?? 1)));
  if (code !== 0) throw new Error(`agent-tui daemon start failed with code ${code}`);
  process.stdout.write('agent-tui daemon started\n');
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
