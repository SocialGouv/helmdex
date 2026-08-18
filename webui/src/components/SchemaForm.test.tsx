import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import SchemaForm, { type JSONSchema } from "./SchemaForm";

const schema: JSONSchema = {
  type: "object",
  properties: {
    replicaCount: { type: "integer", default: 1, description: "Number of replicas" },
    image: {
      type: "object",
      properties: {
        repository: { type: "string" },
        pullPolicy: { type: "string", enum: ["Always", "IfNotPresent", "Never"], default: "IfNotPresent" },
      },
    },
    ingress: {
      type: "object",
      properties: {
        enabled: { type: "boolean", default: false },
      },
    },
    tolerations: { type: "array" },
  },
};

describe("SchemaForm", () => {
  it("renders typed fields from the schema", () => {
    render(<SchemaForm schema={schema} value={{}} onChange={() => {}} />);
    expect(screen.getByText("replicaCount")).toBeDefined();
    expect(screen.getByText("repository")).toBeDefined();
    expect(screen.getByText("pullPolicy")).toBeDefined();
    expect(screen.getByText("enabled")).toBeDefined();
    // Arrays fall back to a raw JSON textarea.
    expect(screen.getByText("tolerations")).toBeDefined();
  });

  it("emits nested updates without clobbering siblings", () => {
    const onChange = vi.fn();
    render(
      <SchemaForm
        schema={schema}
        value={{ image: { repository: "repo/x" }, replicaCount: 2 }}
        onChange={onChange}
      />,
    );
    const pullPolicy = screen.getByText("pullPolicy").closest("label")!.querySelector("select")!;
    fireEvent.change(pullPolicy, { target: { value: JSON.stringify("Always") } });
    expect(onChange).toHaveBeenCalledWith({
      image: { repository: "repo/x", pullPolicy: "Always" },
      replicaCount: 2,
    });
  });

  it("number input produces numbers and clears to undefined", () => {
    const onChange = vi.fn();
    // Stateful harness: the form is controlled, so re-render with each change
    // like the real dialog does.
    function Harness() {
      const [value, setValue] = useState<unknown>({});
      return (
        <SchemaForm
          schema={schema}
          value={value}
          onChange={(v) => {
            onChange(v);
            setValue(v);
          }}
        />
      );
    }
    render(<Harness />);
    const input = screen.getByText("replicaCount").closest("label")!.querySelector("input")!;
    fireEvent.change(input, { target: { value: "3" } });
    expect(onChange).toHaveBeenLastCalledWith({ replicaCount: 3 });
    fireEvent.change(input, { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith({});
  });
});
