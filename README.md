# Kratos Project Template

A project template for creating new Kratos services with HTTP and gRPC
transports, protobuf-first APIs, Wire dependency injection, OpenAPI generation,
and a small CRUD example.

Use this repository as a starting point for a new service. The included sample
resource is only reference code for API shape, layering, code generation, and
testing. Replace it with your own domain model when creating a real project.

## Create a New Project

1. Copy or generate a repository from this template.
2. Update the Go module path:

```bash
go mod edit -module github.com/your-org/your-service
```

3. Replace existing import paths that reference this template module.
4. Rename the command, service metadata, and sample API package to match your
   service.
5. Replace the sample CRUD resource with your own resource.
6. Regenerate code and verify the project:

```bash
make all
go test ./...
```

## What Is Included

- Kratos HTTP and gRPC server setup.
- Protobuf API definitions and generated Go code.
- OpenAPI generation.
- Wire-based dependency injection.
- Layered `service`, `biz`, and `data` packages.
- PostgreSQL repositories implemented with GORM.
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
application lifecycle of periodic stale-upload cleanup.

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

Start PostgreSQL first:

```bash
docker compose up -d postgres
```

MP4 uploads are converted into separate DASH audio and video segments by
FFmpeg. Install both `ffmpeg` and `ffprobe` before testing uploads. On macOS:

```bash
brew install ffmpeg
```

The service uses GORM to create the development schema and inserts three demo
users without creating sample videos. `videos.id` is the numeric auto-increment
primary key, and the API formats it as `BV1`, `BV2`, and so on. Related tables
store `video_id` numeric foreign keys; BVID strings are not persisted.

```bash
go run ./cmd/bilibili-lite -conf ./configs
```

Default local ports are configured in `configs/config.yaml`:

- HTTP: `0.0.0.0:8000`
- gRPC: `0.0.0.0:9000`

The seeded login is `demo` / `demo123456`. Login returns a two-hour Access JWT
and a 30-day Refresh JWT:

```bash
curl -X POST http://127.0.0.1:8000/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo123456"}'
```

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

JWTs are stateless in this local version and no Redis or session table is used.
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

The request allocates the numeric video row and BV identifier immediately. It
returns after DASH processing reaches `ready`; final metadata is submitted in a
separate publication step:

```bash
curl -X POST http://127.0.0.1:8000/api/v1/videos/BV1/publish \
  -H 'Authorization: Bearer <access-token>' \
  -H 'Content-Type: application/json' \
  -d '{"title":"My video","description":"Uploaded locally","tags":["local","DASH"]}'
```

The lifecycle is `processing -> ready -> published`, with terminal `failed`
and `deleted` states. Every inserted video consumes its auto-increment ID even
when processing fails or it is later deleted. DASH is the only supported
playback format; the original MP4 is retained only as a temporary transcoding
input. Published manifests live at
`/media/dash/<BVID>/manifest.mpd`. Incomplete jobs live under
`storage/.uploads` and are removed after 30 seconds without upload or transcode
activity.

Homepage and public submission feeds use paginated list endpoints:

```text
GET /api/v1/videos?page_size=20&page_token=...
GET /api/v1/users/{user_id}/videos?page_size=20&page_token=...
```

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

Embedded, versioned PostgreSQL migrations in `internal/data/migrations` are
applied once at service startup and recorded in `schema_migrations`. Add a new
numbered SQL file for every schema change; do not rewrite an applied migration.

## Docker

```bash
docker build -t <your-image-name> .
docker run --rm -p 8000:8000 -p 9000:9000 \
  -v </path/to/your/configs>:/data/conf \
  <your-image-name>
```
