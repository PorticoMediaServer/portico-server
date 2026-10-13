# Live TV and DVR Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


Contract ID: `PC-LIVE-DVR`
Revision: `portico.october.1`
Status: **approved**
Owners: Product, Server Live Sources, DVR Scheduler, Playback, Client Foundations
Imports: `PC-FOUNDATION` viewer scope, permissions, and resource lanes
Coordinates with: `PC-IDENTITY`, `PC-NETWORKING`, `PC-PLAYBACK`, `PC-LIBRARY-SCANNING`, `PC-METADATA`, `PC-SAVED-DOWNLOADS`, `PC-SETTINGS-OPERATIONS`, `PC-ACCESSIBILITY-LANGUAGE`
Applies to: Portico Server, Web, Client Core, and every first-party client that advertises Live TV or DVR capability

## 1. Purpose and scope

This contract defines tuner-backed Live TV and profile-owned DVR from source configuration through guide presentation, capacity allocation, live playback, scheduling, recording, retention, and failure recovery.

It covers M3U/IPTV, Xtream-compatible sources, HDHomeRun, XMLTV/guide sources, future provider adapters that satisfy the same contract, multiple sources, channel mapping, guide imports, tuner capacity, live/timeshift sessions, one-time and series recording rules, conflicts, recording artifacts, retention, and restart reconciliation.

It does not define commercial streaming services, provider subscription/licensing, conditional-access decryption, DVD/Blu-ray capture, or Library Channels. Library Channels are server-programmed media schedules and MUST remain a separate source/tab/capability from tuner-backed Live TV even when they share guide presentation components.

This is a clean pre-release contract. Conforming implementations MUST implement this revision directly; they MUST NOT retain contradictory legacy source models, guide identities, conflict ordering, tuner counters, recording states, or completion heuristics. A one-time migration may transform development data, but a continuing compatibility branch requires a governed exception.

## 2. Authorities and privacy boundary

| Authority | Owns |
| --- | --- |
| server owner | source credentials, provider addresses, channel mapping, discovered/overridden tuner capacity, recording roots, global retention/storage policy |
| selected Portico Server | source/channel/program identity, published guide generation, tuner allocations, scheduler decisions, recording artifacts, failure truth |
| active authority-qualified `ViewerScope` | channel favorites/hidden state, one-time recordings, series rules, recording visibility, playback progress, keep/delete choice |
| playback contract | delivery plan, media grant, transcode/remux, timeline and player lifecycle |
| client | presentation, current guide viewport, non-durable drafts; never scheduler or capacity truth |

Consumer responses MUST NOT reveal source credentials, provider URLs, headers, raw import errors, tuner occupancy belonging to other viewers, recording filesystem paths, global storage paths, or another profile's rules/recordings. Owner operations require the owner role and the route's declared scope. Where the current registry marks an operation session-only, a broad key cannot substitute for a session.

Every viewer-owned favorite, rule, allocation, recording visibility record, playback state, and event retains the stable owner tuple plus independently checked authorization and operation revisions. A policy refresh never changes recording ownership or destroys its history. Same-string Local and Hosted IDs are never merged.

## 3. Source lifecycle

Canonical source states:

`draft -> validating -> active`

Side states are `refreshing`, `degraded`, `disabled`, `credentials-required`, and `removing`; terminal state is `removed`.

1. Source creation validates type, locator, credentials, response bounds, supported playlist/guide shape, and safe network policy before publication.
2. Test/preview MUST NOT publish a source, channels, guide, or credentials. Save is an explicit second action.
3. Secrets are plain at rest under server filesystem permissions, write-only after creation, redacted from responses, and never copied into viewer DTOs, URLs, logs, or diagnostics exports.
4. HDHomeRun discovery is advisory. The owner confirms the exact device/route and discovered tuner count before save. Discovery cannot overwrite an existing source.
5. A source has `tunerCountMode`: `discovered`, `configured`, or `defaulted`. The effective count and its provenance are visible to the owner. Zero MUST NOT mean unlimited. Unknown capacity is displayed as unknown with a conservative planning estimate of one; the estimate alone is not an admission cap. Confirmed physical limits or explicit owner caps govern allocation; see the Live/DVR architecture amendment.
6. Disabling stops new sessions and future rule reconciliation while preserving channel identity, guide history needed by existing recordings, rules, and recordings.
7. Removing a source with active allocations or retained recordings is prohibited. The UI offers Disable. Destructive removal becomes available only after dependent recordings/rules are separately resolved.
8. Updating a source uses an expected revision. Connection-critical changes validate before replacing the last working configuration. A failed validation leaves the active configuration intact.

## 4. Guide ingest, identity, and publication

### 4.1 Stable identity

- Source IDs are server-stable.
- Channel IDs are server-stable across refresh when provider identity/locator evidence identifies the same channel.
- A programme occurrence has a stable server ID and provider/source lineage. Its revision binds the published guide generation and observed interval. A corrected time or a new guide generation does not by itself create a different occurrence. Where provider evidence cannot prove continuity, expose an unresolved mapping rather than title-match a recording to another programme.
- Owner channel mapping explicitly connects tuner channel to guide channel. Automatic matching is proposed evidence and requires confirmation when ambiguous.

### 4.2 Atomic guide generations

Guide refresh states:

`scheduled -> fetching -> parsing -> validating -> staging -> publishing -> current`

Side/terminal states are `retryable-failure`, `invalid`, `cancelled`, and `superseded`.

1. The current guide remains queryable while a refresh stages. Clients MUST NOT see a half-imported source or blank guide merely because refresh is underway.
2. Publication atomically switches the source's active import generation after channels, programs, mappings, and reconciliation input validate.
3. Duplicate provider rows are deterministically collapsed. Invalid time intervals, unbounded payloads, hostile URLs, and impossible channel references are rejected or quarantined with owner diagnostics.
4. A failed refresh keeps the last valid guide and shows its age. `No guide data` and `guide refresh failed` are distinct states.
5. Guide queries use an explicit or server-resolved stable window. Cursor continuation pins that window, filter, source set, timezone, sort, and generation.
6. Server timestamps are UTC instants. Clients render the selected display timezone and handle DST repeated/skipped local times without changing recording instants.
7. A conforming source integration MUST publish at least seven days forward whenever the upstream source supplies that range, plus enough recent history to show in-progress and recently completed DVR items. A shorter upstream range is published honestly as a capability limitation; neither the server nor a client fabricates unavailable guide data. Clients read the published range rather than assuming seven days are always present.

### 4.3 Reconciliation after publication

The server reconciles only future `scheduled` work to the new generation. Running, completed, failed, incomplete, or cancelled recordings retain their historical decision context.

- A rule match is idempotent by `(ruleId, canonicalProgramIdentity)`.
- Time/title changes update the scheduled instance revision rather than create a duplicate.
- Removed programs become `guide-removed` and remain visible as an issue until the next policy decision; they are not silently deleted.
- Changed viewer access, disabled profile, channel policy, or rule state cancels unauthorized future instances.
- A stale client program ID is resolved only through canonical provider evidence; title/time fuzzy matching never silently schedules a different programme.

## 5. Consumer guide contract

Every capable client provides consistent access to:

- **Channels**: current/next programme, channel number/name/logo, favorite state, and play action;
- **Guide**: time-axis presentation with source, day, group/category, favorites, search, and stable cursor/window behavior;
- **DVR**: schedule, recordings, series rules, conflicts/issues, and storage state appropriate to the viewer.

Clients may adapt geometry (grid on TV/Web, list/time cards on narrow mobile), but MUST preserve source/channel/program identity, selected time, recording status, conflicts, and actions. A client without a full grid MUST still allow browsing future programmes and scheduling.

Required semantic states are `loading`, `retained-refreshing`, `ready`, `no-sources`, `no-channels`, `no-guide-data`, `filter-empty`, `permission-denied`, `server-offline`, and `guide-stale`. Owner setup instructions appear only to an authorized owner; other viewers receive neutral availability copy.

Channel favorites and hidden state are active-profile state. Hidden channels are excluded from ordinary browse but remain addressable in owner management and existing recording detail. Favorite order never changes the canonical channel number.

## 6. Tuner allocation and capacity

Every tuner-backed live session and recording owns one durable allocation lease against a source unless the source adapter publishes safe multiplex sharing as a future capability. Capacity is coordinated across server processes, not in process memory alone.

Allocation identity includes source, channel, kind (`live-session` or `dvr-recording`), consumer ID, lease generation/token, acquired time, heartbeat, and expiry.

Rules:

1. Acquire, transition-to-running, and release are transactional or fenced so a stale worker cannot release/revive a newer lease.
2. Active media requests heartbeat the exact live allocation. DVR workers heartbeat independently.
3. Stale leases are pruned only after a bounded grace period and artifact/session reconciliation.
4. Capacity errors include effective capacity, current demand relevant to the viewer, next safe retry when known, and a canonical message ID.
5. Clients MUST NOT infer unlimited capacity from missing values or start a second tune to test availability.

### 6.1 Reservation and priority policy

Accepted scheduled recordings are protected reservations for their padded intervals. Higher numeric rule/recording priority wins when reservations overlap beyond capacity; equal priority is resolved by earlier padded start, then stable recording ID. This order is deterministic across restarts.

Live TV is interactive but does not silently defeat a protected recording:

- A live session may start only if capacity remains available for protected reservations during its current renewable lease horizon.
- Before a reservation boundary would require its tuner, the client receives a typed `reservation-required` event and visible countdown.
- If the viewer does not stop or move and no other tuner becomes available, the server ends that live allocation at the protected boundary so the scheduled recording can begin.
- Portico MUST NOT interrupt one viewer's live session without the countdown/event merely because another client refreshed capacity.

This is the canonical policy: an accepted scheduled recording is a protected commitment and preempts a conflicting live session only at its reservation boundary after the typed event and visible countdown. Clients MUST NOT choose a different priority rule.

## 7. Live playback and timeshift

Live session states:

`requesting -> allocating -> tuning -> buffering -> playing-live`

Additional states are `paused-timeshift`, `playing-timeshift`, `reconnecting`, `ending`, `ended`, and terminal `failed`.

1. Tune creates one normal playback session and operation-scoped media grant. Baseline account authentication does not bypass the live grant.
2. The response publishes delivery plan, channel/program projection, allocation lease semantics, and effective live timeline.
3. The live timeline contains `windowStart`, `liveEdge`, `position`, `seekable`, and supported operations. Clients never invent a DVR buffer duration.
4. When timeshift is supported, Pause moves to `paused-timeshift`; Seek within the published window moves to `playing-timeshift`; **Go Live** seeks to the current live edge. Seeking before `windowStart` is rejected and returns current bounds.
5. When timeshift is not supported, Pause/Seek controls are absent or disabled with a truthful explanation. A client MUST NOT simulate Pause by stopping and retuning.
6. Provider interruption enters `reconnecting` while buffered media remains playable. Retry is bounded by source policy and `PC-NETWORKING`; final failure retains channel/program context and offers a new tune action.
7. Channel change creates/acquires the new session before releasing the old allocation only when capacity permits; otherwise it performs a fenced stop-then-tune and never leaks two leases.
8. Back/Stop closes the live session and releases the allocation. App suspension alone does not immediately mean Stop; platform policy uses bounded background grace and heartbeats.
9. Progress for live content records activity/session health, not a permanent resume point. Timeshift position may restore only within the same still-valid session/window.

## 8. Recording intents and rules

### 8.1 One-time recording

**Record** on a programme creates exactly one profile-owned scheduled recording. The server binds source/channel/program/generation, title snapshot, padded interval, priority, authorization revision, and optional rule link. Repeating the action with the same idempotency key returns the same recording.

### 8.2 Series rule

A series rule contains stable rule ID, active profile, source, optional channel restriction, canonical series/program evidence, match type, required/blocked keywords, allowed/blocked channels, start/end padding, retention days, maximum retained episodes, priority, enabled state, revision, and created/updated times.

The rule editor MUST show the effective meaning in ordinary language: new/all episodes where evidence supports it, channel restrictions, padding, retention, episode cap, and conflicts. If the provider lacks reliable new/series evidence, Portico states that limitation and uses only the selected supported predicates.

Rule edits are compare-and-swap with `expectedRevision` and idempotency key. Editing reconciles future scheduled instances transactionally. It does not rewrite historical recording metadata or delete completed artifacts unless the user separately confirms a retention/delete action.

Disabling a rule cancels its future scheduled instances and preserves completed/incomplete recordings. Deleting a rule requires a choice to keep or delete its future scheduled instances; completed recordings remain unless separately selected.

## 9. Conflict detection and resolution

Conflict computation is interval-based over padded times and effective per-source capacity. It includes one-time items and rule-generated instances, and must be rerun after source capacity, guide, padding, priority, rule, permission, or schedule changes.

Recording states distinguish:

- `scheduled`: expected to record;
- `at-risk`: currently satisfiable but threatened by live use, stale source, or storage forecast;
- `conflicted`: loses deterministic priority at one or more intervals;
- `blocked`: invalid/unauthorized/missing guide relation;
- `running`, `completed`, `incomplete`, `failed`, `cancelled`, `pending-delete`, `deleted`.

The conflict UI shows actual capacity, peak demand, overlapping items, losing intervals, and actions. Required resolution actions are: raise/prefer this recording, lower another rule/recording, edit padding/channel/rule, cancel this instance, or leave unresolved. A conflict is never communicated only by an icon.

If no resolution occurs, the deterministic scheduler records the winner(s). Losers transition to `failed` with `tuner-conflict`, not `completed` or silently absent.

## 10. Scheduler and recording lifecycle

Canonical lifecycle:

`candidate -> scheduled -> claiming -> running -> finalizing -> completed -> retained`

Side/terminal outcomes:

- `at-risk`, `conflicted`, and `blocked` precede start;
- `cancelled` may occur before/during running;
- `incomplete` is playable/recoverable partial output with a truthful warning;
- `failed` has no trustworthy playable artifact;
- `pending-delete -> deleting -> deleted` handles retention/manual removal.

### 10.1 Start and write

1. Scheduler processes due work in deterministic priority order without a global arbitrary item cap.
2. Before claim it revalidates profile, permissions, source, channel/program relationship, guide generation, rule revision, storage floor, and capacity.
3. Allocation acquisition and `scheduled -> claiming/running` are lease-token fenced.
4. Output writes to a lease-specific partial path. Only successful provider completion plus minimum duration/size/probe and atomic publication may become `completed`.
5. Provider/transcode stderr, credentials, URLs, and raw filesystem errors are diagnostics, never viewer copy.
6. A cancel request fences the worker and cannot be overwritten by a late success.

### 10.2 Restart and crash recovery

- Fresh lease owned by another process is left untouched.
- Stale running work is reconciled from allocation token, partial/final artifact existence, size, and media probe.
- A fully published valid artifact may finalize as completed after a crash.
- A valid partial is preserved as `incomplete` when safe; an untrusted/empty artifact becomes failed and is cleaned.
- The old worker cannot publish after a replacement lease generation exists.
- Scheduled work still in the future remains scheduled; elapsed work without an artifact receives a typed missed/interrupted failure.

### 10.3 Recording import and playback

Completed and valid incomplete artifacts are imported into a profile-scoped Recorded TV library projection while preserving recording ID and source/program provenance. They are ordinary playable media through `PC-PLAYBACK`, not a special unprotected file endpoint. Another profile cannot discover them through media ID guessing.

Incomplete playback visibly identifies the truncated interval. Progress, watched state, downloads, metadata, and deletion follow their respective contracts.

## 11. Storage, retention, and deletion

The owner configures recording roots and global reserve floor. Each profile rule may specify `retentionDays` and `maxRecordingsPerSeries`; the more protective applicable rule wins when global policy would otherwise delete earlier. A viewer can mark a recording **Keep**; kept recordings are exempt from automatic retention until Keep is removed.

Retention order is deterministic and visible:

1. explicitly expired, not kept;
2. over-series-cap, oldest completed first;
3. watched and beyond retention interval;
4. unwatched and beyond retention interval;
5. emergency storage-floor candidates only under an owner-approved policy.

Automatic retention MUST NOT delete:

- running/finalizing/incomplete-recovery artifacts;
- an artifact with an active playback/download preparation use;
- a kept recording;
- an artifact whose ownership/profile mapping is uncertain.

An eligible active-playback artifact transitions to `pending-delete` and is retried after use plus grace. Manual Delete requires confirmation, immediately removes it from ordinary browse, revokes future grants, waits/fences active operations, deletes bytes, then removes catalogue state. Failure remains `pending-delete` with owner diagnostics; reclaimed space is not reported early.

Storage status shown to ordinary viewers is their relevant recording use/availability, not raw global paths or other viewers' usage. The owner sees measured used/available bytes, reserve floor, forecast, pending cleanup, and write-test health.

## 12. Failure taxonomy and recovery

Canonical failure families include:

- `source-unreachable`, `source-authentication`, `source-invalid`, `guide-invalid`, `guide-stale`;
- `channel-unavailable`, `tune-failed`, `provider-ended`, `tuner-capacity`, `reservation-required`;
- `recording-conflict`, `storage-low`, `storage-full`, `recording-root-unavailable`, `permission-revoked`;
- `interrupted-restart`, `artifact-invalid`, `post-process-failed`, `client-update-required`.

The product distinguishes provider/source trouble, server trouble, local client/network trouble, and policy/permission denial. Retryable refresh/tune failures preserve the last valid guide and current screen. The UI never instructs an ordinary viewer to edit source configuration unless that viewer is authorized to do so.

## 13. Platform adaptation

- Television, mobile, and Web may use different guide geometry and remote/touch interaction, but all support programme detail, tune, recording status, and conflict meaning.
- A constrained platform MAY omit source administration and advanced rule editing. It MUST still show existing rule/recording truth and offer only capabilities it can complete.
- A client incapable of Live playback MUST hide/disable Live destinations based on published capability; it cannot display guide entries that pretend to tune.
- Platform players follow the same live timeline. Native seekable-window quirks are capability facts, not permission to invent a longer buffer.
- Printed/tiny links and pointer-only hover affordances are prohibited on TV.

## 14. Security requirements

All source fetches and redirects follow SSRF/network policy, size/time limits, safe DNS/IP revalidation, and credential redaction. Logos and guide artwork are fetched/served through bounded safe paths. Live/DVR manifests and segments require operation-scoped grants through headers or protected cookies; query credentials are prohibited.

Every viewer operation rechecks active profile restrictions and channel policy. DVR rule IDs, recording IDs, guide IDs, source IDs, and playback session IDs are not authorization. Owner endpoints and consumer endpoints use separate response types and operation classifications.

## 15. Conformance

`state-machines.yaml` defines normative vocabulary; `conformance-vectors.yaml` defines minimum hostile cases. Evidence must include:

- atomic guide-generation and stable-cursor tests;
- guide reconciliation and duplicate-rule-match tests;
- cross-process capacity, heartbeat, stale-prune, lease fencing, and deterministic priority tests;
- live/timeshift window, reconnect, stop, channel-change, and reservation-boundary tests;
- schedule/padding/conflict/rule revision tests;
- provider failure, disk-full, cancellation race, restart, partial/final artifact validation, and retention-during-playback tests;
- privacy tests proving consumer projections exclude secrets, paths, raw errors, global occupancy, and other-profile data;
- real client guide/tune/DVR interaction matrices.

### 15.1 Applicability matrix

| Rule | Server | Hosted | Web | Client Core | Apple | Android | Roku | Cast | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| source/guide authority and atomic publication | required | not applicable | owner/consumer projection | canonical models | required where offered | required where offered | required where offered | not applicable | guide-generation fixtures |
| tuner allocation and DVR scheduling | required | not applicable | controls/projection | canonical outcomes | required where offered | required where offered | required where offered | receiver consumes planned session only | lease/conflict fixtures |
| live/timeshift playback | playback authority | not applicable | required where capable | timeline model | required where capable | required where capable | required where capable | required where receiver supports Live | real player vectors |
| recording browse/privacy | required | not applicable | required | canonical projection | required | required | required | not applicable | cross-profile privacy tests |

An approved exception uses this exact record from `../governance.md`, with evidence preventing expansion:

```text
PORTICO-CONTRACT-EXCEPTION
Contract: PC-LIVE-DVR#<section-id>
Scope: <platform, surface, or capability>
Decision: <the exact deviation>
Reason: <why the canonical rule cannot or should not apply>
Approved-by: <name or recorded decision reference>
Review-when: <expiry date, capability change, or other objective trigger>
Evidence: <test or fixture that prevents accidental expansion>
```

The approved record MUST also appear in this contract's exception registry. A comment without boundary evidence is not an approved exception.

## 16. Product decision

Protected recording reservations preempt a conflicting live session at the reservation boundary after a visible countdown. This gives an accepted recording reliable meaning while preserving honest warning and a chance to move or stop the live session.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.

## October shared input and private recording behaviour

One upstream acquisition per source/channel serves all current viewers and DVR workers. Each has its own durable lease/private reader and may need its own conversion. Admission counts channels rather than multiplying tuner use per viewer. The last reader retains allocation until physical close after a three-second grace.

The same airing recorded by two profiles shares payload storage through a capture leader and immutable hard links, with independent private catalogue ownership and deletion. Active leader cancellation does not seamlessly transfer capture. Guide extension during a running capture is not adopted dynamically. Reliable season/episode facts publish a recorded show/season/episode hierarchy; title identity attach-point integration remains a handoff.

Playlist/guide import retains valid rows, reports bounded diagnostics, supports compressed guides and infers missing stops where possible. Large guides spool to private SQLite and publish bounded checkpoints. Default bounds are 50,000 channels, five million programmes and 512 MiB; owner limits remain real hostile-input/resource bounds. Full five-million-row performance has not been measured. Remote ingestion is distinct from the 48 MiB manual JSON preview budget.

Current live publishing is MPEG-TS. Shared planning filters conversion targets to that transport. fMP4-only live audio/video and full probe-time mpv selection remain unfinished. Reservation countdowns, actual tuners, Windows physical locks/retirement, capture recovery and device codecs need acceptance. Standalone XMLTV-only setup and catch-up playback are not delivered.

## October scope and evidence

Shared input/private recording code is implemented; real tuner/capture/Windows confinement and all-client reservation/presentation acceptance remain open.

Sources: [livetv](../../../_program/reports/livetv.md), [planner-wiring](../../../_program/reports/planner-wiring.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
