import type { WorkspaceInfo, WorkspacesState } from "../lib/desktop";

/**
 * An in-memory stand-in for the Wails workspace bindings, mirroring the Go
 * side's contract: every mutation returns the full tab state, closing the
 * active tab activates its neighbor, opening an already-open folder just
 * activates it.
 */
export type FakeDesktop = {
  calls: string[];
  /** Arms the next OpenRepoDialog with a picked folder (default: cancelled). */
  pickFolder: (dir: string) => void;
  state: () => WorkspacesState;
  restore: () => void;
};

export function installFakeDesktop(roots: string[]): FakeDesktop {
  let seq = 0;
  const tab = (root: string): WorkspaceInfo => ({
    id: `w${++seq}`,
    root,
    name: root.split("/").filter(Boolean).pop() ?? root,
  });
  const tabs: WorkspaceInfo[] = roots.map(tab);
  let activeId = tabs[0]?.id ?? "";
  let nextDialogDir: string | null = null;
  const calls: string[] = [];

  const state = (): WorkspacesState => ({ workspaces: [...tabs], activeId });

  window.go = {
    main: {
      App: {
        Workspaces: async () => state(),
        OpenRepoDialog: async () => {
          calls.push("OpenRepoDialog");
          if (nextDialogDir) {
            const existing = tabs.find((t) => t.root === nextDialogDir);
            if (existing) {
              activeId = existing.id;
            } else {
              const t = tab(nextDialogDir);
              tabs.push(t);
              activeId = t.id;
            }
            nextDialogDir = null;
          }
          return state();
        },
        SetActiveWorkspace: async (id: string) => {
          calls.push(`SetActiveWorkspace ${id}`);
          if (!tabs.some((t) => t.id === id)) throw new Error(`unknown workspace ${id}`);
          activeId = id;
          return state();
        },
        CloseWorkspace: async (id: string) => {
          calls.push(`CloseWorkspace ${id}`);
          const i = tabs.findIndex((t) => t.id === id);
          if (i < 0) throw new Error(`unknown workspace ${id}`);
          tabs.splice(i, 1);
          if (activeId === id) activeId = (tabs[Math.min(i, tabs.length - 1)] ?? { id: "" }).id;
          return state();
        },
      },
    },
  };

  return {
    calls,
    pickFolder: (dir: string) => {
      nextDialogDir = dir;
    },
    state,
    restore: () => {
      delete window.go;
    },
  };
}
