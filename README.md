# plural-api

The backend for Plural, a decentralized, open-source social network. People
post photos and short videos, follow each other, like and comment. Moderators
review reports and remove content.

The app that uses this API is [`plural-client`](https://github.com/plural-social/plural-client).

## Contents

- [Quick start](#quick-start)
- [How it fits together](#how-it-fits-together)
- [Project layout](#project-layout)
- [Development commands](#development-commands)
- [Configuration](#configuration)
- [API reference](#api-reference)
- [Media and video processing](#media-and-video-processing)
- [Moderation](#moderation)
- [Testing](#testing)
- [Deployment](#deployment)

## Quick start

You need Go 1.27, Docker, and ffmpeg (`brew install ffmpeg` on macOS).

```bash
make run              # terminal 1: starts Postgres and SeaweedFS in Docker, then the API on :4100
make run-transcoder   # terminal 2: converts uploaded videos
make seed             # terminal 3 (optional): adds test users, posts and follows
```

`make seed` creates the users `alice`, `bob` and `carol`. They all use the
password `password123`. It needs `curl`, `jq` and `ffmpeg`.

Check that it works:

```bash
curl -s localhost:4100/health
# {"status":"ok"}
```

Video posts stay `processing` until the transcoder runs. Keep
`make run-transcoder` open whenever you upload videos.

## How it fits together

```
plural-client (Android, iOS, web)
      │  HTTPS + JSON, Authorization: Bearer <token>
      ▼
plural-api  ───────────────────►  Postgres
      │  stores files                  ▲
      ▼                                │ claims video jobs
S3-compatible storage  ◄──────  plural-api-transcoder (ffmpeg)
```

There are two programs:

| Program | What it does |
|---|---|
| `plural-api` | The HTTP API. It also processes images during the upload request |
| `plural-api-transcoder` | A worker. It takes video jobs from Postgres and converts the videos with ffmpeg |

Both read the same Postgres database and the same storage bucket. The API
applies database migrations on startup.

The API is one program today. Each package in `internal/` is one domain, so a
domain can move into its own service later.

## Project layout

| Path | What it holds |
|---|---|
| `cmd/plural-api/` | The API: flags, wiring, routes and HTTP handlers (`<domain>Handlers.go`) |
| `cmd/plural-api-transcoder/` | The video worker: poll loop and job processing |
| `internal/account/` | Accounts, passwords, sessions, profiles |
| `internal/media/` | Image processing, video probing and transcoding |
| `internal/post/` | Posts |
| `internal/social/` | Follows, likes, comments |
| `internal/moderation/` | Blocks, reports, moderator actions |
| `internal/pagination/` | Cursor encoding for paged lists |
| `internal/blob/` | The S3 storage client |
| `internal/postgres/` | All SQL, one file per domain. Migrations are in `migrations/` |
| `sql/create.sql` | The full current schema. Keep it in sync with the migrations |
| `scripts/seed.sh` | Test data for development |

## Development commands

| Command | What it does |
|---|---|
| `make run` | Start Postgres and SeaweedFS in Docker, then run the API |
| `make run-transcoder` | Run the video transcoder |
| `make seed` | Add test users, posts, videos, follows, likes and a comment |
| `make test` | Run all tests with the race detector |
| `make generate` | Regenerate the test mocks |
| `make fmt` | Format the code with gofumpt |
| `make build` | Build the API Docker image |
| `make build-transcoder` | Build the transcoder Docker image (includes ffmpeg) |

Run `make generate`, `make fmt`, `go vet ./...` and `make test` before every commit.

## Configuration

Every setting is a command-line flag. Each flag also works as an environment
variable: `-db.connection` becomes `DB_CONNECTION`, and `-s3.access.key`
becomes `S3_ACCESS_KEY`.

### API flags

| Flag | Default | What it does |
|---|---|---|
| `-port` | `4100` | Port to listen on |
| `-db.connection` | local Docker Postgres | Postgres connection string |
| `-cors.origins` | `http://localhost:8081` | Browser origins allowed to call the API, comma-separated. `*` allows any |
| `-public.url` | empty | Base URL for file links. When empty, links use the host the client called |
| `-trust.proxy.headers` | `false` | Use `X-Forwarded-For` and `X-Forwarded-Proto`. Turn on only behind a proxy that sets them |
| `-instance.name` | `Plural` | Name shown in the app |
| `-instance.domain` | `localhost` | Public domain of this instance |
| `-instance.terms.url` | empty | Terms page. When set, sign-up requires agreeing to it |
| `-s3.endpoint` | `localhost:8333` | Storage address, as `host:port` with no scheme |
| `-s3.access.key`, `-s3.secret.key` | `plural` | Storage credentials |
| `-s3.bucket` | `plural-media` | Bucket name. The API creates it if it is missing |
| `-s3.region` | `us-east-1` | Storage region |
| `-s3.use.ssl` | `false` | Use HTTPS to reach storage |
| `-media.max.video.bytes` | 300 MB | Largest video upload |
| `-media.tmp.dir` | system temp dir | Where video uploads wait before they go to storage |
| `-startup.timeout`, `-shutdown.timeout` | `30s` | Time limits for starting and stopping |

### Transcoder flags

The transcoder uses the same `-db.connection` and `-s3.*` flags as the API. It
also has these:

| Flag | Default | What it does |
|---|---|---|
| `-poll.interval` | `2s` | Wait between checks when no job is ready |
| `-job.timeout` | `10m` | Longest time one video may take |
| `-stale.after` | `15m` | Take over a job whose worker stopped responding. Must be longer than `-job.timeout` |
| `-max.attempts` | `3` | Tries before a video is marked failed |
| `-ffmpeg.path`, `-ffprobe.path` | `ffmpeg`, `ffprobe` | Paths to the tools |
| `-tmp.dir` | system temp dir | Where work files go |

## API reference

All requests and responses are JSON, except file uploads (`POST /media`) and
file downloads (`/files/...`).

**Authentication.** Log in with `POST /sessions` to get a token. Send it on
later requests as `Authorization: Bearer <token>`. Tokens last 90 days.

**Errors** look like this:

```json
{"error": "human readable message", "type": "machineReadableType", "details": {}}
```

`type` and `details` are optional. Clients should branch on `type`, not on the
message.

**Lists** return one page at a time:

```json
{"items": [], "nextCursor": "opaque-string"}
```

Pass `nextCursor` back as `?cursor=` to get the next page. `nextCursor` is
missing on the last page. `?limit=` sets the page size, from 1 to 30 (default 10).

**Rate limits.** Login, sign-up, uploads, reports and account deletion are
limited per IP address. Too many requests return `429` with type `rateLimited`.

In the tables below, **Auth** means:

- *required*: needs a token.
- *optional*: works without a token, but a token changes the result (for
  example `likedByMe`).
- *none*: no token needed.

### Instance and health

| Method | Path | Auth | What it does |
|---|---|---|---|
| `GET` | `/health` | none | Checks the database. Returns `{"status":"ok"}` |
| `GET` | `/instance` | none | Instance name, domain, version and terms URL |

### Accounts and sessions

| Method | Path | Auth | What it does |
|---|---|---|---|
| `POST` | `/accounts` | none | Sign up: `{"username","email","password"}` |
| `POST` | `/sessions` | none | Log in: `{"login","password"}`. `login` is a username or email. Returns a token and your account |
| `DELETE` | `/sessions/current` | required | Log out |
| `GET` | `/me` | required | Your account |
| `PATCH` | `/me` | required | Change `{"displayName","bio"}`. Fields you leave out stay the same |
| `PUT` | `/me/avatar` | required | Set your photo: `{"mediaUuid"}`. Use an image you uploaded that no post uses |
| `DELETE` | `/me/avatar` | required | Remove your photo |
| `DELETE` | `/me` | required | Delete your account and everything in it, for good: `{"password"}` |

Example:

```bash
TOKEN=$(curl -s -X POST localhost:4100/sessions \
  -d '{"login":"alice","password":"password123"}' | jq -r .token)
curl -s localhost:4100/me -H "Authorization: Bearer $TOKEN"
```

### Users and follows

| Method | Path | Auth | What it does |
|---|---|---|---|
| `GET` | `/users/{username}` | optional | A profile with post and follower counts |
| `GET` | `/users/{username}/posts` | optional | That user's posts, newest first |
| `PUT` | `/users/{username}/follow` | required | Follow |
| `DELETE` | `/users/{username}/follow` | required | Unfollow |

### Media

| Method | Path | Auth | What it does |
|---|---|---|---|
| `POST` | `/media` | required | Upload a photo or video as raw bytes. Set `Content-Type` to the file type |
| `GET` | `/media/{uuid}` | required | Your own upload, including its processing status |
| `GET` | `/files/{key...}` | none | A stored file. Supports range requests, so videos can seek |

Upload example:

```bash
curl -s -X POST localhost:4100/media -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: image/jpeg' --data-binary @photo.jpg
```

See [Media and video processing](#media-and-video-processing) for formats and limits.

### Posts, feeds, likes and comments

| Method | Path | Auth | What it does |
|---|---|---|---|
| `POST` | `/posts` | required | Create a post: `{"mediaUuid","caption"}` |
| `GET` | `/posts/{uuid}` | optional | One post |
| `DELETE` | `/posts/{uuid}` | required | Delete your post and its files |
| `GET` | `/feed/following` | required | Posts by you and the people you follow |
| `GET` | `/feed/explore` | optional | All posts on this instance |
| `PUT` | `/posts/{uuid}/like` | required | Like |
| `DELETE` | `/posts/{uuid}/like` | required | Remove your like |
| `GET` | `/posts/{uuid}/comments` | optional | Comments, newest first |
| `POST` | `/posts/{uuid}/comments` | required | Comment: `{"body"}`, 1 to 500 characters |
| `DELETE` | `/comments/{uuid}` | required | Delete a comment. Allowed for its author, the post's author and moderators |

Every post includes `likeCount`, `commentCount` and `likedByMe`. Feeds show
only posts whose media is ready.

### Blocks and reports

| Method | Path | Auth | What it does |
|---|---|---|---|
| `PUT` | `/users/{username}/block` | required | Block a user |
| `DELETE` | `/users/{username}/block` | required | Unblock |
| `GET` | `/me/blocks` | required | Accounts you blocked |
| `POST` | `/reports` | required | Report: `{"targetType","targetUuid","reason","details"}` |

`targetType` is `post`, `comment` or `account`. `reason` is `spam`,
`harassment`, `nudity`, `violence`, `illegal` or `other`.

### Moderation (moderators and admins only)

| Method | Path | Who | What it does |
|---|---|---|---|
| `GET` | `/admin/reports` | moderator | Reports, with a preview of what was reported. `?status=open\|resolved\|dismissed` (default `open`) |
| `POST` | `/admin/reports/{uuid}/resolve` | moderator | Act on a report: `{"action","note"}` |
| `POST` | `/admin/accounts/{uuid}/unsuspend` | admin | Lift a suspension |

`action` is one of:

| Action | Effect |
|---|---|
| `dismiss` | Close this report. The content stays |
| `removeContent` | Hide the post or comment and close every open report on it |
| `suspendAccount` | Suspend the author and close every open report on the target |

## Media and video processing

### Images

| | |
|---|---|
| Formats | JPEG, PNG, WebP |
| Largest file | 20 MB |
| What is stored | A display copy (longest side up to 2048 px) and a thumbnail (up to 480 px), both JPEG |

The API re-encodes every image during the upload. This removes all metadata,
including GPS location. The original file is never stored.

### Videos

| | |
|---|---|
| Formats | MP4, MOV, WebM |
| Largest file | 300 MB |
| Length | 1 second to 3 minutes |
| What is stored | One H.264/AAC MP4 (short side up to 720 px) and a JPEG poster frame |

A video upload returns at once with status `processing`. The transcoder then:

1. Checks the file with `ffprobe`. It must be a readable video of the allowed
   length, at most 7680 px on a side.
2. Converts it with `ffmpeg`. The output plays in every browser and on every
   phone. It starts playing before the download finishes. All metadata,
   including location, is removed.
3. Saves the video and a poster frame, marks the media `ready`, and deletes the
   original upload.

When a video is unreadable or out of range, it becomes `failed` with a reason
the uploader can read, such as `video must be 3 minutes or shorter`. Other
errors are retried with a growing delay, up to `-max.attempts` times.

### How the job queue works

Video jobs are rows in the `media_job` table. There is no separate queue
service.

- A transcoder takes a job with `FOR UPDATE SKIP LOCKED`. You can run several
  transcoders at once, and each video is still processed once.
- If a transcoder crashes, its job stays `running`. Another transcoder takes it
  over once the job is older than `-stale.after`.
- On Ctrl+C or `SIGTERM`, the transcoder finishes its current video before it
  exits.

### Storage

Files go to any S3-compatible service. Development uses SeaweedFS from
`compose.yaml`. Each file is stored under `media/<uuid>/`. Files never change
after they are written, so `/files/...` responses can be cached for a year.
Original video uploads are never served.

## Moderation

There are three roles:

| Role | Can do |
|---|---|
| `user` | Post, block, report |
| `moderator` | Also review reports, remove posts and comments, suspend users |
| `admin` | Also suspend moderators and lift suspensions |

Nobody can suspend an admin through the API.

A suspended account cannot log in. Its sessions end at once. Its profile,
posts and comments are hidden from everyone.

When two users block each other, in either direction, neither sees the other's
posts or comments, and existing follows between them are removed.

To make the first admin, set the role in the database:

```bash
docker compose exec postgres psql -U plural -c "UPDATE account SET role = 'admin' WHERE username = 'alice'"
```

The API reads roles on every request, so the change works at once. The user
does not need to log in again.

## Testing

```bash
make test
```

Some tests need real services. Without them, those tests are skipped.

| Tests | Need | How to enable |
|---|---|---|
| Database (`internal/postgres`) | Postgres | `TEST_DATABASE_URL='postgres://plural:plural@localhost:5432/plural?sslmode=disable'` |
| Storage (`internal/blob`) | SeaweedFS or another S3 service | `TEST_S3_ENDPOINT=localhost:8333` |
| Video (`internal/media`) | `ffmpeg` and `ffprobe` | Have them in your `PATH` |

To run everything:

```bash
docker compose up -d postgres seaweedfs
TEST_DATABASE_URL='postgres://plural:plural@localhost:5432/plural?sslmode=disable' \
TEST_S3_ENDPOINT=localhost:8333 make test
```

The database tests write to the database you point them at. Use a test or
development database, not production.

## Deployment

The defaults are for local development. Before you run this in production:

- Serve the API over HTTPS behind a proxy, and set `-trust.proxy.headers`.
- Set `-public.url`, `-instance.domain` and `-cors.origins` to your real addresses.
- Use a real storage bucket and real credentials. The development credentials are `plural` / `plural`.
- Run at least one transcoder (`make build-transcoder` builds an image with
  ffmpeg). Without it, videos never leave `processing`.
- Set `-instance.terms.url` if you have terms of use.
- Make the first admin (see [Moderation](#moderation)).
