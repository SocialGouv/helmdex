import { useQueryClient } from "@tanstack/react-query";
import { FolderOpen } from "lucide-react";

// Wails bindings injected by the desktop shell; absent in the browser.
declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          OpenRepoDialog: () => Promise<string>;
        };
      };
    };
  }
}

export function isDesktop(): boolean {
  return typeof window !== "undefined" && !!window.go?.main?.App;
}

// Desktop-only: native directory picker that switches the served repo.
export default function RepoSwitcher() {
  const qc = useQueryClient();
  if (!isDesktop()) return null;

  const open = async () => {
    const dir = await window.go!.main!.App!.OpenRepoDialog();
    if (dir) {
      await qc.invalidateQueries();
    }
  };

  return (
    <button
      onClick={() => void open()}
      className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-muted transition-colors hover:bg-panel-2 hover:text-text"
    >
      <FolderOpen className="h-4 w-4" /> Open repository…
    </button>
  );
}
