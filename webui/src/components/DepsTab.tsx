import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as Dialog from "@radix-ui/react-dialog";
import { Eye, Plus, Tag, Trash2 } from "lucide-react";
import { api } from "../api/client";
import type { DepInfo, InspectKind, InstanceInfo } from "../api/types";
import AddDepWizard from "./AddDepWizard";

function InspectDialog({
  inst,
  dep,
  onClose,
}: {
  inst: InstanceInfo;
  dep: DepInfo;
  onClose: () => void;
}) {
  const [kind, setKind] = useState<InspectKind>("readme");
  const content = useQuery({
    queryKey: ["inspect", inst.name, dep.id, kind],
    queryFn: () => api.depInspect(inst.name, dep.id, kind),
  });

  return (
    <Dialog.Root open onOpenChange={(v) => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 flex h-[80vh] w-[80vw] max-w-4xl -translate-x-1/2 -translate-y-1/2 flex-col rounded-lg border border-border bg-panel p-5 shadow-xl">
          <Dialog.Title className="mb-3 flex items-center gap-3 font-medium">
            {dep.id}@{dep.version}
            <div className="flex gap-1">
              {(["readme", "values", "schema"] as InspectKind[]).map((k) => (
                <button
                  key={k}
                  onClick={() => setKind(k)}
                  className={`rounded px-2 py-1 text-xs capitalize ${
                    kind === k ? "bg-accent text-bg" : "bg-panel-2 text-muted hover:text-text"
                  }`}
                >
                  {k}
                </button>
              ))}
            </div>
          </Dialog.Title>
          <div className="min-h-0 flex-1 overflow-auto rounded-md bg-panel-2 p-4">
            {content.isLoading && <div className="text-muted">Loading (may pull the chart)…</div>}
            {content.isError && <div className="text-error">{(content.error as Error).message}</div>}
            {content.data !== undefined && (
              <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">
                {content.data}
              </pre>
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function VersionDialog({
  inst,
  dep,
  onClose,
}: {
  inst: InstanceInfo;
  dep: DepInfo;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const isOCI = dep.repository.startsWith("oci://");
  const versions = useQuery({
    queryKey: ["versions", inst.name, dep.id],
    queryFn: () => api.depVersions(inst.name, dep.id),
    enabled: !isOCI,
  });
  const [manual, setManual] = useState("");

  const setVersion = useMutation({
    mutationFn: (version: string) => api.setDepVersion(inst.name, dep.id, version, !isOCI),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["instance", inst.name] });
      onClose();
    },
  });

  return (
    <Dialog.Root open onOpenChange={(v) => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 flex max-h-[70vh] w-96 -translate-x-1/2 -translate-y-1/2 flex-col rounded-lg border border-border bg-panel p-5 shadow-xl">
          <Dialog.Title className="mb-3 font-medium">
            {dep.id} · current {dep.version}
          </Dialog.Title>

          <form
            className="mb-3 flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (manual.trim()) setVersion.mutate(manual.trim());
            }}
          >
            <input
              value={manual}
              onChange={(e) => setManual(e.target.value)}
              placeholder={isOCI ? "exact version / pinned tag" : "exact version"}
              className="flex-1 rounded-md border border-border bg-panel-2 px-2 py-1.5 text-sm outline-none focus:border-accent"
            />
            <button
              type="submit"
              disabled={!manual.trim() || setVersion.isPending}
              className="rounded-md bg-accent px-3 py-1.5 text-sm text-bg disabled:opacity-50"
            >
              Set
            </button>
          </form>
          {setVersion.isError && (
            <div className="mb-2 text-sm text-error">{(setVersion.error as Error).message}</div>
          )}

          {isOCI ? (
            <div className="text-sm text-muted">
              OCI repository: version listing is not available; set the exact pinned tag.
            </div>
          ) : (
            <div className="min-h-0 flex-1 overflow-auto">
              {versions.isLoading && <div className="text-muted">Loading versions…</div>}
              {versions.isError && <div className="text-error">{(versions.error as Error).message}</div>}
              {versions.data?.versions.map((v) => (
                <button
                  key={v}
                  onClick={() => setVersion.mutate(v)}
                  disabled={setVersion.isPending}
                  className={`flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-sm hover:bg-panel-2 ${
                    v === dep.version ? "text-accent" : ""
                  }`}
                >
                  {v}
                  {v === versions.data.bestStable && (
                    <span className="rounded bg-panel-2 px-1.5 text-[10px] uppercase text-accent-2">
                      best stable
                    </span>
                  )}
                </button>
              ))}
            </div>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export default function DepsTab({ inst }: { inst: InstanceInfo }) {
  const qc = useQueryClient();
  const [adding, setAdding] = useState(false);
  const [inspecting, setInspecting] = useState<DepInfo | null>(null);
  const [versioning, setVersioning] = useState<DepInfo | null>(null);

  const remove = useMutation({
    mutationFn: (depID: string) => api.removeDep(inst.name, depID),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["instance", inst.name] }),
  });

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <span className="text-sm text-muted">
          {inst.deps.length} {inst.deps.length === 1 ? "dependency" : "dependencies"} in Chart.yaml
        </span>
        <button
          onClick={() => setAdding(true)}
          className="flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-bg"
        >
          <Plus className="h-4 w-4" /> Add dependency
        </button>
      </div>

      {remove.isError && <div className="mb-2 text-sm text-error">{(remove.error as Error).message}</div>}

      <div className="overflow-hidden rounded-lg border border-border">
        <table className="w-full text-sm">
          <thead className="bg-panel text-left text-xs uppercase tracking-wide text-muted">
            <tr>
              <th className="px-3 py-2">ID</th>
              <th className="px-3 py-2">Chart</th>
              <th className="px-3 py-2">Version</th>
              <th className="px-3 py-2">Repository</th>
              <th className="px-3 py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {inst.deps.map((d) => (
              <tr key={d.id} className="border-t border-border hover:bg-panel">
                <td className="px-3 py-2 font-medium">{d.id}</td>
                <td className="px-3 py-2">{d.name}</td>
                <td className="px-3 py-2 font-mono text-xs">{d.version}</td>
                <td className="max-w-md truncate px-3 py-2 text-muted" title={d.repository}>
                  {d.repository}
                </td>
                <td className="px-3 py-2">
                  <div className="flex justify-end gap-1">
                    <button
                      onClick={() => setInspecting(d)}
                      className="rounded p-1 text-muted hover:bg-panel-2 hover:text-text"
                      title="Inspect readme / default values / schema"
                    >
                      <Eye className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => setVersioning(d)}
                      className="rounded p-1 text-muted hover:bg-panel-2 hover:text-text"
                      title="Change version"
                    >
                      <Tag className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => {
                        if (window.confirm(`Remove dependency ${d.id} from Chart.yaml?`)) {
                          remove.mutate(d.id);
                        }
                      }}
                      className="rounded p-1 text-muted hover:bg-panel-2 hover:text-error"
                      title="Remove"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {inst.deps.length === 0 && (
              <tr>
                <td colSpan={5} className="px-3 py-8 text-center text-muted">
                  No dependencies yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {adding && <AddDepWizard inst={inst} onClose={() => setAdding(false)} />}
      {inspecting && <InspectDialog inst={inst} dep={inspecting} onClose={() => setInspecting(null)} />}
      {versioning && <VersionDialog inst={inst} dep={versioning} onClose={() => setVersioning(null)} />}
    </div>
  );
}
