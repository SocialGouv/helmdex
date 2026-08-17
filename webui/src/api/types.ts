export interface RepoInfo {
  root: string;
  optedIn: boolean;
  appsDir: string;
  templatesDir: string;
  configPath: string;
  configSource: string;
  platform: string;
}

export interface DepInfo {
  id: string;
  name: string;
  alias?: string;
  version: string;
  repository: string;
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

export interface ServerEvent {
  type: string;
  instance?: string;
  message?: string;
}

export type InspectKind = "readme" | "values" | "schema";
