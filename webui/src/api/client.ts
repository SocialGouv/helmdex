import type {
  AHPackage,
  AuthDetectResponse,
  AuthLoginRequest,
  AuthLoginResult,
  AuthRequiredInfo,
  ConfigSource,
  SetsInfo,
  SourcesInfo,
  CatalogEntryWithSource,
  DepVersions,
  FileInfo,
  InspectKind,
  InstanceInfo,
  RepoInfo,
  StoredCredential,
  TemplateInfo,
  TokenPage,
  ValuesGetResponse,
  ValuesValidateResponse,
  WorkspaceAuthHost,
} from "./types";

class ApiError extends Error {
  status: number;
  /** Set when the server classified the failure as a missing/invalid
   * credential for a remote — the UI offers a sign-in flow then. */
  authRequired?: AuthRequiredInfo;
  constructor(status: number, message: string, authRequired?: AuthRequiredInfo) {
    super(message);
    this.status = status;
    this.authRequired = authRequired;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: init?.body ? { "Content-Type": "application/json" } : undefined,
    ...init,
  });
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    let authRequired: AuthRequiredInfo | undefined;
    try {
      const body = (await res.json()) as { error?: string; authRequired?: AuthRequiredInfo };
      if (body.error) message = body.error;
      if (body.authRequired?.candidates?.length) authRequired = body.authRequired;
    } catch {
      // non-JSON error body: keep the status text
    }
    throw new ApiError(res.status, message, authRequired);
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
  validateValues: (name: string, content: string) =>
    request<ValuesValidateResponse>(`/api/instances/${encodeURIComponent(name)}/values/validate`, {
      method: "POST",
      body: JSON.stringify({ content }),
    }),

  addDep: (
    name: string,
    dep: {
      name: string;
      repository: string;
      version: string;
      alias?: string;
      sourceKind?: string;
      catalogID?: string;
      catalogSource?: string;
    },
  ) =>
    request<InstanceInfo>(`/api/instances/${encodeURIComponent(name)}/deps`, {
      method: "POST",
      body: JSON.stringify(dep),
    }),
  detachDep: (name: string, depID: string) =>
    request<InstanceInfo>(
      `/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}/detach`,
      { method: "POST" },
    ),
  removeDep: (name: string, depID: string) =>
    request<void>(`/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}`, {
      method: "DELETE",
    }),
  setDepVersion: (name: string, depID: string, version: string, validate = true) =>
    request<InstanceInfo>(
      `/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}/version`,
      { method: "POST", body: JSON.stringify({ version, validate }) },
    ),
  depValuesGet: (name: string, depID: string) =>
    request<{ file: string; depID: string; found: boolean; value: unknown }>(
      `/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}/values`,
    ),
  depValuesSet: (name: string, depID: string, value: unknown, regen = true) =>
    request<void>(`/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}/values`, {
      method: "PUT",
      body: JSON.stringify({ value, regen }),
    }),
  depVersions: (name: string, depID: string) =>
    request<DepVersions>(
      `/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}/versions`,
    ),
  depInspect: (name: string, depID: string, kind: InspectKind, version?: string) =>
    request<string>(
      `/api/instances/${encodeURIComponent(name)}/deps/${encodeURIComponent(depID)}/inspect?kind=${kind}` +
        (version ? `&version=${encodeURIComponent(version)}` : ""),
    ),

  sets: (name: string) => request<SetsInfo>(`/api/instances/${encodeURIComponent(name)}/sets`),
  enableSet: (name: string, set: string, depID?: string) =>
    request<void>(`/api/instances/${encodeURIComponent(name)}/sets`, {
      method: "POST",
      body: JSON.stringify({ set, depID: depID || undefined }),
    }),
  disableSet: (name: string, set: string, depID?: string) =>
    request<void>(`/api/instances/${encodeURIComponent(name)}/sets`, {
      method: "DELETE",
      body: JSON.stringify({ set, depID: depID || undefined }),
    }),

  templates: () => request<TemplateInfo[]>("/api/templates"),

  sources: () => request<SourcesInfo>("/api/config/sources"),
  saveSources: (platform: string, sources: ConfigSource[]) =>
    request<SourcesInfo>("/api/config/sources", {
      method: "PUT",
      body: JSON.stringify({ platform, sources }),
    }),

  authCreds: () => request<StoredCredential[]>("/api/auth/creds"),
  authRemoveCred: (host: string, kind?: string) =>
    request<void>(
      `/api/auth/creds/${encodeURIComponent(host)}${kind ? `?kind=${encodeURIComponent(kind)}` : ""}`,
      { method: "DELETE" },
    ),
  authDetect: (host: string, kind: string) =>
    request<AuthDetectResponse>("/api/auth/detect", {
      method: "POST",
      body: JSON.stringify({ host, kind }),
    }),
  authLogin: (req: AuthLoginRequest) =>
    request<AuthLoginResult>("/api/auth/login", { method: "POST", body: JSON.stringify(req) }),
  authTokenPage: (host: string, kind: string, open: boolean) =>
    request<TokenPage>("/api/auth/token-page", {
      method: "POST",
      body: JSON.stringify({ host, kind, open }),
    }),
  authHosts: () => request<WorkspaceAuthHost[]>("/api/auth/hosts"),

  catalog: () => request<CatalogEntryWithSource[]>("/api/catalog"),
  catalogSync: () => request<unknown>("/api/catalog/sync", { method: "POST" }),
  ahSearch: (q: string, limit = 25) =>
    request<AHPackage[]>(`/api/artifacthub/search?q=${encodeURIComponent(q)}&limit=${limit}`),
};

export { ApiError };
