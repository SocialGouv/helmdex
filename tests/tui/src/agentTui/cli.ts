import process from 'node:process';
import { execa } from '../exec';

export type AgentTuiCli = {
  bin: string;
  daemonStart(): Promise<void>;
  daemonStop(): Promise<void>;
  health(): Promise<any>;
  run(cmd: string, args: string[], opts?: { cwd?: string; cols?: number; rows?: number; env?: NodeJS.ProcessEnv }): Promise<string>;
  screenshot(sessionId: string, opts?: { json?: boolean; stripAnsi?: boolean }): Promise<any>;
  press(sessionId: string, keys: string[]): Promise<void>;
  type(sessionId: string, text: string): Promise<void>;
  waitText(sessionId: string, text: string, timeoutMs?: number): Promise<void>;
  waitStable(sessionId: string, timeoutMs?: number): Promise<void>;
  kill(sessionId: string): Promise<void>;
};

function defaultBin(): string {
  return process.env.AGENT_TUI_BIN ?? './node_modules/.bin/agent-tui';
}

export function agentTuiCli(): AgentTuiCli {
  const bin = defaultBin();

  return {
    bin,
    async daemonStart() {
      // NOTE: `daemon start` prints a human message even with --json.
      // So we don't parse JSON here; we just run it and ignore stdout.
      await execa(bin, ['daemon', 'start', '--json'], { stdio: 'pipe' });
    },
    async daemonStop() {
      // Idempotent.
      try {
        await execa(bin, ['daemon', 'stop', '--json'], { stdio: 'pipe' });
      } catch {
        // ignore
      }
    },
    async health() {
      return await captureJson(bin, ['health', '--json']);
    },
    async run(cmd: string, args: string[], opts?: { cwd?: string; cols?: number; rows?: number; env?: NodeJS.ProcessEnv }) {
      const cols = opts?.cols ?? 120;
      const rows = opts?.rows ?? 40;
      const payload = await captureJson(
        bin,
        ['run', '--json', '--cols', String(cols), '--rows', String(rows), cmd, '--', ...args],
        { cwd: opts?.cwd, env: opts?.env }
      );
      if (!payload?.session_id) throw new Error(`agent-tui run did not return session_id: ${JSON.stringify(payload)}`);
      return String(payload.session_id);
    },
    async screenshot(sessionId: string, opts?: { json?: boolean; stripAnsi?: boolean }) {
      const args = ['screenshot', '-s', sessionId];
      if (opts?.stripAnsi) args.push('--strip-ansi');
      // `--json` is a shorthand; it does not take an argument.
      if (opts?.json ?? true) args.push('--json');
      return await captureJson(bin, args);
    },
    async press(sessionId: string, keys: string[]) {
      await execa(bin, ['press', '--json', '-s', sessionId, ...keys], { stdio: 'pipe' });
    },
    async type(sessionId: string, text: string) {
      await execa(bin, ['type', '--json', '-s', sessionId, text], { stdio: 'pipe' });
    },
    async waitText(sessionId: string, text: string, timeoutMs = 30_000) {
      await execa(bin, ['wait', '--assert', '--json', '-s', sessionId, '-t', String(timeoutMs), text], { stdio: 'pipe' });
    },
    async waitStable(sessionId: string, timeoutMs = 30_000) {
      await execa(bin, ['wait', '--assert', '--json', '-s', sessionId, '-t', String(timeoutMs), '--stable'], { stdio: 'pipe' });
    },
    async kill(sessionId: string) {
      try {
        await execa(bin, ['kill', '--json', '-s', sessionId], { stdio: 'pipe' });
      } catch {
        // ignore
      }
    }
  };
}

async function captureJson(cmd: string, args: string[], opts?: { cwd?: string; env?: NodeJS.ProcessEnv }): Promise<any> {
  const { spawn } = await import('node:child_process');
  return await new Promise((resolve, reject) => {
    const child = spawn(cmd, args, {
      cwd: opts?.cwd,
      env: { ...process.env, ...(opts?.env ?? {}) },
      stdio: ['ignore', 'pipe', 'pipe']
    });
    let out = '';
    let err = '';
    child.stdout.on('data', (d) => (out += String(d)));
    child.stderr.on('data', (d) => (err += String(d)));
    child.on('error', reject);
    child.on('close', (code) => {
      if (code !== 0) return reject(new Error(`${cmd} ${args.join(' ')} exited ${code}: ${err.trim()}`));
      const trimmed = out.trim();
      // agent-tui sometimes prints multi-line pretty JSON, and sometimes
      // prints extra human messages even in --json mode.
      // Strategy: extract the first full JSON value (object/array) from stdout.
      try {
        resolve(parseFirstJsonValue(trimmed));
      } catch (e) {
        reject(new Error(`Failed to parse JSON from ${cmd} ${args.join(' ')}: ${String(e)}\nstdout=${trimmed}\nstderr=${err.trim()}`));
      }
    });
  });
}

function parseFirstJsonValue(s: string): any {
  const txt = (s ?? '').trim();
  if (!txt) return {};

  // Fast path: whole stdout is JSON.
  try {
    return JSON.parse(txt);
  } catch {
    // continue
  }

  const start = txt.search(/[\[{]/);
  if (start < 0) throw new Error('no JSON object/array found in stdout');
  const extracted = extractBalancedJson(txt.slice(start));
  return JSON.parse(extracted);
}

function extractBalancedJson(s: string): string {
  // Extract a complete JSON object/array from the start of `s`, including nested
  // structures, ignoring braces inside strings.
  const stack: string[] = [];
  let inString = false;
  let escape = false;

  for (let i = 0; i < s.length; i++) {
    const ch = s[i] ?? '';

    if (inString) {
      if (escape) {
        escape = false;
        continue;
      }
      if (ch === '\\') {
        escape = true;
        continue;
      }
      if (ch === '"') {
        inString = false;
      }
      continue;
    }

    if (ch === '"') {
      inString = true;
      continue;
    }

    if (ch === '{') {
      stack.push('}');
      continue;
    }
    if (ch === '[') {
      stack.push(']');
      continue;
    }

    const want = stack.at(-1);
    if (want && ch === want) {
      stack.pop();
      if (stack.length === 0) {
        return s.slice(0, i + 1);
      }
    }
  }

  throw new Error('unterminated JSON value in stdout');
}
