import { useState } from "react";
import { Link, useLocation, useParams } from "wouter";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Hammer, Pencil } from "lucide-react";
import { api } from "../api/client";
import { useMountedRef } from "../lib/useMounted";
import ErrorWithAuth from "../components/ErrorWithAuth";
import DepsTab from "../components/DepsTab";
import ValuesTab from "../components/ValuesTab";
import FilesTab from "../components/FilesTab";
import SetsTab from "../components/SetsTab";

const TABS = ["deps", "values", "sets", "files"] as const;
type Tab = (typeof TABS)[number];

export default function InstancePage() {
  const params = useParams<{ name: string; tab?: string }>();
  const name = params.name;
  const tab: Tab = TABS.includes(params.tab as Tab) ? (params.tab as Tab) : "deps";
  const [, navigate] = useLocation();
  const qc = useQueryClient();
  const mounted = useMountedRef();
  const [renaming, setRenaming] = useState(false);
  const [newName, setNewName] = useState(name);

  const inst = useQuery({ queryKey: ["instance", name], queryFn: () => api.instance(name) });

  const apply = useMutation({
    mutationFn: (relock: boolean) => api.applyInstance(name, relock),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["instance", name] });
      void qc.invalidateQueries({ queryKey: ["files", name] });
    },
  });

  const rename = useMutation({
    mutationFn: () => api.renameInstance(name, newName.trim()),
    onSuccess: (i) => {
      void qc.invalidateQueries({ queryKey: ["instances"] });
      // Unmounted = the user switched/closed the folder tab meanwhile:
      // navigating would hijack the now-active tab's URL.
      if (!mounted.current) return;
      setRenaming(false);
      navigate(`/instances/${encodeURIComponent(i.name)}/${tab}`, { replace: true });
    },
  });

  return (
    <div className="flex h-full flex-col p-6">
      <div className="mb-4 flex items-center gap-3">
        <Link href="/" className="rounded p-1 text-muted hover:bg-panel-2 hover:text-text">
          <ArrowLeft className="h-4 w-4" />
        </Link>
        {renaming ? (
          <form
            className="flex items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (newName.trim() && newName.trim() !== name) rename.mutate();
              else setRenaming(false);
            }}
          >
            <input
              autoFocus
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              className="rounded-md border border-border bg-panel-2 px-2 py-1 text-lg font-semibold outline-none focus:border-accent"
            />
            <button type="submit" className="rounded-md bg-accent px-2 py-1 text-sm text-bg">
              Rename
            </button>
          </form>
        ) : (
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            {name}
            {inst.data && !inst.data.managed && (
              <span className="rounded bg-panel-2 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-muted">
                direct
              </span>
            )}
            <button
              onClick={() => {
                setNewName(name);
                setRenaming(true);
              }}
              className="rounded p-1 text-muted hover:bg-panel-2 hover:text-text"
              title="Rename"
            >
              <Pencil className="h-3.5 w-3.5" />
            </button>
          </h1>
        )}
        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={() => apply.mutate(false)}
            disabled={apply.isPending}
            className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-sm hover:bg-panel-2 disabled:opacity-50"
            title="Reconcile Chart.lock/charts (and regenerate values.yaml in managed mode)"
          >
            <Hammer className="h-4 w-4" />
            {apply.isPending ? "Applying…" : "Apply"}
          </button>
          <button
            onClick={() => apply.mutate(true)}
            disabled={apply.isPending}
            className="rounded-md border border-border px-3 py-1.5 text-sm text-muted hover:bg-panel-2 disabled:opacity-50"
            title="Force helm dependency update"
          >
            Relock
          </button>
        </div>
      </div>

      {apply.isError && (
        <ErrorWithAuth
          className="mb-2"
          error={apply.error}
          onRetry={() => apply.mutate(apply.variables ?? false)}
        />
      )}
      {rename.isError && <div className="mb-2 text-sm text-error">{(rename.error as Error).message}</div>}
      {inst.isError && <div className="text-error">{(inst.error as Error).message}</div>}

      {/* Plain button tab bar: Radix Tabs' focus-driven activation does not
          fire reliably under WebKitGTK (desktop build). */}
      <div className="flex min-h-0 flex-1 flex-col">
        <div role="tablist" className="mb-4 flex gap-1 border-b border-border">
          {TABS.map((t) => (
            <button
              key={t}
              role="tab"
              aria-selected={tab === t}
              onClick={() => navigate(`/instances/${encodeURIComponent(name)}/${t}`, { replace: true })}
              className={`border-b-2 px-3 py-2 text-sm capitalize ${
                tab === t ? "border-accent text-text" : "border-transparent text-muted hover:text-text"
              }`}
            >
              {t === "deps" ? "Dependencies" : t}
            </button>
          ))}
        </div>
        <div className="min-h-0 flex-1">
          {inst.data && tab === "deps" && <DepsTab inst={inst.data} />}
          {inst.data && tab === "values" && <ValuesTab inst={inst.data} />}
          {inst.data && tab === "sets" && <SetsTab inst={inst.data} />}
          {inst.data && tab === "files" && <FilesTab inst={inst.data} />}
        </div>
      </div>
    </div>
  );
}
