import { useEffect, useState } from "react";

// Theme preference: dark, light, or follow the OS ("system", the default).
// The resolved theme is applied as <html data-theme="dark|light">, which the
// CSS palette in index.css keys on.

export type ThemePref = "dark" | "light" | "system";

const STORAGE_KEY = "helmdex.theme";
const CHANGE_EVENT = "helmdex:theme-changed";

export function themePref(): ThemePref {
  const v = localStorage.getItem(STORAGE_KEY);
  return v === "dark" || v === "light" ? v : "system";
}

function systemPrefersDark(): boolean {
  // jsdom has no matchMedia; dark is the app's historical default.
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? true;
}

export function resolvedTheme(): "dark" | "light" {
  const pref = themePref();
  if (pref === "system") return systemPrefersDark() ? "dark" : "light";
  return pref;
}

function apply() {
  document.documentElement.dataset.theme = resolvedTheme();
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

export function setThemePref(pref: ThemePref) {
  if (pref === "system") {
    localStorage.removeItem(STORAGE_KEY);
  } else {
    localStorage.setItem(STORAGE_KEY, pref);
  }
  apply();
}

// initTheme applies the stored preference and keeps "system" in sync with the
// OS. Call once before rendering.
export function initTheme() {
  apply();
  window.matchMedia?.("(prefers-color-scheme: dark)").addEventListener("change", () => {
    if (themePref() === "system") apply();
  });
}

function subscribe(fn: () => void): () => void {
  window.addEventListener(CHANGE_EVENT, fn);
  return () => window.removeEventListener(CHANGE_EVENT, fn);
}

export function useThemePref(): [ThemePref, (p: ThemePref) => void] {
  const [pref, setPref] = useState<ThemePref>(themePref);
  useEffect(() => subscribe(() => setPref(themePref())), []);
  return [pref, setThemePref];
}

// Monaco theme id for the current resolved theme.
export function useEditorTheme(): "vs-dark" | "light" {
  const [theme, setTheme] = useState(resolvedTheme);
  useEffect(() => subscribe(() => setTheme(resolvedTheme())), []);
  return theme === "dark" ? "vs-dark" : "light";
}
