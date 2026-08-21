# Native authentication and connection conformance

The machine-readable acceptance fixture is published with Client Core at
`@portico/client-core/fixtures/native-auth-connection.v1.json`. Native client
repositories should run the same state transitions against their platform
credential and networking adapters. The fixture describes protocol ownership;
it does not prescribe Keychain, Keystore, account-container, or navigation UI.

## Credential ownership

- A local native session is created, refreshed, and revoked by the selected
  Portico server. Client Core persists successful password, Quick Connect, and
  TV grant exchanges through `CredentialAdapter` before returning them.
- A Hosted account native session is created, refreshed, and revoked by Hosted
  Services. `HostedServicesClient` keeps those scoped secrets in request bodies;
  the platform account container owns their durable storage.
- Hosted Services issues only a short-lived, one-time `bootstrapAccessToken`
  for the selected server. The client presents it once to that verified server,
  then persists the returned server-native `accessToken` and `refreshToken`.
  Automatic refresh always goes directly to the selected server's native-session
  route; Hosted Services never issues or rotates a durable server refresh family.

Do not store a Hosted account refresh token in `LocalServerSession`. Hosted
account credentials and selected-server credentials have different audiences
and revocation boundaries.

## Durable state transitions

`CredentialAdapter.save` must be atomic from the platform's perspective. Client
Core awaits it before releasing any request that depends on a rotated token. A
failed save clears the in-memory/session-store copy and asks the adapter to
clear, avoiding a split-brain state where memory uses a token that cannot be
restored after process death.

Credential publication is also a required part of the transaction. If
`SessionStore.set` rejects, Client Core attempts every configured credential
deletion and returns `credential_persistence_failed` only when cleanup is fully
verified; it never reports a live memory-only success. If any compensating
deletion fails, Client Core instead throws
`CredentialCleanupUncertainError` with `failClosed: true` and the complete
`rollbackFailures`. Platforms must publish their restart cleanup quarantine,
deny restore, and keep authenticated runtime/UI unavailable until deletion is
independently verified.

Only one refresh is in flight per client instance. Concurrent `401` responses
join that operation, then retry once with the durable replacement. A second
`401` is terminal for that request and cannot recursively refresh.

Refresh `4xx` responses other than `408` and `429` are terminal and clear the
active selected-server credential. Network failures, `408`, `429`, and `5xx`
responses preserve it for a later attempt. Server-side refresh-family replay
protection remains authoritative.

## Hosted server selection

`connectHostedServer` performs these steps in order:

1. Verify the Hosted Ed25519 route document and validity window.
2. Probe LAN candidates before public direct candidates.
3. Require the health response to match both server ID and persistent public-key
   fingerprint.
4. Obtain a short-lived Hosted bootstrap credential.
5. Exchange it for a server-native credential at `/api/auth/portico/sessions`.
6. Require the issued credential and the final authenticated `/api/auth/me` to
   match the selected authority, account, server, and profile. The final
   `/api/auth/me` authorization revision is authoritative and may advance after
   credential issuance.
7. Check the Product Contract API v1 marker after authentication.
8. Fence and drain the previous application runtime through `stageCandidate`.
9. Publish the candidate credential to `SessionStore`, resolve its durable
   write, and only then call the staged candidate's `publish` operation.

`connectResilientHostedServer` performs verification in an isolated temporary
store. The application-visible runtime is fenced before candidate credentials
are published, so old UI and mutations cannot execute under the new identity
while durability is unresolved. If verification, credential publication, or
runtime publication fails, Client Core restores the previous active and durable
state. If restoration cannot be trusted, it attempts every active and durable
deletion and directs the staged runtime to remain fail-closed.

An ordinary `TrustedServerConnectionAdapter.save` error certifies that its
write was atomic and left the prior durable value intact. That narrow case may
return a live `memory-only` result with `durabilityError`. An adapter that may
have partially written, or whose own compensation failed, must throw
`TrustedServerDurabilityUncertainError`. Client Core treats that as fatal,
restores or removes durable state, and never publishes the candidate runtime.

New native shells should prefer `connectResilientHostedServer`. It preserves
the same signed-route checks, exchanges the Hosted bootstrap token for a
server-native credential at `/api/auth/portico/sessions`, and races fresh
Hosted discovery against the account/server-scoped record described in
`TRUSTED_SERVER_CONNECTIONS.md`.

## App restart and switching

On app startup, construct `PorticoClient` with a `CredentialAdapter.load`
implementation. The first request single-flights the load and restores the
selected route and bearer credential. Account switching is platform-owned: the
Hosted client's `accessToken` can be a value provider backed by the active
account container. Server switching is handled by
`connectResilientHostedServer`; never
overwrite the durable selected server before bootstrap completes.
Each server/profile choice must own an `AbortController`. Abort the preceding
choice, await that predecessor's transaction cleanup, and then start the newer
one with its own signal. Client Core checks the signal after every asynchronous
boundary, including immediately after staged runtime publication. Serializing
the aborted predecessor's rollback prevents it from restoring old credentials
over a newer successful choice.

Infrastructure-only validation remains outside deterministic fixtures: real
Bonjour delivery, OS secure-storage access controls, TLS trust-store behavior,
NAT/firewall reachability, background suspension, and actual clock skew must be
tested in each platform app and on real networks.
