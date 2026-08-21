# Portico Client Core Implementation Notes

## Web UI Integration

Portico Web consumes `@porticomediaserver/client-core` through its runtime and data-source adapters. React owns presentation and browser lifecycle; the shared package owns the transport contract and direct-route policy.

Recommended web adapter responsibilities:

- Read runtime config from `window.__PORTICO_CONFIG__` and `import.meta.env`.
- Persist active hosted/local server sessions in `window.sessionStorage`.
- Provide `csrfToken: "1"` for local server mutations.
- Provide a hosted CSRF token reader from `document.cookie`.
- Provide `browserPlaybackClientProfile`. The shared browser profile is intentionally conservative for direct-play decisions: it advertises HLS, common MP4/WebM browser codecs, and stereo AAC/MP3-style audio, but it does not advertise HEVC, AC3, or EAC3 from static `canPlayType` checks alone. Native Apple/TV/mobile apps should supply explicit platform profiles when they can prove broader codec, passthrough, or multichannel support.
- Re-emit `onMutation(tags)` through the web UI's existing data revalidation event.
- Establish Hosted connections only through a verified direct server route. The package intentionally exposes no iframe bridge, tunnel, or fallback relay transport.
- Own one `ViewerSyncCoordinator` per authenticated viewer generation; map
  visibility/network/player state and application events through the shared
  lifecycle documented in `VIEWER_SYNC_COORDINATOR.md`.

The shared package should own endpoint paths, request bodies, response types, resource URL token policy, playback normalization, mutation tags, and hosted route selection. The web UI should own display and browser lifecycle.

## TypeScript Client App Integration

Each platform app should create one `PorticoClient` per selected server route and one `HostedServicesClient` for Portico account work.

Before treating a directly selected server as ready, call
`client.checkCompatibility()` after authentication. This is deliberately an
explicit bootstrap boundary rather than a hidden guard on every request.
Hosted connections perform the split check automatically: the public server
API v1 is validated before `/api/auth/me`, and the authenticated Product
Contract revision is validated afterward. Present `PorticoCompatibilityError`
messages directly or map their stable codes to platform-native recovery UI.

Feature availability must be read from Product Contract capabilities using
`supportsServerCapability()`. Do not infer optional features from server build
versions, and do not reject unknown capability identifiers.

For a library workspace, fetch `libraryPivotBrowseCapabilities(libraryId, pivotId)`
after the server-published destination chooses a pivot. That response is already
narrowed to the selected library and pivot, including enum values, dynamic facet
sources, semantic input hints, progressive complexity, sorts, and presentation
fields. Use `resolveBrowseWorkspaceQuery()` before issuing a browse request and
the shared filter compiler/chip helpers for advanced editors. Presence operators
have one canonical wire value: `null`. Build Saved-view requests with
`serializeSavedViewDraft()` so native and web clients preserve the same query,
sort, presentation-field, and title rules. Use `withBrowseAlphaSeek()` on every
title-ascending continuation so the alphabet anchor remains in cursor scope.

Use `resolveMediaActionCommand()` only for an action ID present on the current
resource. API commands declare single or per-item execution and return a typed
method/path/body plan; client-flow commands identify an application-owned
picker or editor. The caller owns confirmation presentation and applies the
published invalidation tags after success.

Resolve browse/search cards with `resolveMediaCardViewModel()` and full detail
resources with `resolveMediaDetailViewModel()`. Both return the same
platform-neutral `MediaViewModel`: switch on `destination.kind` in the native
navigation adapter, render `artwork.shape.aspectRatio` and `fit` with native
image primitives, and dispatch only the returned `actionIds`. Do not maintain
an Apple- or Web-local kind/artwork table. A kind added to Product Contract is
resolved immediately; an unpublished future kind remains visible through the
conservative detail/poster fallback.

Platform adapters must provide:

- `fetch` transport when the runtime does not expose a WHATWG-compatible global `fetch`.
- Hosted connection runtime adapters for base64 decoding, UTF-8 encoding, Ed25519 verification, abort controllers, timers, and clock access when those browser-standard capabilities are not globally available. Prefer explicit adapters over global polyfills; see `HOSTED_RUNTIME_ADAPTERS.md`.
- Secure session storage using the platform's secret store.
- A durable cleanup quarantine for `CredentialCleanupUncertainError`; the
  typed error means at least one credential deletion failed and must latch the
  application fail-closed across process restart.
- An atomic `TrustedServerConnectionAdapter`. Multi-store adapters must throw
  `TrustedServerDurabilityUncertainError` when a partial write cannot be fully
  compensated; an ordinary save error asserts that the prior value is intact.
- A `stageCandidate` runtime transaction that fences old requests before active
  credential publication, publishes UI only after durability resolves, and can
  restore the previous runtime or remain fail-closed.
- A conservative `playbackClientProfile` for that platform and device.
- Cache/query invalidation from `onMutation(tags)`.
- An `eventStream` adapter when platform fetch does not expose a browser
  `ReadableStream`. It may yield decoded strings or byte chunks with an
  injected stateful UTF-8 decoder.
- Resource loading that respects authenticated URLs returned by `resourceUrl` and `imageResourceUrl`.
- One viewer-generation `ViewerSyncCoordinator`, with player continuity marked
  above visible-route interaction and hidden/background reconciliation.

### iOS and tvOS playback

Build `applePlaybackCapabilityProfile()` from runtime AVFoundation facts and
the active local/remote route limits. Pass the returned `clientProfile` to
playback start, restore, prepare-next, and handoff calls. `maxBitrateKbps` is a
native-facing route value; Client Core converts it to the server contract's
bits-per-second field.

AVKit remains app-owned behind the platform PlayerAdapter. The shared contract
owns delivery eligibility, HEVC and HDR/Dolby Vision declarations, audio codec
and channel limits, subtitle native-versus-burn-in policy, and response/error
fixtures. Do not infer capabilities from a device model in shared code.

Seed the event counter from `nextEventSequence`, write playback observations to
`PATCH /api/playback-sessions/{sessionId}`, renew an expired media grant through
the session route, and reload queue state after a revision conflict. Never fall
back to the retired `POST /api/media/{id}/progress` route; it is unavailable.

Render native failures from `ApiError.detail` and use `code` for recovery
branching. `requestId` is always promoted independently of structured
`details`; `responseHeaders` and parsed `retryAfterMs`/`retryAt` support retry
UI without platform-specific header parsing. Client Core adds `X-Request-ID`
to requests by default, and apps may inject a generator for native telemetry.

Platform-specific code must continue to own:

- Native media player surfaces.
- Remote/focus navigation.
- App lifecycle cleanup.
- Offline/download storage.
- Push notifications and device permissions.

TV and mobile clients should use the shared Nearby TV Setup methods rather than re-declaring request and response shapes locally. The server client exposes `createTVSetupSession`, `tvSetupSession`, `authorizeTVSetupGrant`, and `redeemTVSetupGrant` for Local Auth. The Hosted Services client exposes `createTVSetupSession`, scoped-secret `tvSetupSession`, `authorizeTVSetupGrant`, and `redeemTVSetupSession` so a signed-out TV can display a code before choosing a server. Both implementations create protocol-v1 `XXXX-XXXX` codes from `ABCDEFGHJKMNPQRSTUVWXYZ23456789`; clients normalize lowercase, ordinary spaces, and at most one dash but reject every other shape. Binary, HLS, image, and Server-Sent Event routes should be consumed through the shared URL helpers so hosted/Portico tokens are applied consistently.

## Security Notes

- Do not persist passwords, raw stream URLs, provider URLs, bearer tokens in logs, or local filesystem paths.
- Use `Authorization: Bearer ...` through the session store for server-scoped device/hosted tokens.
- Public routes must be HTTPS. LAN/local HTTP acceptance belongs in platform route policy before the client is instantiated.
- Resource URL builders never place account or device credentials in a query string. Playback responses carry short-lived, playback-session-scoped `media_grant` URLs for native media elements; ordinary artwork uses cookie/header authentication.

## Feature Coverage Standard

This package covers every canonical operation needed by Portico Web and future first-party clients. Generated OpenAPI types remain the transport source of truth; adapter tests verify that the web runtime consumes those operations without redefining paths, pivots, permissions, or resource semantics.
