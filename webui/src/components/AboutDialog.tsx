import { useMutation, useQuery } from "@tanstack/react-query";
import * as Dialog from "@radix-ui/react-dialog";
import { Boxes, ExternalLink, RefreshCw } from "lucide-react";
import { api } from "../api/client";
import { openExternal } from "../lib/desktop";
import { checkForUpdates, setAutoCheckEnabled, useUpdatesStatus } from "../lib/updates";

// About: installed version, link to the project, and the update section
// (manual check, download link, automatic-check opt-out).
export default function AboutDialog({ onClose }: { onClose: () => void }) {
  const version = useQuery({ queryKey: ["version"], queryFn: api.version });
  const { autoCheck, check } = useUpdatesStatus();
  const manualCheck = useMutation({ mutationFn: checkForUpdates });

  return (
    <Dialog.Root open onOpenChange={(v) => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 w-[26rem] -translate-x-1/2 -translate-y-1/2 rounded-lg border border-border bg-panel p-5 shadow-xl">
          <Dialog.Title className="mb-3 flex items-center gap-2 font-medium">
            <Boxes className="h-4 w-4 text-accent" /> About helmdex
          </Dialog.Title>

          {version.isError && (
            <div className="text-sm text-error">{(version.error as Error).message}</div>
          )}
          {version.data && (
            <div className="space-y-1 text-sm">
              <div>
                <span className="text-muted">Version:</span>{" "}
                <span className="font-mono">
                  {version.data.version}
                  {version.data.commit ? ` (${version.data.commit})` : ""}
                </span>
              </div>
              <button
                onClick={() => openExternal(version.data.repoUrl)}
                className="flex items-center gap-1 text-accent hover:underline"
              >
                {version.data.repoUrl.replace(/^https:\/\//, "")}
                <ExternalLink className="h-3 w-3" />
              </button>
            </div>
          )}

          <div className="mb-1 mt-5 text-xs uppercase tracking-wide text-muted">Updates</div>
          <div className="space-y-2 text-sm">
            <div className="flex items-center gap-2">
              <button
                onClick={() => manualCheck.mutate()}
                disabled={manualCheck.isPending}
                className="flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted transition-colors hover:bg-panel-2 hover:text-text disabled:opacity-50"
              >
                <RefreshCw className={`h-3.5 w-3.5 ${manualCheck.isPending ? "animate-spin" : ""}`} />
                Check for updates
              </button>
              {check &&
                (check.updateAvailable && check.releaseUrl ? (
                  <button
                    onClick={() => openExternal(check.releaseUrl)}
                    className="flex items-center gap-1.5 rounded-md bg-accent px-2.5 py-1 text-xs font-medium text-bg"
                  >
                    Download {check.latest} <ExternalLink className="h-3 w-3" />
                  </button>
                ) : check.updateAvailable ? (
                  <span className="text-xs text-accent">Update available: {check.latest}</span>
                ) : (
                  <span className="text-xs text-accent-2">
                    Up to date{check.latest ? ` (latest: ${check.latest})` : ""}
                  </span>
                ))}
            </div>
            {manualCheck.isError && (
              <div className="text-xs text-error">{(manualCheck.error as Error).message}</div>
            )}
            <label className="flex items-center gap-2 text-xs text-muted">
              <input
                type="checkbox"
                checked={autoCheck}
                onChange={(e) => setAutoCheckEnabled(e.target.checked)}
                className="accent-accent"
              />
              Check for new versions automatically (once a day)
            </label>
          </div>

          <div className="mt-5 flex justify-end border-t border-border pt-3">
            <Dialog.Close className="rounded-md px-3 py-1.5 text-sm text-muted hover:bg-panel-2">
              Close
            </Dialog.Close>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
