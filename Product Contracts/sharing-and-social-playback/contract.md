# Sharing, Remote Playback, and Social Playback Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


Contract ID: `PC-SHARING-SOCIAL`
Revision: `portico.october.1`
Status: **approved**
Owners: Product, Server Social Playback, Server Authorization, Hosted Memberships, Playback, Client Foundations
Imports: `PC-FOUNDATION` viewer scope, permissions, and resource lanes
Coordinates with: `PC-IDENTITY`, `PC-ONBOARDING`, `PC-NETWORKING`, `PC-PLAYBACK`, `PC-SAVED-DOWNLOADS`, `PC-ACCESSIBILITY-LANGUAGE`
Applies to: Portico Server, Web, Client Core, Apple, Android, television clients, Roku, and the Google Cast receiver; Hosted Services only for global-account identity, server-membership policy, and optional opaque invitation delivery

## 1. Purpose and scope

This contract defines three related but separate product capabilities:

1. inviting a Portico Account to become a member of a server and managing that membership;
2. moving or remotely controlling playback on Cast, AirPlay, or another authorized Portico player;
3. synchronizing multiple eligible viewers through Watch Together.

They share identity, authorization, handoff, and privacy rules but MUST NOT collapse into one concept. Server membership is access. A playback receiver is a temporary device authority. Watch Together is a server-scoped social playback session coordinated entirely by the Portico Server hosting the media. None creates a global friends/social graph.

This contract does not define public social profiles, public activity feeds, media piracy/export, anonymous guest links, cross-server Watch Together, a Hosted media relay, or arbitrary remote-control access to another person's device.

Hosted Services MUST NOT store or coordinate Watch Together room state, queue state, participant readiness, timing, commands, event streams, media identity, playback plans, grants, or progress. It may authenticate a Portico Account, publish signed membership/privacy policy to the server, resolve an explicitly selected eligible account for invitation, and deliver an opaque wake/notification containing only the server/invitation route needed to contact that server. The invitation ticket and every subsequent group operation are server-issued and server-redeemed.

This is a clean pre-release contract. Conforming implementations MUST implement this revision directly; they MUST NOT retain contradictory legacy invitation secrets, membership projections, receiver tokens, Cast authority, Watch Together events, or multi-writer synchronization. A one-time migration may transform development data, but a continuing compatibility branch requires a governed exception.

## 2. Canonical authorities and separations

| Resource | Authority | Scope |
| --- | --- | --- |
| invitation and Hosted membership | Hosted Services | one Portico Account and one server |
| local server projection/permission enforcement | selected Portico Server | exact authority-qualified `ViewerScope` and policy generation |
| saved-resource share | selected Portico Server | one playlist/collection and named server members; not server access |
| playback receiver session | selected Portico Server | one `ViewerScope`, receiver, playback session, and generation |
| AirPlay route | Apple OS plus originating Portico player | one local player session; not a Portico credentialed receiver |
| Watch Together group | selected Portico Server | one server, host/participant `ViewerScope` values, queue, and revision lineage |
| personal progress/history | selected Portico Server | each participant's own `ViewerScope` |

Being invited to a server does not imply friendship, activity visibility, permission to control another device, or access to every library. Sharing a playlist/collection does not grant missing media/library access. Joining Watch Together requires existing access to the exact media.

Every participant, receiver, share, queue mutation, event, and restoration record carries the complete authority-qualified `ViewerScope` and current authorization revision. A same-string ID from another authority is a different principal.

## 3. Server invitations

### 3.1 Invitation content

An invitation binds:

- immutable invitation ID and server ID;
- owner/creator ID;
- delivery by an explicit link; no email delivery;
- recipient binding: normalized verified email, canonical username/account ID, or unbound direct-link capability as explicitly selected;
- role `user` (owner transfer is a different high-risk operation);
- library/permission template, content restriction, subordinate-profile allowance, and resource limits;
- created, expiry, accepted, revoked, and delivery state;
- one current revision and idempotency identity.

The owner reviews the effective access summary before Send: selected libraries, play/download/Live/DVR/Watch Together/feedback permissions, rating/label restrictions, subordinate profiles, bandwidth/stream/download/DVR limits, expiry, and recipient-binding type. No checkbox default may silently grant every library or administrative ability.

### 3.2 Invitation state machine

`draft -> creating -> pending-delivery -> pending`

Terminal states are `accepted`, `declined`, `revoked`, and `expired`. `delivery-failed` is retryable and does not invalidate an otherwise retrievable pending invitation. `accepting` is an atomic intermediate state.

1. Create, resend, revoke, decline, and accept are idempotent mutations.
2. Resend preserves invitation ID, recipient binding, access policy, expiry semantics, and acceptance truth. It rotates any delivery secret rather than creating parallel invitations.
3. Revoke wins over an uncommitted acceptance. Accepted membership cannot be removed by revoking the old invitation; membership revocation is explicit.
4. Accept requires an authenticated Portico Account and a current invitation secret/session. It atomically creates or returns exactly one membership for `(serverId, accountId)` and marks the invitation accepted.
5. An email-bound invitation requires the account to own that exact verified contact address. Apple Hide My Email is treated as a normal verified address; Portico never asks for or infers the hidden address.
6. A username/account-bound invitation requires that exact account ID after resolution. Username reuse or case differences do not change the bound immutable account.
7. A direct-link invitation intentionally allows whichever authenticated account proves possession first. The confirmation page says this plainly and shows the account about to accept.
8. Link secrets are high entropy, expire, are stored hashed, and are removed from the browser address before account continuation. They never appear in referrers, analytics, logs, screenshots generated by diagnostics, or server policy snapshots.
9. Acceptance succeeds even if the server is offline. It creates Hosted membership; actual server connection is a later networking/selection step.
10. Acceptance failures use stable recovery semantics without exposing private recipient or invitation details. A malformed or unknown code returns `invite_not_found`. A known invitation presented by an account that does not satisfy its recipient binding returns `invite_wrong_recipient` regardless of the invitation's state. Only after recipient eligibility succeeds may the service return `invite_expired`, `invite_revoked`, or `invite_consumed`; direct-link possession is the recipient proof for this rule. These codes select reviewed guidance only and never include the recipient, inviter, server, prior accepter, raw diagnostic, or secret.

### 3.3 Invitation privacy

While pending, the inviter may see the address/username they entered and delivery status. Other members do not. The recipient sees inviter display identity, server name, access summary, expiry, and any material restriction before accepting.

After acceptance, ordinary membership surfaces use Portico username and profile/display identity. They MUST NOT reveal contact email, invite address, Apple relay/private-email flag, sign-in provider, role-internal IDs, or authentication metadata. Declining does not reveal another account identity to the inviter.

Enumeration-resistant search returns only the minimal resolved display identity needed for confirmation and is rate limited. Entering an email MUST NOT disclose whether a Portico Account exists before an authorized send result.

## 4. Membership lifecycle and propagation

The local server owns membership and access. Owner changes commit local policy and fence relevant operations; a durable membership journal pushes discovery-index updates to Hosted. Hosted presence/route/index evidence never grants access by itself. Departures and signer revocations are learned through checked wake/check-in paths.

There is no seven-day Hosted membership lease or policy-update-pending fiction for a local committed change. Client cached role presentation still needs quiet refresh when authority changes. Invitation secrets and saved-resource grants retain independent expiry/revocation and exact owner checks.

## 5. Saved-resource sharing

Playlist and collection sharing is server-local and limited to eligible active server members. Shares have `viewer` or `editor` rights.

- Owner controls title, summary, visibility, share list, and deletion.
- Editor may change contents/order only when the resource contract permits; editor cannot reshare, transfer ownership, change visibility, or delete.
- Viewer cannot mutate.
- A shared resource never exposes media the recipient cannot otherwise access. Inaccessible entries appear only as an unavailable count/placeholder when needed to preserve ordering; private metadata is not leaked.
- Candidate search returns display identity only. No email/login/permission disclosure is permitted.
- Removing a share immediately removes resource access but does not remove server membership or media access.

## 6. Playback destinations and discovery

The destination picker may combine sections for **This device**, Google Cast, AirPlay/system routes, and authorized Portico players, but each target declares its transport and capabilities. A client MUST NOT present unsupported volume, seek, queue, Live, subtitle, audio-track, or quality controls.

Discovery alone is not authorization. Target name, type, current connection state, and capability facts may be shown. Recent targets must be re-discovered or server-verified before use; a cached friendly name cannot receive credentials.

Selecting a target has states:

`idle -> discovering -> target-selected -> connecting -> preparing -> transferring-authority -> remote-playing`

Side/terminal states are `connection-failed`, `handoff-failed-local-retained`, `reconnecting`, `disconnecting`, and `disconnected`.

The picker remains usable without active playback. Selecting a target with nothing playing makes it the next playback destination only when the transport supports durable selection; otherwise it explains that media must start first.

### 6.1 Mobile sender implementation baseline

A mobile build advertises Google Cast support only when the complete sender path is present and configured for Portico's Custom Web Receiver. A route button, device picker, or native dependency by itself is not support. iOS/iPadOS and Android phone/tablet senders MUST:

- use Google's supported platform Sender SDK and its standard Cast button, discovery, session manager, automatic reconnect/resume, and remote-media state rather than a Portico discovery or state imitation;
- initialize the Cast context with the exact configured Custom Receiver application ID and declare every platform-required local-network, discovery-service, manifest/provider, and background/foreground-service capability;
- expose active playback outside the full player through the platform-appropriate mini controller and expanded/notification/lock-screen controls, without creating a second playback authority;
- send the same versioned Portico bootstrap, command, terminal, and reconciliation model through one cross-platform client adapter; neither mobile platform may own a divergent transfer protocol;
- derive controls from receiver capabilities and authoritative receiver state, survive sender background/process recreation, and reconnect to the same receiver session/generation without reloading media; and
- hide or clearly disable Cast when the receiver ID, required SDK integration, compatible Client Core/Server contract, or exact reconciliation capability is unavailable. A connected-but-nonfunctional button is prohibited.

Television receiver builds, Android TV, Fire TV, and tvOS do not become Cast senders merely because they share React Native player code. Sender support on those targets requires a separately approved product capability and platform conformance evidence.

### 6.2 First-party Portico receiver authorization

Receiver registration and control are separate from viewer login. The current implementation binds receiving device identity to its authenticated server session, uses the server's receiver/handoff routes and retains the committed destination session for pause/resume/seek/stop. Server session-change events must reach the actual destination player. Control disconnection is not Stop.

The earlier per-device X25519 direct-LAN command design is superseded as an implementation description. Do not infer possession keys, sealed commands or a second protocol from historical schema references. Authorisation, receiver identity, exact handoff generation and readiness fences remain required. Paused preparation must not emit an audible first frame; that client boundary remains unfinished.

Cast uses its separate reviewed pairing proof and narrow playback-only member session. A failed takeover cannot stop the working receiver. Reconnection needs installation-private controller proof, keyed by stable receiver identity rather than friendly name. Duplicate TV-name storage and Android Cast support remain gaps.

## 7. Google Cast / Portico receiver contract

### 7.1 Topology and bootstrap

The centrally hosted CAF receiver is a static shell, not a Portico Account client, server directory, media proxy, or analytics service. It talks directly to the already selected Portico Server.

1. Receiver creates an ephemeral proof key and challenge.
2. Sender requests an exact-server, exact-profile, exact-playback bootstrap bound to receiver ID, configured receiver origin, challenge, capabilities, client instance, source session, and generation.
3. Server returns a short-lived (normally 60-second), one-use envelope encrypted to the receiver proof key. Cast custom data carries only ciphertext and non-secret correlation identifiers.
4. Receiver proves key possession and atomically redeems directly with the selected server for a receiver-scoped session plus media grant.
5. Receiver credentials use a dedicated authorization scheme and are restricted to one receiver session, profile, generation, origin, and operation set. Media grants are separate and narrower.
6. CAF request handlers attach media authorization only to the exact verified server origin and approved manifest/variant/segment/subtitle families. Off-origin redirects and credential query strings fail closed.
7. Receiver renews its own grant/session and reports progress. Sender sleep/departure does not strand playback.

### 7.2 Two-phase authority handoff

PC-PLAYBACK document 32 owns the single handoff protocol for first-party receivers, Cast and return-to-local playback. The source continues during preparation. The candidate prepares paused/unpresented with only the bounded staging authority allowed by that contract. Readiness permits an explicit atomic commit; first-playing is post-commit observation, not a second or alternative commit trigger. Commit transfers the exact ownership revision and existing stream slot; independent sessions are unaffected.

Failure before commit leaves the source intact. An ambiguous commit reconciles the same durable handoff identity and cannot create a second target or guess source resumption. After a committed transfer, recovery follows current ownership; a late sender or target event cannot restore the previous actor. Platforms unable to prepare safely use the contract's explicit serial attachment path.

Handoff preserves media/version, position, track intent, requested quality, queue/revision, repeat/shuffle, profile and server while replanning for the receiver. The receiver owns subsequent renewal, progress and queue advancement. Control disconnection is not Stop.

### 7.3 Remote control and lifecycle

- Command mutations use command ID/idempotency key plus expected generation. Duplicate commands return the accepted result; stale commands never affect the player.
- Receiver owns Up Next, automatic queue advance, completion, passout protection, segment skipping, progress, and renewal while sender is absent.
- Sender reconnect reads server/receiver state and resumes control; it does not reload media or reset queue.
- Disconnect means detach controller while receiver continues. Stop means end playback and revoke the receiver session. The UI MUST distinguish them.
- Receiver timeout/network loss enters reconnecting while media buffer remains viable. Server revocation, membership/profile loss, Stop, or terminal expiry ends playback.
- Multiple authorized senders for the same profile MAY control the session only after server-issued controller authorization. Discovery presence alone cannot control it.

## 8. AirPlay contract

AirPlay uses Apple's system route picker and media-route UI. Portico does not imitate the picker or send Portico server credentials to an arbitrary AirPlay endpoint.

For ordinary AVPlayer AirPlay:

- the originating Portico player/session remains playback and progress authority;
- the OS transports/render routes the media and reports route/player state;
- Portico continues progress, queue, quality/track intent, and session renewal through the originating app;
- app suspension follows the OS background playback contract and MUST NOT be interpreted as Stop;
- the originating player observes the system's active external-playback state and does not apply ordinary local-video background pausing while AirPlay remains active;
- route and capability changes are published as generation-fenced player events, not inferred from view visibility or app foreground state;
- route loss returns to the local player when the OS reports a safe continuation, otherwise pauses with recoverable route-lost state;
- local and system volume behavior follow the route capability; unsupported volume controls are absent.

If a future AirPlay target runs a first-party Portico receiver with independent credentials, it follows the receiver two-phase handoff contract rather than this originating-player model.

## 9. Server switching during remote playback

A remote playback session is pinned to its originating server and profile. Switching the browsing context to another server MUST NOT retarget it, title-match its queue, or silently stop it.

The client offers an explicit choice when the switch would hide the controller:

- **Keep playing and switch**: retain a compact controller labelled with the originating server while browsing the new server;
- **Stop and switch**: stop/revoke remote playback, then switch;
- **Cancel**.

If the platform cannot safely retain two verified server contexts, it must offer Stop and Cancel and explain the constraint. Account sign-out stops/revokes remote playback and clears controller credentials. Membership revocation on the originating server ends it regardless of the current browsing server.

**Keep playing and switch** creates a detached operation-scoped controller context, not a second general viewer runtime. It holds only the originating server identity, receiver/playback session, controller grant, authorization revision, minimal presentation, and Stop/control operations. It cannot browse, search, fetch unrelated artwork, mutate Saved state, or issue a new playback session. The ordinary runtime switch still fences and drains the old general server context. The detached controller ends on playback end, Stop, sign-out, authorization revision, grant expiry, or receiver loss.

## 10. Watch Together eligibility and privacy

Watch Together is scoped to one selected Portico Server. Every participant must:

- have an active account/profile/server session;
- have `watch-together.participate` plus `media.play` permission;
- have current access to the loaded item and every item when it becomes current;
- use a client that advertises the required Watch Together operations and compatible playback capability.

The server-hosted group directory and server-issued invitations reveal only group name, host display profile, media presentation allowed to the viewer, participant display profiles, readiness, and expiry/activity state. They never reveal emails, usernames used for login, roles, source paths, IP addresses, exact network quality, unrelated server memberships, or hidden profiles. No consumer Hosted push or email carries this room or media presentation.

Groups are discoverable only to explicitly invited profiles or under a server-owner-approved eligible-members policy. Portico v1 SHOULD use explicit invitations; a server-wide public room is not implied.

## 11. Watch Together state and roles

Group states:

`creating -> lobby -> preparing -> ready -> playing | paused -> ending -> ended`

Side states are `host-reconnecting`, `degraded`, and `failed`.

Participant states:

`invited -> joined -> preparing -> ready -> playing | paused | buffering | away | disconnected -> left`

The creator profile is host. Host alone controls group play/pause/seek/load/next/previous/stop, queue mutation, shuffle/repeat, readiness override, and End Group. Participants control local volume, caption appearance, and their permitted audio/subtitle streams. A participant's local Pause request is a host command request, not an unsynchronized local pause disguised as group state.

This host-authority model intentionally differs from Plex's historical any-participant control. It prevents conflicting commands and makes revision ownership obvious. A future democratic-control option requires a contract change.

The host may explicitly transfer host authority to an eligible ready participant using a revisioned transaction. Without transfer, explicit host Leave ends the group. Temporary host disconnect does not immediately end it: group enters `host-reconnecting`; current playback may continue, but no new authoritative commands are accepted. At two minutes without the same host authority returning, the group pauses atomically. At ten minutes it ends. A reconnect before two minutes restores the prior playing/paused state; a reconnect after the pause boundary returns to paused and requires an explicit host Play.

## 12. Lobby, readiness, and start

1. Joining performs authorization and current-item compatibility checks before publishing membership.
2. Each participant prepares its own server playback plan, media grant, decoder, tracks, and initial buffer. Track and quality choices may differ; timeline/media identity may not.
3. `ready` means the participant can start the exact media at the commanded position with its current plan. A mere HTTP success or metadata load is not ready.
4. The lobby shows joined/preparing/ready/unavailable states. The host may start when all joined participants are ready or explicitly **Start anyway** after seeing who is not ready.
5. Late joiners prepare and then load the extrapolated current timeline. They do not restart or pause the group.
6. One member buffering does not automatically pause everyone. The member reports buffering and catches up on recovery. Host may choose Pause. This avoids feedback loops and griefing; another global policy would require a contract change.

## 13. Authoritative synchronization

The server snapshot contains at least:

- `groupRevision` for membership/settings/queue changes;
- `playbackRevision` for authoritative timeline/media commands;
- `reconnectGeneration` for stream/resubscription lineage;
- media ID, state, position anchor, `positionUpdatedAt`, `serverTime`, playback rate, queue revision, and last command ID.

Rules:

1. Host commands require expected group revision and idempotency key. Commit updates group state, playback revision when applicable, command receipt, and event publication atomically.
2. Same key/body returns current materialized state plus receipt evidence. Same key/different body is rejected.
3. Participants ignore lower reconnect generation, lower revision, lower playback revision, and exact duplicates.
4. Target position while playing is `anchorPosition + elapsedServerTime * authoritativeRate`, bounded to the playable timeline.
5. If media differs, participant loads the authoritative media/position/state.
6. While locally buffering, participant reports status and does not issue corrective seek/rate feedback.
7. Canonical correction bands:
   - under `0.75s`: no correction;
   - `0.75s..<3s` while playing: temporary rate correction bounded `0.90..1.10` for at most four seconds;
   - `>=3s`: authoritative seek with paused/playing state applied atomically.
8. A platform unable to perform bounded rate correction MAY use a seek at a published platform threshold, but requires a documented capability adaptation and equivalent convergence evidence.
9. Local commands generated while disconnected are not replayed as group commands. On reconnect, current server snapshot wins.
10. Event transport may be SSE, WebSocket, or constrained polling. Transport cannot define different group semantics.

## 14. Queue, media access, and personal state

The group queue follows `PC-SAVED-DOWNLOADS`: revisioned order, idempotent mutations, stable items, deterministic shuffle, and no implicit playlist persistence. Every queued item is checked against all current participants before acceptance. Authorization is checked again when it becomes current because membership/library/profile policy may have changed.

Before a late participant receives a group snapshot, the server computes that participant's visibility for the loaded item and every existing future queue entry. An inaccessible future entry is represented only by an opaque unavailable placeholder preserving queue position; its title, artwork, metadata, source, and owner are omitted. The host receives an eligibility summary and must remove the participant or replace/remove the entry before it can become current. Joining never turns an old queue into a metadata side channel.

If an item is no longer accessible to one participant, the server does not leak its metadata. The host receives an eligibility summary and may remove the participant or choose another item. Portico MUST NOT silently eject the participant and start private media without a host-visible decision.

Every participant owns a separate playback session/progress stream for personal history. Group timeline synchronization does not write the host's progress into other profiles or collapse multiple participants into one user state. Completion, watched state, and Last Played update for each participant whose accepted session evidence meets the normal thresholds.

## 15. Failure and recovery

Canonical families include `target-unavailable`, `handoff-timeout`, `receiver-load-failed`, `route-lost`, `controller-detached`, `server-offline`, `membership-revoked`, `media-no-longer-accessible`, `group-full`, `host-unavailable`, `revision-conflict`, `participant-incompatible`, and `client-update-required`.

- Retryable target/handoff failures retain local playback and current screen.
- Receiver reconnect uses existing session/generation; it never starts a duplicate.
- Watch Together event-stream interruption retains last snapshot, marks connection state, and polls/reconnects with backoff. On return, a full current snapshot precedes deltas.
- Group end is durable and terminal. Delayed events/commands cannot resurrect it.
- A server restart reconstructs active groups/receiver sessions from durable state or ends them explicitly; fake successful state is prohibited.
- If the user switches profiles, the old profile leaves/detaches before the new profile can discover or control the group/session.

## 16. Platform adaptation

- Google Cast uses the independent receiver model; AirPlay normally uses originating-player authority.
- Television clients may be receiver/control targets and may create/join Watch Together when all required operations exist. Tiny hyperlink actions and pointer-only controls are prohibited.
- Constrained clients may use polling instead of streaming events and hard seeks instead of rate correction only as declared capability adaptations.
- A platform without provider SDK/route support hides that destination type. It cannot offer a button that opens an explanatory dead end.
- UI may vary between sheet, popover, full-screen TV page, and system route picker, but target state, Disconnect vs Stop, server label, host/participant role, and failures remain explicit.

## 17. Security requirements

All receiver bootstraps are one-use, proof-of-possession bound, origin/audience scoped, and short lived. Receiver/controller/media credentials are separate and never placed in URLs, Cast custom data as bearer values, logs, analytics, or referrers. Configured receiver origins are exact allowlists; arbitrary reflected CORS is prohibited.

Group IDs, invitation IDs, membership IDs, receiver IDs, and server IDs are not authorization. Every read/event/mutation rechecks the current principal and scope. Event streams close promptly after revocation and cannot continue leaking participant or media state.

Abuse controls rate-limit invitation/search/group creation/join/commands independently. Malformed/unknown invitation secrets remain enumeration resistant. State-specific recovery codes are allowed only after a high-entropy code resolves to a known invitation and the authenticated account satisfies its recipient binding (or the invitation is intentionally unbound); all other accounts receive the same `invite_wrong_recipient` result independent of state. Blocking/reporting a global Portico user is out of scope; server owners can revoke membership and group hosts can remove participants.

## 18. Conformance

`state-machines.yaml` defines normative vocabulary. `conformance-vectors.yaml` defines minimum hostile cases. Evidence includes:

`remote-playback-acceptance-matrix.yaml` is the canonical physical-device gate. A result is valid only when it binds the named app build, OS/device, Sender SDK, receiver application/device, Server/OpenAPI, Client Core artifact, case outcome, and evidence reference required by that artifact. Every applicable case must pass; omissions and ungoverned waivers fail the profile.

- invitation duplicate/lost response, resend/revoke/accept race, expired link, wrong account binding, Hide My Email, and offline-server acceptance tests;
- signed policy generation rollback, expiry, push-hint spoof, permission narrowing, and session revocation tests;
- Cast wrong key/origin/app/server/profile, replay, lost-ready, two-phase handoff, reconnect, sender departure/process recreation, stale command, renewal, queue advance, Cast-to-local replacement, and distinct Stop/Disconnect tests on both supported mobile sender platforms;
- platform integration checks for the exact Custom Receiver ID, required iOS discovery services, Android `OptionsProvider`/manifest metadata, standard Cast button, current supported Sender SDK, session resume, and mini/expanded/notification controller surfaces;
- AirPlay active-route background, route/capability change, route loss/local return, interruption, PiP interaction, and single-progress-authority tests on Apple platforms;
- physical iOS-to-Cast, Android-to-Cast, and Apple-to-AirPlay acceptance on registered receivers before a release build advertises the corresponding capability; simulators and mocks are necessary preflight evidence but do not satisfy this gate;
- Watch Together invite/access/lobby/readiness, concurrent command, duplicate receipt, out-of-order event, drift, buffering, host disconnect, queue access change, server/profile switching, and per-participant progress tests;
- cross-platform interaction/accessibility matrices and credential/log scanning.

### 18.1 Applicability matrix

| Rule | Server | Hosted | Web | Client Core | Apple | Android | Roku | Cast | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| invitations and membership policy | enforcement/projection | authoritative membership | required management/acceptance | canonical outcomes | required | required | consume active membership | not applicable | invitation/policy fixtures |
| Cast bootstrap and controller | issues grants/session | discovery hint only | sender where offered | canonical handoff | sender/controller | sender/controller | target/controller where offered | receiver required | replay/origin/handoff fixtures |
| Watch Together group authority | sole authority, coordinator, directory, and invitation-ticket issuer | not applicable; may deliver only an opaque wake notification and signed account/membership policy | required where offered | canonical state/revisions | required where offered | required where offered | required where offered | participant only if supported | drift/host/privacy fixtures plus Hosted no-room-state test |
| profile/server fencing | required | policy authority | required | required | required | required | required | required | switch/revocation tests |

An approved exception uses this exact record from `../governance.md`, with boundary evidence:

```text
PORTICO-CONTRACT-EXCEPTION
Contract: PC-SHARING-SOCIAL#<section-id>
Scope: <platform, surface, or capability>
Decision: <the exact deviation>
Reason: <why the canonical rule cannot or should not apply>
Approved-by: <name or recorded decision reference>
Review-when: <expiry date, capability change, or other objective trigger>
Evidence: <test or fixture that prevents accidental expansion>
```

The approved record MUST also appear in this contract's exception registry. A comment without boundary evidence is not an approved exception.

## 19. Product decisions

Watch Together is fully coordinated by the Portico Server hosting the media. It uses explicit invitations rather than server-wide public rooms, host-only authoritative controls, no automatic group pause for one buffering participant, a two-minute host-disconnect pause and ten-minute end, and a keep-playing server switch only where the platform can retain the narrow verified controller defined in section 9. Hosted Services never becomes the room authority. Clients may not choose different social rules by platform.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.

## October social implementation boundary

Watch Together keeps explicit invitations, host-only commands, Start anyway, no automatic group pause for one buffering member, 0.75/3-second correction bands and two/ten-minute host-loss pause/end. A seek/title load now holds the group clock for up to five seconds, ending early when current readiness permits, while preserving playing/paused intent. Heartbeat, readiness, queue, commands and governance have separate fences. Silent non-host members leave after ten minutes; post-deadline self-claim is explicit.

Hidden current/future titles are redacted from snapshots/events/receipts. Private participants use private presentation. Handoff retains a controllable destination session; Cast continues independently of the phone. Paused initial loads, remote session-event delivery, native proof storage and actual Cast/timing/device acceptance remain gaps. No new chat, remote keyboard, D-pad or volume protocol is introduced.

Server invitations may target a Portico username. No mail is delivered. Residual verified-email binding and Hosted-owned membership text in the older sections are not proof of current local-server enforcement; local membership remains final media authority, as the Hosted architecture describes.

## October scope and evidence

Separate group fences and persistent receiver control exist. Paused first-frame, remote command delivery, duplicate receiver names and physical Cast/TV acceptance remain open.

Sources: [social](../../../_program/reports/social.md), [native-sync](../../../_program/reports/native-sync.md), [accounts-security](../../../_program/reports/accounts-security.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
