# plural-api

Backend for Plural, a decentralized, open-source social network.

Starts as a monolith. Each package under `internal/` is a domain that can move
into its own service later.

## Layout

| Path | What |
|---|---|
| `cmd/plural-api` | HTTP server: flags, wiring, handlers (`<domain>Handlers.go`) |
| `cmd/plural-api-transcoder` | Video worker: claims jobs from Postgres and runs ffmpeg |
| `internal/<domain>` | Domain types and logic (e.g. `account`) |
| `internal/postgres` | Postgres store and migrations (applied on startup) |
| `sql/create.sql` | Latest full schema; keep in sync with migrations |

## Development

```bash
make run              # start Postgres and SeaweedFS (S3) in Docker and run the API on :4100
make run-transcoder   # run the video transcoder (needs ffmpeg: brew install ffmpeg)
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
| `PUT` | `/me/avatar` | Set your avatar from your own ready image that no post uses: `{"mediaUuid"}`. Returns your account |
| `DELETE` | `/me/avatar` | Remove your avatar |
| `GET` | `/users/{username}` | Public profile. Token optional (sets `isSelf`) |

`POST /accounts` is also rate limited per IP.

```bash
curl -s -X POST localhost:4100/sessions -d '{"login":"alice","password":"password123"}'
curl -s localhost:4100/me -H "Authorization: Bearer $TOKEN"
```
| `POST` | `/media` | Upload raw bytes. Images (`image/jpeg\|png\|webp`, max 20 MB) are processed at once. Videos (`video/mp4\|quicktime\|webm`, max 300 MB, 1 s to 3 min) return `status: processing` and become `ready` or `failed` after the transcoder runs. For images: (`Content-Type: image/jpeg\|png\|webp`, max 20 MB). Re-encoded to a 2048 px display JPEG and a 480 px thumbnail; all metadata (incl. GPS) is dropped. Rate limited |
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

Every user (`author`, profile, `me`) includes `avatarUrl` when an avatar is set. It is the 480 px thumbnail.

Feeds only contain posts whose media is ready, newest first, with the same `?limit=` and `?cursor=` paging as other lists.
| `PUT` | `/posts/{uuid}/like` | Like a post (idempotent, bearer token required) |
| `DELETE` | `/posts/{uuid}/like` | Remove your like |
| `GET` | `/posts/{uuid}/comments` | Comments on a post, newest first. Token optional |
| `POST` | `/posts/{uuid}/comments` | Comment: `{"body"}` (1-500 characters) |
| `DELETE` | `/comments/{uuid}` | Delete a comment (its author or the post's author) |

Post responses include `likeCount`, `commentCount` and `likedByMe`.
| `PUT` | `/users/{username}/block` | Block a user. Removes follows both ways; you stop seeing each other's posts and comments |
| `DELETE` | `/users/{username}/block` | Unblock |
| `GET` | `/me/blocks` | Accounts you blocked |
| `POST` | `/reports` | Report: `{"targetType":"post\|comment\|account","targetUuid","reason","details"}`. Rate limited |
| `GET` | `/admin/reports` | Moderators: reports by `?status=open\|resolved\|dismissed` (default `open`), with a preview of the target |
| `POST` | `/admin/reports/{uuid}/resolve` | Moderators: `{"action":"dismiss\|removeContent\|suspendAccount","note"}` |
| `POST` | `/admin/accounts/{uuid}/unsuspend` | Admins: lift a suspension |

```bash
curl -s -X POST localhost:4100/media -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: image/jpeg' --data-binary @photo.jpg
```

## Storage

Files live in any S3-compatible service. Development uses SeaweedFS (compose).
Keys: `media/<uuid>/display.jpg`, `media/<uuid>/thumb.jpg`. Keys never change
content, so `/files` responses are cached for a year.

## Moderation

Roles are `user`, `moderator` and `admin`. Moderators review reports, remove
posts and comments, and suspend users. Only admins suspend moderators or lift
suspensions; nobody suspends an admin through the API.

A suspended account cannot log in, its sessions end at once, and its profile,
posts and comments are hidden from everyone.

Make the first admin directly in the database:

```bash
docker compose exec postgres psql -U plural -c "UPDATE account SET role = 'admin' WHERE username = 'alice'"
```

Roles are read from the database on every request, so the change applies
right away with no new login.

## Videos

Video uploads are stored as-is and queued in the `media_job` table. The
transcoder (`cmd/plural-api-transcoder`) claims jobs with
`FOR UPDATE SKIP LOCKED`, so several transcoders can run at once without
processing a video twice. For each job it:

1. Checks the video with `ffprobe` (readable, 1 s to 3 min, at most 7680 px).
2. Transcodes it to one H.264/AAC MP4, short side at most 720 px, with the
   index at the start (`faststart`) and all metadata (including location) removed.
3. Takes a JPEG poster frame, stores both files, marks the media `ready` and
   deletes the original.

Unreadable or out-of-range videos fail at once with a reason the uploader
sees. Other errors retry with a growing delay, up to `-max.attempts`. A worker
that crashes leaves its job `running`; another worker takes it over once the
lock is older than `-stale.after`. On SIGINT/SIGTERM the transcoder finishes
its current job before exiting.

```bash
make run-transcoder        # local, needs ffmpeg and ffprobe in PATH
make build-transcoder      # Docker image with ffmpeg included
```

Transcoder flags: the same `-db.connection` and `-s3.*` flags as the API, plus
`-poll.interval` (2s), `-job.timeout` (10m), `-stale.after` (15m, must exceed
`-job.timeout`), `-max.attempts` (3), `-ffmpeg.path`, `-ffprobe.path` and
`-tmp.dir`.

API flags for videos: `-media.max.video.bytes` (300 MB) and `-media.tmp.dir`
(where uploads are buffered before they go to storage).
