import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Layers, Plus, X } from "lucide-react";
import { api } from "../api/client";
import type { InstanceInfo } from "../api/types";

// Managed-mode sets: marker files that select preset layers imported on
// apply. Global sets (values.set.<set>.yaml) and per-dependency sets
// (values.dep-set.<depID>--<set>.yaml).
export default function SetsTab({ inst }: { inst: InstanceInfo }) {
  const qc = useQueryClient();
  const sets = useQuery({ queryKey: ["sets", inst.name], queryFn: () => api.sets(inst.name) });
  const [newSet, setNewSet] = useState("");
  const [newDepSet, setNewDepSet] = useState<{ depID: string; set: string }>({ depID: "", set: "" });

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["sets", inst.name] });
    void qc.invalidateQueries({ queryKey: ["files", inst.name] });
  };
  const enable = useMutation({
    mutationFn: ({ set, depID }: { set: string; depID?: string }) => api.enableSet(inst.name, set, depID),
    onSuccess: invalidate,
  });
  const disable = useMutation({
    mutationFn: ({ set, depID }: { set: string; depID?: string }) => api.disableSet(inst.name, set, depID),
    onSuccess: invalidate,
  });

  if (!inst.managed) {
    return (
      <div className="rounded-lg border border-dashed border-border p-8 text-center text-muted">
        Sets are a managed-mode feature (layered values). This instance is direct-mode: its values files
        are user-owned and edited in place.
      </div>
    );
  }

  return (
    <div className="max-w-3xl space-y-6">
      {(enable.isError || disable.isError) && (
        <div className="text-sm text-error">{((enable.error ?? disable.error) as Error)?.message}</div>
      )}

      <section>
        <h3 className="mb-2 flex items-center gap-2 font-medium">
          <Layers className="h-4 w-4 text-accent" /> Global sets
        </h3>
        <p className="mb-3 text-sm text-muted">
          A set is selected when its marker file exists; the matching preset layer is imported on apply.
        </p>
        <div className="flex flex-wrap items-center gap-2">
          {sets.data?.sets.map((s) => (
            <span key={s} className="flex items-center gap-1 rounded-md bg-panel-2 px-2 py-1 text-sm">
              {s}
              <button
                onClick={() => disable.mutate({ set: s })}
                className="text-muted hover:text-error"
                title={`Disable set ${s}`}
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </span>
          ))}
          <form
            className="flex items-center gap-1"
            onSubmit={(e) => {
              e.preventDefault();
              if (newSet.trim()) {
                enable.mutate({ set: newSet.trim() });
                setNewSet("");
              }
            }}
          >
            <input
              value={newSet}
              onChange={(e) => setNewSet(e.target.value)}
              placeholder="set name (e.g. prod)"
              className="w-40 rounded-md border border-border bg-panel-2 px-2 py-1 text-sm outline-none focus:border-accent"
            />
            <button type="submit" className="rounded-md bg-accent p-1 text-bg" title="Enable set">
              <Plus className="h-4 w-4" />
            </button>
          </form>
        </div>
      </section>

      <section>
        <h3 className="mb-2 flex items-center gap-2 font-medium">
          <Layers className="h-4 w-4 text-accent-2" /> Per-dependency sets
        </h3>
        <div className="space-y-2">
          {inst.deps.map((d) => (
            <div key={d.id} className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-panel px-3 py-2">
              <span className="text-sm font-medium">{d.id}</span>
              {(sets.data?.depSets[d.id] ?? []).map((s) => (
                <span key={s} className="flex items-center gap-1 rounded-md bg-panel-2 px-2 py-1 text-sm">
                  {s}
                  <button
                    onClick={() => disable.mutate({ set: s, depID: d.id })}
                    className="text-muted hover:text-error"
                    title={`Disable ${d.id} set ${s}`}
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </span>
              ))}
              <form
                className="ml-auto flex items-center gap-1"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (newDepSet.depID === d.id && newDepSet.set.trim()) {
                    enable.mutate({ set: newDepSet.set.trim(), depID: d.id });
                    setNewDepSet({ depID: "", set: "" });
                  }
                }}
              >
                <input
                  value={newDepSet.depID === d.id ? newDepSet.set : ""}
                  onChange={(e) => setNewDepSet({ depID: d.id, set: e.target.value })}
                  placeholder="add set…"
                  className="w-28 rounded-md border border-border bg-panel-2 px-2 py-1 text-sm outline-none focus:border-accent"
                />
                <button type="submit" className="rounded-md bg-accent p-1 text-bg" title="Enable dep set">
                  <Plus className="h-3.5 w-3.5" />
                </button>
              </form>
            </div>
          ))}
        </div>
        <p className="mt-3 text-sm text-muted">
          Run Apply to import the matching preset layers and regenerate values.yaml.
        </p>
      </section>
    </div>
  );
}
