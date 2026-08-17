import { useState } from "react";
import { Link, useLocation, useParams } from "wouter";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as Tabs from "@radix-ui/react-tabs";
import { ArrowLeft, Hammer, Pencil } from "lucide-react";
import { api } from "../api/client";
import DepsTab from "../components/DepsTab";
import ValuesTab from "../components/ValuesTab";
import FilesTab from "../components/FilesTab";

const TABS = ["deps", "values", "files"] as const;
type Tab = (typeof TABS)[number];

export default function InstancePage() {
  const params = useParams<{ name: string; tab?: string }>();
  const name = params.name;
  const tab: Tab = TABS.includes(params.tab as Tab) ? (params.tab as Tab) : "deps";
  const [, navigate] = useLocation();
  const qc = useQueryClient();
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

      {apply.isError && <div className="mb-2 text-sm text-error">{(apply.error as Error).message}</div>}
      {rename.isError && <div className="mb-2 text-sm text-error">{(rename.error as Error).message}</div>}
      {inst.isError && <div className="text-error">{(inst.error as Error).message}</div>}

      <Tabs.Root
        value={tab}
        onValueChange={(v) => navigate(`/instances/${encodeURIComponent(name)}/${v}`, { replace: true })}
        className="flex min-h-0 flex-1 flex-col"
      >
        <Tabs.List className="mb-4 flex gap-1 border-b border-border">
          {TABS.map((t) => (
            <Tabs.Trigger
              key={t}
              value={t}
              className="border-b-2 border-transparent px-3 py-2 text-sm capitalize text-muted data-[state=active]:border-accent data-[state=active]:text-text"
            >
              {t === "deps" ? "Dependencies" : t}
            </Tabs.Trigger>
          ))}
        </Tabs.List>
        <Tabs.Content value="deps" className="min-h-0 flex-1">
          {inst.data && <DepsTab inst={inst.data} />}
        </Tabs.Content>
        <Tabs.Content value="values" className="min-h-0 flex-1">
          {inst.data && <ValuesTab inst={inst.data} />}
        </Tabs.Content>
        <Tabs.Content value="files" className="min-h-0 flex-1">
          {inst.data && <FilesTab inst={inst.data} />}
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
