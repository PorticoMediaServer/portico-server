#!/usr/bin/env bash
set -euo pipefail

# Run from the server module inside an admitted, disposable Linux runner job.
# Dependencies stay in owned scratch or the job-labelled Docker fixture.
test "$(uname -s)" = Linux
test "$(uname -m)" = x86_64
test -f ../third_party/rclone/bundle.lock.json
test -n "${TMPDIR:-}"
test "$(realpath "$TMPDIR")" = "$TMPDIR"
case "$TMPDIR" in /tmp|/var/tmp|/|"$HOME") exit 2 ;; esac
for required in python3; do
    if ! command -v "$required" >/dev/null; then
        echo "Native fixture prerequisite is missing: $required" >&2
        exit 2
    fi
done

fixture_scratch=$(mktemp -d "$TMPDIR/remote-native-XXXXXXXX")
export PORTICO_SAMBA_TEST_JOB="remote-sources-$(basename "$fixture_scratch" | tr '[:upper:]' '[:lower:]')"
docker_fixture=0
cleanup() {
    result=$?
    trap - EXIT HUP INT TERM
    cleanup_failed=0
    if test "$docker_fixture" = 1; then
    containers=$(docker ps -aq --filter label=portico=remote-sources-r40 --filter "label=portico.job=$PORTICO_SAMBA_TEST_JOB") || cleanup_failed=1
    while read -r container; do
        if test -n "$container"; then
            docker rm --force "$container" >/dev/null || cleanup_failed=1
        fi
    done <<< "${containers:-}"
    networks=$(docker network ls -q --filter label=portico=remote-sources-r40 --filter "label=portico.job=$PORTICO_SAMBA_TEST_JOB") || cleanup_failed=1
    while read -r network; do
        if test -n "$network"; then
            docker network rm "$network" >/dev/null || cleanup_failed=1
        fi
    done <<< "${networks:-}"
    if test -f "$fixture_scratch/samba-image"; then
        docker image rm "$(cat "$fixture_scratch/samba-image")" >/dev/null || cleanup_failed=1
    fi
    fi
    rm -rf -- "$fixture_scratch"
    if test "$cleanup_failed" != 0; then
        echo 'Owned native fixture cleanup failed.' >&2
        if test "$result" = 0; then result=1; fi
    fi
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

export PORTICO_FIXTURE_SCRATCH="$fixture_scratch"
python3 - <<'PY'
import hashlib, json, os, pathlib, urllib.error, urllib.request, zipfile

lock = json.loads(pathlib.Path('../third_party/rclone/bundle.lock.json').read_text())['targets']['linux-amd64']
root = pathlib.Path(os.environ['PORTICO_FIXTURE_SCRATCH'])
archive = root / 'rclone.zip'
urls = [lock['url'], 'https://github.com/rclone/rclone/releases/download/v1.75.2/rclone-v1.75.2-linux-amd64.zip']
for index, url in enumerate(urls):
    try:
        request = urllib.request.Request(url, headers={'User-Agent': 'Portico-test-dependency/1'})
        with urllib.request.urlopen(request, timeout=60) as source, archive.open('xb') as target:
            total = 0
            while True:
                chunk = source.read(1 << 20)
                if not chunk:
                    break
                total += len(chunk)
                if total > 64 << 20:
                    raise SystemExit('locked rclone archive exceeds 64 MiB')
                target.write(chunk)
        break
    except urllib.error.HTTPError as error:
        if error.code != 403 or index != 0:
            raise
        archive.unlink(missing_ok=True)
        print('Canonical archive returned 403; using the same-hash official release asset.')
if hashlib.sha256(archive.read_bytes()).hexdigest() != lock['sha256']:
    raise SystemExit('locked rclone archive digest mismatch')
with zipfile.ZipFile(archive) as package:
    data = package.read(lock['archiveRoot'] + '/' + lock['binary'])
    if hashlib.sha256(data).hexdigest() != lock['files'][lock['binary']]:
        raise SystemExit('locked rclone executable digest mismatch')
    target = root / 'rclone'
    target.write_bytes(data)
    target.chmod(0o700)
archive.unlink()
print('Official locked rclone archive and executable verified; no owner state used.')
PY
export PORTICO_RCLONE_TEST_PROGRAM="$fixture_scratch/rclone"

if test "${PORTICO_CONTROLLER_ONLY_TEST:-}" = 1; then
    go test -json -tags=rcloneintegration -count=1 -timeout=180s "$@" \
        ./internal/remotesources -run '^TestSourceControllerRealSharedProcessAndScanPublication$'
    exit 0
fi
if ! command -v docker >/dev/null; then
    echo 'Native fixture prerequisite is missing: docker' >&2
    exit 2
fi
if docker info --format '{{json .SecurityOptions}}' | python3 -c 'import json,sys; sys.exit(0 if "name=rootless" in json.load(sys.stdin) else 1)'; then
    echo 'Native NAS fixtures require rootful Docker routing for their isolated 192.168 network.' >&2
    exit 2
fi
docker_fixture=1
docker build --network=host --label portico=remote-sources-r40 --label "portico.job=$PORTICO_SAMBA_TEST_JOB" \
    --iidfile "$fixture_scratch/samba-image" internal/rcloneprogram/testdata/smb
export PORTICO_SAMBA_TEST_IMAGE="$(cat "$fixture_scratch/samba-image")"
go test -json -tags=rcloneintegration -count=1 -timeout=180s "$@" \
    ./internal/rcloneprogram -run '^TestLocked(DAV|SFTP|SMB)MixedOperationBudget$'
if test "${PORTICO_NATIVE_SAVE_TEST:-}" = 1; then
    go test -json -tags=rcloneintegration -count=1 -timeout=180s "$@" \
        ./internal/remotesources -run '^Test(NativeSavedNASConnectionAndForbiddenEdits|SourceControllerRealSharedProcessAndScanPublication)$'
fi
