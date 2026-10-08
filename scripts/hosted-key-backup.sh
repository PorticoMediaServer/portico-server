#!/bin/sh
# Encrypted online-key bundle. Runs as the Hosted service user, which owns the
# 0600 signing, certificate and sealing files (portico-hosted-key-backup.service).
set -eu
umask 077
: "${PORTICO_ONLINE_KEY_DIRECTORY:?Set the online signing/sealing configuration directory}"
: "${PORTICO_BACKUP_AGE_RECIPIENT_FILE:?Set an age PUBLIC recipient file}"
: "${PORTICO_KEY_BACKUP_TARGET:?Set an off-provider rclone directory}"
: "${PORTICO_BACKUP_SCRATCH:?Set a private scratch directory}"
[ -d "$PORTICO_ONLINE_KEY_DIRECTORY" ] && [ -d "$PORTICO_BACKUP_SCRATCH" ]
# Only a manifest of online files is included. Never recursively collect roots,
# user home directories or private-root material.
: "${PORTICO_ONLINE_KEY_MANIFEST:?Set relative online file manifest}"
python3 - "$PORTICO_ONLINE_KEY_MANIFEST" <<'PY'
import pathlib,sys
rows=pathlib.Path(sys.argv[1]).read_text().splitlines()
if not rows or any(not x or pathlib.PurePosixPath(x).is_absolute() or '..' in pathlib.PurePosixPath(x).parts or x.startswith('-') or 'root-private' in x for x in rows):
 raise SystemExit('Invalid online backup manifest')
PY
bundle=$(mktemp "$PORTICO_BACKUP_SCRATCH/online-keys.XXXXXXXX.age")
plain=$(mktemp "$PORTICO_BACKUP_SCRATCH/online-keys.XXXXXXXX.tar")
trap 'rm -f "${plain:?}" "${bundle:?}"' EXIT HUP INT TERM
# GNU tar --no-recursion makes directories in the manifest harmless.
tar --no-recursion -C "$PORTICO_ONLINE_KEY_DIRECTORY" -cf "$plain" -T "$PORTICO_ONLINE_KEY_MANIFEST"
age -R "$PORTICO_BACKUP_AGE_RECIPIENT_FILE" -o "$bundle" "$plain"
rm -f "${plain:?}"
rclone copyto "$bundle" "$PORTICO_KEY_BACKUP_TARGET/online-keys-$(date -u +%Y%m%dT%H%M%SZ).tar.age"
