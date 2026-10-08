# Channels and Guide: Live TV and Library Channels (design spec)

Applies to web, iPhone, iPad and TV. The shared model (§9) is in client-core.

Sources:
- the existing guide contract (`packages/client-core/src/channel-guide.ts`, `GET /v1/guide`);
- windowed guide reads, and Library Channels as scheduled VOD;
- the one event stream (`/v1/events`);
- the demo Live TV fixture (`deployment/demo/portico-live-fixture.py`: 14 channels, 9 days, a no-guide channel, a channel with gaps, a very long title).

## 1. Principles

1. **One destination, Channels, organized by source.** Each channel source is its own channel library, named by its owner, and **Library Channels** is one more source. Sources sit under Channels the way libraries sit under the library section. With two or more sources, **All channels** combines them and is where Channels opens. (IA approved by Justin, 23 Sep.)
2. **The server owns the schedule; the client owns the viewport.** Clients never compute a schedule. They choose which channels and which hours to look at, and ask for exactly that.
3. **O(visible) in both axes.** The client holds only the channels near the visible rows and the programs near the visible hours. A 1,000-channel, 14-day guide costs what fits on the screen, plus a small prefetch margin. Measured in the §9 tests.
4. **Time is real time.** Everything is laid out in UTC milliseconds. The viewer's time zone (region preference, else the system's) is used only to label the ruler and to find day starts. DST days have 23 or 25 hours, and the grid shows exactly that.
5. **Honest gaps.** Where the guide has no data, the grid shows it plainly ("No information"). Nothing is stretched or invented. A channel with no guide at all still plays.
6. **Nothing in this spec changes the player.** Watching a channel opens the standard player (Spec — Playback Protocol v1: channel sessions for live, VOD sessions for Library Channels).

## 2. The Channels destination

### 2.0 Structure: Channels → sources

**Channels** replaces "Live TV" everywhere. It is shown when at least one channel source exists and hidden otherwise (product rule: hide only disabled features).

Under Channels, in this order:

| Entry | Shown when | Opens |
|---|---|---|
| **All channels** | There are 2 or more sources | Every source's channels in one guide or list. Favorites are cross-source. |
| **One entry per source** | Always, in the owner's order (`position`) | That source's guide or list, named only by the owner ("Antenna", "Sports package"). **The client never shows a source's technical type** (M3U, Xtream, HDHomeRun, tuner): every live source gets the same neutral channels glyph. The **Library Channels** source is the exception: it always gets its own badge and glyph (§2.0a). |
| **Recordings** | At least one live source supports recording (`recordAvailable` on the source), and the viewer may record | The DVR: scheduled recordings, series rules, and recorded programs. Library Channels never offer Record, so a server with only Library Channels has no Recordings entry. |

Where the entries sit, per platform:
- **Web and TV:** in the rail or sidebar, under a "Channels" heading, exactly as libraries sit under theirs. The heading itself opens the first entry (All channels, or the only source).
- **Phone:** one "Channels" tab. A **source picker** sits at the top of the screen: segmented chips when there are 3 or fewer entries (All channels, the sources and Recordings), otherwise a dropdown titled with the current source ("Antenna ▾").
- **Where Channels opens** (Justin, 23 Sep): **always All channels** when there are 2 or more sources, and otherwise the only source. It does not reopen the last source; picking a source is a deliberate step each time.

**Guide and List views.** Every source, including All channels, offers **Guide** and **List** views, and the choice is remembered **per source** (a server preference, `channels.view.<sourceId>`).
- Defaults: Guide on TV, web and iPad; List on phones in portrait (§6).
- Wide layouts show a segmented Guide | List control in the header.
- Phones show it next to the source picker.

### 2.0a Library Channels identity and name

- **Badge and glyph everywhere.** Library Channels (the owner's scheduled channels built from the library) carry a distinctive badge and glyph wherever they appear:
  - the source entry in the rail, sidebar or picker;
  - each of their rows in All channels and in lists (a small badge beside the number);
  - their program cells' leading edge on wide cells;
  - the program sheet header;
  - the player's channel banner.
  - The glyph is a new registry icon `libraryChannel`: a TV with a small star or film frame, to be designed with the icon set. The badge text is the feature name.
- **The name lives in one catalogue ID.** The feature name, "Library Channels" (Justin, 23 Sep), is `channels.custom.name`. Every surface, and the badge, uses it, so a rename is a one-line catalogue change. No screen or core code hard-codes the words.

### 2.1 Channel identity (a row header)

| Element | Rule |
|---|---|
| Logo | Square slot. The source logo (`logoPath`), letterboxed on the raised surface. With no logo, the channel's initials on a tinted tile, the tint derived from the channel id so it's stable. Never a broken image. |
| Number | `number` as published (e.g. `101`, `5.1`). Blank for Library Channels unless the owner numbered them. |
| Name | One line, truncated with an ellipsis. The full name goes in the accessibility label and the tooltip. |
| Favorite | A filled star when the channel is a favorite. |
| Source | In **All channels** only, a small label under the number with the source's owner-given name (e.g. "Antenna"), never its type, so two "101"s from different sources can be told apart. Within a source's own guide there is no label. Library Channels rows show the §2.0a badge instead. |
| Unavailable | When `tuneAvailable` is false, the row is dimmed to 60%, and the reason (`unavailableMessage`, catalogue text) shows in the detail sheet and as the row's accessibility hint. It is still listed, because hiding it would look like data loss. |

### 2.2 Groups, favorites and sorting

- **Filter chips (one row, scrolls horizontally), within the current source:**
  - All;
  - Favorites (shown only when the source has at least one favorite);
  - one chip per channel group of that source, in server order (`group`, e.g. News, Sports, Movies, Kids).
  - In All channels, a chip per source is not needed (the source entries exist), but Favorites is cross-source there.
  - Chips are single-select. Group and Favorites are server-side filters (§8), so a filtered guide is still windowed.
- **Category filter (guide only):** a "Showing: Everything" menu with Movies, Sports, News, Kids and Series. It highlights matching programs and dims the rest to 40%. It never hides rows, because hiding would break the time grid. The data is the program's first category.
- **Sort:** by number (default) or by name, from a menu next to the chips. It is a server-side order, so paging stays consistent. It is remembered per source (a server preference, `channels.sort.<sourceId>`).
- **All channels order:** sources in the owner's order, then each source's channels in its own sort, so channel numbers from different sources don't interleave. Sorting by name interleaves them alphabetically.
- **Favorite toggle:** from the channel's menu (long press or ⋯) and from the program sheet. It uses the existing `saveChannelPreference` (request id and expected revision, never optimistic).
- **Hide channel:** owner and admin only, from the same menu. Hidden channels appear only under a "Show hidden channels" toggle in the Sort menu.

## 3. The guide grid (wide layouts: TV, iPad, web, landscape phone)

```
┌──────────────┬──────────────────────────────────────────────────────────────┐
│ [chips] [sort][Now] [Today ▾]                                                │  controls
├──────────────┼──────────┬──────────┬──────────┬──────────┬──────────┬────────┤
│              │ 7:00 PM  │ 7:30 PM  │ 8:00 PM ┃│ 8:30 PM  │ 9:00 PM  │        │  time ruler (sticky)
├──────────────┼──────────┴──────────┼─────────┃┴──────────┴──────────┼────────┤
│ [logo] 101   │ World Tonight       │ Market W┃ Late Edition         │ …      │
│ Northwind    │ 7:00 – 8:00 PM      │ 8:00 – 8┃                      │        │
├──────────────┼─────────────────────┴─────────┃──────────────────────┴────────┤
│ [logo] 902   │ The Night Shift   │ No information ┃ │ Station Nine …          │
└──────────────┴──────────────────────────────────────────────────────────────┘
                                                ┃ = the now line
```

### 3.1 Geometry

| Measure | Web / iPad | Phone landscape | TV (1920×1080 canvas) |
|---|---|---|---|
| 30-minute column | 240 px | 180 pt | 360 px |
| Channel column | 220 px | 150 pt | 360 px |
| Row height | 72 px | 64 pt | 120 px |
| Ruler height | 40 px | 36 pt | 64 px |
| Cell gap (vertical line between programs) | 2 px | 2 pt | 4 px |
| Visible span | viewport width ÷ px per ms, typically 2–3 h | about 1.5 h | 2 h (5.33 columns) |

- `pxPerMs = column / 1,800,000`.
- A program's `x = (max(start, viewStart) − viewStart) × pxPerMs`, and its `width = (min(end, viewEnd) − max(start, viewStart)) × pxPerMs − gap`.
- These numbers live in `packages/design` (`guideSpec`, added with the screens) so the platforms match.

### 3.2 Time ruler and the now line

- **Ruler labels.** Ticks every 30 minutes, labelled in the viewer's time format (`i18n.time`: "7:30 PM" or "19:30" per the hour-cycle preference).
  - The hour label is emphasized; the half hour is secondary.
  - At a day boundary, the tick label becomes the day ("Tomorrow", "Thu 24") for that tick.
  - On a DST change, the ruler shows the repeated or skipped local hour as it really is (e.g. two "1:30 AM" ticks). Columns stay 30 real minutes wide.
- **Sticky ruler.** It sticks to the top of the grid and scrolls horizontally with the programs.
- **Now line.**
  - A 2 px (TV 4 px) accent line runs from the ruler through all rows at `now`, with a small "Now" tab on the ruler.
  - It moves every 30 s (a timer that only updates the line's x, never re-lays out cells).
  - It is hidden when `now` is outside the visible span. The Now button then shows an arrow: "Now ‹" or "Now ›".

### 3.3 Program cells

**Content, chosen by the cell's width W (px on the platform's own scale):**

| Width | Shows |
|---|---|
| W ≥ 200 (TV 300) | Title (1 line) · time range or "Ends 8:40 PM" · flags (Live, New, Premiere) as small chips · episode line ("S2 E5 · The Arrival") when present |
| 96 ≤ W < 200 (TV 150–300) | Title (up to 2 lines on TV, 1 elsewhere) · flags as a single dot |
| 32 ≤ W < 96 | Title, truncated with an ellipsis |
| W < 32 | No text: a sliver with the cell background only |

**Truncation.**
- Titles end with an ellipsis, never mid-word when a word boundary exists within the last 20% of the space.
- The full title is always in the accessibility label and the detail sheet.

**Clipping at the window edges.**
- A program that started before `viewStart` is drawn from the left edge with a "‹" continuation mark. Its title stays pinned to the left edge while the program is on screen (sticky text), so a 3-hour movie that began an hour ago still shows its name.
- A program that ends after `viewEnd` gets a "›" mark.

**States.**
- **On now** (`start ≤ now < end`): a slightly raised background, and a progress bar (2 px) along the bottom showing elapsed time.
- **Past** (`end ≤ now`): text at secondary contrast.
- **Future:** normal.
- **Focused / hovered:** the platform focus treatment (TV: ring plus 1.02 scale inside the row, no overlap with neighbors; web: ring).
- **Recording scheduled:** a red dot before the title. **Recording now:** a pulsing dot.
- **Series rule:** a stacked-dot glyph.

**Gaps.** Time with no program is a "No information" cell of the same row height, in a dashed-outline style.
- It is focusable on TV so focus never jumps over a hole.
- Its sheet offers only "Watch channel".
- A channel with no guide data at all is one "No information" cell across the whole span.

**Overlaps (bad guide data).** A program that starts before the previous one ends is drawn from the previous one's end. One that ends up empty is dropped. Detail: §9.2.

### 3.4 Navigation controls

- **Now:** scrolls so `now` sits at 25% of the visible span, and focuses (TV) or highlights the on-now program of the focused channel.
- **Day menu:** Today, Tomorrow, then weekday names, up to the guide's `availableEnd`. A day starts at the local midnight; picking one scrolls the view to 6:00 PM that day (prime time), or to now for Today.
- **Horizontal range:** limited to the guide's published range (`availableStart`–`availableEnd`, union over sources). Past programs are browsable back to the retention start (24 h), which the demo fixture exercises with one day back.
- **Paging keys:**
  - Web: PageUp/PageDown move a screen of rows, Shift + scroll moves time, and Home/End jump to the first or last channel.
  - TV: a long press of left/right moves 2 hours; the play/pause button jumps to now.

### 3.5 Program detail sheet

A single program tapped or selected opens a sheet: the side sheet on TV and iPad, and the bottom sheet (half detent) on phones.
- **Header:** the channel logo and name, then the title, episode line, time range with the relative day ("Tomorrow, 8:00–9:00 PM"), rating chip, category chips and flags.
- **Body:** the description (the full text, scrollable).

**Actions, in order, shown only when allowed:**

| Action | Shown when | Does |
|---|---|---|
| **Watch live** | On now and `tuneAvailable` | Starts a channel session. Library Channels: "Watch", joining at the current offset. |
| **Watch from start** | On now, and allowed (§5 for Library Channels; live only with timeshift) | Starts at the program's start. |
| **Record** | Future or on now, live channel, `recordAvailable` | Creates a one-time recording (DVR API). |
| **Record series** | The program has a `seriesId` | Creates a series rule. It opens a small confirm sheet with "New episodes only" (default) or "All episodes", and "This channel" (default) or "Any channel". |
| **Remind me** | Future | **In-app only** (Justin: no push or system notifications of any kind). While Portico is open, a banner or toast appears when the program is about to start ("World Tonight starts in 2 minutes on Northwind News", with Watch). The guide marks reminded programs with a small bell ("Reminders" marker). Nothing is shown while the app is closed. Reminders are stored per profile on the device. |
| **Cancel recording / Stop reminding** | Replaces its action when set | — |
| **Favorite channel** | Always, as a secondary action | — |

- Unavailable actions are absent, not disabled, except "Watch live" on an unavailable channel. It shows disabled with the reason, so the viewer knows why.
- **TV focus:** the sheet opens with focus on the primary action (Watch live, or Record for a future program), and Back closes it back onto the cell.

## 4. TV focus model

- **Vertical movement goes channel by channel; horizontal movement goes through time.** The grid is one focus region. The ruler, controls and chips are separate regions above it.
- **Focus time.** The grid keeps a *focus time*: the start of the focused cell, clamped into the visible span. Up and Down move to the adjacent channel's cell that contains the focus time (`focusTarget`, §9.2), not to the geometrically nearest cell. So moving through 50 channels at 8:00 PM stays at 8:00 PM even across long movies. Left and Right move to the previous or next cell in the row and update the focus time to that cell's start, clamped to the visible start.
- **Scrolling follows focus.**
  - Vertically, the focused row stays in the middle third of the viewport. It scrolls by row, never by page.
  - Horizontally, when the focused cell's start leaves the visible span, the span shifts so the cell starts at 25% of it.
  - A cell longer than the visible span (a 3-hour movie) is focused as a whole, and moving right leaves it for the next cell.
- **The channel column is not focusable.** The row's channel actions (favorite, the channel's own list of upcoming programs) are on Menu or long press over any cell of that row. Up from the first row goes to the chips and the controls. Down from the chips returns to the grid at the remembered row and focus time.
- **No focus traps.**
  - Left from the leftmost visible cell scrolls time back (if the range allows); at the range start it does nothing.
  - Left never goes to the channel column, because the rail opens only with a deliberate long press on the left edge, or Menu from the grid.
  - Right at the range end does nothing.
  - Down at the last channel does nothing.
- **Entry focus:** the on-now cell of the first channel, or of the last focused channel when returning to the destination.
- **Gaps and slivers are focusable**, so focus never skips time. A sliver narrower than 32 px is focused with a minimum-width focus ring overlay that doesn't resize the layout.

## 5. Library Channels specifics

- **What plays.** A Library Channel plays library items on a server-authored schedule. The guide cells are the scheduled items: the title (episode line for episodes), the poster as the cell's leading thumbnail when W ≥ 200, and the runtime.
- **Now / next.** The detail sheet and the List view show "Now: The Night Shift (S2 E5) · 22 min left" and "Next: Station Nine at 8:44 PM".
- **Watch from start** is shown when the channel's policy allows restart:
  - the owner setting `allowRestart` on the channel;
  - the current item is on disk and playable;
  - the server advertises `canStartOver` on the now entry.
  - It starts a VOD session for the current item at 0 with channel context. At the item's end, playback rejoins the channel at the live offset, with the toast "Back to live on Lantern Classics".
- **"Open in library":** a secondary action on a Library Channel program that opens the title page.
- **Not recordable.** Library Channels never offer Record (already enforced: `recordAvailable` is false with `not-recordable`).
- **Owner shortcut:** for owners, "Edit channel" in the channel's menu opens the Library Channel editor (web console).

## 6. Phone layout (portrait)

- **Top of the screen:** the source picker (§2.0: chips for 3 or fewer entries, otherwise a dropdown), then the Guide | List control, then the filter chips.
- **Channel list with now/next** (the default phone view for every source). Windowed list of channel rows:

  ```
  [logo] 101 Northwind News                                   ★
         World Tonight · 7:00–8:00 PM   ▰▰▰▰▰▱▱ 22 min left
         Next: Market Watch at 8:00 PM
  ```

  - Tap opens the channel's timeline. Long press opens the channel menu.
  - A "Watch" icon button on the row starts the channel directly.
  - Rows for channels without a guide show "No guide information", and Watch still works.
- **Per-channel timeline.** A vertical list of the channel's programs, from 1 h ago through the guide's end, grouped by day with sticky day headers.
  - It opens scrolled to the on-now program, which is highlighted with its progress bar.
  - Tapping a program opens the detail sheet.
  - It is windowed by time (the same store, one channel).
- **Landscape phone and iPad:** the compact grid (§3.1, phone-landscape column). At ≥ 700 pt the grid is the default view, unless the viewer chose List for this source. The Guide | List control remains.
- **iPad:** follows the web and TV structure: sources are listed in the sidebar under Channels, not in a picker.
- **Recordings on phone:** reached from the source picker as its own entry, when shown.

## 7. Web layout

- **Sidebar:** the Channels heading, then All channels (when there are 2 or more sources), one entry per source, and Recordings (when shown), under the libraries. Each entry is a route: `/channels/all`, `/channels/<sourceId>` and `/channels/recordings`, with `?view=guide|list` reflecting the remembered view.
- **Grid by default** for Guide (≥ 900 px wide). Below that, and for List, it uses the phone list layout.
- **Keyboard:**
  - arrow keys move focus as on TV;
  - Enter opens the sheet;
  - `W` watches, `R` records, `N` jumps to now.
  - Focus uses the web focus ring. The grid is an ARIA `grid` with `row`/`gridcell` roles. Each cell's name is "8:00 to 9:00 PM, World Tonight, Northwind News, New".
- **Scrolling:** the mouse wheel scrolls channels. Shift + wheel, or a horizontal trackpad gesture, scrolls time. Buttons on the ruler edges ("‹ 2 h", "2 h ›") are for mouse users.
- **The detail sheet** is a side sheet (the existing dialog spec) with the same actions as §3.5.

## 8. Data, windows and performance (O(visible))

### 8.1 Server contract

The current `GET /v1/guide` returns at most 30 channels per cursor page, each with its programs for one time window, one kind per request. The target splits this into two windowed reads. PERF-13 asks for one query per window.

**Sources.** `GET /v1/channel-sources` returns the Channels entries for this viewer:

```
{items: [{id, name, type: 'm3u'|'xtream'|'hdhomerun'|'tuner'|'library', position, channelCount,
          recordAvailable, guideState: 'ready'|'refreshing'|'stale'|'none', guideUpdatedAt?, availableStart?, availableEnd?}],
 revision}
```

- Only sources the viewer may see are listed.
- `recordAvailable` on a source decides whether Recordings is shown (§2.0). It is always false for `library`.
- `type` is for the client's logic only (the Library Channels badge, recording rules). It is never rendered: clients show the owner's `name` and a neutral glyph (§2.0).

**Channels.** `GET /v1/channels?sourceId=<id>|all&group=&favorites=&sort=number|name&includeHidden=&from=<offset>&limit=<≤100>` returns:

```
{items: [{id, sourceId, kind: 'live'|'library', number, name, group, logoUrl, favorite, hidden, tuneAvailable, tuneUnavailableReason,
          recordAvailable, recordUnavailableReason, preferenceRevision, guide: 'full'|'partial'|'none'}],
 page: {total, start, revision}}
```

- `sourceId=all` is All channels, ordered as in §2.2. The `favorites` filter is cross-source there.

- It uses offset windows, because the guide scrolls to arbitrary rows; cursor paging can't jump.
- The `revision` changes when the channel set or order changes.

**Programs.** `GET /v1/guide/programs?channelIds=<≤100 ids>&start=<ms ISO>&end=<ms ISO>` returns:

```
{items: {<channelId>: [{id, start, end, title, subtitle?, episode?: {season?, number?, display?},
          categories: string[], rating?: {system, value}, year?, starRating?,
          flags: {live, new, premiere, repeat, finale?}, seriesId?, recording?: {id, state}, image?}]},
 availableStart, availableEnd, generation}
```

**Program fields the sheet and the cells need.** Today's guide DTO has none of these, so they are required in the target contract, taken from XMLTV:

| Field | From XMLTV | Shown |
|---|---|---|
| `subtitle` | `<sub-title>` | Episode line: "S2 E5 · The Arrival" |
| `episode.season`, `episode.number` | `<episode-num system="xmltv_ns">` (0-based, converted to 1-based) | Episode line |
| `episode.display` | `<episode-num system="onscreen">` | When there is no season or number (e.g. "Part 2") |
| `categories` | `<category>`, in order | Category chips, and the category filter (first category) |
| `rating` | `<rating system><value>` | Rating chip (e.g. TV-14). It is also used by profile restrictions (the server filters restricted programs as it does library items). |
| `year` | `<date>`, for movies | Facts line |
| `starRating` | `<star-rating>` | Not shown by default (a detail-sheet line, "7/10"). |
| `flags.live` | `<live/>` | "Live" chip |
| `flags.new` | `<new/>`, or `<previously-shown>` absent with provider evidence | "New" chip; drives "New episodes only" series rules |
| `flags.premiere` | `<premiere>` | "Premiere" chip |
| `flags.repeat` | `<previously-shown>` | Nothing (the absence of New) |
| `image` | `<icon src>` | Leading thumbnail on wide cells and in the sheet (proxied as an artwork URL, never the provider's URL) |

Library Channel programs fill these from the library item: episode numbers, rating, genres as categories, year, and the poster as `image`.

- The window is at most 12 hours.
- Programs overlapping the window are included whole (their true start and end).
- Descriptions are included only when `detail=full` (the sheet asks for one program: `GET /v1/guide/programs/{id}`).

**Library Channel now.** `GET /v1/channels/{id}/now` returns `{entry: {itemId, startsAt, endsAt, offsetMs, canStartOver}, next: [...]}`.

**Events.** `live.guide` on `/v1/events` with `{generation}` invalidates cached program tiles. Clients never poll the guide on a timer.

**Stopgap adapter (decided: build the screens against it now).** Today's `/v1/guide` cursors are signed per time window and kind, and it reports no total. So an adapter over it walks every cursor page for each 3-hour block. That is fine for 100 channels or fewer (the demo has 14), and it's what the first screens use.
- `packages/client-core/src/guide/legacy-source.ts`: `legacyGuideSource(api, serverId, {kind, sourceId, timezone})`.
- It maps today's `GuideChannel`/`GuideProgramme` to the §9 types. The fields today's DTO lacks (episode, rating, categories, flags) stay empty until the server provides them.
- Sources come from today's source list (`GuideSource`), plus one "Library Channels" entry when the library-channel guide has channels.
- Above 100 channels it still works but is slow, and it logs a development warning.

§8.1 is the target.

### 8.2 Client windows

- **Tiles.** The client divides the guide into tiles: **channel pages** (50 channels) × **time blocks** (3 hours, aligned to UTC multiples of 3 h). A tile is one programs request.
- **Resident set.** The visible rows and span, plus a prefetch margin: one channel page above and below, and one block before and after. At most 24 tiles and 6 channel pages are resident. Older ones are evicted least-recently-used.
- **In-flight limit.** At most 4 requests at a time. Requests for tiles that left the interest are aborted.
- **Rendering.** It mounts only the visible rows (plus 3 above and below) and, within each row, only the cells intersecting the visible span. A cell that crosses a tile boundary appears once: it is deduplicated by program id.
- **Budgets,** checked in tests (§9.4) for 1,000 channels × 14 days:
  - at most 24 tiles and 6 channel pages resident, whatever the scroll history (under 10,000 programs at realistic densities);
  - a full-screen scroll from now to +13 days issues requests only for the tiles it passes through (and aborts them when passed);
  - layout of a visible 12 × 3 h window takes under 2 ms in node.

### 8.3 Watching a recording while it records (after B8)

> Until the server supports it, clients offer "Watch live" on the channel being recorded, and "Watch from start" only after the recording completes. Decided 23 Sep, matching Plex, which offers "watch from start" during a recording.

**Today.** Capture writes one MPEG-TS through a `mediaartifact` writer and checkpoints a closed prefix about once a second (`recordingmedia/driver.go`). The catalogue item and asset appear only at publication, after capture ends (`dvr/publication.go`). Playback serves closed assets only.

**DVR half.**
- A recording-scoped playable source, `recording:<id>`, that reads the latest checkpointed prefix of the capture object. It never reads past the last checkpoint, and the object never leaves the artifact store.
- Its authority is the recording's owner (the owner key), plus the profile's Recordings switch (P8) and content restrictions on the programme's rating. This is the same fence as a published recording. The source is refused once the recording is cancelled, failed or deleted.
- `GET /v1/dvr/recordings/{id}` reports `watchFromStart: {available, reason}` while the state is `recording` or `finalizing`.
- At publication, the in-progress playback moves to the published item without restarting. The session keeps its position, and the source identity maps to the new asset.

**Delivery half.**
- A growing-source delivery mode: an HLS **EVENT** playlist cut from the growing TS, which appends segments as checkpoints advance and seeks within the captured span.
- At publication it ends the playlist (`#EXT-X-ENDLIST`), or continues on the published VOD asset.
- Live latency is not a goal. The playlist trails the last checkpoint by about two segments.

**Client.** `playableRecording` also accepts `recording` and `finalizing` when `watchFromStart.available` is true. "Watch from start" opens at position 0; "Watch live" still tunes the channel.

## 9. Shared model (client-core `src/guide/`, built now)

### 9.1 `time.ts`
- `SLOT_MS` (30 min) and `BLOCK_MS` (3 h).
- `floorTo(ms, step)`, `ceilTo`.
- `blocksCovering(start, end)` returns block starts.
- `rulerTicks(viewStart, viewEnd, {slotMs, pxPerMs, zone})` returns `{atMs, x, kind: 'hour'|'half'|'day'}`. Labels come from the platform's `i18n.time`/`i18n.date`, so the model stays language-free. `kind: 'day'` marks the first tick of a local day.
- `localDayStart(ms, timeZone)` finds the local midnight on or before `ms` with `Intl`, and handles DST days of 23 or 25 h.
- `dayStarts(fromMs, toMs, timeZone)` lists the days for the day menu.
- `nowX(now, viewStart, pxPerMs)` returns `undefined` when outside the span.

### 9.2 `layout.ts`
- `layoutRow(programs, {viewStart, viewEnd, pxPerMs, gapPx, now, rangeStart?, rangeEnd?})` returns cells: `{kind: 'program'|'gap', start, end, x, width, clippedStart, clippedEnd, tier: 'full'|'title2'|'title'|'sliver', state: 'past'|'now'|'future', program?}`.
  - Programs are sorted, overlaps are trimmed, empties dropped, and gaps inside the published range become gap cells.
  - `tier` follows §3.3 (thresholds are passed per platform).
- `focusTarget(cells, focusTimeMs)` returns the cell containing the time. If none contains it, it returns the nearest cell by start.
- `neighbor(cells, index, direction)` handles Left/Right.

### 9.3 `now-next.ts`
- `nowNext(programs, now)` returns `{now?, next?, progress, remainingMs, gap: boolean}`.
- `nextChange(programs, now)` returns when the "now" changes, so a list re-renders once per program boundary, not on a timer.

### 9.4 `store.ts`: `GuideWindowStore`
- **Data source:** `{channels(from, limit, signal) → {items, total, revision}; programs(channelIds, start, end, signal) → Record<channelId, Program[]>}`.
- **Viewport:** `setViewport({firstRow, lastRow, start, end})` computes the interest (rows and blocks plus the margin), fetches the missing channel pages and tiles (at most 4 in flight), and aborts or evicts outside it.
- **Reads:**
  - `channelAt(row)` returns a channel or a placeholder;
  - `programsFor(row, start, end)` returns the merged, deduplicated, sorted programs from resident tiles, plus `complete`;
  - `total` is the channel count;
  - `version` changes on every change (for `useSyncExternalStore`), with per-row subscriptions like the windowed collection.
- **`invalidate({programs?, channels?})`:** on a `live.guide` event or a filter or sort change, a new generation starts. Old tiles stay on screen until their replacements arrive.
- **Tests** (`test/guide-*.test.ts`):
  - a deterministic synthetic source at 1,000 channels × 14 days;
  - the demo fixture shapes (14 channels, 9 days, a no-guide channel, a gaps channel, a long title, DST days);
  - bounds on resident tiles and in-flight requests;
  - deduplication across tile boundaries;
  - focus movement across a long movie;
  - now/next across gaps.

## 10. Empty, loading and error states

| Situation | Shows |
|---|---|
| No sources and no Library Channels | The destination is hidden. For owners, a "Set up Channels" card in Settings → Server (existing, renamed). |
| One source in All channels mode | There is no All channels entry with one source; Channels opens the source directly. |
| A source has no channels | "No channels in Antenna yet". Owners: "Channels appear after the next guide refresh", with "Refresh now". Others: "Ask the server owner to finish setting up this source." |
| Filter matches nothing | "No channels in Sports", with "Show all channels". |
| First load | Skeleton rows (the channel column and grey bars), never a spinner over an empty grid. |
| A tile fails | Its cells show a quiet "Couldn't load this part of the guide · Try again" band across the failed rows and hours. The rest of the guide stays usable. Retries back off (5 s, 15 s, 45 s). |
| Channel list fails | A full-screen error with Try again, using the shared error presenter (X-04 wording). |
| Guide stale (`refreshState` degraded) | A notice above the grid: "The guide was last updated Tuesday at 3:10 AM." For owners, it links to the source. |
| No guide data for a channel | "No information" across the row. Watch still works. |
| Channel unavailable | The row is dimmed, and the reason shows on focus/hover and in the sheet (catalogue text for `tuneUnavailableReason`). |
| Program in the past | The sheet offers "Watch channel" and, if it was recorded, "Play recording". |

All copy goes into the catalogue under `guide.*` and `channels.*` (US English, CON-17 retry wording).

## 11. Accessibility

- Every cell's accessibility name gives time, title, channel, and flags. The row header gives number and name.
- The now line is decorative, and the on-now cell says "On now, 22 minutes left".
- With Reduce Motion, jumps to now and scroll-following on TV animate instantly.
- Contrast: text on cell backgrounds meets 4.5:1 in every state, including past (secondary) text on the on-now background.

## 12. Decisions

1. **Stopgap adapter:** yes, for 100 channels or fewer, so the screens can be built against the demo now. §8.1 is the target.
2. **Channels structure:** approved by Justin (23 Sep) with three changes:
   - Channels always opens on All channels when there are 2 or more sources;
   - source types (M3U, Xtream, HDHomeRun) are never shown, only owner names and a neutral glyph;
   - Library Channels get their own badge and glyph everywhere, and the feature name sits in one catalogue ID (§2.0, §2.0a).
3. **Remind me:** in-app only, by policy (Justin, 23 Sep: no push or system notifications of any kind, on any platform). No `UNUserNotificationCenter`, and no web Notifications API.
   - While the app is open: a banner or toast at start time, and a bell marker on reminded programs in the guide and the program sheet.
   - Reminders are stored per profile on the device, keyed by program id, and removed after they fire, or when the program leaves the guide.
4. **Watch from start on live channels:** hidden entirely until the server advertises timeshift. The client shows it only when the channel sets `canStartOver`.
   - Library Channels get it through `/v1/channels/{id}/now` (§5).
   - No disabled "coming soon" button.
5. **Category filter:** it dims non-matching programs to 40% and never hides rows (§2.2). The grid stays a stable map of channels; hiding rows by what's on right now would reshuffle it every half hour.
   - List view is different: there the filter hides channels whose on-now program doesn't match, because a list has no time axis to keep stable.
