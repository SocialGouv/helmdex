import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import * as Dialog from "@radix-ui/react-dialog";
import { ExternalLink, KeyRound, LoaderCircle } from "lucide-react";
import { api } from "../api/client";
import type { AuthCandidate, AuthLoginRequest, DetectedCredential } from "../api/types";

// Sign-in dialog for a private chart source (OCI registry, Helm repo or git
// remote). Ordered by friction:
//   1. credentials detected in local config (docker/podman/helm/git/gh/glab)
//      — one click, the secret never transits through the browser
//   2. a PAT, with the provider's token-creation page opened pre-filled
//   3. username/password, or an SSH key path for git remotes
export default function AuthDialog({
  candidates,
  onClose,
  onSuccess,
}: {
  candidates: AuthCandidate[];
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [selected, setSelected] = useState<AuthCandidate>(candidates[0]);
  const [username, setUsername] = useState("");
  const [secret, setSecret] = useState("");
  const [sshKeyPath, setSshKeyPath] = useState("");
  const [useSSH, setUseSSH] = useState(false);

  const detect = useQuery({
    queryKey: ["auth-detect", selected.host, selected.kind],
    queryFn: () => api.authDetect(selected.host, selected.kind),
    staleTime: 30_000,
  });

  const login = useMutation({
    mutationFn: (req: AuthLoginRequest) => api.authLogin(req),
    onSuccess: () => {
      onSuccess();
      onClose();
    },
  });

  const openTokenPage = useMutation({
    mutationFn: () => api.authTokenPage(selected.host, selected.kind, true),
  });

  const loginDetected = (c: DetectedCredential) =>
    login.mutate({
      host: selected.host,
      kind: selected.kind,
      method: "detected",
      source: c.source,
      sourceHost: c.host,
      url: selected.url,
    });

  const loginManual = (e: React.FormEvent) => {
    e.preventDefault();
    login.mutate({
      host: selected.host,
      kind: selected.kind,
      method: "manual",
      username: username.trim() || undefined,
      secret: useSSH ? undefined : secret,
      sshKeyPath: useSSH ? sshKeyPath.trim() : undefined,
      url: selected.url,
    });
  };

  const kindLabel =
    selected.kind === "oci" ? "OCI registry" : selected.kind === "git" ? "git remote" : "Helm repository";

  return (
    <Dialog.Root open onOpenChange={(v) => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 flex max-h-[85vh] w-[30rem] -translate-x-1/2 -translate-y-1/2 flex-col overflow-auto rounded-lg border border-border bg-panel p-5 shadow-xl">
          <Dialog.Title className="mb-1 flex items-center gap-2 font-medium">
            <KeyRound className="h-4 w-4 text-accent" />
            Sign in to {selected.host}
          </Dialog.Title>
          <p className="mb-3 text-sm text-muted">
            This {kindLabel} requires authentication. Credentials are stored locally
            (~/.config/helmdex) and never leave this machine.
          </p>

          {candidates.length > 1 && (
            <div className="mb-3 flex flex-wrap gap-1">
              {candidates.map((c) => (
                <button
                  key={`${c.host}|${c.kind}`}
                  onClick={() => setSelected(c)}
                  className={`rounded px-2 py-1 text-xs ${
                    c.host === selected.host && c.kind === selected.kind
                      ? "bg-accent text-bg"
                      : "bg-panel-2 text-muted hover:text-text"
                  }`}
                >
                  {c.host} ({c.kind})
                </button>
              ))}
            </div>
          )}

          {/* 1. detected local credentials */}
          <div className="mb-3">
            <div className="mb-1 text-xs uppercase tracking-wide text-muted">Found on this machine</div>
            {detect.isLoading && (
              <div className="flex items-center gap-2 text-sm text-muted">
                <LoaderCircle className="h-4 w-4 animate-spin" /> Scanning local config
                (docker, git, gh, glab)…
              </div>
            )}
            {detect.isError && (
              <div className="text-sm text-error">{(detect.error as Error).message}</div>
            )}
            {detect.data && detect.data.candidates.length === 0 && (
              <div className="text-sm text-muted">
                No usable credentials found in local config.
              </div>
            )}
            {detect.data?.candidates.map((c) => (
              <button
                key={`${c.source}|${c.host}`}
                onClick={() => loginDetected(c)}
                disabled={login.isPending}
                className="mb-1 flex w-full items-center justify-between rounded-md border border-border px-3 py-2 text-left text-sm hover:bg-panel-2 disabled:opacity-50"
              >
                <span>
                  Use {c.label}
                  {c.username ? ` (${c.username})` : ""}
                </span>
                <span className="text-xs text-muted">{c.host}</span>
              </button>
            ))}
          </div>

          {/* 2 + 3. token / password / ssh key */}
          <div className="mb-1 flex items-center justify-between">
            <div className="text-xs uppercase tracking-wide text-muted">
              {useSSH ? "SSH key" : "Access token / password"}
            </div>
            {selected.kind === "git" && (
              <button
                onClick={() => setUseSSH(!useSSH)}
                className="text-xs text-accent hover:underline"
              >
                {useSSH ? "Use a token instead" : "Use an SSH key instead"}
              </button>
            )}
          </div>

          {!useSSH && detect.data?.tokenPage.url && (
            <button
              onClick={() => openTokenPage.mutate()}
              className="mb-2 flex items-center gap-1.5 text-left text-sm text-accent hover:underline"
              title={detect.data.tokenPage.url}
            >
              <ExternalLink className="h-3.5 w-3.5" />
              Create a token in the browser ({detect.data.tokenPage.host})
            </button>
          )}
          {openTokenPage.data?.openError && (
            <div className="mb-2 break-all text-xs text-warn">
              Could not open a browser — visit: {openTokenPage.data.url}
            </div>
          )}

          <form onSubmit={loginManual} className="flex flex-col gap-2">
            {useSSH ? (
              <input
                value={sshKeyPath}
                onChange={(e) => setSshKeyPath(e.target.value)}
                placeholder="Private key path, e.g. ~/.ssh/id_ed25519 (stored by path, never read)"
                className="rounded-md border border-border bg-panel-2 px-2 py-1.5 text-sm outline-none focus:border-accent"
              />
            ) : (
              <>
                <input
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="Username (optional — any value works with a PAT)"
                  autoComplete="username"
                  className="rounded-md border border-border bg-panel-2 px-2 py-1.5 text-sm outline-none focus:border-accent"
                />
                <input
                  value={secret}
                  onChange={(e) => setSecret(e.target.value)}
                  type="password"
                  placeholder="Token or password"
                  autoComplete="current-password"
                  className="rounded-md border border-border bg-panel-2 px-2 py-1.5 text-sm outline-none focus:border-accent"
                />
              </>
            )}
            {login.isError && (
              <div className="text-sm text-error">{(login.error as Error).message}</div>
            )}
            <div className="flex justify-end gap-2 pt-1">
              <Dialog.Close className="rounded-md px-3 py-1.5 text-sm text-muted hover:bg-panel-2">
                Cancel
              </Dialog.Close>
              <button
                type="submit"
                disabled={login.isPending || (useSSH ? !sshKeyPath.trim() : !secret)}
                className="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-bg disabled:opacity-50"
              >
                {login.isPending ? "Signing in…" : "Sign in"}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
