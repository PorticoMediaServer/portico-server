# Hosted services

Hosted is the small service a home server can't be: Portico Accounts, sign-in across servers, a directory of each account's servers, claim, names and certificates for remote access, and the public web app. It's designed to stay cheap at scale. Day to day, clients talk to the user's own server; Hosted sees sign-in, claim, occasional directory refreshes and certificate issuance, with no periodic chatter.

Code: `portico-internal/hosted-services` (Go modules `hosted`, `dns`, and a copy of `apikit`). Deployment: [Operations](08%20Operations.md) and `deployment/hosted/TOPOLOGY.md`.

## Hosted API (`hosted/cmd/hosted`)

Served at web.getportico.tv (`/v1/*` and SSO callbacks) behind Cloudflare and Caddy, backed by PostgreSQL 17 (`internal/postgres`).

| Package | Role |
|---|---|
| `identity`, `sessions`, `security` | Portico Accounts: registration with a verified contact, password (with a cool-off on failures), TOTP 2FA, recovery, device-bound sessions. Account security details: `hosted/ACCOUNT_SECURITY.md` |
| `oidclogin` | Sign in with Apple and Google (web and native; `api/NATIVE_SSO.md`) |
| `device` | TV code sign-in: the TV shows a code, a signed-in phone or computer approves it |
| `directory` | The account's servers and memberships. **The user's server is authoritative** for membership and pushes changes; Hosted keeps an index for discovery |
| `claims` | Claiming a server into an account, and the signed binding the server keeps |
| `directroute`, `certificates` | Each claimed server's name (`c-<label>.<namespace>.direct.getportico.tv`), its published addresses, and ACME DNS-01 certificates from Google Public CA |
| `mailoutbox` | A durable outbox for the only email Hosted sends: contact verification, password reset, server invitations |
| `appstore` | Verifies App Store Server Notifications |
| `wake` | Delivers a wake to a server's registered address when an event needs the server (event-driven, no polling) |
| `egress` | Confines Hosted's calls to server-advertised endpoints (public HTTPS origins only) |
| `contract`, `httpapi` | The route registry (apikit) and HTTP layer; `cmd/apigen` generates `api/hosted.openapi.json` and the TypeScript contract under `contracts/src/hosted` |

**Trust.** Hosted signs what servers and clients must trust (claim bindings, directory answers) with keys certified by an offline **root key**. Release builds compile the root's public key in (`deployment/hosted/root-public-key.pem`), and so does the server (`hostedtrust`). Key custody: `deployment/hosted/KEYS.md`.

**Economy rules.** Every Hosted call from a client or server uses random delay and exponential backoff. Clients learn about new servers through a cheap version check a few times a day while open, plus a Refresh Servers button. Servers push real changes (address changes, revocations) when they happen, and presence is inferred from traffic. Email is a cost center, so there are no alert emails; alerts are in-app.

## Hosted web host (`hosted/cmd/web`)

Serves the Hosted-only build of the web app at web.getportico.tv, and the Apple app-site-association file (for app links on `tv.getportico.portico.ios`).

## DNS (`dns/cmd/portico-dns`)

Two nodes (ns1 and ns2) answer for `direct.getportico.tv` (each server's published addresses) and `acme.getportico.tv` (DNS-01 challenges):

- `dnsdist` on port 53 routes the `direct` zone to `portico-dns` and the `acme` zone to BIND; everything else is refused.
- `portico-dns` reads the address feed from Hosted over authenticated HTTPS, caches aggressively, and reaches back only when it must.
- BIND on ns1 takes RFC 2136 updates from Hosted over WireGuard and transfers the zone to ns2.

Setup and restore: `dns/deploy/`.

## Cast receiver

One Google Cast receiver application serves every Portico server. The page is built into each server at `/receiver/cast/` and published unchanged at cast.getportico.tv (`cast-receiver/README.md`). It never calls Hosted.

## What Hosted is not

- Not a relay. Remote access is port forwarding or none (free); a paid relay is a possible future feature and the only paid one.
- Not a metadata service. Servers fetch metadata from providers directly.
- Not a store of anyone's library, watch history or media.
