# Saved State and Offline Downloads Contract

> Current Portico product contract, reconciled 5 September 2026.


Contract ID: `PC-SAVED-DOWNLOADS`  
Revision: `portico.reconciled.1`  
Status: **approved**  
Owners: Product, Server Personal State, Client Foundations, Offline Media  
Imports: `PC-FOUNDATION` viewer scope, permissions, and resource lanes  
Coordinates with: `PC-IDENTITY`, `PC-NETWORKING`, `PC-PLAYBACK`, `PC-METADATA`, `PC-SETTINGS-OPERATIONS`, `PC-ACCESSIBILITY-LANGUAGE`  
Applies to: Portico Server, Web, Client Core, and every first-party client; Hosted Services only where a later contract explicitly enables privacy-preserving account sync

## 1. Purpose and scope

This contract defines durable viewer state and media made available without a server connection. It prevents a common media-client failure mode: each client appearing correct alone while progress, watched state, queues, saved resources, and downloads disagree after interruption, profile switching, server switching, or concurrent use.

It covers:

- playback progress, completion, watched state, watch history, reactions, and personal ratings;
- My List (stored as `watchlisted`; named Watchlist before 2 Oct 2026), Favorites, saved views, playlists, collections, and playback queues;
- mutation ordering, idempotency, conflicts, cache invalidation, and durability;
- viewer/profile/server scope and switching;
- offline preparation, transfer, validation, playback, reconciliation, deletion, and storage policy.

It does not define the playback delivery ladder, metadata matching, library deletion, commercial DRM, cloud-hosted media, or cross-server aggregation. Portico v1 downloads are DRM-free copies or server-prepared derivatives of media the active viewer is allowed to download. They are not perpetual licenses.

This is a clean pre-release contract. Conforming implementations MUST implement this revision directly; they MUST NOT preserve contradictory legacy state shapes, mutation ordering, merge rules, download receipts, or implicit list semantics. A migration may transform persisted development data once, but a permanent compatibility branch requires a governed exception.

## 2. Canonical concepts and authority

| Concept | Canonical owner | Scope | Meaning |
| --- | --- | --- | --- |
| viewer state | selected Portico Server | `(ViewerScope, mediaId)` | current watchlisted, favorite, reaction, watched, resume, rating, and last-played values |
| playback event stream | selected Portico Server | `(playbackSessionId, generation)` | ordered evidence produced by the current playback authority |
| watch history | selected Portico Server | active viewer profile | chronological viewing records; distinct from current watched state |
| My List | selected Portico Server | active viewer profile | unordered intent to return to media |
| Favorites | selected Portico Server | active viewer profile | positive personal preference; not a playlist |
| playlist | selected Portico Server | owner plus named shares | explicit ordered entries; duplicate media is valid |
| collection | selected Portico Server | owner plus named shares | semantic membership; each media item appears at most once |
| saved view | selected Portico Server | active viewer profile | named library query, sort, pivot, and presentation |
| playback queue | playback authority | playback session or social group | ephemeral ordered intent for current playback, not a saved playlist |
| download preparation | selected Portico Server | `ViewerScope` + media/version/quality | server work producing or selecting exact transferable bytes |
| local download | one installation storage domain | `ViewerScope` + media/version/quality | validated offline artifact plus a minimal local catalogue record |

Hosted Services MUST NOT receive media titles, progress, history, ratings, playlist contents, collection contents, or download catalogues merely because the viewer uses a Portico Account. Any future cross-server sync requires an approved privacy contract and explicit user choice.

The client is a cache and command issuer, not an independent durable authority. A client MAY display last verified state while offline, but MUST label it by the exact server and viewer scope and MUST NOT merge it into another profile or server.

## 3. Viewer scope and switching

1. Every read, draft, optimistic mutation, event, queue, and offline record MUST retain the complete applicable authority-qualified stable owner key and the separate authorization/version evidence required by `PC-FOUNDATION`. Media ID alone, or account/profile/server IDs without authority, are never sufficient keys.
2. Selecting another profile or server is an authority transition. The client MUST cancel or generation-fence in-flight writes, clear uncommitted optimistic overlays, load the target scope, and only then publish the new viewer runtime.
3. A late response from the previous scope MUST be ignored even if its timestamp or revision is numerically newer.
4. A downloaded artifact MUST appear only for the profile that acquired it, unless a future approved contract explicitly allows household sharing. Storage deduplication MAY reuse encrypted bytes internally but cannot broaden catalogue visibility or authorization.
5. Server removal, membership revocation, profile disablement, or permission loss immediately prevents new online use and new download grants. Existing offline bytes become `authorization-unverified`; the client follows section 10 rather than silently granting indefinite access.
6. Sign-out removes local credentials, scoped caches, offline index keys, pending personal mutations, and protected download material for that signed-out account. A product-owner-approved keep-downloads feature would require a new contract; it is not implied.

## 4. Personal media state

### 4.1 Independent fields

Watchlisted, favorite, reaction, watched, resume progress, personal rating, and last-played time are distinct. An implementation MUST NOT infer one from another except for the explicit playback completion rules below.

- `watchlisted` and `favorite` are booleans.
- `reaction` is `none`, `like`, or `dislike`.
- `rating` is an integer `0..10`; `0` means no rating.
- `watched` is the current explicit completion state.
- `progressSeconds` is a resumable position, not a playback telemetry percentage.
- `lastPlayedAt` records accepted playback activity, not a card view or manual rating.

Marking watched MUST set `watched=true` and clear the resume point. Marking unwatched MUST set `watched=false` and clear the resume point unless the same explicit operation includes a new resume position. Adding to Watchlist or Favorites MUST NOT change watched state. Clearing watch history MUST preserve Watchlist, Favorites, reaction, rating, playlists, collections, and downloads.

### 4.2 Progress state machine

Canonical states are:

`never-played -> below-resume-threshold -> in-progress -> completed`

Explicit user actions may transition `completed -> never-played` (Mark Unwatched) and any non-playing state to `completed` (Mark Watched). Beginning playback after completion creates a new playback session without clearing `watched` until accepted progress crosses the configured resume threshold; at that point it becomes `in-progress` and `watched=false`. This prevents a one-second accidental replay from erasing completion.

The server applies the active viewer's approved progress thresholds:

- below start threshold: no resume point;
- at or above start threshold and below completion threshold: resumable and not watched;
- at or above completion threshold or an accepted terminal `completed=true`: watched with no resume point.

Tracks MAY record listening history without exposing a resume point. Audiobooks MUST preserve chapter-accurate resume position. Hierarchy rollups (episode to season/show, track to album) are projections computed from child state and MUST NOT overwrite child truth.

### 4.3 Ordered playback writes

Document 32, PC-PLAYBACK, owns online playback occurrence, command lane, observation ordering, handoff and canonical resume-writer selection. Each independent playback has its own telemetry/history. The same profile may play on multiple devices; profile equality never transfers ownership or stops another occurrence. Explicit handoff transfers only the named source occurrence and its queue.

Personal state consumes accepted playback evidence through one domain interface. A stale generation, duplicate event, old occurrence or prior explicit-mutation fence cannot overwrite canonical resume/watched state. Progress persistence cannot block decode, seeking or local Pause/Stop. Ordinary AirPlay changes output route without creating a receiver actor. Watch Together maintains separate participant playback evidence rather than copying host progress into every profile.

The completion and resume thresholds in section 4.2 remain product requirements; the active playback contract determines which evidence is eligible to apply them. Offline evidence is a separate reconciled event stream, not a fabricated currently active online session.

### 4.4 Explicit mutations and concurrency

Every personal-state mutation MUST carry an idempotency key and expected state revision in the completed mutation protocol. The accepted result returns the complete current state plus a new revision.

- Same idempotency key and same body returns the original result.
- Same key with a different body is rejected.
- Stale expected revision returns a typed conflict and the current state.
- The UI reconciles the current state and asks again only when user intent cannot be safely replayed.
- Boolean set operations may be retried with the same key and identical request within the receipt lifetime. They may not silently rebase a stale expected revision over newer intent. Toggle operations are prohibited at the API boundary.
- Explicit Mark Watched/Unwatched or rating changes take precedence over older playback evidence. They do not grant future stale sessions permission to undo the choice.
- An offline explicit mutation MUST also carry a stable device mutation ID, the device's monotonic mutation sequence, its base revision, and the authored instant as bounded evidence. Network arrival order is never treated as proof of later user intent.
- When two explicit offline mutations share an ancestor and express incompatible intent, the server records a conflict instead of silently choosing whichever request arrived first. The current viewer is shown the two explicit outcomes with device and authored-time context and may choose the result; replaying that resolution is idempotent.
- Until that conflict is resolved, the server preserves the last uncontested canonical value and returns `needs-resolution`. It MUST NOT oscillate between values as devices reconnect.

## 5. Saved resources

### 5.1 My List and Favorites

My List and Favorites are profile-private media sets. They are not shareable resources, cannot contain duplicate media, and have no user-defined ordering. Canonical browse sort and filtering MAY be selected by the client or saved as a Saved View, but set membership remains independent.

Removing an item whose media is now missing succeeds idempotently. A missing but retained library item remains visible with an unavailable state until normal library reconciliation removes it; the client MUST NOT silently hide saved intent merely because playback is temporarily unavailable.

### 5.2 Playlists

1. A playlist owns a stable playlist ID, owner, title, optional summary, visibility, share grants, revision, and ordered entry list.
2. Each entry has its own stable `entryId`. The same media ID MAY appear multiple times; delete and reorder operations address entry IDs, never media IDs.
3. Reordering sends the complete intended order of retained entries plus `expectedRevision`. Partial or duplicate entry-ID orders are rejected.
4. Concurrent mutation uses compare-and-swap. The server never applies a reorder to a different revision by best effort.
5. If media disappears, the entry persists as unavailable until the owner removes it or the media identity is authoritatively deleted. The server does not retarget an entry based only on title.
6. Deleting a playlist requires explicit confirmation and removes the resource and share grants, not the media.

### 5.3 Collections

Collections are semantic, unordered membership sets. Each media ID appears at most once. Add/remove batches are set operations, idempotent by value, and revision-checked. Collections MUST NOT be treated as playback order; a user may explicitly create a queue from a collection using a declared sort.

Deleting a collection removes grouping and shares only. It MUST NOT delete media, downloaded artifacts, progress, or metadata-provider collection relationships unless the user separately chose that destructive action.

### 5.4 Saved views

A Saved View stores one library ID, canonical pivot, validated query expression, stable sort list, presentation choice, pinned state, and revision. It stores no materialized media IDs. Opening it evaluates against current library truth and permissions. If a field/pivot is no longer supported, the server returns `needs-review` with the invalid components; clients MUST NOT silently broaden the query.

### 5.5 Shares and visibility

Saved-resource shares disclose only current display identity and view/edit rights. Login names, email addresses, roles, unrelated permissions, and Apple relay-address status are not shown. Owner, viewer, and editor behavior follows `PC-SHARING-SOCIAL`.

## 6. Playback queues

1. A queue is scoped to one playback authority: local session, Cast receiver session, or Watch Together group.
2. Queue items have stable queue-entry IDs, source context, and explicit order. Duplicate media is valid.
3. Queue revision changes on add, remove, reorder, shuffle materialization, repeat-mode change, or automatic advancement.
4. Mutations require expected revision and idempotency key. A stale mutation returns current queue; it is not merged by array position.
5. Shuffle MUST materialize or seed a reproducible order for the current queue revision. Every participant/receiver sees the same order.
6. The current item remains addressable even if removed from the future queue; its completion transition determines the next surviving item.
7. A queue is not promoted to a playlist implicitly. **Save queue as playlist** is an explicit copy operation.
8. When a server switch makes queue items unavailable, the old queue remains with the old server session. It does not cross servers or match by title.

## 7. Offline download intent and quality

The user may request Source or any published download quality. Portico MUST respect the exact choice. Source means source quality, but the server MAY remux without loss when the original container is not usable by the device; it MUST disclose that compatibility remux. A lower-quality selection is a hard ceiling, not a hint the planner may silently exceed.

Before commit the client shows, when known:

- chosen quality and whether preparation/transcoding is required;
- measured or clearly labelled estimated size;
- available device storage and applicable storage limit;
- Wi-Fi-only/cellular policy;
- whether the selection expands to a season, show, album, artist, playlist, or next-episode rule.

The server rechecks `media.download`, viewer/library/media access, server identity, profile identity, and current authorization revision when creating preparation and again when issuing a transfer grant.

## 8. Server preparation state machine

Canonical states:

`requested -> queued -> preparing -> ready -> transferring-observed -> retained`

Side/terminal states:

- `paused` may resume to `queued` or `preparing`;
- `failed-retryable` may retry using the same preparation ID and incremented attempt;
- `failed-terminal` requires a changed source, quality, permission, or new request;
- `cancelled` stops work and schedules unreferenced artifact cleanup;
- `removed` is terminal and hidden from ordinary lists;
- `authorization-changed` is terminal for that preparation, never automatically rebound to a new viewer.

Rules:

1. Create is idempotent by `(idempotencyKey, serverId, accountId, profileId, request body)`.
2. The preparation binds exact media/version, requested quality, server identity, authorization revision, and resulting artifact digest.
3. Cancellation is cooperative but durable: once accepted, late workers cannot publish `ready` without a newer lease generation.
4. A server restart reconciles queued/running work from durable job and artifact state. It never declares success from a path string alone.
5. `ready` requires a stable file or stream, measured byte length where possible, cryptographic digest, and compatibility facts.
6. Transfer grants are short-lived, single scope, clean-URL credentials. Account authentication alone does not bypass them.
7. Server artifact retention and client offline retention are separate. Removing local bytes does not automatically delete a reusable optimized server artifact; server policy owns that lifecycle.

## 9. Client transfer and local state machine

Canonical client states:

`not-downloaded -> waiting-for-server -> queued -> transferring -> verifying -> available-offline`

Additional states are `paused`, `waiting-for-wifi`, `waiting-for-storage`, `server-unreachable`, `failed-retryable`, `failed-terminal`, `authorization-unverified`, and `deleting`.

1. Transfer writes to an installation-private temporary location. `available-offline` is published only after expected length/digest validation and atomic rename/publication.
2. Resume uses validated byte ranges and an unchanged artifact identity. An ETag/digest/version change discards incompatible partial bytes rather than splicing artifacts.
3. The local catalogue and bytes commit atomically from the product's perspective. Catalogue-without-bytes becomes `repair-required`; bytes-without-catalogue are quarantined then deleted.
4. Playback uses the downloaded artifact without contacting the server when it is `available-offline` and its local authorization is still valid.
5. The download catalogue includes only minimal presentation metadata needed offline, the selected streams/quality, source server label/ID, viewer label/ID, digest, size, acquired/last-verified timestamps, and local progress outbox. It MUST NOT include server credentials or hidden library metadata.
6. Download progress and watched changes created offline enter a per-viewer outbox with stable operation IDs. They never write into another signed-in profile.
7. Local storage pressure pauses new work before evicting completed downloads. Automatic eviction is permitted only under a user-approved policy and follows deterministic priority; pinned downloads are never automatically evicted.

## 10. Offline authorization, expiry, and reconciliation

Portico does not implement commercial DRM, but offline access still follows the server owner's sharing and download permissions.

### 10.1 Immutable offline authorization receipt

Every completed download has exactly one current locally protected receipt preserving the semantic bindings below. The original [`offline-download-authorization-receipt.schema.json`](../../../References/Contract%20Baseline%202026-09-05/saved-state-and-downloads/offline-download-authorization-receipt.schema.json) is a reference to port under the current ownership and revision rules, not the rebuilt wire authority. The selected Portico Server issues it only after it has rechecked download authorization and bound the exact verified bytes. The receipt is immutable after signature and contains only:

- `version`, the fixed receipt format version `1`; `purpose`, fixed to `offline-download-authorization`; and an opaque `receiptId`;
- the complete server-bound `ViewerScope`, including `authorizationRevision`;
- the issuer Server ID and the established Server identity fingerprint: the literal lowercase prefix `sha256:` followed by the canonical unpadded base64url encoding of the SHA-256 digest of the raw 32-byte Ed25519 public key already pinned for that Server;
- the exact preparation ID, media ID, media-version ID, and quality ID;
- the exact artifact SHA-256 digest and byte length;
- `lastVerifiedAt`, the Server's successful authorization-verification instant, and `verifyBy`, published by that Server at exactly `lastVerifiedAt + 2,592,000 seconds` (30 consecutive 24-hour days);
- one Ed25519 signature.

The issuer Server ID MUST exactly equal `ViewerScope.serverId` and the originating Server stored with the local download. The signing-key fingerprint MUST be byte-for-byte equal to the established fingerprint exposed and pinned through that Server's authenticated identity; Portico does not transcode it or introduce a second key-identity representation. Validators MUST base64url-decode the suffix after `sha256:` and require exactly 32 digest bytes. Padded base64, standard base64, hexadecimal, a differently cased prefix, or any other alternate representation is invalid even if it purports to name the same key. A key supplied with the receipt or an unpinned replacement key is never trusted. The receipt's preparation, media, version, quality, digest, and size MUST exactly equal both the completed preparation and the locally verified artifact. The receipt's stable owner tuple MUST equal the protected local catalogue owner; its authorization revision must equal the issuance evidence stored with that receipt, not necessarily the currently refreshed online revision. These equalities are semantic validation rules in addition to JSON Schema validation.

The connection credential and protected download catalogue MUST retain the originating Server's already verified raw 32-byte Ed25519 public key across restart together with the Server ID and fingerprint. Hosted connections obtain it from the signed route/verified attachment identity; direct local authentication obtains it from the successful native-session credential response. In both cases the client computes the canonical fingerprint from the raw key before pinning it. Persisting only the fingerprint is insufficient and cannot be repaired by derivation. If the raw key is absent or cannot be proven to match the pin, the client reports `authorization-unverified`, preserves protected bytes, and reacquires the identity through an authenticated Server connection; it MUST NOT accept the receipt's fingerprint as a key source or delete otherwise coherent bytes merely because older development state omitted the raw key.

To sign or verify, remove the root `signature` member, serialize the remaining receipt using RFC 8785 JSON Canonicalization Scheme, encode that canonical payload as UTF-8, and sign or verify those bytes with Ed25519. No field is excluded from the canonical payload other than `signature`. Adding, removing, or changing any signed value invalidates the receipt; JSON object member order alone does not.

### 10.2 Local validity and deletion boundary

An artifact is playable offline only when the receipt passes schema, equality, pinned-key, signature, artifact-digest, and artifact-size validation and the current instant is strictly before `verifyBy`. At `verifyBy` and afterward, it becomes `authorization-unverified`: playback is blocked, but the receipt and protected bytes are retained for revalidation. Clients MUST NOT add clock skew, a grace period, a calendar-month interpretation, or a platform-local verification interval.

Opening Portico online revalidates receipts opportunistically without blocking already-valid local playback. Temporary Server or Hosted unavailability before `verifyBy` does not remove access. Temporary unavailability at or after `verifyBy` blocks playback but does not delete the receipt, catalogue entry, or protected bytes. A transport failure, timeout, or absence of a revalidation response is never evidence of revocation.

When the client explicitly learns membership revocation, profile deletion, download-permission removal, media revocation, remote wipe, or sign-out, local playback fails closed and the associated protected bytes, receipt, and catalogue entry are deleted. An identity mismatch authorizes the same deletion only when the client independently establishes an intrinsic disagreement between the signed receipt and its protected catalogue scope, pinned originating Server/key, or artifact. A schema-invalid receipt, failed signature, unpinned or wrong key, or artifact digest/size mismatch is integrity-invalid and follows the same deletion path. Deletion is resumable and MUST be confirmed by the filesystem under section 11.

A correctly scoped download belonging to another inactive profile is not an identity mismatch merely because that profile is not selected. It remains protected and invisible until that exact viewer scope is active. Clients revalidate a receipt only under its own stable authenticated server-bound owner tuple, permitting a newer current authorization revision; they never probe it under the currently selected but different viewer.

### 10.3 Privacy-safe revalidation outcomes

Receipt revalidation has exactly four application outcomes. This is an outcome vocabulary, not a requirement for a particular transport or endpoint shape.

| Outcome | Meaning | Client result |
| --- | --- | --- |
| `valid-replacement` | The originating Server verified the exact receipt binding and current authorization for the same account, profile, Server, preparation, media version, quality, and artifact. | Atomically store the returned newly signed receipt, then discard the superseded receipt. Playback may continue. |
| `revoked` | The exact authenticated scope is known to have lost the relevant membership, profile, permission, media, or remote-wipe authorization. | Fail closed and delete the associated protected bytes, receipt, and catalogue entry. |
| `invalid` | The request is malformed or the receipt, issuer, pinned key, signature, preparation binding, or artifact binding fails validation within the exact authenticated scope. | Fail closed and delete the associated protected bytes, receipt, and catalogue entry. |
| `out-of-scope` | The receipt's Server, authority, account or profile is not the authenticated stable owner tuple. A revision difference alone is not out-of-scope; it requires current authorization evaluation. | Fail closed for this attempt and reveal no receipt or media facts. Preserve the protected local record and retry only under its own exact scope; this outcome alone is not deletion authority. |

`valid-replacement` returns a complete new immutable receipt with a new `receiptId`, a current server-bound `ViewerScope` and `authorizationRevision`, a new `lastVerifiedAt`, and `verifyBy` exactly 30 days later. It retains the same issuer Server, preparation, media, version, quality, artifact digest, and artifact size. Revalidation MUST NOT edit, extend, countersign, or reuse the old receipt in place. The old receipt remains unchanged until the replacement has been durably committed; a crash cannot leave a partially rewritten receipt.

`revoked`, `invalid`, and `out-of-scope` disclose only the stable outcome and generic recovery semantics. They MUST NOT return media titles, paths, library facts, profile labels, digests, byte sizes, whether a receipt exists in another scope, or whether a differently scoped receipt would be valid. For an authenticated request outside the signed stable owner tuple, the Server returns the same `out-of-scope` outcome without looking up or confirming the receipt. An accidental revalidation attempt under the currently selected different profile or account is therefore not evidence that the stored receipt is corrupt and MUST NOT delete it. Deletion requires an intrinsic mismatch independently established between the receipt, protected catalogue, pinned originating Server/key, or artifact, or a `revoked`/`invalid` result from the properly scoped originating authority. A timeout, unreachable Server, or transport failure produces no application outcome and therefore cannot be converted into `revoked`, `invalid`, or `out-of-scope`.

A Server cannot remotely enumerate unrelated files on the device and can revoke only receipts it issued. The client submits only the one receipt it is revalidating; no API in this contract accepts arbitrary media IDs, file paths, or digest searches as receipt-enumeration keys.

Reconciliation order after connectivity returns:

1. verify account/profile/server identity and authorization receipt;
2. fetch preparation/artifact status if transfer was incomplete;
3. upload offline explicit mutations and playback events using original operation IDs;
4. accept server conflicts/current revisions;
5. refresh minimal catalogue metadata without changing artifact identity;
6. apply deletion/revocation outcomes;
7. publish reconciled UI state.

Offline observations have monotonic device event sequence, not necessarily increasing position: deliberate seeks and replay may move position backward. Reconciliation must preserve that intent without allowing stale evidence to overwrite a later canonical writer. Explicit offline Mark Watched/Unwatched and rating actions are verbatim user intent and may move state in either direction. If two devices make incompatible explicit changes from a shared ancestor, section 4.4's `needs-resolution` flow applies; request arrival order does not choose a winner and devices do not oscillate.

## 11. Deletion and destructive behavior

| User action | Required result |
| --- | --- |
| Cancel transfer | stop/revoke pending transfer; delete partial local bytes; retain other downloads |
| Remove download | delete local bytes, local receipt, catalogue entry, and its unsent playback telemetry only after reconciling explicit state mutations |
| Remove from My List/Favorites | change that set only |
| Delete playlist/collection/view | delete resource and grants only; never media |
| Clear watch history | remove chronological history records in the active `ViewerScope`; preserve current resume/watched state, Saved resources, ratings, reactions, and downloads |
| Reset viewing activity | separately named and confirmed broader action removing history plus resume/watched state in the active `ViewerScope`; preserve Saved resources, ratings, reactions, and downloads |
| Delete media from server | separate owner action; downloaded copies enter revoked/deletion according to server policy and cannot hide the server-side destructive confirmation |
| Sign out | remove the account's local protected downloads and scoped state |

Deletion is idempotent. Interrupted deletion resumes on next launch. The UI MUST NOT report reclaimed space until the filesystem confirms removal. Secure erasure claims are prohibited on storage where the OS cannot guarantee them.

## 12. User-visible failure states

Clients distinguish: device offline, server offline, permission denied, source missing, preparation failed, storage full, storage location unavailable, artifact changed, integrity check failed, authorization needs verification, and client update required. Raw paths, provider URLs, transport strings, media grants, digests, and worker errors remain diagnostic-only.

Retryable errors keep stable content, progress, and actions visible. A transient refresh MUST NOT empty Completed Downloads, My List, playlist entries, or queue. Retry follows `PC-NETWORKING`; manual Retry is offered only when it represents a meaningful user decision or immediate override.

## 13. Platform adaptation

- Web and television MAY omit offline-download capability when the platform does not provide protected durable storage, but must not show a dead Download action.
- Mobile and desktop MAY use OS background-transfer APIs and platform storage pickers.
- Android MAY support one selected removable-storage root. Changing it is a migration/destructive operation with an explicit choice; it MUST NOT silently delete existing downloads.
- Clients may vary list density and native progress UI, but use the same state names/outcomes and destructive semantics.
- A platform cannot rename Source into Original if it may perform lossy conversion; any compatibility remux remains lossless and disclosed.

## 14. Privacy and security

Download bytes, receipts, catalogue, and offline outbox MUST use application-private storage and OS data protection where available. Reusable account/server credentials never appear in URLs. Narrow media/download capability transport follows the active delivery protocol, with exact scope, expiry and redaction; it must not become reusable account authentication. Credentials and grant values never appear in filenames, logs, notifications, analytics, unprotected backups or shared storage. Share sheets/export are separate explicit actions and are out of scope.

Profile restrictions, content rating policy, library access, and media availability are checked before showing personal state and before playback/download. Clients MUST NOT reveal saved titles from a prior viewer while the new viewer is loading.

## 15. Conformance

`state-machines.yaml` is the normative state vocabulary. `conformance-vectors.yaml` contains minimum adversarial outcomes. Required evidence includes:

- offline-receipt JSON Schema validation, canonical-payload/signature fixtures, state validation, and generated contract drift checks;
- session generation/sequence/idempotency tests;
- concurrent explicit mutation and queue/playlist revision tests;
- profile/server switch with late response tests;
- interrupted preparation, transfer resume, digest mismatch, storage-full, and restart recovery tests;
- offline playback and reconciliation tests across explicit watched/unwatched conflicts;
- 30-day receipt-boundary, replacement, hostile-scope, permission-loss, sign-out, and download-deletion tests;
- UI state inspection on every capable platform.

### 15.1 Applicability matrix

| Rule | Server | Hosted | Web | Client Core | Apple | Android | Roku | Cast | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| canonical personal state/revisions | required | not applicable unless later sync contract | required | required | required | required | required | receiver updates through server | mutation fixtures |
| offline protected downloads | issues artifacts/receipts | not applicable | not applicable unless protected durable client exists | required projection | required where offered | required where offered | not applicable | not applicable | transfer, receipt, storage tests |
| profile/server fencing | required | policy input only | required | required | required | required | required | required | late-response/switch fixtures |
| destructive semantics | required | not applicable | required | canonical outcomes | required | required | required where action exists | not applicable | interruption and idempotency tests |

An approved exception uses this exact record from `../governance.md`, including a test or fixture proving its boundary:

```text
PORTICO-CONTRACT-EXCEPTION
Contract: PC-SAVED-DOWNLOADS#<section-id>
Scope: <platform, surface, or capability>
Decision: <the exact deviation>
Reason: <why the canonical rule cannot or should not apply>
Approved-by: <name or recorded decision reference>
Review-when: <expiry date, capability change, or other objective trigger>
Evidence: <test or fixture that prevents accidental expansion>
```

The approved record MUST also appear in this contract's exception registry. A comment without boundary evidence is not an approved exception.

## 16. Product decision

An offline authorization receipt remains usable for 30 days after successful server verification. Explicit revocation applies immediately whenever learned; temporary unreachability after the deadline blocks playback without deleting the protected bytes.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.
