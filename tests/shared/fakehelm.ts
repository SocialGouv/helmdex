import fs from 'node:fs';
import path from 'node:path';
import { repoRoot } from './repoFixture';

/**
 * The fake Helm binary (internal/testutil/fakehelm) makes Helm-dependent
 * flows hermetic: no network, no registry, no Helm install.
 *
 * Two ways to reach it, and the difference matters:
 *
 * - HELMDEX_NO_BUNDLED_HELM=1 makes helmdex resolve Helm through PATH. The
 *   real code paths run, against the fake registry.
 * - HELMDEX_E2E_STUB_HELM=1 does the same for helmutil, but *also* turns on
 *   the TUI's own bypasses (internal/tui/e2e_env.go), which replace
 *   Helm-dependent results with placeholders. Useful to reach a screen
 *   quickly, useless to prove the screen is right.
 *
 * Prefer the first.
 */

export function fakeHelmDir(): string {
  const dir = path.join(repoRoot(), 'bin', 'fakehelm');
  const bin = path.join(dir, 'helm');
  if (!fs.existsSync(bin)) {
    throw new Error(
      `fake helm not built at ${bin} — run \`task test:fakehelm\` (the E2E task targets do it for you)`
    );
  }
  return dir;
}

/** PATH with the fake Helm first. */
export function pathWithFakeHelm(): string {
  return `${fakeHelmDir()}${path.delimiter}${process.env.PATH ?? ''}`;
}

export type FakeHelmOptions = {
  /** Append one JSON line per Helm invocation to this file. */
  logPath?: string;
  /** Delay every matching Helm call, to make async UI states observable. */
  delayMs?: number;
  /** Restrict delay/block to calls whose arguments start with this prefix. */
  delayCommandPrefix?: string;
  /**
   * Hold matching calls open until this file exists. Prefer it over delayMs
   * when a test needs to act while an operation is in flight: the test
   * decides when it ends, so there is no timer to lose a race against.
   */
  blockUntilPath?: string;
};

/**
 * Environment for a helmdex process that must run real Helm code paths
 * against the fake registry.
 */
export function realHelmEnv(opts: FakeHelmOptions = {}): Record<string, string> {
  const env: Record<string, string> = {
    PATH: pathWithFakeHelm(),
    HELMDEX_NO_BUNDLED_HELM: '1',
    // Explicitly off: specs share a process, and a sibling spec that set
    // these would otherwise silently put this one back on placeholders.
    HELMDEX_E2E_STUB_HELM: '',
    HELMDEX_E2E_STUB_ARTIFACTHUB: ''
  };
  if (opts.logPath) env.HELMDEX_FAKE_HELM_LOG = opts.logPath;
  if (opts.delayMs !== undefined) env.HELMDEX_FAKE_HELM_DELAY_MS = String(opts.delayMs);
  if (opts.delayCommandPrefix) env.HELMDEX_FAKE_HELM_DELAY_CMD = opts.delayCommandPrefix;
  if (opts.blockUntilPath) env.HELMDEX_FAKE_HELM_BLOCK_UNTIL = opts.blockUntilPath;
  return env;
}

/**
 * Waits until a Helm call whose arguments start with `prefix` has been
 * recorded. Use this to synchronise on work actually reaching Helm, rather
 * than on the screen that announces it.
 */
export async function waitForHelmCall(
  logPath: string,
  prefix: string,
  timeoutMs = 30_000
): Promise<string[]> {
  const deadline = Date.now() + timeoutMs;
  let calls: string[] = [];
  while (Date.now() < deadline) {
    calls = fakeHelmCalls(logPath);
    if (calls.some((c) => c.startsWith(prefix))) return calls;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(
    `Timed out waiting for a helm call starting with ${JSON.stringify(prefix)}; recorded: ${JSON.stringify(calls)}`
  );
}

/** Helm invocations recorded so far, each as its joined argument list. */
export function fakeHelmCalls(logPath: string): string[] {
  if (!fs.existsSync(logPath)) return [];
  return fs
    .readFileSync(logPath, 'utf8')
    .split('\n')
    .filter((l) => l.trim() !== '')
    .map((l) => (JSON.parse(l) as { args: string[] }).args.join(' '));
}
