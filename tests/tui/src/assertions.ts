import stripAnsi from 'strip-ansi';

export function normalizeScreenText(s: string): string {
  let out = stripAnsi(s ?? '');
  out = out.replace(/\r\n/g, '\n');
  out = out
    .split('\n')
    .map((l) => l.replace(/[ \t]+$/g, ''))
    .join('\n');
  return out.trimEnd();
}

export function expectIncludes(screen: string, needle: string, hint?: string): void {
  if (!screen.includes(needle)) {
    const msg = hint ? `\nHint: ${hint}` : '';
    throw new Error(`Expected screen to include ${JSON.stringify(needle)} but it did not.${msg}\n--- SCREEN ---\n${screen}\n--- END ---`);
  }
}

