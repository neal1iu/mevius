# mevius

A self-hosted control plane for composable free-tier developer stacks.

## Quick start

```bash
cp .env.example .env
# Edit .env — both MEVIUS_MASTER_KEY and MEVIUS_API_TOKEN are required.
# Generate values with: openssl rand -hex 32

docker compose up --build -d
```

Open http://localhost, enter your `MEVIUS_API_TOKEN`, and start composing.

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `MEVIUS_MASTER_KEY` | ✓ | — | 256-bit master key (64 hex chars). Generate with `openssl rand -hex 32`. |
| `MEVIUS_API_TOKEN` | ✓ | — | Bearer token for API authentication. Generate with `openssl rand -hex 32`. |
| `MEVIUS_DB_PATH` | | `/data/mevius.db` | SQLite database path (inside the api container; persisted to the `mevius-data` volume). |
| `MEVIUS_HTTP_PORT` | | `80` | Host port for the web UI (e.g., set to `8080` if port 80 is in use). |
| `MEVIUS_PUBLIC_URL` | Environment-configured OAuth only | — | Browser-reachable Mevius origin, such as `https://mevius.example.com`. The Accounts form stores this value per OAuth provider instead. |
| `MEVIUS_GITHUB_OAUTH_CLIENT_ID` / `..._CLIENT_SECRET` | | — | Enables GitHub OAuth. Both values are required together. |
| `MEVIUS_GITHUB_OAUTH_SCOPES` | | `repo workflow delete_repo read:org` | Space- or comma-separated scopes requested by GitHub OAuth. |
| `MEVIUS_CLOUDFLARE_OAUTH_CLIENT_ID` / `..._CLIENT_SECRET` | | — | Enables Cloudflare OAuth. Both values are required together. |
| `MEVIUS_CLOUDFLARE_OAUTH_SCOPES` | | — | Least-privilege scopes configured for the Cloudflare OAuth client. |
| `MEVIUS_VERCEL_OAUTH_CLIENT_ID` / `..._CLIENT_SECRET` | | — | Enables a Vercel connectable-account integration. Both values are required together. |
| `MEVIUS_VERCEL_OAUTH_SLUG` | Vercel OAuth only | — | URL slug of the Vercel integration used to build its installation URL. |

## OAuth connections

Token connections and OAuth connections can be used side by side. Select OAuth
on the Accounts page to open the OAuth application setup form. Mevius prefills
the provider's standard authorization endpoint, token endpoint, scopes, and
PKCE setting. Enter the OAuth client ID, client secret, and public Mevius URL,
then register the callback URL shown by the form with the provider.

Environment variables remain available for immutable deployments. When both
exist, the encrypted database configuration saved by the form takes precedence.
Example callback URLs are:

```text
https://mevius.example.com/api/v1/connections/oauth/callback/github
https://mevius.example.com/api/v1/connections/oauth/callback/cloudflare
https://mevius.example.com/api/v1/connections/oauth/callback/vercel
```

After configuration, the Accounts page starts the provider authorization flow. Mevius keeps the
state and PKCE verifier in a short-lived server-side session, exchanges the
authorization code on the backend, encrypts the resulting credential, then
asks the user to choose one discovered provider scope. Access and refresh
tokens are never returned to frontend JavaScript.

GitHub and Cloudflare use their standard OAuth applications. Vercel requires a
connectable-account integration and its URL slug. Provider-specific endpoint
and scope overrides are available through `..._AUTH_URL`, `..._TOKEN_URL`, and
`..._SCOPES` variables with the same provider prefix.

## Backup

```bash
# The SQLite database is stored on a Docker volume. To back it up:
docker compose exec api sh -c 'cp /data/mevius.db /tmp/backup.db && docker compose cp api:/tmp/backup.db ./mevius-backup-$(date +%F).db'
```

Restore by copying a backup into the volume and restarting.

## Seed data

```bash
docker compose exec api mevius seed
```

This populates the database with scoped connections, global resource instances,
project links, and an example `source_repo` relation.

## Schema epoch 4 reset

This release intentionally replaces the old Slot/Binding database with the
ResourceInstance model. There is no automatic data migration. On startup,
Mevius detects an old `slot` or `binding` table and exits before changing it.
Existing ResourceInstance databases at epoch 2 or 3 are migrated in place to add
OAuth connection metadata, authorization sessions, and encrypted OAuth client
configuration.

Back up before resetting:

```bash
docker compose exec api sh -c 'cp /data/mevius.db /tmp/mevius-epoch1.db'
docker compose cp api:/tmp/mevius-epoch1.db ./mevius-epoch1.db
```

For a local (non-Compose) database, stop Mevius and remove the configured DB
file plus its `-wal` and `-shm` sidecars. For the Compose database, reset the
named volume explicitly:

```bash
docker compose down
docker volume rm mevius_mevius-data
docker compose up --build -d
docker compose exec api mevius seed
```

The volume name can differ when `COMPOSE_PROJECT_NAME` is set; use
`docker volume ls` to identify the project-scoped `mevius-data` volume. Mevius
never deletes a database, volume, backup, or untracked file automatically.

## Development

```bash
# Terminal 1 — backend (hot-reload via CompileDaemon or plain go run)
MEVIUS_MASTER_KEY=$(openssl rand -hex 32) \
MEVIUS_API_TOKEN=$(openssl rand -hex 32) \
go run ./cmd/mevius

# Terminal 2 — frontend (Vite dev server with proxy to backend)
cd web && npm run dev
```

The Vite dev server proxies `/api` to `http://localhost:8080`, so no CORS setup is needed.

## Architecture

- **api**: Go HTTP server (chi router) serving REST at `/api/v1/*`
- **web**: React SPA (Vite + Tailwind + shadcn/ui) consuming the same-origin API
- **nginx**: Production reverse proxy — serves static assets, proxies `/api/` to the api container, handles SPA fallback routing

The control-plane model is:

```text
ProviderConnection -> ResourceInstance <- ProjectResource <- Project
                          |
                          +---- ResourceRelation ----> ResourceInstance
```

Provider and product definitions live in the Go registry. Pipeline runs,
deployments, logs, and DNS records are read from providers in real time and are
not persisted.
