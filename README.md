# bilibili-lite

A Kratos-based video service with PostgreSQL persistence, Meilisearch search,
Redis recommendations, bounded FFmpeg workers, adaptive DASH playback, and a
React frontend.

## What Is Included

- Kratos HTTP and gRPC server setup.
- Protobuf API definitions and generated Go code.
- OpenAPI generation.
- Wire-based dependency injection.
- Layered `service`, `biz`, and `data` packages.
- PostgreSQL repositories implemented with GORM.
- Meilisearch-backed published-video search with PostgreSQL degradation.
- Redis-backed time-decayed homepage ranking with PostgreSQL fallback.
- Bounded asynchronous FFmpeg workers and adaptive multi-bitrate DASH.
- Unit tests for the service layer.
- Server-streaming and bidirectional-streaming examples.

## Project Layout

```text
api/                  Protobuf APIs and generated bindings
cmd/                  Application entrypoints
configs/              Local configuration
internal/server/      HTTP and gRPC server construction
internal/service/     Transport-facing service methods
internal/biz/         Usecases, entities, errors, repository interfaces
internal/data/        PostgreSQL repositories and persistence models
internal/media/       Upload jobs, FFmpeg processing, local DASH storage
internal/middleware/  JWT implementation and request identity middleware
internal/worker/      Kratos-managed background cleanup jobs
third_party/          Protobuf dependencies
storage/              Video & photo resources
web/                  Web frontend
openapi.yaml          Generated OpenAPI document
```

## API Template Practices

The sample CRUD API demonstrates common conventions for Kratos projects:

- Resource-oriented methods: create, get, list, update, delete.
- HTTP annotations with `google.api.http`.
- Required fields with `google.api.field_behavior`.
- List requests with `page_size`, `page_token`, `filter`, and `order_by`.
- Pagination with `go.einride.tech/aip/pagination`.
- Partial updates with `google.protobuf.FieldMask` and `fieldmask.Update`.
- Streaming RPC definitions for one-way and bidirectional streams.

The data layer stores users, videos, playback metadata, and user video
interactions in PostgreSQL. Repository implementations keep GORM models inside
`internal/data/models.go` and expose domain objects to `internal/biz`. Media
processing is isolated in `internal/media`; `internal/worker` owns the
application lifecycle of bounded transcode, search-outbox, ranking, and stale-upload workers.

## Development Commands

Install generators:

```bash
make init
```

Regenerate API bindings and OpenAPI:

```bash
make api
```

Regenerate config protobufs:

```bash
make config
```

Run all generation steps, Wire, and module cleanup:

```bash
make all
```

Build:

```bash
make build
```

Test:

```bash
go test ./...
```

## Run Locally

The committed `.env.example` contains local development values. Real deployment
configuration belongs in the ignored `.env` file. Start only the dependencies
when running the Go process directly:

```bash
docker compose \
  -f docker-compose.yml \
  -f docker-compose.dev.yml \
  --env-file .env.example \
  up -d postgres meilisearch redis
```

MP4 uploads are converted into separate DASH audio and video segments by
FFmpeg. Install both `ffmpeg` and `ffprobe` before testing uploads. On macOS:

```bash
brew install ffmpeg
```

The service applies embedded SQL migrations and, when `BILI_SEED_ENABLED=true`,
inserts three demo users plus one administrator without creating sample videos.
`videos.id` is the numeric auto-increment
primary key, and the API formats it as `BV1`, `BV2`, and so on. Related tables
store `video_id` numeric foreign keys; BVID strings are not persisted.

Load `.env.example` into the shell before starting Kratos. The local media path
remains relative to `cmd/bilibili-lite`:

```bash
cd cmd/bilibili-lite
set -a
source ../../.env.example
set +a
go run . -conf ../../configs
```

Alternatively, run the complete backend stack in containers. The bind-mounted
`data/media` directory is intentionally not committed:

```bash
mkdir -p data/media
docker compose --env-file .env.example up -d --build
```

Default local ports are configured in `.env.example`:

- HTTP: `0.0.0.0:8000`
- gRPC: `0.0.0.0:9000`

For an Internet deployment, keep the ordinary API and large uploads on
separate hostnames. `VITE_API_ORIGIN` should point to the Cloudflare-proxied
API hostname, while `VITE_UPLOAD_ORIGIN` points to a DNS-only hostname that
bypasses proxy request-body limits. Both hostnames reverse-proxy to the same
Kratos HTTP service and must allow the frontend origin through CORS. The
production defaults are:

```text
VITE_API_ORIGIN=https://bili.madenroll.com
VITE_UPLOAD_ORIGIN=https://bili-upload.madenroll.com
```

The Compose Caddy overlay uses `BILI_PUBLIC_HOST` and `BILI_UPLOAD_HOST` for
the corresponding server names.

With `.env.example`, the seeded login is `demo` / `demo123456`. Login returns a two-hour Access JWT
and a 30-day Refresh JWT:

```bash
curl -X POST http://127.0.0.1:8000/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo123456"}'
```

The local administrator seed creates `admin` with `BILI_SEED_PASSWORD`. If an `admin`
username already exists, startup only repairs its role to `admin` and preserves
the existing password and profile. Open `/admin` to log in and enter the
moderation queue.

Refresh both tokens with the Refresh JWT:

```bash
curl -X POST http://127.0.0.1:8000/api/v1/auth/refresh \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"<refresh-token>"}'
```

Pass the Access JWT to logout:

```bash
curl -X POST http://127.0.0.1:8000/api/v1/auth/logout \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <access-token>' \
  -d '{}'
```

JWTs remain stateless: Redis is used for the homepage ranking, not sessions.
Logout discards both browser tokens; an Access JWT already issued remains valid
until its two-hour expiry.

Upload one MP4 after login. A custom cover is optional and must precede the
video part; when omitted, FFmpeg captures a random video frame:

```bash
curl -X POST http://127.0.0.1:8000/api/v1/videos/upload \
  -H 'Authorization: Bearer <access-token>' \
  -F 'cover=@/absolute/path/cover.png;type=image/png' \
  -F 'file=@/absolute/path/video.mp4;type=video/mp4'
```

The request allocates the numeric video row and BV identifier immediately and
returns `processing` after the source is safely queued. One bounded worker by
default produces an adaptive DASH manifest with a no-upscale 360p/480p/720p/1080p/
1440p/2160p ladder; 4K is emitted only when the source reaches 2160p. Poll
`GET /api/v1/videos/{bvid}/upload-status` until `ready`, then submit metadata
for administrator review:

```bash
curl -X POST http://127.0.0.1:8000/api/v1/videos/BV1/submit-review \
  -H 'Authorization: Bearer <access-token>' \
  -H 'Content-Type: application/json' \
  -d '{"title":"My video","description":"Uploaded locally","tags":["local","DASH"]}'
```

The source MP4 limit is 2 GiB. Active videos consume their original upload
size against a 10 GiB per-user quota; administrators are exempt. Failed and
deleted jobs release quota, remove temporary input, and abandoned worker claims
are recovered after restart. The moderation lifecycle is
`processing -> ready -> pending_review -> published/rejected`; upload failures
and owner deletion use `failed` and `deleted`. Every inserted video consumes
its auto-increment ID even when processing fails or it is later deleted. Only
`published` videos appear in public lists, search, history hydration, and
playback APIs. DASH is the only supported playback format; the original MP4 is
retained only as a temporary transcoding input. Published manifests live at
`/media/dash/<BVID>/manifest.mpd`. Incomplete jobs live under
`cmd/bilibili-lite/storage/.uploads` during local development and are removed
after 30 seconds without upload or transcode activity.

Homepage recommendations and public submission feeds use paginated endpoints:

```text
GET /api/v1/videos?page_size=20&page_token=...
GET /api/v1/videos/recommended?page_size=20&page_token=...
GET /api/v1/users/{user_id}/videos?page_size=20&page_token=...
GET /api/v1/search/videos?query=...&order=1&page_size=20&page_token=...
```

PostgreSQL is authoritative for video visibility and metadata. Meilisearch is
a replaceable projection behind the repository search interface. Startup and
queries fall back to PostgreSQL when it is unavailable. A PostgreSQL-triggered
outbox records index mutations atomically and the background consumer retries
them with exponential backoff. Search `order` values cover relevance, views,
publication time, danmaku, and favorites.

The recommendation endpoint reads a Redis sorted set rebuilt every 30 seconds
from logarithmic engagement signals plus a seven-day freshness decay. If Redis
is unavailable, the same score is calculated from PostgreSQL so the homepage
continues to load.

A logged-in view is counted only after completing a server-timed session at
least five seconds after it started. The same account/video pair can increment
at most once per hour and ten times per Asia/Shanghai calendar day.

Like operations require an Access JWT and are idempotent:

```bash
curl -X GET http://127.0.0.1:8000/api/v1/videos/BV1/like \
  -H 'Authorization: Bearer <access-token>'
curl -X POST http://127.0.0.1:8000/api/v1/videos/BV1/like \
  -H 'Authorization: Bearer <access-token>'
curl -X DELETE http://127.0.0.1:8000/api/v1/videos/BV1/like \
  -H 'Authorization: Bearer <access-token>'
```

Favorites use the same desired-state semantics as likes: repeating `POST` or
`DELETE` does not change the count twice. Coins use an irreversible cumulative
target of one or two coins, atomically debit `users.coin_balance`, and reject a
lower target. Shares are append-only events deduplicated by a client-generated
`request_id`. PostgreSQL is the source of truth for every interaction and its
counter. Redis ranking remains an infrastructure detail and does not leak into
handlers or domain models.

The browser applies like and favorite changes optimistically but rolls them
back when the server rejects or cannot receive the request. It deliberately
does not persist an offline outbox: reconnecting reads the authoritative state
instead of replaying stale toggles or irreversible coin/share actions.

Interaction history and public discussion endpoints are paginated:

```text
GET    /api/v1/users/me/video-likes
GET    /api/v1/users/me/video-favorites
GET    /api/v1/users/me/video-coins
GET    /api/v1/users/me/watch-history
POST   /api/v1/videos/{bvid}/danmakus
DELETE /api/v1/videos/{bvid}/danmakus/{danmaku_id}
GET    /api/v1/videos/{bvid}/comments
POST   /api/v1/videos/{bvid}/comments
DELETE /api/v1/videos/{bvid}/comments/{comment_id}
```

Administrator moderation endpoints are protected by both JWT identity and the
admin middleware:

```text
GET    /api/v1/admin/videos?status=pending_review|rejected
GET    /api/v1/admin/videos/{bvid}
GET    /api/v1/admin/videos/{bvid}/play
POST   /api/v1/admin/videos/{bvid}/approve
POST   /api/v1/admin/videos/{bvid}/reject
POST   /api/v1/admin/videos/{bvid}/take-down
DELETE /api/v1/admin/videos/{bvid}?reason=...
```

Embedded, versioned PostgreSQL migrations in `internal/data/migrations` are
applied once at service startup and recorded in `schema_migrations`. Add a new
numbered SQL file for every schema change; do not rewrite an applied migration.

## Deploy the backend on a VM

`make build` produces one native executable at `bin/bilibili-lite`. The binary
contains the Go runtime, Kratos, and Go module code, so a deployment host and the
Compose runtime do not need Go, Kratos, a compiler, or source dependencies. The
runtime image only adds CA certificates and the external `ffmpeg`/`ffprobe`
programs used by media workers.

This is the backend image. Build `web/dist` separately and serve it through a
static host or reverse proxy; Node.js is not needed by the Kratos container.

For a repository checked out at `/home/ubuntu/bili`, prepare configuration and a
host-owned media directory before the first start:

```bash
cd /home/ubuntu/bili
cp .env.example .env
mkdir -p /home/ubuntu/bili/data/media
id -u
id -g
```

Edit `.env` and set `BILI_RUNTIME_UID`/`BILI_RUNTIME_GID` to the two `id`
outputs, and set `BILI_MEDIA_HOST_DIR=/home/ubuntu/bili/data/media`. Do not use `~`
inside `.env`. Replace the PostgreSQL, Redis, Meilisearch, JWT, and seed
passwords before starting. Meilisearch requires at least 16 bytes for a
production master key, and `BILI_AUTH_SECRET` requires at least 32 bytes:

```bash
docker compose up -d --build
docker compose ps
```

The base Compose file publishes only the application HTTP/gRPC ports on the
configured bind host; PostgreSQL, Redis, and Meilisearch stay inside the Docker
network. `docker-compose.dev.yml` is only for exposing those dependencies on a
developer machine.

On a dedicated VM where ports 80 and 443 are free, set `BILI_PUBLIC_HOST` to
the backend DNS name and `BILI_FRONTEND_ORIGIN` to the exact Vercel origin, then
start the optional Caddy overlay:

```bash
docker compose \
  -f docker-compose.yml \
  -f docker-compose.caddy.yml \
  up -d --build
```

If the VM already has a reverse proxy, do not start the Caddy overlay. Add the
backend hostname to the existing proxy and forward it to the application
instead; only one process can bind the host's ports 80 and 443.

On an OCI block volume, mount the filesystem at a stable host path such as
`/mnt/bili-data`, create `/mnt/bili-data/media`, and set
`BILI_MEDIA_HOST_DIR=/mnt/bili-data/media`. The application creates these
subdirectories inside it:

```text
media/
├── .uploads/   # temporary source files and in-flight jobs
├── avatars/    # user avatars
└── dash/       # published manifests, covers, and media segments
```

PostgreSQL, Meilisearch, and Redis use Docker named volumes and require a
separate backup policy. The media bind mount is independent from container
replacement, so rebuilding or recreating `bili` does not remove video files.

`BILI_SEED_ENABLED=true` creates the demo users and administrator with
`BILI_SEED_PASSWORD`. Use a strong unique value for the first production boot;
after the accounts exist, it can be changed to `false`. Services bind to
`127.0.0.1` by default; place a TLS reverse proxy in front of HTTP instead of
publishing PostgreSQL, Redis, Meilisearch, or gRPC directly.
