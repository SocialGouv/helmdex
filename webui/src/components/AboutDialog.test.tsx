import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AboutDialog from "./AboutDialog";
import { resetUpdatesForTests } from "../lib/updates";
import { renderWithProviders } from "../test/render";
import { installFakeApi, type FakeApi } from "../test/fakeApi";
import { installFakeDesktop, type FakeDesktop } from "../test/fakeDesktop";

let fake: FakeApi;
let desktop: FakeDesktop | undefined;

const updateAvailable = {
  current: "v0.5.0",
  latest: "v0.6.0",
  updateAvailable: true,
  releaseUrl: "https://github.com/SocialGouv/helmdex/releases/tag/v0.6.0",
};

beforeEach(() => {
  localStorage.clear();
  resetUpdatesForTests();
});

afterEach(() => {
  desktop?.restore();
  desktop = undefined;
  fake?.restore();
  vi.restoreAllMocks();
});

describe("AboutDialog", () => {
  it("shows the installed version and repo link", async () => {
    fake = installFakeApi();
    renderWithProviders(<AboutDialog onClose={() => {}} />);

    expect(await screen.findByText("v0.5.0 (abc1234)")).toBeDefined();
    expect(screen.getByText("github.com/SocialGouv/helmdex")).toBeDefined();
  });

  it("reports up-to-date after a manual check", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<AboutDialog onClose={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /check for updates/i }));
    expect(await screen.findByText(/up to date/i)).toBeDefined();
  });

  // Browser mode has no binary to replace: the primary action stays a link.
  it("offers the download link when an update is available (browser)", async () => {
    fake = installFakeApi({ updateCheck: updateAvailable });
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    const user = userEvent.setup();
    renderWithProviders(<AboutDialog onClose={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /check for updates/i }));
    await user.click(await screen.findByRole("button", { name: /download v0\.6\.0/i }));

    expect(open).toHaveBeenCalledWith(
      "https://github.com/SocialGouv/helmdex/releases/tag/v0.6.0",
      "_blank",
      "noopener",
    );
  });

  it("self-updates then restarts in the desktop, release page as second option", async () => {
    desktop = installFakeDesktop(["/home/jo/infra"]);
    fake = installFakeApi({ updateCheck: updateAvailable });
    const user = userEvent.setup();
    renderWithProviders(<AboutDialog onClose={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /check for updates/i }));
    // Primary: in-place update. Secondary: the release page link.
    expect(await screen.findByRole("button", { name: /or open the release page/i })).toBeDefined();
    await user.click(screen.getByRole("button", { name: /update to v0\.6\.0 & restart/i }));

    expect(desktop.calls).toContain("ApplyUpdate v0.6.0");
    expect(desktop.calls).toContain("RestartApp");
  });

  // disabled={isPending} only lands at the next React commit: same-task
  // clicks must be stopped by the in-flight guard, or several concurrent
  // binary writes race on disk.
  it("coalesces same-task double clicks into one ApplyUpdate", async () => {
    desktop = installFakeDesktop(["/home/jo/infra"]);
    fake = installFakeApi({ updateCheck: updateAvailable });
    const user = userEvent.setup();
    renderWithProviders(<AboutDialog onClose={() => {}} />);
    await user.click(await screen.findByRole("button", { name: /check for updates/i }));

    const btn = await screen.findByRole("button", { name: /update to v0\.6\.0 & restart/i });
    fireEvent.click(btn);
    fireEvent.click(btn);
    fireEvent.click(btn);

    await waitFor(() =>
      expect(desktop!.calls.filter((c) => c.startsWith("ApplyUpdate")).length).toBe(1),
    );
  });

  it("says the update IS installed when only the restart fails", async () => {
    desktop = installFakeDesktop(["/home/jo/infra"]);
    fake = installFakeApi({ updateCheck: updateAvailable });
    window.go!.main!.App!.RestartApp = async () => {
      throw new Error("relaunch /apps/x.AppImage: permission denied");
    };
    const user = userEvent.setup();
    renderWithProviders(<AboutDialog onClose={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /check for updates/i }));
    await user.click(await screen.findByRole("button", { name: /update to v0\.6\.0/i }));

    const msg = await screen.findByText(/is installed — restart the app manually/);
    expect(msg.textContent).toContain("permission denied");
  });

  it("surfaces a failed self-update and keeps the release page fallback", async () => {
    desktop = installFakeDesktop(["/home/jo/infra"]);
    fake = installFakeApi({ updateCheck: updateAvailable });
    window.go!.main!.App!.ApplyUpdate = async () => {
      throw new Error("automatic update is not supported on macOS yet");
    };
    const user = userEvent.setup();
    renderWithProviders(<AboutDialog onClose={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /check for updates/i }));
    await user.click(await screen.findByRole("button", { name: /update to v0\.6\.0/i }));

    expect(await screen.findByText(/not supported on macOS/)).toBeDefined();
    expect(desktop.calls).not.toContain("RestartApp");
    expect(screen.getByRole("button", { name: /or open the release page/i })).toBeDefined();
  });

  it("surfaces a failing manual check", async () => {
    fake = installFakeApi();
    fake.failNext("GET", "/api/version/check", 502, "check latest release: rate limited");
    const user = userEvent.setup();
    renderWithProviders(<AboutDialog onClose={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /check for updates/i }));
    expect(await screen.findByText(/rate limited/)).toBeDefined();
  });

  it("toggles the automatic check preference", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<AboutDialog onClose={() => {}} />);

    const checkbox = screen.getByRole("checkbox", { name: /check for new versions/i });
    expect((checkbox as HTMLInputElement).checked).toBe(true);
    await user.click(checkbox);
    expect(localStorage.getItem("helmdex.updates.autoCheck")).toBe("off");
    expect((checkbox as HTMLInputElement).checked).toBe(false);
  });
});
