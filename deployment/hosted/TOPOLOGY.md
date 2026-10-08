# Portico VPS topology

This is the authoritative inventory of the five Portico VPSs as of 23 September 2026. Every host runs only the current Portico build and what that build needs. Nothing else is installed or kept: no Docker, no Patroni/etcd/Redis/HAProxy, no old releases or backups.

To check a host against this document at any time, run the read-only inventory script. It prints units, timers, cron, listening ports, the firewall, WireGuard, users, `authorized_keys` comments, directories and packages, and it never prints file contents:

```sh
ssh portico-core-us-1 'sh -s' < scripts/vps-inventory.sh
```

The output from 23 September for each host is in the [appendix](#appendix-inventory-output-23-september-2026).

## Roles

| SSH alias | Role | Provider | Public IPv4 | WireGuard | OS |
|---|---|---|---|---|---|
| `portico-core-us-1` | **Hosted**: API, web app, public site, Cast receiver, PostgreSQL | Netcup G12 | 159.195.14.223 | 10.77.0.10 | Debian 13 |
| `portico-dns-primary-1` | **ns1**.portico-dns.getportico.tv: direct DNS, and the ACME primary | InterServer | 174.138.183.45 | 10.77.0.2 | Ubuntu 24.04 |
| `portico-dns-de-1` | **ns2**.portico-dns.getportico.tv: direct DNS, and the ACME secondary | Netcup Micro | 188.68.32.46 | 10.77.0.3 | Debian 13 |
| `portico-demo` | **Demo** server (demo.getportico.tv), managed by the lead | Netcup Micro #2 | 188.68.34.120 | none | Debian 13 |
| `portico-runner` | **Build and test runner**: Go, Node and PostgreSQL test suites, managed by the lead | Contabo | 66.94.102.39 | none | Ubuntu 24.04 |

The ns1 and ns2 names are fixed by the published NS records and are not swapped: ns1 is the InterServer host and ns2 is the Netcup host.

## Access

All five hosts intentionally share **one** SSH key, `~/.ssh/portico_vps_ed25519` on Justin's Mac. It was renamed from `portico_contabo_vps_ed25519`, and a compatibility symlink remains until nothing uses the old name. On every host its `authorized_keys` comment reads `ecg-contabo-vps-admin-2026-05-01`. That text is cosmetic; it is the same key. `~/.ssh/config` defines these aliases:

| Alias | Logs in as | Notes |
|---|---|---|
| `portico-core-us-1` | root | |
| `portico-dns-primary-1` | root | |
| `portico-dns-de-1` | root | |
| `portico-demo` | root | |
| `portico-runner` | runner | use `-l root` for administration |

On the Hosted and DNS hosts, root's `authorized_keys` holds that key and nothing else. There are no other login users: the old `portico-admin` and the provider's `kvm` user have been removed. `sshd` accepts public keys only: `PasswordAuthentication no`, `PermitRootLogin prohibit-password`, `AuthenticationMethods publickey`. Every deploy script uses these aliases. None of them takes or reads a key path.

## What talks to what

```
 viewers ──HTTPS──> Cloudflare (proxied, origin-pull mTLS) ──> Caddy :443 on core-us-1
                                                               ├─ web.getportico.tv  /v1/*, SSO callbacks -> Hosted 127.0.0.1:19410
                                                               │                     everything else    -> web-host 127.0.0.1:19418
                                                               ├─ getportico.tv      static site /opt/portico-public-site/current
                                                               │                     (/.well-known/apple-app-site-association -> web-host)
                                                               ├─ www.getportico.tv  301 -> getportico.tv
                                                               └─ cast.getportico.tv static receiver /opt/portico-cast-receiver/current
 Hosted ──SQL──> PostgreSQL 17 127.0.0.1:5432 (database portico_hosted)
 Hosted ──SMTP STARTTLS──> smtp-relay.gmail.com:587 (direct; allow-listed by source IP)
 Hosted ──RFC 2136 update (TSIG acme-update.)──> BIND on ns1 10.77.0.2:5354 over wg0   (ACME DNS-01 for *.direct names)
 ns1 BIND ──NOTIFY/IXFR (TSIG acme-transfer.)──> ns2 BIND 10.77.0.3:5354 over wg0
 ns1/ns2 portico-dns ──HTTPS, authenticated──> https://web.getportico.tv/v1/dns/addresses (public, via Cloudflare)
 resolvers ──DNS :53 UDP/TCP──> dnsdist on ns1/ns2
                                 ├─ direct.getportico.tv -> portico-dns 127.0.0.1:5353
                                 ├─ acme.getportico.tv   -> BIND <own wg0 IP>:5354
                                 └─ anything else        -> REFUSED
```

WireGuard (`wg0`, UDP 51820) is a three-node mesh: core-us-1, ns1 and ns2. The only traffic that needs it is the ACME path: Hosted sends updates to ns1, and ns1 transfers the zone to ns2. The DNS address feed uses public HTTPS instead. The demo and the runner are not on WireGuard.

## portico-core-us-1: Hosted

**Services** (systemd):

| Unit | What | Listens |
|---|---|---|
| `portico-astra-hosted.service` | Hosted API. Its release is at `/opt/portico-astra-hosted/releases/<build>/hosted`; it runs as `portico-astra-hosted` and is hardened with `ProtectSystem=strict` and `MemoryMax=384M` | 127.0.0.1:19410 (API), 127.0.0.1:19530 (metrics) |
| `portico-astra-web.service` | web-host, which serves `/opt/portico-astra-hosted/releases/<build>/web` (the Hosted web build, separate from the demo's); runs as `portico-astra-hosted` with `PORTICO_APPLE_TEAM_ID=WB7BUK5Z7C` | 127.0.0.1:19418 |
| `postgresql@17-main.service` | Debian `postgresql` 17, cluster `17/main`. It listens on loopback only; `pg_hba` allows scram-sha-256 on loopback and peer authentication on the socket | 127.0.0.1:5432, [::1]:5432 |
| `caddy.service` | Caddy 2.11 from Caddy's official apt repository (`/etc/apt/sources.list.d/caddy-stable.list`). Its config is [`Caddyfile`](Caddyfile), which imports [`web.caddy`](web.caddy) as `/etc/caddy/portico-web.caddy`. HTTP/1.1 and HTTP/2 only | :80, :443 (TCP), 127.0.0.1:2019 (admin) |
| `nftables.service` | [`nftables.conf`](nftables.conf) | |
| `wg-quick@wg0.service` | peers: ns1 and ns2 | UDP 51820 |

**Timers and cron:** only the Debian defaults: apt-daily, dpkg-db-backup, e2scrub, fstrim, logrotate and tmpfiles-clean. There are no Portico timers.

**Firewall:**
- Input policy is drop.
- TCP 22 is open to everyone.
- TCP 80 and 443 are open only to Cloudflare's published ranges, because every public name is proxied.
- UDP 51820 is open only to 174.138.183.45 and 188.68.32.46.
- Nothing listens on wg0 on this host.
- Forwarding is dropped.

**Directories:**

| Path | Contents |
|---|---|
| `/opt/portico-astra-hosted/releases/<build>/` | the current release: `hosted`, `web-host`, `portico-dns`, `web/`, `release.json` |
| `/opt/portico-public-site/site-4ab1d363/`, with `current` pointing at it | the getportico.tv static site. It is **not in the repo**; these are the files the old container served, kept until Justin decides the site's future |
| `/opt/portico-cast-receiver/cast-<commit>/`, with `current` pointing at it | `portico-server/server/receiver/cast/` (index.html, receiver.js, assets/) from that commit |
| `/etc/portico-astra-hosted/` | portico-astra-hosted, 0700; every file 0600. `database-url`, `signing.json`, `signing.json.certificate.json`, `signing.json.seal.json`, `oidc.json`, `apple.p8`, `smtp.json`, `acme.json`, `acme-seal`, `dns-mac` (the DNS feed key) |
| `/etc/caddy/tls/` | caddy, 0700: `cloudflare-origin.crt`/`.key` (the Cloudflare Origin CA certificate for `*.getportico.tv` and `getportico.tv`) and `cloudflare-origin-pull-ca.pem` (the Authenticated Origin Pulls CA) |
| `/var/backups/portico-hosted/20260923T160737Z-wave3-final.dump` | the last dump from the retired Patroni database, taken when it moved to the local PostgreSQL |
| `/var/lib/postgresql/17/main` | the database |

**Service users:** `portico-astra-hosted`, `postgres`, `caddy`.

**Apple app-site-association:**
- web-host generates the file whenever `PORTICO_APPLE_TEAM_ID` is set (see `hosted-services/hosted/cmd/web/main.go`). Without it, the file answers 404.
- The same file is served at `https://web.getportico.tv/.well-known/apple-app-site-association` and at the apex (`/.well-known/...` and `/apple-app-site-association`, which Caddy routes to web-host).
- It lists app ID `WB7BUK5Z7C.tv.getportico.portico.ios` for:
  - `applinks`, covering the components `/account`, `/device`, `/join`, `/together/join` and `/app/account-return`;
  - `activitycontinuation`;
  - `webcredentials`.
- It matches the iOS entitlement (`applinks:web.getportico.tv` and `applinks:getportico.tv`).
- The tvOS app (`tv.getportico.portico.tv`) has no associated-domains entitlement, so it isn't listed.
- Verified on 23 September: both hosts answer 200 with `application/json`.

**Certificates:**
- The origin certificate is a Cloudflare Origin CA certificate for `*.getportico.tv` and `getportico.tv`, installed by hand and **valid until 30 July 2041**. Nothing renews it automatically; before it expires, issue a new one in the Cloudflare dashboard and replace the two files.
- The Authenticated Origin Pulls CA (`origin-pull.cloudflare.net`) is **valid until 1 November 2029**. Replace `cloudflare-origin-pull-ca.pem` with Cloudflare's successor before then, or origin pulls will fail.
- Cloudflare issues the public edge certificates itself.
- Hosted's own signing certificate expires on 2027-09-23; see KEYS.md. `/metrics` exposes `portico_signing_certificate_expiry_timestamp_seconds`.
- ACME certificates for member servers are issued by Hosted through Google Trust Services, using DNS-01 against the ns1 BIND.

**Deploy:**
- **Hosted and the web app:** `python3 scripts/deploy-hosted-release.py <release-dir> --operator-account-id acct_9RoUPl1SaespD87akQtmvfF6FnL7wNkTcSOgQE6qtXs` (the Apple Team ID defaults to WB7BUK5Z7C). The default `--host` is the `portico-core-us-1` alias. The script:
  1. writes both units;
  2. restarts them;
  3. waits for the new build ID and `/ready`;
  4. keeps the new release and the one it replaced, and deletes older releases.
- **Rollback:** point the two units back at the previous release, then run `systemctl daemon-reload` and restart both. If the new release migrated the schema, the old binary refuses it. In that case restore the pre-deploy dump (`pg_restore --no-owner --no-acl`) first; RUNBOOK.md has the steps.
- **Caddy:** edit `deployment/hosted/Caddyfile` and copy it to `/etc/caddy/Caddyfile`, then run `caddy validate --config /etc/caddy/Caddyfile` followed by `systemctl reload caddy`.
- **Cast receiver:** in the portico-server repository, `git archive <commit> server/receiver/cast`. Unpack it without `test/` into `/opt/portico-cast-receiver/cast-<commit>`, then move `current` to it.

## portico-dns-primary-1 (ns1) and portico-dns-de-1 (ns2)

Both nodes are built the same way by [`scripts/provision-dns-node.sh ns1|ns2`](../../scripts/provision-dns-node.sh), which renders the templates in [`deployment/dns/`](../dns/). Only BIND's role differs: ns1 is the primary for `acme.getportico.tv` and accepts TSIG-signed updates from Hosted, and ns2 is a secondary that transfers from ns1.

**Services** (systemd):

| Unit | What | Listens |
|---|---|---|
| `dnsdist.service` | dnsdist 2.0 from PowerDNS's official `dnsdist-20` apt repository, pinned at priority 600. It runs as `_dnsdist` with only CAP_NET_BIND_SERVICE. Config: [`dnsdist.conf.in`](../dns/dnsdist.conf.in). It routes the two zones, refuses everything else, applies per-source and global rate limits, and caches the direct zone | public IPv4 :53 UDP and TCP |
| `portico-dns.service` | runs `/opt/portico-dns/current/portico-dns dns` as user `portico-dns`. It reads [`portico-dns.env`](../dns/portico-dns.env) and keeps state in `/var/lib/portico-dns/addresses.json` | 127.0.0.1:5353 (DNS), 127.0.0.1:5354 (HTTP `/ready`, `/metrics`) |
| `named.service` | BIND 9 from the OS: 9.18 on Ubuntu (ns1), 9.20 on Debian (ns2). Config: [`named.primary.conf.in`](../dns/named.primary.conf.in) or [`named.secondary.conf.in`](../dns/named.secondary.conf.in). TSIG keys live in `/etc/bind/portico-keys.conf` (root:bind 0640; ns1 holds `acme-update.` and `acme-transfer.`, ns2 holds `acme-transfer.`). A drop-in orders it after `wg-quick@wg0` | own wg0 IP :5354 UDP and TCP |
| `nftables.service` | [`nftables.conf.in`](../dns/nftables.conf.in) | |
| `wg-quick@wg0.service` | peers: core-us-1 and the other DNS node | UDP 51820 |

**Timers and cron:** only the OS defaults. ns1 keeps its 512 MB swap file (`/home/swapfile`), now mounted from `/etc/fstab`.

**Firewall:**
- Input policy is drop.
- UDP and TCP 53 are open to everyone. Inbound UDP 53 and the replies to it bypass conntrack, so a flood cannot fill the connection table.
- TCP 22 is open to everyone. DHCP replies are accepted.
- UDP 51820 is open only to core-us-1 and the other DNS node.
- TCP and UDP 5354 on wg0 are open only to 10.77.0.10 and the peer's wg0 address.
- Forwarding is dropped.
- `/etc/sysctl.d/90-portico-dns.conf` raises the conntrack and SYN backlog limits.

**Directories:**

| Path | Contents |
|---|---|
| `/opt/portico-dns/releases/<build>/portico-dns`, with `current` pointing at it | the responder binary (the current release and at most one previous) |
| `/etc/portico-dns/` | root:portico-dns 0750: `portico-dns.env` (no secrets) and `feed-secret` (portico-dns 0600, the same value Hosted holds as `dns-mac`) |
| `/var/lib/portico-dns/addresses.json` | the address table, rebuilt from the feed |
| `/etc/bind/named.conf`, `/etc/bind/portico-keys.conf`, `/var/lib/bind/db.acme.getportico.tv` | BIND config and the ACME zone |
| `/etc/dnsdist/dnsdist.conf` | the rendered dnsdist config |

**Service users:** `portico-dns`, `bind`, `_dnsdist`.

**Deploy:**
- **A new portico-dns:** `python3 scripts/deploy-dns-release.py <release-dir> --node ns2`, then the same with `--node ns1`. For each node, the script:
  1. uploads and hash-checks the binary and verifies the build it reports;
  2. moves `current` to it and restarts the unit;
  3. waits up to 180 s for the authenticated feed sync and the local health check, and puts the previous release back if either fails;
  4. checks public UDP and TCP from the Mac.
  Do one node at a time, so at least one nameserver is always answering.
- **Config changes:** edit the templates, then run `scripts/provision-dns-node.sh ns1|ns2`. It installs the configs, checks them with `named-checkconf`, `dnsdist --check-config` and `nft -c`, then stops. Afterwards run `systemctl reload named` or `systemctl restart dnsdist`, plus `systemctl restart nftables`.
- **After a Hosted database restore:** run `python3 scripts/rebuild-dns-after-hosted-restore.py --hosted-restored-and-writes-paused`. It stops both responders, rebuilds each table from the feed as `portico-dns`, compares the two, and restarts both.
- **A new DNS node** (after an OS reinstall):
  1. bring up `wg0` with the two peers;
  2. put `/etc/bind/portico-keys.conf` and `/etc/portico-dns/feed-secret` in place, with values from the other node or from Hosted;
  3. run the provision script;
  4. deploy the release;
  5. start `named`, `portico-dns` and `dnsdist`.

**Check:**

```sh
for ns in 174.138.183.45 188.68.32.46; do for z in direct.getportico.tv acme.getportico.tv; do
  dig +norec SOA $z @$ns; dig +norec +tcp SOA $z @$ns; dig +norec NS $z @$ns; done; done
```

**ACME update test,** from core-us-1: a TSIG `nsupdate` to 10.77.0.2:5354 adds a TXT record under `acme.getportico.tv`. The record must be visible on ns1 and ns2 over UDP and TCP within seconds, then gets deleted. This test passed on 23 September.

## portico-demo (managed by the lead)

- **Services:**
  - `portico-demo-current.service`: the Portico server, on 127.0.0.1:32503;
  - `portico-synthetic-live.service`: a synthetic live-TV source, on 127.0.0.1:32620;
  - `portico-live-guide.timer`.
- **Edge:** nginx on :80 and :443, with a certbot certificate for demo.getportico.tv (`certbot.timer`).
- **Firewall:** ufw is active. It allows TCP 22, 80 and 443 only, over IPv4 and IPv6. The `ufw.service` unit reports inactive, but that is only its oneshot loader; the rules are loaded.
- **Files:** `/opt/portico` (releases), `/srv/portico-demo` (the demo media), and `/var/lib/portico-demo-*` and `/var/lib/portico-synthetic-live` (state).
- **Not on WireGuard.**
- **Deploy:** `scripts/stage-demo-server.py`, over the `portico-demo` alias.
- **Its web client is its own build,** served by the demo server at demo.getportico.tv. It is separate from the Hosted web build that core-us-1 serves at web.getportico.tv, and each is deployed on its own.

## portico-runner (build and test runner, managed by the lead)

- **Role:** runs the Go suites (`go test -p 3`), the Node suites and the PostgreSQL-backed Hosted tests, so the Mac only does simulator builds and small tests.
- **Workflow:** each lane syncs its tree with `git ls-files services | rsync -a --files-from=- ./ portico-runner:work/<lane>/`. Include `--others --exclude-standard` to carry new files that aren't committed yet. Tests run with `ssh portico-runner "bash -lc '...'"`, or with `scripts/runner-test.sh` once published.
- **Tooling:** Go 1.26.2 and Node 22.22.2 (npm 10.9.7) in `/usr/local`, Python 3.12, git, rsync, gcc and make. Go's build cache is in `~runner/.cache/go-build` and its module cache in `~runner/go/pkg/mod`.
- **Test database:** PostgreSQL 16 (Ubuntu) on 127.0.0.1:5432; the test URL is in `~runner/hosted-test.url`.
- **Directories:** `~runner/work/<lane>`, `~runner/cache` and `~runner/slots`.
- **Firewall:** ufw is the only firewall; `nftables.service` is disabled.
- **Not on WireGuard.**

## Public names

| Name | Served by | Status |
|---|---|---|
| `web.getportico.tv` | core-us-1: Hosted and the web app | current |
| `getportico.tv` | core-us-1: static site, plus the app-site-association file from web-host | current. The clients link to /terms, /privacy and /help, and the iOS app lists `applinks:getportico.tv` |
| `www.getportico.tv` | core-us-1: 301 to the apex | current |
| `cast.getportico.tv` | core-us-1: Cast receiver | current |
| `demo.getportico.tv` | portico-demo | current |
| `ns1`/`ns2.portico-dns.getportico.tv`, delegations of `direct.` and `acme.getportico.tv` | DNS nodes | current |
| `api.`, `cloud.`, `operator.`, `app.`, `beta-web.getportico.tv` | nothing (the old stack) | **stale: delete these Cloudflare records**. Caddy no longer has sites for them |

The Cloudflare token the old origin-failover controller used (`cloudflare-dns-controller-token`, zone `getportico.tv`) was removed from the host. It is still valid at Cloudflare until it is revoked. The same applies to `operator.getportico.tv`'s Cloudflare Access application.

## Appendix: inventory output, 23 September 2026

Produced with `scripts/vps-inventory.sh`. For core-us-1, ns1 and ns2 it was taken after the clean-up and a reboot; for the demo and the runner, after the lead's clean-up of them.

<details><summary><code>portico-core-us-1</code></summary>

```
## host
hostname: portico-core-us-1
os: Debian GNU/Linux 13 (trixie)
kernel: 6.12.90+deb13.1-amd64
address: eth0             UP             159.195.14.223/22 2a0a:4cc0:101:536:d4d6:56ff:fe95:5029/64 fe80::d4d6:56ff:fe95:5029/64 
address: wg0              UNKNOWN        10.77.0.10/32 

## enabled or running units (non-distribution)
caddy.service                                active    enabled
ifupdown-pre.service                         active    static
portico-astra-hosted.service                 active    enabled
portico-astra-web.service                    active    enabled
postgresql.service                           active    enabled
postgresql@17-main.service                   active    enabled-runtime
infrastructure: nftables.service=active wg-quick@wg0.service=active qemu-guest-agent.service=active ssh.service=active 

## unit files under /etc/systemd/system (excluding .wants links)
/etc/systemd/system/portico-astra-hosted.service
/etc/systemd/system/portico-astra-web.service

## timers
apt-daily-upgrade.timer
apt-daily.timer
dpkg-db-backup.timer
e2scrub_all.timer
fstrim.timer
logrotate.timer
systemd-tmpfiles-clean.timer

## cron
/etc/crontab:17 *	* * *	root	cd / && run-parts --report /etc/cron.hourly
/etc/crontab:25 6	* * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.daily; }
/etc/crontab:47 6	* * 7	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.weekly; }
/etc/crontab:52 6	1 * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.monthly; }
/etc/cron.d/e2scrub_all:30 3 * * 0 root test -e /run/systemd/system || SERVICE_MODE=1 /usr/libexec/e2fsprogs/e2scrub_all_cron
/etc/cron.d/e2scrub_all:10 3 * * * root test -e /run/systemd/system || SERVICE_MODE=1 /sbin/e2scrub_all -A -r
/etc/cron.d/kernel:10 13 * * * root /sbin/fstrim -a > /dev/null 2>&1
cron.hourly/daily/weekly/monthly: apt-compat dpkg logrotate 

## listening sockets
tcp *:443
tcp *:80
tcp 0.0.0.0:22
tcp 127.0.0.1:19410
tcp 127.0.0.1:19418
tcp 127.0.0.1:19530
tcp 127.0.0.1:2019
tcp 127.0.0.1:5432
tcp [::1]:5432
tcp [::]:22
udp 0.0.0.0:51820
udp [::]:51820
caddy hosted postgres sshd web-host 

## firewall (nftables)
table inet portico {
	set cloudflare4 {
		type ipv4_addr
		flags interval
		elements = { 103.21.244.0/22, 103.22.200.0/22,
			     103.31.4.0/22, 104.16.0.0/13,
			     104.24.0.0/14, 108.162.192.0/18,
			     131.0.72.0/22, 141.101.64.0/18,
			     162.158.0.0/15, 172.64.0.0/13,
			     173.245.48.0/20, 188.114.96.0/20,
			     190.93.240.0/20, 197.234.240.0/22,
			     198.41.128.0/17 }
	}
	set cloudflare6 {
		type ipv6_addr
		flags interval
		elements = { 2400:cb00::/32,
			     2405:8100::/32,
			     2405:b500::/32,
			     2606:4700::/32,
			     2803:f800::/32,
			     2a06:98c0::/29,
			     2c0f:f248::/32 }
	}
	set dns_nodes {
		type ipv4_addr
		elements = { 174.138.183.45, 188.68.32.46 }
	}
	chain input {
		type filter hook input priority filter; policy drop;
		ct state established,related accept
		ct state invalid drop
		iifname "lo" accept
		meta l4proto { icmp, ipv6-icmp } accept
		tcp dport 22 accept
		ip saddr @cloudflare4 tcp dport { 80, 443 } accept
		ip6 saddr @cloudflare6 tcp dport { 80, 443 } accept
		ip saddr @dns_nodes udp dport 51820 accept
		counter packets 294 bytes 17325 drop
	}
	chain forward {
		type filter hook forward priority filter; policy drop;
	}
	chain output {
		type filter hook output priority filter; policy accept;
	}
}

## wireguard
interface: wg0
  listening port: 51820
peer: NsXqz8vj9fpX7IPPDRz/T11oy1dVmHKcvDdaRDnG1BA=
  endpoint: 188.68.32.46:51820
  allowed ips: 10.77.0.3/32
peer: OxmsspNV2b0L7iqRmDMtCyPoPqjxaUdzTF97xinxqWg=
  endpoint: 174.138.183.45:51820
  allowed ips: 10.77.0.2/32

## containers
docker: not installed
containerd: not installed

## users (uid >= 100, with a shell or owning a service)
systemd-network 998 / /usr/sbin/nologin
dhcpcd 100 /usr/lib/dhcpcd /bin/false
messagebus 991 /nonexistent /usr/sbin/nologin
sshd 990 /run/sshd /usr/sbin/nologin
systemd-timesync 989 / /usr/sbin/nologin
portico-astra-hosted 988 /nonexistent /usr/sbin/nologin
postgres 101 /var/lib/postgresql /bin/bash
caddy 999 /var/lib/caddy /usr/sbin/nologin

## ssh authorized_keys (type and comment only)
sshd AuthorizedKeysFile: .ssh/authorized_keys .ssh/authorized_keys2
/root/.ssh/authorized_keys: ssh-ed25519 ecg-contabo-vps-admin-2026-05-01

## directories
/opt
  root:root 755 dir /opt/portico-astra-hosted
  root:root 755 dir /opt/portico-astra-hosted/releases
  root:root 755 dir /opt/portico-cast-receiver
  root:root 755 dir /opt/portico-cast-receiver/cast-5d1aa977
  root:root 777 dir /opt/portico-cast-receiver/current
  root:root 755 dir /opt/portico-public-site
  root:root 777 dir /opt/portico-public-site/current
  root:root 755 dir /opt/portico-public-site/site-4ab1d363
/srv
/etc/portico-astra-hosted
  portico-astra-hosted:portico-astra-hosted 600 file /etc/portico-astra-hosted/acme-seal
  portico-astra-hosted:portico-astra-hosted 600 file /etc/portico-astra-hosted/acme.json
  portico-astra-hosted:portico-astra-hosted 600 file /etc/portico-astra-hosted/apple.p8
  portico-astra-hosted:portico-astra-hosted 600 file /etc/portico-astra-hosted/database-url
  portico-astra-hosted:portico-astra-hosted 600 file /etc/portico-astra-hosted/dns-mac
  portico-astra-hosted:portico-astra-hosted 600 file /etc/portico-astra-hosted/oidc.json
  portico-astra-hosted:portico-astra-hosted 600 file /etc/portico-astra-hosted/signing.json
  portico-astra-hosted:portico-astra-hosted 600 file /etc/portico-astra-hosted/signing.json.certificate.json
  portico-astra-hosted:portico-astra-hosted 600 file /etc/portico-astra-hosted/signing.json.seal.json
  portico-astra-hosted:portico-astra-hosted 600 file /etc/portico-astra-hosted/smtp.json
/var/backups/portico-hosted
  root:root 600 file /var/backups/portico-hosted/20260923T160737Z-wave3-final.dump
/etc/caddy
  root:root 644 file /etc/caddy/Caddyfile
  root:root 644 file /etc/caddy/portico-web.caddy
  caddy:caddy 700 dir /etc/caddy/tls
  caddy:caddy 600 file /etc/caddy/tls/cloudflare-origin-pull-ca.pem
  caddy:caddy 600 file /etc/caddy/tls/cloudflare-origin.crt
  caddy:caddy 600 file /etc/caddy/tls/cloudflare-origin.key
/var/lib/caddy
  caddy:caddy 644 file /var/lib/caddy/.bash_logout
  caddy:caddy 644 file /var/lib/caddy/.bashrc
  caddy:caddy 700 dir /var/lib/caddy/.config
  caddy:caddy 700 dir /var/lib/caddy/.config/caddy
  caddy:caddy 700 dir /var/lib/caddy/.local
  caddy:caddy 700 dir /var/lib/caddy/.local/share
  caddy:caddy 644 file /var/lib/caddy/.profile
/etc/wireguard
  root:root 600 file /etc/wireguard/wg0.conf
/etc/postgresql
  postgres:postgres 755 dir /etc/postgresql/17
  postgres:postgres 755 dir /etc/postgresql/17/main

## packages of interest
caddy 2.11.4
postgresql 17+278

## toolchains (build and test runner)
python3: Python 3.13.5 (/usr/bin/python3)
rsync: rsync  version 3.4.1  protocol version 32 (/usr/bin/rsync)
psql: psql (PostgreSQL) 17.11 (Debian 17.11-0+deb13u1) (/usr/bin/psql)
```

</details>

<details><summary><code>portico-dns-primary-1</code></summary>

```
## host
hostname: portico-dns-primary-1
os: Ubuntu 24.04.4 LTS
kernel: 6.8.0-139-generic
address: eth0             UP             174.138.183.45/28 metric 100 2604:a00:50:43:216:3eff:fe35:e7dc/64 fe80::216:3eff:fe35:e7dc/64 
address: wg0              UNKNOWN        10.77.0.2/32 

## enabled or running units (non-distribution)
dnsdist.service                              active    enabled
named.service                                active    enabled
portico-dns.service                          active    enabled
rc-local.service                             active    static
syslog.socket                                active    static
tpm-udev.path                                inactive  enabled
infrastructure: nftables.service=active wg-quick@wg0.service=active qemu-guest-agent.service=inactive ssh.service=active 

## unit files under /etc/systemd/system (excluding .wants links)
/etc/systemd/system/bind9.service
/etc/systemd/system/chronyd.service
/etc/systemd/system/default.target
/etc/systemd/system/hibernate.target
/etc/systemd/system/hybrid-sleep.target
/etc/systemd/system/named.service.d/portico.conf
/etc/systemd/system/portico-dns.service
/etc/systemd/system/sleep.target
/etc/systemd/system/sshd-keygen@.service.d/disable-sshd-keygen-if-cloud-init-active.conf
/etc/systemd/system/suspend.target
/etc/systemd/system/systemd-networkd-wait-online.service

## timers
apt-daily-upgrade.timer
apt-daily.timer
dpkg-db-backup.timer
e2scrub_all.timer
fstrim.timer
logrotate.timer
man-db.timer
motd-news.timer
systemd-tmpfiles-clean.timer
ua-timer.timer
update-notifier-download.timer
update-notifier-motd.timer

## cron
/etc/crontab:17 *	* * *	root	cd / && run-parts --report /etc/cron.hourly
/etc/crontab:25 6	* * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.daily; }
/etc/crontab:47 6	* * 7	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.weekly; }
/etc/crontab:52 6	1 * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.monthly; }
/etc/cron.d/e2scrub_all:30 3 * * 0 root test -e /run/systemd/system || SERVICE_MODE=1 /usr/lib/x86_64-linux-gnu/e2fsprogs/e2scrub_all_cron
/etc/cron.d/e2scrub_all:10 3 * * * root test -e /run/systemd/system || SERVICE_MODE=1 /sbin/e2scrub_all -A -r
cron.hourly/daily/weekly/monthly: apt-compat dpkg logrotate man-db man-db 

## listening sockets
tcp 0.0.0.0:22
tcp 10.77.0.2:5354
tcp 127.0.0.1:5353
tcp 127.0.0.1:5354
tcp 127.0.0.1:953
tcp 127.0.0.53%lo:53
tcp 127.0.0.54:53
tcp 174.138.183.45:53
tcp [::1]:953
tcp [::]:22
udp 0.0.0.0:51820
udp 10.77.0.2:5354
udp 127.0.0.1:323
udp 127.0.0.1:5353
udp 127.0.0.53%lo:53
udp 127.0.0.54:53
udp 174.138.183.45%eth0:68
udp 174.138.183.45:53
udp [::1]:323
udp [::]:51820
chronyd dnsdist named portico-dns sshd systemd-network systemd-resolve 

## firewall (nftables)
table inet portico {
	set wg_endpoints {
		type ipv4_addr
		elements = { 159.195.14.223, 188.68.32.46 }
	}
	set wg_sources {
		type ipv4_addr
		elements = { 10.77.0.3, 10.77.0.10 }
	}
	chain prerouting_raw {
		type filter hook prerouting priority raw; policy accept;
		iifname "eth0" udp dport 53 notrack
	}
	chain output_raw {
		type filter hook output priority raw; policy accept;
		oifname "eth0" udp sport 53 notrack
	}
	chain input {
		type filter hook input priority filter; policy drop;
		iifname "lo" accept
		udp dport 53 accept
		tcp dport 53 accept
		ct state invalid drop
		ct state established,related accept
		meta l4proto { icmp, ipv6-icmp } accept
		tcp dport 22 accept
		udp sport 67 udp dport 68 accept
		udp sport 547 udp dport 546 accept
		ip saddr @wg_endpoints udp dport 51820 accept
		iifname "wg0" ip saddr @wg_sources meta l4proto { tcp, udp } th dport 5354 accept
		counter packets 1532 bytes 80884 drop
	}
	chain forward {
		type filter hook forward priority filter; policy drop;
	}
	chain output {
		type filter hook output priority filter; policy accept;
	}
}

## wireguard
interface: wg0
  listening port: 51820
peer: NsXqz8vj9fpX7IPPDRz/T11oy1dVmHKcvDdaRDnG1BA=
  endpoint: 188.68.32.46:51820
  allowed ips: 10.77.0.3/32
peer: p+A22yjliU78dXcJULekNcsbmSjVyWDWAG3vhoYOGDQ=
  endpoint: 159.195.14.223:51820
  allowed ips: 10.77.0.10/32

## containers
docker: not installed
containerd: not installed

## users (uid >= 100, with a shell or owning a service)
systemd-network 998 / /usr/sbin/nologin
systemd-timesync 997 / /usr/sbin/nologin
dhcpcd 100 /usr/lib/dhcpcd /bin/false
messagebus 101 /nonexistent /usr/sbin/nologin
systemd-resolve 992 / /usr/sbin/nologin
pollinate 102 /var/cache/pollinate /bin/false
polkitd 991 / /usr/sbin/nologin
syslog 103 /nonexistent /usr/sbin/nologin
uuidd 104 /run/uuidd /usr/sbin/nologin
tss 106 /var/lib/tpm /bin/false
sshd 109 /run/sshd /usr/sbin/nologin
_chrony 110 /var/lib/chrony /usr/sbin/nologin
bind 113 /var/cache/bind /usr/sbin/nologin
_dnsdist 114 /nonexistent /usr/sbin/nologin
portico-dns 996 /nonexistent /usr/sbin/nologin
tcpdump 999 /nonexistent /usr/sbin/nologin

## ssh authorized_keys (type and comment only)
sshd AuthorizedKeysFile: .ssh/authorized_keys .ssh/authorized_keys2
/root/.ssh/authorized_keys: ssh-ed25519 ecg-contabo-vps-admin-2026-05-01
/root/.ssh/authorized_keys2: from="66.45.228.251" AAAAB3NzaC1yc2EAAAABIwAAAgEAvuKNsgUCyIoXcpiYkfOikuzlY1TlGGKgU6jMqEu/abStxgncwIX6eV19F5WAl8WYFbpOaolIFAR1Slxd2t7FuSK9B9BGqLNYdhwOLd75EPK71gAbnE2proZvOkuVSNb6Eq6ZHzlWiRVISXZyeGfMiJWr8/BDaIOJQaUUJ5/PcLOcuvpQxqslCninf2usswNQ6feRgYRbebgY6ydBuWpvf1moTxBogAVkh5cvdmGFmFlK5L2OMnJJgfwaLHkE//F60CU5LTaZPMuK/DEM0TyPBKdNAR+4oNiw3NdX/CzCq8VPZyjaIpNkGCsMgZGC4gYcY7TXSOek+870ONGaPKKcQJVJe3IE48zeGQSAUe4FoZwoGVvOMuyM1Lh7986Q6Co8zLiGUOfvfD08kmsCtRuRhA04VigVKEEY/b1zS8T4wC1slb77HhbTL+Q0rF84rh0m0pZ2BFUDwpM64shsTfy7JVr8akN7A68UMA5yT/G7U0o3YsZW/Q0dmu/KaOv/s1sJ1Fhie/om5qsg31qZr1R9GyiOCq3qB5ZC8J8sH3ZKhHEH5ulO6nf6J02WIYJJUuIu2CSqlsvOWNwgp5z1H2T0HA407cetqRcGH+4ymBvXiLcPZTRi5wO/QGBX1NvyNP2MFaASeNm+EIvWXlQVVXnHIT5UdPLYHVv+L+YHkOT185k= root@tech.trouble-free.net

## directories
/opt
  root:root 755 dir /opt/portico-dns
  root:root 777 dir /opt/portico-dns/current
  root:root 755 dir /opt/portico-dns/releases
/srv
/etc/portico-dns
  portico-dns:portico-dns 600 file /etc/portico-dns/feed-secret
  root:portico-dns 640 file /etc/portico-dns/portico-dns.env
/var/lib/portico-dns
  portico-dns:portico-dns 600 file /var/lib/portico-dns/.dns-journal-4139796529
  portico-dns:portico-dns 600 file /var/lib/portico-dns/addresses.json
/etc/wireguard
  root:root 600 file /etc/wireguard/wg0.conf
/etc/bind
  root:root 644 file /etc/bind/bind.keys
  root:root 644 file /etc/bind/db.0
  root:root 644 file /etc/bind/db.127
  root:root 644 file /etc/bind/db.255
  root:root 644 file /etc/bind/db.empty
  root:root 644 file /etc/bind/db.local
  root:bind 640 file /etc/bind/named.conf
  root:bind 644 file /etc/bind/named.conf.default-zones
  root:bind 644 file /etc/bind/named.conf.local
  root:bind 644 file /etc/bind/named.conf.options
  root:bind 640 file /etc/bind/portico-keys.conf
  bind:bind 640 file /etc/bind/rndc.key
  root:root 644 file /etc/bind/zones.rfc1918
/etc/dnsdist
  root:_dnsdist 640 file /etc/dnsdist/dnsdist.conf
  root:root 644 file /etc/dnsdist/dnsdist.conf-dist

## packages of interest
bind9 1:9.18.39-0ubuntu0.24.04.7
dnsdist 2.0.10-1pdns.ubuntu24.04
nodejs 18.19.1+dfsg-6ubuntu5

## toolchains (build and test runner)
node: v18.19.1 (/usr/bin/node)
python3: Python 3.12.3 (/usr/bin/python3)
git: git version 2.43.0 (/usr/bin/git)
rsync: rsync  version 3.2.7  protocol version 31 (/usr/bin/rsync)
```

</details>

<details><summary><code>portico-dns-de-1</code></summary>

```
## host
hostname: portico-dns-de-1
os: Debian GNU/Linux 13 (trixie)
kernel: 6.12.100+deb13-amd64
address: eth0             UP             188.68.32.46/22 2a03:4000:10:f04:88ef:71ff:fe76:b91e/64 fe80::88ef:71ff:fe76:b91e/64 
address: wg0              UNKNOWN        10.77.0.3/32 

## enabled or running units (non-distribution)
dnsdist.service                              active    enabled
ifupdown-pre.service                         active    static
named.service                                active    enabled
portico-dns.service                          active    enabled
infrastructure: nftables.service=active wg-quick@wg0.service=active qemu-guest-agent.service=active ssh.service=active 

## unit files under /etc/systemd/system (excluding .wants links)
/etc/systemd/system/bind9.service
/etc/systemd/system/named.service.d/portico.conf
/etc/systemd/system/portico-dns.service

## timers
apt-daily-upgrade.timer
apt-daily.timer
dpkg-db-backup.timer
e2scrub_all.timer
fstrim.timer
logrotate.timer
systemd-tmpfiles-clean.timer

## cron
/etc/crontab:17 *	* * *	root	cd / && run-parts --report /etc/cron.hourly
/etc/crontab:25 6	* * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.daily; }
/etc/crontab:47 6	* * 7	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.weekly; }
/etc/crontab:52 6	1 * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.monthly; }
/etc/cron.d/e2scrub_all:30 3 * * 0 root test -e /run/systemd/system || SERVICE_MODE=1 /usr/libexec/e2fsprogs/e2scrub_all_cron
/etc/cron.d/e2scrub_all:10 3 * * * root test -e /run/systemd/system || SERVICE_MODE=1 /sbin/e2scrub_all -A -r
/etc/cron.d/kernel:26 22 * * * root /sbin/fstrim -a > /dev/null 2>&1
cron.hourly/daily/weekly/monthly: apt-compat dpkg logrotate 

## listening sockets
tcp 0.0.0.0:22
tcp 10.77.0.3:5354
tcp 127.0.0.1:5353
tcp 127.0.0.1:5354
tcp 127.0.0.1:953
tcp 188.68.32.46:53
tcp [::1]:953
tcp [::]:22
udp 0.0.0.0:51820
udp 10.77.0.3:5354
udp 127.0.0.1:5353
udp 188.68.32.46:53
udp [::]:51820
dnsdist named portico-dns sshd 

## firewall (nftables)
table inet portico {
	set wg_endpoints {
		type ipv4_addr
		elements = { 159.195.14.223, 174.138.183.45 }
	}
	set wg_sources {
		type ipv4_addr
		elements = { 10.77.0.2, 10.77.0.10 }
	}
	chain prerouting_raw {
		type filter hook prerouting priority raw; policy accept;
		iifname "eth0" udp dport 53 notrack
	}
	chain output_raw {
		type filter hook output priority raw; policy accept;
		oifname "eth0" udp sport 53 notrack
	}
	chain input {
		type filter hook input priority filter; policy drop;
		iifname "lo" accept
		udp dport 53 accept
		tcp dport 53 accept
		ct state invalid drop
		ct state established,related accept
		meta l4proto { icmp, ipv6-icmp } accept
		tcp dport 22 accept
		udp sport 67 udp dport 68 accept
		udp sport 547 udp dport 546 accept
		ip saddr @wg_endpoints udp dport 51820 accept
		iifname "wg0" ip saddr @wg_sources meta l4proto { tcp, udp } th dport 5354 accept
		counter packets 366 bytes 23523 drop
	}
	chain forward {
		type filter hook forward priority filter; policy drop;
	}
	chain output {
		type filter hook output priority filter; policy accept;
	}
}

## wireguard
interface: wg0
  listening port: 51820
peer: p+A22yjliU78dXcJULekNcsbmSjVyWDWAG3vhoYOGDQ=
  endpoint: 159.195.14.223:51820
  allowed ips: 10.77.0.10/32
peer: OxmsspNV2b0L7iqRmDMtCyPoPqjxaUdzTF97xinxqWg=
  endpoint: 174.138.183.45:51820
  allowed ips: 10.77.0.2/32

## containers
docker: not installed
containerd: not installed

## users (uid >= 100, with a shell or owning a service)
systemd-network 998 / /usr/sbin/nologin
dhcpcd 100 /usr/lib/dhcpcd /bin/false
messagebus 991 /nonexistent /usr/sbin/nologin
sshd 990 /run/sshd /usr/sbin/nologin
systemd-timesync 989 / /usr/sbin/nologin
bind 101 /var/cache/bind /usr/sbin/nologin
_dnsdist 102 /nonexistent /usr/sbin/nologin
portico-dns 999 /nonexistent /usr/sbin/nologin

## ssh authorized_keys (type and comment only)
sshd AuthorizedKeysFile: .ssh/authorized_keys .ssh/authorized_keys2
/root/.ssh/authorized_keys: ssh-ed25519 ecg-contabo-vps-admin-2026-05-01

## directories
/opt
  root:root 755 dir /opt/portico-dns
  root:root 777 dir /opt/portico-dns/current
  root:root 755 dir /opt/portico-dns/releases
/srv
/etc/portico-dns
  portico-dns:portico-dns 600 file /etc/portico-dns/feed-secret
  root:portico-dns 640 file /etc/portico-dns/portico-dns.env
/var/lib/portico-dns
  portico-dns:portico-dns 600 file /var/lib/portico-dns/.dns-journal-313432907
  portico-dns:portico-dns 600 file /var/lib/portico-dns/addresses.json
/etc/wireguard
  root:root 600 file /etc/wireguard/wg0.conf
/etc/bind
  root:bind 640 file /etc/bind/named.conf
  root:bind 644 file /etc/bind/named.conf.local
  root:bind 644 file /etc/bind/named.conf.options
  root:bind 644 file /etc/bind/named.conf.root-hints
  root:bind 640 file /etc/bind/portico-keys.conf
  bind:bind 640 file /etc/bind/rndc.key
/etc/dnsdist
  root:_dnsdist 640 file /etc/dnsdist/dnsdist.conf
  root:root 644 file /etc/dnsdist/dnsdist.conf-dist

## packages of interest
bind9 1:9.20.29-1~deb13u1
dnsdist 2.0.10-1pdns.debian13

## toolchains (build and test runner)
python3: Python 3.13.5 (/usr/bin/python3)
rsync: rsync  version 3.4.1  protocol version 32 (/usr/bin/rsync)
```

</details>

<details><summary><code>portico-demo</code></summary>

```
## host
hostname: portico-demo
os: Debian GNU/Linux 13 (trixie)
kernel: 6.12.100+deb13-amd64
address: eth0             UP             188.68.34.120/22 2a03:4000:10:342:a8d9:4cff:fe3b:fdcd/64 fe80::a8d9:4cff:fe3b:fdcd/64 

## enabled or running units (non-distribution)
certbot.timer                                active    enabled
ifupdown-pre.service                         active    static
nginx.service                                active    enabled
portico-demo-current.service                 active    enabled
portico-live-guide.timer                     active    enabled
portico-synthetic-live.service               active    enabled
ufw.service                                  inactive  enabled
infrastructure: nftables.service=inactive wg-quick@wg0.service=inactive qemu-guest-agent.service=active ssh.service=active 

## unit files under /etc/systemd/system (excluding .wants links)
/etc/systemd/system/portico-demo-current.service
/etc/systemd/system/portico-live-guide.service
/etc/systemd/system/portico-live-guide.timer
/etc/systemd/system/portico-synthetic-live.service

## timers
apt-daily-upgrade.timer
apt-daily.timer
certbot.timer
dpkg-db-backup.timer
e2scrub_all.timer
fstrim.timer
logrotate.timer
portico-live-guide.timer
systemd-tmpfiles-clean.timer

## cron
/etc/crontab:17 *	* * *	root	cd / && run-parts --report /etc/cron.hourly
/etc/crontab:25 6	* * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.daily; }
/etc/crontab:47 6	* * 7	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.weekly; }
/etc/crontab:52 6	1 * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.monthly; }
/etc/cron.d/certbot:0 */12 * * * root test -x /usr/bin/certbot -a \! -d /run/systemd/system && perl -e 'sleep int(rand(43200))' && certbot -q renew --no-random-sleep-on-renew
/etc/cron.d/e2scrub_all:30 3 * * 0 root test -e /run/systemd/system || SERVICE_MODE=1 /usr/libexec/e2fsprogs/e2scrub_all_cron
/etc/cron.d/e2scrub_all:10 3 * * * root test -e /run/systemd/system || SERVICE_MODE=1 /sbin/e2scrub_all -A -r
/etc/cron.d/kernel:15 5 * * * root /sbin/fstrim -a > /dev/null 2>&1
cron.hourly/daily/weekly/monthly: apt-compat dpkg logrotate 

## listening sockets
tcp 0.0.0.0:22
tcp 0.0.0.0:443
tcp 0.0.0.0:80
tcp 127.0.0.1:32503
tcp 127.0.0.1:32620
tcp [::]:22
tcp [::]:443
tcp [::]:80
nginx portico-server python3 sshd 

## firewall (nftables)
table ip filter {
	chain ufw-before-logging-input {
	}
	chain ufw-before-logging-output {
	}
	chain ufw-before-logging-forward {
	}
	chain ufw-before-input {
		iifname "lo" counter packets 15209180 bytes 12899940232 accept
		ct state related,established counter packets 15749292 bytes 16180961343 accept
		ct state invalid counter packets 10455 bytes 1419983 jump ufw-logging-deny
		ct state invalid counter packets 10455 bytes 1419983 drop
		ip protocol icmp icmp type destination-unreachable counter packets 0 bytes 0 accept
		ip protocol icmp icmp type time-exceeded counter packets 0 bytes 0 accept
		ip protocol icmp icmp type parameter-problem counter packets 0 bytes 0 accept
		ip protocol icmp icmp type echo-request counter packets 69599 bytes 4541933 accept
		udp sport 67 udp dport 68 counter packets 666894 bytes 204069956 accept
		counter packets 1504075 bytes 109535514 jump ufw-not-local
		ip daddr 224.0.0.251 udp dport 5353 counter packets 5563 bytes 1850371 accept
		ip daddr 239.255.255.250 udp dport 1900 counter packets 0 bytes 0 accept
		counter packets 1498512 bytes 107685143 jump ufw-user-input
	}
	chain ufw-before-output {
		oifname "lo" counter packets 15152781 bytes 12897007484 accept
		ct state related,established counter packets 13931402 bytes 15994821882 accept
		counter packets 18100 bytes 2759052 jump ufw-user-output
	}
	chain ufw-before-forward {
		ct state related,established counter packets 0 bytes 0 accept
		ip protocol icmp icmp type destination-unreachable counter packets 0 bytes 0 accept
		ip protocol icmp icmp type time-exceeded counter packets 0 bytes 0 accept
		ip protocol icmp icmp type parameter-problem counter packets 0 bytes 0 accept
		ip protocol icmp icmp type echo-request counter packets 0 bytes 0 accept
		counter packets 0 bytes 0 jump ufw-user-forward
	}
	chain ufw-after-input {
		udp dport 137 counter packets 380 bytes 29627 jump ufw-skip-to-policy-input
		udp dport 138 counter packets 1 bytes 29 jump ufw-skip-to-policy-input
		tcp dport 139 counter packets 534 bytes 24536 jump ufw-skip-to-policy-input
		tcp dport 445 counter packets 3262 bytes 151224 jump ufw-skip-to-policy-input
		udp dport 67 counter packets 6 bytes 640 jump ufw-skip-to-policy-input
		udp dport 68 counter packets 3 bytes 84 jump ufw-skip-to-policy-input
		fib daddr type broadcast counter packets 0 bytes 0 jump ufw-skip-to-policy-input
	}
	chain ufw-after-output {
	}
	chain ufw-after-forward {
	}
	chain ufw-after-logging-input {
		limit rate 3/minute burst 10 packets counter packets 26 bytes 2802 log prefix "[UFW BLOCK] "
	}
	chain ufw-after-logging-output {
	}
	chain ufw-after-logging-forward {
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW BLOCK] "
	}
	chain ufw-reject-input {
	}
	chain ufw-reject-output {
	}
	chain ufw-reject-forward {
	}
	chain ufw-track-input {
	}
	chain ufw-track-output {
		ip protocol tcp ct state new counter packets 250 bytes 33100 accept
		ip protocol udp ct state new counter packets 16099 bytes 2617685 accept
	}
	chain ufw-track-forward {
	}
	chain INPUT {
		type filter hook input priority filter; policy drop;
		counter packets 33209507 bytes 29400505161 jump ufw-before-logging-input
		counter packets 33209507 bytes 29400505161 jump ufw-before-input
		counter packets 993612 bytes 76297955 jump ufw-after-input
		counter packets 989426 bytes 76091815 jump ufw-after-logging-input
		counter packets 989426 bytes 76091815 jump ufw-reject-input
		counter packets 989426 bytes 76091815 jump ufw-track-input
	}
	chain OUTPUT {
		type filter hook output priority filter; policy accept;
		counter packets 29102290 bytes 28894588790 jump ufw-before-logging-output
		counter packets 29102290 bytes 28894588790 jump ufw-before-output
		counter packets 18107 bytes 2759424 jump ufw-after-output
		counter packets 18107 bytes 2759424 jump ufw-after-logging-output
		counter packets 18107 bytes 2759424 jump ufw-reject-output
		counter packets 18107 bytes 2759424 jump ufw-track-output
	}
	chain FORWARD {
		type filter hook forward priority filter; policy drop;
		counter packets 0 bytes 0 jump ufw-before-logging-forward
		counter packets 0 bytes 0 jump ufw-before-forward
		counter packets 0 bytes 0 jump ufw-after-forward
		counter packets 0 bytes 0 jump ufw-after-logging-forward
		counter packets 0 bytes 0 jump ufw-reject-forward
		counter packets 0 bytes 0 jump ufw-track-forward
	}
	chain ufw-logging-deny {
		ct state invalid limit rate 3/minute burst 10 packets counter packets 0 bytes 0 return
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW BLOCK] "
	}
	chain ufw-logging-allow {
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW ALLOW] "
	}
	chain ufw-skip-to-policy-input {
		counter packets 4186 bytes 206140 drop
	}
	chain ufw-skip-to-policy-output {
		counter packets 0 bytes 0 accept
	}
	chain ufw-skip-to-policy-forward {
		counter packets 0 bytes 0 drop
	}
	chain ufw-not-local {
		fib daddr type local counter packets 1498512 bytes 107685143 return
		fib daddr type multicast counter packets 5563 bytes 1850371 return
		fib daddr type broadcast counter packets 0 bytes 0 return
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 jump ufw-logging-deny
		counter packets 0 bytes 0 drop
	}
	chain ufw-user-input {
		tcp dport 22 counter packets 11 bytes 676 accept
		tcp dport 80 counter packets 0 bytes 0 accept
		tcp dport 443 counter packets 2 bytes 120 accept
	}
	chain ufw-user-output {
	}
	chain ufw-user-forward {
	}
	chain ufw-user-logging-input {
	}
	chain ufw-user-logging-output {
	}
	chain ufw-user-logging-forward {
	}
	chain ufw-user-limit {
		limit rate 3/minute burst 5 packets counter packets 0 bytes 0 log prefix "[UFW LIMIT BLOCK] "
		counter packets 0 bytes 0 reject
	}
	chain ufw-user-limit-accept {
		counter packets 0 bytes 0 accept
	}
}
table ip6 filter {
	chain ufw6-before-logging-input {
	}
	chain ufw6-before-logging-output {
	}
	chain ufw6-before-logging-forward {
	}
	chain ufw6-before-input {
		iifname "lo" counter packets 115 bytes 33041 accept
		rt type 0 counter packets 0 bytes 0 drop
		ct state related,established counter packets 592436 bytes 4781717499 accept
		meta l4proto ipv6-icmp icmpv6 type echo-reply counter packets 0 bytes 0 accept
		ct state invalid counter packets 18 bytes 1488 jump ufw6-logging-deny
		ct state invalid counter packets 18 bytes 1488 drop
		meta l4proto ipv6-icmp icmpv6 type destination-unreachable counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type packet-too-big counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type time-exceeded counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type parameter-problem counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type echo-request counter packets 189 bytes 10584 accept
		meta l4proto ipv6-icmp icmpv6 type nd-router-solicit ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type nd-router-advert ip6 hoplimit 255 counter packets 18099 bytes 1158336 accept
		meta l4proto ipv6-icmp icmpv6 type nd-neighbor-solicit ip6 hoplimit 255 counter packets 11535 bytes 830520 accept
		meta l4proto ipv6-icmp icmpv6 type nd-neighbor-advert ip6 hoplimit 255 counter packets 14192 bytes 954624 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-query counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-report counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-done counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 ip6 daddr fe80::/10 udp sport 547 udp dport 546 counter packets 0 bytes 0 accept
		ip6 daddr ff02::fb udp dport 5353 counter packets 582 bytes 247480 accept
		ip6 daddr ff02::f udp dport 1900 counter packets 0 bytes 0 accept
		counter packets 797 bytes 63868 jump ufw6-user-input
	}
	chain ufw6-before-output {
		oifname "lo" counter packets 115 bytes 33041 accept
		rt type 0 counter packets 0 bytes 0 drop
		ct state related,established counter packets 535588 bytes 76806807 accept
		meta l4proto ipv6-icmp icmpv6 type destination-unreachable counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type packet-too-big counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type time-exceeded counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type parameter-problem counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type echo-request counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type echo-reply counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type nd-router-solicit ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type nd-neighbor-advert ip6 hoplimit 255 counter packets 11535 bytes 738240 accept
		meta l4proto ipv6-icmp icmpv6 type nd-neighbor-solicit ip6 hoplimit 255 counter packets 25701 bytes 1850472 accept
		meta l4proto ipv6-icmp icmpv6 type nd-router-advert ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-query counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-report counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-done counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" counter packets 435 bytes 33060 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		counter packets 3841 bytes 508200 jump ufw6-user-output
	}
	chain ufw6-before-forward {
		rt type 0 counter packets 0 bytes 0 drop
		ct state related,established counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type destination-unreachable counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type packet-too-big counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type time-exceeded counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type parameter-problem counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type echo-request counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type echo-reply counter packets 0 bytes 0 accept
		counter packets 0 bytes 0 jump ufw6-user-forward
	}
	chain ufw6-after-input {
		udp dport 137 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
		udp dport 138 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
		tcp dport 139 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
		tcp dport 445 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
		udp dport 546 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
		udp dport 547 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
	}
	chain ufw6-after-output {
	}
	chain ufw6-after-forward {
	}
	chain ufw6-after-logging-input {
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW BLOCK] "
	}
	chain ufw6-after-logging-output {
	}
	chain ufw6-after-logging-forward {
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW BLOCK] "
	}
	chain ufw6-reject-input {
	}
	chain ufw6-reject-output {
	}
	chain ufw6-reject-forward {
	}
	chain ufw6-track-input {
	}
	chain ufw6-track-output {
		meta l4proto tcp ct state new counter packets 3259 bytes 260720 accept
		meta l4proto udp ct state new counter packets 582 bytes 247480 accept
	}
	chain ufw6-track-forward {
	}
	chain INPUT {
		type filter hook input priority filter; policy drop;
		counter packets 637963 bytes 4785017440 jump ufw6-before-logging-input
		counter packets 637963 bytes 4785017440 jump ufw6-before-input
		counter packets 5 bytes 480 jump ufw6-after-input
		counter packets 5 bytes 480 jump ufw6-after-logging-input
		counter packets 5 bytes 480 jump ufw6-reject-input
		counter packets 5 bytes 480 jump ufw6-track-input
	}
	chain OUTPUT {
		type filter hook output priority filter; policy accept;
		counter packets 577215 bytes 79969820 jump ufw6-before-logging-output
		counter packets 577215 bytes 79969820 jump ufw6-before-output
		counter packets 3841 bytes 508200 jump ufw6-after-output
		counter packets 3841 bytes 508200 jump ufw6-after-logging-output
		counter packets 3841 bytes 508200 jump ufw6-reject-output
		counter packets 3841 bytes 508200 jump ufw6-track-output
	}
	chain FORWARD {
		type filter hook forward priority filter; policy drop;
		counter packets 0 bytes 0 jump ufw6-before-logging-forward
		counter packets 0 bytes 0 jump ufw6-before-forward
		counter packets 0 bytes 0 jump ufw6-after-forward
		counter packets 0 bytes 0 jump ufw6-after-logging-forward
		counter packets 0 bytes 0 jump ufw6-reject-forward
		counter packets 0 bytes 0 jump ufw6-track-forward
	}
	chain ufw6-logging-deny {
		ct state invalid limit rate 3/minute burst 10 packets counter packets 0 bytes 0 return
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW BLOCK] "
	}
	chain ufw6-logging-allow {
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW ALLOW] "
	}
	chain ufw6-skip-to-policy-input {
		counter packets 0 bytes 0 drop
	}
	chain ufw6-skip-to-policy-output {
		counter packets 0 bytes 0 accept
	}
	chain ufw6-skip-to-policy-forward {
		counter packets 0 bytes 0 drop
	}
	chain ufw6-user-input {
		tcp dport 22 counter packets 0 bytes 0 accept
		tcp dport 80 counter packets 0 bytes 0 accept
		tcp dport 443 counter packets 0 bytes 0 accept
	}
	chain ufw6-user-output {
	}
	chain ufw6-user-forward {
	}
	chain ufw6-user-logging-input {
	}
	chain ufw6-user-logging-output {
	}
	chain ufw6-user-logging-forward {
	}
	chain ufw6-user-limit {
		limit rate 3/minute burst 5 packets counter packets 0 bytes 0 log prefix "[UFW LIMIT BLOCK] "
		counter packets 0 bytes 0 reject
	}
	chain ufw6-user-limit-accept {
		counter packets 0 bytes 0 accept
	}
}

## wireguard

## containers
docker: not installed
containerd: not installed

## users (uid >= 100, with a shell or owning a service)
systemd-network 998 / /usr/sbin/nologin
dhcpcd 100 /usr/lib/dhcpcd /bin/false
messagebus 991 /nonexistent /usr/sbin/nologin
sshd 990 /run/sshd /usr/sbin/nologin
systemd-timesync 989 / /usr/sbin/nologin
portico 988 /var/lib/portico /usr/sbin/nologin

## ssh authorized_keys (type and comment only)
sshd AuthorizedKeysFile: .ssh/authorized_keys .ssh/authorized_keys2
/root/.ssh/authorized_keys: ssh-ed25519 ecg-contabo-vps-admin-2026-05-01

## directories
/opt
  root:root 755 dir /opt/portico
  root:root 755 dir /opt/portico/demo-releases
/srv
  root:root 755 dir /srv/portico-demo
  root:root 755 dir /srv/portico-demo-20260919
  root:root 755 dir /srv/portico-demo-20260919/Anime
  root:root 755 dir /srv/portico-demo-20260919/Audiobooks
  root:root 755 dir /srv/portico-demo-20260919/Movies
  root:root 755 dir /srv/portico-demo-20260919/Music
  root:root 755 dir /srv/portico-demo-20260919/TV
  root:root 644 file /srv/portico-demo-20260919/fixture-provenance.json
  root:root 644 file /srv/portico-demo-20260919/synthetic-audio.m4a
  root:root 644 file /srv/portico-demo-20260919/synthetic-video.mp4
  root:root 755 dir /srv/portico-demo/live
/etc/portico-demo-20260919.env
/var/lib/portico-demo-20260923
  portico:portico 700 dir /var/lib/portico-demo-20260923/analysis-artifacts
  portico:portico 700 dir /var/lib/portico-demo-20260923/analysis-artifacts/objects
  portico:portico 700 dir /var/lib/portico-demo-20260923/analysis-artifacts/producer-locks
  portico:portico 700 dir /var/lib/portico-demo-20260923/analysis-artifacts/staging
  portico:portico 700 dir /var/lib/portico-demo-20260923/artwork
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00066e2fe1b6d58f7e63c8134cf98d1fa2fd09e5ac9f0c2491935ad397d0f3e4.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0009b319574bb4288ab3e6ab09d40b6e7a360f0b54c04458e276a2738d460f2e.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/000e2ec1882ed28c178c368fe35a4d953ac1fba31ebad1b2979715608bc3c0e3.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0012ee436b4446fabd27d9d9f6c3d055d66c2537500268211f5149e4c3c6617a.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0013b6fab6dd699eb4e3236a93e170e82572d1c0fa772e5348fdddb49e8b1a65.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0022f453ce521d54fe3afc0b555d8dcfec854cd550e3f43283778a01e4b60635.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0023be1ba23928e9d2151a8a8e01e5ba1f20bb1c94fdabfdd4bdec0cc82ba019.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/002a73545dcf77d5977a725804b2f482d4ca844d22e0836042c3762be1ea223d.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/002f02aab05b0e0bfe1272a0a28202218170d9fe4437a01448d47231cb9edb42.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/003becebcd0cf16ba5dcc51169069457f7a7e200acb489de11d4d57b64604758.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/003f85917ef5b4659649b45ae20c5ff9dbc1d5ab6d3508828d7c5d100d0f516c.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0044e3cb9185ae180aa7ee239e6134ca0703f19073014e4c057fc57acf9b9224.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00464a5fa7dd208b8c9c613cbfeafda9395c3fb5ac55e4a4d5ff20af255d0f1f.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0048dccdbafbc6fee82501f09fae88b70e9fd6eabd527560472f66f6c031b37f.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/005c8ffb39e14ce704790c8c9f381d3bc8eb965be016028ab3848aaefa1b4a31.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/005cded3cd9baef1a6058bea9672c8075bead190467211638b1b167b44667049.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/006580ad0d174a450261bfbfc4e517eb465fa092e7621a70f3c198e208233141.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0068f284553e80d1bab475082058a2669a1f71ccca2ebd2ea3c8624f9279fb98.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/006df73d19700008cb7644d5d0b780ec5331af54d2a0fb02ab7bb1ce1a6aff0e.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/007947fdcf66b64953d04575fe92d14ed242589fe93695725cb95035df10a5c9.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/007d401e167dfae77c7384658a492ffadf9f40fbe77af3c50938cdf0bdfda383.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0087edd4874e13b11aff0b8129e2388b4b7a261c54889afa0b68724b63ec6b2b.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0097aa94782ed2c5c551c6f661d4e8ac5c2099edac2f9c60f0814c08f3afe05d.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/009e6b2d6a3b0aef45681086a10657638901a5bed5990dfd301578e56634fae9.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/009ff5fe8e60a61dff5f3831c3c102b5d6f24f338cc85402933fe17f03af2439.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00a824f78588850deeab0491de3f20229c015a04662c3ccd0d864d9f48b4f937.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00ac53d40d1abf54a181e99d2a729f0b8b7b8aa9e0f07a924e2d7a82d7198e81.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00adc608b164b7734bcdc420ebc892725000917cd727ab032d31144bcd8961f7.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00ae35af5bc263cd951bd51c19622d5ae54293874c5b885dead20022bb259b7f.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00b2c20f019e08c6a0f9b6d6390787ef31ee8c501d7e29fbafbe37fedf908379.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00b40bfa5282f2597e24d86ceeb5ab8b53e441f82c8ae047a4b45855de17baec.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00b44c00c8a47fa20b9d50e7ede11e1f341e0f4d3a85acaf93bbe98a81cf98c4.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00b8ab8541e3b988baf7b0ac1a94c079551b0861e9fa7d8247188552d9352138.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00bc59d7130e2a2c752eed09968f995c8713f263a7080ebd246077e098539320.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00cad6cbd690fd58e433012179968aceb3f8683f036d86d0f2524db88db60816.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00d5469768ed79b8d29daf3457b41fef14fbdf8fe25a27d178f5aab377b72763.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00d743190c06320cbd70b585e6daf2f9dbc54656261267715ea771ff8327fdf1.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00db82f88d5d3d00432b81355c6c4af35eb73197bb61634f09d865eb67be5803.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00e71dbd750d777dcea77c211488256e3da052799adc435a54cdc864240cad2d.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00ea1201f05d27d0a394ecb2aaadaf08f009e004587b6d909ff4ffc40b04f76d.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00f2eb2d34513b238d6061ab200309cca4696ba530cdd3ffa133e7a44baa4195.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00f4374b2e42f3bb004a80cdfee2de621411b4741683c7187a0dacb242f92383.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00f4f3528e4d07a75899c1e7a3dad10bff379c96e13e8455b1cbcb898d61502f.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00f9b14bb07ae13a92a3c42becfe62014306bf26d5b29d6b66a46ed0ee96d093.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/00fbe3b39bbfee5a495e795b5b716c1857cca8f1de8cacc3a592737a396edce7.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0101112ac2e92516b5ba1415f25af89b2c9b2a9e2f7669782ef4d97cb1ce6b96.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0107f01eaa448857640cf2239917a22aa424d1bb6bfe5fbacac3501f9dc552d3.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0110e0d1c278bcf7c3484bc974d09d395457eb69d82c1f7a8f418142e058c66c.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01128c43b0390354fbf5e642c278714b29664b56fa5eab7c4b1d9787e86cfe7c.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/011cc43e1fb773cf6309451b424b892be833ec644e7aa059cfc6b12880b00f71.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01389149241d1c3643f098944c3cfb98f82510c7d44eccbf994a473ce495d32f.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0139025c2657fa50047224e145577adeea0490ea73f503ff06b7b8d2dba25048.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0146afab39fb07eb91ecd659ad14ad908b0d951e14ea041cf583b903338f0063.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0155d806ec302fc00673bb048b0a49a7ba0bbcbcff8b9864f3778bbda051eaa8.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/015838796b959f3762b2397ea1f3fc9fb7e75613582d0b1afb61e3c4695703c0.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0177c1e75ff6c8006f1c8950eba567ad3928bf920bb415c0fb2c6afbd34beab6.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01781b0917551f319db9641595c3fb80c70a5b54bb7a2c9d8612b288b6c07f51.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/017c9653728b60b19ba06647adb661ce95a84cbdf43662174440176fd76361fc.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/017ea2f59af6c646d3dadbd05357c360b15147fc7e0f4506619c42c65224f4eb.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0180370ffb344430e8dc5540f15c2d25f3185b2dd71209673a29560c48801492.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01856d976fc67fd29c2ace01bdb1f5dff1541d47038c47d8e1f8de146c7982f8.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0190e39332f30e89d30fa4c74cd87e0a130d71445a8a03dad04c736d76be4499.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/0191ad5a53330ace1825b63e6b8ddbb4a9a48e7e11c44369a7cad5685f87ceea.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01b090cfc9e22703b98f9a531263c70539a2737b1db1c99e515a505f24c0fb92.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01b2bfe03d422b09794c8f40996ca64200373f5d4c11bf0856c05103fa90d710.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01b4e6afdb25109e3599e50d1cc6fd542134a2613bbe6931d703a58e17486f61.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01c4aeef36aac43b986a19043c51978d1a1a533f45de68b0a14f29caced8eb99.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01c949e4142c5c9d4e8aa99bcbe7197cdf205d865328e7cea4e86cbdb4f81b3e.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01d69d7762c10d5918b0244ad0d4632aa9c30a90f52d7dbf1f011e5df27fb098.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01dc58fd5525bad3c21a0922aaa96a76df65e47ad29a52c64edd18c506808783.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01e71cf3e7941031b8be82579e0954ddccc6d8cbc7a69765383450c69b2050c0.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01e7bc8eca7b34ee4fcd5720fd485cc8cf7b6a73261619f196f9e72f61271561.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01eb59e423533d6d2293f63a12407b214dbd20f23cf97b87cb01de12e5a0ae1c.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01f0c779f9525a6c646d3c0d6d2a14749222a82f0a013249c11ee31b99c5a353.img
  portico:portico 600 file /var/lib/portico-demo-20260923/artwork/01f6f98eccc44ed57cf09675db4aa8c3bda28a052faa2ad4e55a9fd5e8db1443.img
  ... 10410 more entries
/var/lib/portico-demo-20260923.bak-pre-0bcbd792
  portico:portico 700 dir /var/lib/portico-demo-20260923.bak-pre-0bcbd792/analysis-artifacts
  portico:portico 700 dir /var/lib/portico-demo-20260923.bak-pre-0bcbd792/analysis-artifacts/objects
  portico:portico 700 dir /var/lib/portico-demo-20260923.bak-pre-0bcbd792/analysis-artifacts/producer-locks
  portico:portico 700 dir /var/lib/portico-demo-20260923.bak-pre-0bcbd792/analysis-artifacts/staging
  portico:portico 700 dir /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00066e2fe1b6d58f7e63c8134cf98d1fa2fd09e5ac9f0c2491935ad397d0f3e4.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0009b319574bb4288ab3e6ab09d40b6e7a360f0b54c04458e276a2738d460f2e.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/000e2ec1882ed28c178c368fe35a4d953ac1fba31ebad1b2979715608bc3c0e3.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0012ee436b4446fabd27d9d9f6c3d055d66c2537500268211f5149e4c3c6617a.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0013b6fab6dd699eb4e3236a93e170e82572d1c0fa772e5348fdddb49e8b1a65.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0022f453ce521d54fe3afc0b555d8dcfec854cd550e3f43283778a01e4b60635.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0023be1ba23928e9d2151a8a8e01e5ba1f20bb1c94fdabfdd4bdec0cc82ba019.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/002a73545dcf77d5977a725804b2f482d4ca844d22e0836042c3762be1ea223d.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/002f02aab05b0e0bfe1272a0a28202218170d9fe4437a01448d47231cb9edb42.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/003becebcd0cf16ba5dcc51169069457f7a7e200acb489de11d4d57b64604758.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/003f85917ef5b4659649b45ae20c5ff9dbc1d5ab6d3508828d7c5d100d0f516c.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0044e3cb9185ae180aa7ee239e6134ca0703f19073014e4c057fc57acf9b9224.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00464a5fa7dd208b8c9c613cbfeafda9395c3fb5ac55e4a4d5ff20af255d0f1f.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0048dccdbafbc6fee82501f09fae88b70e9fd6eabd527560472f66f6c031b37f.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/005c8ffb39e14ce704790c8c9f381d3bc8eb965be016028ab3848aaefa1b4a31.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/005cded3cd9baef1a6058bea9672c8075bead190467211638b1b167b44667049.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/006580ad0d174a450261bfbfc4e517eb465fa092e7621a70f3c198e208233141.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0068f284553e80d1bab475082058a2669a1f71ccca2ebd2ea3c8624f9279fb98.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/006df73d19700008cb7644d5d0b780ec5331af54d2a0fb02ab7bb1ce1a6aff0e.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/007947fdcf66b64953d04575fe92d14ed242589fe93695725cb95035df10a5c9.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/007d401e167dfae77c7384658a492ffadf9f40fbe77af3c50938cdf0bdfda383.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0087edd4874e13b11aff0b8129e2388b4b7a261c54889afa0b68724b63ec6b2b.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0097aa94782ed2c5c551c6f661d4e8ac5c2099edac2f9c60f0814c08f3afe05d.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/009e6b2d6a3b0aef45681086a10657638901a5bed5990dfd301578e56634fae9.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/009ff5fe8e60a61dff5f3831c3c102b5d6f24f338cc85402933fe17f03af2439.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00a824f78588850deeab0491de3f20229c015a04662c3ccd0d864d9f48b4f937.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00ac53d40d1abf54a181e99d2a729f0b8b7b8aa9e0f07a924e2d7a82d7198e81.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00adc608b164b7734bcdc420ebc892725000917cd727ab032d31144bcd8961f7.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00ae35af5bc263cd951bd51c19622d5ae54293874c5b885dead20022bb259b7f.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00b2c20f019e08c6a0f9b6d6390787ef31ee8c501d7e29fbafbe37fedf908379.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00b40bfa5282f2597e24d86ceeb5ab8b53e441f82c8ae047a4b45855de17baec.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00b44c00c8a47fa20b9d50e7ede11e1f341e0f4d3a85acaf93bbe98a81cf98c4.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00b8ab8541e3b988baf7b0ac1a94c079551b0861e9fa7d8247188552d9352138.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00bc59d7130e2a2c752eed09968f995c8713f263a7080ebd246077e098539320.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00cad6cbd690fd58e433012179968aceb3f8683f036d86d0f2524db88db60816.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00d5469768ed79b8d29daf3457b41fef14fbdf8fe25a27d178f5aab377b72763.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00d743190c06320cbd70b585e6daf2f9dbc54656261267715ea771ff8327fdf1.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00db82f88d5d3d00432b81355c6c4af35eb73197bb61634f09d865eb67be5803.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00e71dbd750d777dcea77c211488256e3da052799adc435a54cdc864240cad2d.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00ea1201f05d27d0a394ecb2aaadaf08f009e004587b6d909ff4ffc40b04f76d.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00f2eb2d34513b238d6061ab200309cca4696ba530cdd3ffa133e7a44baa4195.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00f4374b2e42f3bb004a80cdfee2de621411b4741683c7187a0dacb242f92383.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00f4f3528e4d07a75899c1e7a3dad10bff379c96e13e8455b1cbcb898d61502f.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00f9b14bb07ae13a92a3c42becfe62014306bf26d5b29d6b66a46ed0ee96d093.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/00fbe3b39bbfee5a495e795b5b716c1857cca8f1de8cacc3a592737a396edce7.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0101112ac2e92516b5ba1415f25af89b2c9b2a9e2f7669782ef4d97cb1ce6b96.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0107f01eaa448857640cf2239917a22aa424d1bb6bfe5fbacac3501f9dc552d3.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0110e0d1c278bcf7c3484bc974d09d395457eb69d82c1f7a8f418142e058c66c.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01128c43b0390354fbf5e642c278714b29664b56fa5eab7c4b1d9787e86cfe7c.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/011cc43e1fb773cf6309451b424b892be833ec644e7aa059cfc6b12880b00f71.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01389149241d1c3643f098944c3cfb98f82510c7d44eccbf994a473ce495d32f.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0139025c2657fa50047224e145577adeea0490ea73f503ff06b7b8d2dba25048.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0146afab39fb07eb91ecd659ad14ad908b0d951e14ea041cf583b903338f0063.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0155d806ec302fc00673bb048b0a49a7ba0bbcbcff8b9864f3778bbda051eaa8.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/015838796b959f3762b2397ea1f3fc9fb7e75613582d0b1afb61e3c4695703c0.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0177c1e75ff6c8006f1c8950eba567ad3928bf920bb415c0fb2c6afbd34beab6.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01781b0917551f319db9641595c3fb80c70a5b54bb7a2c9d8612b288b6c07f51.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/017c9653728b60b19ba06647adb661ce95a84cbdf43662174440176fd76361fc.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/017ea2f59af6c646d3dadbd05357c360b15147fc7e0f4506619c42c65224f4eb.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0180370ffb344430e8dc5540f15c2d25f3185b2dd71209673a29560c48801492.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01856d976fc67fd29c2ace01bdb1f5dff1541d47038c47d8e1f8de146c7982f8.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0190e39332f30e89d30fa4c74cd87e0a130d71445a8a03dad04c736d76be4499.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/0191ad5a53330ace1825b63e6b8ddbb4a9a48e7e11c44369a7cad5685f87ceea.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01b090cfc9e22703b98f9a531263c70539a2737b1db1c99e515a505f24c0fb92.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01b2bfe03d422b09794c8f40996ca64200373f5d4c11bf0856c05103fa90d710.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01b4e6afdb25109e3599e50d1cc6fd542134a2613bbe6931d703a58e17486f61.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01c4aeef36aac43b986a19043c51978d1a1a533f45de68b0a14f29caced8eb99.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01c949e4142c5c9d4e8aa99bcbe7197cdf205d865328e7cea4e86cbdb4f81b3e.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01d69d7762c10d5918b0244ad0d4632aa9c30a90f52d7dbf1f011e5df27fb098.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01dc58fd5525bad3c21a0922aaa96a76df65e47ad29a52c64edd18c506808783.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01e71cf3e7941031b8be82579e0954ddccc6d8cbc7a69765383450c69b2050c0.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01e7bc8eca7b34ee4fcd5720fd485cc8cf7b6a73261619f196f9e72f61271561.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01eb59e423533d6d2293f63a12407b214dbd20f23cf97b87cb01de12e5a0ae1c.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01f0c779f9525a6c646d3c0d6d2a14749222a82f0a013249c11ee31b99c5a353.img
  portico:portico 600 file /var/lib/portico-demo-20260923.bak-pre-0bcbd792/artwork/01f6f98eccc44ed57cf09675db4aa8c3bda28a052faa2ad4e55a9fd5e8db1443.img
  ... 10408 more entries
/var/lib/portico-synthetic-live
  portico:portico 644 file /var/lib/portico-synthetic-live/pattern.mp4
  portico:portico 644 file /var/lib/portico-synthetic-live/serve.py
  portico:portico 755 dir /var/lib/portico-synthetic-live/streams
  portico:portico 755 dir /var/lib/portico-synthetic-live/streams/demo-cinema
  portico:portico 755 dir /var/lib/portico-synthetic-live/streams/demo-family
  portico:portico 755 dir /var/lib/portico-synthetic-live/streams/demo-music

## packages of interest
certbot 4.0.0-2+deb13u1
nginx 1.26.3-3+deb13u7

## toolchains (build and test runner)
python3: Python 3.13.5 (/usr/bin/python3)
rsync: rsync  version 3.4.1  protocol version 32 (/usr/bin/rsync)
```

</details>

<details><summary><code>portico-runner</code></summary>

```
## host
hostname: portico-runner
os: Ubuntu 24.04.4 LTS
kernel: 6.8.0-138-generic
address: eth0             UP             66.94.102.39/21 2605:a144:2327:3137::1/64 fe80::250:56ff:fe63:a474/64 
address: docker0          DOWN           172.17.0.1/16 fe80::ccf6:9bff:fea3:fe57/64 

## enabled or running units (non-distribution)
apport-autoreport.path                       inactive  enabled
apport-autoreport.timer                      inactive  enabled
apport-forward.socket                        inactive  enabled
apport.service                               active    enabled
mdcheck_continue.timer                       inactive  enabled
mdcheck_start.timer                          inactive  enabled
mdmonitor-oneshot.timer                      inactive  enabled
postgresql.service                           active    enabled
postgresql@16-main.service                   active    enabled-runtime
ssl-cert.service                             inactive  enabled
syslog.socket                                active    static
tpm-udev.path                                inactive  enabled
infrastructure: nftables.service=inactive wg-quick@wg0.service=inactive qemu-guest-agent.service=inactive ssh.service=active 

## unit files under /etc/systemd/system (excluding .wants links)

## timers
apport-autoreport.timer
apt-daily-upgrade.timer
apt-daily.timer
dpkg-db-backup.timer
e2scrub_all.timer
fstrim.timer
fwupd-refresh.timer
logrotate.timer
man-db.timer
motd-news.timer
snapd.snap-repair.timer
sysstat-collect.timer
sysstat-summary.timer
systemd-tmpfiles-clean.timer
ua-timer.timer
update-notifier-download.timer
update-notifier-motd.timer

## cron
/etc/crontab:17 *	* * *	root	cd / && run-parts --report /etc/cron.hourly
/etc/crontab:25 6	* * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.daily; }
/etc/crontab:47 6	* * 7	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.weekly; }
/etc/crontab:52 6	1 * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.monthly; }
/etc/cron.d/e2scrub_all:30 3 * * 0 root test -e /run/systemd/system || SERVICE_MODE=1 /usr/lib/x86_64-linux-gnu/e2fsprogs/e2scrub_all_cron
/etc/cron.d/e2scrub_all:10 3 * * * root test -e /run/systemd/system || SERVICE_MODE=1 /sbin/e2scrub_all -A -r
/etc/cron.d/staticroute:@reboot root ip route replace $(ip route list dev eth0 scope link | head -n1 | awk '{ print $1 }') via $(ip route list dev eth0 | awk '/default/{ print $3 }') dev eth0 &>/dev/null
/etc/cron.d/sysstat:5-55/10 * * * * root command -v debian-sa1 > /dev/null && debian-sa1 1 1
/etc/cron.d/sysstat:59 23 * * * root command -v debian-sa1 > /dev/null && debian-sa1 60 2
cron.hourly/daily/weekly/monthly: apport apt-compat dpkg logrotate man-db sysstat free fstrim man-db 

## listening sockets
tcp 0.0.0.0:22
tcp 127.0.0.1:5432
tcp 127.0.0.53%lo:53
tcp 127.0.0.54:53
tcp [::1]:5432
tcp [::]:22
udp 127.0.0.53%lo:53
udp 127.0.0.54:53
postgres sshd systemd-resolve 

## firewall (nftables)
table ip filter {
	chain INPUT {
		type filter hook input priority filter; policy drop;
		counter packets 33720 bytes 22555892 jump ufw-before-logging-input
		counter packets 33720 bytes 22555892 jump ufw-before-input
		counter packets 62 bytes 4344 jump ufw-after-input
		counter packets 62 bytes 4344 jump ufw-after-logging-input
		counter packets 62 bytes 4344 jump ufw-reject-input
		counter packets 62 bytes 4344 jump ufw-track-input
	}
	chain OUTPUT {
		type filter hook output priority filter; policy accept;
		counter packets 21290 bytes 2447350 jump ufw-before-logging-output
		counter packets 21290 bytes 2447350 jump ufw-before-output
		counter packets 4 bytes 284 jump ufw-after-output
		counter packets 4 bytes 284 jump ufw-after-logging-output
		counter packets 4 bytes 284 jump ufw-reject-output
		counter packets 4 bytes 284 jump ufw-track-output
	}
	chain FORWARD {
		type filter hook forward priority filter; policy drop;
		counter packets 0 bytes 0 jump ufw-before-logging-forward
		counter packets 0 bytes 0 jump ufw-before-forward
		counter packets 0 bytes 0 jump ufw-after-forward
		counter packets 0 bytes 0 jump ufw-after-logging-forward
		counter packets 0 bytes 0 jump ufw-reject-forward
		counter packets 0 bytes 0 jump ufw-track-forward
	}
	chain ufw-before-logging-input {
	}
	chain ufw-before-logging-output {
	}
	chain ufw-before-logging-forward {
	}
	chain ufw-before-input {
		iifname "lo" counter packets 14265 bytes 1361275 accept
		ct state related,established counter packets 17075 bytes 20353873 accept
		ct state invalid counter packets 3 bytes 132 jump ufw-logging-deny
		ct state invalid counter packets 3 bytes 132 drop
		ip protocol icmp icmp type destination-unreachable counter packets 0 bytes 0 accept
		ip protocol icmp icmp type time-exceeded counter packets 0 bytes 0 accept
		ip protocol icmp icmp type parameter-problem counter packets 0 bytes 0 accept
		ip protocol icmp icmp type echo-request counter packets 4 bytes 120 accept
		udp sport 67 udp dport 68 counter packets 2268 bytes 833496 accept
		counter packets 105 bytes 6996 jump ufw-not-local
		ip daddr 224.0.0.251 udp dport 5353 counter packets 0 bytes 0 accept
		ip daddr 239.255.255.250 udp dport 1900 counter packets 0 bytes 0 accept
		counter packets 105 bytes 6996 jump ufw-user-input
	}
	chain ufw-before-output {
		oifname "lo" counter packets 14265 bytes 1361275 accept
		ct state related,established counter packets 7021 bytes 1085791 accept
		counter packets 4 bytes 284 jump ufw-user-output
	}
	chain ufw-before-forward {
		ct state related,established counter packets 0 bytes 0 accept
		ip protocol icmp icmp type destination-unreachable counter packets 0 bytes 0 accept
		ip protocol icmp icmp type time-exceeded counter packets 0 bytes 0 accept
		ip protocol icmp icmp type parameter-problem counter packets 0 bytes 0 accept
		ip protocol icmp icmp type echo-request counter packets 0 bytes 0 accept
		counter packets 0 bytes 0 jump ufw-user-forward
	}
	chain ufw-after-input {
		udp dport 137 counter packets 0 bytes 0 jump ufw-skip-to-policy-input
		udp dport 138 counter packets 0 bytes 0 jump ufw-skip-to-policy-input
		tcp dport 139 counter packets 0 bytes 0 jump ufw-skip-to-policy-input
		tcp dport 445 counter packets 0 bytes 0 jump ufw-skip-to-policy-input
		udp dport 67 counter packets 0 bytes 0 jump ufw-skip-to-policy-input
		udp dport 68 counter packets 0 bytes 0 jump ufw-skip-to-policy-input
		fib daddr type broadcast counter packets 0 bytes 0 jump ufw-skip-to-policy-input
	}
	chain ufw-after-output {
	}
	chain ufw-after-forward {
	}
	chain ufw-after-logging-input {
		limit rate 3/minute burst 10 packets counter packets 29 bytes 1775 log prefix "[UFW BLOCK] "
	}
	chain ufw-after-logging-output {
	}
	chain ufw-after-logging-forward {
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW BLOCK] "
	}
	chain ufw-reject-input {
	}
	chain ufw-reject-output {
	}
	chain ufw-reject-forward {
	}
	chain ufw-track-input {
	}
	chain ufw-track-output {
		ip protocol tcp ct state new counter packets 0 bytes 0 accept
		ip protocol udp ct state new counter packets 4 bytes 284 accept
	}
	chain ufw-track-forward {
	}
	chain ufw-logging-deny {
		ct state invalid limit rate 3/minute burst 10 packets counter packets 3 bytes 132 return
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW BLOCK] "
	}
	chain ufw-logging-allow {
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW ALLOW] "
	}
	chain ufw-skip-to-policy-input {
		counter packets 0 bytes 0 drop
	}
	chain ufw-skip-to-policy-output {
		counter packets 0 bytes 0 accept
	}
	chain ufw-skip-to-policy-forward {
		counter packets 0 bytes 0 drop
	}
	chain ufw-not-local {
		fib daddr type local counter packets 105 bytes 6996 return
		fib daddr type multicast counter packets 0 bytes 0 return
		fib daddr type broadcast counter packets 0 bytes 0 return
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 jump ufw-logging-deny
		counter packets 0 bytes 0 drop
	}
	chain ufw-user-input {
		tcp dport 22 counter packets 42 bytes 2600 accept
		tcp dport 80 counter packets 0 bytes 0 accept
		tcp dport 443 counter packets 1 bytes 52 accept
		ip saddr 10.77.0.10 ip daddr 10.77.0.0/24 iifname "wg0" tcp dport 2379 counter packets 0 bytes 0 accept
		ip saddr 10.77.0.10 ip daddr 10.77.0.0/24 iifname "wg0" tcp dport 2380 counter packets 0 bytes 0 accept
		ip saddr 10.77.0.10 ip daddr 10.77.0.0/24 iifname "wg0" tcp dport 26379 counter packets 0 bytes 0 accept
		ip saddr 10.77.0.2 ip daddr 10.77.0.0/24 iifname "wg0" tcp dport 2379 counter packets 0 bytes 0 accept
		ip saddr 10.77.0.2 ip daddr 10.77.0.0/24 iifname "wg0" tcp dport 2380 counter packets 0 bytes 0 accept
		ip saddr 10.77.0.2 ip daddr 10.77.0.0/24 iifname "wg0" tcp dport 26379 counter packets 0 bytes 0 accept
		ip saddr 10.77.0.10 ip daddr 10.77.0.0/24 iifname "wg0" tcp dport 6380 counter packets 0 bytes 0 accept
		ip saddr 10.77.0.2 ip daddr 10.77.0.0/24 iifname "wg0" tcp dport 6380 counter packets 0 bytes 0 accept
		ip saddr 10.77.0.0/24 iifname "wg0" tcp dport 8008 counter packets 0 bytes 0 accept
		ip saddr 10.77.0.0/24 iifname "wg0" tcp dport 5433 counter packets 0 bytes 0 accept
		ip saddr 10.77.0.10 ip daddr 10.77.0.11 iifname "wg0" tcp dport 2587 counter packets 0 bytes 0 accept
	}
	chain ufw-user-output {
	}
	chain ufw-user-forward {
	}
	chain ufw-user-logging-input {
	}
	chain ufw-user-logging-output {
	}
	chain ufw-user-logging-forward {
	}
	chain ufw-user-limit {
		limit rate 3/minute burst 5 packets counter packets 0 bytes 0 log prefix "[UFW LIMIT BLOCK] "
		counter packets 0 bytes 0 reject
	}
	chain ufw-user-limit-accept {
		counter packets 0 bytes 0 accept
	}
}
table ip6 filter {
	chain INPUT {
		type filter hook input priority filter; policy drop;
		counter packets 5 bytes 344 jump ufw6-before-logging-input
		counter packets 5 bytes 344 jump ufw6-before-input
		counter packets 2 bytes 128 jump ufw6-after-input
		counter packets 2 bytes 128 jump ufw6-after-logging-input
		counter packets 2 bytes 128 jump ufw6-reject-input
		counter packets 2 bytes 128 jump ufw6-track-input
	}
	chain OUTPUT {
		type filter hook output priority filter; policy accept;
		counter packets 4 bytes 288 jump ufw6-before-logging-output
		counter packets 4 bytes 288 jump ufw6-before-output
		counter packets 0 bytes 0 jump ufw6-after-output
		counter packets 0 bytes 0 jump ufw6-after-logging-output
		counter packets 0 bytes 0 jump ufw6-reject-output
		counter packets 0 bytes 0 jump ufw6-track-output
	}
	chain FORWARD {
		type filter hook forward priority filter; policy drop;
		counter packets 0 bytes 0 jump ufw6-before-logging-forward
		counter packets 0 bytes 0 jump ufw6-before-forward
		counter packets 0 bytes 0 jump ufw6-after-forward
		counter packets 0 bytes 0 jump ufw6-after-logging-forward
		counter packets 0 bytes 0 jump ufw6-reject-forward
		counter packets 0 bytes 0 jump ufw6-track-forward
	}
	chain ufw6-before-logging-input {
	}
	chain ufw6-before-logging-output {
	}
	chain ufw6-before-logging-forward {
	}
	chain ufw6-before-input {
		iifname "lo" counter packets 0 bytes 0 accept
		rt type 0 counter packets 0 bytes 0 drop
		ct state related,established counter packets 2 bytes 144 accept
		meta l4proto ipv6-icmp icmpv6 type echo-reply counter packets 0 bytes 0 accept
		ct state invalid counter packets 0 bytes 0 jump ufw6-logging-deny
		ct state invalid counter packets 0 bytes 0 drop
		meta l4proto ipv6-icmp icmpv6 type destination-unreachable counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type packet-too-big counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type time-exceeded counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type parameter-problem counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type echo-request counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type nd-router-solicit ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type nd-router-advert ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type nd-neighbor-solicit ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type nd-neighbor-advert ip6 hoplimit 255 counter packets 1 bytes 72 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-query counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-report counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-done counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 ip6 daddr fe80::/10 udp sport 547 udp dport 546 counter packets 0 bytes 0 accept
		ip6 daddr ff02::fb udp dport 5353 counter packets 0 bytes 0 accept
		ip6 daddr ff02::f udp dport 1900 counter packets 0 bytes 0 accept
		counter packets 2 bytes 128 jump ufw6-user-input
	}
	chain ufw6-before-output {
		oifname "lo" counter packets 0 bytes 0 accept
		rt type 0 counter packets 0 bytes 0 drop
		ct state related,established counter packets 3 bytes 216 accept
		meta l4proto ipv6-icmp icmpv6 type destination-unreachable counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type packet-too-big counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type time-exceeded counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type parameter-problem counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type echo-request counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type echo-reply counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type nd-router-solicit ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type nd-neighbor-advert ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type nd-neighbor-solicit ip6 hoplimit 255 counter packets 1 bytes 72 accept
		meta l4proto ipv6-icmp icmpv6 type nd-router-advert ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-query counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-report counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type mld-listener-done counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 255 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		ip6 saddr fe80::/10 meta l4proto ipv6-icmp xt match "icmp6" ip6 hoplimit 1 counter packets 0 bytes 0 accept
		counter packets 0 bytes 0 jump ufw6-user-output
	}
	chain ufw6-before-forward {
		rt type 0 counter packets 0 bytes 0 drop
		ct state related,established counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type destination-unreachable counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type packet-too-big counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type time-exceeded counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type parameter-problem counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type echo-request counter packets 0 bytes 0 accept
		meta l4proto ipv6-icmp icmpv6 type echo-reply counter packets 0 bytes 0 accept
		counter packets 0 bytes 0 jump ufw6-user-forward
	}
	chain ufw6-after-input {
		udp dport 137 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
		udp dport 138 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
		tcp dport 139 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
		tcp dport 445 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
		udp dport 546 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
		udp dport 547 counter packets 0 bytes 0 jump ufw6-skip-to-policy-input
	}
	chain ufw6-after-output {
	}
	chain ufw6-after-forward {
	}
	chain ufw6-after-logging-input {
		limit rate 3/minute burst 10 packets counter packets 2 bytes 128 log prefix "[UFW BLOCK] "
	}
	chain ufw6-after-logging-output {
	}
	chain ufw6-after-logging-forward {
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW BLOCK] "
	}
	chain ufw6-reject-input {
	}
	chain ufw6-reject-output {
	}
	chain ufw6-reject-forward {
	}
	chain ufw6-track-input {
	}
	chain ufw6-track-output {
		meta l4proto tcp ct state new counter packets 0 bytes 0 accept
		meta l4proto udp ct state new counter packets 0 bytes 0 accept
	}
	chain ufw6-track-forward {
	}
	chain ufw6-logging-deny {
		ct state invalid limit rate 3/minute burst 10 packets counter packets 0 bytes 0 return
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW BLOCK] "
	}
	chain ufw6-logging-allow {
		limit rate 3/minute burst 10 packets counter packets 0 bytes 0 log prefix "[UFW ALLOW] "
	}
	chain ufw6-skip-to-policy-input {
		counter packets 0 bytes 0 drop
	}
	chain ufw6-skip-to-policy-output {
		counter packets 0 bytes 0 accept
	}
	chain ufw6-skip-to-policy-forward {
		counter packets 0 bytes 0 drop
	}
	chain ufw6-user-input {
		tcp dport 22 counter packets 0 bytes 0 accept
		tcp dport 80 counter packets 0 bytes 0 accept
		tcp dport 443 counter packets 0 bytes 0 accept
	}
	chain ufw6-user-output {
	}
	chain ufw6-user-forward {
	}
	chain ufw6-user-logging-input {
	}
	chain ufw6-user-logging-output {
	}
	chain ufw6-user-logging-forward {
	}
	chain ufw6-user-limit {
		limit rate 3/minute burst 5 packets counter packets 0 bytes 0 log prefix "[UFW LIMIT BLOCK] "
		counter packets 0 bytes 0 reject
	}
	chain ufw6-user-limit-accept {
		counter packets 0 bytes 0 accept
	}
}

## wireguard

## containers
docker: not installed
containerd: not installed

## users (uid >= 100, with a shell or owning a service)
systemd-network 998 / /usr/sbin/nologin
systemd-timesync 996 / /usr/sbin/nologin
dhcpcd 100 /usr/lib/dhcpcd /bin/false
messagebus 101 /nonexistent /usr/sbin/nologin
syslog 102 /nonexistent /usr/sbin/nologin
systemd-resolve 991 / /usr/sbin/nologin
uuidd 103 /run/uuidd /usr/sbin/nologin
tss 104 /var/lib/tpm /bin/false
sshd 105 /run/sshd /usr/sbin/nologin
pollinate 106 /var/cache/pollinate /bin/false
tcpdump 107 /nonexistent /usr/sbin/nologin
landscape 108 /var/lib/landscape /usr/sbin/nologin
fwupd-refresh 990 /var/lib/fwupd /usr/sbin/nologin
polkitd 989 / /usr/sbin/nologin
postgres 110 /var/lib/postgresql /bin/bash
runner 1002 /home/runner /bin/bash

## ssh authorized_keys (type and comment only)
sshd AuthorizedKeysFile: .ssh/authorized_keys .ssh/authorized_keys2
/root/.ssh/authorized_keys: ssh-ed25519 ecg-contabo-vps-admin-2026-05-01
/home/runner/.ssh/authorized_keys: ssh-ed25519 ecg-contabo-vps-admin-2026-05-01

## directories
/opt
/srv
/var/lib/portico-cutover-evidence
  root:root 500 dir /var/lib/portico-cutover-evidence/20260803T032018Z
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T032018Z/CUTOVER_COMPLETE
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T032018Z/EVIDENCE.sha256
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T032018Z/cutover-proof.txt
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T032018Z/hosted-migrate.log
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T032018Z/portico.dump.sha256
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T032018Z/signing-continuity.txt
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T032018Z/source.manifest
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T032018Z/target-migrated-schema.txt
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T032018Z/target-restored.manifest
  root:root 500 dir /var/lib/portico-cutover-evidence/20260803T035208Z
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T035208Z/CUTOVER_COMPLETE
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T035208Z/EVIDENCE.sha256
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T035208Z/cutover-proof.txt
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T035208Z/hosted-migrate.log
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T035208Z/portico.dump.sha256
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T035208Z/signing-continuity.txt
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T035208Z/source.manifest
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T035208Z/target-migrated-schema.txt
  root:root 400 file /var/lib/portico-cutover-evidence/20260803T035208Z/target-restored.manifest
/var/lib/portico-ha-bootstrap
  root:root 644 file /var/lib/portico-ha-bootstrap/app.done
  root:root 644 file /var/lib/portico-ha-bootstrap/etcd.done
  root:root 644 file /var/lib/portico-ha-bootstrap/import.done
  root:root 644 file /var/lib/portico-ha-bootstrap/patroni.done
  root:root 644 file /var/lib/portico-ha-bootstrap/preflight.done
  root:root 644 file /var/lib/portico-ha-bootstrap/redis.done
  root:root 644 file /var/lib/portico-ha-bootstrap/wireguard.done
/var/lib/portico-origin-controller
  root:root 700 dir /var/lib/portico-origin-controller/evidence
  root:root 600 file /var/lib/portico-origin-controller/evidence/generation-3-attempt-1787920283121286144-operation.json
  root:root 600 file /var/lib/portico-origin-controller/state.json
/var/lib/portico-release-evidence
  root:root 700 dir /var/lib/portico-release-evidence/0.1.0-ha.20260831.35
  root:root 400 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.35/direct-dns-mac-key.pre-wipe
  root:root 600 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.35/ha.env.candidate
  root:root 600 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.35/ha.env.pre-36
  root:root 600 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.35/ha.env.pre-wipe
  root:root 600 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.35/hosted.env.pre-36
  root:root 600 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.35/hosted.env.pre-wipe
  root:root 400 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.35/input-checksums.txt
  root:root 400 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.35/service-hmac-key.pre-wipe
  root:root 700 dir /var/lib/portico-release-evidence/0.1.0-ha.20260831.36
  root:root 600 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.36/active-env.sha256
  root:root 600 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.36/images.lock
  root:root 600 file /var/lib/portico-release-evidence/0.1.0-ha.20260831.36/images.lock.sha256
/etc/postgresql
  postgres:postgres 755 dir /etc/postgresql/16
  postgres:postgres 755 dir /etc/postgresql/16/main

## packages of interest
postgresql 16+257build1.1

## toolchains (build and test runner)
go: go version go1.26.2 linux/amd64 (/usr/local/bin/go)
node: v22.22.2 (/usr/local/bin/node)
npm: 10.9.7 (/usr/local/bin/npm)
python3: Python 3.12.3 (/usr/bin/python3)
git: git version 2.43.0 (/usr/bin/git)
rsync: rsync  version 3.2.7  protocol version 31 (/usr/bin/rsync)
gcc: gcc (Ubuntu 13.3.0-6ubuntu2~24.04.1) 13.3.0 (/usr/bin/gcc)
make: GNU Make 4.3 (/usr/bin/make)
psql: psql (PostgreSQL) 16.15 (Ubuntu 16.15-0ubuntu0.24.04.1) (/usr/bin/psql)
```

</details>
