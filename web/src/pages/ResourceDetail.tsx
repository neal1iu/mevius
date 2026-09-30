import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import { apiFetch } from "@/lib/api";
import {
  document,
  send,
  submitOperation,
  types,
  type Catalog,
  type ResourceDetail as Detail,
  type Operation,
  type Action,
  type View,
  type Resource,
} from "@/lib/catalog";
import {
  SchemaFields,
  ErrorMessage,
  Snapshot,
  buttonClass,
} from "@/components/CatalogForm";
export default function ResourceDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [accessID, setAccessID] = useState("");
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const [relationType, setRelationType] = useState("related_to");
  const [target, setTarget] = useState("");
  const detail = useQuery({
    queryKey: ["resource", id],
    queryFn: () => apiFetch<Detail>(`/resource-instances/${id}`),
  });
  const catalog = useQuery({
    queryKey: ["catalog"],
    queryFn: () => apiFetch<Catalog>("/catalog"),
  });
  const resources = useQuery({
    queryKey: ["resources"],
    queryFn: () => apiFetch<Resource[]>("/resource-instances"),
  });
  const resource = detail.data?.resource;
  const descriptor = types(catalog.data).find(
    (t) => t.id === resource?.resource_type_id,
  );
  const accesses = detail.data?.accesses ?? [];
  const selected =
    accesses.find((a) => a.id === accessID) ??
    (accesses.length === 1 ? accesses[0] : undefined);
  async function work(fn: () => Promise<unknown>) {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      await qc.invalidateQueries({ queryKey: ["resource", id] });
      await qc.invalidateQueries({ queryKey: ["resources"] });
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  if (!resource)
    return (
      <div className="p-6">
        <ErrorMessage error={detail.error} />
        {detail.isPending ? "Loading…" : "Resource unavailable"}
      </div>
    );
  return (
    <div className="space-y-5 p-6">
      <Link to="/inventory">← Resource directory</Link>
      <h1 className="text-2xl font-semibold">{resource.display_name}</h1>
      <p>
        {resource.resource_type_id} · origin {resource.origin} · identity{" "}
        {resource.identity_key}
      </p>
      <ErrorMessage error={error} />
      <label className="grid gap-2">
        Access path
        <select
          className="rounded border p-2"
          value={selected?.id ?? ""}
          onChange={(e) => setAccessID(e.target.value)}
        >
          <option value="">Select an access path</option>
          {accesses.map((a) => (
            <option key={a.id} value={a.id}>
              {a.connection_label} · {a.scope_label} · {a.validation_state} ·
              credential revision {a.credential_revision} ·{" "}
              {a.error_code ?? "observed"}
            </option>
          ))}
        </select>
      </label>
      {accesses.length > 1 && !selected && (
        <p>Choose the authorization path for observations and operations.</p>
      )}
      {selected && (
        <>
          <p>
            Last success: {selected.last_success_at ?? "never"} · last attempt:{" "}
            {selected.last_attempt_at}
          </p>
          <Snapshot value={selected.observation} />
          <Snapshot value={selected.capabilities} />
          <button
            className={buttonClass}
            disabled={busy}
            onClick={() =>
              void work(() =>
                send(`/resource-instances/${id}/refresh`, {
                  access_id: selected.id,
                }),
              )
            }
          >
            Refresh this access
          </button>
        </>
      )}
      <div className="flex gap-3">
        <button
          className={buttonClass}
          disabled={busy}
          onClick={() =>
            void work(() =>
              send(
                `/resource-instances/${id}/protection`,
                { delete_protection: !resource.delete_protection },
                "PATCH",
              ),
            )
          }
        >
          {resource.delete_protection
            ? "Remove deletion protection"
            : "Protect from deletion"}
        </button>
        <button
          className={buttonClass}
          disabled={busy}
          onClick={() => {
            if (
              window.confirm(
                "Remove from the local directory? The remote resource will be preserved.",
              )
            )
              void work(async () => {
                await apiFetch(`/resource-instances/${id}`, {
                  method: "DELETE",
                });
                navigate("/inventory");
              });
          }}
        >
          Forget locally; retain remote resource
        </button>
      </div>
      {!descriptor && (
        <p>
          This historical resource type is not registered. Its identity and
          snapshots remain available; operations are disabled.
        </p>
      )}
      <h2 className="font-semibold">Relations</h2>
      {detail.data?.relations.map((rel) => (
        <div className="rounded border p-3" key={rel.id}>
          <p>
            {rel.relation_type} · {rel.origin} ·{" "}
            {rel.observed_access_id ?? "annotation"}
          </p>
          {rel.reference.resolved_resource_id ? (
            <Link to={`/inventory/${rel.reference.resolved_resource_id}`}>
              Open verified target
            </Link>
          ) : (
            <p>Unresolved remote reference</p>
          )}
          <Snapshot value={rel.reference} />
          {rel.origin === "user" && (
            <button
              className={buttonClass}
              disabled={busy}
              onClick={() =>
                void work(() =>
                  apiFetch(`/resource-relations/${rel.id}`, {
                    method: "DELETE",
                  }),
                )
              }
            >
              Remove annotation
            </button>
          )}
        </div>
      ))}
      <form
        className="flex flex-wrap gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          void work(async () => {
            const r = resources.data?.find((r) => r.id === target);
            if (!r) throw new Error("Select a resource");
            const t = types(catalog.data).find(
              (t) => t.id === r.resource_type_id,
            );
            if (!t) throw new Error("Target type unavailable");
            await send("/resource-relations", {
              from_resource_id: resource.id,
              relation_type: relationType,
              reference: {
                provider_id: t.provider_id,
                provider_instance_id: r.provider_instance_id,
                resource_type_id: r.resource_type_id,
                identity_parts: JSON.parse(r.identity_key),
                remote: r.locator,
              },
              attributes: document({}),
            });
          });
        }}
      >
        <select
          value={relationType}
          onChange={(e) => setRelationType(e.target.value)}
        >
          {catalog.data?.relations
            .filter((r) => r.origin === "user")
            .map((r) => (
              <option key={r.id} value={r.id}>
                {r.id}
              </option>
            ))}
        </select>
        <select
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          required
        >
          <option value="">Select related resource</option>
          {resources.data
            ?.filter((r) => r.id !== id)
            .map((r) => (
              <option key={r.id} value={r.id}>
                {r.display_name}
              </option>
            ))}
        </select>
        <button className={buttonClass} disabled={busy}>
          Add annotation
        </button>
      </form>
      <h2 className="font-semibold">Live views</h2>
      {descriptor?.views.map((v) => (
        <ViewPanel
          key={v.id}
          view={v}
          resourceID={resource.id}
          accessID={selected?.id}
        />
      ))}
      <h2 className="font-semibold">Write operations</h2>
      {descriptor?.actions
        .filter((a) => a.target === "resource")
        .map((action) => (
          <ActionPanel
            key={action.id}
            action={action}
            resource={resource}
            accessID={selected?.id}
            allowed={
              (selected?.capabilities[action.capability]?.availability ===
                "available" ||
                (selected?.capabilities[action.capability]?.availability ===
                  "unknown" &&
                  action.authorization === "remote")) &&
              !selected?.error_code &&
              (action.effect !== "delete" ||
                (resource.origin === "mevius" && !resource.delete_protection))
            }
            onDone={() => qc.invalidateQueries({ queryKey: ["resource", id] })}
          />
        ))}
      <OperationHistory resourceID={resource.id} />
    </div>
  );
}
function ViewPanel({
  view,
  resourceID,
  accessID,
}: {
  view: View;
  resourceID: string;
  accessID?: string;
}) {
  const [input, setInput] = useState<Record<string, unknown>>({});
  const [result, setResult] = useState<unknown>();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  return (
    <form
      className="space-y-3 rounded border p-4"
      onSubmit={async (e) => {
        e.preventDefault();
        setBusy(true);
        setError(undefined);
        try {
          setResult(
            await apiFetch(
              `/resource-instances/${resourceID}/views/${view.id}?access_id=${encodeURIComponent(accessID ?? "")}&input=${encodeURIComponent(JSON.stringify(document(input, view.input.version)))}`,
            ),
          );
        } catch (e) {
          setError(e);
        } finally {
          setBusy(false);
        }
      }}
    >
      <h3>{view.display_name}</h3>
      <SchemaFields schema={view.input} value={input} onChange={setInput} />
      <button className={buttonClass} disabled={!accessID || busy}>
        Read live view
      </button>
      <ErrorMessage error={error} />
      {result !== undefined && <Snapshot value={result} />}
    </form>
  );
}
function ActionPanel({
  action,
  resource,
  accessID,
  allowed,
  onDone,
}: {
  action: Action;
  resource: Resource;
  accessID?: string;
  allowed: boolean;
  onDone: () => Promise<unknown>;
}) {
  const [input, setInput] = useState<Record<string, unknown>>({});
  const [op, setOp] = useState<Operation>();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  return (
    <form
      className="space-y-3 rounded border p-4"
      onSubmit={async (e) => {
        e.preventDefault();
        if (
          action.effect === "delete" &&
          !window.confirm("Permanently delete the remote resource?")
        )
          return;
        setBusy(true);
        setError(undefined);
        try {
          const result = await submitOperation({
            resource_type_id: resource.resource_type_id,
            action_id: action.id,
            target_kind: "resource",
            target_id: resource.id,
            access_id: accessID,
            input: document(input, action.input.version),
          });
          setOp(result);
          await onDone();
        } catch (e) {
          setError(e);
        } finally {
          setBusy(false);
        }
      }}
    >
      <h3>{action.display_name}</h3>
      <SchemaFields schema={action.input} value={input} onChange={setInput} />
      <button
        className={buttonClass}
        disabled={!allowed || !accessID || busy || op?.status === "unknown"}
      >
        Submit {action.display_name}
      </button>
      {!allowed && (
        <p className="text-sm text-muted-foreground">
          This access or deletion policy does not allow the action.
        </p>
      )}
      <ErrorMessage error={error} />
      {op && (
        <>
          <p>Operation status: {op.status}</p>
          {op.status === "unknown" && (
            <p role="alert">
              Remote outcome unknown. Check the provider before submitting
              another operation.
            </p>
          )}
          <Snapshot value={op} />
        </>
      )}
    </form>
  );
}
function OperationHistory({ resourceID }: { resourceID: string }) {
  const operations = useQuery({
    queryKey: ["operations"],
    queryFn: () => apiFetch<Operation[]>("/operations"),
  });
  return (
    <details>
      <summary>Operation journal</summary>
      <ErrorMessage error={operations.error} />
      {operations.data
        ?.filter((o) => o.target_id === resourceID)
        .map((o) => (
          <Snapshot key={o.id} value={o} />
        ))}
    </details>
  );
}
