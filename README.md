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

## Provider instances and authorizations

Create a deployment instance in Connections, then add token or OAuth
credentials. An authorization can bind several remote scopes. Replacing or
removing credentials preserves resource IDs and project attachments. Inventory
imports resources through a bound scope; details let you select the exact
access path used for observations, views and actions.

OAuth application configuration is available in Connections. Register the
callback `https://YOUR-MEVIUS/api/v1/connections/oauth/callback/PROVIDER`.
Environment configuration takes precedence over the editable database settings.
Vercel needs an integration installation URL (or `MEVIUS_VERCEL_OAUTH_SLUG`
for environment configuration). OAuth tokens and PKCE verifiers stay on the
server; session consumption creates the authorization atomically and erases
session secrets. Expired sessions are scrubbed. Expiring OAuth credentials
currently require reauthorization; no refresh-token background process is used.

## Explicit epoch 6 reset

This pre-release changes the database and `/api/v1` wire shape. Epoch 5 and
other old databases are refused without mutation. **Startup never resets data.**
Stop the server, create a consistent backup and explicitly reset:

```bash
# Local installation; uses MEVIUS_DB_PATH (default ./mevius.db).
mevius reset --confirm --backup ./mevius-pre-epoch6.backup.db
mevius seed # optional offline demo snapshot; no fake usable credentials
```

For Compose, stop the API before resetting the same named volume:

```bash
docker compose stop api
docker compose run --rm api reset --confirm --backup /data/mevius-pre-epoch6.backup.db
docker compose run --rm --entrypoint sh api -c 'cat /data/mevius-pre-epoch6.backup.db' > ./mevius-pre-epoch6.backup.db
docker compose up -d
```

The backup command uses SQLite `VACUUM INTO`, including committed WAL data.
It refuses an existing backup path and initializes epoch 6 only after backup
succeeds. Backups contain encrypted authorization data: preserve the master key
separately and protect backups. Restoring epoch 5 requires an epoch 5 binary.
After release, the baseline is frozen and changes use incremental migrations.

## Architecture and development

See [the resource catalog contract](docs/resource-catalog.md) for integration
interfaces, identity rules, operation guarantees and the acceptance-test map.
New integrations implement a compiled Go adapter plus versioned product/type
schemas. Products may declare multiple resource types. The core and generic UI
have no resource-kind/provider routing branches.

```bash
# Backend: set MEVIUS_API_TOKEN and MEVIUS_MASTER_KEY first.
go run ./cmd/mevius
# Frontend, separate terminal (API proxy points at localhost:8080).
cd web
npm ci
npm run dev
```

```bash
go test -race ./...
go vet ./...
cd web
npm run build
npm run test -- --run
npm run lint
```

Use HTTPS for deployed instances. The UI keeps stable, opaque idempotency keys
across HTTP retries and page reloads without storing action payloads. Unknown
operation outcomes are displayed and never automatically executed again.
Remote deletion is restricted to proven Mevius creations with an available
selected permission, no protection and no blocking attachments/references.
Forgetting removes local inventory only and preserves the remote resource.
