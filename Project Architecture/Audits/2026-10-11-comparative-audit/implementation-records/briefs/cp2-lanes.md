# Completion pass 2 (15 Oct) — the last open audit bugs and gaps

Read `cp-common.md` first: every rule there applies (no subagents/helpers, Justin's identity and no AI attribution, never push, vertical slices across server/web/native/TV/Vega, shared packages canonical in the server and mirrored with `scripts/shared-packages.mjs --write`, apikit scopes, baseline schema edits, performance discipline, don't weaken guards, report format). Differences:
- Your worktrees are `$G/server-cp2-<lane>` and `$G/native-cp2-<lane>` on branch `audit-implementation/cp2-<lane>`, from main-server `6ae3ccf4` / main-native `ea762a7`.
- Justin's product rules added since cp-common was written (all in `PROGRAM-BRIEF.md` §1 — read it): **player choices** (Settings decide every start; in-player changes last one playback only; no per-title memory), PiP has no subtitle support, collections have manual order, Administrator scope as built, "start next during credits" off by default, tuner pre-emption default = recording fails.
- A final verification lane is committing small fixes to `audit-implementation/main` now, and other cp2 lanes run in parallel. **Before you finish, merge `audit-implementation/main` into your branch in both worktrees**, regenerate (`go run ./cmd/apigen`), re-mirror (`--write`, commit the lock), regenerate licence lists if a lockfile changed, and rerun your tests. Web `npm run check` includes a hook lint (oxlint; `npm ci` in web if missing) and a rem-only font-size test; native has `npm run lint:hooks`.
- Report: `$PR/reports/cp2-<lane>.md` in the cp-common format.

Source for every item: the owner's report Appendix D rows and the lane reports named below in `_program/reports/`.

## cp2-viewer — viewer browsing and playback gaps (server + web + native + TV + Vega)
1. **B-18.27 remainder: library and saved-item pin controls.** `library-pins.ts` exists in client-core with no consumer. Give viewers pin/unpin (and order of pins if the model has it) on web, phone/tablet and TV/Vega where navigation shows pinned items; follow existing navigation patterns. Read cp-owner-parity.md §9.
2. **Whole-group playback** (cp-queues-playlists.md "Not done"): story-arc/range/non-season episode groups play the whole group via a server selector; **Home shelf rows** play the whole row via a server snapshot/selector of what the row showed (bounded, e.g. the row's members at request time, capped like the row toolbar's 500). Sorted Home row play should then send the server query too.
3. **Lists spanning libraries** (cp-browse-parity.md "Not finished"): a letter rail (server sort letters already exist on entries — provide the bucket/anchor facts for the cross-library pivot); **History across libraries** no longer stops at the 10,000 most recent plays (keep cost bounded and flat; e.g. paged keyset over the history index rather than a member pre-read cap).
4. **Music Playlists tab** (the list of playlists): the full sort/filter options appropriate to playlists, same as other groupings.

## cp2-native — native app gaps (native phone/tablet/TV/Vega; server only where needed)
1. **Remove an entry from a personal collection on native,** with the same occurrence-safe Undo the web has (cp-queues-playlists.md).
2. **Sorted My List / Favorites / History on native draw cards by kind** (square album art, posters for films/shows, etc.), as the unsorted lists did.
3. **"Forced only"** shown only for image subtitle tracks that mix forced and ordinary lines (as web), and verify our own mpv build honours the option (`third_party/media-engine/`, `_program/reports/own-mpv.md`). It is an in-player, session-only choice (player choices rule).
4. **Collection reorder with drag handles** on phone/tablet (menu moves stay as the accessible fallback).
5. **Seek-index exact resume on native**: Android and Apple audio engines use the server's per-file seek index for MP3/ADTS/Ogg (see cp-followups.md) to land exactly. Android: test what you can (Kotlin type-check, unit tests, emulator if available). Apple: write carefully, cannot be compiled here — list precise Mac checks.
6. **Forgot PIN on phones** (and tablets/TV if the profile PIN screen exists there): the same recovery the web offers (read cp-owner-parity.md §9 and the web flow; follow the existing account-security rules; don't weaken anything).

## cp2-server — server-heavy gaps and audit bugs (server + web + phone/tablet owner UI)
1. **Merging whole albums, books and artists** (cp-owner-parity.md decision 10): server merge of container titles with their tracks/parts/discs and personal state preserved, plus split/undo consistent with existing title merge; owner UI on web and phone/tablet.
2. **WebDAV source commands get the silent retry** (412/428 re-read and retry) like rclone mounts (cp-owner-parity.md §9).
3. **B-13.10 Watch Together cost at the room-size ceiling:** benchmark a room at the configured maximum members with realistic event rates (projection/event fan-out), find anything that grows with the square of the group size and fix it so cost per member is flat; record before/after.
4. **B-04.35 "Complete" scanning cost:** measure Default vs Complete scanning per file on a synthetic library (generated media, sizes representative of films/episodes/music), then implement the shared-decode optimization (one decode pass feeding fingerprint, loudness and any other Complete analyses) if it is not already there; record before/after per-file cost. Keep scan work bounded and off the write gate.

## cp2-reads — reads that still grow with library size (server; clients only if a contract changes)
Justin earlier deferred these; now do them. Sources: `reports/final-pass.md` §7, `reports/perf-reads.md` (remaining growth list), `reports/perf-rebuild.md`, owner's report §2. Use the scalebench tool (`server/internal/catalog/scalebench`, see final-pass.md §8 for exact invocations) at **50k and 200k** to show before/after shape; **never run 1M to completion** (Justin: run only until the shape is clear; a partial 1M seed/derive is fine if you need it, stop it early). Keep scratch ≤ 10 GB under `$PR/scratch/cp2-reads/`, delete DBs at the end.
1. Broad multi-word search prefixes (prefix-term expansion and the full-text relevance fallback; up to 258 ms at 200k).
2. Per-person credit counts in the People group.
3. Songs/episodes first page sorted ascending by duration or community rating (grows linearly; 70 ms at 200k).
4. Deep pages in fact-order sorts (counted blocks / keyset).
5. The full composed Home in one call (`HomeRows`): make it flat or retire it if no first-party client uses it (check web/native/Vega/TV first; Hosted?).
6. From perf-reads' list: indexes or maintained facts for filters on versions, duration, resolution, credit names and genre presence; search by year/date added; restricted Home (376 ms).
Every change must keep per-item write cost flat (measure the add/trickle cost too) and stay correct under restriction matrices. Record a before/after table per read at 50k and 200k.
