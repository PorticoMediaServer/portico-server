# Title pages: shows, seasons, episodes and the shared hero (normative)

Applies to web, iPhone/iPad and TV. Anime uses exactly the same structure as TV shows. The **core player layout is untouched**; this is the detail and browse structure only.

## 1. The shared hero (every media type)

Every title page opens with the same fixture, `TitleHero`, filled differently per type. One component per platform, one set of slots:

| Slot | Content |
|---|---|
| Backdrop | Full-bleed backdrop (w=1920 on TV/desktop, 800 on phones), a bottom and left scrim for legibility, a blurhash colour while loading. No backdrop → a tinted gradient from the poster's dominant colour, never an empty box. |
| Identity | Logo art if present, else the title in display type. |
| Facts line | One line, `·`-separated, type-specific (below). Rating badge (content rating) as a chip. |
| Tagline | The title's tagline on a line of its own above the synopsis, when it has one. Never run into the synopsis. |
| Synopsis | Two lines, "More" expands inline (web/phone) or opens a text sheet (TV). |
| Credit lines | Under the synopsis: "Directed by …", "Written by …" (or "Created by …" for a show), "Starring …" — up to three or four names each, one entry per person. |
| Pick before play | Under the credit lines: up to three compact controls, **Version**, **Audio**, **Subtitles**, preselected so the usual viewer never touches them. A control appears only when there is a real choice (two versions, two audio tracks, any subtitle track); with no choice anywhere the row is absent. Subtitles reads "Automatic" until changed (the viewer's subtitle preference decides), then Off or a language. The choice travels with Play and is what plays. |
| Primary action | Context-aware (below), with a thin progress bar under the label when resuming. |
| Secondary actions | **One row on every title (2 Oct 2026):** Play/Resume · My List · Favorite (heart) · Watched · ⋯. Shows carry the same My List and Favorite as movies (the title's own flags, kept on the container personal-state route). Music has no "watched": an album or artist row is Play · Shuffle · Favorite · ⋯. An audiobook says "Finished". The ⋯ menu holds the rest (Play from start, Rate, Add to playlist, Add to collection, Download, Edit metadata, Refresh metadata) and never repeats what is on the row. **Like/Dislike does not exist**: the heart, star ratings and Not interested cover it. |
| Artwork | Poster (2:3) beside the identity on wide layouts only when there is no logo art; square for music; circle for artists and people. |

Per type:

| Type | Facts line | Primary action |
|---|---|---|
| Movie | Year · runtime · genres · rating | Play / Resume (12 min left) |
| Show / Anime | Years (2008–2013) · N seasons (or N episodes when seasons are hidden) · genres · network · rating | Resume S2 E4 / Play S2 E5 / Play S1 E1 (next-up rule, same as Continue Watching) |
| Album | Artist · year · N tracks · length | Play / Shuffle |
| Artist | Genres · N albums | Play (top tracks) / Shuffle |
| Audiobook | Author · narrator · length | Resume chapter 7 / Play |
| Person | Known for · born | — (filmography below) |
| Collection / Playlist | N items · total length | Play / Shuffle |

Below the hero, each type has its own sections. The hero never changes height between types on the same platform (TV: 60% of the screen; desktop: min(70vh, 720px); phone: 16:9 backdrop plus the identity block).

## 2. The show page (TV shows and anime)

Sections, in order:

1. **Hero** (above).
2. **Episodes**
   - **Season switcher:** tabs for up to 8 seasons, a dropdown beyond that, Specials last. It defaults to the season containing the next-up episode (else season 1). Its "⋯" menu holds Play season, Mark season watched/unwatched and Season info (season poster, overview, air dates).
   - **Season progress** ("6 of 10 watched") sits beside the switcher.
   - **Next up** is pinned above the season's list when it belongs to that season and is not already its first row.
   - **Episodes:** each has its still (a placeholder in the show's colour with the episode code when there is none — never the show backdrop), number and title, air date · runtime, a two-line synopsis, a progress bar and a watched check.
     - **Web: a list by default** (still at about 200 px, then the copy; Play and ⋯ on hover), with a grid of 2–4 columns as a device preference. The still plays; the copy opens the episode panel. A row takes focus: Enter plays, Up and Down move through the list.
     - Phone: a vertical list (still left, text right, two-line synopsis). TV: a focusable list with the focused episode's still and synopsis shown large beside it.
     - The list is **windowed** with `createWindowedCollection` (never load a whole season up front).
3. **Up next** is the hero's primary action and the pinned row above the list (above). No separate shelf.
4. **Cast & crew**
   - **Show cast:** a row of circular portraits, character names under them, series regulars first.
   - When a season is selected and the server has **season credits**, the row title becomes "Cast · Season 2" and shows that season's regulars. A toggle chip "Whole series" switches back.
   - The **episode** view (below) lists guest stars, director and writers.
   - Credits come from the server per scope (`show`, `season`, `episode`); clients never merge or dedupe them.
5. **Extras:** trailers, featurettes and behind the scenes (landscape).
6. **More like this:** recommendations (posters), from the recommendation service, fresh every load.
7. **Details:** network/studio, country, original title, genres. A movie's and an episode's Details end with **Files**: per file, what it is ("4K · HEVC · Dolby Vision · MKV"), resolution, size, bitrate, duration, frame rate, every audio track (language, codec, layout) and subtitle track, and the path for owners.

The season is **state, not a page**. Web keeps it in the URL (`/show/<id>?season=2`), so Back and links work. Changing season never reloads the hero.

## 3. The episode view

A lightweight layer over the show page, never a separate destination with its own hero:

| Platform | Open | Content |
|---|---|---|
| TV | Select on a card **plays/resumes**. Long-press or the "⋯" button opens the episode panel (a right-side sheet). | Still, S2 E4 · title, air date · runtime, synopsis, guest stars, director and writers, audio/subtitle versions, Play/Resume, Play from start, Mark watched, previous and next episode buttons. |
| iPhone/iPad | Tapping the **still** plays; tapping the **text** opens a half-height sheet (detent to full). | Same. |
| Web | Clicking the still plays (hover shows a play button); clicking the copy opens a right-side panel with a deep link `/show/<id>/episode/<episodeId>` (a direct visit renders the show page with the panel open; a link without `?library=` resolves it). | Same, plus the pick-before-play row, ⋯ (download, playlist) and the episode's Files. |

## 4. Long-running, daily and anime shows (per-show settings; Plex parity and beyond)

Per-show settings, set by owners/admins from the show's Edit dialog ("Advanced"), stored on the server and applied in the server's projections (clients just render what the server declares):

> **Built 2 Oct 2026 (frontend rework pass 7):** four switches, edited from the season switcher's ⋯ menu ("Show settings"): **Hide seasons**, **Number episodes across seasons** ("Episode 1043"), **Split into ranges of 100**, **Newest first**. Stored in `catalog_show_settings`; the show workspace returns `settings` and, for a flat show, groups `all` / `range:<start>` / `specials` in place of seasons. Library-level defaults, DVD ordering, years grouping, Keep and Delete after watching in the table below are not built.

| Setting | Values | Effect |
|---|---|---|
| Seasons | Library default / Show / **Hide** | Hide: the switcher disappears and episodes are one flat list across seasons (grouped headers optional). |
| Episode sorting | Library default / Oldest first / **Newest first** | Newest first is the default for daily shows (news, talk shows). |
| Episode ordering | Library default / Aired / DVD / **Absolute** | Absolute (anime) numbers episodes 1…N across seasons and labels them "Episode 1043". |
| Episode grouping (long shows) | Automatic / Seasons / Ranges / Years | Automatic: seasons if there are ≤ 30 seasons, otherwise ranges of 100 (1–100, 101–200…) for absolute ordering, or years for daily shows (date-based episodes). |
| Keep | All / latest 5/10/30 / last 7/30 days | Server retention for unwatched episodes (daily shows). |
| Delete after watching | Never / after a day / after a week | Server deletes watched episodes (owner's history) after the delay. |

- **Daily shows** (date-based episodes, model) show the air date as the episode's primary label ("Mon, Sep 22 · Guest: Colin Kaepernick"), not "Episode 101".
- **"Jump to":** an episode-number field (absolute/aired) or a date picker (daily). Uses the collection's `positionIndex`/`start` random access.
- Library-level defaults for all of the above live in Library settings (anime libraries default to Absolute ordering + Ranges; a "news/talk" genre heuristic can suggest Newest first).

## 5. Server data this needs

- Show projection: hero fields (logo, backdrop, blurhash, facts, next-up episode with progress), season list with counts and watched counts, per-show settings, extras, credits summary.
- Episodes: a windowed page by season / range / year / flat, honouring the sort, ordering and grouping settings, with `positionIndex` for ranges and letter/number jumps. Card fields: still (landscape `w` variants), number labels per ordering, air date, runtime, progress, watched.
- Credits scoped `show | season | episode`: regulars, guest stars, director, writers, with portraits (circle crop is client-side from square art).
- Recommendations for a title ("More like this"), fresh per request, restriction-aware.
- Per-show and per-library settings with retention (Keep / Delete after watching) run as background Operations.
- All projections restriction-aware (the `catalog.Viewer` scope).

## 6. Delivery

- The web and Apple apps build `TitleHero` and the show page against today's endpoints where fields exist, using placeholders and fallbacks where they don't (no dead UI). They switch to the `/v1` contract when it lands.
- The per-show settings UI (Edit → Advanced) is built once the settings exist server-side.
