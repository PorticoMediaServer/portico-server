# Review of playback core

## Outcome
Seven independent findings: **0 blocker, 6 high, 1 medium, 0 low; all seven fixed**. Rejected starts/re-plans preserve working grants, queue links and progress authority. Session reads/replays enforce profile ownership and current item visibility. Refused mutations cannot cancel another viewer's preparations. Known audio bitrate ceilings now close copy/original routes; original/copy bandwidth and current playback locality count toward upload budgeting. Controllers preserve the playing device's engine and connection context.

Work performed on `ehler-homelab-mini-1`, worktree `server-review-playback-core`, base `aac183d`. No commits, push, deploy, helpers or agents. The context refresh failed because its lock path is read-only; canonical project, standards and verification guidance was read directly. Programme and lane briefs, the original lane report including merge notes, relevant audit bug entries, BUG-REGISTER and FIX-LOG were consulted.

## Decisions I made
- Keep same-profile control across devices; a shared device id alone never confers another profile's session.
- Stage candidate presentations privately, then publish the grant, v1 row and personal writer together. Failed candidates are stopped using an uncanceled cleanup context. A replacement remains private for at most one minute while being validated.
- A PATCH inherits its occurrence's personal-intent order. A successful new start, including an explicit replacement start, receives a new order. Prepared audio does not claim personal state merely by being built.
- Remember explicitly chosen tracks only after successful publication. Each PATCH attempt gets its own media/subtitle operation identity so an aborted candidate cannot poison a retry.
- Controllers use the playing device's current token-family binding and established locality; playing-device requests update locality from the owner's network classification.
- Preserve the planner's existing 10 percent bitrate tolerance. This review closes missing known-audio comparisons; it does not change conversion output selection or introduce stricter policy thresholds.
- Source bandwidth is conservative for copied audio/video (the whole original rate), rather than a zero conversion budget.

## Bugs
- B-02.1 — Already fixed. One document retains transport-specific codec lists; engine/container tests pass.
- B-02.2 — Already fixed. Passthrough remains separate and depends on the active audio output route; targeted test passes.
- B-02.3 — Already fixed. Embedded audio switching reaches the planner without a translation.
- B-02.4 — Already fixed. Device tone mapping is declared and tested independently of display HDR.
- B-02.5 — Already fixed. Transport tables, interlacing, codec normalization and subtitleRendering are retained.
- B-02.6 — Already fixed in server/shared client. PATCH re-plans in place; this review fixes its failure/publication semantics (F1). Player screens belong to client lanes.
- B-02.7 — Already fixed. No-key start is 428; legacy routes are absent. Lifecycle test verifies both.
- B-02.8 — Not fixed: sustained unfinished rewatch clearing watched is deliberate under the current contract, as V02 confirms.
- B-02.9 — Already fixed. HTTP owner-network classification supplies playback locality; F5/F7 correct persistence and controller reuse of that classification.
- B-02.10 — Already fixed. Old cellular HDR preference keys are gone; HDR is narrowed by the owner/capability override.
- B-02.11 — Already fixed. Device-aware best-version selection is bounded to eight planned candidates, with catalogue quality ordering.
- B-02.12 — Already fixed on base. Stored/current ceilings remain enforced; the strengthened subtitle route test now also holds MaxConversions=1.
- B-02.13 — Already fixed for engine failures. Timeline records appropriate engine errors; PATCH performs recovery. Error does not independently mark the title watched.
- B-02.14 — Already fixed in planning. fMP4 conversion transports exist; real producer/device qualification remains with conversion lanes.
- B-02.15 — Already fixed in planning/declarations. Codec tags close original Apple HEVC; targeted test passes. Physical AVPlayer evidence remains outstanding.
- B-02.16 — Not fixed fully: generic failures now lapse after six hours and final-route retries stay bounded; owner inspection/reset remains an acknowledged console feature gap. Memory is still keyed to sign-in families, not all sign-ins of an installation.
- B-02.17 — Already fixed. Audio and subtitles use the shared complete ISO language table.
- B-02.18 — Already fixed for analysed sources. Unknown-rate video is assumed from picture size under a ceiling; the deliberate unanalysed-source fallback still trusts codec summaries (not changed here).
- B-02.19 — Already fixed. Conversion off still permits copy-only repackaging; targeted test passes.
- B-02.20 — Already fixed. Premature ended keeps resume rather than completing below 80 percent of duration.
- B-02.21 — Already fixed. PATCH accepts the player position; shared controller sends it. In-place recovery tests retain it.
- B-02.22 — Not fixed here: finite VOD producer normalization is acknowledged, off by default and owned by the conversion lane.
- B-17.1 — Already fixed in the server/shared protocol path. Session changes and failure recovery keep the queue; this review hardens rejected changes. Client screen qualification remains separate.
- B-17.2 — Already fixed in the server/shared protocol path. Session changes and failure recovery keep the queue; this review hardens rejected changes. Client screen qualification remains separate.
- B-17.3 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.4 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.5 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.6 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.7 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.8 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.9 — Already fixed in FIX-LOG (`891cdec`); shortcut UI is outside this lane and was not re-tested here.
- B-17.10 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.11 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.12 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.13 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.14 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.15 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.16 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.17 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.18 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.19 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.20 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.21 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.22 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.23 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.24 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.25 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-17.26 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review.
- B-20.1 — Already fixed; same root as B-02.2. Output-route passthrough tests pass.
- B-20.2 — Already fixed; same root as B-02.4. Device tone-mapping test passes.
- B-20.3 — Already fixed in FIX-LOG; native implementation/physical acceptance is outside this review and was not re-tested here.
- B-20.4 — Not fixed in this lane: native player/device behavior is outside this worktree and requires its owning client lane.
- B-20.5 — Not fixed in this lane: native player/device behavior is outside this worktree and requires its owning client lane.
- B-20.6 — Already fixed in FIX-LOG; native implementation/physical acceptance is outside this review and was not re-tested here.
- B-20.7 — Not fixed in this lane: native player/device behavior is outside this worktree and requires its owning client lane.
- B-20.8 — Already fixed in shared canonical names and FIX-LOG native probe; native acceptance was not run.
- B-20.9 — Already fixed in the declaration/planner; same root as B-02.15. Physical acceptance remains outstanding.
- B-20.10 — Already fixed in shared androidProfile zero-channel handling; native probe/device acceptance is separate.
- B-20.11 — Already fixed in shared declarations: native Android claims text only until a renderer exists.
- B-20.12 — Not fixed in this lane: native player/device behavior is outside this worktree and requires its owning client lane.
- B-20.13 — Already fixed in FIX-LOG; native implementation/physical acceptance is outside this review and was not re-tested here.
- B-20.14 — Already fixed in shared androidProfile: software-only video is capped at 1080p; actual native probe reporting is a client-lane check.
- B-20.15 — Already fixed in the wire format: the unused features claims are absent.
- B-20.16 — Already fixed in FIX-LOG; native implementation/physical acceptance is outside this review and was not re-tested here.
- B-20.17 — Not fixed in this lane: native player/device behavior is outside this worktree and requires its owning client lane.
- B-20.18 — Not fixed in this lane: native player/device behavior is outside this worktree and requires its owning client lane.
- B-20.19 — Not fixed in this lane: native player/device behavior is outside this worktree and requires its owning client lane.
- B-20.20 — Already fixed in the shared merge: decoder and passthrough lists coexist and the active route is retained. Native sink reporting is outside this lane.
- B-20.21 — Not fixed in this lane: native player/device behavior is outside this worktree and requires its owning client lane.
- B-20.22 — Not fixed in this lane: native player/device behavior is outside this worktree and requires its owning client lane.
- B-20.23 — Not fixed in this lane: native player/device behavior is outside this worktree and requires its owning client lane.
- N-V02.1 — Already fixed. Shared recovery uses the accepted queue session and PATCH; failure-in-place regression verifies the server side.

## API changes
No routes, request/response shapes or key scopes added/removed. Existing registry declarations still require ScopePlayback for start/change/stop and ScopeRead for reads/options.
- `GET /v1/playback/sessions/{id}` and idempotent start replays now return not_found for another profile or newly inaccessible title.
- PATCH/DELETE leave preparation intact on authorization/revision/input refusal.
- `presentation.bitrateKbps` now reflects source rate for original/copied streams (conversion budgets for converted video/audio).
- PATCH retains the playing device's engine/locality when another device controls it and persists actual playing-device reach on new bytes.
`GOFLAGS=-p=2 go run ./cmd/apigen` succeeded; generated contract files did not change. Shared client parsers already accept these values and need no wire-shape changes.

## Schema changes
None. Existing playback_private_presentations rows hold provisional candidates; no migration or schema-sync was necessary.

## Owner or viewer settings added or wired
None added. Existing audio/video ceilings, upload capacity and conversion concurrency are enforced along the corrected paths. MaxConversions=1 excludes the old presentation when its staged replacement's subtitle plan reserves its own conversion.

## Needed from others
- Conversion output registration/target selection/producer qualification remain with the separate conversion lane; none was changed here. Only the HLS admission count was adjusted for private replacement ownership.
- Native/web player screens, native capability probe accuracy, mpv packaging, Automatic measurement and device codec/HDR/subtitle acceptance remain with client lanes.
- Owner route-failure inspection/reset and any wanted forget-track control remain the acknowledged console/settings gaps from the original report. Failure memory still follows a sign-in family, despite the original report describing it as per device.
- Original architecture/developer documentation describing removed capability/legacy session routes still needs its owning documentation lane to reconcile it with the programme decisions and current registry.

## Test notes for other environments
Passed on Ubuntu, with `GOFLAGS=-p=2`:
- Targeted httpapi regressions: rejected re-plan (invalid audio, induced revision race, final-write fault); rejected replacement start (audio, readiness, write fault); rejected ordinary start retaining the old progress writer; restricted GET/replay; playing-device locality; actual original bandwidth/upload budget; controller engine/locality.
- Existing session lifecycle, Original/default/Automatic quality stepping, throughput refusal, remembered tracks, engine/override choices, failure keeping the queue, subtitle sidecar/burn/original delivery, start readiness and private audio transition/fence/opening-byte checks. `TestPlaybackV1PreparedProgressActivatesOnlyAtCommit` additionally verifies no writer before preparation commit and the inherited writer/order after commit.
- `TestSubtitleReplanRoutesHoldToTheMembersCurrentCeiling`, strengthened with MaxConversions=1, covering separate subtitle routes renewing media generations as well as PATCH and current member ceilings.
- Targeted playback and playbackv1 quality, engine, passthrough, tone-mapping, force-stereo, codec-tag, conversion-off/remux, unknown-bitrate, subtitle-replan, access, refused-preparation and paused-state tests.
- `go vet ./internal/playback ./internal/playbackv1 ./internal/httpapi`; `go build ./...`; API generator; `git diff --check`.

Reproduce the important failures before fixes by applying the three new test files to the base. Tests F1–F7 fail on that implementation. F6 observed original rate=0 and subsequent ceiling=15808000 instead of 9808000 under a 20 Mb/s capacity with a 6 Mb/s stream. F7 observed engine=native and locality=local instead of the playing device's mpv/remote. F1's ordinary-start follow-up likewise failed before deferred activation was added.

Physical acceptance: use Apple TV, Android TV/Fire TV and an mpv-capable handset with two selectable audio tracks and a two-item queue. Change quality/audio/subtitles at a nonzero position and force a decode failure; pass means the same session/queue entry, correct engine, replacement generation and current position. Send an invalid track and stale If-Match; pass means the old media remains playable, progress continues and prepared next remains usable. Control an away mpv player from a home device with different capabilities; pass means its engine/remote ceilings remain in force. With one conversion slot, switch two burn-in subtitles; pass means no capacity refusal. Inspect a subsequent remote start under a declared upload capacity; its ceiling must account for original/copied streams already playing.

No full test suites, device/browser playback, GPU, native builds, Windows/macOS cross-builds, deployment or capacity tests were run. The subtitle fixture uses available FFmpeg/toolchain facts; user namespaces were unavailable, and the existing code reported its normal sandbox fallback. No sandbox policy was changed.

## Not finished
No independent review finding remains open. The final cumulative snapshot supersedes snapshot 001; it includes the ordinary-start and private-audio progress follow-ups. Physical playback and throughput acceptance are unverified. Existing client/console/documentation gaps listed above remain outside this lane. The review does not claim every original cross-client bug is implemented or verified, nor that every mutation in the separate subtitle service is now one transaction with the v1 resource layer.

## Findings (recorded before the corresponding implementation changes)
### F1 — Failed re-plans invalidate the working presentation
Severity: high. `server/internal/playbackv1/sessions.go:709–759`, `sessions_change.go:197–279`, `server/internal/playback/service.go:399`.
`CreateV1` commits termination of the old presentation before explicit audio validation, subtitle selection, and the v1 revision-guarded write. An invalid audio index stops the newly created presentation too, leaving the unchanged v1 row pointing at a stopped old grant. A racing state report or failed final write also leaves an orphan replacement and invalid old grant. The early media creation also claims personal progress before the change succeeds. Smallest sound fix: stage a private replacement without ending or claiming the old stream, then publish it, transfer its progress claim and retire the old stream in the transaction updating the v1 row; discard it on failure. Serialize account admission through PATCH as starts already do.

### F2 — Session changes cancel another viewer's prepared playback before authorization
Severity: high. `server/internal/playbackv1/sessions_change.go:57,321`.
PATCH and DELETE call `cancelPreparedFor(id)` before `session` checks ownership. Any signed-in caller who knows another session id can cancel its next item even though the request later returns 404. A stale If-Match or malformed change also cancels it. Smallest sound fix: move cancellation after successful authorized mutation, leaving preparations intact on refusal.

### F3 — Session reads and idempotent replays bypass profile/content fences
Severity: high. `server/internal/playbackv1/sessions.go:437–439,890–897,921,1225–1243,1012`.
`owns` grants access for a shared device id regardless of account/profile/authority; start replay does not check ownership at all; GET and replay do not recheck item visibility. A profile switch can expose the outgoing profile's title and grant from its ended row; newly restricted titles remain visible through GET/replay. Replacement validation also checks only device identity. Smallest sound fix: require account/profile/authority ownership, check visibility on read/replay using a read-only item authorization callback, and apply the same ownership fence to replacement starts.

### F4 — Copy routes ignore the audio bitrate ceiling
Severity: high. `server/internal/playback/delivery_plan.go:460–476,517–534`.
The target's audio ceiling is considered when choosing conversion audio, but original and copy-remux routes never compare it to the source audio bitrate. A client can request a small numeric audio limit and still receive the high bitrate original; owner ceilings have the same hole. Smallest sound fix: reject audio-preserving routes when the source exceeds the audio ceiling, without changing conversion target selection.

### F5 — Re-plans leave stale connection locality in upload accounting
Severity: medium. `server/internal/playbackv1/sessions_change.go:269`, `server/internal/httpapi/upload_budget.go:30`.
Admission correctly treats a re-plan from outside home networks as remote, but PATCH retains the original row's `location`. A session started at home and re-planned away is therefore omitted from subsequent remote upload budgeting. Smallest sound fix: persist current server-decided locality alongside the replacement presentation.


### F6 — Original and copied presentations report zero bandwidth to upload budgeting
Severity: high. `server/internal/playbackv1/sessions.go:789`, `server/internal/httpapi/upload_budget.go:30`.
`describe` derives `presentation.bitrateKbps` from conversion encoder budgets. Original and copy plans have zero encoder budgets, even when `Trace.BitRate` says the source is 6 Mb/s. Upload admission sums this field, so direct/remux remote sessions cost zero bandwidth and new sessions can exceed the owner's upload budget. Smallest sound fix: report the delivery rate (source bitrate for copy/original, encoder rates for conversion) using the existing `planKbps` helper. Reproduction will assert the response rate and actual subsequent upload ceiling.

### F7 — A controller device can change the playing device's engine and locality
Severity: high. `server/internal/playbackv1/sessions_change.go:195`, `server/internal/playback/service.go:94`, `server/internal/httpapi/playback_v1_routes.go:182`.
Same-profile control from another device is supported, but PATCH plans from the controller's principal/token and capability document. A controller without mpv can therefore change an mpv presentation to native on the device still playing it. It also treats the controller's connection as the media connection, allowing a home controller to remove remote constraints and upload accounting from an away player. Smallest sound fix: plan replacements using the playing device's media binding/capabilities; retain its established locality for controller requests, and read current member/upload ceilings for that established reach.

## Fixes
- **F1 fixed.** `PendingActivation`/deferred replacements use private candidate rows; `PublishPresentationTx` atomically activates the grant/personal writer with the v1 insert/update and retires the old presentation only on success. Error paths discard candidates without canceling cleanup. Original/replacement starts, audio validation, revision races and final-write/readiness faults have repository regressions. Actual media generation is refreshed before replacing a grant renewed by the separate subtitle route. Staged subtitle conversion shares the old slot. Track memory waits for publication. Prepared audio activates its inherited progress writer in its release/queue-commit transaction, with a new regression checking both sides of the boundary.
- **F2 fixed.** Cancellation moved after authorized successful PATCH; DELETE reaches it through authorized End. `TestRefusedMutationsKeepPreparedPlayback` covers unauthorized PATCH/DELETE, stale revisions and invalid state.
- **F3 fixed.** Account/profile/authority ownership is required for sessions/replay/replacement starts, and read-only item authorization is wired into the resource caller. `TestDeviceDoesNotConferAnotherProfilesSession` and `TestPlaybackV1ReadAndReplayRecheckVisibility` pass.
- **F4 fixed.** Audio-preserving original/remux routes reject known source audio above the target ceiling; `TestAudioCeilingClosesAudioPreservingRoutes` covers both containers without touching conversion output selection.
- **F5 fixed.** Replacement publication persists server-decided playing-device locality. `TestPlaybackV1ReplanRecordsCurrentLocality` covers moving both ways.
- **F6 fixed.** Presentation bandwidth uses `planKbps`; `TestPlaybackV1OriginalBandwidthCountsTowardUploadBudget` asserts both response rate and the budget for the next stream.
- **F7 fixed.** Same-profile controllers plan through the playing device's current binding and retain its media locality. Current remote/member/upload ceilings are read for that established reach. `TestPlaybackV1ControllerKeepsPlayingDeviceEngineAndLocality` passes.

## Snapshots
- 001 — Keep playback replacements safe and enforce session ownership and ceilings. Captures the seven initial fixes.
- 002 (final cumulative snapshot) — Activate personal playback state only when its session is accepted. Includes the ordinary-start and private-audio progress follow-ups. Final targeted tests, package vet, server build and diff checks passed; no worktree changes follow this snapshot.
