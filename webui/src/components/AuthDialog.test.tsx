import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { installFakeApi, type FakeApi } from "../test/fakeApi";
import { renderWithProviders } from "../test/render";
import AuthDialog from "./AuthDialog";
import AuthRequiredNotice from "./AuthRequiredNotice";
import { api } from "../api/client";

/**
 * The dialog is the only path from "401 on a private chart" to a stored
 * credential, so what matters is: detected local credentials are offered and
 * a click signs in without the secret ever transiting the client; the manual
 * token form sends what was typed; a failed login stays in the dialog.
 */

let fake: FakeApi;
afterEach(() => fake.restore());

const candidate = {
  host: "pic-registry.example.org",
  kind: "oci" as const,
  url: "oci://pic-registry.example.org/org/app",
};

describe("AuthDialog", () => {
  it("offers detected local credentials and signs in with one click", async () => {
    fake = installFakeApi();
    fake.state.auth.detect = [
      { source: "docker-config", host: "pic-registry.example.org", username: "jo", label: "Docker login" },
    ];
    const onSuccess = vi.fn();
    renderWithProviders(
      <AuthDialog candidates={[candidate]} onClose={() => {}} onSuccess={onSuccess} />,
    );

    await userEvent.click(await screen.findByText(/Use Docker login \(jo\)/));

    await waitFor(() => expect(onSuccess).toHaveBeenCalled());
    const login = fake.lastCall("POST", "/api/auth/login");
    expect(login?.body).toMatchObject({
      host: "pic-registry.example.org",
      kind: "oci",
      method: "detected",
      source: "docker-config",
      url: candidate.url,
    });
    // The secret is resolved server-side: the client never sends one.
    expect((login?.body as Record<string, unknown>).secret).toBeUndefined();
  });

  it("submits a manually entered token", async () => {
    fake = installFakeApi();
    const onSuccess = vi.fn();
    renderWithProviders(
      <AuthDialog candidates={[candidate]} onClose={() => {}} onSuccess={onSuccess} />,
    );

    await screen.findByText(/No usable credentials found/);
    await userEvent.type(screen.getByPlaceholderText(/Username/), "jo");
    await userEvent.type(screen.getByPlaceholderText(/Token or password/), "glpat-abc");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => expect(onSuccess).toHaveBeenCalled());
    expect(fake.lastCall("POST", "/api/auth/login")?.body).toMatchObject({
      method: "manual",
      username: "jo",
      secret: "glpat-abc",
    });
  });

  it("keeps the dialog open and shows the server message when sign-in fails", async () => {
    fake = installFakeApi();
    fake.failNext("POST", "/api/auth/login", 400, "registry login to pic-registry.example.org failed: 401");
    renderWithProviders(
      <AuthDialog candidates={[candidate]} onClose={() => {}} onSuccess={() => {}} />,
    );

    await userEvent.type(await screen.findByPlaceholderText(/Token or password/), "bad");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByText(/registry login .* failed: 401/)).toBeDefined();
  });

  it("offers an SSH key alternative for git remotes", async () => {
    fake = installFakeApi();
    const onSuccess = vi.fn();
    renderWithProviders(
      <AuthDialog
        candidates={[{ host: "pic.example.org", kind: "git", url: "https://pic.example.org/g.git" }]}
        onClose={() => {}}
        onSuccess={onSuccess}
      />,
    );

    await userEvent.click(await screen.findByText("Use an SSH key instead"));
    await userEvent.type(screen.getByPlaceholderText(/Private key path/), "/home/jo/.ssh/id_ed25519");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => expect(onSuccess).toHaveBeenCalled());
    expect(fake.lastCall("POST", "/api/auth/login")?.body).toMatchObject({
      kind: "git",
      sshKeyPath: "/home/jo/.ssh/id_ed25519",
    });
  });
});

describe("AuthRequiredNotice", () => {
  it("renders nothing for ordinary errors and a sign-in entry for auth errors", async () => {
    fake = installFakeApi();
    fake.failNext("GET", "/api/instances/alpha/deps/nginx/versions", 502, "boom");
    const plainErr = await api.depVersions("alpha", "nginx").catch((e: unknown) => e);
    fake.failNext("GET", "/api/instances/alpha/deps/nginx/versions", 401, "401 Unauthorized", {
      authRequired: { candidates: [candidate] },
    });
    const authErr = await api.depVersions("alpha", "nginx").catch((e: unknown) => e);

    const onResolved = vi.fn();
    const { rerender } = renderWithProviders(
      <AuthRequiredNotice error={plainErr} onResolved={onResolved} />,
    );
    expect(screen.queryByText(/Authentication required/)).toBeNull();

    rerender(<AuthRequiredNotice error={authErr} onResolved={onResolved} />);
    expect(screen.getByText(/Authentication required for pic-registry.example.org/)).toBeDefined();

    // Sign in through the notice → dialog → resolved callback.
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await userEvent.type(await screen.findByPlaceholderText(/Token or password/), "glpat-abc");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => expect(onResolved).toHaveBeenCalled());
  });
});
