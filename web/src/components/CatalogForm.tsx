import { useState } from "react";
import type { Schema } from "@/lib/catalog";
export function SchemaFields({
  schema,
  value,
  onChange,
}: {
  schema: Schema;
  value: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
}) {
  const properties = (schema.schema.properties ?? {}) as Record<
    string,
    { type?: string; enum?: string[]; description?: string }
  >;
  const required = (schema.schema.required ?? []) as string[];
  return (
    <div className="grid gap-3">
      {Object.entries(properties).map(([key, field]) => (
        <label key={key} className="grid gap-1 text-sm">
          {key.replaceAll("_", " ")}
          {required.includes(key) ? " *" : ""}
          {field.enum ? (
            <select
              value={String(value[key] ?? "")}
              onChange={(e) => onChange({ ...value, [key]: e.target.value })}
            >
              <option value="">Select</option>
              {field.enum.map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          ) : field.type === "boolean" ? (
            <input
              type="checkbox"
              checked={Boolean(value[key])}
              onChange={(e) => onChange({ ...value, [key]: e.target.checked })}
            />
          ) : field.type === "object" || field.type === "array" ? (
            <JSONField
              value={value[key] ?? (field.type === "array" ? [] : {})}
              onChange={(v) => onChange({ ...value, [key]: v })}
            />
          ) : (
            <input
              className="rounded border px-3 py-2"
              required={required.includes(key)}
              type={
                field.type === "integer" || field.type === "number"
                  ? "number"
                  : "text"
              }
              value={String(value[key] ?? "")}
              onChange={(e) => {
                const next = { ...value };
                if (e.target.value === "") delete next[key];
                else
                  next[key] =
                    field.type === "integer" || field.type === "number"
                      ? Number(e.target.value)
                      : e.target.value;
                onChange(next);
              }}
            />
          )}
          {field.description && (
            <span className="text-muted-foreground">{field.description}</span>
          )}
        </label>
      ))}
    </div>
  );
}
function JSONField({
  value,
  onChange,
}: {
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  const [text, setText] = useState(JSON.stringify(value));
  const [invalid, setInvalid] = useState(false);
  return (
    <>
      <textarea
        className="rounded border px-3 py-2 font-mono"
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          try {
            onChange(JSON.parse(e.target.value));
            setInvalid(false);
            e.currentTarget.setCustomValidity("");
          } catch {
            setInvalid(true);
            e.currentTarget.setCustomValidity("Invalid JSON");
          }
        }}
      />
      {invalid && <span role="alert">Invalid JSON</span>}
    </>
  );
}
export function ErrorMessage({ error }: { error: unknown }) {
  return error ? (
    <p role="alert" className="rounded border border-red-300 p-3 text-red-700">
      {error instanceof Error ? error.message : String(error)}
    </p>
  ) : null;
}
export function Snapshot({ value }: { value: unknown }) {
  return (
    <pre className="max-h-96 overflow-auto rounded border bg-muted/30 p-3 text-xs">
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}
export const buttonClass =
  "rounded border px-3 py-2 text-sm hover:bg-muted disabled:opacity-50";
