# Administration: people, diagnostics and connectivity

Companion to `admin-access.openapi.yaml`. The OpenAPI document is the contract;
this page explains the rules behind it and says where each one is enforced.

## Access tiers

Three tiers, ordered `owner` > `admin` > `member`. They are defined once, in
`internal/identity/access_tier.go`, and asked through a single predicate:

```go
identity.Grants(role, needed)   // does this role satisfy this requirement?
identity.Administrative(role)   // owner or admin
identity.ManagesTier(actor, target)
```

Nothing else in the server compares role strings. `Grants` ranks unknown roles
below `member`, so a future or corrupted value can never out-rank a real one.

| Tier | May |
| --- | --- |
| `owner` | Everything. Exactly one account per server. |
| `admin` | Libraries, metadata, invitations, member limits, device trust, diagnostics reads and the debug window, connectivity **status**. |
| `member` | Nothing administrative. |

An `admin` may **not**: change server security or connectivity **policy**, take
backups, create or revoke API keys, export the diagnostics bundle, or create,
promote, demote or edit another `admin` or the owner.

Two places carry the tier into the running server:

- `internal/httpapi/administration.go` — `tierAuthorityTx` is the admin-tier
  counterpart of `ownerAuthorityTx`. It re-reads the account's **live**
  membership row inside the transaction that does the work, so a token minted
  while an account was an administrator stops working the moment the account is
  demoted. `d.administrator(r)` is the request-level helper; owner-only routes
  keep `d.owner(r)`.
- `internal/identity/direct_policy.go` — `DirectAccess` grants an administrator
  every library, using `Administrative` rather than a second `== "owner"` test.

Changing a tier bumps the account epoch and revokes its sessions, so a demotion
takes effect immediately rather than at the next token expiry.

## Invitations

Invitations are addressed by **account email**, because a direct server has no
mail transport of its own: the address identifies the invitee, and the code is
delivered out of band. One pending invitation per address; at most 200 pending.

The code is returned exactly once, in the `201` from
`POST /v1/admin/access/invitations`. Only its SHA-256 digest is stored. A replay
of the same `operationId` returns the stored document, which never carries the
code.

`POST /v1/access/invitations/accept` is unauthenticated — the invitee has no
account yet — and is rate-limited per peer with the same limiter the sign-in
routes use. Accepting creates the account with the invitation's tier and library
allow list; the password must satisfy the same policy the owner's own member
creation uses (`identity.ValidDirectPassword`).

## Per-member limits

One envelope per account, replaced whole so a client never observes half a
policy. An account with no stored envelope reads as unrestricted at revision 1,
so a client can write against that revision without a create step.

| Limit | Enforced in |
| --- | --- |
| `maxStreams` | `httpapi.admitPlayback`, before a playback lease exists (`POST /v1/playback/sessions` including channel and queue-entry starts, queue transitions, `POST /v2/playback/occurrences`). Counts live legacy sessions and live `playback_v1_sessions` rows; a v1 session's legacy presentation row is excluded, so one stream counts once. |
| `remoteBitrateKbps` | `httpapi.admitPlayback` returns it as `Decision.MaxVideoBitrateBPS` when the request is remote. Every v1 start path admits through `playbackv1.Caller.Admit` (item, channel, queue-entry starts and queue transitions), and a v1 PATCH that re-plans (quality, version, audio or part change) re-admits the same way, so the cap reaches the transcode decision on each; the playback-options preview reads the same cap through `v1Reach` without admitting. The server-wide ceiling is applied separately through `RegistryDeliverySettings.Delivery()`; the delivery policy clamps against both, so the lower wins. |
| `maxContentRating` / `allowUnrated` | `httpapi.admitPlayback` per title, and catalog visibility through `access.Enforcer.VisibleItem` (one item) and `access.Enforcer.VisibilityClause` (a SQL predicate for the browse projection). |
| `tagPolicy.deniedLabels` | Same pair. Matched case-insensitively against the catalog's `label` and `tag` attributes. |
| `schedule` | `httpapi.admitSession` (every sign-in: `POST /v1/sessions`, `POST /v1/direct/sign-in`, `POST /v1/auth/two-factor/challenge`, `POST /v1/auth/register`, `POST /v1/auth/refresh`), `httpapi.admitPlayback` and `httpapi.admitChannel`, evaluated in the member's own IANA timezone. |
| `channelPolicy` | `httpapi.admitChannel`, on v1 channel starts only (`POST /v1/playback/sessions` with `channelId`, matched against the canonical v1 id `live:<source>:<ch>` / `library:<ch>`). Writes accept canonical ids only. |
| Device trust | `httpapi.admitPlayback`, when `deviceApprovalRequired` is on. |

The content rating ladder is published: `TV-Y`, `TV-Y7`, `G`, `TV-G`, `PG`,
`TV-PG`, `PG-13`, `TV-14`, `R`, `TV-MA`, `NC-17`. A rating the server does not
recognise is **unrated**, and unrated titles pass only when `allowUnrated` is
true — silently hiding a title whose rating the server cannot read is worse than
showing it, so that choice is the owner's.

An administrator is not exempt by accident. Limits are per account and an owner
or admin simply has none by default; a stored envelope on an administrative
account is applied like any other.

## Devices

Devices are recorded at playback admission from three optional request headers:
`X-Portico-Device-Id`, `X-Portico-Device-Name`, `X-Portico-Device-Platform`. An
unknown identifier is recorded as `pending`, so an owner who turns approval on
sees real devices to approve rather than an empty list. Blocking a device also
stops its account's live playback leases.

**Merge note (identity workstream).** This area needs a device inventory in
order to publish trust and an approval policy, and the identity workstream owns
the canonical device record. Until that record lands, `access_devices` is the
minimal inventory. When the identity workstream's `devices` table arrives, keep
`access_devices`' `trust`, `revision` and first/last-seen columns, read `name`,
`platform` and ownership from that table instead — replace the `SELECT` in
`access.Store.Devices` and the upsert in `access.Store.Observe` with a join —
and drop the duplicated columns. No wire shape has to change.

## API keys

Owner only. A key is a second credential for the whole server, so it sits with
the other security controls rather than with the people page.

Scopes, narrowest first: `read-only`, `playback`, `library-management`, `full`.
`access.ScopeAllows(scope, needed)` is the predicate; operation classes use the
same names. A key never exceeds its account's tier.

A key authenticates like a bearer token — `Authorization: Bearer pk_…` — and is
resolved in `httpapi.bearerPrincipal` / `Dependencies.principal`. It produces a
principal with authority `api-key`, which is a **distinct principal type**: every
helper that requires a live session family rejects it unless it names that
authority explicitly. Every successful authentication records `lastUsedAt`.

## Message logs

The standard library's `log` package stays the writing interface for the whole
server. `servicelog.Recorder` is installed as `log.SetOutput`'s destination (teed
to stderr, which a container operator still needs), so every existing
`log.Printf` lands in the message log with no call site changing.

A line may carry optional prefixes: `warn: …` sets the level, `[playback] …`
sets the category, and both may appear in either order. Anything else is an
`info` line in the `server` category. The standard date/time prefix is dropped in
favour of the recorder's own timestamp.

Storage is two-layer:

- A bounded in-memory ring (5000 records) answers `GET /v1/admin/logs` and the
  SSE tail. Paging is by `sequence`, which is monotonic for the life of the
  process.
- Rotating files under `<state>/logs/` (`messages.log`, `messages.1.log`, …,
  4 MiB each, 4 kept) survive a restart and travel in the diagnostics bundle.

Rotation closes the active file before moving it and removes the destination
first. Windows refuses to rename or remove an open file, and refuses a rename
onto an existing path; doing both unconditionally keeps one code path rather
than a per-OS branch only one target would ever exercise.

Per-category retention (`server`, `playback`, `scan`, `network`, `client`) is a
settings row. A category with no entry keeps whatever the ring holds.

The debug window is **runtime state, not a settings change**: it raises the
effective level to `debug` for up to 240 minutes and reverts on its own, without
touching the saved `logLevel` or moving the settings revision. Its expiry is
persisted, so a restart inside the window keeps debugging rather than silently
going quiet — which matters, because the restart is often the thing being
debugged.

## Client log uploads

Any authenticated principal, including an API key, may upload. The body is
bounded twice — by the request reader and by the store — at 256 KiB; the account
and profile come from the credential, never from the body. The newest 500
uploads are kept regardless of the retention sweep, so a misbehaving client
cannot fill the state directory.

## Diagnostics bundle

Owner only, because it carries the settings document. A zip of `manifest.json`,
`settings.json`, `system-report.json`, `connectivity.json`, `messages.json` and
`logs/`. Settings have every field the registry marks `secret` removed — the
bundle writer does not guess which those are; it asks the registry.

The archive stops at 16 MiB. A support archive that grows without a ceiling is a
denial-of-service against the server's own disk and against the person trying to
send it.

## Connectivity

Policy lives in the settings registry — one authority for every server setting —
and `/v1/admin/connectivity/policy` is its typed projection. `revision` is the
settings document revision, so this page and the general settings page conflict
correctly instead of silently overwriting each other.

| Setting | Effect |
| --- | --- |
| `remoteSignInPolicy` | `allow` / `owner-only` / `off`, applied in `httpapi.admitRemoteSignIn` after a sign-in succeeds. A refused session is revoked before the response. Sessions already signed in are not disturbed. |
| `remoteBitrateLimitKbps` | Server-wide ceiling, exposed as `playback.DeliveryConfiguration.MaxVideoBitrateBPS` through `RegistryDeliverySettings.Delivery()`. |
| `secureConnectionsPolicy` | `required` / `preferred` / `lan-plain-allowed`. Reported in the status report; `required` without a ready certificate raises a warning. |
| `lanNetworks` | CIDR list, stored masked. It decides what "remote" means for the remote sign-in policy and the remote bitrate cap. |
| `accessUrls` | Extra URLs clients may use, such as a reverse proxy address. |
| `lanDiscoveryEnabled` | mDNS/Bonjour advertisement, applied immediately on save. |

### LAN discovery

`connectivity.Advertiser` is portable: it uses only `net`'s UDP multicast
support. It sends unsolicited DNS-SD announcements for `_portico._tcp.local.`
every minute, and — when it can also bind the multicast group for reading —
answers queries that name the service.

A host that refuses either operation (a container without multicast, a
restricted Windows service account, a network with IGMP filtered) is not a
failure. The advertiser reports the reduced state and the status report shows it:

| `state` | Meaning |
| --- | --- |
| `stopped` | The owner turned it off. |
| `advertising` | Announcing and answering queries. |
| `announcing` | Announcing only; this host would not let the multicast group be read, so query-only browsers may not see the server. |
| `degraded` | The last announcement could not be sent; `detail` says why. |
| `unavailable` | This host refused a multicast socket at all. `supported` is false. |

### Status report

`GET /v1/admin/connectivity/status` combines the policy with what the host can
observe: interfaces from `net.Interfaces` (portable on every target; a host that
refuses the enumeration yields an empty list rather than an error), each
address classified as `public`, `lan` or `loopback` against the configured LAN
networks, the certificate state from `internal/networking`, and the real
discovery state. `warnings` names policy this host cannot satisfy:
`secure_connections_required_without_tls`,
`remote_sign_in_without_public_address`,
`lan_discovery_unsupported_on_this_host`.

## Concurrency and retries

Every mutation carries `expectedRevision` and `operationId`.

- `expectedRevision` is the revision of the document being changed. A mismatch
  is `409 administration_conflict` with `error.currentRevision`.
- `operationId` is an idempotency key of 8–128 characters. The same key with the
  same body replays the stored response; the same key with a different body is
  `400 invalid_request` naming `operationId`. Receipts are a retry aid, not
  history: the newest 2000 are kept.

Lists page by opaque cursor (sort key plus identifier), never by offset, so a
concurrent insert cannot cause a row to be skipped.
