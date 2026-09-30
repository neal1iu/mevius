import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { apiFetch } from "@/lib/api";
import { send, type Project } from "@/lib/catalog";
import { ErrorMessage, buttonClass } from "@/components/CatalogForm";
export default function Projects() {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const projects = useQuery({
    queryKey: ["projects"],
    queryFn: () => apiFetch<Project[]>("/projects"),
  });
  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-semibold">Projects</h1>
      <ErrorMessage error={error ?? projects.error} />
      <form
        className="flex flex-wrap gap-3"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError(undefined);
          try {
            await send("/projects", { name, description });
            setName("");
            setDescription("");
            await qc.invalidateQueries({ queryKey: ["projects"] });
          } catch (e) {
            setError(e);
          } finally {
            setBusy(false);
          }
        }}
      >
        <input
          className="rounded border p-2"
          placeholder="Project name"
          aria-label="Project name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
        <input
          className="rounded border p-2"
          placeholder="Purpose"
          aria-label="Purpose"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
        <button className={buttonClass} disabled={busy}>
          Create project
        </button>
      </form>
      <div className="grid gap-3">
        {projects.data?.map((p) => (
          <Link
            className="rounded border p-4 hover:bg-muted"
            key={p.id}
            to={`/projects/${p.id}`}
          >
            <h2>{p.name}</h2>
            <p className="text-muted-foreground">{p.description}</p>
          </Link>
        ))}
      </div>
    </div>
  );
}
