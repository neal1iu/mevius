import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import { apiFetch } from "@/lib/api";
import {
  send,
  types,
  type Catalog,
  type Project,
  type Resource,
  type Attachment,
} from "@/lib/catalog";
import { ErrorMessage, buttonClass } from "@/components/CatalogForm";
export default function ProjectDetail() {
  const { id } = useParams();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [resourceID, setResourceID] = useState("");
  const [alias, setAlias] = useState("");
  const [role, setRole] = useState("");
  const [purpose, setPurpose] = useState("");
  const [environment, setEnvironment] = useState("");
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const detail = useQuery({
    queryKey: ["project", id],
    queryFn: () =>
      apiFetch<{ project: Project; resources: Attachment[] }>(
        `/projects/${id}`,
      ),
  });
  const resources = useQuery({
    queryKey: ["resources"],
    queryFn: () => apiFetch<Resource[]>("/resource-instances"),
  });
  const catalog = useQuery({
    queryKey: ["catalog"],
    queryFn: () => apiFetch<Catalog>("/catalog"),
  });
  const selected = resources.data?.find((r) => r.id === resourceID);
  const roles =
    types(catalog.data).find((t) => t.id === selected?.resource_type_id)
      ?.compatible_roles ?? [];
  async function work(fn: () => Promise<unknown>) {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      await qc.invalidateQueries({ queryKey: ["project", id] });
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-5 p-6">
      <Link to="/projects">← Projects</Link>
      <h1 className="text-2xl font-semibold">
        {detail.data?.project.name ?? "Project"}
      </h1>
      <p>{detail.data?.project.description}</p>
      <ErrorMessage error={error ?? detail.error} />
      <p>
        Attach resources independently. Parents and dependencies can remain
        outside this project. A resource can have multiple aliases.
      </p>
      <form
        className="grid gap-3 rounded border p-4"
        onSubmit={(e) => {
          e.preventDefault();
          void work(async () => {
            await send(`/projects/${id}/resources`, {
              resource_id: resourceID,
              alias,
              role: role || roles[0],
              purpose,
              environment,
            });
            setAlias("");
          });
        }}
      >
        <select
          aria-label="Resource"
          value={resourceID}
          onChange={(e) => {
            setResourceID(e.target.value);
            setRole("");
          }}
          required
        >
          <option value="">Select resource</option>
          {resources.data?.map((r) => (
            <option key={r.id} value={r.id}>
              {r.display_name}
            </option>
          ))}
        </select>
        <input
          className="rounded border p-2"
          aria-label="Alias"
          placeholder="Alias"
          value={alias}
          onChange={(e) => setAlias(e.target.value)}
          required
        />
        <select
          aria-label="Role"
          value={role || roles[0] || ""}
          onChange={(e) => setRole(e.target.value)}
          required
        >
          {roles.map((r) => (
            <option key={r}>{r}</option>
          ))}
        </select>
        <input
          className="rounded border p-2"
          aria-label="Purpose"
          placeholder="Purpose"
          value={purpose}
          onChange={(e) => setPurpose(e.target.value)}
        />
        <input
          className="rounded border p-2"
          aria-label="Environment label"
          placeholder="Optional environment label"
          value={environment}
          onChange={(e) => setEnvironment(e.target.value)}
        />
        <button className={buttonClass} disabled={busy || !roles.length}>
          Attach resource
        </button>
      </form>
      {detail.data?.resources.map((link) => (
        <div className="flex justify-between rounded border p-4" key={link.id}>
          <div>
            <Link to={`/inventory/${link.resource_id}`}>{link.alias}</Link>
            <p>
              {link.role} · {link.purpose} · {link.environment}
            </p>
          </div>
          <button
            className={buttonClass}
            disabled={busy}
            onClick={() =>
              void work(() =>
                apiFetch(`/projects/${id}/resources/${link.id}`, {
                  method: "DELETE",
                }),
              )
            }
          >
            Detach
          </button>
        </div>
      ))}
      <button
        className={buttonClass}
        disabled={busy}
        onClick={() => {
          if (
            window.confirm(
              "Delete this local project and its attachments? Remote resources will be preserved.",
            )
          )
            void work(async () => {
              await apiFetch(`/projects/${id}`, { method: "DELETE" });
              await qc.invalidateQueries({ queryKey: ["projects"] });
              navigate("/projects");
            });
        }}
      >
        Delete local project
      </button>
    </div>
  );
}
