import { afterEach, describe, it } from 'vitest';
import { createTempHelmdexRepo, rmTempRepo } from '../src/repoHarness';
import { startHelmdexTui, writeArtifact } from '../src/sessionHarness';
import { expectIncludes } from '../src/assertions';

let cleanup: Array<() => Promise<void>> = [];
afterEach(async () => {
  for (const fn of cleanup.reverse()) {
    try {
      await fn();
    } catch {
      // ignore
    }
  }
  cleanup = [];
});

describe('TUI user input', () => {
  it('creates an instance and renames it', async () => {
    const repo = await createTempHelmdexRepo();
    cleanup.push(() => rmTempRepo(repo));

    const h = await startHelmdexTui(repo);
    cleanup.push(() => h.kill());

    // Create instance.
    await h.press(['n']);
    await h.waitStable(30_000);
    await h.type('alpha');
    await h.press(['Enter']);
    await h.waitStable(30_000);
    let screen = await h.screenshotText();
    if (!screen.includes('alpha')) await writeArtifact('user-input-after-create.txt', screen);
    expectIncludes(screen, 'alpha', 'top bar should include newly created instance');

    // Deps -> Values -> Instance.
    await h.press(['ArrowRight', 'ArrowRight']);
    await h.waitStable(30_000);
    screen = await h.screenshotText();
    expectIncludes(screen, 'r: rename', 'instance tab should show rename hint');

    // Rename.
    await h.press(['r']);
    await h.waitStable(30_000);
    // Input is pre-filled with current name, clear with backspaces (short names).
    await h.press(['Backspace', 'Backspace', 'Backspace', 'Backspace', 'Backspace']);
    await h.type('beta');
    await h.press(['Enter']);
    await h.waitStable(30_000);
    screen = await h.screenshotText();
    if (!screen.includes('beta')) await writeArtifact('user-input-after-rename.txt', screen);
    expectIncludes(screen, 'Name: beta', 'instance details should show renamed instance');
  });
});

