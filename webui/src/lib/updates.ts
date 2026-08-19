import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { UpdateCheck } from "../api/types";

// New-version checking: an explicit check from the About dialog, plus an
// opt-out daily automatic check. The preference and throttle live in
// localStorage; the last result lives in-memory for the session.

const AUTO_KEY = "helmdex.updates.autoCheck"; // "off" opts out; default on
const LAST_AT_KEY = "helmdex.updates.lastCheckAt"; // epoch ms of last check
const LAST_RESULT_KEY = "helmdex.updates.lastResult"; // persisted UpdateCheck
const CHANGE_EVENT = "helmdex:updates-changed";
const DAY_MS = 24 * 60 * 60 * 1000;

// Last check result. Persisted: without it the update badge would vanish on
// every relaunch until the daily throttle allows the next check.
let lastCheck: UpdateCheck | null | undefined;

function loadLastCheck(): UpdateCheck | null {
  if (lastCheck !== undefined) return lastCheck;
  const raw = localStorage.getItem(LAST_RESULT_KEY);
  try {
    lastCheck = raw ? (JSON.parse(raw) as UpdateCheck) : null;
  } catch {
    // A corrupt cache entry is not worth an error — drop it.
    lastCheck = null;
  }
  return lastCheck;
}

export function autoCheckEnabled(): boolean {
  return localStorage.getItem(AUTO_KEY) !== "off";
}

export function setAutoCheckEnabled(on: boolean) {
  if (on) {
    localStorage.removeItem(AUTO_KEY);
  } else {
    localStorage.setItem(AUTO_KEY, "off");
  }
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

// Single-flight: the throttle stamp is only written on resolution, so rapid
// App remounts (desktop tab switches, StrictMode double effects) would
// otherwise fire parallel GitHub-bound checks.
let inflight: Promise<UpdateCheck> | undefined;

export function checkForUpdates(): Promise<UpdateCheck> {
  if (inflight) return inflight;
  inflight = api
    .versionCheck()
    .then((result) => {
      lastCheck = result;
      localStorage.setItem(LAST_RESULT_KEY, JSON.stringify(result));
      localStorage.setItem(LAST_AT_KEY, String(Date.now()));
      window.dispatchEvent(new Event(CHANGE_EVENT));
      return result;
    })
    .finally(() => {
      inflight = undefined;
    });
  return inflight;
}

// maybeAutoCheck runs at most once a day when auto-checking is on. A failed
// AUTOMATIC check only logs: an offline launch must not open with an error
// banner — the About dialog's manual check surfaces errors properly.
export function maybeAutoCheck() {
  if (!autoCheckEnabled()) return;
  const lastAt = Number(localStorage.getItem(LAST_AT_KEY) ?? 0);
  if (Date.now() - lastAt < DAY_MS) return;
  void checkForUpdates().catch((err: unknown) => {
    console.warn("helmdex: automatic update check failed:", err);
  });
}

export function lastUpdateCheck(): UpdateCheck | null {
  return loadLastCheck();
}

export interface UpdatesStatus {
  autoCheck: boolean;
  check: UpdateCheck | null;
}

function snapshot(): UpdatesStatus {
  return { autoCheck: autoCheckEnabled(), check: loadLastCheck() };
}

export function useUpdatesStatus(): UpdatesStatus {
  const [status, setStatus] = useState<UpdatesStatus>(snapshot);
  useEffect(() => {
    const refresh = () => setStatus(snapshot());
    window.addEventListener(CHANGE_EVENT, refresh);
    // A check can resolve between the initial render and this effect (tab
    // switches remount App): re-snapshot so that event is not lost.
    refresh();
    return () => window.removeEventListener(CHANGE_EVENT, refresh);
  }, []);
  return status;
}

// Test hook: forget the in-memory state so the next read reloads storage.
export function resetUpdatesForTests() {
  lastCheck = undefined;
  inflight = undefined;
}
