import { useCallback, useEffect, useRef, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useLocation } from "wouter";
import { Boxes, FolderOpen } from "lucide-react";
import App from "../App";
import { setApiBase } from "../api/client";
import { desktopApp, type WorkspacesState } from "../lib/desktop";
import { makeQueryClient } from "../lib/queryClient";
import WorkspaceRail from "./WorkspaceRail";

// DesktopShell is the desktop-only root: it renders the folder-tabs rail and
// mounts the regular App for the active workspace. Each tab keeps its own
// React Query client (warm cache) and its last route; the Go side owns the
// tab list, persistence and the native window title.
const RAIL_KEY = "helmdex.rail"; // "expanded" widens the folder rail

export default function DesktopShell() {
  const [state, setState] = useState<WorkspacesState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [railExpanded, setRailExpanded] = useState(() => localStorage.getItem(RAIL_KEY) === "expanded");
  const clients = useRef(new Map<string, QueryClient>());
  const tabRoutes = useRef(new Map<string, string>());
  const [location, navigate] = useLocation();
  const locationRef = useRef(location);
  locationRef.current = location;
  const stateRef = useRef(state);
  stateRef.current = state;

  // adopt mirrors a binding-returned state: points the API client at the
  // active workspace, restores that tab's last route, drops closed tabs'
  // caches.
  const adopt = useCallback(
    (next: WorkspacesState) => {
      const prev = stateRef.current;
      const openIds = new Set(next.workspaces.map((w) => w.id));
      for (const id of [...clients.current.keys()]) {
        if (!openIds.has(id)) {
          clients.current.get(id)?.clear();
          clients.current.delete(id);
          tabRoutes.current.delete(id);
        }
      }
      if (next.activeId !== prev?.activeId) {
        if (prev?.activeId) tabRoutes.current.set(prev.activeId, locationRef.current);
        setApiBase(next.activeId ? `/ws/${next.activeId}` : "");
        if (next.activeId) navigate(tabRoutes.current.get(next.activeId) ?? "/");
      }
      setState(next);
    },
    [navigate],
  );

  const run = useCallback(
    (op: Promise<WorkspacesState>) => {
      op.then(adopt).catch((e: unknown) => setError(String(e)));
    },
    [adopt],
  );

  const openDialog = useCallback(() => run(desktopApp().OpenRepoDialog()), [run]);
  const activate = useCallback(
    (id: string) => {
      if (id !== stateRef.current?.activeId) run(desktopApp().SetActiveWorkspace(id));
    },
    [run],
  );
  const close = useCallback((id: string) => run(desktopApp().CloseWorkspace(id)), [run]);
  const toggleRail = useCallback(() => {
    setRailExpanded((v) => {
      const next = !v;
      if (next) {
        localStorage.setItem(RAIL_KEY, "expanded");
      } else {
        localStorage.removeItem(RAIL_KEY);
      }
      return next;
    });
  }, []);

  useEffect(() => {
    run(desktopApp().Workspaces());
  }, [run]);

  // Tab shortcuts: Ctrl+O open folder, Ctrl+1..9 jump, Ctrl+PgUp/PgDn cycle.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!e.ctrlKey || e.altKey || e.metaKey || e.shiftKey) return;
      // Never steal chords from text editing (inputs, textareas, Monaco).
      const target = e.target as HTMLElement | null;
      if (target?.closest?.("input, textarea, [contenteditable='true'], .monaco-editor")) return;
      const tabs = stateRef.current?.workspaces ?? [];
      if (e.key === "o") {
        e.preventDefault();
        openDialog();
        return;
      }
      if (e.key === "b") {
        e.preventDefault();
        toggleRail();
        return;
      }
      if (e.key >= "1" && e.key <= "9") {
        const tab = tabs[Number(e.key) - 1];
        if (tab) {
          e.preventDefault();
          activate(tab.id);
        }
        return;
      }
      if ((e.key === "PageUp" || e.key === "PageDown") && tabs.length > 1) {
        const idx = tabs.findIndex((w) => w.id === stateRef.current?.activeId);
        if (idx === -1) return;
        e.preventDefault();
        const dir = e.key === "PageUp" ? -1 : 1;
        activate(tabs[(idx + dir + tabs.length) % tabs.length].id);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [openDialog, activate, toggleRail]);

  if (!state) return null;

  const errorToast = error && (
    <button
      onClick={() => setError(null)}
      className="fixed bottom-3 left-16 z-50 rounded-md border border-error bg-panel px-3 py-2 text-left text-xs text-error"
      title="Dismiss"
    >
      {error}
    </button>
  );

  if (state.workspaces.length === 0) {
    return (
      <>
        <Welcome onOpen={openDialog} />
        {errorToast}
      </>
    );
  }

  let client = clients.current.get(state.activeId);
  if (!client) {
    client = makeQueryClient();
    clients.current.set(state.activeId, client);
  }

  return (
    <div className="flex h-full">
      <WorkspaceRail
        state={state}
        expanded={railExpanded}
        onActivate={activate}
        onClose={close}
        onAdd={openDialog}
        onToggleExpanded={toggleRail}
      />
      <div className="h-full min-w-0 flex-1">
        <QueryClientProvider client={client}>
          <App key={state.activeId} />
        </QueryClientProvider>
      </div>
      {errorToast}
    </div>
  );
}

// Welcome is shown when no folder is open (first launch, or every tab closed).
function Welcome({ onOpen }: { onOpen: () => void }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-6">
      <div className="flex items-center gap-3">
        <Boxes className="h-8 w-8 text-accent" />
        <span className="text-2xl font-semibold tracking-tight">helmdex</span>
      </div>
      <p className="max-w-sm text-center text-sm text-muted">
        Open a GitOps repository folder to browse and manage its Helm umbrella chart
        instances.
      </p>
      <button
        onClick={onOpen}
        className="flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-medium text-bg transition-opacity hover:opacity-90"
      >
        <FolderOpen className="h-4 w-4" /> Open folder…
      </button>
      <p className="text-xs text-muted">Ctrl+O</p>
    </div>
  );
}
