import { defineConfig } from 'vitest/config';

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

