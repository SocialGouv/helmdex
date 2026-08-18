import { afterEach, describe, expect, it } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CatalogPage from "./Catalog";
import { renderWithProviders } from "../test/render";
import { installFakeApi, type FakeApi } from "../test/fakeApi";

let fake: FakeApi;
afterEach(() => fake?.restore());

describe("Catalog page", () => {
  it("shows each entry pinned, sourced and with its default sets", async () => {
    fake = installFakeApi();
    renderWithProviders(<CatalogPage />);

    expect(await screen.findByText("bitnami-nginx-15.0.0")).toBeDefined();
    expect(screen.getByText("Example")).toBeDefined();
    expect(screen.getByText("nginx@15.0.0")).toBeDefined();
    expect(screen.getByText("set:dev")).toBeDefined();
  });

  it("tells the user what to do when nothing is synced", async () => {
    fake = installFakeApi({ catalog: [] });
    renderWithProviders(<CatalogPage />);

    expect(await screen.findByText(/Catalog is empty/)).toBeDefined();
  });

  it("syncs sources and refreshes the list", async () => {
    fake = installFakeApi({ catalog: [] });
    const user = userEvent.setup();
    renderWithProviders(<CatalogPage />);

    await screen.findByText(/Catalog is empty/);
    fake.state.catalog = [
      {
        Entry: {
          ID: "bitnami-postgresql-15.5.0",
          Chart: { Repo: "https://charts.bitnami.com/bitnami", Name: "postgresql" },
          Version: "15.5.0",
        },
        SourceName: "Example",
      },
    ];
    await user.click(screen.getByRole("button", { name: /Sync sources/ }));

    expect(await screen.findByText("bitnami-postgresql-15.5.0")).toBeDefined();
    await waitFor(() => expect(fake.requests).toContain("POST /api/catalog/sync"));
  });

  it("surfaces a failed sync", async () => {
    fake = installFakeApi();
    const user = userEvent.setup();
    renderWithProviders(<CatalogPage />);

    await screen.findByText("bitnami-nginx-15.0.0");
    fake.failNext("POST", "/api/catalog/sync", 502, "clone https://git.example.invalid/presets: repository not found");
    await user.click(screen.getByRole("button", { name: /Sync sources/ }));

    expect(await screen.findByText(/repository not found/)).toBeDefined();
  });

  it("surfaces a failed catalog read", async () => {
    fake = installFakeApi();
    fake.failNext("GET", "/api/catalog", 500, "read catalog dir: permission denied");
    renderWithProviders(<CatalogPage />);

    expect(await screen.findByText(/permission denied/)).toBeDefined();
  });
});
