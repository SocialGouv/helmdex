import { useState } from "react";

// Minimal JSON-Schema-driven structured editor for chart values overrides
// (web counterpart of the TUI's schemaform). Supports objects, strings,
// numbers, integers, booleans and enums; anything else falls back to a raw
// JSON textarea for that node.

export interface JSONSchema {
  type?: string | string[];
  title?: string;
  description?: string;
  default?: unknown;
  enum?: unknown[];
  properties?: Record<string, JSONSchema>;
  items?: JSONSchema;
  required?: string[];
  [key: string]: unknown;
}

type Obj = Record<string, unknown>;

function schemaType(s: JSONSchema): string {
  if (Array.isArray(s.type)) return s.type[0] ?? "";
  return s.type ?? (s.properties ? "object" : "");
}

function getChild(value: unknown, key: string): unknown {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return (value as Obj)[key];
  }
  return undefined;
}

function setChild(value: unknown, key: string, child: unknown): Obj {
  const base: Obj = value && typeof value === "object" && !Array.isArray(value) ? { ...(value as Obj) } : {};
  if (child === undefined) {
    delete base[key];
  } else {
    base[key] = child;
  }
  return base;
}

function FieldLabel({ name, schema }: { name: string; schema: JSONSchema }) {
  return (
    <span className="text-sm" title={schema.description}>
      {schema.title || name}
      {schema.description && <span className="ml-1 cursor-help text-xs text-muted">ⓘ</span>}
    </span>
  );
}

function JSONFallback({
  value,
  schema,
  onChange,
}: {
  value: unknown;
  schema: JSONSchema;
  onChange: (v: unknown) => void;
}) {
  const [text, setText] = useState(() => (value === undefined ? "" : JSON.stringify(value, null, 2)));
  const [err, setErr] = useState("");
  return (
    <div>
      <textarea
        value={text}
        placeholder={schema.default !== undefined ? `default: ${JSON.stringify(schema.default)}` : "JSON…"}
        onChange={(e) => {
          const t = e.target.value;
          setText(t);
          if (t.trim() === "") {
            setErr("");
            onChange(undefined);
            return;
          }
          try {
            onChange(JSON.parse(t));
            setErr("");
          } catch {
            setErr("invalid JSON");
          }
        }}
        rows={3}
        className="w-full rounded-md border border-border bg-panel-2 px-2 py-1 font-mono text-xs outline-none focus:border-accent"
      />
      {err && <div className="text-xs text-error">{err}</div>}
    </div>
  );
}

export function SchemaField({
  name,
  schema,
  value,
  onChange,
}: {
  name: string;
  schema: JSONSchema;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  const t = schemaType(schema);

  if (schema.enum && schema.enum.length > 0) {
    return (
      <label className="flex items-center justify-between gap-3 py-1">
        <FieldLabel name={name} schema={schema} />
        <select
          value={value === undefined ? "" : JSON.stringify(value)}
          onChange={(e) => onChange(e.target.value === "" ? undefined : JSON.parse(e.target.value))}
          className="w-56 rounded-md border border-border bg-panel-2 px-2 py-1 text-sm outline-none"
        >
          <option value="">
            {schema.default !== undefined ? `(default: ${JSON.stringify(schema.default)})` : "(unset)"}
          </option>
          {schema.enum.map((opt) => (
            <option key={JSON.stringify(opt)} value={JSON.stringify(opt)}>
              {String(opt)}
            </option>
          ))}
        </select>
      </label>
    );
  }

  switch (t) {
    case "boolean":
      return (
        <label className="flex items-center justify-between gap-3 py-1">
          <FieldLabel name={name} schema={schema} />
          <span className="flex w-56 items-center gap-2">
            <input
              type="checkbox"
              checked={value === undefined ? schema.default === true : value === true}
              onChange={(e) => onChange(e.target.checked)}
            />
            {value === undefined && <span className="text-xs text-muted">(default)</span>}
            {value !== undefined && (
              <button type="button" onClick={() => onChange(undefined)} className="text-xs text-muted hover:text-text">
                reset
              </button>
            )}
          </span>
        </label>
      );
    case "integer":
    case "number":
      return (
        <label className="flex items-center justify-between gap-3 py-1">
          <FieldLabel name={name} schema={schema} />
          <input
            type="number"
            value={value === undefined || value === null ? "" : String(value)}
            placeholder={schema.default !== undefined ? `default: ${String(schema.default)}` : ""}
            onChange={(e) => {
              const raw = e.target.value;
              if (raw === "") {
                onChange(undefined);
                return;
              }
              onChange(t === "integer" ? parseInt(raw, 10) : parseFloat(raw));
            }}
            className="w-56 rounded-md border border-border bg-panel-2 px-2 py-1 text-sm outline-none focus:border-accent"
          />
        </label>
      );
    case "string":
      return (
        <label className="flex items-center justify-between gap-3 py-1">
          <FieldLabel name={name} schema={schema} />
          <input
            value={value === undefined || value === null ? "" : String(value)}
            placeholder={schema.default !== undefined ? `default: ${String(schema.default)}` : ""}
            onChange={(e) => onChange(e.target.value === "" ? undefined : e.target.value)}
            className="w-56 rounded-md border border-border bg-panel-2 px-2 py-1 text-sm outline-none focus:border-accent"
          />
        </label>
      );
    case "object":
      if (schema.properties && Object.keys(schema.properties).length > 0) {
        return (
          <fieldset className="my-1 rounded-md border border-border/60 px-3 py-1">
            <legend className="px-1 text-xs uppercase tracking-wide text-muted" title={schema.description}>
              {schema.title || name}
            </legend>
            {Object.entries(schema.properties).map(([key, child]) => (
              <SchemaField
                key={key}
                name={key}
                schema={child}
                value={getChild(value, key)}
                onChange={(v) => onChange(setChild(value, key, v))}
              />
            ))}
          </fieldset>
        );
      }
      return (
        <div className="py-1">
          <FieldLabel name={name} schema={schema} />
          <JSONFallback value={value} schema={schema} onChange={onChange} />
        </div>
      );
    default:
      // Arrays, unions, untyped nodes: raw JSON editing for this subtree.
      return (
        <div className="py-1">
          <FieldLabel name={name} schema={schema} />
          <JSONFallback value={value} schema={schema} onChange={onChange} />
        </div>
      );
  }
}

export default function SchemaForm({
  schema,
  value,
  onChange,
}: {
  schema: JSONSchema;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  if (!schema.properties || Object.keys(schema.properties).length === 0) {
    return <JSONFallback value={value} schema={schema} onChange={onChange} />;
  }
  return (
    <div>
      {Object.entries(schema.properties).map(([key, child]) => (
        <SchemaField
          key={key}
          name={key}
          schema={child}
          value={getChild(value, key)}
          onChange={(v) => onChange(setChild(value, key, v))}
        />
      ))}
    </div>
  );
}
