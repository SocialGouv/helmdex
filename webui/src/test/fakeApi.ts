import { vi } from "vitest";
import type {
  CatalogEntryWithSource,
  DepInfo,
  DetectedCredential,
  FileInfo,
  InstanceInfo,
  RepoInfo,
  SchemaViolation,
  SetsInfo,
  SourcesInfo,
  StoredCredential,
  TemplateInfo,
  WorkspaceAuthHost,
} from "../api/types";

/**
 * An in-memory stand-in for the helmdex HTTP API, installed as `fetch`.
 *
 * Stubbing at the network boundary rather than at the module boundary keeps
 * api/client.ts in the test: URL building, encoding, status handling and the
 * `{error}` contract are all exercised. Mutations update the state, so a
 * refetch after a mutation returns what the mutation did — the property most
 * component tests actually depend on.
 *
 * The Go suite owns whether the real server behaves this way
 * (internal/server/*_test.go); this only owns whether the UI does the right
 * thing when it does.
 */

export type FakeApiState = {
  repo: RepoInfo;
  instances: InstanceInfo[];
  templates: TemplateInfo[];
  files: Record<string, FileInfo[]>;
  fileContents: Record<string, string>;
  values: Record<string, unknown>;
  depValues: Record<string, unknown>;
  sets: Record<string, SetsInfo>;
  catalog: CatalogEntryWithSource[];
  sources: SourcesInfo;
  versions: Record<string, string[]>;
  inspect: Record<string, string>;
  /** Violations returned by POST /values/validate (empty = passes). */
  schemaViolations: SchemaViolation[];
  auth: {
    creds: StoredCredential[];
    /** Candidates every /api/auth/detect call returns. */
    detect: DetectedCredential[];
    hosts: WorkspaceAuthHost[];
  };
};

/** One recorded exchange, body included: some flags only live in the body. */
export type RecordedCall = {
  method: string;
  path: string;
  body: unknown;
};

export type FakeApi = {
  state: FakeApiState;
  /** Every request the UI made, in order, as "METHOD /path". */
  requests: string[];
  /**
   * The same requests with their decoded bodies. Asserting on `requests`
   * alone cannot tell apart two calls that differ only by payload — Apply vs
   * Relock, or a deferred regen.
   */
  calls: RecordedCall[];
  /** The last call matching a method and path prefix, body included. */
  lastCall(method: string, pathPrefix: string): RecordedCall | undefined;
  /** Force the next matching request to fail. `extra` fields are merged
   * into the error body (e.g. an `authRequired` payload). */
  failNext(
    method: string,
    pathPrefix: string,
    status: number,
    message: string,
    extra?: Record<string, unknown>,
  ): void;
  restore(): void;
};

function dep(partial: Partial<DepInfo> & { id: string }): DepInfo {
  return {
    name: partial.name ?? partial.id,
    version: "1.0.0",
    repository: "https://example.invalid/charts",
    ...partial,
  };
}

export function defaultState(): FakeApiState {
  const alpha: InstanceInfo = {
    name: "alpha",
    path: "/repo/apps/alpha",
    managed: true,
    deps: [
      dep({ id: "postgresql", version: "15.5.0", sourceKind: "catalog", catalogID: "bitnami-postgresql-15.5.0", catalogSource: "Example" }),
      dep({ id: "nginx", version: "15.0.0", sourceKind: "arbitrary" }),
    ],
  };
  const legacy: InstanceInfo = {
    name: "legacy",
    path: "/repo/apps/legacy",
    managed: false,
    deps: [],
  };

  return {
    repo: {
      root: "/repo",
      optedIn: true,
      appsDir: "apps",
      templatesDir: "templates",
      configPath: "/repo/helmdex.yaml",
      configSource: "repo",
      platform: "eks",
    },
    instances: [alpha, legacy],
    templates: [{ name: "review", path: "/repo/templates/review" }],
    files: {
      alpha: [
        { path: "Chart.yaml", size: 120, isDir: false },
        { path: "values.instance.yaml", size: 42, isDir: false },
        { path: "values.yaml", size: 64, isDir: false },
      ],
      legacy: [{ path: "values.yaml", size: 64, isDir: false }],
    },
    fileContents: {
      "alpha/Chart.yaml": "apiVersion: v2\nname: alpha\nversion: 0.1.0\n",
      "alpha/values.instance.yaml": "replicaCount: 1\n",
      "alpha/values.yaml": "replicaCount: 1\n",
      "legacy/values.yaml": "# user-owned\nreplicaCount: 2\n",
    },
    values: { alpha: { replicaCount: 1 }, legacy: { replicaCount: 2 } },
    depValues: {},
    sets: {
      alpha: { managed: true, sets: ["dev"], depSets: { postgresql: ["ha-production"] } },
      legacy: { managed: false, sets: [], depSets: {} },
    },
    catalog: [
      {
        Entry: {
          ID: "bitnami-nginx-15.0.0",
          Description: "Fixture NGINX",
          Chart: { Repo: "https://charts.bitnami.com/bitnami", Name: "nginx" },
          Version: "15.0.0",
          DefaultSets: ["dev"],
        },
        SourceName: "Example",
      },
    ],
    sources: {
      platform: "eks",
      sources: [
        {
          Name: "Example",
          Git: { URL: "https://git.example.invalid/presets" },
          Presets: { Enabled: true, ChartsPath: "charts" },
          Catalog: { Enabled: true, Path: "catalog.yaml" },
        },
      ],
      savePath: "/repo/helmdex.yaml",
      saveSource: "repo",
    },
    versions: {
      "alpha/postgresql": ["16.0.0", "15.6.0", "15.5.0"],
      "alpha/nginx": ["15.2.0", "15.1.0", "15.0.0"],
    },
    inspect: {
      "alpha/nginx/readme": "# nginx\n\nFixture readme.",
      "alpha/nginx/values": "replicaCount: 1\n",
      "alpha/nginx/schema": '{"type":"object","properties":{"replicaCount":{"type":"integer"}}}',
    },
    schemaViolations: [],
    auth: { creds: [], detect: [], hosts: [] },
  };
}


/** Splits a "$.a.b" values path into its keys. "$" addresses the root. */
function pathKeys(path: string): string[] {
  const trimmed = path.trim();
  if (trimmed === "$" || trimmed === "") return [];
  return trimmed.replace(/^\$\.?/, "").split(".").filter(Boolean);
}

function getAtPath(root: unknown, path: string): unknown {
  let cur = root;
  for (const key of pathKeys(path)) {
    if (typeof cur !== "object" || cur === null) return undefined;
    cur = (cur as Record<string, unknown>)[key];
  }
  return cur;
}

/** Applies a value at a path, so a refetch reflects what the mutation did. */
function setAtPath(root: unknown, path: string, value: unknown): unknown {
  const keys = pathKeys(path);
  if (keys.length === 0) return value;
  const out = typeof root === "object" && root !== null ? { ...(root as Record<string, unknown>) } : {};
  let cur = out;
  for (const key of keys.slice(0, -1)) {
    const next = cur[key];
    cur[key] = typeof next === "object" && next !== null ? { ...(next as Record<string, unknown>) } : {};
    cur = cur[key] as Record<string, unknown>;
  }
  cur[keys[keys.length - 1]] = value;
  return out;
}

type Failure = {
  method: string;
  pathPrefix: string;
  status: number;
  message: string;
  /** Extra fields merged into the error body (e.g. authRequired). */
  extra?: Record<string, unknown>;
};

/** JSON bodies are decoded; anything else (file writes) is kept verbatim. */
function decodeBody(body: BodyInit | null | undefined): unknown {
  if (body === undefined || body === null) return undefined;
  const raw = String(body);
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

export function installFakeApi(overrides: Partial<FakeApiState> = {}): FakeApi {
  const state: FakeApiState = { ...defaultState(), ...overrides };
  const requests: string[] = [];
  const calls: RecordedCall[] = [];
  const failures: Failure[] = [];
  const original = globalThis.fetch;

  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  const text = (body: string, status = 200) =>
    new Response(body, { status, headers: { "Content-Type": "text/plain; charset=utf-8" } });
  const noContent = () => new Response(null, { status: 204 });
  const fail = (status: number, message: string) => json({ error: message }, status);

  const instance = (name: string) => state.instances.find((i) => i.name === name);

  globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const raw = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    const url = new URL(raw, "http://localhost");
    const method = (init?.method ?? "GET").toUpperCase();
    // Route like the Go side: the desktop shell prefixes /ws/<id>, which the
    // workspace manager strips before dispatching to a workspace server.
    const path = decodeURIComponent(url.pathname).replace(/^\/ws\/[^/]+(?=\/)/, "");
    requests.push(`${method} ${url.pathname}${url.search}`);
    calls.push({ method, path: `${url.pathname}${url.search}`, body: decodeBody(init?.body) });

    const planned = failures.findIndex((f) => f.method === method && path.startsWith(f.pathPrefix));
    if (planned >= 0) {
      const f = failures.splice(planned, 1)[0];
      return json({ error: f.message, ...f.extra }, f.status);
    }

    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : undefined;

    // /api/repo
    if (path === "/api/repo") return json(state.repo);
    if (path === "/api/templates") return json(state.templates);
    if (path === "/api/catalog") return json(state.catalog);
    if (path === "/api/catalog/sync" && method === "POST") return json([{ SourceName: "Example" }]);
    if (path === "/api/config/sources") {
      if (method === "PUT") {
        state.sources = {
          ...state.sources,
          platform: String(body?.platform ?? ""),
          sources: (body?.sources ?? []) as SourcesInfo["sources"],
        };
      }
      return json(state.sources);
    }
    if (path === "/api/artifacthub/search") return json([]);

    // /api/auth
    if (path === "/api/auth/creds" && method === "GET") return json(state.auth.creds);
    const credDel = path.match(/^\/api\/auth\/creds\/(.+)$/);
    if (credDel && method === "DELETE") {
      const host = credDel[1];
      const kind = url.searchParams.get("kind");
      const before = state.auth.creds.length;
      state.auth.creds = state.auth.creds.filter(
        (c) => !(c.host === host && (!kind || c.kind === kind)),
      );
      if (state.auth.creds.length === before) return fail(404, `no stored credential for "${host}"`);
      return noContent();
    }
    if (path === "/api/auth/detect" && method === "POST") {
      const host = String(body?.host ?? "");
      return json({
        candidates: state.auth.detect,
        tokenPage: {
          provider: "gitlab",
          host,
          url: `https://${host}/-/user_settings/personal_access_tokens?name=helmdex`,
        },
      });
    }
    if (path === "/api/auth/login" && method === "POST") {
      const host = String(body?.host ?? "");
      const kind = String(body?.kind ?? "") as StoredCredential["kind"];
      if (!host) return fail(400, "host is required");
      const cred: StoredCredential = {
        host,
        kind,
        username: (body?.username as string) || "jo",
        source: String(body?.source ?? "manual"),
      };
      state.auth.creds = [...state.auth.creds.filter((c) => !(c.host === host && c.kind === kind)), cred];
      state.auth.hosts = state.auth.hosts.map((h) =>
        h.host === host && h.kind === kind ? { ...h, hasCredential: true } : h,
      );
      return json({ ...cred, verified: true });
    }
    if (path === "/api/auth/token-page" && method === "POST") {
      const host = String(body?.host ?? "");
      return json({
        provider: "gitlab",
        host,
        url: `https://${host}/-/user_settings/personal_access_tokens?name=helmdex`,
      });
    }
    if (path === "/api/auth/hosts") return json(state.auth.hosts);

    // /api/instances
    if (path === "/api/instances") {
      if (method === "POST") {
        const name = String(body?.name ?? "");
        if (!name) return fail(400, "instance name is required");
        if (instance(name)) return fail(400, `instance already exists at /repo/apps/${name}`);
        const created: InstanceInfo = { name, path: `/repo/apps/${name}`, managed: true, deps: [] };
        state.instances.push(created);
        state.sets[name] = { managed: true, sets: [], depSets: {} };
        state.files[name] = [{ path: "Chart.yaml", size: 80, isDir: false }];
        return json(created, 201);
      }
      return json(state.instances);
    }

    const m = path.match(/^\/api\/instances\/(.+)$/);
    if (!m) return fail(404, "not found");

    const [instName, ...rest] = m[1].split("/");
    const sub = rest.join("/");
    const inst = instance(instName);
    if (!inst) return fail(404, `instance "${instName}" not found`);

    if (sub === "") {
      if (method === "DELETE") {
        state.instances = state.instances.filter((i) => i.name !== instName);
        return noContent();
      }
      return json(inst);
    }

    if (sub === "rename" && method === "POST") {
      const newName = String(body?.newName ?? "");
      if (!newName) return fail(400, "new name is required");
      if (instance(newName)) return fail(400, `instance already exists at /repo/apps/${newName}`);
      inst.name = newName;
      inst.path = `/repo/apps/${newName}`;
      return json(inst);
    }

    if (sub === "apply" && method === "POST") return json(inst);

    if (sub === "files") return json(state.files[instName] ?? []);

    if (sub === "file") {
      const rel = url.searchParams.get("path") ?? "";
      const key = `${instName}/${rel}`;
      if (method === "PUT") {
        state.fileContents[key] = String(init?.body ?? "");
        return noContent();
      }
      const content = state.fileContents[key];
      if (content === undefined) return fail(404, `open ${rel}: no such file or directory`);
      return text(content);
    }

    if (sub === "values") {
      const file = inst.managed ? "values.instance.yaml" : "values.yaml";
      if (method === "PUT") {
        state.values[instName] = setAtPath(state.values[instName], String(body?.path ?? "$"), body?.value);
        return noContent();
      }
      const p = url.searchParams.get("path") ?? "$";
      const value = getAtPath(state.values[instName], p);
      return json({ file, path: p, found: value !== undefined, value: value ?? null });
    }

    if (sub === "values/regen" && method === "POST") return noContent();

    if (sub === "values/validate" && method === "POST") {
      return json({ violations: state.schemaViolations });
    }

    if (sub === "deps") {
      if (method === "POST") {
        const added = dep({
          id: String(body?.alias || body?.name || ""),
          name: String(body?.name ?? ""),
          version: String(body?.version ?? ""),
          repository: String(body?.repository ?? ""),
          sourceKind: (body?.sourceKind as DepInfo["sourceKind"]) ?? "arbitrary",
          catalogID: body?.catalogID as string | undefined,
          catalogSource: body?.catalogSource as string | undefined,
        });
        if (!added.name || !added.version || !added.repository) {
          return fail(400, "dependency name, version and repository are required");
        }
        inst.deps.push(added);
        return json(inst, 201);
      }
      return json(inst.deps);
    }

    const depMatch = sub.match(/^deps\/(.+?)(?:\/(detach|version|values|versions|inspect))?$/);
    if (depMatch) {
      const depID = depMatch[1];
      const action = depMatch[2];
      const target = inst.deps.find((d) => d.id === depID);
      if (!target) return fail(404, `dependency "${depID}" not found`);

      if (!action && method === "DELETE") {
        inst.deps = inst.deps.filter((d) => d.id !== depID);
        return noContent();
      }
      if (action === "detach" && method === "POST") {
        if (target.sourceKind !== "catalog") return fail(400, `dependency "${depID}" is not catalog-attached`);
        target.sourceKind = "arbitrary";
        target.catalogID = undefined;
        target.catalogSource = undefined;
        return json(inst);
      }
      if (action === "version" && method === "POST") {
        const version = String(body?.version ?? "");
        const known = state.versions[`${instName}/${depID}`] ?? [];
        if (!version) return fail(400, "version is required");
        if (known.length > 0 && !known.includes(version)) {
          return fail(400, `invalid version ${JSON.stringify(version)} for ${depID}`);
        }
        target.version = version;
        return json(inst);
      }
      if (action === "values") {
        const key = `${instName}/${depID}`;
        if (method === "PUT") {
          state.depValues[key] = body?.value;
          return noContent();
        }
        const value = state.depValues[key];
        return json({
          file: inst.managed ? "values.instance.yaml" : "values.yaml",
          depID,
          found: value !== undefined,
          value: value ?? null,
        });
      }
      if (action === "versions") {
        const versions = state.versions[`${instName}/${depID}`] ?? [];
        return json({ current: target.version, versions, bestStable: versions[0] ?? "" });
      }
      if (action === "inspect") {
        const kind = url.searchParams.get("kind") ?? "";
        if (!["readme", "values", "schema"].includes(kind)) {
          return fail(400, "kind must be readme|values|schema");
        }
        const content = state.inspect[`${instName}/${depID}/${kind}`];
        if (content === undefined) return fail(502, `no ${kind} available for ${depID}`);
        return text(content);
      }
    }

    if (sub === "sets") {
      // The real server derives this from the instance on disk, so it can
      // never disagree with it; keep the single source of truth here too.
      const info = { ...(state.sets[instName] ?? { sets: [], depSets: {} }), managed: inst.managed };
      const setName = String(body?.set ?? "");
      const depID = body?.depID ? String(body.depID) : "";
      if (method === "POST") {
        if (!info.managed) return fail(400, "sets are a managed-mode feature; this instance is direct-mode");
        if (!setName) return fail(400, `invalid set name ${JSON.stringify(setName)}`);
        // Enabling an already-enabled set is a no-op, and the real server
        // says so with 204 rather than pretending to have created it.
        const already = depID
          ? (info.depSets[depID] ?? []).includes(setName)
          : info.sets.includes(setName);
        if (depID) info.depSets[depID] = [...new Set([...(info.depSets[depID] ?? []), setName])];
        else info.sets = [...new Set([...info.sets, setName])];
        state.sets[instName] = info;
        return new Response(null, { status: already ? 204 : 201 });
      }
      if (method === "DELETE") {
        if (depID) info.depSets[depID] = (info.depSets[depID] ?? []).filter((s) => s !== setName);
        else info.sets = info.sets.filter((s) => s !== setName);
        state.sets[instName] = info;
        return noContent();
      }
      return json(info);
    }

    return fail(404, `no route for ${method} ${path}`);
  }) as unknown as typeof fetch;

  return {
    state,
    requests,
    calls,
    lastCall(method, pathPrefix) {
      const wanted = method.toUpperCase();
      return [...calls].reverse().find((c) => c.method === wanted && c.path.startsWith(pathPrefix));
    },
    failNext(method, pathPrefix, status, message, extra) {
      failures.push({ method: method.toUpperCase(), pathPrefix, status, message, extra });
    },
    restore() {
      globalThis.fetch = original;
    },
  };
}
