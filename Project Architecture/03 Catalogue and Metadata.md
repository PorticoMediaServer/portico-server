# Catalogue and metadata

The catalogue is everything the server knows about the media in its libraries: films, shows, seasons, episodes, artists, albums, songs, books, audiobook files, collections, people and their artwork. Two packages own it:

- `internal/compactcatalog`: the **write side**. It holds the facts and the derived data kept from them.
- `internal/catalog`: the **read side**. It covers Home, Discover, browse, detail pages, search and recommendations, plus the per-request snapshot and memo rules.

## Entities and facts

Every catalogue object is a row in `catalog_entities` (an integer id, a kind, a library, a public id), with narrow side tables for the facts that kind has. Kinds are data (`catalog_kinds`) with flags (`playable`, `container`, `searchable`, `browsable`, `listening`), and code asks the flags, not kind lists. A new kind is an insert plus, if needed, a new side table ([persistence rules](../../apps/portico-server/server/internal/persistence/README.md)).

Shared facts use compact vocabularies: genres, tags, studios, networks, countries and credit roles are interned once (`catalog_terms`, credit labels) and referenced by number. External identities (TMDB, TVDB, AniList, IMDb, MusicBrainz) live in `catalog_external_ids`.

Facts are written synchronously, in the writer's own transaction, through the write API (`compactcatalog/write.go`). Scanning, metadata refresh, manual edits and the metadata editor all go through it.

## Derived data

Everything a read path needs that isn't a fact is **derived**: search documents, browse rows and their sort keys, availability, counts, related titles, categories, inherited attributes, facet counts, and recommendation features. Derivations are maintained, not cached:

- The write API and small triggers on the fact tables queue work on `catalog_dirty(domain, entity_id, revision)`.
- One supervised worker drains the queue in bounded batches, one domain at a time, with short background transactions.
- Each derivation has a **domain number** and a **version** (`catalog_derivations`). Raising the version rebuilds that domain in the background after startup. Readers of a rebuilding domain answer `ErrBuilding` until it catches up, and clients show a building state.
- Any derived table can be emptied and rebuilt from facts.

| Domain | Derivation |
|---|---|
| 17 | Search documents |
| 18, 19 | Browse rows (sort keys, filter columns) and browse edges (membership) |
| 20, 28 | Availability of items and of libraries |
| 21 | Related titles |
| 25, 26, 29 | Item and asset metrics, asset issues |
| 27 | Categories |
| 30 | Book context (series, position) |
| 31 | Attributes a container inherits from its members |
| 32 | Recommendation features ([Recommendations](05%20Recommendations.md)) |
| 33 | Facet counts (filter option counts) |

A new title appears in search, browse, Recently Added and recommendations in the same drain. A removed title leaves at once through cascades, and availability is also checked on read.

**Cost:** cataloguing is linear in the titles changed (about 6 ms per entity at 10k films on a Mac, 13.5 ms on the runner at 100k). Bulk removal is proportional to what was removed and never rewrites unrelated rows.

## Read paths

`internal/catalog` builds every document a client reads:

- **Home**: an ordered, profile-arrangeable list of rows (Continue Watching, Recently added, Favorites, Recommended, the personal rows, Trending, saved views, …). Each row is a bounded page with its own cursor. A row may be off by default (the per-library Recently added rows) until the profile's layout places it. The Home document also names its **hero** (`hero: {rowId, entryId}`): the first entry of Continue Watching or of Continue Listening, whichever was played more recently, so the hero is an unfinished title or the next episode of the show last watched. There is no hero when neither row is on the viewer's Home. `client-core`'s `homeHero` writes every word on it (the show as the title and "S2 E3 · Episode" under it, the artist or author for something listened to, "Resume · 24m left"), and a title with no wide artwork shows its cover beside the words. Song entries carry the viewer's `plays`; an artist page's five most played songs read the maintained counts ("Popular" when the owner shares activity across profiles, "Your most played" otherwise).
- **A library's tabs** are authored in client-core, not published by the server: Discover first, then the kind's own (TV and anime: Shows, Recently aired, Collections; movies: Movies, Collections; music: Artists, Albums, Songs, Genres, Playlists, and Unmatched for owners; audiobooks: Books, Authors, Series). There is no Categories tab; genres, decades and studios are filters. Browse is a query engine over the derived browse rows: a filter tree (`all`/`any`/`not` of predicates on genre, year, rating, people, duration, date added, watched state, …), sorts with position anchors for the scrub rail, and cursor paging. Counts and facet option counts come from derived counts, never from `count(*)` over a library.
- **Recently aired is one card per show** (Justin, 2 Oct 2026): the tab reads the shows pivot sorted by `latestAired` (a show's most recently aired episode, newest first), and each show entry carries `latestEpisode` (air date, season and episode number, title, and how many of the show's episodes aired that day). The card is the show's poster; its caption is the episode ("S1 E4 · Sep 6") or the count ("4 episodes · Sep 17"). A row is posters or squares, never both, and never repeats one poster per episode.
- **Browse entries** carry what a card or a table row needs without a second read: year, rating, resolution, air date (episodes), the album's artist, the book's author, the song's album, and an absolute episode number for a show numbered across its seasons. The `aired` sort orders episodes by air date (an expression sort: offset paging, no letter anchors). The `match` field finds albums made from untagged files.
- **Detail pages** (title, show workspace, season, artist, album, book, person): the facts plus related rows and recommendation rows. A show's **per-show settings** (`catalog_show_settings`: hide seasons, absolute numbering, ranges of 100, newest first) are applied in the show workspace, which then lists groups (`all`, `range:<start>`, `specials`) in place of seasons; owners set them with `PUT /v1/admin/libraries/{id}/shows/{show}/settings`.
- **Search**: across the libraries a viewer may see, from the search documents.

Rules every read path follows:

- **One snapshot per request.** All reads for a document see one database snapshot.
- **O(visible).** A page does bounded, indexed work, independent of library size. Guards in the tests reject statements that would scan a large table.
- **Restrictions in the query.** A profile's library access and content restrictions are joined into the reads, not filtered afterwards.
- **Memo only under exact keys.** A result may be memoised only when keyed by every revision it read (catalogue, viewer, restriction fence, and any feature revisions), so nothing stale is ever served.

## Metadata

`internal/metadata` matches library items to provider records and refreshes them. `internal/metadataprovider` holds the bounded provider transports:

| Media | Providers |
|---|---|
| Films and TV | **One provider per library** (2 Oct 2026, as Plex and Jellyfin have it; never a chain): TMDB by default, for anime libraries too; TheTVDB is the alternative an owner picks in the library's Metadata settings, and an anime library may also pick AniList (which names no episodes) |
| Music | MusicBrainz, with AcoustID fingerprints for matching |
| Books and audiobooks | Embedded tags and local files |

- Keys ship with the server (the TMDB project token in `metadata/project.go` is intentionally distributed), so the owner configures nothing.
- **Local metadata only** is a per-library provider choice that never goes online (`internal/localonly`, `internal/localmetadata`).
- **Changing a library's provider re-matches the library** (2 Oct 2026). Every title matched with the provider it left goes back to unmatched and is searched for again with the new one; what it shows stays until the new match is published, and a different identity supersedes the old provider's facts (details, ratings, tags, credits, episode data). Identity locks on those titles are released, since they kept a match the library can no longer refresh. A show that moves to TheTVDB retires the other provider's facts on its episodes so TheTVDB's episode publication can name them. An automatic TheTVDB match is no longer recorded as an owner's lock. (`screen_review.go` `screenRematchLibraryTx`, `screen_publish.go`.)
- **A local NFO beside an online provider** (2 Oct 2026): the NFO owns the credit departments it lists (in practice the cast: the parser reads `<actor>`), and the library's provider supplies the departments it leaves out (director, writers, producers). A film's credits are rows per provider, so both sit side by side; a show's are one field, so the provider's share is the `creditsOnline` field, read beside `credits` (`screen_publish.go` `screenCreditsBesideLocal`).
- Refreshes are conditional (ETag/Last-Modified) where the provider supports it.
- **Artwork:** each entity has chosen artwork per role (poster, backdrop, logo, still, square, …) in `artwork_selections`. Images are stored once by digest, resized on request, and served with long-lived cache headers.
- **Manual metadata and Fix Match:** the owner can edit fields, lock them against refresh, and re-match an item from candidate rows with a title/year/provider search.
- **Media analysis** (`internal/mediaanalysis`, `probefacts`, `audiofacts`) records measured facts about files (streams, loudness, encoder delay, chapters, markers) separately from provider metadata.
