#!/bin/sh
# Run on an isolated drill host only. No deploy, DNS update, promotion or live DB.
set -eu
umask 077
: "${PORTICO_RESTORE_DIRECTORY:?Set a new empty absolute restore directory}"
: "${PORTICO_RESTORE_PORT:?Set an unused PostgreSQL drill port}"
: "${PORTICO_RESTORE_HOSTED_PORT:?Set an unused Hosted drill port}"
: "${PORTICO_RESTORE_HOSTED_BINARY:?Set the reviewed Hosted binary}"
: "${PORTICO_RESTORE_KEY_FILE:?Set restored ONLINE signing material}"
: "${PORTICO_RESTORE_SIGNIN_JSON_FILE:?Set a private synthetic-account sign-in request file}"
python3 - "$PORTICO_RESTORE_DIRECTORY" "$PORTICO_RESTORE_PORT" "$PORTICO_RESTORE_HOSTED_PORT" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1])
if not p.is_absolute() or p==pathlib.Path('/') or p.is_symlink() or (p.exists() and any(p.iterdir())):raise SystemExit('Restore target must be a new empty absolute directory')
ports=[int(x) for x in sys.argv[2:]]
if len(set(ports))!=2 or any(n<1024 or n>65535 or 32500<=n<=32510 or n in (19424,5173,5432) for n in ports):raise SystemExit('Unsafe drill ports')
p.mkdir(mode=0o700,parents=True,exist_ok=True)
PY
started=$(date +%s)
# A17: replay every archived WAL segment (the default recovery target), not
# just the base backup, so the drill measures the real recovery point.
pgbackrest --stanza=hosted --pg1-path="$PORTICO_RESTORE_DIRECTORY" restore
pg_ctl -D "$PORTICO_RESTORE_DIRECTORY" -l "$PORTICO_RESTORE_DIRECTORY/drill-postgres.log" -o "-h 127.0.0.1 -p $PORTICO_RESTORE_PORT -c archive_mode=off" -w start
hosted_pid=
cleanup() {
 if [ -n "$hosted_pid" ]; then kill "$hosted_pid" 2>/dev/null || true; wait "$hosted_pid" 2>/dev/null || true; fi
 pg_ctl -D "$PORTICO_RESTORE_DIRECTORY" -m fast -w stop
}
trap cleanup EXIT HUP INT TERM
printf 'postgres://127.0.0.1:%s/%s?sslmode=disable\n' "$PORTICO_RESTORE_PORT" "${PORTICO_RESTORE_DATABASE:-portico_hosted}" > "$PORTICO_RESTORE_DIRECTORY/database-url"
# The primary commits a recovery-point marker. Read it before Hosted starts: its
# age at the moment the restore began bounds the data this restore would lose.
marker=$(psql -h 127.0.0.1 -p "$PORTICO_RESTORE_PORT" -d "${PORTICO_RESTORE_DATABASE:-portico_hosted}" -Atc "SELECT floor(extract(epoch FROM checked_at)) FROM hosted_readiness WHERE singleton")
[ -n "$marker" ] || { echo 'No recovery-point marker in the restored copy' >&2; exit 1; }
rpo=$((started-${marker%.*}))
# The primary writes the marker every 30 s while other writes happen and not
# at all while idle (A95): 60 s RPO + 30 s marker interval = 90 s. A primary
# idle before the restore point also shows an old marker (no data was lost);
# re-run the drill after activity rather than accept it.
[ "$rpo" -le 90 ] || { echo "Recovery point marker is ${rpo}s old: over 90 s (60 s RPO + 30 s marker interval), or the primary was idle" >&2; exit 1; }
# --restore-drill starts no outbound or queue workers: no signed wakes to real
# servers, no ACME, mail, exports, alerts or pruning. Provider and mail configs
# are omitted too. The metrics listener takes any free loopback port.
"$PORTICO_RESTORE_HOSTED_BINARY" --restore-drill --listen "127.0.0.1:$PORTICO_RESTORE_HOSTED_PORT" --metrics-listen 127.0.0.1:0 --database-url-file "$PORTICO_RESTORE_DIRECTORY/database-url" --key-file "$PORTICO_RESTORE_KEY_FILE" --trusted-proxy-cidrs 127.0.0.1/32 --claim-audience "${PORTICO_RESTORE_CLAIM_AUDIENCE:-https://web.getportico.tv}" > "$PORTICO_RESTORE_DIRECTORY/hosted.log" 2>&1 &
hosted_pid=$!
attempt=0
until curl --fail --silent --output /dev/null "http://127.0.0.1:$PORTICO_RESTORE_HOSTED_PORT/ready"; do
 attempt=$((attempt+1)); [ "$attempt" -lt 60 ] || exit 1; sleep 1
done
# A native sign-in, as an app makes it: a private continuation binding and an
# idempotent request id. Only a session counts; an MFA challenge (202) means the
# synthetic account needs a factor and the drill is not proven.
python3 - "$PORTICO_RESTORE_SIGNIN_JSON_FILE" "$PORTICO_RESTORE_HOSTED_PORT" <<'PY'
import base64,json,os,sys,urllib.request
body=json.load(open(sys.argv[1]))
body.setdefault('sessionMode','native')
body.setdefault('requestId',base64.urlsafe_b64encode(os.urandom(24)).decode().rstrip('='))
binding=base64.urlsafe_b64encode(os.urandom(32)).decode().rstrip('=')
request=urllib.request.Request('http://127.0.0.1:%s/v1/sessions'%sys.argv[2],data=json.dumps(body).encode(),method='POST',headers={'Content-Type':'application/json','X-Portico-Continuation':binding})
with urllib.request.urlopen(request,timeout=30) as response:
    status,answer=response.status,json.load(response)
if status!=200 or not isinstance(answer,dict) or not answer.get('accessToken'):
    raise SystemExit('Synthetic sign-in did not return a session (status %s); use a synthetic account without MFA'%status)
PY
elapsed=$(($(date +%s)-started))
[ "$elapsed" -le 7200 ] || { echo 'Restore exceeded two-hour RTO' >&2; exit 1; }
printf '{"ready":true,"syntheticSignIn":true,"restoreSeconds":%s,"recoveryPointSeconds":%s}\n' "$elapsed" "$rpo"
