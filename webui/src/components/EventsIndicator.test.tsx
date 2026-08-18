import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen } from "@testing-library/react";
import EventsIndicator from "./EventsIndicator";

/**
 * The indicator is the only place the SSE stream reaches the user, so what
 * matters is that it reflects connection state and the latest frame, and
 * that a malformed frame does not take the stream down.
 */

class FakeEventSource {
  static last: FakeEventSource | null = null;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  closed = false;

  constructor(public url: string) {
    FakeEventSource.last = this;
  }
  close() {
    this.closed = true;
  }
}

beforeEach(() => {
  FakeEventSource.last = null;
  vi.stubGlobal("EventSource", FakeEventSource);
});

afterEach(() => vi.unstubAllGlobals());

describe("EventsIndicator", () => {
  it("subscribes to the stream and starts idle", () => {
    render(<EventsIndicator />);

    expect(FakeEventSource.last?.url).toBe("/api/events");
    expect(screen.getByText("idle")).toBeDefined();
    expect(screen.getByTitle("disconnected")).toBeDefined();
  });

  it("reflects the connection state", () => {
    render(<EventsIndicator />);

    act(() => FakeEventSource.last!.onopen!());
    expect(screen.getByTitle("connected")).toBeDefined();

    act(() => FakeEventSource.last!.onerror!());
    expect(screen.getByTitle("disconnected")).toBeDefined();
  });

  it("shows the latest event and the instance it concerns", () => {
    render(<EventsIndicator />);

    act(() => FakeEventSource.last!.onmessage!({ data: JSON.stringify({ type: "apply.start", instance: "alpha" }) }));
    expect(screen.getByText("apply.start · alpha")).toBeDefined();

    act(() => FakeEventSource.last!.onmessage!({ data: JSON.stringify({ type: "apply.done", instance: "alpha" }) }));
    expect(screen.getByText("apply.done · alpha")).toBeDefined();
  });

  it("keeps the last good frame when a malformed one arrives", () => {
    render(<EventsIndicator />);

    act(() => FakeEventSource.last!.onmessage!({ data: JSON.stringify({ type: "catalog.sync.done" }) }));
    act(() => FakeEventSource.last!.onmessage!({ data: "not json" }));

    expect(screen.getByText("catalog.sync.done")).toBeDefined();
  });

  it("closes the stream when unmounted", () => {
    const { unmount } = render(<EventsIndicator />);
    const es = FakeEventSource.last!;

    unmount();
    expect(es.closed).toBe(true);
  });
});
