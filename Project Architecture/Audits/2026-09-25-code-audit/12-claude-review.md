# Claude review of the 25 September audit

Adversarial review of all 57 entries against the source at the audit's baselines. For each entry: a decision (accept, narrow, reject, conditional), a final priority, and the evidence.

**What "verified" means.** I checked these entries directly in the code: every P1; HS-003/004/005; SV-005/010/011/012; WC-003/006; NC-001/002/006/019/020. The rest were judged on the report's cited evidence and the project's product rules, without a second code read.

Priorities here:
- **P1:** broken for ordinary users, or a real security gap. Fix before anything else.
- **P2:** a real defect in a supported flow.
- **P3:** narrow, rare, tooling, or defense in depth.

## Decisions

### Fix first (P1)

| ID | Decision | Evidence and reasoning |
|---|---|---|
| SV-001 | **Accept, P1** | Verified in `subtitles/refresh.go:36–55`, `subtitles/provider.go:268–298` and `lyrics/acquisition.go:191–264`. Each opens `gated2`, then defers and commits the already-finished `gated`, so the process-wide write gate stays held. One successful subtitle refresh or search, or lyrics acquisition, stalls every later write on the server until restart. Fix: finish `gated2`, preferably by moving all three onto `WithWriteTx`. Add a guard test that fails when a `dbwork.Begin` result is never finished. |
| HS-001 | **Accept, P1** | Verified: `oidc_flows.go` StartFlow makes `completion` optional, and only `"code"` sets `native_completion`. Every current client sends `code` (Hosted always advertises it), so the opt-out only serves nonexistent older builds. Fix: reject a native, non-Apple start without `completion: code`, and drop the legacy test that requires the opt-out. |
| WC-012 | **Accept, P1** | Verified: direct sign-in, credential storage and many actions use `crypto.subtle`, `crypto.randomUUID()` and `navigator.locks`, none of which exist on `http://<LAN-IP>:32500`. That breaks the rule that plain HTTP is first-class, and it's the Plex-style `:32400/web` path. Needs a design decision (below). |

### Real defects (P2)

| ID | Decision | Evidence and reasoning |
|---|---|---|
| WC-001 | **Accept, P2** (from P1) | Verified: `list-position.ts` keys by library, pivot and query only, and nothing calls `clearListWindows()`. A same-tab profile switch can show the previous profile's browsed titles, and keeps them if the new result is empty. Household-scoped, but fix it: key by server and profile, and clear on viewer change. |
| HS-002 | **Accept, P2** (from P1) | Verified: `dns-healthcheck` runs after `OpenTable`, which deletes unreferenced journals and truncates tails. It's invoked only by the two deploy scripts, right after a start, and the table is a rebuildable cache of Hosted's feed. Fix: dispatch the healthcheck before opening the table, plus an interprocess lock. |
| SV-002 | **Accept, P2** (from P1) | Verified: `writeMarker` is a plain `os.WriteFile`, and `main` continues on every `ApplyStaged` error. An unresolved restore can start on a fresh empty database, while the original stays in the pre-restore folder. Needs a crash or I/O failure mid-restore; restores are rare. Fix: atomic marker write (temp file, fsync, rename), and refuse ordinary startup while a restore is unresolved. |
| SV-003 | **Accept, P2** (from P1) | Verified: `source_files` in `build-server-release.py` takes `.go` files, `go.mod`/`go.sum`, fixtures, testdata and `receiver/cast`, but not the embedded `migrations/*.sql` or `fonts/*`, so the snapshot won't compile. This builder isn't used by today's demo deploys; it blocks only a future public release. Fix: copy the whole module tree minus excluded directories. |
| SV-004 | **Accept, P2** (from P1) | Verified: the web app imports `@i18n`, and the Dockerfile never copies `packages/i18n`. Docker isn't used in production today. Fix: one COPY line, and add i18n and apikit to `container-release.mjs`'s digest (with SV-010). |
| Cast: SV-012, SV-008, SV-007, SV-009, HS-009 | **Accept as one item, P2, required before release** | Verified: `receiver/cast/receiver.js` calls `/v2/playback/*` and `/v2/queues/*`, and the server registers no `/v2` routes (tests assert 404). Hosted-authority pairing can't finish (SV-008), failed attempts don't count (SV-007), lost reconnects force re-pairing (SV-009), and the Hosted CSP blocks the sender SDK (HS-009). Cast doesn't work end to end. Chromecast is part of the Play-on rule, so port the receiver to Playback v1 and fix the issuance path together. |
| NC-006 | **Accept, P2 (high)** | Verified: `app/downloads.tsx:84–89` returns after `requestContainer` without `want()`, and the collector only fetches wanted items. A season or show download is prepared on the server but never reaches the iPhone or iPad. Headline feature. |
| NC-019 + NC-001 | **Accept, P2, fix together** | Verified: `ContentProvider.swift:133` replaces the poster URL's query with `size=thumbnail`, dropping the required `grant`, so Top Shelf is empty on a fresh install. `refreshTopShelf` reuses the token by origin alone and is cleared only inside the authenticated shell, so once posters work, the previous profile's Continue Watching would show on the Apple TV home screen. |
| NC-020 | **Accept, P2** | Verified: `app/home.ts:93` fences on `prev.document !== document`, so when two shelves page at once, the second's page is dropped and it stays in `paging` until Home reloads. Fix: fence on the load generation and always clear `paging`. |
| WC-003 | **Accept, P2** | Verified: `track.ts` `startAt` walks packets from 0, and `mp4Packets` reads every byte. Resuming an audiobook (`audiobook_file` is client-decoded on v1) eight hours in downloads everything before that point. The MP4 fix is small (start `mp4Packets` at the sample index from the in-memory table). MP3 and FLAC need seek tables: follow-up. |
| WC-006 | **Accept, P2** | Verified: `static-collection.ts` slices `from = start - base` against page-aligned starts, so with a remembered base of 170 and pages of 60, items shift by 50 on Back. Small fix. |
| WC-005 | Accept, P2 | Container download followers stop after an ordinary refresh; the progress looks stuck. |
| WC-010 | Accept, P2 | The guide fetches the whole lineup for each time window. The spec accepted this stopgap for up to 100 channels, but IPTV lineups with thousands are common. Needs the windowed guide read on the server. |
| NC-002 | Accept, P2 | Verified: `fragmentPairs` indexes `kv[0]` after a split that drops empty pieces, so `portico://invite#=` crashes the app. One-argument fix. |
| NC-007 | Accept, P2 | More than 100 held receipts sends one oversized renewal, which is refused, so every receipt expires. Fix: chunk by the server's batch limit. |
| NC-012 | Accept, P2 | A late playback preflight can start the wrong title, or reopen after close. Fix: a request-intent generation. |
| NC-014 | Accept, P2 | Search and Saved mount every loaded page. That breaks the O(visible) rule; use the windowed grid. |
| NC-023 | Accept, P3 | A very fast first play before capabilities load keeps music on the JS path for that queue's lifetime. |

### Small fixes, rare paths, tooling (P3)

| ID | Decision | Note |
|---|---|---|
| HS-003 | Accept, P3 | Verified: a retry of a native MFA sign-in after a lost response is refused (the replay needs `completed`). One repeated provider sign-in. |
| HS-004 | Accept, P3 | Verified: consumed refresh tokens are pruned after a day even when a live receipt needs them. A lost refresh response plus a day offline means sign-out ("virtually never signed out"). Fix: `NOT EXISTS` in the prune. |
| HS-005 | Narrow, P3 | Verified: shutdown and install acknowledgement don't advance the server-list version; accept those two. Reject the presence part: clients probe servers directly by design. |
| HS-006 | Accept, P3 | Two member removals within 24 hours cut the first grace label short. |
| HS-007 | Accept, P3 | The restore drill's synthetic sign-in lacks the continuation header, so it always fails. Tooling. |
| HS-008 | Accept, P3 | Failed Hosted activation doesn't restore the previous units. Worth automating, and run config checks before migrations. |
| SV-005 | Accept, P3 | Verified: the pre-restore manifest lists `portico.db`, which the resolver rejects. One manifest shape. |
| SV-006 | Accept, P3 | Two overlapping owner restores share the staging paths. Serialize them. |
| SV-010 | Accept, P3 | The release and container source digests omit apikit (and i18n). Fold into the SV-003/004 fix. |
| SV-011 | Accept, P3 | Verified (written 25 Sep): `recLoadTaste` overlays every pending job for the profile. It's large only right after a mass history import, until the worker catches up. Bound the overlay without dropping recent negative feedback. |
| WC-002 | Accept, P3 | Cancelling before the audio deck opens throws in cleanup. |
| WC-007 | Accept, P3 | As downgraded. |
| NC-003 | Accept, P3 | The verification cache is keyed by receipt ID; key it by content hash. No injection path exists. |
| NC-005 | Accept, P3 | Recent searches are module-global and ignore the history preference on display. Trivial. |
| NC-008, NC-009, NC-013, NC-015, NC-021, NC-022 | Accept, P3 | Real but narrow: the storage cap ignores in-flight transfers, bookkeeping acknowledges failed writes, an interruption resumes an intentional pause, and there are accessibility gaps (Field label, timeline adjust actions) and a stale channel API after re-sign-in. The accessibility items matter for App Review and VoiceOver users; batch them. |

### Reject, or defer as defense in depth

| ID | Decision | Reasoning |
|---|---|---|
| WC-004, WC-008, WC-011, NC-010, NC-011 | **Reject for now** | All assume a hostile or broken server feeding oversized bodies. Clients talk to the owner's own server; timeouts and admission already bound exposure. Not Fort Knox. Revisit if third-party servers or proxies become a real path. |
| WC-009 | Narrow, P3 | Keep this one: a corrupt media file in a real library can reach the MP4 parser. Validate table counts against the box size. |
| NC-004 | **Reject** | Falling back to shared defaults when the Keychain write fails stores a limited, read-only feed token in the app group. Under the openness rule (plain data, folder permissions protect it), that's acceptable, and the fallback exists for unsigned builds. |
| NC-024 | Conditional; no action | Native calls are already serialized; no leak shown. |
| AX-001, AX-002, AX-003 | Hand to the dataset project | These concern `portico-recommendation-enhancement` (Codex's project): retries on non-JSON errors, the wheel missing its SQL schema, and export snapshot consistency. They're real tooling defects; pass them to that project. |

## WC-012: options

The web app must work over plain HTTP on the LAN. Options:

1. **Recommended: insecure-context fallbacks in the web bridge.**
   - `randomUUID` from `crypto.getRandomValues`, which is available on HTTP.
   - Credential storage unencrypted in IndexedDB when `crypto.subtle` is missing. That's consistent with the openness rule; the browser's origin isolation still applies.
   - A single-tab or BroadcastChannel lock when Web Locks are missing.
   - Server proof verification with a small audited pure-JS Ed25519/SHA-256 implementation when `crypto.subtle` is missing.

   HTTPS behavior is unchanged.
2. Require HTTPS for the web app off-localhost (a claimed server's `direct.getportico.tv` certificate). This contradicts the HTTP-first rule and leaves unclaimed servers without web access from other devices.
3. A server-side session mode for the self-served web app (cookie session, no client proof). Simplest for the browser, but a second session model to maintain.

## Outcomes (25 September)

Every accepted entry is fixed and committed locally (nothing pushed). AX-001 to AX-003 went to the dataset project, as decided. Rejected entries are unchanged.

| Repository | Commit | Entries |
|---|---|---|
| portico-server | cc180d6 | SV-001, plus an AST guard test that fails when a `dbwork.Begin` handle is never finished |
| portico-server | ce65f8c | SV-002, SV-003, SV-004, SV-005, SV-006, SV-010, SV-011 |
| portico-server | bc9a836 | WC-001, WC-002, WC-003 (MP4), WC-005, WC-006, WC-007, WC-009 |
| portico-server | 910564f | WC-012, option 1: insecure-context fallbacks. Checked in a real browser at a LAN IP over HTTP |
| portico-server | 935abd4 | WC-010: `GET /v1/guide` takes `channels=` (≤50) and `programmes=none`; the guide asks for the channels on screen |
| portico-server | 159225f | NC-023 (the client-core half) |
| portico-server | e84c27f | Cast: SV-012, SV-008, SV-007, SV-009. Migration 0002 |
| portico-internal | 7dcf524 | HS-001 |
| portico-internal | 1816166 | HS-002, HS-003, HS-004, HS-005 (narrowed), HS-006, HS-007, HS-008, HS-009 |
| portico-react-native | 66596f9 | NC-001/019, NC-002, NC-003, NC-005, NC-006, NC-007, NC-008, NC-009, NC-012, NC-013, NC-014, NC-015, NC-020, NC-021, NC-022, NC-023 |

**How Cast now works:**
- The receiver plays through Playback v1, like every other client: an idempotent session start, the presentation URL, timeline reports for progress and the lease, PATCH for a seek the server must produce, and DELETE to end.
- The v1 presentation gained `container`, so CAF is told the HLS segment format and a direct file's type.
- Pairing consumes the code, stores the device and issues the session in one transaction. A Hosted viewer's session goes through the verified Hosted policy horizon.
- A lost reconnect recovers with the replaced token for two minutes. That works only for the same device, and only until the new token is used.
- Failed codes and tokens are charged per source network and per server, in their own transaction.
- A Go test checks every API path in `receiver.js` against the router.

**Found while fixing:**
- Group invite joins had the same rollback-erases-the-charge defect as SV-007. Fixed the same way.
- A revoked Cast pairing left its viewer session alive. It's now ended.
- Each Cast reconnect used to leave a live session family behind. Now each device keeps one.
- Offline-download errors were published but never shown on the Downloads screen.
- Two Apple screen tests mocked the wrong module paths after the repository split. They had been failing silently and now pass.

**Still open (Backlog):**
- A physical Chromecast and Google TV pass, plus the real application ID.
- Sidecar subtitles on Cast.
- MP3 and FLAC seek tables.
- The Apple Saved History tab isn't windowed.
- The unused `profile-vault-crypto.ts`.
