import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import * as Dialog from "@radix-ui/react-dialog";
import { DiffEditor } from "@monaco-editor/react";
import { api } from "../api/client";
import { useEditorTheme } from "../lib/theme";
import type { DepInfo, InstanceInfo } from "../api/types";
import ErrorWithAuth from "./ErrorWithAuth";

type DiffKind = "values" | "schema";

// Compare a dependency's default values (or schema) between the current
// version and a candidate version before upgrading.
export default function DepDiffDialog({
  inst,
  dep,
  onClose,
}: {
  inst: InstanceInfo;
  dep: DepInfo;
  onClose: () => void;
}) {
  const editorTheme = useEditorTheme();
  const [kind, setKind] = useState<DiffKind>("values");
  const [target, setTarget] = useState("");
  const [manual, setManual] = useState("");

  const versions = useQuery({
    queryKey: ["versions", inst.name, dep.id],
    queryFn: () => api.depVersions(inst.name, dep.id),
  });

  const current = useQuery({
    queryKey: ["inspect", inst.name, dep.id, kind, dep.version],
    queryFn: () => api.depInspect(inst.name, dep.id, kind),
  });
  const candidate = useQuery({
    queryKey: ["inspect", inst.name, dep.id, kind, target],
    queryFn: () => api.depInspect(inst.name, dep.id, kind, target),
    enabled: !!target,
  });

  return (
    <Dialog.Root open onOpenChange={(v) => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 flex h-[85vh] w-[90vw] max-w-6xl -translate-x-1/2 -translate-y-1/2 flex-col rounded-lg border border-border bg-panel p-5 shadow-xl">
          <Dialog.Title className="mb-3 flex flex-wrap items-center gap-3 font-medium">
            <span>
              {dep.id}: {dep.version} → {target || "…"}
            </span>
            <div className="flex gap-1">
              {(["values", "schema"] as DiffKind[]).map((k) => (
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
            <form
              className="ml-auto flex items-center gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (manual.trim()) setTarget(manual.trim());
              }}
            >
              {versions.data && (
                <select
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  className="rounded-md border border-border bg-panel-2 px-2 py-1 text-sm outline-none"
                >
                  <option value="">select version…</option>
                  {versions.data.versions.map((v) => (
                    <option key={v} value={v}>
                      {v}
                      {v === versions.data.bestStable ? " (best stable)" : ""}
                    </option>
                  ))}
                </select>
              )}
              <input
                value={manual}
                onChange={(e) => setManual(e.target.value)}
                placeholder="or type a version"
                className="w-40 rounded-md border border-border bg-panel-2 px-2 py-1 text-sm outline-none focus:border-accent"
              />
              <button type="submit" className="rounded-md bg-accent px-2 py-1 text-sm text-bg">
                Load
              </button>
            </form>
          </Dialog.Title>

          <div className="min-h-0 flex-1 overflow-hidden rounded-md border border-border">
            {!target && (
              <div className="grid h-full place-items-center text-muted">
                Pick a target version to diff against {dep.version}.
              </div>
            )}
            {target && (current.isLoading || candidate.isLoading) && (
              <div className="grid h-full place-items-center text-muted">
                Loading artifacts (may pull charts)…
              </div>
            )}
            {target && (current.isError || candidate.isError) && (
              <div className="grid h-full place-items-center p-4">
                <ErrorWithAuth
                  error={current.error ?? candidate.error}
                  onRetry={() => {
                    void current.refetch();
                    void candidate.refetch();
                  }}
                />
              </div>
            )}
            {target && current.data !== undefined && candidate.data !== undefined && (
              <DiffEditor
                language={kind === "schema" ? "json" : "yaml"}
                theme={editorTheme}
                original={current.data}
                modified={candidate.data}
                options={{ readOnly: true, renderSideBySide: true, minimap: { enabled: false }, fontSize: 12 }}
              />
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
