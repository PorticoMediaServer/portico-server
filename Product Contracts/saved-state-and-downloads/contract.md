# Saved State and Offline Downloads Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


Contract ID: `PC-SAVED-DOWNLOADS`
Revision: `portico.october.1`
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
6. Sign-out clears active credentials/presentation. Protected downloads remain unless the person explicitly removes them; learned access revocation is a separate deletion cause.

## 4. Personal media state

### 4.1 Independent fields

Watchlisted, favorite, reaction, watched, resume progress, personal rating, and last-played time are distinct. An implementation MUST NOT infer one from another except for the explicit playback completion rules below.

- `watchlisted` and `favorite` are booleans.
- `reaction` is `none`, `like`, or `dislike`.
- `rating` is an integer `0..10`; `null` means no rating and is distinct from zero.
- `watched` is the current explicit completion state.
- General personal-state wire progress is `progressMs`; client models may convert to seconds. Specialised offline receipt/progress shapes retain reported seconds fields pending time-field reconciliation.
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

## 8. Server preparation and transfer

Current server claim states are `queued`, `running`, `ready`, `paused`, `failed`, `unavailable`, `cancelled` and `expired`. The generated registry and `downloads/types.go` own exact actions/fields. Preparation binds owner, title/version/quality and verified bytes. A ready claim must still refer to available pinned source or published prepared output before transfer.

Transfer grants retain their address while the same artifact is used or renewed, with 24-hour idle lifetime and advertised renewal time. There is no separate first-use replay window. Resume revalidates exact artifact/size/digest; incompatible partials are not spliced. An offline source waits; changed/gone bytes end the claim but do not revoke existing device receipts.

Server claim retention and device offline authorisation remain independent. Unfetched claims have retention; completed local bytes do not depend on retaining the server artifact. Cancel/remove claim actions do not silently withdraw device receipts. Explicit receipt revocation is separate.

## 9. Client transfer and local state

Current native offline states are `downloading`, `paused`, `failed` and `ready`, with separate Wi-Fi/network waiting and explicit hold intent. The protected registry binds each transfer, local file, receipt and bundle to its stable owner. A receipt is acquired before transfer and persisted with the entry. Completion requires verified bytes; stale completion cannot publish after cancellation or owner change.

The local bundle provides titles, grouping, artwork, chapters, markers, audio labels and text subtitles. Complete offline subtitle/player/next-episode behaviour is not yet implemented. iOS cannot reuse its partial after an address change; Android dead-process restart and native transfer tend/recover need qualification. Completed downloads are hidden outside their owner; ordinary sign-out retains them unless removal is selected.

## 10. Offline authorization, expiry, and reconciliation

Portico does not implement commercial DRM, but offline access still follows the server owner's sharing and download permissions.

### 10.1 Signed receipt and exact bytes

The current envelope is defined in `downloads/receipts.go` and generated contracts. It carries `receiptId`, `algorithm`, `keyId`, base64url `payload`, `signature`, decoded `claims` and revision. Ed25519 verifies the exact decoded payload bytes; do not reserialise with an invented JSON canonicalisation rule.

Claims identify `kind: portico.download-receipt`, string version `1`, receipt/key, stable viewer authority/account/profile/server, item/preparation, quality/label, artifact SHA-256/bytes and `issuedAt`/`expiresAt`. Current receipts do not use the historic `verifyBy`, metadata-version or server-fingerprint/JCS schema. A receipt has a dedicated server-held signing key; obtain/pin verification keys through the authenticated selected-server receipt-key service, not from an untrusted receipt. Compare all signed/decoded owner and artifact evidence exactly.

The deadline is thirty consecutive 24-hour days. Expiry blocks playback and retains bytes for renewal. A timeout or unreachable server is not revocation. Wrong-owner attempts disclose no other scope and cannot delete a coherent inactive-profile download merely because it is not selected. Signature/digest/size disagreement is intrinsic invalidity and fails closed.

### 10.2 Revalidation and revocation

Current outcomes are `issued`, `renewed` or `refused` with a closed reason code. Issued/renewed carry a receipt; `titleOnServer` can be false without invalidating local bytes. Current renewal may also return viewing position/watched state. `unknown_receipt`/`unknown_preparation` alone preserves the copy and triggers scoped recovery; other refusal/revocation reasons follow current access policy. Replay cannot restore a withdrawn receipt.

Receipt renewal is daily and bounded, independent of server claim retirement. Current account/profile/library/content permissions and explicit receipt withdrawal still apply. Source/title/prepared-cache tidying is not access revocation. At expiry, no implicit grace extends playback. Clock rollback protection is unfinished and must be qualified rather than described as implemented.

### 10.3 Offline journal

Revalidate under the exact stable owner, reconcile incomplete transfers only against the same artifact, then upload original operation/event identities. Observation time mapped to intent order and monotonic event sequence preserve backward seeks and historical plays without replacing newer resume or explicit watched state. Do not infer newest intent from request arrival or maximum position. Pause-history policy remains applied by the shared writer. Native/real-network reconciliation still needs acceptance.

## 11. Deletion and destructive behavior

| User action | Required result |
| --- | --- |
| Cancel transfer | stop/revoke pending transfer; delete partial local bytes; retain other downloads |
| Remove download | delete local bytes, local receipt, catalogue entry, and its unsent playback telemetry only after reconciling explicit state mutations |
| Remove from My List/Favorites | change that set only |
| Delete playlist/collection/view | delete resource and grants only; never media |
| Clear watch history | remove chronological history records in the active `ViewerScope`; preserve current resume/watched state, Saved resources, ratings, reactions, and downloads |
| Reset viewing activity | no destructive bulk reset is currently implemented; do not describe it as an available action |
| Delete media from server | separate owner action; valid local copies survive source/title/cache retirement and cannot hide server deletion confirmation |
| Sign out | clear online credentials and active presentation; keep protected downloads unless the person explicitly chooses to remove them |

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

## October personal-state and download behaviour

Container Mark Watched/Unwatched applies to all current members, including partially played members. Both clear resume positions and fence older playback. A thirty-second Undo receipt restores the previous completion/progress mix, but refuses later edits/playback, changed membership, overlap or expiry. History, ratings and Saved flags remain. This supersedes protected-member defaults and constant-time container claims; large-container latency still needs qualification.

A server download claim and a signed offline receipt have separate lifetimes. Retention, source replacement, prepared-cache retirement, title removal and removal of the server list claim do not revoke valid local bytes. Current account/profile/library/content permission and explicit receipt revocation still apply. Expired receipts block playback without deleting bytes; renewal requires the same stable owner and current rights. A disappeared title is reported truthfully while its local bundle remains usable. Restoring or merging cannot rewrite a disconnected device's cached receipt; renewal reconciles identity.

Bundles carry title/group facts, artwork, chapters, markers, audio labels and text subtitles. Complete standalone playback also requires the appropriate styled/image originals, companions/fonts and local-player integration; those are unfinished. Chapter/trickplay images and offline next-episode playback remain gaps. No source snapshot is made solely to prolong an original download. Existing optimized recipes remain H.264 SDR/AAC; multipart titles cannot be downloaded as one file.

Ordinary sign-out preserves protected downloads; they remain hidden outside their owner. Explicit removal deletes them resumably. Transfer completion must still be fenced after sign-out, cancellation or owner change. Source and claim housekeeping is never access-revocation evidence.

Offline journal ordering preserves event sequence and observation time mapped to canonical intent order. It records historical plays without replacing a newer resume point or overriding later explicit watched intent. Server tests cover ordering/privacy; clock rollback, iOS partial reuse after URL change and dead-process Android transfer restart remain device gaps. Do not claim monotonic-clock protection is already complete.

## October scope and evidence

Offline current-permission and replay fixes are reported. Source tidying is not revocation. Local extras/next-episode/clock and background-transfer/device acceptance remain open; explicit access revocation still fails closed.

Sources: [personal-state](../../../_program/reports/personal-state.md), [downloads](../../../_program/reports/downloads.md), [review-downloads](../../../_program/reports/review-downloads.md), [native-sync](../../../_program/reports/native-sync.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
