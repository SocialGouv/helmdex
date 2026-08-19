import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Editor from "@monaco-editor/react";
import { AlertTriangle, FileText, RefreshCw, Save } from "lucide-react";
import { api } from "../api/client";
import { useEditorTheme } from "../lib/theme";
import type { InstanceInfo, SchemaViolation } from "../api/types";

// Managed instances: layered values files + generated values.yaml.
// Direct instances: every values file is user-owned and edited in place.
export function valuesFileLabel(inst: InstanceInfo, file: string): string {
  if (!inst.managed) {
    return file === "values.yaml" ? "User-owned · edited in place" : "User-owned · extra values file";
  }
  switch (file) {
    case "values.default.yaml":
      return "Layer 1 · Baseline defaults";
    case "values.platform.yaml":
      return "Layer 2 · Platform overrides";
    case "values.instance.yaml":
      return "Layer 4 · Your overrides (editable)";
    case "values.yaml":
      return "Output · Merged result (generated)";
    default:
      return file.startsWith("values.set.") ? "Layer 3 · Set" : "Values layer";
  }
}

export function editableInMode(inst: InstanceInfo, file: string): boolean {
  if (!inst.managed) return true; // user-owned files, node-safe editing on save
  // Managed mode: the generated output and imported layers are read-only;
  // instance overrides and set markers are user-editable.
  return file !== "values.yaml" && file !== "values.default.yaml" && file !== "values.platform.yaml";
}

export default function ValuesTab({ inst }: { inst: InstanceInfo }) {
  const editorTheme = useEditorTheme();
  const qc = useQueryClient();
  const files = useQuery({ queryKey: ["files", inst.name], queryFn: () => api.files(inst.name) });

  const valuesFiles = useMemo(
    () =>
      (files.data ?? [])
        .filter((f) => !f.isDir && !f.path.includes("/") && /^values.*\.yaml$/.test(f.path))
        .map((f) => f.path)
        .sort((a, b) => (a === "values.yaml" ? -1 : b === "values.yaml" ? 1 : a.localeCompare(b))),
    [files.data],
  );

  const [selected, setSelected] = useState<string | null>(null);
  const file = selected ?? valuesFiles[0] ?? null;

  const content = useQuery({
    queryKey: ["file", inst.name, file],
    queryFn: () => api.readFile(inst.name, file!),
    enabled: !!file,
  });

  const [draft, setDraft] = useState<string | null>(null);
  const [violations, setViolations] = useState<SchemaViolation[] | null>(null);
  useEffect(() => {
    setDraft(null);
    setViolations(null);
  }, [file, content.data]);

  const save = useMutation({
    mutationFn: () => api.writeFile(inst.name, file!, draft ?? ""),
    onSuccess: () => {
      setDraft(null);
      setViolations(null);
      void qc.invalidateQueries({ queryKey: ["file", inst.name] });
      void qc.invalidateQueries({ queryKey: ["files", inst.name] });
    },
  });

  // Save gate: check the draft against the dependencies' values.schema.json
  // first. Violations are advisory — the user can save anyway (bypass).
  const validate = useMutation({
    mutationFn: () => api.validateValues(inst.name, draft ?? content.data ?? ""),
  });
  const handleSave = async () => {
    setViolations(null);
    try {
      const res = await validate.mutateAsync();
      if (res.violations.length > 0) {
        setViolations(res.violations);
        return;
      }
    } catch {
      // Validation itself failed (e.g. invalid YAML) — fall through so the
      // write surfaces the precise syntax error.
    }
    save.mutate();
  };

  const regen = useMutation({
    mutationFn: () => api.valuesRegen(inst.name),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["file", inst.name] }),
  });

  const editable = file ? editableInMode(inst, file) : false;
  const dirty = draft !== null && draft !== content.data;

  return (
    <div className="flex h-full gap-4">
      <div className="w-64 shrink-0 space-y-1 overflow-auto">
        {valuesFiles.map((f) => (
          <button
            key={f}
            onClick={() => setSelected(f)}
            className={`block w-full rounded-md px-3 py-2 text-left ${
              f === file ? "bg-panel-2 ring-1 ring-accent" : "hover:bg-panel"
            }`}
          >
            <div className="flex items-center gap-2 text-sm">
              <FileText className="h-3.5 w-3.5 text-accent" />
              {f}
            </div>
            <div className="text-xs text-muted">{valuesFileLabel(inst, f)}</div>
          </button>
        ))}
        {inst.managed && (
          <button
            onClick={() => regen.mutate()}
            disabled={regen.isPending}
            className="mt-2 flex w-full items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-muted hover:bg-panel-2 disabled:opacity-50"
          >
            <RefreshCw className="h-3.5 w-3.5" />
            Regenerate values.yaml
          </button>
        )}
        {regen.isError && <div className="text-xs text-error">{(regen.error as Error).message}</div>}
      </div>

      <div className="flex min-w-0 flex-1 flex-col overflow-hidden rounded-lg border border-border">
        <div className="flex items-center justify-between border-b border-border bg-panel px-3 py-2">
          <span className="text-sm">
            {file ?? "no values file"}
            {!editable && file && <span className="ml-2 text-xs text-muted">(read-only)</span>}
          </span>
          {editable && (
            <button
              onClick={() => void handleSave()}
              disabled={!dirty || save.isPending || validate.isPending}
              className="flex items-center gap-1.5 rounded-md bg-accent px-3 py-1 text-sm font-medium text-bg disabled:opacity-40"
            >
              <Save className="h-3.5 w-3.5" />
              {validate.isPending ? "Validating…" : save.isPending ? "Saving…" : "Save"}
            </button>
          )}
        </div>
        {save.isError && (
          <div className="border-b border-border px-3 py-1 text-sm text-error">
            {(save.error as Error).message}
          </div>
        )}
        {violations && violations.length > 0 && (
          <div className="border-b border-border bg-panel px-3 py-2 text-sm">
            <div className="mb-1 flex items-center gap-1.5 text-warn">
              <AlertTriangle className="h-4 w-4" />
              {violations.length} schema {violations.length === 1 ? "warning" : "warnings"} — values
              do not match the chart&apos;s values.schema.json
            </div>
            <ul className="mb-2 max-h-40 space-y-0.5 overflow-auto">
              {violations.map((v, i) => (
                <li key={i} className="font-mono text-xs text-muted">
                  <span className="text-text">
                    {v.dep}
                    {v.path ? `.${v.path}` : ""}
                  </span>
                  : {v.message}
                </li>
              ))}
            </ul>
            <button
              onClick={() => {
                setViolations(null);
                save.mutate();
              }}
              className="rounded-md border border-warn px-2.5 py-1 text-xs font-medium text-warn hover:bg-panel-2"
            >
              Save anyway
            </button>
          </div>
        )}
        <div className="min-h-0 flex-1">
          {file && content.data !== undefined && (
            <Editor
              language="yaml"
              theme={editorTheme}
              value={draft ?? content.data}
              onChange={(v) => {
                if (!editable) return;
                setDraft(v ?? "");
                setViolations(null);
              }}
              options={{
                readOnly: !editable,
                minimap: { enabled: false },
                fontSize: 13,
                scrollBeyondLastLine: false,
              }}
            />
          )}
          {content.isError && <div className="p-4 text-error">{(content.error as Error).message}</div>}
        </div>
      </div>
    </div>
  );
}
