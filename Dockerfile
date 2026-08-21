# syntax=docker/dockerfile:1
FROM node:26-bookworm-slim AS web-build
WORKDIR /src
COPY api/product-language ./api/product-language
COPY packages/portico-client-core/package*.json ./packages/portico-client-core/
RUN cd packages/portico-client-core && npm ci --no-audit --no-fund
COPY packages/portico-client-core ./packages/portico-client-core
RUN cd packages/portico-client-core && npm run build
COPY web/package*.json ./web/
RUN cd web && npm ci --no-audit --no-fund
COPY web ./web
RUN cd web && npm run build && npm run verify:bundle

FROM golang:1.26-bookworm AS server-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
ARG VERSION
ARG BUILD_NUMBER
ARG COMMIT
ARG BUILT_AT
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.buildNumber=${BUILD_NUMBER} -X main.channel=stable -X main.commit=${COMMIT} -X main.builtAt=${BUILT_AT} -X main.releaseSafetyClass=protected" \
    -o /out/portico-media-server ./cmd/porticod

FROM debian:bookworm-slim
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/* \
    && groupadd --system portico && useradd --system --gid portico --home-dir /var/lib/portico-media-server portico \
    && mkdir -p /app/bin /app/web /app/licenses /var/lib/portico-media-server \
    && chown -R portico:portico /var/lib/portico-media-server
COPY --from=server-build /out/portico-media-server /app/portico-media-server
COPY --from=web-build /src/web/dist /app/web
COPY .release/ffmpeg/${TARGETARCH}/ffmpeg /app/bin/ffmpeg
COPY .release/ffmpeg/${TARGETARCH}/ffprobe /app/bin/ffprobe
COPY .release/ffmpeg/${TARGETARCH}/LICENSES /app/licenses
COPY LICENSE THIRD-PARTY-NOTICES.md /app/licenses/
RUN chmod 0755 /app/portico-media-server /app/bin/ffmpeg /app/bin/ffprobe
USER portico
WORKDIR /app
ENV PORTICO_ADDR=0.0.0.0:32500 \
    PORTICO_APP_DATA=/var/lib/portico-media-server \
    PORTICO_WEB_DIST=/app/web \
    PORTICO_FFMPEG_PATH=/app/bin/ffmpeg \
    PORTICO_FFPROBE_PATH=/app/bin/ffprobe
EXPOSE 32500
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=5 CMD curl -fsS http://127.0.0.1:32500/api/readiness >/dev/null || exit 1
CMD ["/app/portico-media-server"]
