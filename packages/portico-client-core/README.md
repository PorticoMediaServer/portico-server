# @portico/client-core

Platform-neutral TypeScript client core for Portico.

This package is the shared product/API layer for Portico Web and future TypeScript-capable clients. The Portico Server and Portico Cloud OpenAPI documents own every wire shape; this package adds ergonomic request, playback, and connection-policy wrappers without maintaining a second DTO model.

## What Belongs Here

- API and OpenAPI types.
- Portico Server and Portico Cloud request clients.
- Normalized `ApiError` failures with request correlation and retry timing.
- Browser-default, React Native-injectable Server-Sent Event streaming.
- Resource URL construction for artwork, downloads, DVR streams, Live TV streams, logs, Watch With Friends, and playback receivers.
- Mutation tag mapping for cache invalidation.
- Viewer-scoped subscription, invalidation, query single-flight, bounded cache,
  workload-priority, circuit-breaker, and lifecycle coordination.
- Hosted Services-server route selection and identity verification helpers.
- LAN discovery record normalization, trust evaluation, stale-record handling, and native provider contracts.
- Library filtering/sorting/category helpers.
- Portable browse expression parsing, capability validation, editor compilation,
  query chips, workspace normalization, and Saved-view serialization.
- Selected-library/pivot capability resolution, hierarchy/artwork semantics,
  and versioned action-ID command resolution.
- Contract-resolved `MediaViewModel` render DTOs for shared card, row, and
  detail components, including future-kind and native artwork geometry fallback.
- Framework-neutral destination vocabulary, canonical Portico link parsing and
  serialization, route identity policy, and bounded viewer-fenced restoration.
- Playback response normalization, labels, quality/subtitle helpers, segment helpers, and volume normalization helpers.
- Apple AVKit capability/profile negotiation and transport-neutral playback conformance fixtures.
- Web display preference normalization and settings command-center helpers.

## What Does Not Belong Here

- React DOM components.
- React Native components.
- Native player shells.
- Secure storage implementations.
- tvOS focus controllers.
- Visual app navigation, route mounting, focus, and motion.
- Router-specific parameter lists, navigation objects, or screen components.
- Hosted media relays, iframe bridges, tunnels, or generic proxy transports. Hosted clients establish a verified direct route to the selected server.

## Basic Use

```ts
import {
  createMemorySessionStore,
  createPorticoClient,
  browserPlaybackClientProfile
} from "@portico/client-core";

const sessionStore = createMemorySessionStore();

const client = createPorticoClient({
  apiBaseUrl: "http://127.0.0.1:32500",
  sessionStore,
  credentialAdapter: {
    load: () => secureCredentials.load(),
    save: (session) => secureCredentials.save(session),
    clear: () => secureCredentials.clear()
  },
  csrfToken: "1",
  playbackClientProfile: browserPlaybackClientProfile,
  onMutation(tags) {
    // Invalidate local query/cache state for these tags.
  }
});

const auth = await client.createNativeSession({
  login: "owner",
  password: "secret",
  installationId: "app-generated-stable-installation-id",
  deviceName: "Justin's iPhone",
  app: "Portico",
  platform: "iOS"
});
// Successful native, Quick Connect, and TV grant credentials are durable here.
const home = await client.home();
```

Every request receives an `X-Request-ID` unless the caller already supplied
one. Native shells may inject a request ID generator with `requestId`. Failed
responses throw `ApiError`; use its stable `status`, `code`, `type`, `title`,
`detail`, `details`, `requestId`, `responseHeaders`, `retryAfter`, `retryAt`,
and `retryAfterMs` fields rather than parsing an error message.

## Credential lifecycle

Authenticated server requests automatically perform one refresh-and-retry when
the server returns `401`. Concurrent failures share a single refresh request,
and the retried requests are not released until rotated access and refresh
tokens have been persisted. Provide a platform-appropriate asynchronous
credential adapter rather than putting platform storage inside Client Core:

```ts
const client = createPorticoClient({
  credentialAdapter: {
    load: () => secureCredentials.load(),
    save: (session) => secureCredentials.save(session),
    clear: () => secureCredentials.clear()
  }
});
```

`load` is optional when the app supplies a synchronous `sessionStore`. Keychain,
Keystore, encrypted browser storage, and account-container policy remain owned
by the platform app. Refresh and revoke endpoints are never recursively retried.
Terminal refresh rejection clears both the session store and credential adapter;
transient server or network failures leave the session available for a later
attempt.

## Limited-input device authorization

`createHostedServicesClient` exposes the generic Hosted device flow without
assuming a specific television platform:

```ts
const authorization = await hosted.createDeviceAuthorizationSession({
  deviceName: "Living Room",
  platform: "limited-tv",
  appVersion: "1.0.0",
  installationId: stableInstallationId
});

// Display only authorization.userCode and authorization.verificationUri.
// Keep authorization.deviceCode in platform-protected storage.
await hosted.pollDeviceAuthorizationSession(
  authorization.authorizationSessionId,
  authorization.deviceCode
);

// A signed-in approval client previews and displays the requesting identity
// before submitting a separate approve or deny decision.
const preview = await hosted.previewDeviceAuthorization({
  userCode: authorization.userCode
});
await hosted.decideDeviceAuthorization({
  userCode: authorization.userCode,
  decision: "approve"
});

const redeemed = await hosted.redeemDeviceAuthorizationSession(
  authorization.authorizationSessionId,
  authorization.deviceCode
);
// Persist redeemed.accountCredentials in the platform account credential store.
```

The polling call intentionally exposes Hosted problem responses as `ApiError`:
`authorization_pending` means wait at least `interval`; `slow_down` means add
five seconds to that interval for every later poll in the same flow;
`access_denied` and `expired_token` are terminal. After a transport timeout,
the client must use exponential backoff; generic 5xx responses are retryable
and do not conclude the grant. It must not automatically create a
replacement session when a flow expires. `redeemDeviceAuthorizationSession`
returns a Hosted Portico Account credential family even when the account has no
servers or every server is offline. The client can then list servers and, after
the user selects one, attach with `connectResilientHostedServer`. Hosted
Services issues a bootstrap credential which the verified server exchanges for
its own durable native session at `/api/auth/portico/sessions`. That attachment
rechecks current membership, server auth mode, and remote access policy. Device
codes are sent only in the `X-Portico-Device-Code` header
and never in URLs. Human codes use protocol-v1 `XXXX-XXXX` formatting with the
unambiguous alphabet `ABCDEFGHJKMNPQRSTUVWXYZ23456789`.

## Integration Rule

Do not fork Portico request paths into each app. Add the operation to the owning canonical OpenAPI contract, expose an ergonomic wrapper here, and then consume it from the platform app.

`npm run api:types` regenerates both server and cloud types. The test suite verifies every client-owned request path and method against the corresponding canonical OpenAPI document and rejects retired `/api/v1`, iframe, relay, and compatibility-alias surfaces.

See `docs/IMPLEMENTATION.md` for web and native handoff notes.
See `docs/VIEWER_SYNC_COORDINATOR.md` for the shared Web, React Native, Roku,
and future-client synchronization/query lifecycle.
See `docs/HOSTED_RUNTIME_ADAPTERS.md` for the browser-default runtime boundary and React Native adapter contract.
See `docs/LAN_DISCOVERY.md` for the mDNS/DNS-SD adapter and trust contract.
See `docs/NATIVE_AUTH_CONFORMANCE.md` for credential ownership, durable state
transitions, server switching, and the published cross-client fixture.
See `docs/TRUSTED_SERVER_CONNECTIONS.md` for multi-server persistence and
Hosted-outage recovery.
See `docs/HOSTED_COMPATIBILITY.md` for the automatic Hosted Services version
and schema negotiation gate.
See `docs/DISTRIBUTION_AND_RELEASE.md` for entrypoints, semantic versioning,
artifact verification, and the intentionally non-publishing release procedure.

## Playback lifecycle

New clients create a session with `POST /api/playback-sessions`, send
monotonically ordered events to `PATCH /api/playback-sessions/{sessionId}`, and
close it with `DELETE /api/playback-sessions/{sessionId}`. Seed the local event
counter from `nextEventSequence` on start or restore. Duplicate or stale
sequences are acknowledged but never overwrite newer progress.

The older `POST /api/media/{id}/progress` mutation is fully retired. It is not
registered by the server or published in OpenAPI and must not be used as a
playback fallback.

iOS and tvOS shells should supply runtime AVFoundation facts to
`applePlaybackCapabilityProfile()`, pass its `clientProfile` when starting or
restoring playback, and keep AVKit implementation inside their PlayerAdapter.
The shared `playbackConformanceFixtures` cover direct play, remux, transcode,
grant expiry, queue conflicts, restore, and terminal playback failure.

## Search contract

Fetch the authenticated Product Contract before constructing first-party
search surfaces. `resolveSearchRequest()` applies the server-published quick or
full limit, sort direction, result scope, and continuation requirements.
`orderSearchGroups()` keeps returned groups in canonical order, while
`resolveSearchResultSemantic()` maps wire kinds such as `anime`, `audiobook`,
and `live_channel` to their hierarchy destination and artwork role.

Only submitted full searches should set `recordHistory`; quick/typeahead search
must leave it false. `searchHistory()` and `clearSearchHistory()` use the active
profile's bounded server history, while its visual presentation remains
application-owned. People are a canonical result group and `person()` resolves
their stable detail/credit pivot. Cursors are opaque and may only be replayed
for the same principal, query, group, library scope, sort, and direction.

## Server compatibility

Call `client.checkCompatibility()` after authenticating to a directly selected
server. It validates both the public `/api/system` API revision and the
authenticated Product Contract schema revision. `connectHostedServer()` and
`connectResilientHostedServer()` run
the same checks automatically: API compatibility is checked before account
bootstrap, and Product Contract compatibility is checked after access is
proven.

Incompatible revisions throw `PorticoCompatibilityError` with a stable code,
the reported revision, and Client Core's supported range. Optional behavior is
negotiated with `supportsServerCapability()`; unknown advertised capabilities
are retained and do not make an otherwise compatible server fail.

## Resource identities

Media IDs are server-issued, opaque random values. Clients must treat them as
case-sensitive strings and must never infer a media type, title, file path,
provider, library, or hierarchy from an ID. Older scanner-derived IDs remain
valid as input aliases, but current API responses always return the canonical
opaque ID so clients naturally converge on it.
