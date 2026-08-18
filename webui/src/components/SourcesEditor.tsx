import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Save, Trash2 } from "lucide-react";
import { api } from "../api/client";
import type { ConfigSource } from "../api/types";

const emptySource: ConfigSource = {
  Name: "",
  Git: { URL: "", Ref: "" },
  Presets: { Enabled: true, ChartsPath: "charts" },
  Catalog: { Enabled: true, Path: "catalog.yaml" },
};

// Sources & platform configuration. Saves flow back to the repo
// helmdex.yaml (opted-in repos) or the user config (agnostic repos).
export default function SourcesEditor() {
  const qc = useQueryClient();
  const info = useQuery({ queryKey: ["sources"], queryFn: api.sources });

  const [platform, setPlatform] = useState("");
  const [sources, setSources] = useState<ConfigSource[]>([]);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (info.data && !dirty) {
      setPlatform(info.data.platform);
      setSources(info.data.sources);
    }
  }, [info.data, dirty]);

  const save = useMutation({
    mutationFn: () => api.saveSources(platform.trim(), sources),
    onSuccess: () => {
      setDirty(false);
      void qc.invalidateQueries({ queryKey: ["sources"] });
      void qc.invalidateQueries({ queryKey: ["repo"] });
      void qc.invalidateQueries({ queryKey: ["catalog"] });
    },
  });

  const update = (i: number, patch: Partial<ConfigSource>) => {
    setSources((prev) => prev.map((s, j) => (j === i ? { ...s, ...patch } : s)));
    setDirty(true);
  };

  return (
    <section className="mt-8 max-w-3xl">
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-lg font-medium">Sources</h2>
        <button
          onClick={() => save.mutate()}
          disabled={!dirty || save.isPending}
          className="flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-bg disabled:opacity-40"
        >
          <Save className="h-4 w-4" />
          {save.isPending ? "Saving…" : "Save"}
        </button>
      </div>
      {info.data && (
        <p className="mb-3 text-sm text-muted">
          Saved to {info.data.savePath || "the user config"} ({info.data.saveSource}).
        </p>
      )}
      {save.isError && <div className="mb-2 text-sm text-error">{(save.error as Error).message}</div>}

      <label className="mb-4 block">
        <span className="mb-1 block text-sm text-muted">Platform name (used to pick values.platform.&lt;name&gt;.yaml presets)</span>
        <input
          value={platform}
          onChange={(e) => {
            setPlatform(e.target.value);
            setDirty(true);
          }}
          placeholder="e.g. atlas"
          className="w-60 rounded-md border border-border bg-panel-2 px-3 py-1.5 text-sm outline-none focus:border-accent"
        />
      </label>

      <div className="space-y-3">
        {sources.map((src, i) => (
          <div key={i} className="rounded-lg border border-border bg-panel p-3">
            <div className="flex items-center gap-2">
              <input
                value={src.Name}
                onChange={(e) => update(i, { Name: e.target.value })}
                placeholder="source name"
                className="w-40 rounded-md border border-border bg-panel-2 px-2 py-1 text-sm outline-none focus:border-accent"
              />
              <input
                value={src.Git.URL}
                onChange={(e) => update(i, { Git: { ...src.Git, URL: e.target.value } })}
                placeholder="git URL or local path"
                className="flex-1 rounded-md border border-border bg-panel-2 px-2 py-1 text-sm outline-none focus:border-accent"
              />
              <input
                value={src.Git.Ref ?? ""}
                onChange={(e) => update(i, { Git: { ...src.Git, Ref: e.target.value } })}
                placeholder="ref (optional)"
                className="w-32 rounded-md border border-border bg-panel-2 px-2 py-1 text-sm outline-none focus:border-accent"
              />
              <button
                onClick={() => {
                  setSources((prev) => prev.filter((_, j) => j !== i));
                  setDirty(true);
                }}
                className="rounded p-1 text-muted hover:bg-panel-2 hover:text-error"
                title="Remove source"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </div>
            <div className="mt-2 flex gap-4 text-sm text-muted">
              <label className="flex items-center gap-1.5">
                <input
                  type="checkbox"
                  checked={src.Catalog.Enabled}
                  onChange={(e) => update(i, { Catalog: { ...src.Catalog, Enabled: e.target.checked } })}
                />
                catalog ({src.Catalog.Path || "catalog.yaml"})
              </label>
              <label className="flex items-center gap-1.5">
                <input
                  type="checkbox"
                  checked={src.Presets.Enabled}
                  onChange={(e) => update(i, { Presets: { ...src.Presets, Enabled: e.target.checked } })}
                />
                presets ({src.Presets.ChartsPath || "charts"}/)
              </label>
            </div>
          </div>
        ))}
      </div>

      <button
        onClick={() => {
          setSources((prev) => [...prev, structuredClone(emptySource)]);
          setDirty(true);
        }}
        className="mt-3 flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-sm text-muted hover:bg-panel-2"
      >
        <Plus className="h-4 w-4" /> Add source
      </button>
    </section>
  );
}
