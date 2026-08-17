import { Route, Switch, Link, useRoute } from "wouter";
import { useQuery } from "@tanstack/react-query";
import { Boxes, BookOpen, Home } from "lucide-react";
import { api } from "./api/client";
import Dashboard from "./pages/Dashboard";
import InstancePage from "./pages/Instance";
import CatalogPage from "./pages/Catalog";
import EventsIndicator from "./components/EventsIndicator";
import RepoSwitcher from "./components/RepoSwitcher";

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
          <RepoSwitcher />
          <div className="space-y-1 px-2">
            {repo.data && (
              <>
                <div className="truncate" title={repo.data.root}>
                  {repo.data.root}
                </div>
                <div>
                  {repo.data.optedIn ? "helmdex repo" : "agnostic repo"} · {repo.data.appsDir}/
                </div>
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
