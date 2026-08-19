import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  autoCheckEnabled,
  checkForUpdates,
  lastUpdateCheck,
  maybeAutoCheck,
  resetUpdatesForTests,
  setAutoCheckEnabled,
} from "./updates";
import { installFakeApi, type FakeApi } from "../test/fakeApi";

let fake: FakeApi;

beforeEach(() => {
  localStorage.clear();
  resetUpdatesForTests();
  fake = installFakeApi();
});

afterEach(() => fake.restore());

const checkRequests = () => fake.requests.filter((r) => r.includes("/api/version/check"));

describe("updates", () => {
  it("auto-check defaults to on and the opt-out persists", () => {
    expect(autoCheckEnabled()).toBe(true);
    setAutoCheckEnabled(false);
    expect(autoCheckEnabled()).toBe(false);
    expect(localStorage.getItem("helmdex.updates.autoCheck")).toBe("off");
    setAutoCheckEnabled(true);
    expect(localStorage.getItem("helmdex.updates.autoCheck")).toBeNull();
  });

  it("checkForUpdates records the result and stamps the throttle", async () => {
    const res = await checkForUpdates();
    expect(res.latest).toBe("v0.5.0");
    expect(Number(localStorage.getItem("helmdex.updates.lastCheckAt"))).toBeGreaterThan(0);
  });

  // Desktop tab switches remount App rapidly; before the first check
  // resolves the throttle stamp does not exist yet — the in-flight guard
  // must keep that from fanning out into parallel GitHub-bound calls.
  it("coalesces concurrent checks into one request", async () => {
    maybeAutoCheck();
    maybeAutoCheck();
    maybeAutoCheck();
    await checkForUpdates();
    expect(checkRequests().length).toBe(1);
  });

  // The badge must survive a relaunch: the throttle would otherwise hide a
  // known update for up to a day.
  it("the last result is persisted and reloaded", async () => {
    await checkForUpdates();
    resetUpdatesForTests(); // simulate a fresh page: memory gone, storage kept
    expect(lastUpdateCheck()?.latest).toBe("v0.5.0");

    localStorage.setItem("helmdex.updates.lastResult", "{corrupt");
    resetUpdatesForTests();
    expect(lastUpdateCheck()).toBeNull();
  });

  it("maybeAutoCheck runs at most once a day and respects the opt-out", async () => {
    maybeAutoCheck();
    // The throttle stamp is written when the check resolves.
    await vi.waitFor(() =>
      expect(localStorage.getItem("helmdex.updates.lastCheckAt")).not.toBeNull(),
    );
    expect(checkRequests().length).toBe(1);

    // Fresh stamp → throttled.
    maybeAutoCheck();
    expect(checkRequests().length).toBe(1);

    // Stale stamp but opted out → no call.
    localStorage.setItem("helmdex.updates.lastCheckAt", "1");
    setAutoCheckEnabled(false);
    maybeAutoCheck();
    expect(checkRequests().length).toBe(1);

    // Stale stamp and opted in → runs again.
    setAutoCheckEnabled(true);
    maybeAutoCheck();
    await vi.waitFor(() => expect(checkRequests().length).toBe(2));
  });
});
