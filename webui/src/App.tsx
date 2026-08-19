import { useEffect, useState } from "react";
import { Route, Switch, Link, useRoute } from "wouter";
import { useQuery } from "@tanstack/react-query";
import { Boxes, BookOpen, Home, KeyRound } from "lucide-react";
import { api } from "./api/client";
import { isDesktop } from "./lib/desktop";
import Dashboard from "./pages/Dashboard";
import InstancePage from "./pages/Instance";
import CatalogPage from "./pages/Catalog";
import EventsIndicator from "./components/EventsIndicator";
import CredentialsDialog from "./components/CredentialsDialog";
import ThemeToggle from "./components/ThemeToggle";

function NavLink({ href, children }: { href: string; children: React.ReactNode }) {
  const [active] = useRoute(href === "/" ? "/" : `${href}/*?`);
  return (
    <Link
      href={href}
      className={`flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors ${
        active ? "bg-panel-2 text-text" : "text-muted hover:bg-panel-2 hover:text-text"
      }`}
    >
      {children}
    </Link>
  );
}

export default function App() {
  const repo = useQuery({ queryKey: ["repo"], queryFn: api.repo });
  const [credsOpen, setCredsOpen] = useState(false);

  // Folder-aware tab title in the browser; the desktop window title is
  // native, set by the Go shell.
  const root = repo.data?.root;
  useEffect(() => {
    if (isDesktop() || !root) return;
    const name = root.split(/[\\/]/).filter(Boolean).pop() ?? root;
    document.title = `${name} — Helmdex`;
  }, [root]);

  return (
    <div className="flex h-full">
      <aside className="flex w-60 shrink-0 flex-col border-r border-border bg-panel p-3">
        <div className="mb-4 flex items-center gap-2 px-2 pt-1">
          <Boxes className="h-5 w-5 text-accent" />
          <span className="text-lg font-semibold tracking-tight">helmdex</span>
        </div>
        <nav className="flex flex-col gap-1">
          <NavLink href="/">
            <Home className="h-4 w-4" /> Dashboard
          </NavLink>
          <NavLink href="/catalog">
            <BookOpen className="h-4 w-4" /> Catalog
          </NavLink>
        </nav>
        <div className="mt-auto space-y-1 text-xs text-muted">
          <div className="px-2 pb-1">
            <ThemeToggle />
          </div>
          <button
            onClick={() => setCredsOpen(true)}
            className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-muted transition-colors hover:bg-panel-2 hover:text-text"
          >
            <KeyRound className="h-4 w-4" /> Credentials
          </button>
          {credsOpen && <CredentialsDialog onClose={() => setCredsOpen(false)} />}
          <div className="space-y-1 px-2">
            {repo.data && (
              <>
                <div className="truncate" title={repo.data.root}>
                  {repo.data.root}
                </div>
                <div>
                  {repo.data.optedIn ? "helmdex repo" : "agnostic repo"} · {repo.data.appsDir}/
                </div>
                {repo.data.configError && (
                  <div className="text-error" title={repo.data.configError}>
                    config error — see tooltip
                  </div>
                )}
              </>
            )}
            <EventsIndicator />
          </div>
        </div>
      </aside>
      <main className="min-w-0 flex-1 overflow-auto">
        <Switch>
          <Route path="/" component={Dashboard} />
          <Route path="/catalog" component={CatalogPage} />
          <Route path="/instances/:name" component={InstancePage} />
          <Route path="/instances/:name/:tab" component={InstancePage} />
          <Route>
            <div className="p-8 text-muted">Not found</div>
          </Route>
        </Switch>
      </main>
    </div>
  );
}
