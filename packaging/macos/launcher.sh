#!/bin/sh
set -eu

contents_dir="$(cd -P -- "$(dirname -- "$0")/.." && pwd)"
resources_dir="$contents_dir/Resources"
app_data_dir="$HOME/Library/Application Support/Portico Media Server"

mkdir -p "$app_data_dir"
export PORTICO_APP_DATA="$app_data_dir"
export PORTICO_WEB_DIST="$resources_dir/web"
export PORTICO_FFMPEG_PATH="$resources_dir/bin/ffmpeg"
export PORTICO_FFPROBE_PATH="$resources_dir/bin/ffprobe"

exec "$resources_dir/bin/portico-media-server"
