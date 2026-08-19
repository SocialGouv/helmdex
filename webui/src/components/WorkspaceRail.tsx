import { ChevronsLeft, ChevronsRight, Plus, X } from "lucide-react";
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

// parentDir is the expanded mode's detail line: it disambiguates two open
// folders sharing a basename.
export function parentDir(root: string): string {
  const i = Math.max(root.lastIndexOf("/"), root.lastIndexOf("\\"));
  if (i <= 0) return "/";
  return root.slice(0, i);
}

interface Props {
  state: WorkspacesState;
  expanded: boolean;
  onActivate: (id: string) => void;
  onClose: (id: string) => void;
  onAdd: () => void;
  onToggleExpanded: () => void;
}

// The vertical folder-tabs rail of the desktop shell: one tile per open
// folder, "+" to open another one. Two display modes: compact monograms, or
// expanded rows with the folder name and its parent directory.
export default function WorkspaceRail({
  state,
  expanded,
  onActivate,
  onClose,
  onAdd,
  onToggleExpanded,
}: Props) {
  return (
    <div
      className={`flex shrink-0 flex-col border-r border-border bg-bg py-3 transition-[width] duration-150 ${
        expanded ? "w-56 px-2" : "w-14"
      }`}
    >
      <div
        className={`flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto ${expanded ? "" : "items-center"}`}
      >
        {state.workspaces.map((ws, i) => {
          const active = ws.id === state.activeId;
          const shortcut = i < 9 ? ` — Ctrl+${i + 1}` : "";
          const mono = (
            <span
              style={{ color: `hsl(${railHue(ws.root)} 55% var(--rail-tile-lightness))` }}
              className={`flex shrink-0 items-center justify-center font-semibold ${
                expanded ? "h-8 w-8 rounded-md bg-bg text-xs" : "h-10 w-10 text-sm"
              }`}
            >
              {monogram(ws.name)}
            </span>
          );
          return (
            <div key={ws.id} className="group relative shrink-0">
              <button
                onClick={() => onActivate(ws.id)}
                onAuxClick={(e) => {
                  if (e.button === 1) onClose(ws.id);
                }}
                title={`${ws.root}${shortcut}`}
                aria-label={`Workspace ${ws.name}`}
                aria-current={active ? "true" : undefined}
                className={`rounded-lg border transition-colors ${
                  expanded ? "flex w-full items-center gap-2.5 px-2 py-1.5 text-left" : "flex"
                } ${
                  active
                    ? "border-accent bg-panel-2"
                    : "border-transparent bg-panel opacity-80 hover:opacity-100"
                }`}
              >
                {mono}
                {expanded && (
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">{ws.name}</span>
                    <span className="block truncate text-[10px] text-muted">
                      {parentDir(ws.root)}
                    </span>
                  </span>
                )}
              </button>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onClose(ws.id);
                }}
                title={`Close ${ws.name}`}
                aria-label={`Close ${ws.name}`}
                className={`absolute hidden h-4 w-4 items-center justify-center rounded-full border border-border bg-panel-2 text-muted hover:text-text group-hover:flex ${
                  expanded ? "right-1.5 top-1/2 -translate-y-1/2" : "-right-1 -top-1"
                }`}
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
          className={`flex shrink-0 items-center justify-center rounded-lg border border-dashed border-border text-muted transition-colors hover:bg-panel-2 hover:text-text ${
            expanded ? "w-full gap-2 px-2 py-2 text-sm" : "h-10 w-10"
          }`}
        >
          <Plus className="h-4 w-4" />
          {expanded && "Open folder…"}
        </button>
      </div>
      <button
        onClick={onToggleExpanded}
        title={`${expanded ? "Collapse" : "Expand"} the folder rail — Ctrl+B`}
        aria-label={expanded ? "Collapse the folder rail" : "Expand the folder rail"}
        className={`mt-2 flex shrink-0 items-center justify-center rounded-md py-1 text-muted transition-colors hover:bg-panel-2 hover:text-text ${
          expanded ? "w-full" : "mx-auto w-10"
        }`}
      >
        {expanded ? <ChevronsLeft className="h-4 w-4" /> : <ChevronsRight className="h-4 w-4" />}
      </button>
    </div>
  );
}
