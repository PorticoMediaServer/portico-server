# SMB launch pinning qualification (P2-33)

Run on 2026-10-28 through `ssh 100.113.197.98`, on
`ehler-homelab-mini-2`, entirely in `~/astra-scratch/P2-33/`.
No owner credentials, retained configs, real media, or KDC were used.
The Samba account and file contents below were generated for this test.

## Source and downloads

For each `v` in `1.75.2 1.71.1`, the executed download commands were:

```sh
curl -fLsS https://downloads.rclone.org/v$v/rclone-v$v-linux-amd64.zip -o rclone-$v.zip
unzip -q rclone-$v.zip
curl -fLsS https://downloads.rclone.org/v$v/SHA256SUMS -o SHA256SUMS-$v
expected=$(awk '/rclone-v'"$v"'-linux-amd64.zip$/ {print $1}' SHA256SUMS-$v)
printf '%s  %s\n' "$expected" "rclone-$v.zip" | sha256sum -c -
for f in smb connpool kerberos; do
  curl -fLsS https://raw.githubusercontent.com/rclone/rclone/v$v/backend/smb/$f.go -o $f-$v.go
done
```

Both official ZIP checksums passed. Extracted executable SHA256:

- 1.75.2: `ed7ab1f17876bb006c3df3949a3044959671afd9112362dacaa5dd1647735bc1`
- 1.71.1: `5409cb410e49903af3517654ccc65c89d89f9dc12d7a97b0e13e09a9be6dc74a`

Source reviewed:
[1.75.2 options](https://github.com/rclone/rclone/blob/v1.75.2/backend/smb/smb.go),
[1.75.2 dial/authentication](https://github.com/rclone/rclone/blob/v1.75.2/backend/smb/connpool.go),
[1.71.1 options](https://github.com/rclone/rclone/blob/v1.71.1/backend/smb/smb.go),
[1.71.1 dial/authentication](https://github.com/rclone/rclone/blob/v1.71.1/backend/smb/connpool.go).

Both versions dial `f.opt.Host + ":" + f.opt.Port` using `fshttp.NewDialer`.
The default port is 445. Kerberos selects the configured SPN, or exactly
`"cifs/" + f.opt.Host` when empty (no port). NTLM receives the configured
user, revealed password, domain and SPN; it does not derive its SPN from host.
Thus numeric hosts preserve NTLM, owner SPNs stay intact, and Kerberos needs
an explicit original-host SPN before host replacement. IPv6 hosts need brackets.

## Isolated Samba fixture

Only the container named `portico-p2-33-smb` was started/restarted/removed.
Samba 4.22.11 ran with NTLM user authentication, guest access disabled, a
read-only synthetic share and a high loopback port:

```ini
[global]
server role = standalone server
security = user
map to guest = Never
server min protocol = SMB2
disable netbios = yes
smb ports = 24453
interfaces = 127.0.0.1
bind interfaces only = yes
load printers = no
printing = bsd
printcap name = /dev/null
log level = 0
[media]
path = /media
read only = yes
guest ok = no
valid users = fixture
```

Generated data/account commands:

```sh
mkdir -p share mount
printf 'P2-33 numeric SMB NTLM fixture\n' > share/fixture.txt
# Inside the task-owned Debian container:
apt-get -qq update
apt-get -qq install -y --no-install-recommends samba >/dev/null
useradd -M -s /usr/sbin/nologin fixture
printf 'P2-33-throwaway\nP2-33-throwaway\n' | smbpasswd -s -a fixture
```

Initial fixture setup mounted the packaged Samba conffile read-only, which
interfered with package configuration; binding a replacement file also retained
an old inode. These setup failures produced no passing qualification result.
The final working fixture mounted the scratch directory at `/qualification`,
finished package configuration, and used the configuration above. Exact final
container preparation/start command:

```sh
docker commit portico-p2-33-smb portico-p2-33-smb:qualification >/dev/null
docker rm -f portico-p2-33-smb >/dev/null
docker run -d --name portico-p2-33-smb --label portico.task=P2-33 \
  --memory=384m --cpus=1 --pids-limit=128 --network=host \
  --log-driver=local --log-opt max-size=64k \
  --mount type=bind,source="$PWD/share",target=/media,readonly \
  --mount type=bind,source="$PWD",target=/qualification,readonly \
  portico-p2-33-smb:qualification /bin/sh -c \
  'dpkg --configure -a >/dev/null; id fixture >/dev/null 2>&1 || { useradd -M -s /usr/sbin/nologin fixture; printf "P2-33-throwaway\nP2-33-throwaway\n" | smbpasswd -s -a fixture; }; exec smbd --foreground --no-process-group --debug-stdout --configfile=/qualification/smb.conf'
```

## Exact successful qualification script

Executed as `bash qualify.sh`. The INI rewrite is the selected carrier; there
are no remote-specific environment overrides, duplicate host keys or global
SMB host flags. The non-environment-safe leaf name also works through alias.

```bash
#!/bin/bash
set -eu
cd "$HOME/astra-scratch/P2-33"
for v in 1.75.2 1.71.1; do
 r="$PWD/rclone-v$v-linux-amd64/rclone"
 "$r" version | head -1
 pass=$("$r" obscure P2-33-throwaway)
 cat > original.conf <<EOF
[NAS.leaf-ä]
type=smb
host=ip6-localhost
port=24453
user=fixture
pass=$pass
idle_timeout=1s
[wrapper]
type=alias
remote=NAS.leaf-ä:media
EOF
 sed 's/host=ip6-localhost/host=127.0.0.1/' original.conf > pinned.conf
 if "$r" lsjson 'NAS.leaf-ä:media' --config original.conf --contimeout 2s --retries 1 --low-level-retries 1 > baseline-$v.log 2>&1; then echo 'ERROR baseline unexpectedly worked'; exit 1; fi
 echo "original hostname refused: $(tail -1 baseline-$v.log)"
 strace -f -e trace=connect -o lsjson-$v.trace "$r" lsjson 'NAS.leaf-ä:media' --config pinned.conf
 "$r" lsjson 'NAS.leaf-ä:media/fixture.txt' --stat --config pinned.conf
 "$r" cat 'NAS.leaf-ä:media/fixture.txt' --config pinned.conf
 "$r" cat wrapper:fixture.txt --config pinned.conf
 strace -f -e trace=connect -o serve-$v.trace "$r" serve webdav wrapper: --addr 127.0.0.1:24454 --config pinned.conf --read-only --dir-cache-time 0s > serve-$v.log 2>&1 &
 serve_pid=$!
 trap 'kill "$serve_pid" 2>/dev/null || true; pkill -P "$serve_pid" 2>/dev/null || true; wait "$serve_pid" 2>/dev/null || true' EXIT
 for i in $(seq 1 30); do if curl -fsS http://127.0.0.1:24454/fixture.txt -o before-$v.txt 2>/dev/null; then break; fi; sleep 1; done
 cmp share/fixture.txt before-$v.txt
 docker restart portico-p2-33-smb >/dev/null
 sleep 2
 curl -fsS http://127.0.0.1:24454/fixture.txt -o after-$v.txt
 cmp share/fixture.txt after-$v.txt
 echo "$v serve same process reconnect PASS strace-pid=$serve_pid"
 pkill -P "$serve_pid"; wait "$serve_pid" || true
 trap - EXIT
 "$r" mount wrapper: "$PWD/mount" --config pinned.conf --read-only --dir-cache-time 0s > mount-$v.log 2>&1 &
 mount_pid=$!
 trap 'fusermount3 -u "$PWD/mount" 2>/dev/null || true; kill "$mount_pid" 2>/dev/null || true; wait "$mount_pid" 2>/dev/null || true' EXIT
 for i in $(seq 1 15); do if test -f mount/fixture.txt; then break; fi; if ! kill -0 "$mount_pid" 2>/dev/null; then break; fi; sleep 1; done
 if test -f mount/fixture.txt; then cmp share/fixture.txt mount/fixture.txt; echo "$v mount PASS"; fusermount3 -u "$PWD/mount"; else echo "$v mount UNAVAILABLE"; cat mount-$v.log; fi
 kill "$mount_pid" 2>/dev/null || true; wait "$mount_pid" 2>/dev/null || true
 trap - EXIT
 rg '24453' lsjson-$v.trace | head -3
 rg '24453' serve-$v.trace | head -6
 df -h .
done
```

## Results

For **both 1.75.2 and 1.71.1**:

- Original `host=ip6-localhost` resolved to `::1` and failed with
  `dial tcp [::1]:24453: connect: connection refused`.
- Rewritten `host=127.0.0.1` succeeded with authenticated NTLM:
  lsjson listed `fixture.txt` (31 bytes), stat returned the file, direct cat
  and alias cat returned the generated contents.
- WebDAV GET returned identical bytes before and after restarting Samba,
  without restarting rclone. Each serve trace contained two SMB connects to
  `AF_INET 127.0.0.1:24453`, demonstrating initial dial and reconnect.
- FUSE mount succeeded; reading `mount/fixture.txt` matched the original bytes.
  Both mounts were explicitly unmounted, killed if still alive, and waited.
- The isolated child environment also succeeded with an intentionally poisoned
  parent variable, using these additional exact commands:

```sh
for v in 1.75.2 1.71.1; do
  RCLONE_SMB_HOST=ip6-localhost env -i PATH="$PATH" HOME="$PWD" TMPDIR="$PWD" \
    "./rclone-v$v-linux-amd64/rclone" cat wrapper:fixture.txt --config pinned.conf
done
```

The production launch paths similarly construct fresh explicit environments,
excluding inherited rclone options. With one rewritten host key, the child uses
that INI value. This avoids environment-name normalization and collisions.

Additional IPv6 qualification used the same owned Samba container:

```sh
python3 - <<'PY'
p='smb.conf'
s=open(p).read().replace('interfaces = 127.0.0.1', 'interfaces = 127.0.0.1 ::1')
with open(p, 'w') as f: f.write(s)
PY
docker restart portico-p2-33-smb >/dev/null
sleep 2
for v in 1.75.2 1.71.1; do
  sed 's/host=127.0.0.1/host=[::1]/' pinned.conf > ipv6.conf
  "./rclone-v$v-linux-amd64/rclone" cat wrapper:fixture.txt --config ipv6.conf
done
```

Both versions returned the expected bytes with bracketed numeric IPv6.

Kerberos runtime authentication remains unverified without a KDC. Its SPN path
was checked against both tagged sources and is covered by focused Go tests.
The runtime fixture exercises the INI pin carrier, NTLM, wrappers, reconnects
and command modes; Go tests separately prove policy-approved pin selection and
the production source/candidate/legacy command plumbing.

## Product coverage and Go verification

- `remotesources/egress.go:17` checks every selected native leaf and captures
  deterministic SMB pins. Saved private addresses sort ahead of public ones;
  ties sort by numeric address. Each process intentionally loses multi-address
  failover. A fresh process launch re-pins.
- `remotesources/smb_pins.go:62` rewrites host and, only when needed, the default
  Kerberos SPN in a new INI copy. This supports arbitrary accepted leaf section
  names without environment-key conversion. Owner-set SPNs and NTLM fields stay
  intact. SFTP host/key configuration and its policy tunnel stay intact.
- `remotesources/serve_runtime.go:426` supplies the copy to ServeCommandSpec;
  `rcloneprogram/serve_command.go:131` writes the child-only file.
- `mounts/egress.go:65` supplies the same copy to legacy launches:
  `mounts/service.go:608` and `mounts/guardian.go:65` for mount, and
  `mounts/native.go:187` / `mounts/native.go:137` for lsjson/stat/cat.
- `remotesources/candidate_validation.go:310` supplies it to candidate,
  connection-validation and saved-source-test serve launches. Original material
  is still used for graph interpretation and material checks.
- `remotesources/source_process_lease.go:258` additionally rechecks the retained
  pin against the fresh checked DNS set and cancels with ErrRemoteChanged when
  it disappears. Existing native/imported checks remain in place. Candidate
  process leases also recheck their pins.

Focused tests added in `smb_pins_test.go` cover the synthetic-child owner save,
saved-source test and shared-serve paths (native and alias), all SMB leaves in
mixed SMB/SFTP graphs, alias/crypt/union/combine graphs, DNS changes between
selection and command construction, deterministic selection, unapproved mixed
public/private answers, unapproved private answers, NTLM fields, owner SPNs,
implicit Kerberos SPNs, IPv6 formatting, ambient rclone environment exclusion
and process-lease retirement. Legacy egress tests verify pinned native/mount
launch files and unchanged retained bytes. The SFTP test now also asserts
unchanged launch configuration and absence of SMB pin checks. These tests need
no rclone executable.

All required checks passed from `server/`:

```sh
go build ./...
TMPDIR="$PWD/.scratch/P2-33/tmp" go vet -p 4 ./internal/remotemedia/ ./internal/remotesources/ ./internal/mounts/ ./internal/rcloneprogram/
TMPDIR="$PWD/.scratch/P2-33/tmp" go test -p 4 ./internal/remotemedia/ ./internal/remotesources/ ./internal/mounts/ ./internal/rcloneprogram/ -run 'Test(RcloneSMB|RcloneSFTP|SMBPinned|SourceProcess|BackendCapabilities|NativeApproval|NativeSourceMaterial|NativeSavePublishes|Legacy|ServeCommand|ServeProcess|SFTPProxy|MountPolicyRoute|CandidateSourceTestDrains)' -count=1
```

Results: remotemedia 0.030s; remotesources 6.765s; mounts 3.255s;
rcloneprogram 0.315s. Build and vet exited 0. Touched-file `gofmt -l` and
`git diff --check` were clean.

## Cleanup

After all commands completed, both FUSE mounts had been unmounted and all
qualification rclone children were killed and waited. Final cleanup commands:

```sh
if mountpoint -q "$PWD/mount"; then fusermount3 -u "$PWD/mount"; fi
if pgrep -af '[r]clone-v1\.(75\.2|71\.1)-linux-amd64/rclone'; then exit 1; fi
docker rm -f portico-p2-33-smb
docker image rm portico-p2-33-smb:qualification
docker image rm sha256:89c1ea91e897f0fe0a3731595330bc45b8bdef4047464a07116f891a94054b31
cd "$HOME/astra-scratch"
rm -rf -- "$HOME/astra-scratch/P2-33"
test ! -e "$HOME/astra-scratch/P2-33"
test -z "$(docker ps -aq --filter label=portico.task=P2-33)"
df -h "$HOME"
```

All remote cleanup commands succeeded. Removed: the task container and image,
newly downloaded Debian image, both rclone versions, sources, fixture account,
configuration, data, traces and scratch directory. The preexisting pinned
Debian image and unrelated containers/services were left intact. mini-2 showed
44 GB free before and after, always above the requested 15 GB minimum.
mini-1 showed 374 GB free initially and 370 GB at the final check, with 22 GB
available RAM. The local task temporary test directory was removed; shared
reusable Go caches and other agents' work were preserved.
