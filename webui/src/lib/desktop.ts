// Typed access to the Wails bindings injected by the desktop shell; absent in
// the browser (`helmdex ui`).

export interface WorkspaceInfo {
  id: string;
  root: string;
  name: string;
}

/** Full tab state; every mutating binding returns it so the UI just mirrors. */
export interface WorkspacesState {
  workspaces: WorkspaceInfo[];
  activeId: string;
}

interface DesktopBindings {
  Workspaces: () => Promise<WorkspacesState>;
  OpenRepoDialog: () => Promise<WorkspacesState>;
  SetActiveWorkspace: (id: string) => Promise<WorkspacesState>;
  CloseWorkspace: (id: string) => Promise<WorkspacesState>;
}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: DesktopBindings;
      };
    };
  }
}

export function isDesktop(): boolean {
  return typeof window !== "undefined" && !!window.go?.main?.App;
}

export function desktopApp(): DesktopBindings {
  const app = window.go?.main?.App;
  if (!app) throw new Error("desktop bindings unavailable outside the desktop app");
  return app;
}
