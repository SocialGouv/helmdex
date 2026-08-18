import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as Dialog from "@radix-ui/react-dialog";
import { Save } from "lucide-react";
import { api } from "../api/client";
import type { DepInfo, InstanceInfo } from "../api/types";
import SchemaForm, { type JSONSchema } from "./SchemaForm";

// Structured (values.schema.json driven) editor for one dependency's
// overrides — stored under $.<depID> of the instance's edit file
// (values.instance.yaml in managed mode, values.yaml in direct mode).
export default function DepConfigureDialog({
  inst,
  dep,
  onClose,
}: {
  inst: InstanceInfo;
  dep: DepInfo;
  onClose: () => void;
}) {
  const qc = useQueryClient();

  const schema = useQuery({
    queryKey: ["inspect", inst.name, dep.id, "schema", dep.version],
    queryFn: () => api.depInspect(inst.name, dep.id, "schema"),
    retry: false,
  });
  const current = useQuery({
    queryKey: ["depvalues", inst.name, dep.id],
    queryFn: () => api.depValuesGet(inst.name, dep.id),
  });

  const [draft, setDraft] = useState<unknown>(undefined);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    if (current.data && !loaded) {
      setDraft(current.data.found ? current.data.value : {});
      setLoaded(true);
    }
  }, [current.data, loaded]);

  const save = useMutation({
    mutationFn: () => api.depValuesSet(inst.name, dep.id, draft ?? {}),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["depvalues", inst.name, dep.id] });
      void qc.invalidateQueries({ queryKey: ["file", inst.name] });
      void qc.invalidateQueries({ queryKey: ["files", inst.name] });
      onClose();
    },
  });

  let parsedSchema: JSONSchema | null = null;
  let schemaParseError = "";
  if (schema.data) {
    try {
      parsedSchema = JSON.parse(schema.data) as JSONSchema;
    } catch {
      schemaParseError = "values.schema.json is not valid JSON";
    }
  }

  return (
    <Dialog.Root open onOpenChange={(v) => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 flex h-[80vh] w-[46rem] -translate-x-1/2 -translate-y-1/2 flex-col rounded-lg border border-border bg-panel p-5 shadow-xl">
          <Dialog.Title className="mb-1 font-medium">
            Configure {dep.id}@{dep.version}
          </Dialog.Title>
          <p className="mb-3 text-sm text-muted">
            Overrides are written under <code className="rounded bg-panel-2 px-1">{dep.id}:</code> in the
            instance&apos;s {inst.managed ? "values.instance.yaml" : "values.yaml"}.
          </p>

          <div className="min-h-0 flex-1 overflow-auto pr-1">
            {(schema.isLoading || current.isLoading) && (
              <div className="text-muted">Loading schema (may pull the chart)…</div>
            )}
            {schema.isError && (
              <div className="mb-2 text-sm text-warn">
                No values.schema.json available for this chart — use the Values tab to edit YAML directly.
              </div>
            )}
            {schemaParseError && <div className="mb-2 text-sm text-error">{schemaParseError}</div>}
            {parsedSchema && loaded && (
              <SchemaForm schema={parsedSchema} value={draft} onChange={setDraft} />
            )}
          </div>

          {save.isError && <div className="mt-2 text-sm text-error">{(save.error as Error).message}</div>}
          <div className="mt-3 flex justify-end gap-2 border-t border-border pt-3">
            <Dialog.Close className="rounded-md px-3 py-1.5 text-sm text-muted hover:bg-panel-2">
              Cancel
            </Dialog.Close>
            <button
              onClick={() => save.mutate()}
              disabled={!parsedSchema || save.isPending}
              className="flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-bg disabled:opacity-50"
            >
              <Save className="h-4 w-4" />
              {save.isPending ? "Saving…" : "Save overrides"}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
