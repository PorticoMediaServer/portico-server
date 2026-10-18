# 13 — Sharing and multi-device playback: Watch Together, remote control, casting, sessions, sharing and notifications

Auditor's note: this is a static source review. Nothing here was run. Where a finding says "happens", read it as "the code as written does this"; device and network behaviour was not observed.

## Scope and sources

**Domain.** Watch Together (group playback), "Play on" (moving playback to another Portico device), Google Cast, AirPlay, DLNA, who-is-watching dashboards, how a server is shared with other people (invitations, restrictions, links), and in-app notifications.

**Portico, read in full unless noted**

| Area | Files |
|---|---|
| Docs | `portico-internal/Project Architecture/04 Playback and Media.md`, `Backlog.md`, `portico-internal/Product Contracts/sharing-and-social-playback/contract.md`, `apps/portico-server/server/api/social.md`, `notifications.md`, route lists of `social.openapi.yaml` and `notifications.openapi.yaml`, `portico-internal/cast-receiver/README.md` |
| Server: groups | `server/internal/social/{store,groups,transport,queue,invites,events,privacy,sweeper}.go` |
| Server: Play on and Cast | `server/internal/social/{receivers,handoff,cast,receiver_events}.go`, `server/internal/playbackv1/social.go`, `server/receiver/cast/receiver.js` |
| Server: routes | `server/internal/httpapi/social_routes.go` (all), `restriction_admission.go`, the notification stream limits in `notifications_routes.go`, `route_lanes.go` (social lanes) |
| Server: sharing | `server/internal/access/{invitations,members}.go` (all), `limits.go` and `identity/restrictions.go` (shapes only) |
| Server: notifications | `server/internal/notify/notify.go` (all), `producers.go` (signatures), broadcast and audiences in `operations/notifications.go` |
| Shared client logic | `packages/client-core/src/{group-session,social-playback,remote-playback}.ts`, `cast/{protocol,sender}.ts` (all); `activity.ts`, `playback-v1/admin-sessions.ts`, `account-notifications.ts` (skimmed); the seek path in `client-core/src/index.ts` (lines 1324–1380) |
| Web | `web/src/app/together.tsx`, `web/src/screens/together/Together.tsx`, `web/src/bridge/{cast,sync-rate}.ts` |
| Native | `apps/native/src/app/{together,cast,receiver}.tsx`, `src/player/Destinations.tsx`, `Native/PorticoCast.m` |

**Comparison projects read** (see "How the others do it" for file lists): Jellyfin server SyncPlay and session control, jellyfin-web SyncPlay and remote-control plugins, Plezy Watch Together, its relay server and its companion remote, Streamyfin casting, plus public documentation for Plex, Emby, Apple SharePlay and the Jellyfin Chromecast receiver.

**Not reached.** Android `PorticoCast.kt`, `PorticoNetwork` (Bonjour) internals, `PorticoAudioRoutePicker`, native `Together.tsx`/`InviteSheet.tsx`/`Invitation.tsx`/`ServerMembership.tsx`/`Notifications.tsx` screens beyond their calls into client-core, `inbox.ts`/`notifications.ts` client stores, the Vega override of `remote-playback.ts`, the Hosted side of membership, and the `internal/apievents`/`eventfeed` ledgers. Server invitations and member limits belong mostly to the identity domain; they are covered here only as "how is a server shared".

## How Portico does it today

### Watch Together

A **group** is a row in `social_groups` on one server. Everyone in it must already be signed in to that server. There is no cross-server group and nothing goes through Hosted.

**The server owns a clock, not a player.** The group stores four numbers: the item, a position (`position_us`), the wall-clock moment that position was true (`position_updated_ms`) and a rate. While the state is `playing`, where the group "is" equals `position + (now − moment) × rate` (`social/groups.go:344`). Each member plays the title in their **own** ordinary playback session, with their own quality, audio and subtitle choices, and their own watch history. Members steer themselves onto the clock; the server never commands anyone's player (`api/social.md` §1).

**Commands.** `POST /v1/groups/{id}/transport` accepts `play`, `pause`, `stop`, `seek`, `load`, `next`, `previous`, `set-queue-position` (`social/transport.go:163`). Each command must carry the group **revision** the caller last saw (a stale one is refused with `409 revision_conflict`) and an **idempotency key** (a retry returns the stored receipt). By default only the **host** may send them (`host-only`); the creator can instead open the group as `anyone`.

**Following.** `GroupFollower` in `client-core/src/group-session.ts:320` runs once a second on each member:

1. If the local player has a different title, load the group's title at the group position.
2. Match play/pause.
3. Measure drift (group position minus local position) and apply the server-published bands: under 0.75 s do nothing; 0.75–3 s speed up or slow down by 10 % for at most 4 s; 3 s or more, seek.
4. Tell the server how this member is doing (`ready`, `buffering` or `lagging`) at least every 10 s.

**Clock sync.** Every answer carries `serverTime`. The client treats each request as an NTP-style sample (offset = server time − request midpoint), keeps the last 8 within two minutes, and uses the one with the shortest round trip (`group-session.ts:39–129`).

**Events.** `GET /v1/groups/{id}/events` is a Server-Sent Events stream backed by a durable per-group ledger (512 events retained, resumable by ordinal). The client applies `group.transport` at once and re-reads the whole group after every other event, because members and the queue are projected per viewer.

**Lobby and readiness.** States run `lobby → preparing → ready → playing ⇄ paused → ended`. Loading a title puts the group in `preparing`; it becomes `ready` when every member reports `ready`; the host presses Play (or "Play anyway"). **One member buffering never pauses the others** (deliberate, contract §12.6).

**Host loss.** Measured from the host's last heartbeat or command: silent 30 s → `host-reconnecting`; 120 s → the group pauses; 600 s → it ends (`groups.go:644`). A process-wide sweeper applies this every 5 s. While the host is "reconnecting", any member may claim the host role for themselves.

**Queue.** Up to 256 entries, mutated under a separate `queueRevision`. An entry a member is not allowed to see is sent to that member as an opaque placeholder; the host gets a list of who is blocked by what; the group refuses to start a blocked entry unless the host overrides (`queue.go`, `transport.go:372`).

**Joining.** The host mints an 8-character code (15 minutes, 8 uses by default); another signed-in member of the same server types it (`invites.go`). Web also accepts `/together/join#code=…`. Limits: 32 members, 12 codes per group per hour, 20 join attempts per viewer per hour. A profile restriction "Allow Watch Together" gates every `/v1/groups` route (`restriction_admission.go:44`).

**Not present:** chat, reactions, voice, a kick/remove-member action, automatic advance to the next queue entry, any use of shuffle/repeat by a client, a per-member ping display, cross-server groups, guests without an account.

### "Play on" (Portico-to-Portico handoff)

A TV app registers itself as a **receiver** (`POST /v1/receivers`). A phone lists receivers, keeps those it can also see on the local network by Bonjour (`_portico-player._tcp`), and asks for a **grant**. The first time, the TV shows "Allow this phone?" (`grantPolicy: per-device`).

The move itself is a three-step **handoff** (`social/handoff.go`):

1. **Prepare.** The phone posts the playback session it wants to move. The phone keeps playing.
2. **Readiness.** The TV finds the handoff in its inbox (it polls every 30 s, or is nudged through the notifications event stream), starts the title in its own session, and only when it is really playing reports `playing`.
3. **Commit.** The phone commits; the server ends the phone's session in the same transaction with reason `transferred`.

Anything that fails before the commit leaves the phone playing. A handoff expires after 120 s.

Receivers, grants and handoffs are all scoped to **one exact profile**: a phone can only send to a TV signed in to the same profile on the same server. After the commit the phone has **no remote control** of the TV; the sheet says to use the TV's own remote (`Destinations.tsx:83`).

### Google Cast

One custom receiver page (`server/receiver/cast/`, also published at `cast.getportico.tv`). Flow:

1. The sender asks its server for a one-use 6-character code (`POST /v1/cast/bootstrap`, 5 minutes).
2. It sends `{pair, code, origin}` to the TV over a custom Cast channel.
3. The TV page redeems the code at that origin (`POST /v1/cast/redeem`, unauthenticated) and gets two credentials: a **device token** (kept in `localStorage`, good only for `/v1/cast/reconnect`, rotated on every use) and an **ordinary viewer access token**.
4. The sender sends `{load, itemId, startSeconds}`. The TV opens a normal Playback v1 session with a Cast device profile, plays the HLS or direct URL the server answers with, and reports progress itself.

The sender that paired is the **controller**; other senders may only pause or stop. A second person pairing while something plays triggers an on-TV "X wants to play" prompt and a question to the controller.

Cast needs an **HTTPS** server origin. Subtitles on Cast are limited to those in the stream (Backlog). The receiver has no queue: one title at a time.

### AirPlay, DLNA

AirPlay is the system route picker on Apple devices; the app that started playback stays in charge. There is no server involvement. DLNA is not implemented and is described as a deliberate omission (`social.md` §5); the Backlog lists it as a planned, off-by-default feature.

### Who is watching

Owners get a **Now Playing** list (`/v1/admin/sessions`, refreshed by events) with each session's delivery decision and a **terminate with message** action, plus the older Activity list with **Stop** (`client-core/src/playback-v1/admin-sessions.ts`, `activity.ts`).

### Sharing a server

An owner or admin creates an **invitation**: role (member or admin), a list of allowed libraries, an expiry (1 hour to 30 days), and an optional email used only as a label. The result is one long code shown once. The invitee either creates a local username and password with it or accepts with a Portico Account (`access/invitations.go`). Nothing is emailed. Per-member limits (streams, schedule, channels, ratings, denied labels) and per-profile restrictions (rating ceiling, blocked labels, downloads/Live TV/DVR/Watch Together switches) are set afterwards. There are no public links to a single title and no guest access.

### Notifications

A per-profile **inbox** on the server, plus an owner-only "account-admin" inbox (`internal/notify`). Ten producer categories (download finished/failed, DVR conflict, new device, password change, scan failed, certificate expiring/expired, low storage, feedback received/updated, owner broadcast). Delivery is in-app only: an SSE stream or a long poll, both resumable by one monotonic revision. No push, no email, no webhooks (recorded decision). A separate Hosted feed carries account-level notices (server offline, storage nearly full, invitation accepted, new device, security events).

Path shorthand used below: `S/` = `apps/portico-server/server/internal/`, `CC/` = `apps/portico-server/packages/client-core/src/`, `WEB/` = `apps/portico-server/web/src/`, `RN/` = `apps/portico-react-native/apps/native/src/`.

## How the others do it

### Jellyfin

**SyncPlay** (`jellyfin/Emby.Server.Implementations/SyncPlay/{SyncPlayManager,Group}.cs`, `jellyfin/MediaBrowser.Controller/SyncPlay/GroupStates/*.cs`, `jellyfin/Jellyfin.Api/Controllers/SyncPlayController.cs`; client in `jellyfin-web/src/plugins/syncPlay/core/{PlaybackCore,TimeSync,QueueCore,Manager}.js`).

- **Groups live in memory.** A server restart ends every group. Nothing is stored.
- **Members are sessions (devices), not people.** One user can have a phone and a TV in the same group.
- **No host.** Anyone in the group may play, pause, seek, change the queue. Who may use SyncPlay at all is one per-user setting with three values: create and join, join only, none (`SyncPlayUserAccessType`).
- **Groups are discoverable.** `ListGroups` returns every group whose whole queue the caller is allowed to see. There are no invite codes.
- **Access rule is all-or-nothing.** A user who cannot see every item in the queue cannot see or join the group (`SyncPlayManager.JoinGroup` → `SyncPlayLibraryAccessDeniedUpdate`), and nothing may be queued that any current member cannot see (`Group.AllUsersHaveAccessToQueue`).
- **Four states: Idle, Waiting, Paused, Playing.** The important one is **Waiting**. Any seek, item change, new member or member who reports buffering moves the group to Waiting: everyone is told to pause at the exact position, each client seeks, pauses and sends `Ready` with its position and local time, and only when nobody is buffering does the server send `Unpause` (`WaitingGroupState.cs`).
- **Commands are scheduled in the future.** Every command carries `When` (a server timestamp) and `PositionTicks`. `Unpause` is stamped `now + 2 × the highest ping in the group` (minimum 500 ms), so every client can start at the same instant (`PlayingGroupState.HandleRequest(UnpauseGroupRequest)`). Clients report their measured ping; it is clamped to 10 s (`Group.MaxPing`).
- **Readiness is checked, not trusted.** A `Ready` whose position is more than 500 ms from the group position is answered with a corrective `Seek` to that one client (`MaxPlaybackOffset`).
- **A stuck member cannot hold the group forever.** A member can opt out of making others wait (`IgnoreGroupWait`), anyone can press Play again to force a start, and the server gives up waiting after 30 s (`Group.GroupWaitTimeout`, `HandleGroupWaitTimeout`).
- **Clock sync** is four-timestamp NTP against a dedicated cheap endpoint (`GET /GetUtcTime` returns request-received and response-sent times). The client keeps 8 measurements and uses the one with the least delay; three quick pings, then one a minute (`TimeSync.js`).
- **Drift correction during playback is off by default** (`enableSyncCorrection` false). When turned on: between 60 ms and 3 s the player's speed is set to `1 + drift / 1 s` for one second (so 1 s behind means 2× for a second); otherwise, above 400 ms, it seeks. After each correction, correction is disabled for a short time. All five thresholds and an **extra per-device time offset** are user settings (`jellyfin-web/src/plugins/syncPlay/ui/settings/editor.html`).
- **Queue** is a real shared play queue with shuffle, repeat one/all, play next / play last, move, remove, and **automatic next item**: each client asks for the next item when its copy ends, and the request names the item it thinks is current so duplicates are dropped (`NextItemGroupRequest`, `PlayQueueManager.cs`).
- **Positions are clamped** to the item's runtime (`Group.SanitizePositionTicks`).
- **Leaving is automatic** when a session ends (`SyncPlayManager.OnSessionEnded`); an empty group is removed.
- **Client coverage is thin.** Only jellyfin-web (and the apps that wrap it: Android phone, desktop) can take part. Swiftfin, the Android TV app, Wholphin and jellyfin-vue only expose the admin permission.
- No chat, reactions or voice.

**Remote control** (`jellyfin/Jellyfin.Api/Controllers/SessionController.cs`, `jellyfin/Emby.Server.Implementations/Session/SessionManager.cs`, `jellyfin-web/src/plugins/sessionPlayer/plugin.js`, `jellyfin-web/src/components/remotecontrol/remotecontrol.js`).

- Every client keeps a WebSocket to the server and **registers its capabilities** (playable media types, supported commands, whether it supports media control).
- `GET /Sessions?controllableByUserId=` lists the sessions a user may control. Any client can then act as a **remote** for any other: play these items now / next / last / shuffled / as an instant mix (with start position, audio and subtitle stream), pause, seek, next, previous, stop, set volume, mute, set audio or subtitle stream, set repeat and shuffle, set max bitrate, **show this item on the other screen** (`DisplayContent`), **send a text message that pops up** (`/Message`), plus navigation keys (up, down, left, right, select, back, home, settings, search) and **send text** (`SendString`).
- **Who may control whom:** your own sessions always; other people's only with the per-user permission "Allow remote control of other users"; devices with no signed-in user only with "Allow remote control of shared devices" (`SessionManager.AssertCanControl`).
- A session can have **additional users** attached so one playback counts for two people (`/Sessions/{id}/User/{userId}`).
- Everything is server-mediated, so it works across networks, not only on the LAN.

**Chromecast** (`jellyfin-web/src/plugins/chromecastPlayer/plugin.js`; receiver is the separate public repo `jellyfin/jellyfin-chromecast`, read from GitHub).

- Custom receiver hosted by the project. The sender puts **its own access token and server address in every message** to the receiver (`plugin.js:353–359`). The receiver then talks to the server as that user.
- The receiver **registers as a Jellyfin session** with a device profile, so it shows on the dashboard and can be driven by any Jellyfin remote, not just the sender that started it.
- It has a queue (play next, play last, shuffle, instant mix, next/previous track), repeat mode, audio and subtitle stream switching, max-bitrate setting with automatic bitrate detection, volume and mute.
- The receiver's main controller has no handler that stops playback when a sender disconnects.
- Jellyfin's networking docs warn that Chromecast devices ignore local DNS (`jellyfin-docs/docs/general/post-install/networking/index.md:51`).

**DLNA** is a first-party plugin since 10.9, off unless installed (`jellyfin-docs/docs/general/post-install/networking/3_dlna.md`). It is both a media server for DLNA clients and a "play to" controller for DLNA renderers; DLNA devices appear as controllable sessions.

**Sharing.** No invitations. An administrator creates each user with a password and sets library access, parental rating ceiling, blocked/allowed tags, an access schedule, a per-stream internet bitrate limit, max simultaneous sessions, remote-access permission, download permission, and the two remote-control permissions (`jellyfin-docs/docs/general/server/users/adding-managing-users.md`). "Quick Connect" signs a new device in with a code. No public links.

**Notifications.** Alerts on the admin dashboard and an activity log; everything else is the Webhook plugin (templates for Discord, Slack, ntfy, Gotify, Pushover, SMTP and others; events such as item added, playback start/stop, user created, failed sign-in, task finished) (`jellyfin-docs/docs/general/server/notifications.md`). There is no per-user inbox.

### Plezy

**Watch Together** (`plezy/lib/watch_together/**`, `plezy/relay_protocol.json`, `plezy/server/main.go`).

- **Topology: a dumb relay.** A small Go WebSocket server (default `ice.plezy.app`, self-hostable) knows only rooms and peers. It forwards opaque messages (`broadcast`, `sendTo`), hands out reconnect tokens, and transfers the host role. It never touches media and needs no account. Limits: 8 peers per room, 100 connections, 5 per IP, 3 rooms per IP, rooms expire after 24 h (5 minutes if empty).
- **Room code:** 5 letters/digits, generated on the host.
- **The host's app is the authority,** not a server. The host broadcasts a `PlaybackState`: sequence number, the media key (`serverId:ratingKey`), a phase (`loading`, `waitingForPeers`, `paused`, `playing`), an anchor position, the host wall-clock time of that anchor, a rate, the control mode (`hostOnly` or `anyone`) and the list of peers the room is waiting on. It re-broadcasts every 2 s while playing and 5 s otherwise, so a lost message heals itself (`models/playback_state.dart`, `services/host_playback_coordinator.dart`).
- This is the same "anchor plus elapsed time" clock Portico uses, with two additions: **a `playing` state whose anchor time is in the future is a scheduled group start** (delay 750–2000 ms sized from the peers' measured round trips), and **the room waits for peers** (a stall longer than 500 ms pauses the room, with a 15 s safety timeout after which the room resumes without them).
- **Guests still need the same library item.** A guest who is not on that media server cannot follow. So it works across NAT without port forwarding for the *coordination*, but not across different media servers.
- **Guest follower** (`services/guest_playback_reconciler.dart`) is the closest thing to Portico's `GroupFollower`, and is far more defensive:
  - ticks every 500 ms; drift is the median of the last 3 samples;
  - dead band 350 ms; between 350 ms and 2 s it nudges speed by ±4 % and keeps nudging until drift is under 150 ms;
  - it **checks that the speed change took effect** and disables nudging on players where it did not; nudging is off while audio passthrough is active;
  - above 2 s it seeks to the target **plus 250 ms of lead**, then waits a settle period and a **2 s cooldown** before it may seek again;
  - it clamps the target to just before the end of the file and stops correcting when both sides have reached the end;
  - it refuses to correct when the clock offset looks wrong (an anchor that appears more than 10 s old).
- **Clock sync** (`services/clock_sync.dart`): ping/pong through the relay to the host, 8-sample window, lowest round trip wins, samples over 1 s round trip discarded, and a **clock-step detector** that throws the window away when a new offset disagrees by more than the round trips can explain.
- Host commands are debounced (seek 200 ms). Guests in `anyone` mode send control *requests* that the host applies.
- No chat, reactions, voice or shared queue.

**Companion remote** (`plezy/lib/services/companion_remote/**`, `plezy/lib/services/base_peer_service.dart`, `plezy/lib/models/companion_remote/remote_command.dart`): the phone as a remote for the TV or desktop app. LAN only. The host app broadcasts authenticated UDP beacons every 3 s; the phone connects directly over a WebSocket. Trust is "same account": both sides derive a secret from account identity (HKDF) and encrypt with AES-GCM; failed attempts lock out for 30 s. Commands: D-pad, select, back, context menu, play/pause/stop, seek forward/back, next/previous, skip intro, skip credits, volume up/down/mute/set, jump to a tab, home, search, open subtitle/audio/quality menus, fullscreen, and a state sync back to the phone. No text entry in the command list.

**`agent_control_*`** is not a user feature: it is a debug-build-only hook on the Dart VM service (`!kReleaseMode && PLEZY_AGENT_CONTROL`) that lets an automation agent read status and settings and start or stop playback. **`discord_rpc_service.dart`** publishes "now watching" to Discord, uploading the poster to the relay so Discord can show it. Plezy has no Chromecast, AirPlay-receiver or DLNA code of its own.

### Others

- **Streamyfin** (`streamyfin/components/Chromecast.tsx`, `hooks/useMusicCast.ts`, `utils/profiles/chromecast*.ts`): Google's sender SDK through `react-native-google-cast`, Google's own Cast button and expanded controller, a Chromecast device profile (H.264 and an H.265 variant), and `loadMedia` with a Jellyfin stream URL. No custom receiver logic of its own. AirPlay is the system button.
- **Plex (public information).** Watch Together was dropped from Plex's rebuilt apps in 2025 and remains only in the web app; Plex said it may return. Plex's sharing model is the reference for "friends": invite by email or username, choose libraries, restrict by content rating and label, allow or deny downloads and Live TV, managed users with PINs inside a "Plex Home". Remote control ("Plex Companion") lets one app drive another on the same account. Plex sends mobile push and email (new content, new device) and supports webhooks for subscribers.
- **Emby.** No built-in group watch was found in public release notes; third-party tools exist. Emby has session remote control and DLNA like Jellyfin (shared ancestry).
- **Apple SharePlay** (GroupActivities framework): the operating system carries the coordination and the people are already on a FaceTime call, so voice and video come for free; each participant plays their own copy. No evidence was found that Infuse supports it.
- **Teleparty-style extensions**: each viewer plays their own stream; a side panel carries text chat and reactions.

## Architectural differences

Sizes are rough: **small** = a day or two in one package; **medium** = server, client-core and screens; **large** = a new subsystem or protocol.

### Watch Together

### A-13.1 Who owns the group clock
- **Portico:** The server stores one anchor (position, moment, rate) and publishes it. Members compute where the group is and steer themselves (`S/social/groups.go:344`, `CC/group-session.ts:327`).
- **Jellyfin:** The server runs a state machine and sends each client timed commands ("unpause at 20:14:03.500 from position X"). **Plezy:** the host's app owns the anchor; a relay only forwards it.
- **Why it matters:** An anchor is stateless to follow: a late joiner, a reconnecting client and a third-party client all do the same arithmetic. Timed commands can start everyone on the same frame but need every client to receive and schedule them.
- **Assessment:** **Trade-off (subjective).** Portico's choice is deliberate (contract §13, `api/social.md` §2) and is the simpler, more recoverable model; Plezy independently arrived at the same clock. What Portico is missing is not the model but the two refinements the others add to it (A-13.2, A-13.3).

### A-13.2 Nobody waits for anybody
- **Portico:** After a seek, an item change or a late join, the clock keeps running. Each member loads or seeks on its own and then chases the clock. A buffering member is only a label on the roster. This is a recorded decision (contract §12.6, §19).
- **Jellyfin:** Any of those events moves the group to **Waiting**: all pause, all seek, all report `Ready`, then all resume together. A buffering member pauses the group too, bounded by a 30 s timeout, a per-member "don't wait for me" switch and a force-start. **Plezy:** the room pauses for a stalled peer after 500 ms, for at most 15 s.
- **Why it matters:** In Portico a seek while playing means every member lands behind by however long their own seek took, then corrects. With a transcoding member that is seconds of missed film and repeated corrections (see B-13.2, B-13.3). The others trade a short shared pause for a clean common start. Portico's stated reason is avoiding feedback loops and griefing.
- **Assessment:** **Portico behind** for seeks and item changes; **trade-off** for mid-play buffering. The decision as written covers "one member buffering"; it does not obviously cover "the host just seeked and everybody must reload". A narrow rule ("after a seek or a load, hold the clock until members report ready or N seconds pass, host may start anyway") would keep the no-griefing property. Medium: server state plus follower.

### A-13.3 Commands take effect "now", not at a scheduled moment
- **Portico:** `play` anchors the clock at the server's current time (`S/social/transport.go:297`). By the time a member hears about it (SSE hop, then its player starting) the clock has already moved.
- **Jellyfin:** `Unpause` is stamped `now + 2 × highest ping` (at least 500 ms). **Plezy:** a `playing` state with a future anchor time is a scheduled start, 750–2000 ms ahead sized from measured round trips.
- **Why it matters:** With "now", every start begins with each member a few hundred milliseconds behind; Portico hides this inside its 750 ms dead band rather than removing it. Members can sit up to 0.75 s apart indefinitely, which is audible if two of them are on a voice call.
- **Assessment:** **Portico behind.** Small: let the server set `anchorAt` slightly in the future on play and have `groupTargetPositionUs` (which already clamps negative elapsed time to zero, `CC/social-playback.ts:276`) hold members at the anchor until then.

### A-13.4 How drift is corrected
- **Portico:** Always on. Under 750 ms: nothing. 750 ms–3 s: speed ×1.10 or ×0.90 for at most 4 s (so at most 0.4 s recovered), then a seek if still out. 3 s or more: seek. The server publishes the bands (`S/social/groups.go:102`, `CC/social-playback.ts:285`).
- **Jellyfin:** Off by default. When on: 60 ms–3 s, speed = `1 + drift/1 s` for one second; else seek above 400 ms; thresholds are user settings. **Plezy:** dead band 350 ms; ±4 % until under 150 ms; seek above 2 s.
- **Why it matters:** Portico's band is the loosest of the three (750 ms vs 350 ms vs 60 ms) and its rate step cannot close most of its own band: a drift of 1.2–3 s always ends in a seek after four seconds of sped-up audio. Jellyfin's is the most aggressive and is disabled by default because it misbehaves on some players. Plezy's hysteresis (start at 350, stop at 150) avoids flapping at the band edge.
- **Assessment:** **Portico behind** on tuning, **ahead** on having one published policy for every client. Deliberate numbers (contract §13.7). Suggest: keep nudging until inside an exit threshold instead of a fixed 4 s, and tighten the dead band once A-13.3 exists. Small, client-core only, plus a contract edit.

### A-13.5 The follower has no protection around a seek
- **Portico:** `GroupFollower.tick` seeks to the clock's current position, then measures again a second later (`CC/group-session.ts:347–352`). No lead for seek latency, no cooldown, no settle time, no drift smoothing, no end-of-file handling, no check that a speed change took effect.
- **Plezy:** seeks to target + 250 ms, waits a settle window, enforces a 2 s cooldown, uses a median of 3 samples, clamps at end of file, verifies the nudge, and suspends correction if the clock looks wrong. **Jellyfin:** disables correction after each attempt for a fixed time and counts attempts.
- **Why it matters:** These guards are the scars of real devices. Without them the follower can loop (B-13.2, B-13.3).
- **Assessment:** **Portico behind.** Small to medium, all in `group-session.ts`; no protocol change.

### A-13.6 Clock synchronisation
- **Portico:** Offset = `serverTime` − request midpoint, best of the last 8 samples in two minutes (`CC/group-session.ts:117`). Samples come from whatever requests the room makes: the 10 s heartbeat (a database write on the server) and re-reads. `serverTime` is stamped part-way through request handling.
- **Jellyfin:** a dedicated read-only time endpoint returning two server timestamps, so server processing time is subtracted; burst of 3 then one a minute. **Plezy:** ping/pong to the host, samples over 1 s discarded, clock-step detection.
- **Why it matters:** Quality is similar on a quiet LAN. Under load Portico's samples include time queued behind the single database writer, and a device waking from sleep keeps stale samples for up to two minutes.
- **Assessment:** **Trade-off.** Portico avoids extra traffic (fits "no periodic chatter"). A cheap `GET /v1/time` used for a 3-sample burst on join, plus Plezy's step detector, would be small.

### A-13.7 Who may control playback
- **Portico:** Host only by default; the creator may choose `anyone` (`S/social/transport.go:84`). `anyone` widens play/pause/seek/queue only. Deliberate, and the contract explicitly contrasts it with Plex (§11).
- **Jellyfin:** everyone, always. **Plezy:** host-only or anyone, like Portico; guests' actions are requests the host applies.
- **Why it matters:** Host-only prevents fights over the remote; it also means a member cannot pause when the doorbell rings. The contract says "a participant's local Pause request is a host command request", but no such request exists in the API or clients: a member's Pause button shows "Only the host can play and pause" (`WEB/app/together.tsx:61`).
- **Assessment:** **Trade-off (subjective)**, Portico's default is defensible. The missing "ask the host" path is listed as F-13.6.

### A-13.8 The host role and what happens when the host goes away
- **Portico:** One host. Host leaves → group ends for everyone (`S/social/groups.go:404`). Host silent 30 s → "reconnecting"; 2 min → group pauses; 10 min → group ends. Any member may take over once the host is "reconnecting".
- **Jellyfin:** no host; a group lives until its last session leaves. **Plezy:** host is the clock, so the room cannot run without one; it has atomic host transfer through the relay and the relay tells peers who is eligible.
- **Why it matters:** A host on a phone that locks its screen stops sending heartbeats, so a group can pause or change hands because of a device's power policy (B-13.6). Jellyfin has no such failure mode.
- **Assessment:** **Trade-off.** Portico's timeline is well specified and tested (`TestSocialHostReconnectTimeline`). The takeover rule needs a decision (B-13.6).

### A-13.9 Groups are stored; they survive a restart
- **Portico:** Groups, members, queue, command receipts and a 512-event ledger are SQLite rows. A restart resumes the sweeper for live groups (`S/social/sweeper.go:121`, `S/httpapi/social_routes.go:154`).
- **Jellyfin:** in memory only. **Plezy relay:** snapshots rooms to disk.
- **Why it matters:** A server update in the middle of a film does not dissolve a Portico group. The cost is that every heartbeat and readiness report is a write through SQLite's single writer (A-13.14).
- **Assessment:** **Portico ahead** on durability. The write volume is the thing to trim.

### A-13.10 A member is a profile, not a device
- **Portico:** Membership is keyed by (authority, account, profile) (`S/social/groups.go:236`). The same profile on two devices is one member whose two devices overwrite each other's readiness.
- **Jellyfin / Plezy:** a member is one device/session.
- **Why it matters:** "Phone and TV in the same room under my profile" works by accident in Portico: both follow the clock, but the roster shows one person and one readiness value.
- **Assessment:** **Trade-off.** Fits Portico's profile-centred model (contract §2). Worth a conscious decision whether a second device of the same profile is supported or refused.

### A-13.11 How people find and join a group
- **Portico:** An 8-character code from the host, typed or pasted, or a `/together/join#code=` link on web. Codes are stored as digests, expire, are use-limited and rate-limited (`S/social/invites.go`). A code can be bound to one profile (`recipientProfileId`) but no first-party client sets it (`CC/group-session.ts:266`).
- **Jellyfin:** open the SyncPlay menu and pick from the list of groups you are allowed to see. **Plezy:** 5-character room code on a public relay.
- **Why it matters:** For a household on one server, Jellyfin's list is one tap; Portico needs the code carried out of band (read aloud, messaged). Portico's is safer for a server shared with acquaintances.
- **Assessment:** **Trade-off**, deliberate (contract §10: "SHOULD use explicit invitations"). The gap is delivery: the server already has a notification inbox but a group invitation is never put in it (F-13.3).

### A-13.12 A queue that not everyone may see
- **Portico:** Members who cannot see an entry get a placeholder; the host sees who is blocked; starting a blocked entry is refused unless the host overrides, and the override is announced (`S/social/queue.go:144`, `S/social/transport.go:372`).
- **Jellyfin:** all-or-nothing: such a user cannot see or join the group, and nothing can be queued that a member cannot see. **Plezy:** no check; a guest without the item simply fails to load it.
- **Why it matters:** Portico's is the most considered design here and the only one that keeps a mixed group together.
- **Assessment:** **Portico ahead.** Two leaks undercut it (B-13.9).

### A-13.13 Concurrency control on commands
- **Portico:** Every command needs the current group `revision` and a unique idempotency key; receipts are stored (128 per group) and replayed (`S/social/transport.go:91`).
- **Jellyfin:** a lock per group; requests apply in arrival order; a few carry the playlist item id to drop duplicates. **Plezy:** sequence numbers; highest wins.
- **Why it matters:** Portico's is the strictest: a retried Play can never undo a Pause. But the same `revision` is also advanced by heartbeats and readiness reports, which makes ordinary commands fail (B-13.1).
- **Assessment:** **Portico ahead** in intent, **behind** in effect until B-13.1 is fixed. The fix is to fence commands on `playbackRevision` (which exists in the schema and the snapshot but is never incremented) rather than on the catch-all revision.

### A-13.14 Transport and chattiness
- **Portico:** HTTP POST for commands, an SSE stream for events, and the client **re-reads the whole group after every event** other than a snapshot (`CC/group-session.ts:193`), because members and queue are projected per viewer. Each member also POSTs a heartbeat every 10 s and a readiness report at least every 10 s; both are write transactions.
- **Jellyfin:** one WebSocket per client; events carry their full payload; buffering/ready are only sent on change. **Plezy:** one WebSocket; the host's periodic state is the heartbeat.
- **Why it matters:** For N members Portico does about 2N writes and up to N² group reads every 10 s, and each read recomputes queue visibility for every member × every queue item (`S/social/queue.go:117`). Fine at 4 people; heavy at the 32-member ceiling (B-13.10).
- **Assessment:** **Portico behind** on efficiency. Deliberate in part (no WebSocket; `api/social.md` rejects a second transport). Cheap wins: send readiness only on change plus a slow keep-alive, make the non-host heartbeat a read, and put `revision` in every event so clients re-read only when something they render changed.

### A-13.15 The shared queue stops at "a list"
- **Portico:** The server has `next`, `previous`, `set-queue-position`, shuffle and repeat. No client calls any of them; the only way to change title is to tap an entry (`load`). Nothing advances the queue when a title ends. Server-side `next` ignores the shuffle flag (`S/social/transport.go:311`).
- **Jellyfin:** a complete queue with automatic next item, shuffle, repeat, play next/last.
- **Why it matters:** A group watching a season must have the host tap the next episode each time; meanwhile the clock runs past the end of the finished episode.
- **Assessment:** **Portico behind.** Medium: the server needs the item's duration (A-13.16) or a client "ended" report with duplicate suppression like Jellyfin's.

### A-13.16 The server does not know how long the title is
- **Portico:** Seek targets and the running clock are unbounded (`S/social/transport.go:280`, `groups.go:356`). Contract §13.4 says the target is "bounded to the playable timeline".
- **Jellyfin:** stores the item's runtime with the group and clamps every position. **Plezy:** the guest clamps to its own duration.
- **Why it matters:** The group position can show 2:15:00 of a 1:50:00 film; a late joiner is told to load past the end.
- **Assessment:** **Portico behind**, small: read the duration when an entry becomes current and clamp in `extrapolate` and `seek`.

### A-13.17 What "ready" means
- **Portico:** A member's own periodic claim (`ready`/`buffering`/`lagging`) with a position; stale after 30 s. The server aggregates it and uses it only to move the lobby from `preparing` to `ready` (`S/social/transport.go:455`).
- **Jellyfin:** `Ready` is a step in a handshake; the server compares the reported position with the group's and re-seeks that client if it is more than 500 ms out.
- **Why it matters:** Portico's readiness cannot gate anything after the first Play, and a member who vanished stays "stale" forever (B-13.5).
- **Assessment:** **Trade-off** that follows from A-13.2.

### A-13.18 Playback speed is a group property in the protocol only
- **Portico:** The timeline has a rational `rate` (0.5×–4×) settable on any command (`S/social/transport.go:191`). No client sends it, and a member's own speed setting is not coordinated with the group.
- **Plezy:** rate is part of the synced state and guests may request it. **Jellyfin:** no group rate.
- **Why it matters:** A member watching at 1.25× in a group will be dragged back by a seek every few seconds. The server-side rate change also has an arithmetic slip (B-13.4).
- **Assessment:** **Unclear** whether group speed is wanted. Either wire it up or lock the local speed control while in a room.

### A-13.19 Client coverage
- **Portico:** One follower in `client-core` drives web, iPhone, iPad, Apple TV, Android and TV builds.
- **Jellyfin:** web only (plus web-wrapping apps). **Plezy:** all its platforms (one Flutter codebase).
- **Assessment:** **Portico ahead** of Jellyfin, level with Plezy. The native build has no speed nudge (`RN/app/together.tsx:44`), so it leans entirely on seeks (B-13.3).

### A-13.20 Reach: one server, no third party
- **Portico:** Everyone must be a signed-in member of the same server and must reach it (port forwarding). Nothing leaves the server. Deliberate (contract §1).
- **Plezy:** coordination goes through a public relay, so the *room* works behind NAT, and the relay sees room codes and media titles; media access is still each guest's own problem. **Jellyfin:** same-server like Portico.
- **Assessment:** **Trade-off.** Portico's privacy position is the strongest. If the paid relay arrives, Watch Together benefits for free.

### A-13.21 Limits
- **Portico:** 32 members, 256 queue entries, 128 receipts, 512 events per group; no limit on groups per person and no rate limit on creating groups or sending commands (`S/social/store.go:33–55`).
- **Jellyfin:** none stated. **Plezy relay:** 8 per room, hard connection and per-IP caps, 30-burst/10-per-second message rate.
- **Assessment:** **Trade-off.** Portico's per-group caps are sensible; the missing creation and command limits contradict contract §17 (B-13.13).

### A-13.22 Names in the roster are typed by the member
- **Portico:** `displayName` is free text supplied at create/join, default "Viewer"; only its length is checked (`S/social/groups.go:259`, `store.go:251`).
- **Jellyfin:** the account's user name, set by the server.
- **Why it matters:** A member can appear as "Dad". Notification text is filtered for control and direction-override characters (`S/notify/notify.go`, `text`); group and member names are not.
- **Assessment:** **Portico behind**, small: take the name from the profile, or at least apply the same text filter.

### A-13.23 A privacy switch that hides a participant
- **Portico:** The profile preference "Include me in Watch Together activity" removes the member from every roster and withholds their readiness and host-presence events (`S/social/privacy.go`). The member still joins, follows and counts toward limits.
- **Others:** no equivalent.
- **Why it matters:** The other members cannot tell that person is in the room (B-13.8).
- **Assessment:** **Unclear** what the switch is meant to do; needs a decision.

### A-13.24 Per-profile feature switch
- **Portico:** A profile restriction `allowWatchTogether` gates every `/v1/groups` route in one place (`S/httpapi/restriction_admission.go:44`, `router.go:604`).
- **Jellyfin:** three levels per user (create and join / join only / none).
- **Assessment:** Level. Jellyfin's "join only" is a useful middle value for children's profiles (F-13.10).

### Remote control ("Play on")

### A-13.25 Portico moves playback; the others also drive another device
- **Portico:** One operation: hand the current title to a TV. Afterwards the phone is not a remote (`RN/player/Destinations.tsx:83`). Receivers declare `play/pause/seek/stop`, and grants carry `allowedCommands`, but there is no route or channel that delivers such a command. The product contract describes a direct, encrypted LAN command channel (§6.2); none exists in the code.
- **Jellyfin:** any client can list controllable sessions and send ~45 commands, including navigation keys, text, volume, track choice, "show this on that screen" and pop-up messages. **Plezy:** phone-as-remote with D-pad over the LAN.
- **Why it matters:** "Pause the TV from my phone", "type the search on my phone" and "start something on the kids' TV" are everyday uses Portico cannot do.
- **Assessment:** **Portico behind.** Large if built as the contract's LAN channel; medium if commands ride the existing receiver inbox and notification-stream hint.

### A-13.26 Only the same profile can send to a device
- **Portico:** Receivers, grants and handoffs are all keyed by the exact profile (`S/social/receivers.go:187`, `223`). A parent's phone cannot send to a TV signed in as the child's profile, or to a TV signed in as a housemate.
- **Jellyfin:** your own sessions always; other users' sessions with a permission; signed-out shared devices with another. **Plex:** any device on the account or Home.
- **Why it matters:** Living-room TVs are often left on a shared or child profile.
- **Assessment:** **Trade-off (subjective).** Portico's rule is the safest and matches the contract ("one ViewerScope"). A household rule ("profiles of the same account") would cover most homes; it changes whose history the playback lands in, which is the real design question.

### A-13.27 Finding devices: server list ∩ local network
- **Portico:** The phone shows a TV only if the server lists it for this profile **and** the phone hears its Bonjour advert `_portico-player._tcp` right now (`RN/player/Destinations.tsx:54–55`). "Local network only" is recorded as the owner's rule in that file.
- **Jellyfin:** server list only; works from anywhere. **Plezy:** UDP beacons, LAN only.
- **Why it matters:** Portico needs the local-network permission prompt on iOS and fails on networks that block multicast (guest Wi-Fi, some mesh systems, VLANs), even though the handoff itself goes through the server and would work. The web app has no Bonjour, so it has no "Play on" for Portico devices at all.
- **Assessment:** **Trade-off**, deliberate. Consider falling back to the server's own presence (`online` within 120 s plus same public IP) when Bonjour finds nothing.

### A-13.28 How a TV learns it has been asked
- **Portico:** The TV polls an inbox every 30 s, or holds a long poll, or listens for an id-less hint on the notifications event stream and then reads the inbox (`CC/remote-playback.ts:152–225`). While its player is in front it does not poll at all.
- **Jellyfin:** the server pushes the command down the client's WebSocket.
- **Why it matters:** Portico's design keeps one durable source of truth and tolerates clients that cannot hold a stream. Worst case a request waits up to 30 s (60 s safety interval if the stream is connected but the hint was lost). A TV that is already playing something never sees a new request until playback stops.
- **Assessment:** **Trade-off.** Robust; latency depends on the stream being up.

### A-13.29 Two-phase handoff
- **Portico:** Prepare → TV proves it is playing → commit ends the phone's session in the same transaction; any failure leaves the phone playing (`S/social/handoff.go`).
- **Jellyfin:** "Play this on that session" is fire-and-forget; the controller stops its own playback separately.
- **Why it matters:** Portico never leaves the viewer with nothing playing, and never double-counts a stream slot after commit.
- **Assessment:** **Portico ahead.** Note the TV must be audibly playing before the phone stops, so both play for a second or two; the contract instead asks for the TV to prepare "paused and unpresented" (doc/code disagreement D-13.4).

### A-13.30 What travels with a handoff
- **Portico:** The item id and a position (`S/social/handoff.go:253`, `CC/remote-playback.ts:234`). Not the audio or subtitle choice, quality, version, queue, shuffle or repeat. Contract §7.2 lists all of those as preserved.
- **Jellyfin:** the Play command carries the item list, start index, position, media source, audio and subtitle stream indexes.
- **Assessment:** **Portico behind.** Small to medium: add the fields to the handoff row and to `ReceiverPlayer.load`.

### A-13.31 "Allow this phone?" on the TV
- **Portico:** A receiver's policy decides whether a new controller device must be approved on the TV (`per-device` default, `always-ask`, `open`); grants last 15 minutes and renew on use (`S/social/receivers.go:331`).
- **Others:** no prompt.
- **Why it matters:** Because controller and TV are always the same profile, the prompt protects only against another device signed in to the same profile, and that device can approve itself (B-13.11).
- **Assessment:** **Trade-off.** As built it is ceremony more than security.

### Google Cast

### A-13.32 How the TV gets a credential
- **Portico:** The sender never shares its own sign-in. It asks the server for a one-use code; the TV redeems it for its own session and a rotating device token (`S/social/cast.go`). Failed redemptions are budgeted per network and server-wide.
- **Jellyfin:** the sender sends its own access token to the receiver in every message. **Streamyfin:** the stream URL the TV loads contains the token.
- **Why it matters:** In Portico a TV's access can be revoked without signing the phone out, and the phone's token never sits on a shared device.
- **Assessment:** **Portico ahead.** Two caveats: the TV's token is a full viewer session with the pairing person's role, not the narrow receiver credential the contract describes (B-13.14), and the contract's encrypted proof-of-possession bootstrap is not what was built (D-13.3).

### A-13.33 The TV owns the playback session
- **Portico:** The receiver opens its own Playback v1 session with a probed device profile, reports its own progress and recovers from stalls itself (`server/receiver/cast/receiver.js:393–489`).
- **Jellyfin:** same idea (receiver reports as its own session). **Streamyfin:** the phone resolves a stream URL and Google's default player plays it.
- **Assessment:** **Portico level with Jellyfin, ahead of Streamyfin.** The capability probing (`buildProfile`, `receiver.js:213`) is more careful than a fixed profile.

### A-13.34 One controller, with a takeover prompt
- **Portico:** The sender that paired is the only one that can load, play, seek or read status; others can pause or stop. A second person pairing triggers an on-TV prompt and a question to the controller, with a 30 s default of "no" (`receiver.js:491–559`).
- **Jellyfin / Streamyfin:** any connected sender controls.
- **Assessment:** **Portico ahead** for shared homes. It is more code to get right on real devices (not yet physically tested, per Backlog).

### A-13.35 Cast needs HTTPS, and one shared receiver page
- **Portico:** The receiver accepts only an `https:` origin (`receiver.js:57–64`); the sender refuses to construct without one (`CC/cast/sender.ts:159`). A server reached by plain HTTP on the LAN cannot cast. The same page is served by every server and at `cast.getportico.tv`.
- **Jellyfin:** hosts one receiver; the docs' recurring Chromecast issue is DNS, not HTTPS.
- **Why it matters:** "Plain HTTP stays first-class" is a Portico principle; Cast is an exception the owner will meet as "the Cast button is missing".
- **Assessment:** **Trade-off** forced by the platform; whether Cast devices would load a plain-HTTP LAN server from an HTTPS receiver page was not tested here. Worth one line in the UI explaining why Cast is absent.

### A-13.36 Cast feature breadth
- **Portico:** One title at a time. Audio and subtitle tracks only as the stream carries them (sidecar subtitles are a Backlog item). No queue, no next episode, no quality control, no volume control from the Portico UI.
- **Jellyfin:** queue, next/previous, shuffle, repeat, instant mix, per-stream audio/subtitle switching by re-requesting the stream, max bitrate with automatic detection, volume, "display this item".
- **Assessment:** **Portico behind.** Medium; the receiver already has the session machinery, so a queue is mostly "ask the server for the next item when this one finishes".

### A-13.37 When the phone goes away
- **Portico:** When no sender remains connected the receiver stops playback (`receiver.js:655`). The contract says the opposite twice (§7.1.7, §7.3).
- **Jellyfin:** the receiver has no such handler.
- **Assessment:** **Portico behind** and a doc/code disagreement; see B-13.7.

### A-13.38 A Cast TV is not a session other devices can drive
- **Portico:** A paired Cast TV is listed in the receiver directory (`kind: cast`) but nothing can be sent to it through the server; only the Cast channel controls it.
- **Jellyfin:** the Cast receiver is an ordinary session: visible on the dashboard, controllable from any Jellyfin client.
- **Assessment:** **Portico behind**, follows from A-13.25.

### A-13.39 AirPlay and DLNA
- **Portico:** AirPlay through the system picker only; DLNA absent on purpose, with written reasons (`api/social.md` §5), though the Backlog lists it as planned.
- **Jellyfin:** AirPlay likewise in native apps; DLNA server and play-to as an optional plugin.
- **Assessment:** **Trade-off.** Portico's reasons (no identity, no per-viewer visibility, long-lived unauthenticated URLs) are sound. DLNA remains the only way to reach many older TVs, receivers and Sonos-class speakers.

### Sessions, sharing, notifications

### A-13.40 The owner's view of who is watching
- **Portico:** Now Playing lists sessions with their delivery decision, refreshed by events, and offers "terminate with a message" (`CC/playback-v1/admin-sessions.ts`). A second, older Activity list offers Stop.
- **Jellyfin:** the dashboard shows sessions with transcode reasons, lets an admin send a message **without** stopping, and remote-control the session.
- **Assessment:** Level on visibility; Portico lacks "message without stopping" (F-13.9). Two overlapping lists is a tidy-up item.

### A-13.41 How a server is shared
- **Portico:** An invitation link/code that creates the account, with role, library list and expiry chosen up front (`S/access/invitations.go`). Nothing is emailed; the owner passes the link along.
- **Jellyfin:** an admin types a user name and password for each person. **Plex:** invite by email or username; the invitee accepts in their own account.
- **Assessment:** **Portico ahead** of Jellyfin. Against Plex it lacks delivery and the review-before-send summary the contract describes (§3.1). Content restrictions and limits cannot be attached to the invitation; they are set on the member afterwards, so a new member briefly has unrestricted ratings (F-13.11).

### A-13.42 What can be restricted per person
- **Portico:** Library allow list; member limits (streams, schedule, channels, rating ceiling, denied labels); per-profile rating ceiling, blocked labels and feature switches for downloads, Live TV, DVR, Watch Together.
- **Jellyfin:** much the same, plus remote-access on/off, a per-stream internet bitrate cap, and the two remote-control permissions.
- **Assessment:** Level.

### A-13.43 No public or guest links
- **Portico:** None, by contract (§1: "anonymous guest links" are out of scope).
- **Jellyfin:** none. **Plex:** none for personal media. Audiobookshelf and photo/file servers commonly offer time-limited share links.
- **Assessment:** Level with direct competitors; deliberate.

### A-13.44 A real inbox, in-app only
- **Portico:** A durable per-profile inbox with read/archive state, one revision number that fences writes and resumes streams, de-duplication per condition, a 1,000-item cap per inbox and a strict allowlist of what a notice may make a client do (`S/notify/notify.go`). No push, email or webhook (recorded decision).
- **Jellyfin:** no per-user inbox; admin alerts plus a webhook plugin that reaches Discord, ntfy, email and others. **Plex:** mobile push and email.
- **Why it matters:** Portico's is the better in-app design. But a notice only helps someone who has the app open: "low storage", "certificate expiring" and "scan failed" are exactly the notices an owner wants while away.
- **Assessment:** **Portico ahead** in-app, **behind** on reach. A single owner-configured outbound webhook would cover the away case without push infrastructure or server email (F-13.8); it does touch the recorded "no push or server email" decision, which is Justin's call.

### A-13.45 Notices are raised only by server housekeeping
- **Portico:** Ten categories, all operational (downloads, DVR conflict, security, scan, certificate, storage, feedback, owner broadcast). None for content ("new episode of a show you follow") and none for social events (invited to a group, someone wants to play on your TV).
- **Plex:** new-content push. **Jellyseerr-style tools:** "your request is available".
- **Assessment:** **Portico behind**; see F-13.3 and F-13.7.

## Missing features and capabilities to consider

### F-13.1 A short "wait for everyone" after a seek or a new title
- **What:** After the host seeks or loads a title, hold the clock until members report ready, with a timeout (Jellyfin: 30 s; Plezy: 15 s) and "start anyway".
- **Who has it:** Jellyfin (`WaitingGroupState`), Plezy (`waitingForPeers`).
- **Value:** Removes the "everyone skips ahead then stutters" start, and is the natural cure for B-13.2 and B-13.3.
- **Size / conflicts:** Medium. Touches the recorded decision "one member buffering does not pause everyone" only if extended to mid-play stalls; a seek/load-only rule does not.
- **Opinion:** The most valuable single change in this domain.

### F-13.2 Scheduled starts
- **What:** Play and resume take effect a fraction of a second in the future so all members start on the same instant.
- **Who:** Jellyfin, Plezy. **Size:** small (A-13.3). **Opinion:** do it with F-13.1.

### F-13.3 Invite a person, not a code
- **What:** Pick a server member (or profile) and send a group invitation that lands in their notification inbox with a "Join" action; optionally a list of groups in your household you may join.
- **Who:** Jellyfin lists joinable groups; Plex invited friends by name.
- **Value:** Removes the out-of-band step. The pieces exist: `recipientProfileId` on invites and the inbox with navigate actions.
- **Size:** Medium (a member picker that respects contract §3.3 privacy, a new notice category, a `together` navigation view). No conflict: in-app only.

### F-13.4 The queue actually advancing
- **What:** Next episode starts for the group when one ends; next/previous buttons; shuffle and repeat that the server honours.
- **Who:** Jellyfin. **Size:** medium (A-13.15, A-13.16). **Opinion:** needed before "watch a season together" is usable.

### F-13.5 Remote control of another Portico device
- **What:** From a phone or the web: pause, seek, stop, change volume, audio and subtitle on a TV that is playing; start a title on a TV; use the phone as D-pad and keyboard for the TV app.
- **Who:** Jellyfin (server-mediated, ~45 commands), Plex Companion, Plezy (LAN D-pad remote).
- **Value:** Typing a search on a TV remote is the worst input in the product; a phone keyboard fixes it. Parents pausing the kids' TV is the second use.
- **Size:** Medium if commands go through the existing receiver inbox and hint stream; large as the contract's LAN channel. The contract deliberately removed a server command queue (§6.2.11), so this needs a decision.

### F-13.6 "Ask the host" from a member
- **What:** A member's Pause (or "go back 30 s") becomes a visible request the host can accept with one tap; or the host can lend control for a while.
- **Who:** Plezy forwards guest control requests to the host. The Portico contract already describes it (§11).
- **Size:** Small to medium.

### F-13.7 Content notices
- **What:** "A new episode of a show on your list is available", "the title you asked for was added".
- **Who:** Plex push/email, request tools.
- **Size:** Medium; must be bounded per scan (one notice per show per scan, per interested profile) to respect the fan-out limits. In-app only fits the recorded decision.

### F-13.8 Reaching the owner when the app is closed
- **What:** One owner-configured outbound webhook (ntfy, Discord, Gotify, a home-automation hook) for admin notices.
- **Who:** Jellyfin's Webhook plugin.
- **Conflict:** Brushes the recorded "no push notifications or server email" decision; a webhook is neither, but it is outbound traffic the owner opts into.
- **Size:** Small. **Opinion:** worth asking (Q-5).

### F-13.9 Message a session without stopping it
- **What:** The owner sends "restarting in 10 minutes" to one viewer or everyone watching, shown over playback.
- **Who:** Jellyfin `/Sessions/{id}/Message`. Portico can only attach a message to a terminate, or broadcast to inboxes.
- **Size:** Small.

### F-13.10 Group housekeeping
- **What:** Remove a member; lock the room; a "join only" permission level for restricted profiles; automatic removal of members who vanished.
- **Who:** Jellyfin (three permission levels, automatic leave). The Portico contract says "group hosts can remove participants" (§17) but no such operation exists; the client types even include a `removed` member state the server never produces.
- **Size:** Small.

### F-13.11 Restrictions attached to the invitation
- **What:** Choose rating ceiling, feature switches and limits when inviting, and show a summary before sending; resend and decline.
- **Who:** Plex's share dialog. Contract §3.1–3.2 describe it.
- **Size:** Medium.

### F-13.12 Chat and reactions
- **What:** A text side panel and emoji reactions inside a group.
- **Who:** Teleparty-style tools; SharePlay gets voice and video from FaceTime. Neither Jellyfin nor Plezy has it.
- **Size:** Medium (the event ledger could carry it). **Opinion:** low priority; people in a watch group are usually already on a call or chat. Reactions are cheap and TV-friendly.

### F-13.13 A fuller Cast receiver
- **What:** Next episode and queue on the TV, sidecar subtitles (Backlog), quality choice, volume, and keeping the receiver controllable through the server.
- **Who:** Jellyfin's receiver. **Size:** medium.

### F-13.14 Richer handoff
- **What:** Carry audio/subtitle choice, quality and queue; "bring it back to this device" (the reverse direction); let the web app send to a TV.
- **Who:** Jellyfin's Play command; Plex "continue on". **Size:** small to medium.

### F-13.15 Sync diagnostics and a per-device offset
- **What:** Show each member's drift and ping; let a device add a fixed offset (Bluetooth speakers and some TVs add 100–300 ms of audio delay).
- **Who:** Jellyfin (playback-diff display, "extra time offset" setting).
- **Size:** Small.

### F-13.16 Other renderers
- **What:** DLNA/UPnP renderers and servers, Sonos, AirPlay 2 multi-room audio, voice assistants.
- **Who:** Jellyfin (DLNA plugin). **Size:** large; Backlog already lists DLNA as planned and off by default. Music to network speakers is the strongest case.

### F-13.17 Apple SharePlay as a second carrier
- **What:** Start a Portico group from a FaceTime call on Apple devices; the OS carries invitations and voice.
- **Size:** Medium on Apple only; contradicts "clients may not choose different social rules by platform" (contract §19) unless it is only a front door to the same server group.

### F-13.18 Household "Play on"
- **What:** Send to a TV signed in to another profile of the same account, or to a TV in a shared/guest mode.
- **Who:** Jellyfin (shared-device permission), Plex Home. **Size:** medium; see A-13.26 and Q-3.

### F-13.19 Presence outside the app
- **What:** "Now watching" published to Discord.
- **Who:** Plezy. **Conflict:** contract excludes activity feeds. **Opinion:** recorded for completeness; not recommended.

## Possible bugs and risks in Portico

None of these appear in `Backlog.md` unless stated. Confidence levels: **Confirmed by reading** means the code path was traced; runtime behaviour was still not observed.

### B-13.1 Heartbeats and readiness reports invalidate the host's commands
- **What goes wrong:** Every group write goes through `mutate`, which always saves with a revision bump, even when nothing changed (`S/social/groups.go:597–603`). `Heartbeat` for a non-host does nothing and still bumps (`groups.go:306–318`); `ReportReadiness` bumps too (`S/social/transport.go:468`). Transport, settings, end and host-transfer commands are refused unless `expectedRevision` equals that same number (`transport.go:202`). Clients send a heartbeat every 10 s and a readiness report at least every 10 s per member (`CC/group-session.ts:38`, `358`). A non-host heartbeat publishes no event, so the host learns the new revision only at its own next read.
- **Effect:** The host's Play/Pause/Seek is answered `409 revision_conflict` whenever another member's heartbeat landed since the host last read. The client does not retry; it re-reads and shows "the group changed" (`CC/group-session.ts:222`). In a two-person lobby (where no readiness events flow) the host's number is stale for, on average, about half of every 10 s cycle; with more members it is stale most of the time. This estimate comes from the timers in the code, not from measurement.
- **How the comparison surfaced it:** Jellyfin has no revision fence at all; asking "what advances Portico's fence?" led to `mutate`.
- **Confidence:** Confirmed by reading. **Severity:** High. A test covers the fence (`TestSocialTransportRevisionFenceAndIdempotency`) but not with a second member heartbeating.
- **Fix direction:** Fence commands on `playbackRevision` (declared, published, never incremented anywhere in `S/social`) and stop bumping `revision` for heartbeats and readiness.

### B-13.2 A slow seek is restarted every three seconds and never finishes
- **What goes wrong:** While a seek is pending the player reports the seek target as its position (`WEB/app/together.tsx:40`, `RN/app/together.tsx:32`), `ready` stays true, and `buffering` is false unless the engine is "recovering". The follower keeps measuring: the group clock moves on, so drift grows by one second per second. When it reaches 3 s the follower calls `seek(target)` again (`CC/group-session.ts:350`). `requestSeek` replaces the pending seek with a new revision and restarts the 15 s seek timer (`CC/index.ts:1328–1336`); the completion of the earlier seek is then ignored (`index.ts:1360`).
- **Effect:** Any member whose seek takes 3 s or longer (a server-side seek on a converted stream restarts production; a slow link) is re-seeked every 3 s indefinitely and never plays, with no error shown. This is the classic seek storm.
- **How surfaced:** Plezy's reconciler has a settle window and a 2 s cooldown precisely for this; Portico's has neither. Contract §13.6 also says a buffering participant "does not issue corrective seek/rate feedback".
- **Confidence:** Confirmed by reading; whether real seeks exceed 3 s needs a runtime check. **Severity:** High.

### B-13.3 After a seek, members can loop: wait four seconds, seek again
- **What goes wrong:** A seek lands the member where the group was when the seek *began*; when it completes the group is ahead by the seek's duration L. In the 0.75–3 s band the follower nudges speed for 4 s and then seeks regardless (`CC/group-session.ts:351`). The web nudge recovers at most 0.4 s, so any L above about 1.15 s ends in another seek, which again lands L behind. On the native apps `setRate` is an empty function (`RN/app/together.tsx:44`), so any L above 0.75 s loops.
- **Effect:** A visible re-seek every four to six seconds for as long as the group plays, for members with ordinary HLS seek times.
- **How surfaced:** Plezy seeks to target + 250 ms and enforces a cooldown; Jellyfin pauses the group around a seek.
- **Confidence:** Probable (depends on real seek times, which were not measured). **Severity:** High on native, Medium on web.
- **Fix direction:** Seek to `target + measured seek time`, add a cooldown and settle period, and keep nudging until inside an exit threshold (A-13.5); or F-13.1.

### B-13.4 Changing the group's speed jumps the position
- **What goes wrong:** `applyTransport` stores the new rate first and then computes the current position with it (`S/social/transport.go:266–270`). Time already played at the old rate is multiplied by the new one. Example: playing at 1× for 100 s since the last anchor, then any command with `rate: 2/1` anchors the group 100 s too far ahead.
- **Confidence:** Confirmed by reading. **Severity:** Low today (no first-party client sends `rate`); it is in the documented third-party API.

### B-13.5 Members who disappear stay in the group forever, and the lobby shows everyone as "away"
- **What goes wrong (a):** Nothing ever moves a member out of `joined` except their own Leave (`S/social/groups.go:396`). A member whose app was closed keeps counting toward the 32-member limit, toward queue-visibility blocking, and as "stale", which holds the readiness aggregate at `lagging` (`groups.go:924`). The lobby therefore never reaches `ready` again and the Play button stays "Play anyway". The client's comment says "the server ends a silent membership on its own" (`CC/group-session.ts:284`); it does not.
- **What goes wrong (b):** Presence is computed from the last readiness report only (`groups.go:896–904`). The follower sends no reports until a title is loaded (`CC/group-session.ts:335`), and a member's heartbeat updates nothing. So 30 s after joining a lobby every member, the host included, is shown as away, and after 120 s as disconnected.
- **How surfaced:** Jellyfin removes a member when its session ends.
- **Confidence:** Confirmed by reading. **Severity:** Medium.

### B-13.6 Any member can take the host role after 30 seconds of host silence
- **What goes wrong:** `TransferHost` lets a member name itself whenever the group is in a `host-reconnecting-*` state (`S/social/groups.go:435`), which begins after `HostGraceSeconds = 30`. The code comment says "has passed the pause boundary" (two minutes); the code does not check that. A host whose phone locked, or whose tab was throttled, for 30 s can lose the group to any member, who can then end it or change the title. No readiness or eligibility is checked for the claimant. The contract has no self-claim at all (§11).
- **Confidence:** Confirmed by reading. **Severity:** Medium (design risk; it defeats `host-only`). Both first-party apps expose it as a "Become host" button.

### B-13.7 Casting stops when the phone disconnects
- **What goes wrong:** On any sender disconnect, if no senders remain, the receiver calls `stopPlayback` (`server/receiver/cast/receiver.js:655`). It does not look at the disconnect reason. A phone that sleeps, leaves Wi-Fi or has its app closed by the system typically drops its Cast connection.
- **Effect:** The film stops on the TV shortly after the phone goes to sleep. The contract says a sender's sleep or departure must not strand playback (§7.1.7) and that Disconnect is not Stop (§7.3).
- **How surfaced:** Jellyfin's receiver has no such handler.
- **Confidence:** Confirmed by reading; how quickly real senders are reported as disconnected needs a device check. **Severity:** High for Cast. Backlog notes Cast has never been run on a physical device; this specific behaviour is not listed.

### B-13.8 A profile with the Watch Together privacy switch off is an invisible participant
- **What goes wrong:** The roster query filters out members whose profile preference `privacy.includeInWatchTogether` is false (`S/social/groups.go:882`, `S/social/privacy.go`). Join does not consult the preference. Such a member joins, receives the full group, timeline and queue, and is absent from every other member's list. Readiness totals also skip them, so the lobby can report "all ready" while they are still loading. Only the host's blocked-entries summary can reveal them (`S/social/queue.go:200`, built from the unfiltered list).
- **Confidence:** Confirmed by reading. **Severity:** Medium (the people in a room cannot see everyone who is in it).

### B-13.9 Small leaks around a group
- **(a) The current title's id reaches members who may not see it.** The queue entry is a placeholder, but `timeline.itemId` in every snapshot (`S/social/groups.go:841`) and `itemId` in the `group.transport` event (`S/social/transport.go:220`) are sent to everyone. It is an opaque id, not a title; the web shows "hidden title" because the lookup fails. It still contradicts "nothing else survives" in `api/social.md`.
- **(b) A member who left can keep reading the group.** `ReadGroup` and `ReadQueue` load the membership row but never check that its state is `joined` (`groups.go:527–548`, `queue.go:212–236`); only writes and the event stream do. The roster, timeline and queue remain readable by polling after leaving.
- **(c) The host's device id and playback session id** are published to every member in `group.authority` (`groups.go:838–864`).
- **Confidence:** Confirmed by reading. **Severity:** Low (a), Medium (b), Low (c).

### B-13.10 Cost grows with the square of the group size
- **What goes wrong:** Each readiness event makes every member re-read the group (`CC/group-session.ts:193`). Each read rebuilds the queue projection, which asks "may this member see this item" for every joined member × every distinct queue item (`S/social/queue.go:117–139`, `149–157`), each a database check. At the limits (32 members, 256 entries) that is up to 8,192 checks per read, about a hundred reads a second, plus about six write transactions a second through the single writer.
- **Confidence:** Confirmed by reading (design risk; not measured). **Severity:** Medium at the ceiling, negligible for a handful of people. Conflicts with the "page loads do bounded work" principle.

### B-13.11 "Play on" approvals and readiness are not tied to the TV
- **(a)** `DecideGrant` accepts a decision from any device signed in to the profile (`S/social/receivers.go:490–529`). The phone that asked can approve itself, so `per-device` and `always-ask` do not restrict anything.
- **(b)** `ReportHandoffReadiness` accepts any well-formed `receiverPlaybackId` (`S/social/handoff.go:291`) and never checks that such a session exists, is live, belongs to this profile, plays this item or runs on the receiver. A buggy or hostile same-profile client can make the commit end the phone's session while nothing plays on the TV.
- **(c)** `controllerDeviceId` and the receiver's `keyFingerprint` are client claims; the fingerprint is a hash of the installation id, not of a key (`RN/app/receiver.tsx:40`), and the heartbeat skips the check when it is omitted (`receivers.go:313`).
- **Confidence:** Confirmed by reading. **Severity:** Low (everything is inside one profile's trust), but the names promise more than is enforced.

### B-13.12 A TV that is already playing never sees a "Play on" request
- **What goes wrong:** While its player is in front, the receiver neither polls its inbox nor reacts to stream hints (`CC/remote-playback.ts:170`, `207`). A phone sending to that TV waits for approval (60 s) or for the handoff to expire (120 s) and then reports that the TV did not start.
- **Confidence:** Confirmed by reading. **Severity:** Medium (looks like a hang). Possibly intended ("do not interrupt"), but then the phone should be told at once.

### B-13.13 No limits on creating groups or sending commands
- **What goes wrong:** `CreateGroup`, `Transport`, `MutateQueue`, `ReportReadiness`, `RegisterReceiver` and `PrepareHandoff` have no rate limit and there is no cap on live groups or receivers per viewer. The sweeper reads every live group every 5 s (`S/social/groups.go:748`), and a group only ends after 10 minutes of host silence. A signed-in member can create groups in a loop. Contract §17 requires independent limits on "group creation/join/commands".
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (needs a signed-in member).

### B-13.14 The Cast TV holds a full session with the pairing person's role
- **What goes wrong:** Redemption issues "an ordinary viewer bearer" with the role captured at pairing (`S/social/cast.go:383`, `191`). If the owner casts, the television holds an owner-role access token, renewable for 24 hours through the device token (`CastSessionTTLSeconds`, `S/social/store.go:52`). The `capabilities` list is stored and returned but nothing enforces it. Contract §7.1.5 calls for a credential "restricted to one receiver session, profile, generation, origin, and operation set".
- **Confidence:** Confirmed by reading that the role is passed to `IssueTx`; what an owner bearer bound to a `google-cast` device can reach on admin routes was not traced. **Severity:** Medium (a shared screen is the least trustworthy device in the house).

### B-13.15 After the caster leaves, the TV still has their session and accepts an unpaired load
- **What goes wrong:** When the last sender disconnects, `controllerSender` is cleared but `session` and `scope` are kept (`receiver.js:655`). The check that blocks other senders is `!!controllerSender && …` (`receiver.js:580`), so with no controller a standard media `LOAD` carrying `customData.itemId` is accepted and played with the previous person's session (`receiver.js:584–589`). The app stays open until Cast's idle timeout.
- **Effect:** For a few minutes after someone stops casting, another device on the network can start one of that person's titles on the TV, under their profile, if it knows an item id.
- **Confidence:** Probable (depends on Cast's idle behaviour). **Severity:** Low to Medium.

### B-13.16 A refused takeover leaves a paired device behind
- **What goes wrong:** `handlePair` redeems the newcomer's code before deciding whether the TV is busy (`receiver.js:535`). If the controller says no, the redeemed result is dropped in memory, but the server now has an active Cast device, a receiver-directory entry and a session family for the newcomer, valid up to 24 h. Nothing revokes them.
- **Confidence:** Confirmed by reading. **Severity:** Low (the token is discarded; it is clutter and a stale "Cast device" in that person's list).

### B-13.17 Group state-machine slips
- **(a) Lobby states are lost when the host reconnects.** A silent host turns `lobby`, `preparing` or `ready` into `host-reconnecting-paused` with resume state `paused` (`S/social/groups.go:673–677`); on return the group becomes `paused`, never `preparing`/`ready` again (`groups.go:328`), so the automatic "everyone is ready" promotion (`transport.go:484`) can no longer fire.
- **(b) `load` with an unknown entry id restarts the current title** instead of failing: the target stays at the current position in the queue and the position is reset to 0 (`transport.go:329–348`).
- **(c) Changing title while playing does not prepare.** The comment says the group "re-enters preparing"; the code keeps `playing` from position 0 immediately (`transport.go:354–361`), so everyone joins the new title late by their own load time.
- **(d) `stop` sets the state to `ready`** even in an empty lobby (`transport.go:276–279`).
- **(e) In `anyone` groups with the host gone,** a member's Play is re-paused by the next 5 s sweep, because the host timeline ignores who may control (`groups.go:655–668`).
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium.

### B-13.18 Loading a title in a paused group plays it for a moment
- **What goes wrong:** `GroupPlayer.load(itemId, position, playing)` is implemented on web and native as `engine.play(itemId, position)`; the third argument is ignored (`WEB/app/together.tsx:42`, `RN/app/together.tsx:34`). In a `preparing` or paused group each member's player starts audibly and is paused on the follower's next one-second tick, then seeked back.
- **Confidence:** Confirmed by reading. **Severity:** Low (a blip of sound at each load).

### B-13.19 At the end of a title the group clock keeps running
- **What goes wrong:** Nothing ends or advances the group when the title finishes (A-13.15, A-13.16). Members' players reach `ended`, are no longer `ready`, and report `buffering` every 10 s (`CC/group-session.ts:344`); the room shows a position past the end and everyone "loading" until the host taps another entry.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium.

### B-13.20 The client can adopt an older snapshot over a newer one
- **What goes wrong:** `adopt` replaces the group with whatever snapshot arrives and never compares `revision`, `playbackRevision` or `reconnectGeneration` (`CC/group-session.ts:106–114`). A re-read requested just before a command can return after the command's event and put the old timeline back until the next re-read. Contract §13.3 requires ignoring lower revisions.
- **Confidence:** Probable (a short race). **Severity:** Low (self-heals on the next event).

### B-13.21 An invitation accepted by an existing Portico Account member rewrites that member
- **What goes wrong:** `AcceptPortico` applies the invitation's role and library list to whichever account redeems it, and clears `disabled` (`S/access/invitations.go:479`), for any role except owner. An admin who opens a member-tier invitation meant for someone else is demoted and has their libraries replaced; a disabled member holding any pending link is re-enabled.
- **Related:** the invitation's email is a label only; neither accept path checks it (`invitations.go:344`, `418`), while contract §3.2.5 says an email-bound invitation requires that verified address.
- **Confidence:** Confirmed by reading. **Severity:** Medium for the overwrite, Low for the email (codes are long and single-use; no replay path was found: the state check and the update are in one transaction).

### B-13.22 Owner broadcast edge cases
- **(a)** A server with more than 500 profiles cannot broadcast at all: the request fails with a capacity error rather than paging (`S/operations/notifications.go:736`).
- **(b)** `ProfileScopes` and `AdminScopes` build inbox keys with the literal authority `"local"` (`S/notify/notify.go:370–405`), and the admin audience is granted only to `local` owners (`operations/notifications.go:253`). If any viewer still signs in under the `hosted` authority (that path exists elsewhere in the code), broadcasts and admin notices would not reach them.
- **Confidence:** (a) Confirmed by reading; (b) Speculative, needs someone who knows which authority Portico Account members carry today. **Severity:** Low.

### B-13.23 Names are not filtered
- Group and member names accept any characters, including control and text-direction overrides (`S/social/store.go:251`), unlike notification text. **Confidence:** Confirmed by reading. **Severity:** Low.

## Where Portico is stronger

- **Mixed-visibility groups.** Placeholders, the host's eligibility summary and the announced host override are better than Jellyfin's all-or-nothing rule and absent in Plezy.
- **Strict command semantics.** Revision fencing plus idempotent receipts mean a retry can never undo a newer command (once B-13.1 is fixed).
- **Durability.** Groups, queue and events survive a server restart and resume by event number; Jellyfin loses everything.
- **One follower for every client.** TV, phone and web all take part; in Jellyfin only the web client does.
- **No third party and no token sharing.** No relay sees titles (Plezy's does); Cast never receives the sender's own token (Jellyfin's does).
- **Two-phase handoff** that cannot leave the viewer with nothing playing.
- **Cast controller ownership and takeover prompt**, and a probed device profile.
- **Invite codes done carefully:** digest storage, expiry, use limits, per-viewer attempt limits, unambiguous alphabet, state-specific errors only after the recipient check.
- **Feature switch per profile** enforced in one place for every group route.
- **A real notification inbox** with de-duplication, a single revision for fencing and resuming, and an allowlist of client actions.
- **Invitation links** instead of admin-typed passwords.
- **Published sync bands** so third-party clients behave like first-party ones.
- **Read paths that do not write:** group reads compute host expiry in memory and leave the write to the sweeper (`S/social/groups.go:701`), a thoughtful answer to SQLite's single writer.

## Doc/code disagreements

| # | Document says | Code does |
|---|---|---|
| D-13.1 | Contract §6.2: receivers hold X25519 keys; commands go phone → TV directly on the LAN, sealed; `_portico-receiver._tcp` | No key pair, no LAN command channel, no commands at all. The "fingerprint" is a hash of the installation id (`RN/app/receiver.tsx:40`). Handoff goes through a server inbox (`api/social.md` §3 describes the code correctly). Advert is `_portico-player._tcp` |
| D-13.2 | Contract §2–§3: Hosted is the invitation authority; email or link delivery; states include declined, delivery-failed, resend; email-bound invitations checked against a verified address | Invitations are server-local link codes with states pending/accepted/revoked; nothing is delivered; email is a label (`S/access/invitations.go`) |
| D-13.3 | Contract §7.1: receiver proof key, encrypted one-use envelope (60 s), dedicated authorization scheme, narrow media grants | Six-character code (5 min) over the Cast channel, redeemed for an ordinary viewer bearer (`S/social/cast.go`) |
| D-13.4 | Contract §6.2.7, §7.2: the target prepares "paused and unpresented" before commit | Only `readiness: "playing"` is accepted (`S/social/handoff.go:291`); the TV must be audibly playing first |
| D-13.5 | Contract §7.1.7, §7.3: sender departure does not stop playback; Disconnect ≠ Stop | Receiver stops when no sender remains (`receiver.js:655`) |
| D-13.6 | Contract §7.2: handoff preserves version, tracks, quality, queue, shuffle/repeat | Item id and position only |
| D-13.7 | Contract §10, §12.1: joining checks permission and access to the loaded item first | Join checks only the code and the feature switch; visibility is handled later by placeholders (`S/social/invites.go:125`) |
| D-13.8 | Contract §11: member Pause is a request to the host; host transfer to "an eligible ready participant"; no self-claim; states `creating`, `degraded`, `ending` | No request path; transfer checks only `joined`; self-claim after 30 s; those three states are never produced |
| D-13.9 | Contract §13: target bounded to the timeline; no corrections while buffering; lower revisions ignored | Unbounded (`transport.go:280`); follower corrects while a seek is pending; client compares nothing (`group-session.ts:106`) |
| D-13.10 | Contract §17: rate limits on group creation and commands; hosts can remove participants | Neither exists |
| D-13.11 | Contract §18 names `state-machines.yaml`, `conformance-vectors.yaml`, `remote-playback-acceptance-matrix.yaml` | The contract folder contains only `contract.md` |
| D-13.12 | `api/notifications.md`: 64 parked connections per server; authorization re-derived every two seconds | 512 (`S/httpapi/notifications_routes.go:48`); re-derived before each send and at the 20 s heartbeat (`notifications_routes.go:448`), so a revoked session can keep an idle stream for up to 20 s |
| D-13.13 | `api/social.md` §4: the Cast application id "has no default" and comes from the server setting | Web falls back to a build-time id (`WEB/bridge/cast.ts:67`); native apps use only the build-time id and ignore the server setting. `cast-receiver/README.md` still says native senders are not built |
| D-13.14 | `transport.go:357` comment: selecting an entry "re-enters preparing" | It stays `playing` when it was playing |
| D-13.15 | `group-session.ts:284` comment: "the server ends a silent membership on its own" | It never does |
| D-13.16 | `groups.go:433` comment: self-claim once the host "has passed the pause boundary" | Allowed from 30 s |
| D-13.17 | `api/social.md` §5: DLNA is a deliberate omission | `Backlog.md` lists DLNA under planned features |

## Open questions for Justin

1. **Waiting.** Is "one member buffering never pauses the group" meant to cover the moment right after the host seeks or changes title? If a short, bounded wait there is acceptable, most of the sync problems above disappear (F-13.1).
2. **Host takeover.** Should a member be able to take the host role at all without the host handing it over? If yes, after how long (30 s as coded, the 2-minute pause, or only when the group is about to end)?
3. **Whose TV.** Should "Play on" stay limited to the same profile, or extend to other profiles of the same account (and then whose history does the playback belong to)?
4. **Remote control.** Is driving another device (pause/seek/volume, phone as keyboard) in scope for release? If so, through the server (works everywhere, simple) or the LAN channel the contract describes?
5. **Reaching the owner.** Does "no push notifications or server email" also rule out an owner-configured outbound webhook for admin notices?
6. **The privacy switch.** What should "Include me in Watch Together activity" do: hide the person from rosters (today), stop them being invited, or stop them joining?
7. **Finding a group.** Keep codes only, or also deliver invitations into the inbox and/or list a household's open groups?
8. **The contract.** Sections 3, 6.2 and 7.1 describe designs that were not built (Hosted invitations, LAN command channel, encrypted Cast bootstrap). Should the contract be rewritten to match the code, or is the code expected to move?
9. **Cast and plain HTTP.** Is "Cast needs remote access and a certificate" acceptable as a stated limitation?
10. **Group speed and the queue.** Should a group have a shared playback speed, and should the queue auto-advance, or is Watch Together meant for one title at a time?
11. **Second device, same profile.** Supported in a group, or refused?
12. **Cast token.** Should a television ever hold an owner-role session, or should Cast sessions always be issued at member level?

## Evidence boundary

- Nothing was executed. No server, client, test, simulator or Cast device was run. Every "happens" above is a reading of source.
- **Timing claims are the weakest.** How long real seeks take (B-13.2, B-13.3), how often a host's revision is stale in practice (B-13.1), how soon Cast reports a sender as disconnected (B-13.7) and how long the Cast app idles (B-13.15) all need a device.
- **Not traced:** what an owner-role bearer issued to a Cast device can reach (B-13.14); which authority Portico Account members sign in under today (B-13.22b); Android `PorticoCast.kt`; Bonjour internals; the Hosted side of membership; the Vega copy of `remote-playback.ts`; native Together, invitation and notification screens beyond their client-core calls.
- **Comparison limits:** the Jellyfin Chromecast receiver was read as one file through a web fetch, not cloned; Plex, Emby, SharePlay and Infuse statements come from public pages and search results (Plex's Watch Together status in particular should be re-checked before being quoted). Jellyfin's DLNA plugin and Webhook plugin were not read as code.
- Whether a Cast device will load a plain-HTTP LAN server from an HTTPS receiver page was not established.

## Outside my domain

- The Watch Together screens contain hard-coded English strings where a catalogue message is expected (`WEB/app/together.tsx:61`, `68`; `RN/app/together.tsx:53`, `59`) and `WEB/screens/together/Together.tsx:11` binds `defaultI18n` at module load rather than the viewer's language.
- Two owner-facing session lists (`activity.ts` and `playback-v1/admin-sessions.ts`) overlap; the Backlog already notes the older history route is unused.
- `S/access/invitations.go` accepts an invitation with an empty library list; what an empty list grants (everything or nothing) was not traced and matters for contract §3.1 ("no default may silently grant every library").

Sources for the public-information statements: [Android Authority on Plex removing Watch Together](https://www.androidauthority.com/plex-watch-together-removed-3529939/), [AlternativeTo news](https://alternativeto.net/news/2025/2/plex-ends-support-for-watch-together-feature/), [Plex forum thread](https://forums.plex.tv/t/watch-together-going-away-in-app/906895?page=15), [Apple GroupActivities documentation](https://developer.apple.com/documentation/groupactivities.md), [Firecore forum SharePlay request](https://community.firecore.com/t/shareplay-where-is-it/50558), [Emby community watch-party request](https://emby.media/community/topic/110999-watch-party/page/8/), [jellyfin-chromecast `maincontroller.ts`](https://raw.githubusercontent.com/jellyfin/jellyfin-chromecast/master/src/components/maincontroller.ts).
