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

FROM node:24.21.0-bookworm@sha256:3d27e5c11e5786e309ec3e03f93ae536eb36e6e5eb3714d5eb3300a36157add0 AS web
WORKDIR /src
COPY package.json package-lock.json ./
COPY web ./web
COPY packages/contracts ./packages/contracts
COPY packages/client-core ./packages/client-core
COPY packages/design ./packages/design
COPY packages/i18n ./packages/i18n
COPY third_party/hls.js ./third_party/hls.js
# The subtitle renderer and its fonts: the web build copies them, unchanged, into the application.
COPY third_party/libass-wasm ./third_party/libass-wasm
COPY third_party/subtitle-fonts ./third_party/subtitle-fonts
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

FROM golang:1.26.9-bookworm@sha256:d9c68c2c51161e12fd77e4c6320687c9cd86e1af1e3ad6e6cd63ff970641453c AS build
WORKDIR /src/server
COPY trust/hosted-root-public-key.pem /identity/root-public-key.pem
COPY scripts/inspect-hosted-root.go /identity/inspect-hosted-root.go
COPY apikit /src/apikit
COPY server/go.mod server/go.sum ./
RUN test "$(go env GOVERSION)" = "go$(sed -n 's/^go //p' go.mod)" && go mod download
COPY server ./
COPY --from=web /out/build.env /identity/build.env
ENV CGO_ENABLED=0
RUN ROOT_KEY=$(go run /identity/inspect-hosted-root.go /identity/root-public-key.pem) && test -n "$ROOT_KEY" && . /identity/build.env && go build -tags release -buildvcs=false -mod=readonly -trimpath -ldflags "-s -w -X portico.local/server/internal/hostedtrust.OfficialRoot=$ROOT_KEY -X portico.local/server/internal/buildinfo.Version=$VERSION -X portico.local/server/internal/buildinfo.BuildID=$BUILD_ID -X portico.local/server/internal/buildinfo.SourceDigest=$SOURCE_DIGEST -X portico.local/server/internal/buildinfo.BuiltAt=$BUILT_AT" -o /out/portico-server ./cmd/server

# The qualified FFmpeg bundle for the architecture this image is being built
# for. A multi-architecture build supplies one bundle per architecture, in
# third_party/ffmpeg/bundle/linux-amd64 and .../linux-arm64, and each image
# takes its own. A single-architecture build may instead place the native
# bundle directly in third_party/ffmpeg/bundle. Either way the final stage
# verifies what it was given before the image is finished.
FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587 AS ffmpeg
RUN apt-get update && apt-get install --yes --no-install-recommends python3 bash && rm -rf /var/lib/apt/lists/*
ARG TARGETOS
ARG TARGETARCH
COPY third_party/ffmpeg/bundle/ /bundle/
RUN if [ -d "/bundle/${TARGETOS}-${TARGETARCH}" ]; then mv "/bundle/${TARGETOS}-${TARGETARCH}" /ffmpeg; else mv /bundle /ffmpeg; fi

COPY scripts/verify-ffmpeg-manifest.py scripts/verify-ffmpeg-bundle.sh /tmp/
RUN bash /tmp/verify-ffmpeg-bundle.sh /ffmpeg full

# Fetch only the pinned architecture, verify the upstream signature and every payload byte.
FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587 AS rclone
RUN apt-get update && apt-get install --yes --no-install-recommends python3 curl gnupg ca-certificates && rm -rf /var/lib/apt/lists/*
ARG TARGETOS
ARG TARGETARCH
COPY scripts/rclone-bundle.py /src/scripts/rclone-bundle.py
COPY third_party/rclone/ /src/third_party/rclone/
RUN python3 /src/scripts/rclone-bundle.py prepare /rclone --target "${TARGETOS}-${TARGETARCH}" --execute-version

FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
ARG TARGETARCH
RUN apt-get update \
 && apt-get install --yes --no-install-recommends bubblewrap ca-certificates util-linux tini tzdata libva2 libva-drm2 mesa-va-drivers \
 && if [ "$TARGETARCH" = amd64 ]; then apt-get install --yes --no-install-recommends intel-media-va-driver i965-va-driver; fi \
 && rm -rf /var/lib/apt/lists/*

# Runtime writes are restricted to mounted /config and /transcode. Numeric
# identities work with Unraid and TrueNAS without editing /etc/passwd.
RUN mkdir -p /config /transcode
COPY packaging/docker/entrypoint.sh /usr/local/bin/portico-entrypoint
COPY LICENSE THIRD-PARTY-NOTICES.md /usr/share/doc/portico/
COPY server/internal/imagework/SCALE_LICENSE /usr/share/doc/portico/licenses/image-scaling.txt
COPY server/internal/thirdparty/sqlite/LICENSE /usr/share/doc/portico/licenses/sqlite-driver.txt
COPY server/internal/mediaexec/fonts/LICENSE /usr/share/doc/portico/licenses/decoder-fonts.txt
COPY third_party/hls.js/LICENSE /usr/share/doc/portico/licenses/hls-js.txt
COPY web/src/ui/fonts/OFL.txt /usr/share/doc/portico/licenses/manrope.txt
COPY packages/design/LICENSE-LUCIDE /usr/share/doc/portico/licenses/lucide.txt
COPY --from=rclone /rclone/LICENSE /usr/share/doc/portico/licenses/rclone.txt
COPY --from=rclone /rclone/DEPENDENCY-NOTICES.txt /usr/share/doc/portico/licenses/rclone-dependencies.txt
COPY --from=rclone /rclone/ /opt/portico/third_party/rclone/
COPY --from=build /out/portico-server /usr/local/bin/portico-server
COPY --from=web /out/web /opt/portico/web
COPY --from=web /out/release.json /opt/portico/release.json
# Supply the qualified bundle from the manual toolchain workflow (see the
# ffmpeg stage above for where it goes).
COPY --from=ffmpeg /ffmpeg/ /opt/portico/third_party/ffmpeg/

ENV PORTICO_WEB_DIR=/opt/portico/web \
    PORTICO_DATA_DIR=/config \
    PORTICO_CONTAINER=1 \
    HOME=/config \
    TMPDIR=/config/Temp \
    PORTICO_BIND=[::]:32500 \
    PORTICO_RCLONE=/opt/portico/third_party/rclone/rclone \
    PORTICO_FFMPEG=/opt/portico/third_party/ffmpeg/bin/ffmpeg \
    PORTICO_FFPROBE=/opt/portico/third_party/ffmpeg/bin/ffprobe
VOLUME ["/config"]
EXPOSE 32500
WORKDIR /config

# tini forwards signals and reaps orphans. It is the whole of point 1 above; do
# not replace it with the binary directly unless the runtime supplies its own
# init (`docker run --init`, or a Kubernetes pod with shareProcessNamespace and
# a reaping PID 1).
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/portico-entrypoint"]
CMD ["/usr/local/bin/portico-server"]

# The readiness route needs no authentication and no database, by design, so it
# answers while the server is still opening its state directory — which is what
# makes it usable as a container health check rather than only as a liveness one.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/portico-server", "--health-check"]
