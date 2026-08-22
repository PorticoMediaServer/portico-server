#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 BTBN_MAKEIMAGE_SCRIPT" >&2
  exit 2
fi

recipe="$1"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

awk '
  $0 == "./generate.sh \"$TARGET\" \"$VARIANT\" \"${ADDINS[@]}\"" {
    print
    print ""
    print "# Loading the final dependency image into Docker can fail for targets with"
    print "# hundreds of layers. Portico exports that image directly to its private"
    print "# registry cache, then pulls it back for the FFmpeg build. The local cache"
    print "# exporter is also disabled in this mode because the registry image is the"
    print "# durable cache and BuildKit can deadlock while exporting both outputs."
    print "FINAL_CACHE_ARGS=("
    print "    \"--cache-from=type=local,src=.cache/${IMAGE/:/_}\""
    print "    \"--cache-to=type=local,mode=max,dest=.cache/${IMAGE/:/_}\""
    print ")"
    print "FINAL_IMAGE_OUTPUT=(--load --tag \"$IMAGE\")"
    print "if [[ -n \"${PORTICO_CACHE_IMAGE:-}\" ]]; then"
    print "    FINAL_CACHE_ARGS=()"
    print "    FINAL_IMAGE_OUTPUT=(--push --tag \"$PORTICO_CACHE_IMAGE\")"
    print "    if [[ -n \"${PORTICO_CACHE_METADATA:-}\" ]]; then"
    print "        FINAL_IMAGE_OUTPUT+=(--metadata-file \"$PORTICO_CACHE_METADATA\")"
    print "    fi"
    print "fi"
    inserted++
    next
  }
  $0 == "    --cache-from=type=local,src=.cache/\"${IMAGE/:/_}\" \\" && inserted == 1 {
    print "    \"${FINAL_CACHE_ARGS[@]}\" \\"
    cache_from_replaced++
    next
  }
  $0 == "    --cache-to=type=local,mode=max,dest=.cache/\"${IMAGE/:/_}\" \\" && inserted == 1 {
    cache_to_removed++
    next
  }
  $0 == "    --load --tag \"$IMAGE\" ." && inserted == 1 {
    print "    \"${FINAL_IMAGE_OUTPUT[@]}\" ."
    replaced++
    next
  }
  { print }
  END {
    if (inserted != 1 || cache_from_replaced != 1 || cache_to_removed != 1 || replaced != 1) exit 1
  }
' "$recipe" > "$tmp"

test "$(grep -c 'Portico exports that image directly' "$tmp")" -eq 1
chmod +x "$tmp"
mv "$tmp" "$recipe"
trap - EXIT
