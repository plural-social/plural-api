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
make run        # start Postgres and SeaweedFS (S3) in Docker and run the API on :4100
make test       # go test -cover -race ./...
make seed       # add test users (alice, bob, carol; password123), posts and follows. Needs curl, jq, ffmpeg
make generate   # regenerate mocks (//go:generate go tool mock ...)
make fmt        # gofumpt
```

| Flag | Default | Use |
|---|---|---|
| `-cors.origins` | `http://localhost:8081` | Comma-separated allowed browser origins; `*` allows any |
| `-instance.domain` | `localhost` | Public domain of this instance |
| `-instance.name` | `Plural` | Display name of this instance |
| `-trust.proxy.headers` | `false` | Use `X-Forwarded-For` / `X-Forwarded-Proto` (rate limits, file URLs). Only behind a proxy that sets them |
| `-public.url` | empty | Base URL for file links. Empty builds links from the request host |
| `-s3.endpoint` | `localhost:8333` | S3-compatible storage (`host:port`, no scheme) |
| `-s3.access.key` / `-s3.secret.key` | `plural` | S3 credentials |
| `-s3.bucket` | `plural-media` | Bucket, created on startup if missing |
| `-s3.region` | `us-east-1` | S3 region |
| `-s3.use.ssl` | `false` | Use HTTPS to reach S3 |

Flags can also be set as env vars (`-db.connection` → `DB_CONNECTION`).

## Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check (pings Postgres) |
| `POST` | `/accounts` | Create an account: `{"username","email","password"}` |
| `GET` | `/instance` | Instance info: `{"domain","name","version"}` |
| `POST` | `/sessions` | Log in: `{"login","password"}` (username or email). Returns `{token,expiresAt,me}`. Rate limited per IP |
| `DELETE` | `/sessions/current` | Log out (bearer token required) |
| `GET` | `/me` | Current account (bearer token required) |
| `PATCH` | `/me` | Update `{"displayName","bio"}`; omitted fields are unchanged |
| `GET` | `/users/{username}` | Public profile. Token optional (sets `isSelf`) |

`POST /accounts` is also rate limited per IP.

```bash
curl -s -X POST localhost:4100/sessions -d '{"login":"alice","password":"password123"}'
curl -s localhost:4100/me -H "Authorization: Bearer $TOKEN"
```
| `POST` | `/media` | Upload an image as raw bytes (`Content-Type: image/jpeg\|png\|webp`, max 20 MB). Re-encoded to a 2048 px display JPEG and a 480 px thumbnail; all metadata (incl. GPS) is dropped. Rate limited |
| `GET` | `/media/{uuid}` | Your own media item (bearer token required) |
| `GET` | `/files/{key...}` | Stored files under `media/`, with HTTP range support |
| `POST` | `/posts` | Create a post from your media: `{"mediaUuid","caption"}` (bearer token required) |
| `GET` | `/posts/{uuid}` | A post. Token optional |
| `DELETE` | `/posts/{uuid}` | Delete your post and its files |
| `GET` | `/users/{username}/posts` | A user's posts, newest first. `?limit=` (1-30) and `?cursor=` for paging |
| `PUT` | `/users/{username}/follow` | Follow a user (idempotent, bearer token required) |
| `DELETE` | `/users/{username}/follow` | Unfollow (idempotent) |
| `GET` | `/feed/following` | Posts by you and people you follow (bearer token required) |
| `GET` | `/feed/explore` | All posts on this instance. Token optional |

Feeds only contain posts whose media is ready, newest first, with the same `?limit=` and `?cursor=` paging as other lists.

```bash
curl -s -X POST localhost:4100/media -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: image/jpeg' --data-binary @photo.jpg
```

## Storage

Files live in any S3-compatible service. Development uses SeaweedFS (compose).
Keys: `media/<uuid>/display.jpg`, `media/<uuid>/thumb.jpg`. Keys never change
content, so `/files` responses are cached for a year.
