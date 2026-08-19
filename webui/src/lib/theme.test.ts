import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { initTheme, resolvedTheme, setThemePref, themePref } from "./theme";

function stubSystemScheme(dark: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query.includes("dark") ? dark : !dark,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

beforeEach(() => {
  localStorage.clear();
  delete document.documentElement.dataset.theme;
});

afterEach(() => vi.unstubAllGlobals());

describe("theme", () => {
  it("defaults to system, falling back to dark without matchMedia", () => {
    expect(themePref()).toBe("system");
    initTheme();
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("follows the OS scheme in system mode", () => {
    stubSystemScheme(false);
    initTheme();
    expect(document.documentElement.dataset.theme).toBe("light");

    stubSystemScheme(true);
    setThemePref("system");
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("applies and persists an explicit preference", () => {
    setThemePref("light");
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(localStorage.getItem("helmdex.theme")).toBe("light");
    expect(resolvedTheme()).toBe("light");

    setThemePref("dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("clears the stored preference when returning to system", () => {
    setThemePref("light");
    setThemePref("system");
    expect(localStorage.getItem("helmdex.theme")).toBeNull();
    expect(themePref()).toBe("system");
  });
});
