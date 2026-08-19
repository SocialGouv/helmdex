import { afterEach, describe, expect, it } from "vitest";
import { api, ApiError, setApiBase } from "./client";
import { installFakeApi, type FakeApi } from "../test/fakeApi";

/**
 * The client is the whole contract with the Go server: URL building,
 * encoding, and how failures reach the UI.
 */

let fake: FakeApi;
afterEach(() => {
  fake?.restore();
  setApiBase("");
});

describe("api client", () => {
  // The desktop shell scopes every call to the active workspace by prefixing
  // its /ws/<id> base; the browser keeps the bare /api paths.
  it("prefixes requests with the workspace base once set", async () => {
    fake = installFakeApi();

    await api.repo();
    expect(fake.requests[fake.requests.length - 1]).toBe("GET /api/repo");

    setApiBase("/ws/w2");
    await api.repo();
    expect(fake.requests[fake.requests.length - 1]).toBe("GET /ws/w2/api/repo");
  });

  it("surfaces the server's error message, not the status line", async () => {
    fake = installFakeApi();
    fake.failNext("GET", "/api/instances", 500, "read apps dir: permission denied");

    await expect(api.instances()).rejects.toThrow("read apps dir: permission denied");
  });

  it("falls back to the status text when the error body is not JSON", async () => {
    fake = installFakeApi();
    const original = globalThis.fetch;
    globalThis.fetch = async () => new Response("<html>502</html>", { status: 502, statusText: "Bad Gateway" });
    try {
      await expect(api.instances()).rejects.toThrow(/502/);
    } finally {
      globalThis.fetch = original;
    }
  });

  it("exposes the status code on the error", async () => {
    fake = installFakeApi();
    fake.failNext("GET", "/api/instances/ghost", 404, 'instance "ghost" not found');

    await expect(api.instance("ghost")).rejects.toBeInstanceOf(ApiError);
    fake.failNext("GET", "/api/instances/ghost", 404, 'instance "ghost" not found');
    await api.instance("ghost").catch((e: ApiError) => expect(e.status).toBe(404));
  });

  it("returns nothing for a 204 instead of failing to parse a body", async () => {
    fake = installFakeApi();
    await expect(api.deleteInstance("alpha")).resolves.toBeUndefined();
  });

  it("returns plain text for text endpoints", async () => {
    fake = installFakeApi();
    const readme = await api.depInspect("alpha", "nginx", "readme");
    expect(readme).toContain("# nginx");
    expect(typeof readme).toBe("string");
  });

  // A dep id may contain dots, slashes or spaces: it is a map key, and it
  // travels in the URL path.
  it("percent-encodes instance and dependency identifiers", async () => {
    fake = installFakeApi({
      instances: [
        {
          name: "a b",
          path: "/repo/apps/a b",
          managed: true,
          deps: [{ id: "my.dotted/dep", name: "nginx", version: "1.0.0", repository: "r" }],
        },
      ],
    });

    await api.depValuesGet("a b", "my.dotted/dep").catch(() => undefined);

    const last = fake.requests[fake.requests.length - 1];
    expect(last).toBe("GET /api/instances/a%20b/deps/my.dotted%2Fdep/values");
    // The separator inside the id must stay encoded, or it becomes a route.
    expect(last).not.toContain("dotted/dep");
  });

  // The regen flag only exists in the body: the URL is identical either way,
  // so it has to be asserted there or it is not asserted at all.
  it("sends the regen flag so callers can defer the merge", async () => {
    fake = installFakeApi();

    await api.valuesSet("alpha", "$.replicaCount", 3, false);
    expect(fake.lastCall("PUT", "/api/instances/alpha/values")?.body).toEqual({
      path: "$.replicaCount",
      value: 3,
      regen: false,
    });

    await api.valuesSet("alpha", "$.replicaCount", 4);
    expect(fake.lastCall("PUT", "/api/instances/alpha/values")?.body).toEqual({
      path: "$.replicaCount",
      value: 4,
      regen: true,
    });
  });
});
