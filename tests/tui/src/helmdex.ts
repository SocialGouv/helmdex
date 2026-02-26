import path from 'node:path';
import process from 'node:process';

export function resolveHelmdexBin(): string {
  // Prefer a prebuilt binary (CI/build step), fallback to `go run` is too slow/flaky.
  return process.env.HELMDEX_BIN ?? path.resolve(process.cwd(), 'bin', 'helmdex');
}

export function testEnvForTui(): NodeJS.ProcessEnv {
  return {
    HELMDEX_NO_TITLE: '1',
    HELMDEX_NO_ICONS: '1',
    HELMDEX_NO_LOGO: '1',
    NO_COLOR: '1',

    // E2E toggles (opt-in; tests can override via process.env before launch)
    HELMDEX_E2E_STUB_HELM: process.env.HELMDEX_E2E_STUB_HELM,
    HELMDEX_E2E_STUB_ARTIFACTHUB: process.env.HELMDEX_E2E_STUB_ARTIFACTHUB,
    HELMDEX_E2E_NO_EDITOR: process.env.HELMDEX_E2E_NO_EDITOR
  };
}

export function testEnvArgsForTui(): string[] {
  const env = testEnvForTui();
  // env(1) format: KEY=VALUE
  return Object.entries(env)
    .filter(([, v]) => typeof v === 'string')
    .map(([k, v]) => `${k}=${v}`);
}
