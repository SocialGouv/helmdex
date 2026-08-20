import { afterEach, describe, expect, it } from "vitest";
import { screen } from "@testing-library/react";
import DepDiffDialog from "./DepDiffDialog";
import { renderWithProviders } from "../test/render";
import { installFakeApi, type FakeApi } from "../test/fakeApi";
import type { DepInfo, InstanceInfo } from "../api/types";

let fake: FakeApi | undefined;
afterEach(() => {
  fake?.restore();
  fake = undefined;
});

function instance(): InstanceInfo {
  return { name: "alpha", path: "/repo/apps/alpha", managed: true, deps: [] };
}

function dep(repository: string): DepInfo {
  return { id: "nginx", name: "nginx", version: "15.0.0", repository, sourceKind: "arbitrary" };
}

describe("DepDiffDialog", () => {
  it("offers the version dropdown for a classic repository", async () => {
    fake = installFakeApi();
    renderWithProviders(
      <DepDiffDialog inst={instance()} dep={dep("https://example.invalid/charts")} onClose={() => {}} />,
    );

    expect(await screen.findByRole("option", { name: /15\.2\.0/ })).toBeDefined();
  });

  // OCI versions come from the registry's tags, so the diff flow must offer the
  // same dropdown rather than forcing the user to type a tag from memory.
  it("offers the version dropdown for an OCI repository", async () => {
    fake = installFakeApi();
    renderWithProviders(
      <DepDiffDialog
        inst={instance()}
        dep={dep("oci://registry.example.invalid/org/nginx")}
        onClose={() => {}}
      />,
    );

    expect(await screen.findByRole("option", { name: /15\.2\.0/ })).toBeDefined();
    expect(fake.requests).toContain("GET /api/instances/alpha/deps/nginx/versions");
  });

  // The dropdown is the discoverable path; when the listing fails the dialog
  // must say so rather than quietly showing only "or type a version".
  it("surfaces a failed version listing", async () => {
    fake = installFakeApi();
    fake.failNext("GET", "/api/instances/alpha/deps/nginx/versions", 502, "registry unreachable");
    renderWithProviders(
      <DepDiffDialog
        inst={instance()}
        dep={dep("oci://registry.example.invalid/org/nginx")}
        onClose={() => {}}
      />,
    );

    expect(await screen.findByText(/registry unreachable/)).toBeDefined();
    // The manual field stays usable as the fallback.
    expect(screen.getByPlaceholderText("or type a version")).toBeDefined();
  });
});
