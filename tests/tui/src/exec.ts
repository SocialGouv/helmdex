import { spawn } from 'node:child_process';

export type ExecaOptions = {
  cwd?: string;
  env?: NodeJS.ProcessEnv;
  stdio?: 'inherit' | 'pipe';
};

/** Minimal `execa`-like helper (avoid extra deps). */
export function execa(cmd: string, args: string[], opts: ExecaOptions = {}): Promise<{ exitCode: number }> {
  return new Promise((resolve, reject) => {
    const child = spawn(cmd, args, {
      cwd: opts.cwd,
      env: opts.env,
      stdio: opts.stdio ?? 'pipe'
    });
    child.on('error', reject);
    child.on('close', (code) => {
      if (code === 0) return resolve({ exitCode: 0 });
      reject(new Error(`${cmd} ${args.join(' ')} exited with code ${code ?? 'null'}`));
    });
  });
}

