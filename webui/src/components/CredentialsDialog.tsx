import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as Dialog from "@radix-ui/react-dialog";
import { KeyRound, Trash2 } from "lucide-react";
import { api } from "../api/client";
import type { AuthCandidate } from "../api/types";
import AuthDialog from "./AuthDialog";

// Credential manager: every remote the open workspace references (dependency
// repositories, git sources) with its sign-in state, plus the stored
// credentials with removal. Secrets are never displayed.
export default function CredentialsDialog({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const [signingIn, setSigningIn] = useState<AuthCandidate | null>(null);

  const hosts = useQuery({ queryKey: ["auth-hosts"], queryFn: api.authHosts });
  const creds = useQuery({ queryKey: ["auth-creds"], queryFn: api.authCreds });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["auth-hosts"] });
    void qc.invalidateQueries({ queryKey: ["auth-creds"] });
  };

  const remove = useMutation({
    mutationFn: ({ host, kind }: { host: string; kind: string }) => api.authRemoveCred(host, kind),
    onSuccess: refresh,
  });

  return (
    <Dialog.Root open onOpenChange={(v) => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 flex max-h-[80vh] w-[34rem] -translate-x-1/2 -translate-y-1/2 flex-col overflow-auto rounded-lg border border-border bg-panel p-5 shadow-xl">
          <Dialog.Title className="mb-3 flex items-center gap-2 font-medium">
            <KeyRound className="h-4 w-4 text-accent" /> Credentials
          </Dialog.Title>

          <div className="mb-1 text-xs uppercase tracking-wide text-muted">
            Remotes used by this repository
          </div>
          <div className="mb-4">
            {hosts.isLoading && <div className="text-sm text-muted">Scanning…</div>}
            {hosts.isError && (
              <div className="text-sm text-error">{(hosts.error as Error).message}</div>
            )}
            {hosts.data?.length === 0 && (
              <div className="text-sm text-muted">No remote chart sources in this repository.</div>
            )}
            {hosts.data?.map((h) => (
              <div
                key={`${h.host}|${h.kind}`}
                className="mb-1 flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm"
              >
                <span className="min-w-0 flex-1 truncate" title={h.url}>
                  {h.host} <span className="text-xs text-muted">({h.kind})</span>
                </span>
                {h.hasCredential ? (
                  <span className="text-xs text-accent-2">signed in</span>
                ) : (
                  <button
                    onClick={() => setSigningIn(h)}
                    className="rounded-md bg-accent px-2.5 py-1 text-xs font-medium text-bg"
                  >
                    Sign in
                  </button>
                )}
              </div>
            ))}
          </div>

          <div className="mb-1 text-xs uppercase tracking-wide text-muted">Stored credentials</div>
          <div>
            {creds.data?.length === 0 && (
              <div className="text-sm text-muted">No credentials stored.</div>
            )}
            {creds.data?.map((c) => (
              <div
                key={`${c.host}|${c.kind}`}
                className="mb-1 flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm"
              >
                <span className="min-w-0 flex-1 truncate">
                  {c.host} <span className="text-xs text-muted">({c.kind})</span>
                </span>
                <span className="truncate text-xs text-muted">
                  {c.sshKeyPath ? `ssh key` : c.username} · {c.source}
                </span>
                <button
                  onClick={() => remove.mutate({ host: c.host, kind: c.kind })}
                  disabled={remove.isPending}
                  className="rounded p-1 text-muted hover:bg-panel-2 hover:text-error"
                  title="Remove credential"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            ))}
            {remove.isError && (
              <div className="text-sm text-error">{(remove.error as Error).message}</div>
            )}
          </div>

          <div className="mt-4 flex justify-end border-t border-border pt-3">
            <Dialog.Close className="rounded-md px-3 py-1.5 text-sm text-muted hover:bg-panel-2">
              Close
            </Dialog.Close>
          </div>

          {signingIn && (
            <AuthDialog
              candidates={[signingIn]}
              onClose={() => setSigningIn(null)}
              onSuccess={refresh}
            />
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
