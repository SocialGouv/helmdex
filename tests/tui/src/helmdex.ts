import path from 'node:path';
import process from 'node:process';

export function resolveHelmdexBin(): string {
  // Prefer a prebuilt binary (CI/build step), fallback to `go run` is too slow/flaky.
  return process.env.HELMDEX_BIN ?? path.resolve(process.cwd(), 'bin', 'helmdex');
}

/**
 * Base environment for a TUI under test: no visual noise, and no access to
 * the developer's own helmdex state.
 */
export function testEnvForTui(overrides: Record<string, string> = {}): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = {
    HELMDEX_NO_TITLE: '1',
    HELMDEX_NO_ICONS: '1',
    HELMDEX_NO_LOGO: '1',
    NO_COLOR: '1',

    // E2E toggles (opt-in; tests can override via process.env before launch).
    // HELMDEX_E2E_STUB_HELM also enables the TUI's own bypasses — prefer the
    // fake Helm binary (see tests/shared/fakehelm.ts) when the assertion is
    // about what the flow actually did.
    HELMDEX_E2E_STUB_HELM: process.env.HELMDEX_E2E_STUB_HELM,
    HELMDEX_E2E_STUB_ARTIFACTHUB: process.env.HELMDEX_E2E_STUB_ARTIFACTHUB,
    HELMDEX_E2E_NO_EDITOR: process.env.HELMDEX_E2E_NO_EDITOR
  };
  return { ...env, ...overrides };
}

export function testEnvArgsForTui(overrides: Record<string, string> = {}): string[] {
  const env = testEnvForTui(overrides);
  // env(1) format: KEY=VALUE
  return Object.entries(env)
    .filter(([, v]) => typeof v === 'string')
    .map(([k, v]) => `${k}=${v}`);
}
