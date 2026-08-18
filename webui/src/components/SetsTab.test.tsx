import { afterEach, describe, expect, it } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SetsTab from "./SetsTab";
import { renderWithProviders } from "../test/render";
import { installFakeApi, type FakeApi } from "../test/fakeApi";
import type { InstanceInfo } from "../api/types";

let fake: FakeApi;
afterEach(() => fake?.restore());

const managed: InstanceInfo = {
  name: "alpha",
  path: "/repo/apps/alpha",
  managed: true,
  deps: [
    { id: "postgresql", name: "postgresql", version: "15.5.0", repository: "https://example.invalid/charts" },
  ],
};

const direct: InstanceInfo = { name: "legacy", path: "/repo/apps/legacy", managed: false, deps: [] };

describe("SetsTab", () => {
  it("lists the enabled global and per-dependency sets", async () => {
    fake = installFakeApi();
    renderWithProviders(<SetsTab inst={managed} />);

    expect(await screen.findByText("dev")).toBeDefined();
    expect(await screen.findByText("ha-production")).toBeDefined();
  });

  it("enables and disables a global set", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<SetsTab inst={managed} />);

    await screen.findByText("dev");
    await user.type(screen.getByPlaceholderText(/set name/), "prod");
    await user.click(screen.getByTitle("Enable set"));

    await waitFor(() => expect(fake.state.sets.alpha.sets).toContain("prod"));
    expect(await screen.findByText("prod")).toBeDefined();

    await user.click(await screen.findByTitle("Disable set prod"));
    await waitFor(() => expect(fake.state.sets.alpha.sets).not.toContain("prod"));
  });

  it("scopes a set to one dependency", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<SetsTab inst={managed} />);

    await screen.findByText("ha-production");
    await user.type(screen.getByPlaceholderText("add set…"), "canary");
    await user.click(screen.getByTitle("Enable dep set"));

    await waitFor(() => expect(fake.state.sets.alpha.depSets.postgresql).toContain("canary"));
  });

  it("explains that sets do not apply to a direct-mode instance", () => {
    fake = installFakeApi();
    renderWithProviders(<SetsTab inst={direct} />);

    expect(screen.getByText(/Sets are a managed-mode feature/)).toBeDefined();
    // No way to enable one, so nothing can be written.
    expect(screen.queryByTitle("Enable set")).toBeNull();
    expect(fake.requests.some((r) => r.startsWith("POST") || r.startsWith("DELETE"))).toBe(false);
  });

  it("surfaces a rejected set name", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<SetsTab inst={managed} />);

    await screen.findByText("dev");
    fake.failNext("POST", "/api/instances/alpha/sets", 400, 'invalid set name "../evil"');
    await user.type(screen.getByPlaceholderText(/set name/), "../evil");
    await user.click(screen.getByTitle("Enable set"));

    expect(await screen.findByText(/invalid set name/)).toBeDefined();
  });
});
