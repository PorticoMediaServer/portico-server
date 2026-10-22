#!/bin/sh
set -eu
uid=${PUID:-1000}
gid=${PGID:-1000}
mask=${UMASK:-0077}
for identity in "$uid" "$gid"; do
 case "$identity" in ''|*[!0-9]*) echo 'PUID and PGID must be numeric' >&2; exit 1;; esac
done
case "$mask" in [0-7][0-7][0-7]|0[0-7][0-7][0-7]) ;; *) echo 'UMASK must be three octal digits, optionally prefixed by 0' >&2; exit 1;; esac
umask "$mask"
export HOME=/config TMPDIR=/config/Temp TMP=/config/Temp TEMP=/config/Temp
# A directory existing in the image is not a persistent mount.
if ! awk '$5 == "/config" { found=1 } END { exit !found }' /proc/self/mountinfo; then
 echo 'Map /config to persistent storage before starting Portico' >&2; exit 1
fi
if [ "$(id -u)" = 0 ]; then
 chown "$uid:$gid" /config
 if awk '$5 == "/transcode" { found=1 } END { exit !found }' /proc/self/mountinfo; then chown "$uid:$gid" /transcode; fi
 # Preserve explicit Docker --group-add device groups while dropping group 0.
 groups="$gid"
 for group in $(id -G); do
  if [ "$group" != 0 ] && [ "$group" != "$gid" ]; then groups="$groups,$group"; fi
 done
 exec setpriv --reuid="$uid" --regid="$gid" --groups="$groups" "$@"
fi
exec "$@"
