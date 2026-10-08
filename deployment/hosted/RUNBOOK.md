# Hosted operations: current deployment contract

**Where things run:** see [TOPOLOGY.md](TOPOLOGY.md), which lists the five hosts, their services, ports, firewalls and WireGuard, the shared SSH key and aliases, and how to deploy each host. In short:
- **Hosted (`portico-core-us-1`):**
  - the Hosted API and web-host, as systemd units;
  - PostgreSQL 17 on loopback (database `portico_hosted`);
  - Caddy behind Cloudflare.
- **DNS nodes (`portico-dns-primary-1` = ns1, `portico-dns-de-1` = ns2):**
  - dnsdist;
  - `portico-dns`;
  - BIND for the ACME zone, reached over WireGuard.
- **No Docker anywhere.**

Deploy with `scripts/deploy-hosted-release.py` and `scripts/deploy-dns-release.py --node ns2|ns1`.

This replaces the old `portico-cloud`/Patroni/etcd/Redis runbooks; that stack
was removed from every host on 23 September 2026. Hosted is a PostgreSQL-backed control plane behind Caddy. The
server binary listens only on loopback; Caddy terminates public HTTPS. DNS
nodes read the authenticated incremental address feed. Hosted does not relay
media or playback bytes. No legacy environment-variable signing key is read.

Provision PostgreSQL, Caddy and the Hosted and DNS service users first. Run the
offline root/signing-key ceremony in [KEYS.md](KEYS.md), transfer only the root
PUBLIC key to the build host, and place the certified signing/sealing files on
the Hosted host with service-user-only permissions. Build with
`scripts/build-hosted-release.sh`; it rejects a missing official root file.
Use `--key-file`, `--database-url-file`, and explicit `--claim-audience` and
`--origins` on the Hosted unit. Keep every secret in protected files rather than
command-line flags or the image. `--listen` is a loopback IP only. The web
asset server is a separate loopback process; its `PORTICO_APPLE_TEAM_ID` must
be set to serve the app-site-association file, otherwise it answers 404.

The operational values are the current code contract: a signed policy lease is
90 days; a reachable server checks in weekly and an unreachable one daily,
with immediate server-to-Hosted events for address, revocation, remote-access,
rename and shutdown changes. There is no other periodic presence traffic:
`GET /v1/servers` reports `presence.lastSeenAt` (the last check-in or event),
never a live online flag; apps ask the server itself whether it is up. A member
removal bumps the policy and wakes the server, whose check-in then returns the
rotated member label. Hosted derives active/quiet/dormant status from
traffic and leases (retirement revokes every membership, so retired servers are not listed). Remote address documents contain direct routes; DNS names
under `direct.getportico.tv` use private member labels. A current LAN route
does not authorize public WAN access. No Hosted media relay exists before a
separate paid-relay decision; CGNAT observations are aggregate counters only.

Readiness includes PostgreSQL, the notification listener and the signing
certificate (it fails once the certificate has expired). Scrape the private
metrics listener (`--metrics-listen`, loopback) for queue depth/age, DNS feed
head, CT observations, pool wait, abuse rejections and
`portico_signing_certificate_expiry_timestamp_seconds`. Alert on stalled
wake/certificate/mail queues, missing listener, unknown CT leaf, certificate
expiry within 30 days and backup failure. `POST /ct-observations` needs the
bearer token from `--metrics-token-file` and is off without it.

Portico sends no push notifications. Owner alerts, security events, new
sign-ins, export completion and the operator's key-expiry warning are in-app
notifications (`GET /v1/account/notifications`). Email goes only to verify an
address when setting up an account, for invitations, and for password reset.

Use [BACKUPS.md](BACKUPS.md) for the encrypted base/WAL backup, key-material
backup and restore drill. A restore changes data authority: pause public
traffic, restore into an isolated host, run the verification script and DNS feed
rebuild, check signing/sealing custody and schema version, then resume service.
Never generate a new key to make a missing-key startup error disappear. A
release binary with no official root cannot boot; a development root cannot
serve the production origin. Provider credentials (ACME, mail, Apple and Google
identity) are configured separately and are never embedded in Git.
