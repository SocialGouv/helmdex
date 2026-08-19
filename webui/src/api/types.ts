export interface RepoInfo {
  root: string;
  optedIn: boolean;
  appsDir: string;
  templatesDir: string;
  configPath: string;
  configSource: string;
  configError?: string;
  platform: string;
}

export interface DepInfo {
  id: string;
  name: string;
  alias?: string;
  version: string;
  repository: string;
  sourceKind?: "catalog" | "artifacthub" | "arbitrary";
  catalogID?: string;
  catalogSource?: string;
}

export interface InstanceInfo {
  name: string;
  path: string;
  managed: boolean;
  deps: DepInfo[];
  depError?: string;
}

export interface TemplateInfo {
  name: string;
  path: string;
}

export interface FileInfo {
  path: string;
  size: number;
  isDir: boolean;
}

export interface DepVersions {
  current: string;
  versions: string[];
  bestStable: string;
}

export interface CatalogEntryWithSource {
  Entry: CatalogEntry;
  SourceName: string;
}

export interface CatalogEntry {
  ID: string;
  Description?: string;
  Chart: { Repo: string; Name: string };
  Version: string;
  Digest?: string;
  DefaultSets?: string[];
}

export interface AHPackage {
  RepositoryID: string;
  RepositoryKey: string;
  RepositoryName: string;
  RepositoryURL: string;
  Name: string;
  DisplayName: string;
  Description: string;
  LatestVersion: string;
}

export interface ValuesGetResponse {
  file: string;
  path: string;
  found: boolean;
  value: unknown;
}

export interface SchemaViolation {
  dep: string;
  path: string;
  message: string;
}

export interface ValuesValidateResponse {
  violations: SchemaViolation[];
}

export interface ServerEvent {
  type: string;
  instance?: string;
  message?: string;
}

export type InspectKind = "readme" | "values" | "schema";

export interface SetsInfo {
  managed: boolean;
  sets: string[];
  depSets: Record<string, string[]>;
}

export interface ConfigSource {
  Name: string;
  Git: { URL: string; Ref?: string; Commit?: string };
  Presets: { Enabled: boolean; ChartsPath?: string };
  Catalog: { Enabled: boolean; Path?: string };
}

export interface SourcesInfo {
  platform: string;
  sources: ConfigSource[];
  savePath: string;
  saveSource: string;
}

// --- auth (private chart sources) ---

export type AuthKind = "oci" | "git" | "helm-repo";

/** A remote a failed operation may need credentials for. */
export interface AuthCandidate {
  host: string;
  kind: AuthKind;
  url?: string;
}

/** Rides along 401 API errors so the UI can offer sign-in. */
export interface AuthRequiredInfo {
  candidates: AuthCandidate[];
}

/** A credential found in the user's local configuration (no secret). */
export interface DetectedCredential {
  source: string;
  host: string;
  username?: string;
  label: string;
}

export interface TokenPage {
  provider: string;
  url: string;
  host: string;
  openError?: string;
}

export interface AuthDetectResponse {
  candidates: DetectedCredential[];
  tokenPage: TokenPage;
}

export interface StoredCredential {
  host: string;
  kind: AuthKind;
  username?: string;
  sshKeyPath?: string;
  source?: string;
}

export interface AuthLoginRequest {
  host: string;
  kind: AuthKind;
  method: "manual" | "detected";
  source?: string;
  sourceHost?: string;
  username?: string;
  secret?: string;
  sshKeyPath?: string;
  url?: string;
}

export interface AuthLoginResult {
  host: string;
  kind: AuthKind;
  username?: string;
  source: string;
  verified: boolean;
  message?: string;
}

export interface WorkspaceAuthHost extends AuthCandidate {
  hasCredential: boolean;
}
