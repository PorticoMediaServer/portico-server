# Lane report: server-followups

## 1. Outcome

The small server follow-ups are implemented in `server-server-followups`, branch `audit-implementation/server-followups`. No commit, push or deployment was performed. Cumulative snapshots 001–003 cover the implementation milestones; snapshot 004 is the final complete tree, including the later artwork-ledger, live-role, preparing-playback and returning-root fixes. No worktree changes follow that final snapshot.

- An owner can preview and delete one physical version of a film, episode, track or audiobook file. The preview names one file and carries a fingerprint of the selected title/link/file. Confirmation cannot silently switch to a replacement file or another version. Other copies and personal state stay. The last copy marks the title missing and follows the existing missing-title rule.
- The review queue returns its exact waiting total. A server-wide list and an exact per-library summary exist, with three MCP tools. The shared badge helper uses that total instead of the first page. These are counts of the existing film/TV/anime metadata review queue.
- Local artwork usage is reported separately from fetched artwork. Verification found that successful new images never increased the existing limit ledger; that defect is also fixed.
- Item facts and content cards carry `missing`; detail still carries `missingSince`. Tracks additionally carry `discNumber`. Recent Home and Discover rows name their server-selected See all destination.
- The console can read active cache and backup destinations, reclaimable database bytes, the last reclamation and each pre-upgrade database copy's time, bytes and path, and delete one such copy. The settings document reports qualification of the saved FFmpeg path without a restart, whether it is in use, and a translated refusal reason code. Effective FFmpeg settings now reflect the actual startup choice.
- Role changes already retier signed-in device families atomically on this base. Client-core now accepts legitimate role changes during renewal, refreshes the live viewer every 30 seconds, and re-reads it on a 403, retrying once when the role changed. It retains identity/family/token-generation checks, protected refresh credentials and server retry waits. The existing web callback publishes these viewer updates; no screen was changed.
- Live TV sources retain a bounded list of valid groups and channel keys before filtering. The owner can page/search these choices, including excluded channels, and choose them again.
- Download requests can name `versionId` for one `mediaId`. Admission pins the actual file, including the default selection. The worker uses that file, and optimized downloads select only conversions made from it. Merging or splitting titles moves claims, grant records, receipt records, prepared conversions and request references with their title/file. Download usage is pageable by full authority/account/profile identity, with names for local accounts, and distinguishes prepared storage from original library files. An owner can revoke a profile's claims and offline permissions in bounded pages.
- Administrators can pause, resume and seek other profiles' sessions through typed APIs and MCP. Transport uses the existing revision, conversion seek and event pipeline; no media capability is disclosed. A pause while a session is preparing is retained, and becoming ready cannot overwrite a newer request. One remote-control document lists this profile's playback on its devices, including receivers, and strips hidden titles and their controls.
- A reported, known file that is gone is marked missing during paths-changed, before the queued scan runs, when its root identity and readable parent are verified. Filesystem inspection happens outside the database writer. Offline roots, changed roots, unreadable folders, unknown paths and private DVR libraries are protected. A verified returning root can clear its stale offline flag.
- Inventory has an indexed `assetId` lookup. Client-core's find-one-file helper now makes one request instead of walking every page.

## 2. Decisions I made

- A version deletion uses the existing owner preview/typed-confirmation/receipt vocabulary, with a new fingerprint scoped to one title and file rather than a catch-all library revision. A file used by another title is retained, as the preview says; only this title's link goes. Filesystem root, size and modified-time checks remain.
- No new trash, redirect route, API compatibility path, dependency, deployment or MCP enable switch was added.
- Waiting counts exclude `pending`, `searching`, `pending_search`, `pending_episodes`, `pending_children` and `pending_apply`. Lists can include or exclude those rows. Dismissed/retired titles and retired libraries are excluded.
- Original downloads use library files and contribute zero to `distinctArtifactBytes` and `storedBytes`. Claim bytes still describe bytes destined for devices. The summary remains exact while its people list is paged.
- Download ids and offline permissions survive title editing. Cached title metadata continues to travel with the claim; renewed receipt documents name the surviving title. The preparation lookup remains transactionally deduplicated; its former unique index became a non-unique index so merging two claims cannot invalidate either device's claim.
- Profile download revocation handles at most 100 claims and 100 receipts per call. Repeat with a new operation id while `more` is true.
- Source choice ids are provider channel keys, the values accepted by `enabledChannels`, rather than the viewer directory's hashed ids. Import limits also bound the retained choices. Cursors bind source, generation, kind and search, and retain a 512-byte limit.
- FFmpeg checks use the startup qualifier and process sandbox, with a 15-second bound and a short identity-based cache. Timeout is `unchecked`, not a claim that the build is incompatible. Validation never changes the running tool.
- Administrator transport is deliberately limited to pause/resume/seek. Termination stays on the existing route. Read and mutation receipts carry state and revision, not media URLs.
- The scan lane's live `targeted.go` was inspected read-only and still only queued removal reports. I added a separate evidence file and two local hooks; its pipeline was not restructured.
- Context refresh could not create its lock in the read-only context-state directory. Local project and verification guidance was read. Execution host is personal Ubuntu mini-1.

## 3. Bugs

The lane brief assigns no audit-domain documents or new bug ids. Relevant entries in BUG-REGISTER and FIX-LOG were checked; the cross-lane references below distinguish this work from their owning reports. “Not fixed here” does not assert that another lane left the bug open.

- B-01.3 — Already fixed: scoped key creation/authentication is on the base; new routes declare their scope.
- B-04.14 — Not fixed here: outside the assigned server follow-ups; see web-subtitles report(s) for the owning lane’s status. Not re-verified in this lane.
- B-04.26 — Not fixed here: outside the assigned server follow-ups; see web-subtitles report(s) for the owning lane’s status. Not re-verified in this lane.
- B-04.39 — Not fixed here: outside the assigned server follow-ups; see web-subtitles report(s) for the owning lane’s status. Not re-verified in this lane.
- B-05.8 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-05.9 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-05.18 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-05.24 — Already fixed for completed scans. This lane additionally closes immediate paths-changed removal reporting.
- B-05.25 — Already fixed: file deletion has no state-directory trash; unchanged.
- B-05.27 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-07.1 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-07.3 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-07.6 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-07.11 — Already fixed: raw item deletion route is absent; version deletion uses the approved owner workflow.
- B-07.13 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-07.15 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.1 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.2 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.3 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.4 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.5 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.6 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.7 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.8 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.9 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.10 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.11 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.12 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.13 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.14 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.15 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.16 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.17 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-08.18 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-09.13 — Already fixed: no trash copy is made; unchanged.
- B-09.18 — Already fixed by backup.SetDirectory on the base. This lane exposes the destination in use.
- B-12.3 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-12.5 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-12.18 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-12.19 — Not fixed here: outside the assigned server follow-ups; see identity report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.1 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.2 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.3 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.4 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.5 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.6 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.7 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.8 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.9 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.10 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.11 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.12 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.13 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.14 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.15 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.16 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.17 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.18 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.19 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.20 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.21 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.22 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.23 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.24 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.25 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.26 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-14.27 — Not fixed here: outside the assigned server follow-ups; see downloads report(s) for the owning lane’s status. Not re-verified in this lane.
- B-15.3 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-15.7 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-15.8 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-15.15 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-15.23 — Not fixed here: outside the assigned server follow-ups; see api-platform report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.1 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.2 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.3 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.4 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.5 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.6 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.7 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.8 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.9 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.10 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.11 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.12 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.13 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.14 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.15 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.16 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.17 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.18 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.19 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.20 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.21 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.22 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.23 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.24 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-16.25 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-17.6 — Not fixed here: outside the assigned server follow-ups; see web-subtitles report(s) for the owning lane’s status. Not re-verified in this lane.
- B-17.17 — Not fixed here: outside the assigned server follow-ups; see web-subtitles report(s) for the owning lane’s status. Not re-verified in this lane.
- B-17.19 — Not fixed here: outside the assigned server follow-ups; see web-subtitles report(s) for the owning lane’s status. Not re-verified in this lane.
- B-17.24 — Not fixed here: outside the assigned server follow-ups; see web-subtitles report(s) for the owning lane’s status. Not re-verified in this lane.
- B-17.26 — Not fixed here: outside the assigned server follow-ups; see web-subtitles report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.1 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.2 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.3 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.4 — Not fixed here: paged genre/series aggregates belong to the active search/sort lane.
- B-18.5 — Already fixed: library-removal service and typed route are on the base; unchanged here.
- B-18.6 — Already fixed for shows on the base. Not fixed here for the other containers: provider-specific refresh belongs to metadata/music lanes.
- B-18.7 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.8 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.9 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.10 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.11 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.12 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.13 — Already fixed: title deletion requires preview/confirmation; version deletion preserves that rule.
- B-18.14 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.15 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.16 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.17 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.18 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.19 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.20 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.21 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.22 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.23 — Fixed here: content entries now expose the track disc number; client parsing retains it.
- B-18.24 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.25 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.26 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.27 — Partly addressed here: local-artwork accounting now enforces its wired limit. Unrelated dead settings remain with their owning lanes.
- B-18.28 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.29 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-18.30 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.1 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.2 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.3 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.4 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.5 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.6 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.7 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.8 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.9 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.10 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.11 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.12 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.13 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.14 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.15 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.16 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.17 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.18 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.19 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.20 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.21 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.22 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.23 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.24 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.25 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.
- B-22.26 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane.

## 4. API changes

All new/converted routes are typed on apikit, declare `Requires`, have route-lane entries and the applicable restriction classification. Owner writes are covered by the matrix's automatic administration rule. `go run ./cmd/apigen` regenerated OpenAPI and TypeScript contracts.

| Route | Shape / change |
|---|---|
| `POST /v1/admin/items/{id}/files/{fileId}/delete/preview` | New: existing DeletePreview fields plus `assetId`, `previewKey`; exactly one file. |
| `DELETE /v1/admin/items/{id}/files/{fileId}` | New body `{confirmation,previewKey,operationId}`; existing DeleteResult receipts/totals. |
| `GET /v1/libraries/{id}/metadata/attention` | Adds exact waiting `total`, per-entry `libraryId`, query `includeInProgress` (default true). |
| `GET /v1/admin/metadata/attention` | New server-wide version of the same page; same cursor/limit/filter. |
| `GET /v1/admin/metadata/attention/counts` | New `{total,libraries:[{libraryId,total}]}` without loading titles/candidates. |
| `GET/PUT /v1/admin/metadata/settings` | Response adds `localArtworkUsedBytes`; write shape unchanged. |
| Item facts/content entries | Add boolean `missing`; track cards add optional `discNumber`. Detail's `missingSince` unchanged. |
| Home rows/Discover recent section | Add optional `seeAll:{pivot,sort:[{field:'added',direction:'desc'}]}`. |
| `GET /v1/admin/storage/status` | New database/cache/backup paths, reclaimable bytes, last reclamation, upgrade copies. |
| `DELETE /v1/admin/storage/upgrade-copies/{id}` | New 204 deletion of one validated pre-upgrade-copy id. |
| `GET/PATCH /v1/admin/console/settings` | Response adds optional `ffmpegValidation:{path,state,inUse,reasonCode?,missingFeatures,checkedAt}`; effective FFmpeg path/restart fields use startup state. No setting added. |
| `GET /v1/admin/live/sources/{id}/groups` and `/channels` | New pageable/searchable `{sourceId,generation,kind,total,items:[{id,name,group,channels}],nextCursor}`; queries `cursor,limit,q`. |
| `GET /v1/admin/libraries/{id}/inventory` | Converted existing route to typed registry contract; adds `assetId` query, retains source/state/cursor/limit and response. |
| Download preparation submission | Adds optional `versionId`, accepted only with one `mediaId`; preparation responses expose selected `versionId`. |
| `GET /v1/downloads/usage?scope=server` | Adds `cursor,limit`, `profilesNextCursor`; person rows add full identity, names and `storedBytes`. `distinctArtifactBytes` now excludes original files. |
| `POST /v1/admin/downloads/revoke` | New body `{profileKey,operationId}`, result `{claims,receipts,more}`. |
| `GET /v1/admin/sessions/{id}/controls` | New `{sessionId,revision,state,positionMs,actions}` without URLs. |
| `POST /v1/admin/sessions/{id}/controls` | New body `{action,expectedRevision,positionMs?}` for pause/resume/seek; same control receipt. |
| `GET /v1/playback/controls` | New own-profile remote document, `cursor,limit`, device/title/state/actions, no media URL. Hidden title/control fields are stripped. |
| `POST /v1/libraries/paths-changed` | Same shape; verified reported removals take effect before the queued scan. |

MCP additions: `list_needs_attention`, `library_needs_attention`, `count_needs_attention`; `control_playback` now falls back to the administrator transport API when the session is not the caller's own. Authorization remains in the normal API router.

Client-core includes parsers/services/types for the new shapes: versionDeletionService, serverMetadataAttentionService, storageStatusService, liveSourceChoicesService, playbackControlsService, DownloadsService.serverUsage/revokeProfile, selected-version submission, exact attention badges and one-request inventory lookup. Role freshness uses the existing LocalSessionRefresher and ViewerService. No screen built.

No route was retained as a compatibility alias or removed beyond replacing the legacy inventory registration with its typed registration.

## 5. Schema changes

Baseline only; schema-sync run after the changes.

- `download_preparations.source_asset_id` stores the pinned file token.
- `download_preparation_live` becomes a non-unique filtered index including that token. Submission still deduplicates inside the writer transaction; merging existing claims may preserve duplicates.
- Indexes on receipt/grant item ids and download request item/target ids support identity moves.
- `database_maintenance_status` stores the last reclamation time/bytes, with one baseline singleton row.
- `live_channel_choices` stores bounded pre-filter choice labels by generation/key, with a group index and generation lifetime.
- Generated migration digest, known schema and known columns refreshed. Existing pre-release development databases must be recreated; no old-database conversion added.

## 6. Owner or viewer settings added or wired

No new switches or settings.

- `localArtworkLimitBytes`: existing positive byte limit, default 2 GiB, existing Metadata screen. New images now increase its ledger; successful deduplicated images count once. New usage field is ready for that screen to adopt.
- `ffmpegPath`: existing absolute path, default empty (bundled/default tool), existing Playback screen. Qualification status is available on save/read before restart; actual installation still requires restart. Four refusal strings are in the catalogue.
- Missing-title rule: existing immediately / after_days / never, default after 30 days; last-version deletion settles missing state and immediate cleanup follows the existing rule.
- Source `enabledGroups`/`enabledChannels`: existing lists, default all; picker APIs now supply accepted values including excluded rows. Existing typed-list screen needs picker adoption.
- Viewer role freshness: automatic shared-client behavior, 30-second live reads, immediate re-read on 403 and one retry when the role changed. No owner/viewer switch. Failures stay quiet, sign-in persists, server waits are respected.

## 7. Needed from others

- Console/native screens should adopt versionDeletionService; missing cards; localArtworkUsedBytes; storageStatusService; FFmpeg validation reason; group/channel pickers; serverUsage/revokeProfile; playbackControlsService. The existing attention badge automatically uses `total` once it receives the new parsed page.
- Native/package integration must copy the updated shared packages and exercise role freshness on the real clients. The existing web refresher callback already calls ViewerService.rotate and writes the selected viewer record; no web session screen edit is needed.
- Scan lane: keep the new `removedPathEvidence`/`applyReportedMissingTx` hooks when merging targeted.go. They inspect files before acquiring the writer and recheck source/object identity in the transaction.
- Identity/search-sort lane: retain moveDownloadClaimsTx in absorbTx and SeparateInventoryVersion. This lane did not change show merging design, redirects or the title identity model.
- Active search/sort: browse-capabilities revision, aggregate paging and its screen refresh vocabulary remain theirs; not duplicated.
- Subtitle/planner/conversion lanes: fallback-font catalogue/delivery and SubtitleBesideStream remain theirs. No subtitle renderer or planner restructuring was done here.
- Live TV/settings lane: returning per-source import limit overrides, tuner account/profile display names and subtitle provider credential settings remain outside these additions.
- Product documents should describe the new version-deletion preview, immediate verified reported removals, download file pinning/identity moves, exact review totals and console status fields. Existing obsolete trash/grace-period architecture text remains with the identity/scan documentation update.

## 8. Test notes for other environments

**Run here:** Linux Go build and Windows/macOS cross-compiles of all packages; Go vet on touched packages; client-core TypeScript check. Focused Go tests passed for version deletion, exact attention/filter counts, split/merge download references, named-version pinning, usage/revocation, missing-card projection, source choices, storage status/deletion, incremental reclamation, local-artwork usage/limit, immediate path removal, role-family updates/cache scope, route scopes/lanes, MCP, administrator transport including preparing playback, and hidden-title remote controls. Selected client-core tests passed (53 administration/download/attention/helper tests and 33 session/renewal tests including live role changes). No full repository suite, browser, device, tuner or real alternate FFmpeg build was run.

**Final checkpoint after the application restart:** recovered from Git status and this report, without resetting existing work. Re-ran Linux/Windows/macOS builds, vet on the 11 touched service/API packages, client-core `tsc --noEmit`, the three playback-control cases, immediate reported-removal and artwork tests, and all 33 selected session/renewal tests; all passed. Regenerated API contracts again and checked the final diff for whitespace errors. No new schema changes followed the successful schema-sync. The known matrix failures below remain explicitly unpassed.

**Restriction matrix limitation:** the broader read/write matrix files fail on existing base route-classification gaps (download bundle/artwork/subtitle/request routes, console registry, token/subtitle writes, show refresh) and grant probes. These omissions are visible in the original files, not introduced by this lane. All added routes have their correct classifications; automatic owner-write classification is retained. The new remote-control hidden-title test, scopes and exhaustive route-lane test pass. Integration should repair/re-run the whole matrices.

**Runtime checks someone can follow cold:**
1. Start a fresh-baseline Linux/Windows/macOS server with two versions each of a film, episode, track and book file; enable library deletion. Preview one version, alter or replace that file before confirmation, then submit. Pass: replacement stays and the operation refuses or reports file_changed. Repeat without alteration: exactly that file/link goes, title id and personal state remain. Delete the last version under every missing rule; pass: missing/cleanup follows the rule, no trash directory/copy.
2. Make more than one review page, with waiting and pending rows. Pass: list.total and the aggregate summary equal all waiting titles, not page length, and dismissing one decreases the total. Call the three MCP tools with a scoped owner key; a member key must not read them.
3. Import distinct local covers until the byte ceiling. Pass: usage rises once per stored image, a duplicate costs nothing, a second distinct image exceeding the remaining allowance is refused; fetched provider artwork changes no local-use counter.
4. Make an upgrade copy, read storage status, delete its listed id. Pass: reported path/time/bytes match disk and only that file goes. Traversal/symlink ids are refused. Delete enough database rows to create free pages; after maintenance, reclaimable bytes fall and lastReclaimedAt advances.
5. Save a nonexistent, incompatible and qualified custom FFmpeg path. Pass: unreadable/refused/accepted status and reason appear before restart, no old stream changes, effective/inUse remain the running build. Restart the accepted build; pass: effective path and inUse agree. On each OS confirm the process sandbox is still used.
6. Promote/demote a signed-in member while its browser/app is open. Pass: server authorization changes on the next request, current viewer UI changes within 30 seconds without credential renewal/sign-out; an administrator 403 causes a live viewer read. A second profile, Cast target or viewer-only session must retain its narrower role. Interrupt connectivity or return 429 with Retry-After; pass: no sign-out, no retry before the wait.
7. Import a source with groups News/Sports, enabling only News. Pass: both groups and Sports channels appear in picker pages; selecting Sports and refreshing admits it. Page/search across an import near its limits; pass: no duplicate or dropped choices inside the admitted bound, stale-generation cursors are refused.
8. Queue an explicit lower version, then add a better copy before work runs. Pass: downloaded bytes/hash remain the requested lower file. Merge titles and split the selected file; pass: preparation/grant/receipt references follow the survivor/new title and other versions stay put. Check usage with two profiles, originals and shared converted output; pass: originals occupy zero prepared bytes and shared output counts once. Revoke the target profile in repeated pages; pass: claims/permissions end without affecting the other person.
9. Play from a member on another device. As admin pause/resume/seek through API/MCP and while preparing. Pass: that player receives session.updated and obeys without an audible first frame when paused; receipt has no URL. Members cannot use the admin route. Remote document lists own-profile devices only and hides a title/controls immediately after its restrictions change.
10. Delete a known file outside Portico while its parent/root stay readable; report that exact path. Pass: it is missing before the queued scan runs. Unmount/replace the root, remove read permission, report an unknown path or a private recording: pass is no immediate missing mark. Verify on a real network mount as filesystem readability/identity was tested only with local fixtures.

## 9. Not finished

- No new screens, browser visual verification, native SDK/device build or real remote playback/FFmpeg/tuner test. Remote transport delivery relies on the player lanes consuming session.updated; the server/API behavior is tested, actual pause/seek on receivers is not.
- TV/anime split remains the explicit episode-placement workflow. The generic “separate” action still refuses it: the episode-position uniqueness contract requires choosing a new show/season/number, and this lane did not invent one. Deleting one episode version works through the new generic playable-file path.
- Multipart titles still cannot be downloaded as one file, even when one file is named: existing several_files refusal remains. No new bundle-of-video-parts design.
- Cached offline receipt documents on already-disconnected devices cannot be rewritten during a merge/split; they receive the current title identity on renewal. Their metadata/assets remain usable offline. No merged-title redirect was added.
- Music-specific review queues are not folded into the film/TV/anime attention summary. Counts describe the existing screen-metadata queue only.
- Additional requests left to their active owning lanes: browse-capabilities revision, genre/series paging, refresh on all metadata containers, edition/planner grouping, fallback fonts, per-source limit overrides, tuner person names and subtitle provider credential settings. No MCP toggle was built because none is wanted by the shared decisions.
- Existing restriction matrix classification failures require integration cleanup; they were not hidden or treated as a passing check.
