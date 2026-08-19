import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { api } from "../api/client";
import AuthRequiredNotice from "../components/AuthRequiredNotice";
import SourcesEditor from "../components/SourcesEditor";

export default function CatalogPage() {
  const qc = useQueryClient();
  const catalog = useQuery({ queryKey: ["catalog"], queryFn: api.catalog });
  const sync = useMutation({
    mutationFn: api.catalogSync,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["catalog"] }),
  });

  return (
    <div className="p-6">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Catalog</h1>
        <button
          onClick={() => sync.mutate()}
          disabled={sync.isPending}
          className="flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-bg disabled:opacity-50"
        >
          <RefreshCw className={`h-4 w-4 ${sync.isPending ? "animate-spin" : ""}`} />
          {sync.isPending ? "Syncing…" : "Sync sources"}
        </button>
      </div>

      {sync.isError && (
        <div className="mb-2">
          <div className="text-error">{(sync.error as Error).message}</div>
          <AuthRequiredNotice error={sync.error} onResolved={() => sync.mutate()} />
        </div>
      )}
      {catalog.isLoading && <div className="text-muted">Loading…</div>}
      {catalog.isError && <div className="text-error">{(catalog.error as Error).message}</div>}

      {catalog.data?.length === 0 && (
        <div className="rounded-lg border border-dashed border-border p-8 text-center text-muted">
          Catalog is empty. Configure sources in helmdex.yaml (or the user config for agnostic repos), then
          sync.
        </div>
      )}

      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        {catalog.data?.map((e) => (
          <div key={`${e.SourceName}/${e.Entry.ID}`} className="rounded-lg border border-border bg-panel p-4">
            <div className="flex items-center gap-2">
              <span className="font-medium">{e.Entry.ID}</span>
              <span className="rounded bg-panel-2 px-1.5 py-0.5 text-xs text-muted">{e.SourceName}</span>
            </div>
            {e.Entry.Description && <div className="mt-1 text-sm text-muted">{e.Entry.Description}</div>}
            <div className="mt-2 font-mono text-xs text-muted">
              {e.Entry.Chart.Name}@{e.Entry.Version}
            </div>
            <div className="truncate font-mono text-xs text-muted" title={e.Entry.Chart.Repo}>
              {e.Entry.Chart.Repo}
            </div>
            {e.Entry.DefaultSets && e.Entry.DefaultSets.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1">
                {e.Entry.DefaultSets.map((s) => (
                  <span key={s} className="rounded bg-panel-2 px-1.5 py-0.5 text-xs text-accent-2">
                    set:{s}
                  </span>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>

      <SourcesEditor />
    </div>
  );
}
