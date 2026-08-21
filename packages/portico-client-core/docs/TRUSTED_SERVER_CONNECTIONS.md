# Trusted server connections

Hosted Services is Portico's account, membership, and discovery control plane.
It is not part of the media data plane after a client has attached to a server.

## Durable record

Platforms implement `TrustedServerConnectionAdapter` with protected storage.
Records are keyed by both Hosted account ID and server ID because one Portico
Account can access many independent servers. Each record contains:

- the pinned server ID and public-key fingerprint;
- the current and previous successfully verified HTTPS route hints;
- the last successful connection time; and
- the server-native access/refresh credential family.

Hosted account access and refresh tokens must never be stored in this record.
`save` is an atomic adapter boundary. If a platform writes more than one secure
store, it must compensate a partial write before rejecting. If that compensation
is incomplete, it throws `TrustedServerDurabilityUncertainError` with the
underlying failure details rather than an ordinary error.

An adapter that intentionally keeps the reusable server credential family only
for the current process exposes `persistencePolicy: "reauthorize-on-start"`.
Its separate `durability()` value describes whether restart metadata was saved
successfully. A healthy reauthorization policy is therefore honest without
being presented as a storage failure or user-facing warning.

## Initial attachment

`connectResilientHostedServer()` obtains a signed route document and short-lived
bootstrap token from Hosted Services. After verifying the route's public health
identity, Client Core exchanges that bootstrap token at
`POST /api/auth/portico/sessions`. The server introspects Hosted Services once
and returns its own native credential family. Ordinary API calls and refreshes
then go directly to the server.

## Later launches

Cached current/previous route probes run at the same time as fresh Hosted
discovery. A cached route is only a connection hint: its public health response
must match the pinned server ID and fingerprint before any server credential is
sent. A Hosted timeout or outage therefore cannot block a previously attached
device from opening its server.

Use `connectTrustedServerRecord()` when the Hosted server list itself is
unavailable. This is particularly useful while restoring an app shell from
protected local state.

## Publication transaction

Both connection entry points verify the candidate completely before exposing
it to the application. The platform's `stageCandidate` implementation first
installs a synchronous generation/write fence and drains the old runtime while
the candidate remains isolated. It returns two operations:

- `publish`, which makes the new scope and UI visible only after active
  credential publication and durable-save resolution; and
- `rollback(mode)`, which either restores the previous runtime or leaves all
  authenticated runtime state unavailable when `mode` is `fail-closed`.

If `stageCandidate` itself rejects after changing runtime state, the platform
implementation must restore that state or fail-closed before rejecting; Client
Core cannot invoke a rollback handle that was never returned.

The exact order is: verify route and candidate identity, stage the runtime
fence, publish the candidate credential to `SessionStore`, resolve the atomic
durable save, publish the candidate runtime, and finally publish convenience
state such as the last-connected preference. A final `/api/auth/me` response
must match the selected authority, account, server, and profile. Its
authorization revision may be newer than the revision issued with the native
credential and becomes the authoritative cache/runtime revision.

A `SessionStore.set` failure is fatal. Only an ordinary atomic durable-save
failure or an adapter's explicit `memory-only` health result may return
`memory-only`; an uncertain durable error is fatal. The independent persistence
policy records whether a healthy fresh process restores a saved session or
reauthorizes. Any failed rollback attempts every active and candidate-durable
deletion before requesting `rollback("fail-closed")`.

Callers implement latest-choice-wins by aborting the preceding choice, awaiting
its transaction cleanup, and only then starting the next choice with a fresh
signal. This serialization prevents an older rollback from overwriting a newer
credential publication. Client Core checks cancellation throughout discovery,
probing, bootstrap, validation, persistence, and immediately after runtime
publication.

## Failure semantics

Transport errors, DNS/TLS availability failures, timeouts, generic 401s,
expired access tokens, rate limits, and 5xx responses never erase a record.
Only explicit server-scoped revocation codes do:

- `membership_inactive`
- `server_not_found`
- `server_session_revoked`
- `account_disabled`
- `device_not_allowed`

Losing one server never signs the user out of their Portico Account or removes
other server records.
