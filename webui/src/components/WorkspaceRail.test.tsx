import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import WorkspaceRail, { monogram, parentDir, railHue } from "./WorkspaceRail";
import type { WorkspacesState } from "../lib/desktop";

const state: WorkspacesState = {
  workspaces: [
    { id: "w1", root: "/home/jo/infra-prod", name: "infra-prod" },
    { id: "w2", root: "/home/jo/apps", name: "apps" },
  ],
  activeId: "w1",
};

function renderRail(overrides: Partial<Parameters<typeof WorkspaceRail>[0]> = {}) {
  const props = {
    state,
    expanded: false,
    onActivate: vi.fn(),
    onClose: vi.fn(),
    onAdd: vi.fn(),
    onToggleExpanded: vi.fn(),
    ...overrides,
  };
  render(<WorkspaceRail {...props} />);
  return props;
}

describe("monogram", () => {
  it("uses the initials of multi-word names and the first letters otherwise", () => {
    expect(monogram("infra-prod")).toBe("IP");
    expect(monogram("my_app")).toBe("MA");
    expect(monogram("apps")).toBe("AP");
    expect(monogram("a")).toBe("A");
  });

  it("survives unicode and degenerate names without broken surrogates", () => {
    // Astral-plane chars must stay whole code points, not UTF-16 halves.
    expect(monogram("🚀-🎉")).toBe("🚀🎉");
    expect(monogram("𝕏db")).toBe("𝕏D");
    // Separator-only or empty names still render something.
    expect(monogram("--")).toBe("?");
    expect(monogram("")).toBe("?");
  });
});

describe("railHue", () => {
  it("is deterministic and within the hue circle", () => {
    expect(railHue("/home/jo/infra")).toBe(railHue("/home/jo/infra"));
    const h = railHue("/somewhere/else");
    expect(h).toBeGreaterThanOrEqual(0);
    expect(h).toBeLessThan(360);
  });
});

describe("WorkspaceRail", () => {
  it("shows one tile per folder, full path in the tooltip, active marked", () => {
    renderRail();

    const prod = screen.getByRole("button", { name: "Workspace infra-prod" });
    expect(prod.title).toContain("/home/jo/infra-prod");
    expect(prod.title).toContain("Ctrl+1");
    expect(prod.getAttribute("aria-current")).toBe("true");
    expect(screen.getByRole("button", { name: "Workspace apps" }).getAttribute("aria-current")).toBeNull();
  });

  it("activates on click and opens the dialog from the + tile", async () => {
    const user = userEvent.setup();
    const props = renderRail();

    await user.click(screen.getByRole("button", { name: "Workspace apps" }));
    expect(props.onActivate).toHaveBeenCalledWith("w2");

    await user.click(screen.getByRole("button", { name: "Open folder" }));
    expect(props.onAdd).toHaveBeenCalled();
  });

  it("expanded mode shows names and parent directories; compact only monograms", () => {
    renderRail({ expanded: true });
    expect(screen.getByText("infra-prod")).toBeDefined();
    expect(screen.getAllByText("/home/jo")).toHaveLength(2);
    expect(screen.getByText("Open folder…")).toBeDefined();
    expect(screen.getByRole("button", { name: "Collapse the folder rail" })).toBeDefined();
  });

  it("compact mode renders monograms only, with an expand toggle", async () => {
    const user = userEvent.setup();
    const props = renderRail();
    expect(screen.queryByText("infra-prod")).toBeNull();
    expect(screen.getByText("IP")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Expand the folder rail" }));
    expect(props.onToggleExpanded).toHaveBeenCalled();
  });

  it("parentDir keeps roots disambiguated", () => {
    expect(parentDir("/home/jo/infra-prod")).toBe("/home/jo");
    expect(parentDir("/infra")).toBe("/");
    expect(parentDir("/")).toBe("/");
  });

  it("closes from the badge and from a middle click", async () => {
    const user = userEvent.setup();
    const props = renderRail();

    await user.click(screen.getByRole("button", { name: "Close apps" }));
    expect(props.onClose).toHaveBeenCalledWith("w2");

    fireEvent(
      screen.getByRole("button", { name: "Workspace infra-prod" }),
      new MouseEvent("auxclick", { button: 1, bubbles: true }),
    );
    expect(props.onClose).toHaveBeenCalledWith("w1");
  });
});
