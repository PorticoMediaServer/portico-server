#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Disposable runner-only source/helper check. No host packages or image builds.
set -euo pipefail
repo="$(cd "$(dirname "$0")/../../.." && pwd)"
: "${TMPDIR:?remote job TMPDIR is required}"
job_tmp="$(realpath "$TMPDIR")"
mkdir -p "$job_tmp/container-tmp"
container=""
cleanup() {
  if [[ -n "$container" ]]; then
    docker rm -f "$container" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT
trap 'exit 143' TERM HUP INT
# Resolve the real immutable public dependency image for this host proof and
# record it. This is not the canonical FFmpeg compiler/toolchain contract.
image="ubuntu:24.04"
docker pull "$image"
image_id="$(docker image inspect --format '{{.Id}}' "$image")"
docker image inspect --format '{{json .RepoDigests}}' "$image_id"
container="$(docker create --label portico=video-server-r29 --label portico.purpose=disc-host-source-check \
  --cpus=1 --memory=1g --pids-limit=256 \
  --mount "type=bind,source=$repo,target=/source,readonly" \
  --mount "type=bind,source=$job_tmp/container-tmp,target=/job" \
  --env TMPDIR=/job --env DEBIAN_FRONTEND=noninteractive \
  "$image_id" bash -c '
    set -euo pipefail
    apt-get update
    apt-get install --no-install-recommends -y build-essential ca-certificates git meson ninja-build pkg-config python3
    dpkg-query -W gcc meson ninja-build pkg-config git python3
    exec python3 /source/third_party/portico-disc/tests/qualify-pinned-host.py --work /job/disc-host --static
  ')"
docker start -a "$container"
# No image was built. Only this labelled container and its per-job bind scratch
# belong to the check; public dependency-image cache is not pruned here.
