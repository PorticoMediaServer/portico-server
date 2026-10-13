#!/bin/sh
# The installer registers the system service; opening the app opens its console.
set -eu
open "${PORTICO_CONSOLE_URL:-http://127.0.0.1:32500/}"
