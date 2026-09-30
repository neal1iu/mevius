import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";
import {
  document,
  send,
  types,
  type Catalog,
  type Instance,
  type Connection,
  type OAuthInfo,
  type ScopeResponse,
} from "@/lib/catalog";
import {
  ErrorMessage,
  Snapshot,
  SchemaFields,
  buttonClass,
} from "@/components/CatalogForm";
export default function Accounts() {
  const qc = useQueryClient();
  const [provider, setProvider] = useState("");
  const [key, setKey] = useState("public");
  const [endpoint, setEndpoint] = useState<Record<string, unknown>>({});
  const [instanceID, setInstanceID] = useState("");
  const [label, setLabel] = useState("");
  const [credential, setCredential] = useState("");
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const [scopes, setScopes] = useState<{
    connection: string;
    response: ScopeResponse;
  }>();
  const [clientID, setClientID] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [authURL, setAuthURL] = useState("");
  const [tokenURL, setTokenURL] = useState("");
  const [redirect, setRedirect] = useState(window.location.origin);
  const [oauthScopes, setOAuthScopes] = useState("");
  const [pkce, setPKCE] = useState(true);
  const [oauthSession, setOAuthSession] = useState(
    new URLSearchParams(window.location.search).get("oauth_session") ?? "",
  );
  const catalog = useQuery({
    queryKey: ["catalog"],
    queryFn: () => apiFetch<Catalog>("/catalog"),
  });
  const instances = useQuery({
    queryKey: ["instances"],
    queryFn: () => apiFetch<Instance[]>("/provider-instances"),
  });
  const connections = useQuery({
    queryKey: ["connections"],
    queryFn: () => apiFetch<Connection[]>("/connections"),
  });
  const oauth = useQuery({
    queryKey: ["oauth"],
    queryFn: () => apiFetch<OAuthInfo[]>("/connections/oauth/providers"),
  });
  const session = useQuery({
    queryKey: ["oauth-session", oauthSession],
    queryFn: () =>
      apiFetch<{ status: string; error_code?: string }>(
        `/connections/oauth/sessions/${oauthSession}`,
      ),
    enabled: !!oauthSession,
  });
  const instanceSchema = catalog.data?.providers?.find(
    (p) => p.id === provider,
  )?.instance;
  const providerIDs = [
    ...new Set(types(catalog.data).map((t) => t.provider_id)),
  ];
  const selected = instances.data?.find((i) => i.id === instanceID);
  async function work(fn: () => Promise<unknown>) {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      await qc.invalidateQueries({ queryKey: ["connections"] });
      await qc.invalidateQueries({ queryKey: ["instances"] });
      await qc.invalidateQueries({ queryKey: ["oauth"] });
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-semibold">Provider authorizations</h1>
      <ErrorMessage
        error={error ?? connections.error ?? instances.error ?? oauth.error}
      />
      <form
        className="grid gap-3 rounded border p-4"
        onSubmit={(e) => {
          e.preventDefault();
          void work(async () => {
            const i = await send<Instance>("/provider-instances", {
              provider_id: provider,
              instance_key: key,
              endpoint: document(endpoint, instanceSchema?.version ?? 1),
            });
            setInstanceID(i.id);
          });
        }}
      >
        <h2 className="font-medium">Add deployment instance</h2>
        <select
          aria-label="Provider"
          value={provider}
          onChange={(e) => {
            setProvider(e.target.value);
            setEndpoint({});
          }}
          required
        >
          <option value="">Select provider</option>
          {providerIDs.map((id) => (
            <option key={id}>{id}</option>
          ))}
        </select>
        <input
          aria-label="Instance key"
          className="rounded border p-2"
          placeholder="Stable instance key"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          required
        />
        {instanceSchema && (
          <SchemaFields
            schema={instanceSchema}
            value={endpoint}
            onChange={setEndpoint}
          />
        )}
        <button className={buttonClass} disabled={busy}>
          Add instance
        </button>
      </form>
      <div className="space-y-3 rounded border p-4">
        <h2 className="font-medium">Authorize an instance</h2>
        <select
          aria-label="Deployment instance"
          className="w-full rounded border p-2"
          value={instanceID}
          onChange={(e) => setInstanceID(e.target.value)}
        >
          <option value="">Select deployment instance</option>
          {instances.data?.map((i) => (
            <option key={i.id} value={i.id}>
              {i.provider_id} · {i.instance_key}
            </option>
          ))}
        </select>
        <input
          aria-label="Authorization label"
          className="w-full rounded border p-2"
          placeholder="Authorization label"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
        />
        <form
          className="flex gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            void work(async () => {
              await send("/connections", {
                provider_instance_id: instanceID,
                label,
                credential,
              });
              setCredential("");
            });
          }}
        >
          <input
            aria-label="Authorization token"
            className="flex-1 rounded border p-2"
            type="password"
            autoComplete="off"
            placeholder="Authorization token"
            value={credential}
            onChange={(e) => setCredential(e.target.value)}
            required
          />
          <button className={buttonClass} disabled={busy || !instanceID}>
            Save authorization
          </button>
        </form>
        <button
          className={buttonClass}
          disabled={
            busy ||
            !instanceID ||
            !oauth.data?.find((p) => p.provider_id === selected?.provider_id)
              ?.configured
          }
          onClick={() =>
            void work(async () => {
              const v = await send<{ authorization_url: string }>(
                "/connections/oauth/start",
                { provider_instance_id: instanceID },
              );
              window.location.assign(v.authorization_url);
            })
          }
        >
          Authorize with OAuth
        </button>
      </div>
      {oauthSession && (
        <div className="rounded border p-4">
          <p>
            OAuth session: {session.data?.status ?? "loading"}{" "}
            {session.data?.error_code}
          </p>
          <ErrorMessage error={session.error} />
          <button
            className={buttonClass}
            disabled={busy || session.data?.status !== "authorized"}
            onClick={() =>
              void work(async () => {
                await send("/connections/oauth/complete", {
                  session_id: oauthSession,
                  label: label || "OAuth authorization",
                });
                setOAuthSession("");
                window.history.replaceState({}, "", window.location.pathname);
              })
            }
          >
            Save OAuth authorization
          </button>
        </div>
      )}
      <details className="rounded border p-4">
        <summary>OAuth application configuration</summary>
        <p>
          Client secrets are encrypted. Remote product secrets are never stored
          here.
        </p>
        <form
          className="mt-3 grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            void work(() =>
              send(
                `/connections/oauth/providers/${provider}/configuration`,
                {
                  client_id: clientID,
                  client_secret: clientSecret,
                  authorization_url: authURL,
                  token_url: tokenURL,
                  scopes: oauthScopes.split(/[ ,]+/).filter(Boolean),
                  pkce,
                  redirect_base_url: redirect,
                },
                "PUT",
              ).then(() => setClientSecret("")),
            );
          }}
        >
          <select
            aria-label="OAuth provider"
            value={provider}
            onChange={(e) => {
              setProvider(e.target.value);
              const config = oauth.data?.find(
                (p) => p.provider_id === e.target.value,
              )?.configuration;
              if (config) {
                setClientID(config.client_id ?? "");
                setClientSecret("");
                setAuthURL(config.authorization_url ?? "");
                setTokenURL(config.token_url ?? "");
                setOAuthScopes(config.scopes?.join(" ") ?? "");
                setPKCE(config.pkce ?? true);
                setRedirect(config.redirect_base_url || window.location.origin);
              }
            }}
            required
          >
            <option value="">Select provider</option>
            {oauth.data?.map((p) => (
              <option key={p.provider_id} value={p.provider_id}>
                {p.provider_id} · {p.configured ? "configured" : "unconfigured"}
              </option>
            ))}
          </select>
          {[
            ["Client ID", clientID, setClientID],
            ["Client secret", clientSecret, setClientSecret],
            ["Authorization URL", authURL, setAuthURL],
            ["Token URL", tokenURL, setTokenURL],
            ["Redirect base URL", redirect, setRedirect],
            ["Scopes", oauthScopes, setOAuthScopes],
          ].map(([name, value, set]) => (
            <input
              key={name as string}
              aria-label={name as string}
              className="rounded border p-2"
              type={name === "Client secret" ? "password" : "text"}
              placeholder={name as string}
              value={value as string}
              onChange={(e) => (set as (v: string) => void)(e.target.value)}
            />
          ))}
          <label>
            <input
              type="checkbox"
              checked={pkce}
              onChange={(e) => setPKCE(e.target.checked)}
            />{" "}
            PKCE
          </label>
          <button className={buttonClass} disabled={busy}>
            Save OAuth application
          </button>
        </form>
      </details>
      <div className="grid gap-3">
        {connections.data?.map((c) => (
          <div key={c.id} className="space-y-2 rounded border p-4">
            <h2>{c.label}</h2>
            <p>
              {
                instances.data?.find((i) => i.id === c.provider_instance_id)
                  ?.provider_id
              }{" "}
              · {c.auth_scheme} · revision {c.credential_revision} ·{" "}
              {c.authorization_state}
            </p>
            <Snapshot value={c.principal} />
            <div className="flex gap-3">
              <button
                className={buttonClass}
                disabled={busy}
                onClick={() =>
                  void work(async () =>
                    setScopes({
                      connection: c.id,
                      response: await apiFetch<ScopeResponse>(
                        `/connections/${c.id}/scopes`,
                      ),
                    }),
                  )
                }
              >
                Discover scopes
              </button>
              <button
                className={buttonClass}
                disabled={busy || !credential}
                onClick={() =>
                  void work(() =>
                    send(
                      `/connections/${c.id}/credential`,
                      { credential },
                      "PATCH",
                    ).then(() => setCredential("")),
                  )
                }
              >
                Rotate with entered token
              </button>
              <button
                className={buttonClass}
                disabled={busy}
                onClick={() => {
                  if (
                    window.confirm(
                      "Remove this authorization? Directory resources and project attachments will remain.",
                    )
                  )
                    void work(() =>
                      apiFetch(`/connections/${c.id}`, { method: "DELETE" }),
                    );
                }}
              >
                Remove authorization
              </button>
            </div>
            {scopes?.connection === c.id &&
              scopes.response.available.map((candidate) => (
                <button
                  className={buttonClass}
                  key={JSON.stringify(candidate.identity_parts)}
                  disabled={busy}
                  onClick={() =>
                    void work(() =>
                      send(`/connections/${c.id}/scopes`, candidate),
                    )
                  }
                >
                  Bind / revalidate {candidate.label}
                </button>
              ))}
          </div>
        ))}
      </div>
    </div>
  );
}
