import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { installFakeApi, type FakeApi } from "../test/fakeApi";
import { renderWithProviders } from "../test/render";
import ValuesTab from "./ValuesTab";
import type { InstanceInfo } from "../api/types";

// Monaco is heavy and DOM-driven; a textarea stand-in is enough to exercise
// the edit → validate → save gate.
vi.mock("@monaco-editor/react", () => ({
  default: ({ value, onChange }: { value?: string; onChange?: (v: string) => void }) => (
    <textarea data-testid="editor" value={value ?? ""} onChange={(e) => onChange?.(e.target.value)} />
  ),
}));

const direct: InstanceInfo = { name: "legacy", path: "/repo/apps/legacy", managed: false, deps: [] };

let fake: FakeApi;
afterEach(() => fake.restore());

async function editValues(): Promise<ReturnType<typeof userEvent.setup>> {
  const user = userEvent.setup();
  renderWithProviders(<ValuesTab inst={direct} />);
  const editor = await screen.findByTestId("editor");
  await user.clear(editor);
  await user.type(editor, "widget: broken");
  return user;
}

describe("ValuesTab schema gate", () => {
  it("saves directly when the draft passes schema validation", async () => {
    fake = installFakeApi();
    const user = await editValues();

    await user.click(screen.getByRole("button", { name: /^Save$/ }));

    await waitFor(() => expect(fake.lastCall("PUT", "/api/instances/legacy/file")).toBeDefined());
    expect(screen.queryByText(/schema warning/i)).toBeNull();
  });

  it("blocks the save and lists violations, then saves on bypass", async () => {
    fake = installFakeApi({
      schemaViolations: [
        { dep: "widget", path: "count", message: "expected type integer, got string" },
      ],
    });
    const user = await editValues();

    await user.click(screen.getByRole("button", { name: /^Save$/ }));

    // The warning is shown and nothing was written yet.
    expect(await screen.findByText(/schema warning/i)).toBeDefined();
    expect(screen.getByText(/widget\.count/)).toBeDefined();
    expect(fake.lastCall("PUT", "/api/instances/legacy/file")).toBeUndefined();

    // Bypass writes.
    await user.click(screen.getByRole("button", { name: /save anyway/i }));
    await waitFor(() => expect(fake.lastCall("PUT", "/api/instances/legacy/file")).toBeDefined());
  });
});
