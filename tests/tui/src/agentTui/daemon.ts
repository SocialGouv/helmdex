import process from 'node:process';

export function resolveAgentTuiBin(): string {
  // Allow overriding, but default to local node_modules bin.
  return process.env.AGENT_TUI_BIN ?? './node_modules/.bin/agent-tui';
}

export async function waitForDaemonReady(daemonUrl: string, timeoutMs: number): Promise<boolean> {
  const start = Date.now();
  // Probe a health endpoint if available; otherwise just open TCP via fetch.
  // NOTE: endpoint is agent-tui-specific; keep centralized.
  const url = daemonUrl.replace(/\/$/, '') + '/health';
  // eslint-disable-next-line no-constant-condition
  while (true) {
    try {
      const res = await fetch(url, { method: 'GET' });
      if (res.ok) return true;
    } catch {
      // ignore
    }
    if (Date.now() - start > timeoutMs) return false;
    await new Promise((r) => setTimeout(r, 50));
  }
}
