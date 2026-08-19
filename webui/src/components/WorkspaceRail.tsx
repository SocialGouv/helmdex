import { Plus, X } from "lucide-react";
import type { WorkspacesState } from "../lib/desktop";

// Deterministic hue per folder path, so tabs are distinguishable at a glance
// even when their monograms collide.
export function railHue(root: string): number {
  let h = 0;
  for (let i = 0; i < root.length; i++) h = (h * 31 + root.charCodeAt(i)) >>> 0;
  return h % 360;
}

// 2-char monogram: initials of the first two words of the folder name, or its
// first two characters. Code-point based: `[0]`/`slice` on UTF-16 units would
// split emoji/astral chars into broken surrogate halves.
export function monogram(name: string): string {
  const words = name
    .split(/[-_. ]+/)
    .map((w) => Array.from(w))
    .filter((w) => w.length > 0);
  let mono: string[];
  if (words.length >= 2) {
    mono = [words[0][0], words[1][0]];
  } else if (words.length === 1) {
    mono = words[0].slice(0, 2);
  } else {
    mono = ["?"];
  }
  return mono.join("").toUpperCase();
}

interface Props {
  state: WorkspacesState;
  onActivate: (id: string) => void;
  onClose: (id: string) => void;
  onAdd: () => void;
}

// The vertical folder-tabs rail of the desktop shell: one tile per open
// folder, "+" to open another one.
export default function WorkspaceRail({ state, onActivate, onClose, onAdd }: Props) {
  return (
    <div className="flex w-14 shrink-0 flex-col items-center gap-2 overflow-y-auto border-r border-border bg-bg py-3">
      {state.workspaces.map((ws, i) => {
        const active = ws.id === state.activeId;
        const shortcut = i < 9 ? ` — Ctrl+${i + 1}` : "";
        return (
          <div key={ws.id} className="group relative">
            <button
              onClick={() => onActivate(ws.id)}
              onAuxClick={(e) => {
                if (e.button === 1) onClose(ws.id);
              }}
              title={`${ws.root}${shortcut}`}
              aria-label={`Workspace ${ws.name}`}
              aria-current={active ? "true" : undefined}
              style={{ color: `hsl(${railHue(ws.root)} 55% var(--rail-tile-lightness))` }}
              className={`flex h-10 w-10 items-center justify-center rounded-lg border text-sm font-semibold transition-colors ${
                active
                  ? "border-accent bg-panel-2"
                  : "border-transparent bg-panel opacity-70 hover:opacity-100"
              }`}
            >
              {monogram(ws.name)}
            </button>
            <button
              onClick={(e) => {
                e.stopPropagation();
                onClose(ws.id);
              }}
              title={`Close ${ws.name}`}
              aria-label={`Close ${ws.name}`}
              className="absolute -right-1 -top-1 hidden h-4 w-4 items-center justify-center rounded-full border border-border bg-panel-2 text-muted hover:text-text group-hover:flex"
            >
              <X className="h-3 w-3" />
            </button>
          </div>
        );
      })}
      <button
        onClick={onAdd}
        title="Open folder… — Ctrl+O"
        aria-label="Open folder"
        className="flex h-10 w-10 items-center justify-center rounded-lg border border-dashed border-border text-muted transition-colors hover:bg-panel-2 hover:text-text"
      >
        <Plus className="h-4 w-4" />
      </button>
    </div>
  );
}
