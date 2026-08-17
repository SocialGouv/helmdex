import type {
  AHPackage,
  CatalogEntryWithSource,
  DepVersions,
  FileInfo,
  InspectKind,
  InstanceInfo,
  RepoInfo,
  TemplateInfo,
  ValuesGetResponse,
} from "./types";

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: init?.body ? { "Content-Type": "application/json" } : undefined,
    ...init,
  });
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      // non-JSON error body: keep the status text
    }
    throw new ApiError(res.status, message);
  }
  if (res.status === 204) return undefined as T;
  const ct = res.headers.get("Content-Type") ?? "";
  if (ct.includes("application/json")) return (await res.json()) as T;
  return (await res.text()) as T;
}

export const api = {
  repo: () => request<RepoInfo>("/api/repo"),

  instances: () => request<InstanceInfo[]>("/api/instances"),
  instance: (name: string) => request<InstanceInfo>(`/api/instances/${encodeURIComponent(name)}`),
  createInstance: (name: string, fromTemplate?: string) =>
    request<InstanceInfo>("/api/instances", {
      method: "POST",
      body: JSON.stringify({ name, fromTemplate: fromTemplate || undefined }),
    }),
  deleteInstance: (name: string) =>
    request<void>(`/api/instances/${encodeURIComponent(name)}`, { method: "DELETE" }),
  renameInstance: (name: string, newName: string) =>
    request<InstanceInfo>(`/api/instances/${encodeURIComponent(name)}/rename`, {
      method: "POST",
      body: JSON.stringify({ newName }),
    }),
  applyInstance: (name: string, relock = false) =>
    request<InstanceInfo>(`/api/instances/${encodeURIComponent(name)}/apply`, {
      method: "POST",
      body: JSON.stringify({ relock }),
    }),

  files: (name: string) => request<FileInfo[]>(`/api/instances/${encodeURIComponent(name)}/files`),
  readFile: (name: string, path: string) =>
    request<string>(`/api/instances/${encodeURIComponent(name)}/file?path=${encodeURIComponent(path)}`),
  writeFile: (name: string, path: string, content: string) =>
    request<void>(`/api/instances/${encodeURIComponent(name)}/file?path=${encodeURIComponent(path)}`, {
      method: "PUT",
      body: content,
    }),

  valuesGet: (name: string, path = "$") =>
    request<ValuesGetResponse>(
      `/api/instances/${encodeURIComponent(name)}/values?path=${encodeURIComponent(path)}`,
    ),
  valuesSet: (name: string, path: string, value: unknown, regen = true) =>
    request<void>(`/api/instances/${encodeURIComponent(name)}/values`, {
      method: "PUT",
      body: JSON.stringify({ path, value, regen }),
    }),
  valuesRegen: (name: string) =>
    request<void>(`/api/instances/${encodeURIComponent(name)}/values/regen`, { method: "POST" }),

  addDep: (name: string, dep: { name: string; repository: string; version: string; alias?: string }) =>
    request<InstanceInfo>(`/api/instances/${encodeURIComponent(name)}/deps`, {
      method: "POST",
      body: JSON.stringify(dep),
    }),
  removeDep: (name: string, depID: string) =>
    request<void>(`/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}`, {
      method: "DELETE",
    }),
  setDepVersion: (name: string, depID: string, version: string, validate = true) =>
    request<InstanceInfo>(
      `/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}/version`,
      { method: "POST", body: JSON.stringify({ version, validate }) },
    ),
  depVersions: (name: string, depID: string) =>
    request<DepVersions>(
      `/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}/versions`,
    ),
  depInspect: (name: string, depID: string, kind: InspectKind) =>
    request<string>(
      `/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}/inspect?kind=${kind}`,
    ),

  templates: () => request<TemplateInfo[]>("/api/templates"),

  catalog: () => request<CatalogEntryWithSource[]>("/api/catalog"),
  catalogSync: () => request<unknown>("/api/catalog/sync", { method: "POST" }),
  ahSearch: (q: string, limit = 25) =>
    request<AHPackage[]>(`/api/artifacthub/search?q=${encodeURIComponent(q)}&limit=${limit}`),
};

export { ApiError };
