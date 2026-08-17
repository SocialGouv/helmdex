import { describe, expect, it } from "vitest";
import { editableInMode, valuesFileLabel } from "./ValuesTab";
import type { InstanceInfo } from "../api/types";

const managed: InstanceInfo = { name: "m", path: "/m", managed: true, deps: [] };
const direct: InstanceInfo = { name: "d", path: "/d", managed: false, deps: [] };

describe("values file roles", () => {
  it("managed mode: generated output and imported layers are read-only", () => {
    expect(editableInMode(managed, "values.yaml")).toBe(false);
    expect(editableInMode(managed, "values.default.yaml")).toBe(false);
    expect(editableInMode(managed, "values.platform.yaml")).toBe(false);
    expect(editableInMode(managed, "values.instance.yaml")).toBe(true);
    expect(editableInMode(managed, "values.set.prod.yaml")).toBe(true);
  });

  it("direct mode: every values file is user-owned and editable", () => {
    expect(editableInMode(direct, "values.yaml")).toBe(true);
    expect(editableInMode(direct, "values.deploy.yaml")).toBe(true);
  });

  it("labels reflect the mode", () => {
    expect(valuesFileLabel(managed, "values.yaml")).toMatch(/generated/i);
    expect(valuesFileLabel(direct, "values.yaml")).toMatch(/edited in place/i);
    expect(valuesFileLabel(direct, "values.deploy.yaml")).toMatch(/user-owned/i);
  });
});
