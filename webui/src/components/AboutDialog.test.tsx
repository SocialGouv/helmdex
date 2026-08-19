import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AboutDialog from "./AboutDialog";
import { resetUpdatesForTests } from "../lib/updates";
import { renderWithProviders } from "../test/render";
import { installFakeApi, type FakeApi } from "../test/fakeApi";

let fake: FakeApi;

beforeEach(() => {
  localStorage.clear();
  resetUpdatesForTests();
});

afterEach(() => {
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

  it("offers the download link when an update is available", async () => {
    fake = installFakeApi({
      updateCheck: {
        current: "v0.5.0",
        latest: "v0.6.0",
        updateAvailable: true,
        releaseUrl: "https://github.com/SocialGouv/helmdex/releases/tag/v0.6.0",
      },
    });
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
