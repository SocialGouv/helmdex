import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Dashboard from "./Dashboard";
import { renderWithProviders } from "../test/render";
import { installFakeApi, type FakeApi } from "../test/fakeApi";

let fake: FakeApi;
afterEach(() => {
  fake?.restore();
  vi.restoreAllMocks();
});

describe("Dashboard", () => {
  it("lists instances with their dependency counts and mode", async () => {
    fake = installFakeApi();
    renderWithProviders(<Dashboard />);

    expect(await screen.findByText("alpha")).toBeDefined();
    expect(screen.getByText("2 dependencies")).toBeDefined();

    // A direct-mode instance is labelled as such.
    const legacy = screen.getByText("legacy").closest("div.group")!;
    expect(within(legacy as HTMLElement).getByText("direct")).toBeDefined();
    expect(within(legacy as HTMLElement).getByText("0 dependencies")).toBeDefined();
  });

  it("surfaces a chart error instead of a dependency count", async () => {
    fake = installFakeApi({
      instances: [
        {
          name: "broken",
          path: "/repo/apps/broken",
          managed: true,
          deps: [],
          depError: "yaml: line 3: mapping values are not allowed",
        },
      ],
    });
    renderWithProviders(<Dashboard />);

    expect(await screen.findByText(/Chart error: yaml: line 3/)).toBeDefined();
  });

  it("creates an instance and navigates to it", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    const { location } = renderWithProviders(<Dashboard />);

    await screen.findByText("alpha");
    await user.click(screen.getByRole("button", { name: /new instance/i }));
    await user.type(await screen.findByPlaceholderText(/instance name/i), "bravo");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(location()).toBe("/instances/bravo"));
    expect(fake.state.instances.map((i) => i.name)).toContain("bravo");
  });

  it("reports why a creation was refused and keeps the dialog open", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<Dashboard />);

    await screen.findByText("alpha");
    await user.click(screen.getByRole("button", { name: /new instance/i }));
    await user.type(await screen.findByPlaceholderText(/instance name/i), "alpha");
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(await screen.findByText(/instance already exists/i)).toBeDefined();
    expect(screen.getByPlaceholderText(/instance name/i)).toBeDefined();
  });

  it("deletes only after the confirmation is accepted", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderWithProviders(<Dashboard />);

    await screen.findByText("alpha");
    await user.click(screen.getByTitle("Delete alpha"));
    expect(confirm).toHaveBeenCalled();
    expect(fake.state.instances.map((i) => i.name)).toContain("alpha");

    confirm.mockReturnValue(true);
    await user.click(screen.getByTitle("Delete alpha"));
    await waitFor(() => expect(fake.state.instances.map((i) => i.name)).not.toContain("alpha"));
    await waitFor(() => expect(screen.queryByText("alpha")).toBeNull());
  });

  it("surfaces a failed deletion", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithProviders(<Dashboard />);

    await screen.findByText("alpha");
    fake.failNext("DELETE", "/api/instances/alpha", 500, "remove apps/alpha: directory not empty");
    await user.click(screen.getByTitle("Delete alpha"));

    expect(await screen.findByText(/directory not empty/)).toBeDefined();
    expect(fake.state.instances.map((i) => i.name)).toContain("alpha");
  });

  it("offers templates as a starting point", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<Dashboard />);

    await user.click(await screen.findByText("review"));
    expect(await screen.findByText(/New instance from template review/)).toBeDefined();
  });

  it("invites the user to create one when the repo is empty", async () => {
    fake = installFakeApi({ instances: [], templates: [] });
    renderWithProviders(<Dashboard />);

    expect(await screen.findByText(/No instances yet/)).toBeDefined();
  });

  it("reports a failure to list instances", async () => {
    fake = installFakeApi();
    fake.failNext("GET", "/api/instances", 500, "read apps dir: permission denied");
    renderWithProviders(<Dashboard />);

    expect(await screen.findByText(/permission denied/)).toBeDefined();
  });
});
