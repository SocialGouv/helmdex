import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import DepsTab from "./DepsTab";
import { renderWithProviders } from "../test/render";
import { installFakeApi, type FakeApi } from "../test/fakeApi";
import type { InstanceInfo } from "../api/types";

let fake: FakeApi;
afterEach(() => {
  fake?.restore();
  vi.restoreAllMocks();
});

function managedInstance(): InstanceInfo {
  return {
    name: "alpha",
    path: "/repo/apps/alpha",
    managed: true,
    deps: [
      {
        id: "postgresql",
        name: "postgresql",
        version: "15.5.0",
        repository: "https://example.invalid/charts",
        sourceKind: "catalog",
        catalogID: "bitnami-postgresql-15.5.0",
        catalogSource: "Example",
      },
      {
        id: "nginx",
        name: "nginx",
        version: "15.0.0",
        repository: "https://example.invalid/charts",
        sourceKind: "arbitrary",
      },
    ],
  };
}

describe("DepsTab", () => {
  it("shows each dependency with its pinned version and source", async () => {
    fake = installFakeApi();
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    expect(screen.getByText("2 dependencies in Chart.yaml")).toBeDefined();

    // The chart name repeats in the ID and Chart columns; the row is what matters.
    const pgRow = screen.getAllByText("postgresql")[0].closest("tr")!;
    expect(within(pgRow).getByText("15.5.0")).toBeDefined();
    // Catalog-attached dependencies are badged with their source.
    expect(within(pgRow).getByText("CAT Example")).toBeDefined();

    const nginxRow = screen.getAllByText("nginx")[0].closest("tr")!;
    expect(within(nginxRow).getByText("15.0.0")).toBeDefined();
    expect(within(nginxRow).getByText("ARB")).toBeDefined();
  });

  it("says so when there is nothing to show", () => {
    fake = installFakeApi();
    renderWithProviders(<DepsTab inst={{ ...managedInstance(), deps: [] }} />);
    expect(screen.getByText("No dependencies yet.")).toBeDefined();
  });

  it("removes a dependency only after confirmation", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    await user.click(screen.getAllByTitle("Remove")[1]);
    expect(confirm).toHaveBeenCalled();
    expect(fake.requests.some((r) => r.startsWith("DELETE"))).toBe(false);

    confirm.mockReturnValue(true);
    await user.click(screen.getAllByTitle("Remove")[1]);
    await waitFor(() =>
      expect(fake.requests).toContain("DELETE /api/instances/alpha/deps/nginx"),
    );
  });

  it("offers detach only for catalog-attached dependencies", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    // One catalog dependency, so exactly one detach action.
    const detachButtons = screen.getAllByTitle("Detach from catalog");
    expect(detachButtons).toHaveLength(1);

    await user.click(detachButtons[0]);
    await waitFor(() =>
      expect(fake.requests).toContain("POST /api/instances/alpha/deps/postgresql/detach"),
    );
  });

  it("surfaces a refused detach", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    fake.failNext("POST", "/api/instances/alpha/deps/postgresql/detach", 400, 'dependency "postgresql" is not catalog-attached');
    await user.click(screen.getByTitle("Detach from catalog"));

    expect(await screen.findByText(/is not catalog-attached/)).toBeDefined();
  });

  it("inspects a dependency's readme, values and schema", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    await user.click(screen.getAllByTitle(/Inspect readme/)[1]);
    expect(await screen.findByText("nginx@15.0.0")).toBeDefined();
    // Readme is rendered as markdown, so the heading becomes a heading.
    expect(await screen.findByRole("heading", { name: "nginx" })).toBeDefined();

    await user.click(screen.getByRole("button", { name: "values" }));
    expect(await screen.findByText(/replicaCount: 1/)).toBeDefined();

    await user.click(screen.getByRole("button", { name: "schema" }));
    expect(await screen.findByText(/"replicaCount"/)).toBeDefined();
  });

  it("reports when an inspect artifact cannot be fetched", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    await user.click(screen.getAllByTitle(/Inspect readme/)[0]);
    expect(await screen.findByText(/no readme available for postgresql/)).toBeDefined();
  });

  it("warms the sibling inspect tabs after the first one loads", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    // Open the readme tab (nginx); do not touch values/schema.
    await user.click(screen.getAllByTitle(/Inspect readme/)[1]);
    await screen.findByRole("heading", { name: "nginx" });

    // The other two artifacts are prefetched from the same cached archive,
    // so switching tabs won't flash "Loading (may pull the chart)".
    await waitFor(() => {
      const reqs = fake.requests.join("\n");
      expect(reqs).toContain("/deps/nginx/inspect?kind=values");
      expect(reqs).toContain("/deps/nginx/inspect?kind=schema");
    });
  });

  it("warms sibling tabs even when the first tab's artifact is absent (404)", async () => {
    fake = installFakeApi();
    // The default readme tab 404s (chart ships no README) — the prefetch must
    // still fire so values/schema don't reload on switch.
    fake.failNext(
      "GET",
      "/api/instances/alpha/deps/nginx/inspect",
      404,
      "this chart does not ship a README file",
    );
    const user = userEvent.setup();
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    await user.click(screen.getAllByTitle(/Inspect readme/)[1]);
    await screen.findByText(/does not ship a README file/);

    await waitFor(() => {
      const reqs = fake.requests.join("\n");
      expect(reqs).toContain("/deps/nginx/inspect?kind=values");
      expect(reqs).toContain("/deps/nginx/inspect?kind=schema");
    });
  });

  it("shows a genuinely absent artifact (404) as muted information, not an error", async () => {
    fake = installFakeApi();
    fake.failNext(
      "GET",
      "/api/instances/alpha/deps/postgresql/inspect",
      404,
      "this chart does not ship a README file",
    );
    const user = userEvent.setup();
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    await user.click(screen.getAllByTitle(/Inspect readme/)[0]);
    const el = await screen.findByText(/does not ship a README file/);
    expect(el.className).toContain("text-muted");
    expect(el.className).not.toContain("text-error");
  });

  it("lists versions and marks the best stable one", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    await user.click(screen.getAllByTitle("Change version")[0]);
    expect(await screen.findByText("postgresql · current 15.5.0")).toBeDefined();

    const best = await screen.findByText("best stable");
    expect(best.closest("button")!.textContent).toContain("16.0.0");

    await user.click(screen.getByRole("button", { name: /16\.0\.0/ }));
    await waitFor(() =>
      expect(fake.requests).toContain("POST /api/instances/alpha/deps/postgresql/version"),
    );
  });

  it("does not offer a version list for OCI dependencies", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    const inst = managedInstance();
    inst.deps = [
      {
        id: "demo",
        name: "demo",
        version: "0.1.0",
        repository: "oci://registry.example.invalid/org/demo",
        sourceKind: "arbitrary",
      },
    ];
    renderWithProviders(<DepsTab inst={inst} />);

    await user.click(screen.getByTitle("Change version"));
    expect(await screen.findByText(/version listing is not available/)).toBeDefined();
    expect(fake.requests.some((r) => r.includes("/versions"))).toBe(false);
  });

  it("surfaces a rejected version", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<DepsTab inst={managedInstance()} />);

    await user.click(screen.getAllByTitle("Change version")[0]);
    await user.type(await screen.findByPlaceholderText("exact version"), "99.0.0");
    await user.click(screen.getByRole("button", { name: "Set" }));

    expect(await screen.findByText(/invalid version "99.0.0"/)).toBeDefined();
  });
});
