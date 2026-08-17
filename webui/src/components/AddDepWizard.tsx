import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as Dialog from "@radix-ui/react-dialog";
import * as Tabs from "@radix-ui/react-tabs";
import { Search } from "lucide-react";
import { api } from "../api/client";
import type { AHPackage, CatalogEntryWithSource, InstanceInfo } from "../api/types";

interface Draft {
  name: string;
  repository: string;
  version: string;
  alias: string;
}

const emptyDraft: Draft = { name: "", repository: "", version: "", alias: "" };

export default function AddDepWizard({ inst, onClose }: { inst: InstanceInfo; onClose: () => void }) {
  const qc = useQueryClient();
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [ahQuery, setAhQuery] = useState("");
  const [ahSearch, setAhSearch] = useState("");

  const catalog = useQuery({ queryKey: ["catalog"], queryFn: api.catalog });
  const ah = useQuery({
    queryKey: ["ah", ahSearch],
    queryFn: () => api.ahSearch(ahSearch),
    enabled: ahSearch.length > 1,
  });

  const add = useMutation({
    mutationFn: () =>
      api.addDep(inst.name, {
        name: draft.name.trim(),
        repository: draft.repository.trim(),
        version: draft.version.trim(),
        alias: draft.alias.trim() || undefined,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["instance", inst.name] });
      onClose();
    },
  });

  const pickCatalog = (e: CatalogEntryWithSource) => {
    setDraft({
      name: e.Entry.Chart.Name,
      repository: e.Entry.Chart.Repo,
      version: e.Entry.Version,
      alias: "",
    });
  };

  const pickAH = (p: AHPackage) => {
    setDraft({ name: p.Name, repository: p.RepositoryURL, version: p.LatestVersion, alias: "" });
  };

  const canSubmit = draft.name.trim() && draft.repository.trim() && draft.version.trim();

  return (
    <Dialog.Root open onOpenChange={(v) => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 flex h-[75vh] w-[44rem] -translate-x-1/2 -translate-y-1/2 flex-col rounded-lg border border-border bg-panel p-5 shadow-xl">
          <Dialog.Title className="mb-3 font-medium">Add dependency to {inst.name}</Dialog.Title>

          <Tabs.Root defaultValue="catalog" className="flex min-h-0 flex-1 flex-col">
            <Tabs.List className="mb-3 flex gap-1 border-b border-border">
              {[
                ["catalog", "Catalog"],
                ["artifacthub", "Artifact Hub"],
                ["arbitrary", "Arbitrary / OCI"],
              ].map(([v, label]) => (
                <Tabs.Trigger
                  key={v}
                  value={v}
                  className="border-b-2 border-transparent px-3 py-2 text-sm text-muted data-[state=active]:border-accent data-[state=active]:text-text"
                >
                  {label}
                </Tabs.Trigger>
              ))}
            </Tabs.List>

            <Tabs.Content value="catalog" className="min-h-0 flex-1 overflow-auto">
              {catalog.isLoading && <div className="text-muted">Loading catalog…</div>}
              {catalog.isError && <div className="text-error">{(catalog.error as Error).message}</div>}
              {catalog.data?.length === 0 && (
                <div className="text-sm text-muted">
                  Catalog is empty. Configure sources and run a sync from the Catalog page.
                </div>
              )}
              {catalog.data?.map((e) => (
                <button
                  key={`${e.SourceName}/${e.Entry.ID}`}
                  onClick={() => pickCatalog(e)}
                  className={`block w-full rounded-md px-3 py-2 text-left hover:bg-panel-2 ${
                    draft.name === e.Entry.Chart.Name && draft.repository === e.Entry.Chart.Repo
                      ? "bg-panel-2 ring-1 ring-accent"
                      : ""
                  }`}
                >
                  <div className="flex items-center gap-2 text-sm">
                    <span className="font-medium">{e.Entry.ID}</span>
                    <span className="text-xs text-muted">
                      {e.SourceName} · {e.Entry.Version}
                    </span>
                  </div>
                  {e.Entry.Description && <div className="text-xs text-muted">{e.Entry.Description}</div>}
                </button>
              ))}
            </Tabs.Content>

            <Tabs.Content value="artifacthub" className="flex min-h-0 flex-1 flex-col">
              <form
                className="mb-2 flex gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  setAhSearch(ahQuery.trim());
                }}
              >
                <input
                  value={ahQuery}
                  onChange={(e) => setAhQuery(e.target.value)}
                  placeholder="Search Artifact Hub charts…"
                  className="flex-1 rounded-md border border-border bg-panel-2 px-3 py-1.5 text-sm outline-none focus:border-accent"
                />
                <button type="submit" className="rounded-md bg-accent px-3 py-1.5 text-sm text-bg">
                  <Search className="h-4 w-4" />
                </button>
              </form>
              <div className="min-h-0 flex-1 overflow-auto">
                {ah.isFetching && <div className="text-muted">Searching…</div>}
                {ah.isError && <div className="text-error">{(ah.error as Error).message}</div>}
                {ah.data?.map((p) => (
                  <button
                    key={`${p.RepositoryKey}/${p.Name}`}
                    onClick={() => pickAH(p)}
                    className={`block w-full rounded-md px-3 py-2 text-left hover:bg-panel-2 ${
                      draft.name === p.Name && draft.repository === p.RepositoryURL
                        ? "bg-panel-2 ring-1 ring-accent"
                        : ""
                    }`}
                  >
                    <div className="flex items-center gap-2 text-sm">
                      <span className="font-medium">{p.DisplayName || p.Name}</span>
                      <span className="text-xs text-muted">
                        {p.RepositoryName} · {p.LatestVersion}
                      </span>
                    </div>
                    <div className="line-clamp-1 text-xs text-muted">{p.Description}</div>
                  </button>
                ))}
              </div>
            </Tabs.Content>

            <Tabs.Content value="arbitrary" className="min-h-0 flex-1 overflow-auto">
              <div className="text-sm text-muted">
                Enter the repository URL (https://… or oci://…), chart name and exact version below.
              </div>
            </Tabs.Content>
          </Tabs.Root>

          <form
            className="mt-3 space-y-2 border-t border-border pt-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (canSubmit) add.mutate();
            }}
          >
            <div className="grid grid-cols-2 gap-2">
              <input
                value={draft.repository}
                onChange={(e) => setDraft({ ...draft, repository: e.target.value })}
                placeholder="repository (https://… or oci://…)"
                className="col-span-2 rounded-md border border-border bg-panel-2 px-3 py-1.5 text-sm outline-none focus:border-accent"
              />
              <input
                value={draft.name}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                placeholder="chart name"
                className="rounded-md border border-border bg-panel-2 px-3 py-1.5 text-sm outline-none focus:border-accent"
              />
              <input
                value={draft.version}
                onChange={(e) => setDraft({ ...draft, version: e.target.value })}
                placeholder="version"
                className="rounded-md border border-border bg-panel-2 px-3 py-1.5 text-sm outline-none focus:border-accent"
              />
              <input
                value={draft.alias}
                onChange={(e) => setDraft({ ...draft, alias: e.target.value })}
                placeholder="alias (optional)"
                className="col-span-2 rounded-md border border-border bg-panel-2 px-3 py-1.5 text-sm outline-none focus:border-accent"
              />
            </div>
            {add.isError && <div className="text-sm text-error">{(add.error as Error).message}</div>}
            <div className="flex justify-end gap-2">
              <Dialog.Close className="rounded-md px-3 py-1.5 text-sm text-muted hover:bg-panel-2">
                Cancel
              </Dialog.Close>
              <button
                type="submit"
                disabled={!canSubmit || add.isPending}
                className="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-bg disabled:opacity-50"
              >
                Add to Chart.yaml
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
