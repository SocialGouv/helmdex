import { useState } from "react";
import { LayoutGrid, List } from "lucide-react";

export type ViewMode = "grid" | "list";

// Per-page display mode (cards grid ⇄ compact list), persisted locally.
export function useViewMode(page: string): [ViewMode, (v: ViewMode) => void] {
  const key = `helmdex.view.${page}`;
  const [mode, setMode] = useState<ViewMode>(() =>
    localStorage.getItem(key) === "list" ? "list" : "grid",
  );
  return [
    mode,
    (v: ViewMode) => {
      localStorage.setItem(key, v);
      setMode(v);
    },
  ];
}

// Compact icon-only segmented control for the display mode.
export default function ViewToggle({
  value,
  onChange,
}: {
  value: ViewMode;
  onChange: (v: ViewMode) => void;
}) {
  const options = [
    { mode: "grid" as const, label: "Grid view", Icon: LayoutGrid },
    { mode: "list" as const, label: "List view", Icon: List },
  ];
  return (
    <div className="inline-flex items-center gap-0.5 rounded-md border border-border bg-panel p-0.5">
      {options.map(({ mode, label, Icon }) => (
        <button
          key={mode}
          onClick={() => onChange(mode)}
          title={label}
          aria-label={label}
          aria-pressed={value === mode}
          className={`rounded p-1.5 transition-colors ${
            value === mode ? "bg-panel-2 text-text" : "text-muted hover:text-text"
          }`}
        >
          <Icon className="h-4 w-4" />
        </button>
      ))}
    </div>
  );
}
