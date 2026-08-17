import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { File as FileIcon, Folder } from "lucide-react";
import Editor from "@monaco-editor/react";
import { api } from "../api/client";
import type { InstanceInfo } from "../api/types";

function languageFor(path: string): string {
  if (path.endsWith(".yaml") || path.endsWith(".yml")) return "yaml";
  if (path.endsWith(".json")) return "json";
  if (path.endsWith(".md")) return "markdown";
  return "plaintext";
}

// Read-only browser over every file of the instance dir — surfaces the
// extra files gitops repos carry (values.deploy.yaml, atlas-env.yaml,
// local templates/…) without attaching any semantics to them.
export default function FilesTab({ inst }: { inst: InstanceInfo }) {
  const files = useQuery({ queryKey: ["files", inst.name], queryFn: () => api.files(inst.name) });
  const [selected, setSelected] = useState<string | null>(null);

  const content = useQuery({
    queryKey: ["file", inst.name, selected],
    queryFn: () => api.readFile(inst.name, selected!),
    enabled: !!selected,
  });

  return (
    <div className="flex h-full gap-4">
      <div className="w-72 shrink-0 overflow-auto">
        {files.isLoading && <div className="text-muted">Loading…</div>}
        {files.isError && <div className="text-error">{(files.error as Error).message}</div>}
        {files.data?.map((f) => (
          <button
            key={f.path}
            disabled={f.isDir}
            onClick={() => setSelected(f.path)}
            className={`flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm ${
              f.isDir ? "text-muted" : "hover:bg-panel"
            } ${f.path === selected ? "bg-panel-2 ring-1 ring-accent" : ""}`}
            style={{ paddingLeft: `${8 + f.path.split("/").length * 12 - 12}px` }}
          >
            {f.isDir ? <Folder className="h-3.5 w-3.5" /> : <FileIcon className="h-3.5 w-3.5 text-accent" />}
            <span className="truncate">{f.path.split("/").pop()}</span>
            {!f.isDir && <span className="ml-auto text-xs text-muted">{f.size}B</span>}
          </button>
        ))}
      </div>
      <div className="min-w-0 flex-1 overflow-hidden rounded-lg border border-border">
        {selected && content.data !== undefined ? (
          <Editor
            language={languageFor(selected)}
            theme="vs-dark"
            value={content.data}
            options={{ readOnly: true, minimap: { enabled: false }, fontSize: 13 }}
          />
        ) : (
          <div className="grid h-full place-items-center text-muted">
            {content.isError ? (content.error as Error).message : "Select a file to preview"}
          </div>
        )}
      </div>
    </div>
  );
}
