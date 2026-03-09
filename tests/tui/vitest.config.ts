import { defineConfig } from 'vitest/config';
import { createRequire } from 'node:module';
import path from 'node:path';

// agent-tui daemon defaults to /run/user/<uid>/agent-tui.sock, which may not
// be accessible from Node.js child processes (e.g. fnm-managed Node).
// Override to a universally writable location.
process.env.AGENT_TUI_SOCKET ??= '/tmp/agent-tui-test.sock';

// The Node.js wrapper (./node_modules/.bin/agent-tui) spawns the native binary
// via spawnSync, but the forked daemon inherits a restricted process context
// that cannot open PTYs. Point directly at the native binary to avoid this.
if (!process.env.AGENT_TUI_BIN) {
  const req = createRequire(import.meta.url);
  const platform = process.platform === 'darwin' ? 'darwin' : 'linux';
  const pkgJson = req.resolve(`agent-tui-${platform}-${process.arch}/package.json`);
  process.env.AGENT_TUI_BIN = path.join(path.dirname(pkgJson), 'bin', 'agent-tui');
}

export default defineConfig({
  test: {
    include: ['tests/tui/specs/**/*.spec.ts'],
    testTimeout: 60_000,
    hookTimeout: 60_000,
    // Run sequentially: a single daemon + single PTY session at a time.
    fileParallelism: false,
    sequence: { concurrent: false }
  }
});

