import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { apiFetch } from "@/lib/api";
import {
  document,
  send,
  submitOperation,
  types,
  type Catalog,
  type Connection,
  type ScopeResponse,
  type Resource,
  type Observation,
  type Operation,
  type Binding,
  type Instance,
} from "@/lib/catalog";
import {
  SchemaFields,
  ErrorMessage,
  Snapshot,
  buttonClass,
} from "@/components/CatalogForm";
export default function Inventory() {
  const qc = useQueryClient();
  const [connection, setConnection] = useState("");
  const [binding, setBinding] = useState("");
  const [typeID, setTypeID] = useState("");
  const [locator, setLocator] = useState<Record<string, unknown>>({});
  const [confirmID, setConfirmID] = useState("");
  const [create, setCreate] = useState<Record<string, unknown>>({});
  const [parent, setParent] = useState<Record<string, unknown>>({});
  const [discovered, setDiscovered] = useState<Observation[]>([]);
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const [operation, setOperation] = useState<Operation>();
  const operations = useQuery({
    queryKey: ["operations"],
    queryFn: () => apiFetch<Operation[]>("/operations"),
  });
  const instances = useQuery({
    queryKey: ["instances"],
    queryFn: () => apiFetch<Instance[]>("/provider-instances"),
  });
  const catalog = useQuery({
    queryKey: ["catalog"],
    queryFn: () => apiFetch<Catalog>("/catalog"),
  });
  const resources = useQuery({
    queryKey: ["resources"],
    queryFn: () => apiFetch<Resource[]>("/resource-instances"),
  });
  const connections = useQuery({
    queryKey: ["connections"],
    queryFn: () => apiFetch<Connection[]>("/connections"),
  });
  const scopes = useQuery({
    queryKey: ["scopes", connection],
    queryFn: () => apiFetch<ScopeResponse>(`/connections/${connection}/scopes`),
    enabled: !!connection,
  });
  const descriptor = types(catalog.data).find((t) => t.id === typeID);
  const action = descriptor?.actions.find((a) => a.effect === "create");
  const selectedBinding = scopes.data?.bindings.find((b) => b.id === binding);
  const selectedScope = scopes.data?.scopes.find(
    (s) => s.id === selectedBinding?.scope_id,
  );
  async function work(fn: () => Promise<unknown>) {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      await qc.invalidateQueries({ queryKey: ["resources"] });
      await qc.invalidateQueries({ queryKey: ["operations"] });
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  async function bindCandidate(index: number) {
    await work(async () => {
      const b = await send<Binding>(
        `/connections/${connection}/scopes`,
        scopes.data!.available[index],
      );
      setBinding(b.id);
      await qc.invalidateQueries({ queryKey: ["scopes", connection] });
    });
  }
  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-semibold">Resource directory</h1>
      <ErrorMessage
        error={error ?? resources.error ?? catalog.error ?? scopes.error}
      />
      <div className="grid gap-3 rounded border p-4">
        <h2 className="font-medium">Discover or import</h2>
        <select
          className="rounded border p-2"
          value={connection}
          onChange={(e) => {
            setConnection(e.target.value);
            setTypeID("");
            setConfirmID("");
            setBinding("");
            setDiscovered([]);
          }}
        >
          <option value="">Select authorization</option>
          {connections.data?.map((c) => (
            <option key={c.id} value={c.id}>
              {c.label} · revision {c.credential_revision}
            </option>
          ))}
        </select>
        <select
          className="rounded border p-2"
          value={binding}
          onChange={(e) => setBinding(e.target.value)}
        >
          <option value="">Select bound scope</option>
          {scopes.data?.bindings.map((b) => (
            <option key={b.id} value={b.id}>
              {scopes.data.scopes.find((s) => s.id === b.scope_id)?.label} ·{" "}
              {b.validation_state}
            </option>
          ))}
        </select>
        {scopes.data?.available.map((s, i) => (
          <button
            className={buttonClass}
            disabled={busy}
            key={JSON.stringify(s.identity_parts)}
            onClick={() => void bindCandidate(i)}
          >
            Bind / revalidate {s.label}
          </button>
        ))}
        <select
          className="rounded border p-2"
          value={typeID}
          onChange={(e) => {
            setTypeID(e.target.value);
            setConfirmID("");
            setLocator({});
            setCreate({});
            setDiscovered([]);
          }}
        >
          <option value="">Select resource type</option>
          {types(catalog.data)
            .filter(
              (t) =>
                !selectedScope ||
                (t.scope_types.includes(selectedScope.scope_type) &&
                  t.provider_id ===
                    instances.data?.find(
                      (i) => i.id === selectedScope.provider_instance_id,
                    )?.provider_id),
            )
            .map((t) => (
              <option key={t.id} value={t.id}>
                {t.display_name} · {t.provider_id}
              </option>
            ))}
        </select>
        {descriptor && (
          <>
            {descriptor.identity.natural && (
              <label className="grid gap-2">
                Natural identity cannot prove renames or recreation. To add
                access to an existing object, explicitly confirm the matching
                directory resource.
                <select
                  value={confirmID}
                  onChange={(e) => setConfirmID(e.target.value)}
                >
                  <option value="">Import as a new object; do not merge</option>
                  {resources.data
                    ?.filter((r) => r.resource_type_id === typeID)
                    .map((r) => (
                      <option key={r.id} value={r.id}>
                        Confirm same object: {r.display_name} ({r.id})
                      </option>
                    ))}
                </select>
              </label>
            )}
            <SchemaFields
              schema={descriptor.locator}
              value={locator}
              onChange={setLocator}
            />
            {descriptor.identity.parent_type && (
              <div>
                <p>
                  Remote parent locator (the parent does not need to be
                  imported)
                </p>
                <SchemaFields
                  schema={
                    types(catalog.data).find(
                      (t) => t.id === descriptor.identity.parent_type,
                    )!.locator
                  }
                  value={parent}
                  onChange={setParent}
                />
              </div>
            )}
            <div className="flex gap-2">
              <button
                className={buttonClass}
                disabled={busy || !binding}
                onClick={() =>
                  void work(async () =>
                    setDiscovered(
                      await send<Observation[]>(
                        `/connection-scopes/${binding}/resource-types/${typeID}/discover`,
                        document(
                          descriptor.identity.parent_type
                            ? { parent_locator: parent }
                            : {},
                        ),
                      ),
                    ),
                  )
                }
              >
                Discover
              </button>
              <button
                className={buttonClass}
                disabled={busy || !binding}
                onClick={() =>
                  void work(() =>
                    send("/resource-instances/import", {
                      connection_scope_id: binding,
                      resource_type_id: typeID,
                      locator: document(locator, descriptor.locator.version),
                      confirm_resource_id: confirmID || undefined,
                    }),
                  )
                }
              >
                Import resource
              </button>
            </div>
            {discovered.map((v) => (
              <div
                className="flex justify-between rounded border p-3"
                key={JSON.stringify(v.identity_parts)}
              >
                <span>{v.display_name}</span>
                <button
                  className={buttonClass}
                  disabled={busy}
                  onClick={() =>
                    void work(() =>
                      send("/resource-instances/import", {
                        connection_scope_id: binding,
                        resource_type_id: typeID,
                        locator: document(
                          v.locator,
                          descriptor.locator.version,
                        ),
                        confirm_resource_id: confirmID || undefined,
                      }),
                    )
                  }
                >
                  Import
                </button>
              </div>
            ))}
          </>
        )}
        {action && (
          <form
            className="space-y-3 border-t pt-4"
            onSubmit={(e) => {
              e.preventDefault();
              void work(async () =>
                setOperation(
                  await submitOperation({
                    resource_type_id: typeID,
                    action_id: action.id,
                    target_kind: "scope",
                    target_id: binding,
                    input: document(create, action.input.version),
                  }),
                ),
              );
            }}
          >
            <h2>{action.display_name}</h2>
            <SchemaFields
              schema={action.input}
              value={create}
              onChange={setCreate}
            />
            <button
              className={buttonClass}
              disabled={busy || !binding || operation?.status === "unknown"}
            >
              Submit write operation
            </button>
          </form>
        )}
        {operation && (
          <>
            <p>
              Operation {operation.id}: {operation.status}
            </p>
            {operation.status === "unknown" && (
              <p role="alert">
                Outcome unknown. Inspect the remote provider before taking
                another action. Replaying this request only returns its journal
                entry.
              </p>
            )}
            <Snapshot value={operation} />
          </>
        )}
      </div>
      <div className="grid gap-3">
        {resources.data?.map((r) => (
          <Link
            className="rounded border p-4 hover:bg-muted"
            key={r.id}
            to={`/inventory/${r.id}`}
          >
            <strong>{r.display_name}</strong>
            <p className="text-sm text-muted-foreground">
              {r.resource_type_id} · {r.origin} · {r.identity_key}
            </p>
          </Link>
        ))}
      </div>
      <details className="rounded border p-4">
        <summary>
          Operation journal (retained after forgetting resources)
        </summary>
        <ErrorMessage error={operations.error} />
        {operations.data?.map((op) => (
          <div key={op.id}>
            <Snapshot value={op} />
            {op.remote_operation_id && (
              <button
                className={buttonClass}
                disabled={busy}
                onClick={() =>
                  void work(async () => {
                    await apiFetch(`/operations/${op.id}/check`, {
                      method: "POST",
                    });
                    await qc.invalidateQueries({ queryKey: ["operations"] });
                  })
                }
              >
                Check submitted operation result
              </button>
            )}
          </div>
        ))}
      </details>
    </div>
  );
}
