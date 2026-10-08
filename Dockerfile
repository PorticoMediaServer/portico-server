# Portico server, containerised.
#
# Two things about this image are load-bearing and easy to get wrong, so they are
# both written down here rather than left to whoever deploys it.
#
# 1. PID 1. The server handles SIGTERM correctly — signal.NotifyContext in
#    cmd/server/main.go — so running it as PID 1 shuts down cleanly. What it
#    cannot do as PID 1 is reap. The media pipeline runs ffmpeg and ffprobe as
#    children of the server and, on Unix, in their own process groups so a
#    cancelled request kills the whole group. If one of those children forks
#    again and the grandchild outlives its parent, the grandchild is reparented
#    to PID 1. A normal init reaps it; a Go program does not, and the process
#    table fills with zombies over days of use. So the image declares an init
#    process as its entrypoint, and a deployment that overrides the entrypoint
#    must pass `--init` (or run under a supervisor that reaps) instead.
#
# 2. ffmpeg and ffprobe. The server shells out to them for conversion, analysis
#    and subtitle rendering. They are installed here and named explicitly, rather
#    than left to PATH, so that a missing one is a build failure instead of a
#    runtime one that only appears when somebody presses play.
#
# 3. The decoder sandbox. bubblewrap is installed so FFmpeg and ffprobe run
#    sandboxed wherever the container may create user namespaces (Docker's
#    default seccomp and AppArmor profiles usually refuse them). Where it can't,
#    the server still plays, records and converts, with the baseline
#    restrictions, and Server › Logs & diagnostics says so and why
#    (server/internal/mediaexec). PORTICO_DECODER_SANDBOX=required
#    makes media jobs fail instead; =off skips the sandbox.

FROM node:24-bookworm AS web
WORKDIR /src
COPY package.json package-lock.json ./
COPY web ./web
COPY packages/client-core ./packages/client-core
COPY packages/design ./packages/design
COPY packages/i18n ./packages/i18n
COPY third_party/hls.js ./third_party/hls.js
COPY server ./server
COPY apikit ./apikit
COPY scripts/container-release.mjs ./scripts/container-release.mjs
COPY Dockerfile ./Dockerfile
ARG PORTICO_VERSION
ARG PORTICO_BUILT_AT
RUN PORTICO_VERSION="$PORTICO_VERSION" PORTICO_BUILT_AT="$PORTICO_BUILT_AT" node scripts/container-release.mjs
RUN npm ci --ignore-scripts --no-audit --no-fund
WORKDIR /src/web
# This is the server-hosted application; do not enable Hosted-only Web mode.
RUN node --input-type=module -e "import {build} from 'vite'; await build({build:{outDir:'/out/web',sourcemap:false}})"
RUN cp /out/portico-build.json /out/web/portico-build.json

FROM golang:1.26-bookworm AS build
WORKDIR /src/server
COPY trust/hosted-root-public-key.pem /identity/root-public-key.pem
COPY scripts/inspect-hosted-root.go /identity/inspect-hosted-root.go
COPY apikit /src/apikit
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server ./
COPY --from=web /out/build.env /identity/build.env
ENV CGO_ENABLED=0
RUN ROOT_KEY=$(go run /identity/inspect-hosted-root.go /identity/root-public-key.pem) && test -n "$ROOT_KEY" && . /identity/build.env && go build -tags release -buildvcs=false -mod=readonly -trimpath -ldflags "-s -w -X portico.local/server/internal/hostedtrust.OfficialRoot=$ROOT_KEY -X portico.local/server/internal/buildinfo.Version=$VERSION -X portico.local/server/internal/buildinfo.BuildID=$BUILD_ID -X portico.local/server/internal/buildinfo.SourceDigest=$SOURCE_DIGEST -X portico.local/server/internal/buildinfo.BuiltAt=$BUILT_AT" -o /out/portico-server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install --yes --no-install-recommends bubblewrap ca-certificates python3 tini \
 && rm -rf /var/lib/apt/lists/*

# An unprivileged user, and a state directory it owns. Everything the server
# writes — the database, converted media, artwork, logs — lives under this one
# path, so it is the only volume a deployment has to think about.
RUN useradd --system --create-home --home-dir /var/lib/portico --shell /usr/sbin/nologin portico
COPY --from=build /out/portico-server /usr/local/bin/portico-server
COPY --from=web /out/web /opt/portico/web
COPY --from=web /out/release.json /opt/portico/release.json
# Supply the native qualified bundle from the manual toolchain workflow.
COPY third_party/ffmpeg/bundle/ /opt/portico/third_party/ffmpeg/
COPY scripts/verify-ffmpeg-manifest.py scripts/verify-ffmpeg-bundle.sh /tmp/
RUN bash /tmp/verify-ffmpeg-bundle.sh /opt/portico/third_party/ffmpeg full

ENV PORTICO_WEB_DIR=/opt/portico/web \
    PORTICO_STATE_DIR=/var/lib/portico \
    PORTICO_BIND=0.0.0.0:32500 \
    PORTICO_FFMPEG=/opt/portico/third_party/ffmpeg/bin/ffmpeg \
    PORTICO_FFPROBE=/opt/portico/third_party/ffmpeg/bin/ffprobe
VOLUME ["/var/lib/portico"]
EXPOSE 32500
USER portico
WORKDIR /var/lib/portico

# tini forwards signals and reaps orphans. It is the whole of point 1 above; do
# not replace it with the binary directly unless the runtime supplies its own
# init (`docker run --init`, or a Kubernetes pod with shareProcessNamespace and
# a reaping PID 1).
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["/usr/local/bin/portico-server"]

# The readiness route needs no authentication and no database, by design, so it
# answers while the server is still opening its state directory — which is what
# makes it usable as a container health check rather than only as a liveness one.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/portico-server", "--health-check"]
