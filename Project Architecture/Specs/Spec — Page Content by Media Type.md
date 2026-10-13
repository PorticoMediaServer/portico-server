# Page content by media type (normative)

October scope and evidence: The server continues to decide screen contents. Home rows publish typed See all destinations; clients do not guess from IDs. Every subgroup must offer the main sort/filter vocabulary. Genres/Series paging is implemented; wider grouping parity and client acceptance remain unfinished. Ratings are integer 0–10, displayed as half-stars with unset distinct from zero. Sources: [personal-state](../../../_program/reports/personal-state.md), [search-sort](../../../_program/reports/search-sort.md), [grouping-sorts](../../../_program/reports/grouping-sorts.md). Existing route examples must be checked against generated contracts before use.

This spec says **what every title page shows, where the data comes from, and what happens when data is missing**. It extends `Spec — Title Pages.md` (the shared hero, the show page and the episode view), which stays valid. **The standard is Plex:** a page for anything in the library should feel complete.

## 0. Rules for every page
1. **Nothing renders empty.** A section without data is omitted, heading included. A hero without a backdrop becomes the **compact hero** (no tall empty band): poster or art beside the title, and a background derived from the art (a blurred, darkened poster or cover) instead of blank space. Never show a large empty area waiting for art.
2. **Never repeat one image as if it were many.** When episode stills are missing, show a styled placeholder (the show's colour and the episode number), never the show backdrop four times. Until the server publishes a dominant colour, the show's colour is a stable tint derived from its id (`titleTintHue` in client-core), the same on every client.
3. **Order of sections:** hero → primary content (episodes, tracks, parts) → people → related → extras → details. Each platform uses its existing components (web rows and grids; Apple shelves and TV focus rows).
4. **Everything scales:** every list is windowed and paged (5–10M-item libraries). Counts and "More like this" are bounded server reads, never library scans.
5. **One metadata agent per library, as in Plex (Justin, 24 Sep):** each library chooses **one agent** for its type (e.g. "Portico Movies", "Portico TV", "Portico Music", "Portico Audiobooks") **or "Local metadata only"**. An agent may combine several upstream sources internally, and the user never picks or configures individual sources: no per-source toggles, no API keys. **Local media assets** (`poster.jpg`, `folder.jpg`, `cover.jpg`, `fanart.jpg`/`backdrop.jpg`, `artist.jpg`, `season01.jpg`, embedded art) are honoured by **every** agent, including Local metadata only, and win over online art unless the owner picks otherwise in the editor. Agents run **on the user's server** (never through Hosted), cache, and refresh on the existing freshness schedule.

## 1. Movie (the reference; mostly done)
Hero: backdrop, poster, title, year · rating · runtime · genres, community rating, logo if present. Actions per `detailActions`. Synopsis. **Cast & crew** (photos; key crew). **More like this** (library-local recommendations). **Extras** (trailers and featurettes in the library). **Collections** it belongs to. **Details** (release date, studio, country, original title, edition, genres, ratings, date added) ending with **Files** (per file: what it is, size, bitrate, every audio and subtitle track, and the path for owners). There is no separate Media information dialog.

## 2. TV show (and anime; the same page)
- **Hero:** backdrop, poster, title, years (e.g. "2008–2013"), N seasons, network, content rating, genres, community rating, logo if present. The primary action is **Resume SxEy / Play next episode / Play S1E1**, followed by the same row as a movie: My List · Favorite · Watched · ⋯ (the ⋯ holds Download all, Edit metadata and the rest).
- **Season switcher:** tabs (web, iPad, TV) or a menu (iPhone) listing **every season, including Specials** (last). Selecting one never reloads the hero (per `Spec — Title Pages.md` §2).
- **Episodes of the selected season:**
  - each episode has its **still** (TVDB or TMDB), number, title, runtime, air date, two-line synopsis, and watched or progress state;
  - "Play season" and "Mark season watched" (as now);
  - a missing still uses the placeholder rule (§0.2).
- **Cast & crew:** the show's main cast (character names), then creators, showrunners and key writers (max 8 plus "All"). Photos are required where the provider has them.
- **More like this:** library-local shows ranked with the same engine and bounds as movies (genres, people, keywords, ratings).
- **Extras:** trailers and featurettes in the library. **Details:** network, studio, first aired, status (Continuing/Ended), original title.
- **Episode view** (panel or sheet): still, SxEy · title, air date, runtime, synopsis, **guest stars**, director and writer, Media information, and actions.

## 3. Music: artist
- **Hero:** the **artist image (circle)** over the **artist background** (fanart). If there's no background, use the compact hero with a blurred version of the artist image. If there's no artist image either, use a coloured initial circle, and the compact hero.
- **Facts:** genres (first 3), country, years active, N albums · N songs. **Biography:** 3 lines with "More", attributed ("From Wikipedia").
- **Actions:** Play, Shuffle, Favorite (heart), ⋯ (artist radio when available).
- **Sections:**
  - **Popular tracks** (top 5 by the viewer's plays, M14);
  - **Albums** (by release year, newest first), then **Singles & EPs**, then **Compilations**, then **Appearances**, all by release-group type;
  - **Similar artists in your library** (shared genres and collaborations);
  - **Music videos** if the library has them.

## 4. Music: album
- **Hero:** cover (square) and a background derived from the cover (blurred), so there's never an empty band.
- **Facts:** artist (link) · year · type (Album/EP/Single/Compilation) · N songs · total length · label · genres. A **review or description** when the provider has one (attributed).
- **Actions:** Play, Shuffle, Favorite (heart), Download, ⋯. No "watched" or "played" marks on music.
- **Track list:** disc headers only for multi-disc albums, then number, title, the featured artist when it differs, duration, and ⋯. Popularity marks are optional.
- **Sections:** **More by <artist>** (the artist's other albums), then **Similar albums in your library**.

## 5. Audiobook (book) and author
- **Book hero:** cover, title, author(s) (links), narrator(s), series name and number (link), duration, publisher, year. Description.
- **Primary action:** Resume chapter N / Play, then Favorite, Finished, Download, ⋯. Wording is "Finished" / "Not started", never "watched". **Chapters or parts list.**
- **Sections:** **More in this series**, **More by <author>**.
- **Author page:** image and biography (Wikipedia) when available, then books grouped by series.

## 6. Person (cast and crew)
Photo, name, known for, biography, born or died, and a filmography of titles **in the library** (windowed), grouped by movies and shows, with role chips (exists; keep).

## 7. Collection and playlist
Hero with a mosaic of the first 4 posters or covers when there's no custom art. Facts: N items and total length. Items windowed. For a playlist, reorder and remove (exists).

## 8. Data sources, per agent (decisions)
Every row below is **internal to the library's chosen agent**. With "Local metadata only", only local tags and local assets apply.

| Need | Source | Notes |
|---|---|---|
| Movie facts, cast, crew, recommendations, extras | TMDB (existing "screen" provider) | Already published |
| Show and episode facts, ordering, stills, season posters | TVDB (existing) | Add episode stills, season posters and characters where TVDB has them |
| Show cast and crew (characters, photos), guest stars | TVDB `characters` first; TMDB `aggregate_credits` via the TVDB↔TMDB cross-id as a fallback | Persons are shared with movies (one people table) |
| Show and movie "More like this" | **Library-local** recommendation engine (bounded) | No online recommendation calls |
| Album cover | Cover Art Archive (existing), then embedded or folder art | |
| Album facts: type, label, year, genres | MusicBrainz release group, release and tags (existing provider) | |
| Artist image, biography, country and years | Local assets (`artist.jpg`, `fanart.jpg`) first; then, **inside the Portico Music agent**: Wikidata → Wikipedia and Wikimedia Commons, via the MusicBrainz artist's Wikidata relation | Not a separate provider and no key. Wikipedia text is CC BY-SA, so show "From Wikipedia" with a link. The Commons image licence is recorded |
| Artist background | Local `fanart.jpg`/`backdrop.jpg`, otherwise the blurred artist image | **No Fanart.tv** (no extra sources or keys, per Justin) |
| Similar artists or albums | Library-local: shared tags and genres, MusicBrainz collaborations | No online calls |
| Book facts, series, author biography | Local tags and assets; inside the **Portico Audiobooks** agent: the existing audiobook metadata, then Open Library, then Wikipedia for the author biography and image | Internal to the agent, not a user-visible provider |

## 9. Delivery
1. **Gap analysis first** (read-only): for each page and section above, record whether the server already has the data, whether the projection exposes it, and whether each client renders it. Many gaps may be client-only; for example, `show_workspace_credits.go` exists on the server.
2. **Server data work:** the provider additions in §8, and projection fields, all behind the library's provider setting, bounded and cached.
3. **Client layouts** (web and Apple): the section order, the compact-hero and placeholder rules, and each page above.
4. **Demo fixtures:** at least two **multi-season shows** (one with Specials), one **anime**, an **artist with albums, EPs, singles and an appearance**, a **multi-disc album**, and an **audiobook series** with two books by one author. Synthetic media named so the online providers match.
EOF
