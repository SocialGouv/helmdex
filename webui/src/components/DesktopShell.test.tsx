import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import DesktopShell from "./DesktopShell";
import { setApiBase } from "../api/client";
import { installFakeApi, type FakeApi } from "../test/fakeApi";
import { installFakeDesktop, type FakeDesktop } from "../test/fakeDesktop";

/**
 * The shell is the desktop-only glue: it must point every API call of the
 * active tab at that workspace's /ws/<id> prefix, mirror the Go-owned tab
 * state, and fall back to the Welcome screen when nothing is open.
 */

// jsdom has no EventSource (EventsIndicator mounts inside App).
class FakeEventSource {
  static urls: string[] = [];
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  constructor(public url: string) {
    FakeEventSource.urls.push(url);
  }
  close() {}
}

let fake: FakeApi;
let desktop: FakeDesktop;

beforeEach(() => {
  FakeEventSource.urls = [];
  vi.stubGlobal("EventSource", FakeEventSource);
  window.history.replaceState(null, "", "/");
  localStorage.clear();
});

afterEach(() => {
  desktop?.restore();
  fake?.restore();
  vi.unstubAllGlobals();
  setApiBase("");
});

describe("DesktopShell", () => {
  it("shows the Welcome screen when no folder is open, then the rail after opening one", async () => {
    desktop = installFakeDesktop([]);
    fake = installFakeApi();
    const user = userEvent.setup();
    render(<DesktopShell />);

    const open = await screen.findByRole("button", { name: /open folder…/i });
    desktop.pickFolder("/home/jo/infra");
    await user.click(open);

    expect(await screen.findByRole("button", { name: "Workspace infra" })).toBeDefined();
    expect(desktop.calls).toContain("OpenRepoDialog");
    // The freshly opened tab is served through its workspace prefix.
    await waitFor(() => expect(fake.requests).toContain("GET /ws/w1/api/repo"));
  });

  it("serves the active tab through its workspace prefix, SSE included", async () => {
    desktop = installFakeDesktop(["/home/jo/infra", "/home/jo/apps"]);
    fake = installFakeApi();
    render(<DesktopShell />);

    expect(await screen.findByText("Dashboard")).toBeDefined();
    await waitFor(() => expect(fake.requests).toContain("GET /ws/w1/api/repo"));
    expect(fake.requests.some((r) => r.includes("/api/") && !r.includes("/ws/w1/"))).toBe(false);
    await waitFor(() => expect(FakeEventSource.urls).toContain("/ws/w1/api/events"));
  });

  it("switches tabs: binding called, API traffic moves to the new prefix", async () => {
    desktop = installFakeDesktop(["/home/jo/infra", "/home/jo/apps"]);
    fake = installFakeApi();
    const user = userEvent.setup();
    render(<DesktopShell />);

    await screen.findByText("Dashboard");
    await user.click(screen.getByRole("button", { name: "Workspace apps" }));

    expect(desktop.calls).toContain("SetActiveWorkspace w2");
    await waitFor(() => expect(fake.requests).toContain("GET /ws/w2/api/repo"));
    expect(
      screen.getByRole("button", { name: "Workspace apps" }).getAttribute("aria-current"),
    ).toBe("true");
  });

  it("supports Ctrl+1..9 and Ctrl+PageDown tab shortcuts", async () => {
    desktop = installFakeDesktop(["/home/jo/infra", "/home/jo/apps"]);
    fake = installFakeApi();
    render(<DesktopShell />);
    await screen.findByText("Dashboard");

    fireEvent.keyDown(window, { key: "2", ctrlKey: true });
    await waitFor(() => expect(desktop.calls).toContain("SetActiveWorkspace w2"));

    // From w2, PageDown cycles back around to w1.
    fireEvent.keyDown(window, { key: "PageDown", ctrlKey: true });
    await waitFor(() => expect(desktop.calls).toContain("SetActiveWorkspace w1"));
  });

  it("does not steal Ctrl+digit chords from text inputs", async () => {
    desktop = installFakeDesktop(["/home/jo/infra", "/home/jo/apps"]);
    fake = installFakeApi();
    render(<DesktopShell />);
    await screen.findByText("Dashboard");

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /new instance/i }));
    const input = await screen.findByPlaceholderText(/instance name/i);
    input.focus();

    fireEvent.keyDown(input, { key: "2", ctrlKey: true });
    expect(desktop.calls).not.toContain("SetActiveWorkspace w2");
  });

  it("toggles the rail between compact and expanded with Ctrl+B, persisted", async () => {
    desktop = installFakeDesktop(["/home/jo/infra"]);
    fake = installFakeApi();
    render(<DesktopShell />);
    await screen.findByText("Dashboard");
    expect(screen.queryByText("/home/jo")).toBeNull();

    fireEvent.keyDown(window, { key: "b", ctrlKey: true });
    expect(await screen.findByText("/home/jo")).toBeDefined();
    expect(localStorage.getItem("helmdex.rail")).toBe("expanded");

    fireEvent.keyDown(window, { key: "b", ctrlKey: true });
    await waitFor(() => expect(screen.queryByText("/home/jo")).toBeNull());
    expect(localStorage.getItem("helmdex.rail")).toBeNull();
  });

  it("opens the folder dialog with Ctrl+O", async () => {
    desktop = installFakeDesktop(["/home/jo/infra"]);
    fake = installFakeApi();
    render(<DesktopShell />);
    await screen.findByText("Dashboard");

    fireEvent.keyDown(window, { key: "o", ctrlKey: true });
    await waitFor(() => expect(desktop.calls).toContain("OpenRepoDialog"));
  });

  it("closes a tab and activates its neighbor; closing the last shows Welcome", async () => {
    desktop = installFakeDesktop(["/home/jo/infra", "/home/jo/apps"]);
    fake = installFakeApi();
    const user = userEvent.setup();
    render(<DesktopShell />);
    await screen.findByText("Dashboard");

    await user.click(screen.getByRole("button", { name: "Close infra" }));
    expect(desktop.calls).toContain("CloseWorkspace w1");
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Workspace apps" }).getAttribute("aria-current"),
      ).toBe("true"),
    );
    await waitFor(() => expect(fake.requests).toContain("GET /ws/w2/api/repo"));

    await user.click(screen.getByRole("button", { name: "Close apps" }));
    expect(await screen.findByRole("button", { name: /open folder…/i })).toBeDefined();
  });

  it("surfaces a failing binding instead of swallowing it", async () => {
    desktop = installFakeDesktop(["/home/jo/infra"]);
    fake = installFakeApi();
    render(<DesktopShell />);
    await screen.findByText("Dashboard");

    window.go!.main!.App!.OpenRepoDialog = async () => {
      throw new Error("picker exploded");
    };
    fireEvent.keyDown(window, { key: "o", ctrlKey: true });

    expect(await screen.findByText(/picker exploded/)).toBeDefined();
  });
});
