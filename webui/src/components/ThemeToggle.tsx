import { Monitor, Moon, Sun } from "lucide-react";
import { useThemePref, type ThemePref } from "../lib/theme";

const OPTIONS: { pref: ThemePref; label: string; Icon: typeof Sun }[] = [
  { pref: "system", label: "System theme", Icon: Monitor },
  { pref: "light", label: "Light theme", Icon: Sun },
  { pref: "dark", label: "Dark theme", Icon: Moon },
];

// Compact icon-only segmented control for the theme preference.
export default function ThemeToggle() {
  const [pref, setPref] = useThemePref();
  return (
    <div className="inline-flex items-center gap-0.5 rounded-md border border-border bg-panel p-0.5">
      {OPTIONS.map(({ pref: p, label, Icon }) => (
        <button
          key={p}
          onClick={() => setPref(p)}
          title={label}
          aria-label={label}
          aria-pressed={pref === p}
          className={`rounded p-1.5 transition-colors ${
            pref === p ? "bg-panel-2 text-text" : "text-muted hover:text-text"
          }`}
        >
          <Icon className="h-3.5 w-3.5" />
        </button>
      ))}
    </div>
  );
}
