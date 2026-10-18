# Outcome

Watch Together now holds its clock for up to five seconds after a seek or title selection, releases early for current readiness, and preserves playing/paused intent. Heartbeats, readiness, queue changes, playback commands and governance no longer share one command fence. Members have separate presence leases, and silent non-host members leave after ten minutes. Hidden titles are redacted in snapshots, events and receipts, including replays; the shared follower pauses the previous title when the new title is hidden.

Cast playback continues independently of the phone. A returning controller can recover the same session with a private controller token, without pairing or loading again. Unpaired standard CAF loads/play/seek are refused. Denied takeover credentials are revoked, their directory entries retired, and generation fencing protects newer pairings from late cleanup. A refused takeover no longer causes the sender to close the receiver.

“Play on” retains the destination playback session and exposes pause, resume, seek, stop and reconnection in client-core using existing Playback v1 session operations. TV approvals and readiness are bound to the authenticated receiving device and a live session playing the proposed title. Receiver inboxes continue polling/listening while playback is in front.

All social protocol routes now belong to the typed apikit registry, with regenerated contracts. Optional request fields are marked optional in those contracts. Viewer routes preserve profile authentication without introducing a blanket device-session requirement; device-bound operations enforce their own device proof. The duplicate authored social OpenAPI file is removed. No commits, pushes, deployments, subagents or helpers were used. Work stayed in the named worktree and programme report/snapshot directories. Host: personal Ubuntu mini-1 (`ehler-homelab-mini-1`). Context-sync failed on its read-only state lock; the existing relevant context was read.

# Decisions I made

- Five-second readiness wait, resolved by a one-second process-wide sweep and also computed on reads. Old readiness cannot release a new seek/load. Readiness must match the playback revision and land within 750 ms of the commanded position. Play can bypass the wait. Mid-play buffering remains local.
- Initial title selection settles to ready; title changes in a playing group resume playing; a paused group remains paused. Completion pauses at the host’s reported final position, without adding automatic queue progression.
- Separate playback, settings, queue and governance fences. The general revision orders snapshots; telemetry does not advance it. Genuine competing playback commands still conflict and are not blindly resent.
- Non-host presence: away after 30 seconds, disconnected after 120, membership leaves after 600. Disconnected members do not block readiness or queue eligibility. In anyone-control groups, active participant activity sustains the group while the host is absent.
- Self-claim is allowed only after two minutes of host silence and only for a connected member with fresh readiness. A loaded title also requires readiness for the current playback revision. Host-directed transfer has the same target eligibility.
- The existing activity privacy setting anonymizes participation rather than hiding occupancy: neutral display name, `private: true`, withheld position/report time, visible readiness and room counts. This keeps the setting functional without invisible room participants.
- Four live groups per creator, eight new groups/hour; 120 commands and 120 telemetry operations/minute per member/group; sixteen active receiver slots when registering/reactivating a Portico receiver, thirty new registrations/hour; sixty handoff proposals/minute. Existing caps remain. Cast issuance budgets/authority are not redesigned.
- Same-profile receiver scope stays. Receiver/controller device ids come from authenticated sessions, never request bodies. Fingerprints remain rotation fences, not a claimed proof-of-possession key design.
- Cast controller recovery uses a 48-character random token delivered only to the paired sender. Browser storage is local to the installation; native apps have an explicit storage seam. This token does not replace the receiver’s server credential or change pairing authority.
- Remote commands use SessionsClient in one place. Pause/resume can retry one unchanged-state/generation conflict; seeks do not blindly retry an ambiguous conflict. Playback-core owns further session revision separation.

# Bugs

- **B-13.1 — fixed.** Playback/settings/governance/queue fences separated; routine member telemetry does not invalidate commands. Focused HTTP interleaving tests pass.
- **B-13.2 — already fixed on base, extended here.** Existing seek settling/in-flight protection retained; current-revision bounded readiness waits added. Slow-seek follower tests pass.
- **B-13.3 — base mitigation retained, extended here.** Existing measured lead/settling behavior remains; new seek/load waits reduce residual drift. Physical timing remains an acceptance test, not a proven device result.
- **B-13.4 — fixed.** Extrapolate elapsed playback before applying the new rate. Rate components and positions are bounded to avoid arithmetic overflow.
- **B-13.5 — fixed.** Presence and readiness timestamps separated; stale non-host membership retired; disconnected members do not hold readiness/eligibility.
- **B-13.6 — fixed.** No takeover at thirty seconds. Two-minute boundary, connected/fresh-ready target and governance fence enforced.
- **B-13.7 — base continuation fix confirmed; recovery fixed here.** Receiver keeps playing/reporting; controller token restores control without restarting the title. Native persistence wiring remains with client lanes.
- **B-13.8 — fixed.** Private participants remain visible and counted, but are anonymized and their personal activity withheld. Privacy tests pass.
- **B-13.9 — (b/c) already fixed, (a) fixed here.** Departed-member access and host-only authority identifiers retained. Current title redacted per reader in snapshots, events, fresh/duplicate receipts; follower handles the hidden-title transition. Screens must update the label as specified below.
- **B-13.10 — improved/fixed hot paths.** Non-host queue projections check only that viewer; readiness frames update client state without full group/queue rereads. Item visibility is memoized within event batches. No ceiling-scale performance benchmark was run.
- **B-13.11 — fixed.** Receiver approvals/heartbeats/readiness bound to the actual authenticated device; session scope, device, item, state and lease checked; commit rechecks destination and source item/liveness. Request device claims removed from the wire. Fingerprint omission is refused.
- **B-13.12 — fixed.** Active playback no longer suppresses inbox polls/hints. Taking one handoff does not block the heartbeat or grant inbox; overlapping inbox reads are single-flight.
- **B-13.13 — fixed for owned paths.** Social creation/command/telemetry/receiver/handoff budgets and resource caps added. Existing global admission and Cast budgets retained.
- **B-13.14 — not changed: accounts-security ownership.** Current base issues ordinary role-bearing Cast sessions; this lane deliberately does not change pairing authority. Integration must supply the narrow playback-only credential.
- **B-13.15 — fixed.** Standard CAF LOAD/PLAY/SEEK require the controller, including after departure. Local-room pause/stop concession retained.
- **B-13.16 — fixed.** Refused/disconnected/replaced pending takeover revokes the unused pairing and retires its directory entry. Cleanup carries the generation in If-Match, so delayed cleanup cannot revoke its replacement. Network-unreachable cleanup is best effort and still needs physical recovery acceptance.
- **B-13.17 — (b/d) already fixed; remaining owned cases fixed.** Pre-play reconnect phases preserved, seek/title changes wait, anyone-control activity is not re-paused solely because the host is absent. Empty pause stays a lobby.
- **B-13.18 — not finished in screens.** The shared follower passes the desired initial playing state, but web/native load adapters still need to honor it. See exact integration work below; screens are outside this lane.
- **B-13.19 — fixed in shared/server coordination.** Follower clamps to known local duration and handles an ended player as ready; fenced host completion pauses the shared clock once. No guessed global duration or new automatic-next feature was added. Client adapters must continue exposing duration after end.
- **B-13.20 — fixed.** Lower revision/playback revision/event ordinal snapshots cannot overwrite newer state; transport receipts and event state carry the playback revision.
- **B-13.21 — active-member fix already on base; remaining disabled-member/recipient policy belongs to accounts-security.** No access/invitation code changed in this lane.
- **B-13.22 — not fixed: outside owned social/casting paths.** Owner broadcast capacity/audience handling is operations/notification work. The audit’s Hosted audience subclaim remains unverified here.
- **B-13.23 — fixed.** Group/member text rejects control characters and direction overrides; direct text validation rejects invalid UTF-8, and existing length bounds are retained.

V08’s downloads findings/N-V08.1 and G2’s other-domain feature gaps remain with their respective lanes; they are not social scope or new-feature authorization.

# API changes

- Group snapshots add **settingsRevision**, **governanceRevision**, **timeline.waitUntil** (nullable), **member.private**; group state adds **waiting**.
- `/v1/groups/{id}/transport`: expectedRevision now means **playbackRevision**; receipts add **playbackRevision** and **groupState**. Transport events carry those fences/state and waitUntil.
- `/v1/groups/{id}/settings`: expectedRevision now means **settingsRevision**.
- `/v1/groups/{id}/host-transfer` and `/end`: expectedRevision now means **governanceRevision**.
- `/v1/groups/{id}/readiness`: sends **playbackRevision** and optional end evidence (**ended**); stale epochs are ignored. Readiness event activity is projected through current privacy, with private/displayName fields, and withheld position/report time for private members.
- Current hidden titles have empty itemId and retain currentEntryId. Membership is checked for event replay, and duplicate receipts are reprojected against current visibility.
- `/v1/receivers` registration removes body deviceId; `/v1/receivers/{id}/grants` removes body controllerDeviceId. Responses still expose their authenticated device ids. Approval/readiness/heartbeat from another device is receiver_required/403.
- `/v1/cast/devices/{id}` DELETE accepts optional **If-Match** pairing generation. Stale generation conflicts without revoking the newer pairing; omission means explicit current unpair.
- Cast namespace: `paired` adds **controlToken**; `status` can carry it to recover control with a changed SDK sender id. Standard media-channel control requires the paired controller.
- Play on controls reuse GET/PATCH/DELETE `/v1/playback/sessions/{id}`; no second command queue or new remote-control route.
- All group, receiver, handoff and Cast protocol routes are registered once through apikit. Generated `packages/contracts` and `server/api/portico.openapi.json` refreshed; `server/api/social.openapi.yaml` deleted. `social.md`, API index and guide checker references updated. The test-only apispec loader now reads the generated document as well as authored documents.
- Registry generation currently assumes an SSE route also has a JSON event-batch schema. The social group events endpoint remains SSE-only; its generated frame contract describes id/event/data. The API lane should allow SSE-only metadata instead of advertising a JSON response for this endpoint.

# Schema changes

Edited only social definitions in `0001_baseline.sql`:

- social_groups: settings_revision, governance_revision, wait_until_ms, wait_resume_state, activity_seen_ms; waiting state; widened resume_state for lobby/preparing/ready/waiting preservation.
- social_group_members: seen_at_ms and readiness_revision.
- Ran `python3 scripts/schema-sync.py`; known-column metadata and schema verification passed. No new migration or old-database compatibility added.

# Owner or viewer settings added or wired

- **No new settings.**
- **privacy.includeInWatchTogether** remains a profile-server boolean, default true, with its existing settings screen. False now anonymizes the joined participant and withholds individual position/report time, while keeping occupancy/readiness truthful. Client screens should render the private-member catalogue label.
- Existing group shuffle/repeat controls retain their settings and gain an independent fence. This lane does not add queue progression or redesign shuffle behavior.
- New catalogue messages: `web.together.state.waiting`, `together.privateViewer`, `playOn.controlUnavailable`.

# Needed from others

**Web and native client lanes (including TV/Fire/Vega shared clients):**

1. Update together state-label mapping for `waiting` using `web.together.state.waiting`; keep the existing screen populated during the hold. Play remains the go-ahead action. Use the shared GroupSessionService rather than sending old revision fences manually.
2. Treat **empty timeline.itemId + nonempty currentEntryId** as the hidden-current-title case (`web.together.hiddenTitle`), rather than “nothing chosen.” Keep the shared follower’s pause/buffering behavior.
3. Honor `GroupPlayer.load(itemId, position, playing)` in `web/src/app/together.tsx` and native `apps/native/src/app/together.tsx`: prepare/load in the requested paused state without an audible first frame. Continue exposing seeking/buffering and a known playable duration, including after natural end. Do not independently advance the room’s title before the follower can report completion. This is B-13.18’s remaining work.
4. Render `member.private` with `together.privateViewer`, without substituting profile names or displaying individual positions. Show self-claim only after the two-minute host deadline; enable host transfer only for fresh-ready connected targets.
5. Keep HandoffController alive after success; render its pause/play/seek/stop methods and remoteSession state. Remove the “use the TV remote” dead end. HttpLocalApi.requestRaw is used automatically; other API adapters must provide it or pass SessionsClient. Persist a committed handoff id when cross-restart remote reconnection is desired and call reconnect(id).
6. Route destination session-change events through the existing PlaybackSessionController/legacy bridge sessionUpdated path, so server PATCH/DELETE takes effect on the actual player. The shared operation path already exists; no replacement command protocol should be added.
7. Native CastController construction must provide installation-private controllerStorage. Browser local storage is already implemented. On SDK-resumed sessions call adopt(), which sends the stored proof. Use a stable receiver identity for storage if two TVs can have the same friendly name; the current fallback key uses server origin plus friendly name.
8. Stop posting removed receiver/controller device-id body fields. Updated ReceiverHost/HandoffController already do this. Use the server’s registered deviceId for receiver adverts/discovery.
9. Honor server cast.applicationId configuration; remove native/build-time fallback behavior when the configured value is empty (existing D-13.13 doc/code discrepancy, not changed in this non-screen lane).

**Accounts-security:** integrate narrow Cast authority without granting profile/admin access. Ensure a newly redeemed receiver credential can revoke **its own unused pairing** via generation-fenced DELETE `/v1/cast/devices/{id}`; this cleanup must not require broad account administration. B-13.14 and remaining B-13.21 stay with that lane. Preserve the behaviour changes in RevokeCastDevice when merging cast.go.

**Playback-core:** the only changed playback-owned seam is internal/playbackv1/social.go (device/state/lease facts). Keep those facts when reworking sessions. Session semantic revision separation is still your ownership; remote seek intentionally does not blindly retry an ambiguous 412. Reconcile receiver capability upload/profile shape with your single capability document.

**Operations/notification owner:** B-13.22 broadcast capacity and Hosted audience reconciliation.

**Documentation/API owner:** update the sharing contract’s old no-wait, invisible activity, host-claim and direct-LAN-control descriptions to these decisions; reconcile the old receiver fingerprint/credential designs. Use the generated social contract, not the removed YAML. Fix SSE-only registry generation as described above.

# Test notes for other environments

Verified on Ubuntu in this worktree:

- `GOFLAGS=-p=2 go -C server build ./...` — passed.
- `GOFLAGS=-p=2 go -C server vet ./internal/social ./internal/httpapi ./internal/playbackv1` — passed.
- `GOFLAGS=-p=2 go -C server test ./internal/httpapi -run '^TestSocial' -count=1` — passed on the final request contracts (11.7 s). Subsequent focused host/receiver/Cast tests passed after stricter eligibility and revocation-generation changes; the added group/name-bound test and stale-generation Cast cleanup/directory retirement test passed separately.
- Focused internal/social tests — passed, including private occupancy/activity, read/write gate behavior and sweeper tests.
- One-file Node runs: group-session 27 passed; social-playback 21 passed; remote-playback 10 passed; cast-sender 11 passed; Cast receiver 20 passed.
- `npx tsc --noEmit -p web/tsconfig.json` — passed; generated contracts were regenerated with apigen.
- Schema-sync verification and `git diff --check` — passed. No full test suite, native build, Docker build, deployment or physical playback run.

Cold device acceptance:

1. Fresh database using this baseline, two distinct profiles/devices on one server, one direct-playing member and one converted stream. Use a private test server with FFmpeg; do not enable conversions on the demo VPS. Play, seek while playing, change title, and seek while paused. Log command/readiness playbackRevision and seek ids. Pass: clock holds once, resumes on matching readiness or at five seconds; a stalled/absent member never extends the bound; paused title loads are silent; slow seeks settle and catch up without repeated cancellation.
2. Keep host controls open while other members heartbeat/report. Pass: controls work unless a real competing command/authority change occurs. Background a guest: presence changes at 30/120 seconds, it stops blocking eligibility, and membership leaves after ten minutes. At thirty seconds host claim must fail; after two minutes only a fresh-ready connected target may take over. Toggle activity privacy: occupancy/readiness remain, identity/position/time are withheld.
3. On Android TV/Fire TV/tvOS/Vega, keep a title playing, request Play on from a second device, approve on the TV. Wrong-device approval/readiness must fail; source continues until destination playback is real. After commit use phone pause/resume/seek/stop and check the destination player actually changes. Drop/reopen the phone and reconnect to the committed handoff.
4. Registered Chromecast/Google TV receiver over a trusted HTTPS origin, with casting configured. Start a film, sleep/kill the phone or leave Wi-Fi. Pass: TV continues progress/renewal, returning phone controls the same playback id with no new start or delete. Unpaired standard LOAD/PLAY/SEEK must fail; room pause/stop still work. Refuse a takeover: current film stays, unused credential/directory entry is retired. Race replacement pairing with delayed cleanup: stale generation cannot revoke the replacement. Check title end and explicit pairing revocation stop the TV even when the final progress report fails.
5. Repeat Cast recovery on browser, iPhone/iPad and Android with controller storage enabled. Test identical TV friendly names and storage refusal; failed adoption must leave the TV playing. These storage/SDK behaviours are not established by fixtures.

# Not finished

- B-13.18 paused initial loads and the listed UI/native storage/session-event integration belong to client lanes and remain required for complete first-party delivery.
- B-13.14, remaining B-13.21 and B-13.22 are outside this lane’s ownership and remain required where confirmed.
- Physical seek timing, actual Cast SDK/controller identity behaviour, native player receipt of session commands and all-platform acceptance were not run.
- No Watch Together redesign, chat, new remote keyboard/D-pad/volume protocol, automatic queue advance or richer handoff options were added.
- Cast cleanup cannot be guaranteed while the server is unreachable. A narrow receiver credential’s cleanup permission must be preserved by accounts-security integration. Friendly-name controller-storage keys need client identity wiring for duplicate TV names.
- Apikit’s generated social SSE route currently has the generator’s implied JSON batch response; the runtime remains SSE-only. This small generator limitation is recorded for the API lane.

Snapshots are cumulative under `_program/snapshots/social`. Milestones 001 and 002 saved. Final snapshot 003 records the verified final tree; no subsequent worktree changes.
