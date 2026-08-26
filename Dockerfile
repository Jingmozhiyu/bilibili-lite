FROM golang:1.25.7-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.Name=bilibili-lite -X main.Version=${VERSION}" \
    -o /out/bilibili-lite ./cmd/bilibili-lite

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg \
    && command -v ffmpeg \
    && command -v ffprobe \
    && ffmpeg -hide_banner -version \
    && ffprobe -hide_banner -version \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 bilibili \
    && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin bilibili

WORKDIR /app
COPY --from=builder /out/bilibili-lite /app/bilibili-lite
COPY configs /app/configs
RUN mkdir -p /data/media && chown -R bilibili:bilibili /data/media

USER bilibili
EXPOSE 8000 9000
VOLUME ["/data/media"]
ENTRYPOINT ["/app/bilibili-lite"]
CMD ["-conf", "/app/configs"]
