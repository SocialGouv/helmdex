import { useState } from "react";
import { Link, useLocation } from "wouter";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Package, Plus, LayoutTemplate, Trash2 } from "lucide-react";
import * as Dialog from "@radix-ui/react-dialog";
import { api } from "../api/client";
import { useMountedRef } from "../lib/useMounted";
import ViewToggle, { useViewMode } from "../components/ViewToggle";
import type { InstanceInfo, TemplateInfo } from "../api/types";

function InstanceCard({ inst, onDelete }: { inst: InstanceInfo; onDelete: (name: string) => void }) {
  return (
    <div className="group relative rounded-lg border border-border bg-panel p-4 transition-colors hover:border-accent">
      <Link href={`/instances/${encodeURIComponent(inst.name)}`} className="block">
        <div className="flex items-center gap-2">
          <Package className="h-4 w-4 text-accent" />
          <span className="font-medium">{inst.name}</span>
          {!inst.managed && (
            <span className="rounded bg-panel-2 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-muted">
              direct
            </span>
          )}
        </div>
        <div className="mt-2 text-sm text-muted">
          {inst.depError
            ? `Chart error: ${inst.depError}`
            : `${inst.deps.length} ${inst.deps.length === 1 ? "dependency" : "dependencies"}`}
        </div>
        {inst.deps.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1">
            {inst.deps.map((d) => (
              <span key={d.id} className="rounded bg-panel-2 px-1.5 py-0.5 text-xs text-muted">
                {d.id}@{d.version}
              </span>
            ))}
          </div>
        )}
      </Link>
      <button
        onClick={() => onDelete(inst.name)}
        className="absolute right-3 top-3 hidden rounded p-1 text-muted hover:bg-panel-2 hover:text-error group-hover:block"
        title={`Delete ${inst.name}`}
      >
        <Trash2 className="h-4 w-4" />
      </button>
    </div>
  );
}

function InstanceRow({ inst, onDelete }: { inst: InstanceInfo; onDelete: (name: string) => void }) {
  return (
    <div className="group flex items-center gap-3 px-3 py-2 transition-colors hover:bg-panel-2">
      <Link
        href={`/instances/${encodeURIComponent(inst.name)}`}
        className="flex min-w-0 flex-1 items-center gap-3"
      >
        <Package className="h-4 w-4 shrink-0 text-accent" />
        <span className="font-medium">{inst.name}</span>
        {!inst.managed && (
          <span className="rounded bg-panel-2 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-muted">
            direct
          </span>
        )}
        <span className={`truncate text-sm ${inst.depError ? "text-error" : "text-muted"}`}>
          {inst.depError
            ? `Chart error: ${inst.depError}`
            : `${inst.deps.length} ${inst.deps.length === 1 ? "dependency" : "dependencies"}`}
        </span>
      </Link>
      <button
        onClick={() => onDelete(inst.name)}
        className="hidden shrink-0 rounded p-1 text-muted hover:bg-panel hover:text-error group-hover:block"
        title={`Delete ${inst.name}`}
      >
        <Trash2 className="h-4 w-4" />
      </button>
    </div>
  );
}

function CreateDialog({
  open,
  onOpenChange,
  fromTemplate,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  fromTemplate?: string;
}) {
  const [name, setName] = useState("");
  const [, navigate] = useLocation();
  const qc = useQueryClient();
  const mounted = useMountedRef();
  const create = useMutation({
    mutationFn: () => api.createInstance(name.trim(), fromTemplate),
    onSuccess: (inst) => {
      void qc.invalidateQueries({ queryKey: ["instances"] });
      // Unmounted = the user switched/closed the folder tab meanwhile:
      // navigating would hijack the now-active tab's URL.
      if (!mounted.current) return;
      onOpenChange(false);
      setName("");
      navigate(`/instances/${encodeURIComponent(inst.name)}`);
    },
  });

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50" />
        <Dialog.Content className="fixed left-1/2 top-1/3 w-96 -translate-x-1/2 rounded-lg border border-border bg-panel p-5 shadow-xl">
          <Dialog.Title className="font-medium">
            {fromTemplate ? `New instance from template ${fromTemplate}` : "New instance"}
          </Dialog.Title>
          <form
            className="mt-4 space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (name.trim()) create.mutate();
            }}
          >
            <input
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="instance name (e.g. myapp-preprod)"
              className="w-full rounded-md border border-border bg-panel-2 px-3 py-2 text-sm outline-none focus:border-accent"
            />
            {create.isError && <div className="text-sm text-error">{(create.error as Error).message}</div>}
            <div className="flex justify-end gap-2">
              <Dialog.Close className="rounded-md px-3 py-1.5 text-sm text-muted hover:bg-panel-2">
                Cancel
              </Dialog.Close>
              <button
                type="submit"
                disabled={!name.trim() || create.isPending}
                className="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-bg disabled:opacity-50"
              >
                Create
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export default function Dashboard() {
  const qc = useQueryClient();
  const instances = useQuery({ queryKey: ["instances"], queryFn: api.instances });
  const templates = useQuery({ queryKey: ["templates"], queryFn: api.templates });
  const [createOpen, setCreateOpen] = useState(false);
  const [createFrom, setCreateFrom] = useState<string | undefined>();
  const [view, setView] = useViewMode("dashboard");

  const del = useMutation({
    mutationFn: (name: string) => api.deleteInstance(name),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["instances"] }),
  });

  const onDelete = (name: string) => {
    if (window.confirm(`Delete instance ${name}? This removes its directory.`)) {
      del.mutate(name);
    }
  };

  const openCreate = (from?: string) => {
    setCreateFrom(from);
    setCreateOpen(true);
  };

  return (
    <div className="p-6">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Instances</h1>
        <div className="flex items-center gap-2">
          <ViewToggle value={view} onChange={setView} />
          <button
            onClick={() => openCreate()}
            className="flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-bg"
          >
            <Plus className="h-4 w-4" /> New instance
          </button>
        </div>
      </div>

      {instances.isLoading && <div className="text-muted">Loading…</div>}
      {instances.isError && (
        <div className="text-error">Failed to load instances: {(instances.error as Error).message}</div>
      )}
      {del.isError && <div className="mb-2 text-error">{(del.error as Error).message}</div>}

      {view === "grid" ? (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {instances.data?.map((inst) => <InstanceCard key={inst.name} inst={inst} onDelete={onDelete} />)}
        </div>
      ) : (
        instances.data &&
        instances.data.length > 0 && (
          <div className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-panel">
            {instances.data.map((inst) => (
              <InstanceRow key={inst.name} inst={inst} onDelete={onDelete} />
            ))}
          </div>
        )
      )}
      {instances.data?.length === 0 && (
        <div className="rounded-lg border border-dashed border-border p-8 text-center text-muted">
          No instances yet. Create one, or copy a template below.
        </div>
      )}

      {templates.data && templates.data.length > 0 && (
        <>
          <h2 className="mb-3 mt-8 text-lg font-medium">Templates</h2>
          {view === "grid" ? (
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
              {templates.data.map((t: TemplateInfo) => (
                <button
                  key={t.name}
                  onClick={() => openCreate(t.name)}
                  className="rounded-lg border border-border bg-panel p-4 text-left transition-colors hover:border-accent-2"
                >
                  <div className="flex items-center gap-2">
                    <LayoutTemplate className="h-4 w-4 text-accent-2" />
                    <span className="font-medium">{t.name}</span>
                  </div>
                  <div className="mt-2 text-sm text-muted">Template · click to create an instance from this blueprint</div>
                </button>
              ))}
            </div>
          ) : (
            <div className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-panel">
              {templates.data.map((t: TemplateInfo) => (
                <button
                  key={t.name}
                  onClick={() => openCreate(t.name)}
                  className="flex w-full items-center gap-3 px-3 py-2 text-left transition-colors hover:bg-panel-2"
                >
                  <LayoutTemplate className="h-4 w-4 shrink-0 text-accent-2" />
                  <span className="font-medium">{t.name}</span>
                  <span className="truncate text-sm text-muted">Template · click to create an instance</span>
                </button>
              ))}
            </div>
          )}
        </>
      )}

      <CreateDialog open={createOpen} onOpenChange={setCreateOpen} fromTemplate={createFrom} />
    </div>
  );
}
