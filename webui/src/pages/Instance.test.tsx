import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Route, Switch } from "wouter";
import InstancePage from "./Instance";
import { renderWithProviders } from "../test/render";
import { installFakeApi, type FakeApi } from "../test/fakeApi";

let fake: FakeApi;
afterEach(() => {
  fake?.restore();
  vi.restoreAllMocks();
});

// The page reads :name and :tab from the route, so it has to be mounted the
// way App.tsx mounts it.
function renderInstance(name = "alpha", tab?: string) {
  return renderWithProviders(
    <Switch>
      <Route path="/instances/:name" component={InstancePage} />
      <Route path="/instances/:name/:tab" component={InstancePage} />
    </Switch>,
    { route: `/instances/${name}${tab ? `/${tab}` : ""}` },
  );
}

describe("Instance page", () => {
  it("opens on the dependencies tab", async () => {
    fake = installFakeApi();
    renderInstance();

    expect(await screen.findByText("2 dependencies in Chart.yaml")).toBeDefined();
    expect(screen.getByRole("tab", { name: "Dependencies" }).getAttribute("aria-selected")).toBe("true");
  });

  it("switches tabs through the URL", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    const { location } = renderInstance();

    await screen.findByText("2 dependencies in Chart.yaml");
    await user.click(screen.getByRole("tab", { name: "sets" }));

    await waitFor(() => expect(location()).toBe("/instances/alpha/sets"));
    expect(await screen.findByText("Global sets")).toBeDefined();
  });

  it("labels a direct-mode instance", async () => {
    fake = installFakeApi();
    renderInstance("legacy");

    expect(await screen.findByText("direct")).toBeDefined();
  });

  // Apply and Relock hit the same endpoint and differ only by payload, so the
  // payload is the whole assertion: without it the two buttons are
  // indistinguishable and either could be wired to the other.
  it("applies and relocks through the same endpoint with different payloads", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderInstance();

    await screen.findByText("2 dependencies in Chart.yaml");

    await user.click(screen.getByRole("button", { name: /^Apply$/ }));
    await waitFor(() =>
      expect(fake.lastCall("POST", "/api/instances/alpha/apply")?.body).toEqual({ relock: false }),
    );

    await user.click(screen.getByRole("button", { name: "Relock" }));
    await waitFor(() =>
      expect(fake.lastCall("POST", "/api/instances/alpha/apply")?.body).toEqual({ relock: true }),
    );
  });

  it("surfaces a failed apply", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderInstance();

    await screen.findByText("2 dependencies in Chart.yaml");
    fake.failNext("POST", "/api/instances/alpha/apply", 500, "helm dependency update failed: registry unreachable");
    await user.click(screen.getByRole("button", { name: /^Apply$/ }));

    expect(await screen.findByText(/registry unreachable/)).toBeDefined();
  });

  it("renames and follows the instance to its new URL", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    const { location } = renderInstance();

    await screen.findByText("2 dependencies in Chart.yaml");
    await user.click(screen.getByTitle("Rename"));

    const input = await screen.findByDisplayValue("alpha");
    await user.clear(input);
    await user.type(input, "bravo");
    await user.click(screen.getByRole("button", { name: "Rename" }));

    await waitFor(() => expect(location()).toBe("/instances/bravo/deps"));
    expect(fake.state.instances.map((i) => i.name)).toContain("bravo");
  });

  it("surfaces a refused rename and stays put", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    const { location } = renderInstance();

    await screen.findByText("2 dependencies in Chart.yaml");
    await user.click(screen.getByTitle("Rename"));
    const input = await screen.findByDisplayValue("alpha");
    await user.clear(input);
    await user.type(input, "legacy");
    await user.click(screen.getByRole("button", { name: "Rename" }));

    expect(await screen.findByText(/instance already exists/)).toBeDefined();
    expect(location()).toBe("/instances/alpha");
  });

  it("reports an instance that no longer exists", async () => {
    fake = installFakeApi();
    renderInstance("ghost");

    expect(await screen.findByText(/instance "ghost" not found/)).toBeDefined();
  });
});
