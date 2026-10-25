# plural-api

Backend for Plural, a decentralized, open-source social network.

Starts as a monolith. Each package under `internal/` is a domain that can move
into its own service later.

## Layout

| Path | What |
|---|---|
| `cmd/plural-api` | HTTP server: flags, wiring, handlers (`<domain>Handlers.go`) |
| `internal/<domain>` | Domain types and logic (e.g. `account`) |
| `internal/postgres` | Postgres store and migrations (applied on startup) |
| `sql/create.sql` | Latest full schema; keep in sync with migrations |

## Development

```bash
make run        # start Postgres in Docker and run the API on :4100
make test       # go test -cover -race ./...
make generate   # regenerate mocks (//go:generate go tool mock ...)
make fmt        # gofumpt
```

| Flag | Default | Use |
|---|---|---|
| `-cors.origins` | `http://localhost:8081` | Comma-separated allowed browser origins; `*` allows any |
| `-instance.domain` | `localhost` | Public domain of this instance |
| `-instance.name` | `Plural` | Display name of this instance |

Flags can also be set as env vars (`-db.connection` → `DB_CONNECTION`).

## Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check (pings Postgres) |
| `POST` | `/accounts` | Create an account: `{"username","email","password"}` |
| `GET` | `/instance` | Instance info: `{"domain","name","version"}` |
