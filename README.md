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

This populates the database with sample projects and slots for exploration.

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