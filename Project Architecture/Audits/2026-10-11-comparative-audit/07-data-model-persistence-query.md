# 07 — Data model, persistence and the query engine

Auditor's note: this is a static source review. Nothing was built, run or measured. Every "what happens" statement about Portico below comes from reading the code in the working tree on 11 October 2026; none was reproduced at runtime. Portico paths are relative to `/home/justin/projects/Portico`. Comparison paths are written `jellyfin/…`, `navidrome/…`, `audiobookshelf/…`.

## Scope and sources

**Domain.** How each product stores what it knows (schema, entity identity, per-user state), how it keeps computed data up to date, how it protects the database under concurrent use, how it changes the schema between versions, and how it answers "show me page N of this library, filtered and sorted" and "search for X".

**Portico, read properly**

- Docs: `portico-internal/Project Architecture/02 Server.md`, `03 Catalogue and Metadata.md`, `Backlog.md`; `portico-internal/Product Contracts/architecture.md`, `architecture-amendments.md`.
- `apps/portico-server/server/internal/persistence`: `README.md`, `migrations.go`, `schema.go`, `maintenance.go`, all of migrations `0002`–`0011`, and of `migrations/0001_baseline.sql` (5,653 lines, 589 tables, 401 indexes, 654 triggers): lines 1–900 in full (catalogue core, personal items, browse rows, visibility rows, recommendation tables, the first block triggers), the vocabulary tables near line 3265, the personal-state and revision table definitions, and a scripted pass over every trigger that queues derived work or bumps a revision.
- `internal/dbwork`: `open.go`, `policy.go`, `class.go`, `context.go`, `read.go`, `write.go`, `gate.go`, `commit.go`, `retry.go`, `wal.go`, `checkpoint_schedule.go`, `integrity.go`, `foreignkeys.go`, `pressure.go`, `reader_pressure.go`, `sort_functions.go`, head of `instrument.go`.
- `internal/compactcatalog`: `write.go` and `derive.go` in full, `derive_search.go`, `derive_browse_blocks.go`, first third of `derive_browse.go`.
- `internal/catalog`: `browse_contract.go`, `browse_expression.go`, `browse_engine.go`, `browse_pages.go`, `search.go`, the cursor code in `discovery.go`, `content.go` (revision and retry helpers), `reads.go`, `inventory.go` (file observation and relocation), `purge.go`, `catalog.go` (delete and movie commit).
- `internal/sorttext`, `internal/facettext`, `internal/entityid` in full; `internal/backup/copy.go`; `cmd/server/maintenance.go`; `apikit/cursor`.
- `packages/client-core/src/browse.ts`, `browse-buckets.ts`, and the stale-continuation handling in `search.ts`, `library-content.ts`, `home-row-page.ts`.

**Portico, not read or only skimmed** (said plainly so nobody assumes otherwise): the 430-odd non-catalogue tables (playback, identity, live TV, downloads, social); most of the 654 triggers line by line (the counted-block triggers are generated and I read the title set only); `derive_facet_counts.go`, `visibility_classes.go`, `personal_sort_buckets.go`, `rec_*`; `statement_cache.go`; `planner_stats.go` beyond its header; `internal/worker`, `internal/supervise`, `internal/catalogtest`; the two OpenAPI files (the Go contract in `browse_contract.go` is the source of both).

**Comparison projects read**: see "How the others do it".

## How Portico does it today

### One file, one writer, a small pool of readers

Everything is in one SQLite file, `server.sqlite`, opened in WAL mode (write-ahead log: readers keep reading the old pages while one writer appends new ones). `internal/dbwork` owns every rule about touching it:

- **Three connection pools on one file** (`dbwork/open.go:33-69`): one connection that only ever writes, six for foreground reads, two for background reads. Each connection has a 4 MiB page cache; memory-mapped I/O is switched off on purpose (`dbwork/policy.go:11-36`).
- **A write gate in Go, above SQLite** (`dbwork/gate.go`). SQLite allows one writer at a time; instead of letting writers collide inside SQLite and learn about it as a "database is busy" error, every write first queues in an in-process gate that picks the next writer by *work class*. There are eight classes from "security fence" (revoking a session) down to "maintenance" (`dbwork/class.go:20-44`). A background writer that has waited gains one rung per second up to the ordinary-request rung, so it cannot starve behind requests, but security, recording and playback writes always go first (`gate.go:19-22`, `219-246`).
- **Every write transaction takes SQLite's write lock at BEGIN** (`_txlock=immediate`, `policy.go:107-115`), so a transaction never reads, then fails when it tries to upgrade to a write.
- **Reads never queue behind the gate.** A request that needs several queries to build one page runs them in one read snapshot (`dbwork.WithReadSnapshot`, `read.go:42-74`), so the page is internally consistent. Snapshots are admitted with permits (four foreground, one background) and are capped at 30 seconds, because a long-lived snapshot stops the log from being recycled.
- **Typed, bounded retries** for "busy" and "locked" (three attempts for requests and background batches, more for job admission and migrations; `retry.go:95-108`). SQLite's own busy timeout is 15 s.
- **Durability**: `synchronous=NORMAL` for everything except the security-fence class, which runs with `synchronous=FULL` on its own connection so a revocation cannot be lost to a power cut (`write.go:118-173`).
- **Log size control**: a housekeeping loop stats the `-wal` file every second and checkpoints every 30 s; past ~64 MiB it tries a truncating checkpoint when the server is quiet, past ~192 MiB it briefly holds back new readers (250 ms) to force one (`wal.go:137-170`, `checkpoint_schedule.go`).
- **Change-driven background loops**: every committed write wakes the loops that subscribed to the tables it touched (`commit.go`), so an idle server runs no polling queries.
- **Foreign keys are on** for every connection; a helper that needs them off must use a pinned connection and the connection is destroyed if enforcement cannot be restored (`foreignkeys.go`).
- **Health**: `PRAGMA quick_check` + `foreign_key_check` run on a schedule only if the owner sets `PORTICO_INTEGRITY_CHECK`, and on demand from an admin diagnostics route (`dbwork/integrity.go`, `cmd/server/maintenance.go:42-56,141-150`). Nothing repairs.

### Entities: kinds as data, narrow side tables

Every catalogue object is one row in `catalog_entities` (`0001_baseline.sql:36-53`): a 64-bit integer id used for every internal reference, a random 16-byte `public_id` (shown to clients as 22 characters), a library, a kind number, an optional parent, title, sort title, a computed sort key, year, and an added time.

- **Kinds are rows in `catalog_kinds`** with flags (playable, container, searchable, browsable, listening). Thirteen ship: movie, show, season, episode, artist, album, song, book, audiobook_file, collection, extra, disc, author (`:9-21`).
- **Facts that only one kind has live in a side table** keyed by the entity id: `catalog_shows`, `catalog_seasons`, `catalog_episodes`, `catalog_albums`, `catalog_songs`, `catalog_books`, `catalog_book_files`, `catalog_collections` (`:113-231`). Descriptive text for playable items is in `catalog_item_details`.
- **Files are not entities.** A file is a row in `catalog_assets` (path, size, mtime, container, codecs, dimensions, duration); `catalog_asset_links` joins files to items, so one item can have several files (versions or parts) and one file can back several items (a multi-episode file).
- **Shared words are stored once**: genres and other vocabularies in `catalog_terms` + `catalog_entity_terms` + `catalog_term_sources` (which provider said so, in which order); tags, studios, networks, content ratings, labels, audio languages, resolution in `catalog_attribute_fields/terms/edges`; credits in `catalog_credits` pointing at `catalog_people` and at interned role/department labels.
- **People and playlists are not entities**; they have their own tables and their own public tokens.
- **Provider ids** (TMDB, TVDB, IMDb, AniList, MusicBrainz) are rows in `catalog_external_ids`.

### Identity: a ledger that maps "where it came from" to a public id

`catalog_identities` (`:57-62`) maps (library root folder, source key) to a public id and is never deleted from. The source key is a path for a movie or a music/audiobook file (`file:<path relative to the library root>`), and a logical name for containers (`show:<key>`, `season:<key>/<n>`, `episode:<showkey>/<numbering>/<season>/<number>`, `artist:`, `album:`, `book:`, `collection:`, `author:`, `recording:`) (`compactcatalog/write.go:85-110`). Re-scanning, or removing and re-adding a library from the same folder, finds the same public ids.

Separately, the scanner recognises a *file* that moved or was renamed on the same disk by its device/inode identity plus unchanged size and mtime, and keeps the same asset (and therefore the same item) (`catalog/inventory.go:203-246`).

### Per-viewer state

Keyed by `(profile_id, item_id)`: `personal_items` (watchlist, favourite, half-star rating, watched, last played, not interested), `progress` (position with a unit: milliseconds, pages or fraction), `personal_history`, `personal_play_counts`, plus `book_resume` per book and `container_personal_state` for shows/albums/artists. The owner's `play_history` copies names in and has no foreign key, so it survives deletions. All the per-item tables cascade-delete with the entity.

### Derived data: maintained tables, not caches

Anything a page needs that is not a stored fact is kept in a derived table and updated when facts change:

1. The write API (`compactcatalog/write.go`) and ~75 small triggers put `(domain, entity id)` rows on one queue, `catalog_dirty`.
2. One supervised worker (`compactcatalog/derive.go:169-196`, `233-272`) drains it: one domain at a time, up to 64 ids per write transaction in the maintenance class, the domain served longest ago first.
3. Each derivation has a version in `catalog_derivations`. If the code's version is higher than the stored one, the worker re-enumerates every entity for that domain in the background.
4. A second tier of "step" workers (visibility classes, personal sort buckets, collection and category visible counts, content eligibility, recommendation profiles) is fed by triggers on the derived tables and runs only when the dirty queue is empty (`derive.go:385-410`).

What is derived: one FTS5 search document per searchable entity (title only); a **browse row** per browsable entity (`catalog_browse_rows`: title, sort key, first character, year, added time, content-rating key, label key, availability, longest duration and best rating of its members); **browse memberships** (which playable items stand behind a container row); **counted blocks** (`catalog_browse_blocks`: for each library, kind and each of five sort orders, a block marker roughly every 1,024–2,048 rows with a count, maintained by generated triggers, so "what is at position 3,000,000?" is a small read); **bucket counts** per first letter, year, decade, added month, rating and duration bucket, split by content-rating and label so a restricted viewer's totals are sums of buckets; **facet counts**; **visibility classes** (a pre-filtered copy of the browse rows for each distinct restriction policy, with their own blocks); recommendation features.

### Revisions and cursors

- `library_revisions(library_id, revision)` is bumped by triggers on practically every catalogue fact table; `viewer_revisions(profile_id, library_id, revision)` is bumped by triggers on a viewer's progress and personal items.
- Every list response carries the two numbers. A continuation cursor is a signed JSON blob holding the scope (library, viewer fence, profile, view, sort, filter fingerprint, limit), the last row's sort key and entity id, the list total, the two revisions and a 30-minute expiry (`catalog/discovery.go:36-66,106-165`). If either revision has moved, the server answers `409 stale_continuation` and the client starts again.
- First pages retry up to four times if a revision moves under them (`catalog/content.go:514-530`).

### The browse engine

`POST /v1/libraries/{id}/browse` (`catalog/browse_engine.go`) takes a *pivot* (Movies, Shows, Episodes, Artists, Albums, Songs, Books, Authors, Collections…), an optional filter tree, up to three sorts, and either a cursor or an absolute `range.start`.

- **Filters** are a JSON tree of `all` / `any` / `not` over predicates on 33 fields (`browse_contract.go:94-163`): title, year, decade, release date, date added, play state, favourite, watchlisted, personal rating, last played, genre, tag, label, collection, content rating, community and critic rating, availability, resolution, dynamic range, audio language, studio, network, actor, director, writer, duration, artist, album, author, narrator, series, entity kind, match state. Limits: depth 5, 40 predicates, 64 KiB, 100 values per list. Each predicate compiles to either a test on the browse row or an `EXISTS` over the row's member items.
- **Sorts**: title, added, year, duration, community rating (all backed by an index and a counted block ordering), plus personal rating, last played, "for you", aired and latest-aired (computed).
- **Paging**: with no filter and one stored sort, any position is reached through the counted blocks, in either direction, and a cursor continues by key. With a filter or a second sort, pages are `LIMIT … OFFSET …` over the filtered order and the total is counted once and carried in the cursor.
- **Totals and the letter rail** for an unfiltered grid come from the bucket counts, not `count(*)`.
- **Restrictions** (library access, content rating, labels) are compiled into the query, or the query reads the viewer's visibility class.
- **After the library changes** a client holding `range.start` + `revision` + `anchorId` gets its position recomputed from the anchor entity's rank in the blocks instead of being sent back to the top (unfiltered, single stored sort only).

### Search

`GET /v1/search` (`catalog/search.go`): the query is split into letter/number tokens, each becomes a prefix term, all must match (`NormalizeSearch`, `:114-135`). It needs 2–128 characters, 1–8 tokens, and at least one token of two or more characters. Nine groups (movies, shows, episodes, artists, albums, songs, books, people, live TV channels) run one after another on one snapshot, each with a 600 ms budget. Matching is SQLite FTS5 with the `unicode61` tokenizer, diacritics removed, prefix indexes for 2–4 characters (`0001_baseline.sql:322-328`). Ranking is FTS5's bm25 plus a large boost for an exact title and a smaller one for a title prefix. Library scope and kind are tokens inside the index, so each group ranks only its own candidates. People are a second FTS table over names.

### Sorting text

`internal/sorttext/sort.go`: an explicit sort title wins; otherwise a leading article is dropped using a per-language list (en, fr, es, de, it, pt, nl; an unknown language drops nothing; no language means English), then the text is case-folded and stripped of combining marks. The letter for the jump rail is the first character if it is a letter in any script, else `#`.

### Migrations

`internal/persistence`: one squashed baseline (it replaced 265 pre-release migrations on 25 September 2026) plus numbered migrations applied in ascending order, each recorded with a SHA-256 digest that is re-checked on every start. Numbers may arrive out of order across development lanes; the runner applies whatever is missing. A database written by a newer binary is refused; a database from before the baseline is refused with "start fresh". The written rules (`persistence/README.md`): additive only, never rewrite a large table at startup, derived data is rebuilt in the background by bumping a derivation version, enum numbers are permanent, kinds are data.

### Backup

`internal/backup/copy.go`: SQLite's online backup API, 16 pages per step, from a background-pool connection holding one read snapshot for the whole copy; then `fsync`, then `PRAGMA quick_check` on the copy before it is listed.

## How the others do it

### Jellyfin (server, master as cloned October 2026)

Read: `jellyfin/src/Jellyfin.Database/**` (entities, `SqliteDatabaseProvider.cs`, `PragmaConnectionInterceptor.cs`, the three locking behaviours, the migration list), `jellyfin/Jellyfin.Server.Implementations/Item/` (`BaseItemRepository.Querying.cs`, `.QueryBuilding.cs`, `.TranslateQuery.cs` search and filter sections, `OrderMapper.cs`, `ItemPersistenceService.cs`), `jellyfin/MediaBrowser.Controller/Entities/InternalItemsQuery.cs`, `BaseItem.cs` (sort names), `jellyfin/Emby.Server.Implementations/Library/LibraryManager.cs` (item ids), `jellyfin/Jellyfin.Server/Migrations/**` (service, backup attribute, routine list), `CleanupUserDataTask.cs`, `FilterController.cs`. Not read: `DtoService`, `ItemCountService`, `PeopleRepository`, `NextUpService`, the in-memory item cache internals, the individual migration routines' bodies.

- **Storage.** One SQLite file, `jellyfin.db`, through Entity Framework Core (an object-relational mapper). Since 10.11 everything that used to be in separate `library.db`, `users.db`, etc. is in this file. The provider layer is pluggable; the readme says PostgreSQL is "experimental" and SQLite is the only supported provider (`jellyfin/src/Jellyfin.Database/readme.md`).
- **One wide table.** Every item of every type is a row in `BaseItems` (`BaseItemEntity.cs`): about 75 columns shared by movies, episodes, songs, folders, live-TV programmes, people-as-items, genres-as-items and so on, a `Type` column holding the C# class name, and a `Data` column of JSON for whatever does not have a column. Side tables hold things that are lists: `ItemValues`/`ItemValuesMap` (artists, album artists, genres, studios, tags, each with a display value and a "clean" value), `Peoples`/`PeopleBaseItemMap`, `UserData`, `AncestorIds` (every ancestor of every item, so "everything under this folder" is one join), `LinkedChildren` (collections, playlists, alternate versions), `BaseItemProviders` (TMDB/IMDb ids), `MediaStreamInfos`, `Chapters`, `BaseItemImageInfos`, `TrickplayInfos`, `KeyframeData`, `MediaSegments`. Genres, studios, tags and artists are *also* stored as delimited strings on the item row.
- **Identity.** An item's id is the MD5 of its type name plus its path (lower-cased unless configured otherwise) (`LibraryManager.cs:797-819`). Move or rename a file and it is a different item.
- **Per-user state.** One `UserData` row per (item, user, key): played, play count, position, favourite, rating, likes, last played, chosen audio/subtitle stream. The key is derived from provider ids where available. When an item is deleted its user data is *detached* to a placeholder item with a retention date, and when an item reporting the same keys is saved later the data is re-attached (`ItemPersistenceService.cs:94-132`, `222-300`). A task deletes detached rows older than 90 days (`CleanupUserDataTask.cs:54`).
- **Concurrency.** Three selectable modes (`DatabaseLockingBehaviorTypes.cs`): `NoLock` (default; rely on SQLite), `Pessimistic` (one process-wide reader/writer lock: every read waits while any write runs; its own doc comment says it is unsafe with asynchronous transactions), `Optimistic` (retry a failed write up to 15 times with back-off, `OptimisticLockBehavior.cs:31-47`). Pragmas are owner-configurable: cache size, locking mode, journal size limit (default 128 MiB), temp store, synchronous (default NORMAL), and arbitrary `#PRAGMA:` entries (`SqliteDatabaseProvider.cs:60-74`). Command timeout 60 s.
- **Maintenance.** A scheduled "optimize database" task checkpoints, runs `VACUUM`, then a full `ANALYZE` (skipped while the library is empty, with a comment explaining that statistics taken on an empty library make the planner choose full scans later), then checkpoints again (`SqliteDatabaseProvider.cs:157-203`). Shutdown checkpoints and truncates the log.
- **Migrations.** Two kinds: EF schema migrations (about 55 for SQLite) and about 45 dated C# "routines" that fix data (`MergeDuplicatePeople`, `HarmonizeConflictingUserData`, `RefreshCleanNamesAndValues`, `RepairAlternateVersionLinks`, `CleanupOrphanedExternalData`, …). A migration can declare that it needs a backup; the service copies the database file first and restores it if the migration throws (`JellyfinMigrationService.cs:320-352`, `SqliteDatabaseProvider.MigrationBackupFast`). Migrations run before the server starts serving. Public reports around 10.11 describe that first start taking hours on large libraries and "database is locked" failures during scans (forum threads and third-party write-ups; I did not verify the GitHub issues they cite).
- **Queries.** `InternalItemsQuery` is a flat object with about 150 optional properties, all AND-ed. `TranslateQuery` turns it into one LINQ query. Paging is `StartIndex` + `Limit` (SQL `OFFSET`); the total is a separate `COUNT` of the same query, skippable with `EnableTotalRecordCount=false` (`BaseItemRepository.Querying.cs:31-64`). About 35 sort keys including `Random` (`ORDER BY random()`, re-rolled on every page). Name jump is a filter (`NameStartsWith`, `NameStartsWithOrGreater`, `NameLessThan`) on the sort name.
- **Search** is a filter on the same query: `CleanName LIKE '%term%'` or `OriginalTitle LIKE '%term%'` (`TranslateQuery.cs:260-275`), ranked exact, prefix-at-word-boundary, prefix, contains (`OrderMapper.MapSearchRelevanceOrder`). No full-text index.
- **Filter option lists** are computed per request with `DISTINCT` over the current query (`BaseItemRepository.Querying.cs:603-660`); they have no counts.
- **Sort names** are lower-cased, lose "the/a/an" at the start, middle and end, lose `, & - { } '`, turn `. + %` into spaces, and have every run of digits left-padded to ten digits so 2 sorts before 10; all three lists are server settings (`BaseItem.cs:992-1040`, `ServerConfiguration.cs:115-127`).

### Plezy

Plezy is a client for Plex, Jellyfin and Emby. It has no server database in this domain; it consumes those servers' offset-and-total paging. I did not read Plezy for this domain and make no claims about it.

### Navidrome (Go + SQLite, the closest technical match)

Read: `navidrome/persistence/sql_search*.go` (function headers and comments), `navidrome/db/optimize.go`, `repair.go`, `backup.go`, `db.go` (headers), `db/migrations/20260220173400_add_fts5_search.go`, `consts/consts.go` (persistent ids), the migration list. Not read: `criteria_sql.go` (smart playlists), repositories in detail.

- Class-per-table model (`media_file`, `album`, `artist`, `tag`, `folder`, `library`), 138 goose migrations.
- **Persistent ids from tags, not paths**: a track's id is computed from `musicbrainz_trackid`, or failing that album id + disc + track number + title; an album's from `musicbrainz_albumid` or album artist + album + version + release date (`consts.go:157-158`). Moving files does not change identity. The recipe is a setting.
- Per-user state in one `annotation` table keyed by (user, item type, item id), joined only when a filter or sort needs it (`sql_annotations.go:51-72`).
- **Search** is FTS5 with `unicode61 remove_diacritics 2`, one index per table, many columns each (title, album, artist, album artist, sort names, participants) with per-column bm25 weights. It also has what Portico's search lacks: a `search_normalized` column of punctuation-stripped text so "rem" finds "R.E.M." and "acdc" finds "AC/DC"; dotted abbreviations become phrases; each token is `(token OR token*)` so an exact word outranks a prefix; a query containing Chinese/Japanese/Korean characters falls back to `LIKE`, because the tokenizer treats a run of CJK characters as one token (`sql_search_fts.go:18-20,52-61,108-114`). Search is two-phase (rank ids on a covering index, then hydrate the page) because joins made large offsets slow "on 100k+ libraries" (`sql_search.go:88-94`).
- Maintenance: full `ANALYZE` when stale ("`PRAGMA optimize` is avoided because its limited analysis misestimates Navidrome's low-cardinality indexes", `optimize.go:17-18`); `PRAGMA integrity_check` and an aggregated foreign-key check; a repair path that rebuilds the FTS tables and triggers (`repair.go`); backup through the online backup API.

### Audiobookshelf (Node, Sequelize, SQLite) — brief

`audiobookshelf/server/models/*` is class-per-table (Book, Podcast, PodcastEpisode, LibraryItem, Author, Series, MediaProgress per user). Search is `LIKE`, with an optional loadable extension for accent-insensitive matching (`Database.js:263-266,957-984`). Pragmas come from environment variables. Migrations are versioned JavaScript files with up and down functions.

## Architectural differences

### A-07.1 Kinds as data with narrow side tables, versus one wide table with a type string
- **Portico:** `catalog_entities` has 13 columns; a kind is a row in `catalog_kinds` with behaviour flags; kind-specific facts sit in eight side tables (`0001_baseline.sql:9-231`). Code asks flags, not kind lists.
- **Jellyfin:** one `BaseItems` table, ~75 columns, `Type` is a C# class name, the rest of the object is JSON in `Data` (`BaseItemEntity.cs`). Navidrome and Audiobookshelf use one table per class.
- **Why it matters:** Portico's rows are small (more fit in memory and on a page) and a new kind does not alter an existing table. Jellyfin can filter any type on any column with one query, but most columns are NULL for most rows, a type rename is a data migration, and anything inside `Data` cannot be filtered or indexed.
- **Assessment:** **Portico ahead.** Deliberate (`persistence/README.md` rule 6). The cost is more joins when composing a detail page, which Portico pays by precomputing browse rows.

### A-07.2 Integer ids inside, random public ids outside, versus a GUID computed from the path
- **Portico:** every internal reference is a 64-bit integer; the client-visible id is 16 random bytes resolved through an index (`entityid/entityid.go`). `catalog_identities` remembers which public id belongs to which (library root, source key) and is never deleted from.
- **Jellyfin:** id = MD5(type name + path). No lookup table is needed: the same path always yields the same id, and a different path never does.
- **Why it matters:** Portico's joins and indexes are 8-byte integers instead of 16-byte GUIDs, which is a large saving at millions of rows across dozens of derived tables. Public ids do not leak paths. Jellyfin's scheme is simpler and stateless but ties identity to the path string and its case rules (`EnableCaseSensitiveItemIds`).
- **Assessment:** **Portico ahead** on size and on not exposing structure. Deliberate (schema header comment, "Direct Design §4"). See A-07.4 and B-07.3 for the places where the ledger's promise is weaker than it reads.

### A-07.3 Files are separate from titles
- **Portico:** `catalog_assets` (one row per file) and `catalog_asset_links` (item ↔ file, with a part index and optional start/end seconds). One title can own several files; one file can back several titles.
- **Jellyfin:** each file is its own `BaseItem`; alternate versions are separate items tied together by `PrimaryVersionId` and `LinkedChildren`, with user data tracked per version and merged in sort expressions (`OrderMapper.cs` comment on alternate versions) and a repair routine for broken links (`RepairAlternateVersionLinks`).
- **Why it matters:** in Portico watched state, ratings and playlists attach to the title, not to a file, so swapping a file for a better copy changes nothing personal. Jellyfin's design needs reconciliation code and has needed repairs.
- **Assessment:** **Portico ahead** in the data model. (Whether the scanner actually groups versions is another matter: Backlog already lists "a second file of the same title becomes a second title".)

### A-07.4 What happens to identity when files move
- **Portico:** two mechanisms. (1) The scanner keeps the same asset when a file with the same device/inode identity, size and mtime shows up at a new path in the same source (`catalog/inventory.go:203-246`), so same-disk moves and renames keep the item. (2) Containers are keyed by logical names (show key, season number, episode numbering), not paths. A copy to another disk, or anything that changes inode/size/mtime, is a new file; for a movie or a music/audiobook file that means a new entity, and the old one becomes "unavailable" and keeps the personal state.
- **Jellyfin:** any path change is a new id, but user data re-attaches by provider-derived keys (see A-07.5).
- **Navidrome:** ids are computed from tags (MusicBrainz ids or album/disc/track/title), so moves never matter.
- **Why it matters:** the common real-world cases are reorganising folders, renaming to a new convention, moving to a bigger disk, and replacing a pool. Portico handles the first two well and the last two not at all for movies and music.
- **Assessment:** **Trade-off.** Portico's same-disk continuity is stricter and safer than Jellyfin's (it never guesses). But the outcome a person sees after moving a library to a new drive is "everything is unwatched", where Jellyfin and Navidrome keep it. What would change my view: if cross-disk moves are expected to go through an owner tool that re-links (the inventory contract mentions an owner "associate" action).

### A-07.5 Personal state is deleted with the title, versus detached and re-attached
- **Portico:** `personal_items`, `progress`, `personal_history`, `personal_play_counts` and `play_counts` all reference the entity `ON DELETE CASCADE` (`0001_baseline.sql:345`, `1277`, `1996`; `0007_play_history.sql`). Playlist entries are kept but their item pointer is set to NULL (`:255`). The identity ledger gives a returning title its old *public* id, but nothing personal comes back with it because everything personal was keyed by the integer id that was deleted.
- **Jellyfin:** user data moves to a placeholder with a retention date and is merged back when an item with the same keys is saved (`ItemPersistenceService.cs:94-132,222-300`); kept 90 days.
- **Why it matters:** in Portico a file going missing does not delete the entity (it becomes unavailable), so ordinary outages are safe. Deletion happens on owner "forget", DVR retention and container clean-up. After a forget-and-rescan, or after a cross-disk move (A-07.4), history is gone for every viewer.
- **Assessment:** **Portico behind** on resilience, by a small margin in everyday use and a large one in the "I rebuilt my library" case. Adopting it is medium: a detached-state table keyed by public id (the ledger already preserves that key) and a re-attach step in `upsertEntity` when a ledger entry is found with no live entity.

### A-07.6 Shape of per-viewer state
- **Portico:** several narrow tables: flags and rating (`personal_items`), position with a unit (`progress`: milliseconds, pages or millionths), history rows, play counts, per-book resume, container-level state. Profile ids are text.
- **Jellyfin:** one `UserData` row holding everything, including chosen audio and subtitle stream indexes.
- **Navidrome:** one `annotation` row per (user, type, item).
- **Why it matters:** Portico can add a medium (books by page, photos by fraction) without new columns, and a progress tick touches a tiny row. Jellyfin's single row is simpler to reason about and to export.
- **Assessment:** **Trade-off**, leaning Portico for scale. Deliberate (README "Progress").

### A-07.7 Genres, studios, tags
- **Portico:** two normalised systems. Genres: `catalog_terms` (vocabulary + key + label), `catalog_entity_terms`, and `catalog_term_sources`, which records *which provider* named the term, the provider's id for it, a label override and its position (the first genre is the primary one). Other attributes (tag, label, studio, network, content rating, audio language, dynamic range, series, release date): `catalog_attribute_fields/terms/edges`. Free-text copies also exist on `catalog_item_details` and `catalog_shows` (`studio`, `network`, `tags`, `labels` columns).
- **Jellyfin:** `ItemValues(Type, Value, CleanValue)` + map, and the same values again as delimited strings on the item row.
- **Why it matters:** Portico knows provenance, so changing a library's provider can retire exactly that provider's genres. It also means the same fact can live in up to three places (text column, attribute edges, term tables), which is where drift comes from.
- **Assessment:** **Portico ahead** on provenance; **Unclear** on redundancy. I did not trace whether the text columns and the edges can disagree.

### A-07.8 People
- **Portico:** `catalog_people` (token, an identity key, name, lower-cased sort name, biography, dates, portrait, provider ids as JSON) and `catalog_credits` rows per entity and provider, with interned role and department labels and the provider's person id. People are found by a separate FTS index kept by triggers.
- **Jellyfin:** `Peoples(Id, Name, PersonType)` and a map with role and order. People are also `BaseItem`s. It has needed `MergeDuplicatePeople` and a scheduled `PeopleValidationTask`.
- **Why it matters:** Portico can keep two people with the same name apart (identity key from the provider) and can show provider-specific credits side by side.
- **Assessment:** **Portico ahead** in model. Two gaps: nothing ever deletes a person nobody is credited as any more (B-07.15), and the browse filters match people by credited name text rather than by person (A-07.32).

### A-07.9 Hierarchy
- **Portico:** `parent_id` on the entity plus explicit foreign keys in side tables (episode → show and season, song → album, album → artist, book file → book). "Which playable items stand behind this container" is a derived table (`catalog_browse_memberships`, source 0–8).
- **Jellyfin:** `ParentId`, `TopParentId`, `SeasonId`, `SeriesId` columns, plus `AncestorIds`, a stored closure of every ancestor of every item, plus `LinkedChildren` for things that are not folder children.
- **Why it matters:** Jellyfin's closure table answers "under this folder, recursively" for arbitrary folder trees, which matters because Jellyfin models real folders as items. Portico does not model folders at all, so it needs only the typed edges. Portico cannot offer "browse by folder".
- **Assessment:** **Trade-off (subjective).** Portico's is smaller and typed; Jellyfin's supports folder view and mixed-content libraries.

### A-07.10 Missing and unaired episodes
- **Portico:** I found no "virtual item" flag in the schema. An episode row exists because a file was catalogued (`catalog/episode_repository.go:124-177`). I did not check whether provider publication ever creates file-less episodes.
- **Jellyfin:** `IsVirtualItem` rows for episodes a provider knows but the library lacks, filterable with `IsMissing` and `IsUnaired` (`TranslateQuery.cs:1005-1030`), used by "display missing episodes" and by Next Up.
- **Assessment:** **Portico behind** if "what am I missing from this season" is wanted; otherwise simply leaner. See F-07.12.

### A-07.11 Collections belong to one library
- **Portico:** `catalog_collections.library_id`, unique name per library (`0001_baseline.sql:218-224`); visible counts and edges are per library.
- **Jellyfin:** a BoxSet is its own item in a dedicated collections folder and can hold movies and shows from any library.
- **Assessment:** **Portico behind** for franchises that span films and series (Star Wars, Marvel). Medium to change: the entity carries one library id and several derived tables assume it.

### A-07.12 Derived tables kept by a queue and a worker, versus computing at query time
- **Portico:** described above. Facts are written synchronously; search documents, browse rows, memberships, counts, blocks, visibility classes and recommendation features follow asynchronously, normally within one worker pass.
- **Jellyfin:** almost nothing is derived in the database. Counts, filter options, "latest", Next Up and played-state of folders are computed per request by SQL over `BaseItems`/`UserData`, helped by an in-process item cache.
- **Navidrome:** a few maintained columns (album and artist aggregates updated during scan), FTS kept by triggers.
- **Why it matters:** this is the central architectural bet. Portico's read cost is independent of library size for the common shapes; the price is write amplification (one new film touches dozens of rows across ~20 tables and fires chains of triggers), a large schema, and a class of bug Jellyfin cannot have: a derived table that silently disagrees with the facts. Jellyfin's price is that page cost grows with the library, which is where its "large library is slow" reputation comes from.
- **Assessment:** **Portico ahead** for the stated 5–10 million target, provided the derived data can be *verified and repaired* in the field. Today the verifier for the counted blocks is a test-and-diagnostics function (`CheckBrowseBlocks`, `derive_browse_blocks.go:55-72`) and there is no owner-facing "check and rebuild" (F-07.17).

### A-07.13 Totals
- **Portico:** an unfiltered grid's total, letter index and restricted-viewer totals come from maintained bucket counts (`catalog_browse_buckets`, split by content-rating key and label key). A filtered list is counted once and the number rides in the cursor.
- **Jellyfin:** `COUNT(*)` of the full query on every request unless the client opts out.
- **Assessment:** **Portico ahead.**

### A-07.14 Reaching position N
- **Portico:** counted blocks give the key at any position in five orderings, both directions, so a scrub rail can jump to row 3,000,000 with a read of a few thousand block rows plus a walk of under 2,048 rows (`browse_pages.go:153-190`).
- **Jellyfin / Navidrome:** `OFFSET N`, which reads and discards N rows.
- **Why it matters:** deep jumps stay fast. The cost is ten generated triggers per browse-row change (join, leave and move for five orderings, and the same again for each visibility class the row is in).
- **Assessment:** **Portico ahead**, with the caveat in A-07.12 about verification.

### A-07.15 Paging contract: signed, revision-bound cursors and anchored ranges, versus start index and limit
- **Portico:** a cursor is opaque, HMAC-signed, bound to the viewer, query, sort, page size and the exact catalogue and viewer revisions, and expires after 30 minutes. A grid can instead ask for `range.start` and, after a change, relocate by `anchorId`. A stale cursor is a 409.
- **Jellyfin:** `StartIndex`/`Limit`. Nothing binds two pages together: if the library changes between pages, rows are silently skipped or repeated.
- **What third-party tools gain and lose:** with Portico's contract a tool can never show a duplicated or skipped row and can never page with a cursor minted for another viewer. It loses the ability to fetch "page 37" statelessly with a plain cursor (it can with `range.start`), must handle 409 by restarting, must finish within 30 minutes, and cannot share a cursor between processes. Tools written for Jellyfin's or Subsonic's offset APIs cannot be pointed at Portico.
- **Assessment:** **Trade-off (subjective).** Correctness is clearly better. The cost is concentrated in one place: how often revisions move (A-07.16, B-07.4).

### A-07.16 Revision counters are bumped by triggers on almost every row
- **Portico:** about 80 triggers bump `library_revisions` (entities, every side table, assets, links, credits, terms, attributes, artwork, metadata fields, extras); progress and personal-item triggers bump `viewer_revisions` per (profile, library). `dbwork/write.go:22-33` notes `library_revisions` moves "four times per catalogue row".
- **Jellyfin:** no revision concept; clients re-fetch.
- **Why it matters:** a single number per library is a blunt instrument. Everything keyed on it (cursors, range revisions, the facet cache, the anchor cache, the filtered-total cache, ETags) is invalidated by any change anywhere in that library, including an artwork fetch for one title, and by one's own playback progress.
- **Assessment:** **Unclear.** The mechanism is sound; the granularity may be too coarse for a library that is being scanned for days or a profile that is always playing music. See B-07.4.

### A-07.17 Write ordering: one gate with a priority ladder, versus selectable locking modes
- **Portico:** one in-process gate, eight classes, ageing for background work, BEGIN IMMEDIATE, dedicated writer connection (`dbwork/gate.go`, `write.go`).
- **Jellyfin:** owner picks NoLock, Pessimistic or Optimistic; none knows what kind of work a write is. The existence of three modes and a 15-step retry table is itself evidence of unresolved contention.
- **Why it matters:** in Portico a progress update or a sign-out fence goes ahead of a scan batch by construction. In Jellyfin they race and the loser retries or fails.
- **Assessment:** **Portico ahead.** One thing Jellyfin has that Portico does not: an escape hatch. If the gate misbehaves in the field there is no setting to change (A-07.21).

### A-07.18 One snapshot per request
- **Portico:** a composed page reads through one read transaction, so counts, rows and cursors agree (`dbwork/read.go:42-74`).
- **Jellyfin:** each repository call opens its own context; a page built from several calls can see different states.
- **Assessment:** **Portico ahead.**

### A-07.19 Durability
- **Portico:** `synchronous=NORMAL`; the security-fence class runs FULL on its own connection (`write.go:118-173`).
- **Jellyfin:** `synchronous` is a setting, default 1 (NORMAL).
- **Assessment:** **Portico ahead** (per-class durability is unusual and well reasoned). Note for completeness: with NORMAL, the last few commits before a power cut can be lost, including a "watched" mark; neither product treats that as a problem.

### A-07.20 Write-ahead log growth
- **Portico:** measured and actively managed (stat every second, passive checkpoints, truncation when quiet, a hard threshold that briefly pauses new readers) (`dbwork/wal.go`, `checkpoint_schedule.go`).
- **Jellyfin:** `journal_size_limit` 128 MiB, truncating checkpoint at shutdown and in the optimize task.
- **Assessment:** **Portico ahead**, by a lot. This is the most carefully engineered part of the domain.

### A-07.21 Owner-tunable database settings
- **Portico:** none. `DefaultPolicy` "deliberately takes no configuration input" (`dbwork/policy.go:39-46`).
- **Jellyfin:** cache size, locking mode, journal limit, temp store, sync mode, arbitrary pragmas, command timeout. Audiobookshelf: pragmas by environment variable.
- **Why it matters:** "low ceremony" argues for none. Target hardware "from weak boxes to a couple of hundred concurrent browsers" argues that one fixed 4 MiB-per-connection cache cannot be right for both.
- **Assessment:** **Trade-off (subjective).** Deliberate. What would change my view: measured results on a large library showing the fixed cache is adequate.

### A-07.22 Page cache and memory mapping
- **Portico:** 4 MiB per connection (nine connections, 36 MiB total), mmap off, temp store in memory.
- **Jellyfin:** SQLite's default cache unless configured.
- **Why it matters:** at millions of items the database is tens of gigabytes; with a 36 MiB cache nearly every non-trivial read depends on the operating system's file cache. That is fine on a box with spare RAM and poor on a small NAS under memory pressure. Performance targets are not yet established (brief), so this is unmeasured.
- **Assessment:** **Unclear.** Needs measurement at target scale.

### A-07.23 Planner statistics
- **Portico:** its own pass that writes the same `sqlite_stat1` rows `ANALYZE` would, by sampling each index in short gated steps, because a real `ANALYZE` is one long write transaction (`persistence/planner_stats.go:17-40`). Backlog notes a restart repeats unfinished tables and that pooled read connections keep the statistics they loaded.
- **Jellyfin:** full `ANALYZE` after `VACUUM` in the optimize task. **Navidrome:** full `ANALYZE` when stale.
- **Why it matters:** all three learned the same lesson independently: statistics from an empty database make SQLite choose table scans later (Portico Backlog "Planner statistics"; Jellyfin `SqliteDatabaseProvider.cs:193-195`). Portico's version never blocks writers.
- **Assessment:** **Portico ahead** in intent; it is also the only hand-written reimplementation of `ANALYZE`, which is a maintenance liability if sampled numbers differ from true ones in a way that changes plans.

### A-07.24 Reclaiming space
- **Portico:** never. No `VACUUM`, no `auto_vacuum`, no incremental vacuum anywhere in the server (searched).
- **Jellyfin:** `VACUUM` in the scheduled optimize task.
- **Why it matters:** after an owner forgets a large library or a derivation is rebuilt, the file keeps its high-water size forever, and backups copy the free pages.
- **Assessment:** **Portico behind.** `VACUUM` is a full rewrite under an exclusive lock, which conflicts with "never paused"; `auto_vacuum=INCREMENTAL` must be chosen before the first table is created, so it is a now-or-never decision before release (the vendored driver supports `_auto_vacuum` in the DSN, `thirdparty/sqlite/driver.go:117`).

### A-07.25 Integrity checking and repair
- **Portico:** `quick_check` and `foreign_key_check`, off unless `PORTICO_INTEGRITY_CHECK` is set; results go to the log and a diagnostics endpoint; nothing repairs. Backups are `quick_check`ed before being listed.
- **Navidrome:** `integrity_check`, aggregated FK check, and a repair that rebuilds FTS tables and triggers. **Jellyfin:** no scheduled check; many one-off repair routines.
- **Assessment:** **Trade-off.** Portico's backup verification is better than either. Its day-to-day detection is opt-in by environment variable, which few owners will find.

### A-07.26 Migration mechanics
- **Portico:** plain SQL files, each applied in one transaction with a recorded digest; numbers can arrive out of order across lanes; a newer database is refused; no file backup is taken first.
- **Jellyfin:** EF migrations plus code routines, staged, with declared backup needs and automatic restore on failure.
- **Why it matters:** SQLite DDL is transactional, so a failed Portico migration leaves the database untouched; that covers "it threw". It does not cover "it succeeded and was wrong", for which the only remedy is a backup the owner happened to have.
- **Assessment:** **Trade-off.** Portico's is simpler and its digest check is stricter. Jellyfin's backup-before-migrate is worth copying (F-07.16).

### A-07.27 What an upgrade does to derived data
- **Portico:** a derivation's version is raised in code; after start the worker re-derives every entity for that domain in the background. Readers that check readiness answer "catalogue is being prepared" until it finishes (`derive.go:124-140`; browse checks domains 18, 19, 20, 25, 26, 28 in `catalog/compact_browse.go:6-8`; search checks 17).
- **Jellyfin:** data fixes run as blocking migration routines before the server serves anything.
- **Why it matters:** Portico starts immediately and stays playable; Jellyfin can sit at a splash screen for hours. But Portico's browse or search for *every* library is unavailable for the length of the rebuild. See B-07.2.
- **Assessment:** **Trade-off**, and currently not the one the README describes.

### A-07.28 Database engine
- **Portico:** SQLite only. The architecture contract says "Do not change engines before measuring the actual bottleneck".
- **Jellyfin:** a provider abstraction with SQLite supported and PostgreSQL experimental.
- **Why it matters:** Portico's schema leans on SQLite specifics throughout (FTS5, `WITHOUT ROWID`, `STRICT`, `ANY` columns, partial indexes, `INDEXED BY`, trigger-maintained counters, WAL snapshots). It could not be ported without a rewrite. Jellyfin pays for portability by being unable to use most of those.
- **Assessment:** **Trade-off (subjective).** Deliberate, and consistent with "one state folder you can copy".

### A-07.29 Where the logic lives
- **Portico:** 589 tables, 401 indexes and 654 triggers in the baseline. A large share of behaviour (revision bumps, dirty queueing, counted blocks, job queues for the step workers) is SQL in triggers, some of it generated by a script (`blocks.py`, referenced at `0001_baseline.sql:832`).
- **Jellyfin:** about 35 entity classes; behaviour is C#.
- **Why it matters:** triggers fire whoever writes, which is why Portico can promise derived data never misses a change. They are also invisible at the call site, hard to step through, and a single statement can fan out into dozens. Test guards exist (`trigger_plan_test.go`, `write_amplification_test.go`, `upsert_trigger_guard_test.go`).
- **Assessment:** **Trade-off (subjective).** I would keep it, and invest in field verification (F-07.17).

### A-07.30 Filter model: a boolean tree, versus a flat list of fixed parameters
- **Portico:** `all`/`any`/`not` over 33 fields with 15 operators, validated against a published capability document, compiled to one WHERE clause (`browse_expression.go`). The same tree is a saved view, a Home row and a Library Channel rule.
- **Jellyfin:** ~150 optional parameters, all AND-ed; OR exists only inside multi-value parameters.
- **Navidrome:** smart playlists have a comparable criteria tree compiled to SQL (`criteria_sql.go`, not read in detail).
- **Assessment:** **Portico ahead** in expressiveness and in being self-describing. Jellyfin covers more individual facts (A-07.31).

### A-07.31 Which facts can be filtered on
- **Portico has, Jellyfin lacks (as first-class filters):** watchlisted, personal rating range, last-played date range, label, collection membership, critic rating, availability (available/partial/unavailable), dynamic range, network, narrator, book series, match state.
- **Jellyfin has, Portico lacks:** has subtitles; audio/subtitle language absent; video type (file, DVD, Blu-ray, ISO); 3D; width/height ranges; has trailer, theme song, theme video, special features; is missing / is unaired; series status (continuing/ended); has TMDB/IMDb/TVDB id or any provider id; has overview; has a given image type; is locked; min date last saved; parental rating min/max as a number; include/exclude item ids; adjacent-to; person type; years as a list; index-number ranges; "is favourite or liked".
- **Assessment:** **Trade-off.** Portico's set is viewer-oriented, Jellyfin's is also a librarian's toolkit. The librarian filters (no match, no artwork, no overview, no subtitles, locked) are the notable gap; see F-07.3.

### A-07.32 Filtering by name instead of by identity
- **Portico:** `actor`, `director`, `writer` compare `catalog_credits.credited_name`; `artist` compares the artist entity's title; `album`, `author`, `narrator`, `studio`, `network` compare text (`browse_contract.go:145-160`). Only `collection` compares an id.
- **Jellyfin:** `PersonIds`, `ArtistIds`, `AlbumArtistIds`, `StudioIds`, `GenreIds`, `AlbumIds`.
- **Why it matters:** two people or two artists with the same name are indistinguishable in a Portico filter; a person credited under a variant spelling ("Robert De Niro" / "Robert DeNiro") is missed. "More with this actor" from a person page presumably uses the person table, but a saved view built from the filter does not.
- **Assessment:** **Portico behind.** Small to add id-typed fields beside the text ones.

### A-07.33 Sorts
- **Portico:** title, added, year, duration, community rating (indexed, with counted blocks); personal rating, last played, for-you, aired, latest-aired (computed; offset paging). Up to three sorts.
- **Jellyfin:** about 35, including random, premiere date, critic rating, play count, is-played, is-favourite, official rating, bitrate, studio, album, album artist, artist, series sort name, date last content added, index numbers.
- **Assessment:** **Portico behind** on breadth, **ahead** on cost per sort. Release date and random are the two most people will look for (F-07.2, F-07.4). Backlog already lists sort-by-plays.

### A-07.34 The alphabet rail
- **Portico:** the server returns `positionIndex`, the row index where each letter starts, from maintained counts; the client jumps by position. Letters are whatever scripts the library contains; non-letters are `#`.
- **Jellyfin:** the client sends `NameStartsWith=K` (or greater/less-than) and gets a filtered list.
- **Why it matters:** Portico's rail scrolls one continuous list; Jellyfin's replaces the list with a subset.
- **Assessment:** **Portico ahead.** Edge: a library with many scripts publishes one anchor per distinct first letter, which for Chinese or Korean titles can be thousands of single-title "letters". I did not find a cap.

### A-07.35 Filter option lists (facets)
- **Portico:** counts per value, per library, per restriction key, maintained (`catalog_facet_counts`), with a library-wide `GROUP BY` fallback on a cache miss (`browse_facets.go:130-222`). The counts do not narrow when other filters are active.
- **Jellyfin:** distinct values under the *current* query, no counts.
- **Assessment:** **Trade-off.** Portico shows numbers and is cheap; Jellyfin's list reflects what is still choosable. See F-07.18.

### A-07.36 Search matching
- **Portico:** FTS5; every token is a prefix; all tokens must match; **title only**; diacritics folded; 2-character minimum.
- **Jellyfin:** substring `LIKE` on the cleaned name or the original title; no minimum; finds mid-word matches; scans.
- **Navidrome:** FTS5 over many columns, with normalised and CJK fallbacks.
- **Why it matters:** Portico's is the only one of the three that stays fast on millions of rows for selective queries, and the only one that cannot find "Spirited Away" by its original title, a song by "artist title", "Alien 1979", or "M*A*S*H" at all (B-07.6, B-07.7, F-07.5).
- **Assessment:** **Portico ahead** on engine, **behind** on what is indexed and on query preprocessing.

### A-07.37 Search ranking
- **Portico:** bm25 + 1000 for an exact title + 100 for a title prefix (`search.go:154-159`). No popularity, rating, recency or play-history signal.
- **Jellyfin:** four tiers (exact, word-boundary prefix, prefix, contains), then sort name.
- **Navidrome:** bm25 with per-column weights; exact token beats prefix.
- **Assessment:** roughly **equal**. bm25 over a one-field document mostly measures title length.

### A-07.38 Search response shape
- **Portico:** nine named groups, each with its own items, exact total, cursor, status and error code; a slow group reports `search_group_timeout` without failing the others; per-viewer and global admission caps (`httpapi/search.go:113-124`).
- **Jellyfin:** one flat list of hints with a total.
- **Assessment:** **Portico ahead.**

### A-07.39 Sort-key rules
- **Portico:** per-language article lists for seven languages, chosen by the title's metadata language; Unicode case folding; combining marks stripped; explicit sort title honoured exactly. No punctuation stripping, no number padding, not configurable (`sorttext/sort.go`).
- **Jellyfin:** English articles only, removed at start, middle and end; punctuation removed or spaced; digits zero-padded to ten; all configurable; not language-aware; diacritics kept in the sort name.
- **Examples of where they differ:** "Les Misérables" (fr) → Portico M, Jellyfin L. "Rocky 10" vs "Rocky 2" → Portico puts 10 first, Jellyfin 2 first. "(500) Days of Summer", "'71", "...And Justice for All" → Portico files under `#` at the very top by the punctuation character; Jellyfin under 5 / 7 / A. "The Office" in a library whose metadata language is German → Portico T (German list has no "the"), Jellyfin O.
- **Assessment:** **Trade-off.** Portico is better for non-English libraries; Jellyfin is better on numbers and punctuation. Both rules could coexist (F-07.10, F-07.11).

### A-07.40 Ordering of non-Latin text
- **Portico:** code-point order of the folded key (SQLite `NOCASE`/binary). No locale collation, no transliteration (pinyin, kana, romanisation). "Østre" sorts after Z; Cyrillic after Latin; CJK by code point.
- **Jellyfin:** the same in SQL (ordinal). Plex offers romanised sort for some scripts (public documentation; not verified here).
- **Assessment:** **Equal** with Jellyfin. The contract's cursor rule mentions binding "locale/collation"; today there is one collation, so nothing needs binding.

### A-07.41 Parental restrictions in queries
- **Portico:** each browse row carries a content-rating key and a label key; counts are stored split by those keys; for a restricted profile a *visibility class* (a materialised, pre-filtered copy of the rows with its own blocks and counts) is built in the background and read directly, with a per-row recheck (`browse_pages.go:226-245`).
- **Jellyfin:** a numeric `InheritedParentalRatingValue` column compared per query, plus tag include/exclude lists.
- **Assessment:** **Portico ahead** on read cost; it stores one extra copy of the browse rows per distinct restriction policy per library, which is a storage and write cost that grows with the number of differently restricted profiles.

### A-07.42 Dates and times in storage
- **Portico:** mostly text (`added_text` RFC 3339 with milliseconds, `release_date` text, `last_played_at` text), one integer mirror (`added_ms`), episode `air_date` as an integer. Comparisons are string comparisons.
- **Jellyfin:** `DateTime` columns throughout (it needed a `FixDates` routine).
- **Assessment:** **Portico behind** in uniformity; it is the root of B-07.8.

### A-07.43 Backup of the database
- **Portico:** online backup API in 16-page steps on a pinned snapshot, fsync, `quick_check` of the copy (`backup/copy.go`).
- **Jellyfin:** truncating checkpoint then a plain file copy (for migrations); a separate backup service produces archives.
- **Navidrome:** online backup API in one step.
- **Assessment:** **Portico ahead** on correctness (a plain file copy of a live WAL database is only safe if nothing writes in between). See B-07.13 for the cost of the pinned snapshot on a large, busy database.

### A-07.44 Orphaned vocabulary and people
- **Portico:** `catalog_people`, `catalog_terms`, `catalog_attribute_terms`, `catalog_credit_labels` and `catalog_identities` are insert-only (no `DELETE` in the server).
- **Jellyfin:** `PeopleValidationTask`, `IsDeadPerson/Genre/Studio/Artist` query flags, clean-up routines.
- **Assessment:** **Portico behind**, low impact until churn accumulates.

### A-07.45 Removing a library
- **Portico:** no route removes a library. `web/src/screens/server/Libraries.tsx:305` notes the control "returns with DELETE /v1/admin/libraries/{id}". `catalog_libraries.retired` exists but nothing sets it. Many foreign keys to entities have no cascade (seasons→show, episodes→show/season, albums→artist, songs→album, files→book, `parent_id`), so removal has to be an ordered, batched background job.
- **Jellyfin:** supported.
- **Assessment:** **Portico behind**; not in `Backlog.md`. See F-07.14.

### A-07.46 The owner's play history
- **Portico:** `play_history` keeps who, what, device, when, how far and whether the server converted it, with names copied in so rows outlive titles, accounts and devices (`0007_play_history.sql`).
- **Jellyfin:** an activity log and per-user play counts; detailed history needs a plugin.
- **Assessment:** **Portico ahead.**

### A-07.47 Guard rails in tests
- **Portico:** every SQL statement is prepared against the schema in tests and full scans of large tables are rejected outside an allowlist; write amplification, trigger plans and per-item seek plans have their own tests (`persistence/statements_scan_test.go`, `trigger_plan_test.go`, `compactcatalog/per_item_seek_test.go`).
- **Jellyfin / Navidrome:** Navidrome has an index-coverage test for sorts; Jellyfin has ordinary unit tests.
- **Assessment:** **Portico ahead.** I did not run them.

### A-07.48 Public id format and third-party ecosystem
- **Portico:** 22-character base64url ids for entities, 43-character tokens for assets, 32-hex for people, text tokens for playlists. Four id formats.
- **Jellyfin:** GUIDs everywhere.
- **Assessment:** trivial technically; noted because a mixed id vocabulary is friction for anyone writing a tool against the API.

### A-07.49 Time-bounded cursors
- **Portico:** every cursor expires 30 minutes after the first page (`BrowseCursorTTLSecond`, `browse_contract.go:22`; continuation pages keep the original expiry, `browse_engine.go:495`).
- **Jellyfin:** nothing to expire.
- **Why it matters:** a grid left open over dinner cannot load its next page; the client restarts the list.
- **Assessment:** **Trade-off (subjective).** The revision binding already protects correctness; the time limit mostly protects signing-key rotation.

## Missing features and capabilities to consider

### F-07.1 Keep a viewer's state when a title is deleted and comes back
- **What:** on delete, park watched/progress/rating/favourite/watchlist/play counts under the title's public id (which the ledger already preserves) instead of cascading them away; re-attach when the same identity, or a title with the same provider id, is catalogued again; expire after a retention period.
- **Who has it:** Jellyfin (`ItemPersistenceService.cs:94-132,222-300`, 90-day clean-up). Navidrome avoids the problem with tag-derived ids.
- **Value:** survives "forget and rescan", a library re-created from another folder, and a move to a new disk if matching is by provider id.
- **Size:** medium (one table, two code paths, a retention sweep). No conflict with principles.
- **Opinion:** worth doing before release; losing everyone's watch history is the kind of event people remember.

### F-07.2 Random order that pages
- **What:** a "shuffle" sort for grids.
- **Who has it:** Jellyfin (`ORDER BY random()`, which re-rolls per page so pages overlap). Plex has it.
- **How it could fit Portico:** a seed carried in the cursor and an order of `hash(seed, entity_id)`; stable across pages and compatible with signed cursors. It has no counted blocks, so it is offset-or-keyset over a computed key (a full pass per page unless a per-row random column is stored and the seed only rotates the starting point).
- **Size:** small to medium. Backlog notes "shuffle is linear in container size" for playback queues, which is a related cost.
- **Opinion:** common request; do the stored-random-column variant so it stays O(visible).

### F-07.3 Librarian filters
- **What:** has/lacks subtitles (and in which language), lacks an audio language, video codec, audio codec, container, has no provider match (today `match` covers albums only), has no poster/backdrop, has no overview, is locked, file size, bitrate, play count, series status.
- **Who has it:** Jellyfin (`InternalItemsQuery`: `HasSubtitles`, `HasNoAudioTrackWithLanguage`, `HasTmdbId`, `ImageTypes`, `HasOverview`, `IsLocked`, `VideoTypes`, `SeriesStatuses`, …).
- **Value:** these are how an owner finds what needs fixing. Portico's attribute-edge design makes each one a new attribute field plus a row in `browseFields`.
- **Size:** small each. **Opinion:** add the "needs attention" set (unmatched, no artwork, no subtitles in my language).

### F-07.4 More sorts
- Release date (premiere date) for films, critic rating, play count (Backlog has "Sort Songs by plays"), content rating, file size, bitrate/resolution, album artist, track order within an album grid, date modified. Jellyfin has all of them (`OrderMapper.cs`). In Portico each stored sort that should jump by position needs a sixth, seventh… block ordering (ten more generated triggers each), so choose few. **Opinion:** release date first.

### F-07.5 Search more than the title
- **What:** index original title, sort title, alternative titles, and for songs and albums the artist and album; let a 4-digit token match the year; let "show name s02e03" find an episode.
- **Who has it:** Jellyfin searches original title as well as name. Navidrome indexes eleven columns per track with weights.
- **Evidence of the gap:** `deriveSearch` stores only `e.title` (`compactcatalog/derive_search.go:15-30`).
- **Size:** medium (more FTS columns, per-column weights, a derivation version bump). **Opinion:** high value; "artist + song" and "title + year" are how people actually type.

### F-07.6 Punctuation-insensitive search and one-letter titles
- **What:** a normalised column so "mash", "swat", "acdc", "walle", "et" find "M*A*S*H", "S.W.A.T.", "AC/DC", "WALL·E", "E.T."; accept one-character queries when they equal a whole title ("X", "M", "W.").
- **Who has it:** Navidrome (`search_normalized`, dotted-abbreviation phrases; `sql_search_fts.go:52-61`). Jellyfin's `LIKE` on a cleaned name has no minimum length.
- **Size:** small. **Opinion:** do it with F-07.5. See B-07.6.

### F-07.7 Search inside Chinese, Japanese and Korean titles
- **What:** FTS5's `unicode61` treats an unspaced run of CJK characters as one word, so only prefix matches work. Options: the `trigram` tokenizer in a second index, or a `LIKE` fallback when the query contains CJK (Navidrome's choice, `sql_search_fts.go:18-20`).
- **Size:** small (fallback) to medium (trigram index roughly triples index size). **Opinion:** the fallback is cheap insurance for anime libraries with native titles.

### F-07.8 More things to find
- Collections, playlists, authors, book series, genres/tags, and live-TV programmes are not search groups (`search.go:38-48`; `catalog_kinds` marks collection and author not searchable). Jellyfin's search covers playlists, box sets and programmes.

### F-07.9 Typo tolerance
- None of the open-source products has it; Plex does (public behaviour). SQLite offers `spellfix1` and trigram similarity. **Opinion:** later; F-07.5 and F-07.6 fix far more failed searches.

### F-07.10 Natural number order in sort keys
- Pad digit runs in the derived sort key so "Rocky 2" precedes "Rocky 10" and "2 Fast 2 Furious" precedes "10 Things I Hate About You". Jellyfin does this (`BaseItem.ModifySortChunks`). Size: small plus a derivation rebuild. **Opinion:** do it before release, because changing sort keys later means rebuilding rows and blocks for every library.

### F-07.11 Sort-key punctuation rule and more article languages
- Skip leading punctuation ("(500) Days of Summer", "'71", "…And Justice for All"); add Scandinavian and other article lists; consider an owner setting. Jellyfin's lists are settings (`ServerConfiguration.cs:115-127`).

### F-07.12 Missing and unaired episodes
- Rows for episodes the provider lists but the library lacks, with an `isMissing`/`isUnaired` filter, a per-show "show missing" switch and counts. Jellyfin has it (`IsVirtualItem`). In Portico's model this is an episode entity with no asset link, which the availability derivation already understands. Size: medium. **Opinion:** a frequent ask from TV collectors; decide whether it is in scope.

### F-07.13 Collections across libraries
- See A-07.11. Size: medium to large.

### F-07.14 Remove a library
- A retire flag that hides the library at once (`catalog_libraries.retired` exists and search already tests it), then a background job that deletes entities child-first in bounded batches and finally the library row. Size: medium. **Opinion:** required before release; today an owner cannot undo "Add library".

### F-07.15 Space reclamation and a size report
- Either `auto_vacuum=INCREMENTAL` from the first release with a slow background `incremental_vacuum`, or an owner-triggered compaction that copies the database with `VACUUM INTO` and swaps it at restart (which fits the existing staged-restore machinery). Plus a per-area size breakdown (`persistence/storage_breakdown_test.go` suggests one exists for tests). Jellyfin runs `VACUUM` on a schedule.

### F-07.16 Automatic database copy before a migration
- Before applying any pending migration, take a backup-API copy (or reuse the newest scheduled backup if recent), keep it until the next successful start, and say where it is. Jellyfin does this and restores on failure. Size: small. **Opinion:** cheap protection against a migration that is wrong rather than failing.

### F-07.17 Owner-facing "check and repair derived data"
- Expose what already exists for tests: `CheckBrowseBlocks`, a recount of buckets against rows, FTS `integrity-check`, and `Worker.Rebuild(domain)` (`derive.go:223-229`, "tests and diagnostics"). Navidrome ships a repair command. **Opinion:** the more the design depends on trigger-maintained counters, the more this matters; a wrong block total today shows up as a grid that skips or repeats rows with no way for an owner to fix it.

### F-07.18 Facet counts that follow the active filters
- After choosing "2020s", show how many of each genre remain. Portico's maintained counts are per library only; a contextual count is a `GROUP BY` over the filtered set (bounded by the filter's matches). Size: medium. **Opinion:** nice, not essential.

### F-07.19 Filter by person, artist and studio identity
- See A-07.32. Size: small.

### F-07.20 Clean-up of unused people, terms and ledger rows
- A background sweep that deletes people with no credits (and their FTS rows), terms with no entities, and ledger rows for identities that have been gone longer than the retention in F-07.1. Jellyfin has `PeopleValidationTask`.

### F-07.21 A scheduled integrity check that is on by default
- Weekly `quick_check` in a quiet period, result surfaced in the owner console rather than only in the log. The code exists (`cmd/server/maintenance.go:141-150`); it is gated on an environment variable.

### F-07.22 Smart playlists
- Navidrome compiles criteria to SQL for playlists. Portico's filter tree plus saved views is the same capability under another name; if "smart playlist" or "smart collection" is wanted as a product term, the engine is already there. No work unless the product wants the noun.

## Possible bugs and risks in Portico

None of these was reproduced. "Confirmed by reading" means the code path is unambiguous; whether it bites depends on runtime conditions I state.

### B-07.1 One permanently failing entity stops all second-tier derived workers
- **What:** the worker runs the step workers (visibility classes, personal sort buckets, collection/category/book visible counts, content eligibility, recommendation profiles) only when no domain has queued work: `Step` selects a domain "WHERE rebuilding=1 OR EXISTS(dirty row)" and calls `runSteps` only on `sql.ErrNoRows` (`compactcatalog/derive.go:245-255`). An id whose derivation always fails is "poisoned" in memory but keeps its `catalog_dirty` row. `drainDomain` skips poisoned ids only while other ids exist; when they are all that is left it reads them again (`:322-326`), the derivation fails, `Step` returns an error, and the loop sleeps five seconds and repeats (`:181-184`). Because the dirty row never leaves, the `ErrNoRows` branch is never reached again.
- **Effect:** restricted profiles' pre-filtered views, personal sorts, collection visible counts, eligibility counts and recommendation taste stop updating, indefinitely, with a log line every five seconds. Restarting does not help; the row is still queued.
- **Same shape, milder:** during any long drain (a first import of a large library, or a derivation rebuild) the steps do not run until the queue is empty, so those views lag by the length of the import.
- **How the comparison surfaced it:** Jellyfin computes these at query time and cannot stall this way; I went looking for what happens when Portico's pipeline hits a bad row.
- **Confidence:** Confirmed by reading (control flow). It needs a deterministic failure in some derivation for some entity to trigger. **Severity:** High if triggered, because nothing self-heals; Medium for the lag variant. **Backlog:** not listed.

### B-07.2 Raising a derivation version takes browsing or search offline for the whole rebuild
- **What:** `prepare` sets `rebuilding=1` when the code's version is newer (`derive.go:199-221`). `browseReady` answers `ErrBuilding` while any of domains 18, 19, 20, 25, 26, 28 is rebuilding (`catalog/compact_browse.go:6-8`); search does the same for 17 (`search.go:480-487`). The flag is per domain, not per library, and clears only when the backfill has walked every entity.
- **Effect:** after an upgrade that changes how browse rows (or search documents) are computed, every library's grid (or search) shows "being prepared" until the rebuild finishes. At millions of entities that is hours. The old rows are still there and still correct by the old rules.
- **Docs:** `persistence/README.md` rule 3 says the rebuild happens "while the old rows keep serving". `03 Catalogue and Metadata.md` says readers "answer ErrBuilding until it catches up". The code does the second.
- **How the comparison surfaced it:** Jellyfin's 10.11 upgrade is known for hours-long blocking migrations; I checked what Portico's equivalent moment looks like.
- **Confidence:** Confirmed by reading for the mechanism; duration is an estimate, not measured. **Severity:** High at target scale (it is the upgrade experience), Low on small libraries. **Backlog:** not listed.

### B-07.3 A new file at a moved movie's old path is attached to the moved movie
- **What:** a movie's ledger key is its path relative to the library root (`compactcatalog/write.go:85-95`), and the ledger is never updated or deleted. When file A moves from path P to Q on the same disk, the scanner keeps A's asset and item (`catalog/inventory.go:203-246`); the ledger still says P → that item. If a different file B later appears at P, B gets a new asset, is "not associated", and `commitMovieItem` (`catalog/catalog.go:496-515`) calls `EnsureEntityTx` with key `file:P`, which finds the *existing, live* entity (A's movie) and links B to it as another file of the same title.
- **Effect:** two different films under one title; B has no title of its own. Likely when files land under a generic or reused name and are then renamed/moved into place ("Inbox/movie.mkv", "New Folder/video.mkv", a downloader's fixed temp name inside the library root). The same applies to music and audiobook files (`audio_repository.go:287`).
- **How the comparison surfaced it:** Jellyfin's id is the path hash, so a new file at an old path is a new item unless the old one is still there; I traced what Portico's ledger does in the same situation.
- **Confidence:** Probable (each step read; the interaction not run). **Severity:** Medium. **Backlog:** not listed (the related "second file of the same title becomes a second title" item is the opposite problem).

### B-07.4 Cursors and range revisions go stale on every progress report and every scan commit
- **What:** a cursor is refused unless both stored revisions equal the current ones (`catalog/discovery.go:161-163`). `viewer_revisions` is bumped by a trigger on every insert or update of `progress_activity` (`0001_baseline.sql:3946-3948`), which the playback progress path writes on each report (`playback/service.go:637`, `catalog/personal_activity.go:138`). `library_revisions` is bumped by triggers on almost every catalogue table.
- **Effect (a):** a profile that is playing something from library L invalidates that profile's cursors for L at the progress-report interval. The plain case is music: listening while scrolling Songs, Albums or a filtered view. Unfiltered single-sort grids recover through `range` + `anchorId`; filtered views, multi-sort views and personal sorts cannot (`browse_engine.go:579-582` returns `ErrStaleContinuation` for shapes without blocks), so "load more" fails and the list restarts from the top.
- **Effect (b):** while a library is being scanned or its metadata refreshed, the same happens to every viewer of that library for the duration. `consistentRead`'s own comment says "Background metadata and artwork work bumps library revisions constantly on a busy server" (`catalog/content.go:514-518`).
- **Effect (c):** every revision-keyed memo (facet cache, anchor cache, filtered totals) misses at the same rate, so each first page of a filtered view re-counts the pivot.
- **How the comparison surfaced it:** Jellyfin's offset paging never refuses a page; I asked what the price of Portico's stricter contract is in practice.
- **Confidence:** Confirmed by reading for the mechanism; how visible it is depends on the progress interval and on client screens, which I did not run. **Severity:** Medium (design risk). **Backlog:** not listed. Options: bump the viewer revision only when a fact a list can show changes (watched flips, not every position tick); keep a separate revision for "membership or order changed" versus "decoration changed"; let filtered cursors carry on by key after a revision change when the sort is a stored column.

### B-07.5 A search can fail with "the library changed" although it read one consistent snapshot
- **What:** `Search` runs inside one read snapshot (`httpapi/home.go:163-173`), but its before and after revision checks call `searchRevision(ctx, s.db, …)` on the raw database handle, outside the snapshot (`catalog/search.go:274`, `350-356`). If any library in scope commits a change while the nine groups run, the after-check differs and the whole response is `409 stale_continuation`, first page included. Unlike `Content`, `Detail`, `Home` and the show workspace, `Search` is not wrapped in `consistentRead`. Client-core then clears the results and shows "The library changed. Refresh these results." (`packages/client-core/src/search.ts:175-178`).
- **Effect:** search is unreliable exactly while a scan or metadata refresh is running, which on a new server is the first hours or days. The two extra reads also each take a pooled connection the admission accounting does not see.
- **Confidence:** Confirmed by reading; frequency depends on commit rate during scans. **Severity:** Medium. **Backlog:** not listed. Fix is small: read the revision through the snapshot (then the after-check is unnecessary) or retry first pages.

### B-07.6 Some titles cannot be found by search at all
- **What:** `NormalizeSearch` splits on anything that is not a letter or digit, makes every token a prefix term, and rejects the query unless some token has two or more characters (`search.go:114-135`). The index tokenizes titles the same way.
- **Examples:** "M*A*S*H" → tokens m, a, s, h → rejected as too short; "MASH" → one token that matches nothing, because the indexed tokens are single letters. Same for "S.W.A.T." / "swat", "E.T." / "et", "R.E.M." / "rem", "AC/DC" / "acdc" (finds nothing; "ac dc" works), "WALL·E" / "walle". One-character titles ("X", "M", "Q", "Z") are rejected by the 2-character minimum.
- **How the comparison surfaced it:** Navidrome has explicit code for dotted abbreviations and a punctuation-stripped column; Jellyfin's substring match on a cleaned name has no minimum.
- **Confidence:** Confirmed by reading (tokenizer behaviour reasoned from FTS5 `unicode61` rules, not run). **Severity:** Medium. **Backlog:** not listed.

### B-07.7 Search finds Chinese, Japanese and Korean titles only from their first character
- **What:** `unicode61` makes an unspaced CJK run one token; Portico's terms are prefixes; so "神隠し" does not find "千と千尋の神隠し". A one-character CJK query is rejected by the length rule.
- **Confidence:** Confirmed by reading plus Navidrome's documented experience of the same tokenizer (`navidrome/persistence/sql_search_fts.go:18-20`). **Severity:** Medium for libraries with native-script titles (anime libraries with Japanese metadata), none otherwise. **Backlog:** not listed.

### B-07.8 Date filters compare raw text
- **What:** date fields accept `YYYY-MM-DD` or RFC 3339 (`browse_expression.go:267-273`) and are compiled to plain comparisons on the stored text (`:362-411`). `dateAdded` is stored as `2026-10-05T14:23:11.123Z` (`catalog/discovery.go:90-92`).
- **Effect:** `dateAdded equals "2026-10-05"` never matches. `between ["2026-10-01","2026-10-05"]` and `at-most "2026-10-05"` leave out everything added on the 5th. A value with a time-zone offset is compared character by character against UTC text. `lastPlayedAt` and `releaseDate` behave the same way with their own stored formats.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (wrong results, quietly; affects saved views and date-range controls). **Backlog:** not listed.

### B-07.9 The seek-by-letter request breaks on non-ASCII letters
- **What:** `wanted := strings.ToUpper(request.Seek.Prefix[:1])` takes the first *byte* (`browse_engine.go:556`). For "Д", "É" or "千" that is half a character, no anchor matches, `start` stays at `total` and is clamped to the last row. The server publishes anchors for those letters (`sorttext.Letter` accepts any script), so the API advertises jumps it cannot perform.
- **Confidence:** Confirmed by reading. A text search of the web and native app sources found no caller that sends `seek` (they appear to jump by the `positionIndex` row index instead; I did not trace this), so today it looks latent. **Severity:** Low. **Backlog:** not listed.

### B-07.10 The client's letter-bucket match disagrees with the server's letters
- **What:** `bucketIndexFor` maps any first character outside A–Z to `#` and uses the value's raw first character (`packages/client-core/src/browse-buckets.ts:52-58`). The server's letters come from the folded sort key: article removed, accents stripped, any script's letter kept.
- **Effect:** when a client re-anchors after a publication, "The Matrix" looks under T instead of M, "Élite" under `#` instead of E, any Cyrillic/Greek/CJK title under `#`.
- **Confidence:** Confirmed by reading the function; I did not trace what value callers pass (if they pass the server's sort key the article case disappears). **Severity:** Low. **Backlog:** not listed.

### B-07.11 `DELETE /v1/items/{id}` is a raw entity delete
- **What:** the owner route (`httpapi/router.go:1110-1120`) calls `catalog.Service.Delete` (`catalog/catalog.go:328-350`), which resolves any entity id and calls `DeleteEntityTx`. For a show, season, album, artist or book with children the `DELETE` violates a foreign key that has no cascade (for example `catalog_seasons.show_id`, `0001_baseline.sql:129`) and the request fails as a database error. For a movie it deletes the entity and every viewer's state for it but leaves the file's asset and inventory record, so the next scan finds an unassociated file and creates the movie again under the same public id, without the history. It runs on `context.Background()`, so it has no deadline and ignores the request's class.
- **Confidence:** Confirmed by reading for the delete and the FK; Probable for "recreated at next scan". **Severity:** Low to Medium (owner-only; the proper path is `PurgeItemTx`). **Backlog:** not listed.

### B-07.12 Case and accent handling is ASCII-only in filters
- **What:** filters use `COLLATE NOCASE` and `LIKE` (`browse_expression.go:343-349,362-383`), both of which fold A–Z only in SQLite, on the raw title or value. Search's exact and prefix boosts use the same (`search.go:157-159`).
- **Effect:** `title contains "amelie"` does not match "Amélie" (search does). `genre equals "comédie"` does not match "Comédie". "ÉLITE" typed in capitals gets no exact-title boost.
- **Confidence:** Confirmed by reading. **Severity:** Low. **Backlog:** not listed.

### B-07.13 The backup pins one snapshot for the whole copy
- **What:** `copyDatabase` opens a read transaction and keeps it until the last page is copied (`backup/copy.go:35-41`, loop at `:66-99`), 16 pages per step. While it is open no checkpoint can recycle log frames written after it. The hard-pressure path can pause *new* readers for 250 ms but cannot make this one finish.
- **Effect:** on a multi-gigabyte database being backed up during a scan, the `-wal` file grows by everything written during the backup, on the same volume, and readers slow down as the log lengthens. Afterwards `quick_check` reads the whole copy.
- **Confidence:** Speculative; needs a runtime check at scale (the mechanism is standard SQLite behaviour; the size reached is unknown). **Severity:** Medium at target scale. **Backlog:** not listed. A scheduled backup could wait for scan idle, or the copy could use `VACUUM INTO`, which also compacts.

### B-07.14 Migrations 0003, 0007, 0008 and 0010 do what the migration rules forbid
- **What:** `0003_drop_reaction.sql` runs `ALTER TABLE personal_items DROP COLUMN` (SQLite rewrites the table; `personal_items` is profiles × items). `0007` aggregates all of `personal_history` at startup. `0008` walks every show's metadata fields with `json_each`. `0010` updates metadata work rows library-wide. README rules 1 and 2 say never drop a column of a large table and never let a migration's cost grow with the library.
- **Effect today:** none that matters; the only databases are the demo and development ones. The risk is precedent: these are the examples the next migration will be modelled on, and nothing in the test suite appears to enforce rules 1–2 (`TestMigrationsApplyAfterHigherNumbers` covers ordering only, per `migrations.go:32`).
- **Confidence:** Confirmed by reading. **Severity:** Low now. **Backlog:** not listed.

### B-07.15 Insert-only tables grow without bound
- **What:** `catalog_identities` keeps a row per identity ever seen, each repeating the library root path as text and indexed twice (`0001_baseline.sql:57-62`); `catalog_people`, its FTS index, `catalog_terms`, `catalog_attribute_terms` and `catalog_credit_labels` are never pruned. People search counts by testing every name match for a visible credit (`search.go:738`).
- **Effect:** slow growth; at 10 million identities the ledger alone is on the order of gigabytes (estimate from row shape, not measured). A side effect of never pruning: a different film placed at a long-forgotten path receives the old film's public id, which a client cache or a download record may still associate with the old title.
- **Confidence:** Confirmed by reading; sizes estimated. **Severity:** Low. **Backlog:** not listed.

### B-07.16 Nothing reports a stuck write gate in production
- **What:** the "held for more than 20 s" detector that names the holder is active only when `PORTICO_DBWORK_STRICT=1` (`dbwork/gate.go:87-94`, `traceHolders`). In production a gated transaction that never finishes (for example a helper that calls a gated write with a fresh context while its caller holds the gate, which would wait on itself) leaves every later writer queued until its own deadline, with the cause visible only as `currentHeldMillis` in a diagnostics document.
- **Confidence:** Speculative as to occurrence (I found no such call path; `catalog` and `compactcatalog` contain about 25 uses of `context.Background()` that I did not all trace). Confirmed that no alarm exists. **Severity:** Low to Medium (latent trap). **Backlog:** not listed. A log line and a health-state change after N seconds of hold would cost nothing.

### B-07.17 Search sort quirks
- **What:** `releaseYear` for shows and books orders by a constant (`search.go:164-168`), so those groups come back in public-id order while advertising the sort. `title` sort orders by `lower(d.title)`, not the sort key, so "The Matrix" files under T in search and under M in browse, and non-ASCII capitals sort apart (`:162-163`).
- **Confidence:** Confirmed by reading. **Severity:** Low. **Backlog:** not listed.

### B-07.18 Derived batches may hold the write gate for long stretches
- **What:** the worker commits up to 64 units per transaction in the maintenance class (`derive.go:44`, `233-272`). The architecture doc gives 6–13.5 ms per entity for cataloguing. If a browse-row unit costs a few milliseconds including its trigger chains (ten block triggers, each with an `OFFSET 1024` probe when a block splits), a batch holds the single writer for a few hundred milliseconds, during which a progress write or a playback start waits.
- **Confidence:** Speculative; needs measurement (`GateStats.MaxHeldMilli` by class is already collected). **Severity:** Low to Medium. **Backlog:** not listed.

### B-07.19 Opening the live database in another program
- **What:** "open data" invites owners to open `server.sqlite` in a viewer. A second process holding a read transaction pins the log exactly like B-07.13; one that starts a write blocks Portico's writer for up to the 15 s busy timeout per attempt, three attempts.
- **Confidence:** Speculative (standard SQLite behaviour). **Severity:** Low. A sentence in the owner documentation ("open a backup copy, not the live file") would cover it.

## Where Portico is stronger

- **Read cost that does not grow with the library** for the main shapes: totals, the letter rail, deep position jumps and restricted-profile grids come from maintained counts and blocks. Jellyfin counts and offsets on every request.
- **Write ordering by meaning.** A sign-out or a progress update goes ahead of a scan batch by design, not by luck or retry.
- **One snapshot per page**, so a response never mixes two states.
- **Write-ahead log control** that is measured, bounded and tested. Jellyfin sets a size limit and hopes.
- **Small integer keys internally, opaque ids externally.**
- **Titles separate from files**, so personal state belongs to the title.
- **Provenance on genres and credits**, which makes "one provider per library" switchable without guesswork.
- **Paging that cannot skip or repeat**, bound to viewer and query, with anchored relocation after a change.
- **A filter language** (boolean tree, published capabilities, one compiler shared by grids, saved views, Home rows and Library Channels).
- **Grouped search** with per-group budgets and honest partial failure; FTS rather than table scans.
- **Language-aware articles and Unicode folding** in sort keys.
- **Backups verified before they are listed.**
- **Migration discipline**: digests, refusal of newer and pre-baseline databases with a plain message, written rules.
- **Per-class durability** for security-relevant writes.
- **Test guards** against full scans, quadratic derivations and trigger surprises.
- **An owner's play history that survives deletions.**

## Doc/code disagreements

1. **Number of migrations.** `02 Server.md` says "one baseline migration"; `persistence/README.md` says that until the first release "a schema change edits `0001_baseline.sql` directly". The tree has migrations 0002–0011 and `schemaVersion = 11` (`persistence/schema.go:24`).
2. **Serving during a rebuild.** README rule 3: "the worker rebuilds it in the background … while the old rows keep serving". Code and `03 Catalogue and Metadata.md`: readers answer `ErrBuilding` (B-07.2).
3. **Migration rules versus migrations.** README rules 1–2 versus `0003` (drop column), `0007`, `0008`, `0010` (library-sized work at startup) (B-07.14).
4. **Reserved kind numbers.** README: 14–49 future media kinds, 50–89 future containers and groups, 90–99 test fixtures. Baseline comment (`0001_baseline.sql:5-8`): 14–19 video/audio growth, 20–29 reading, 30–39 photos, 40–99 unassigned.
5. **Search budget.** `Backlog.md` ("Known costs"): "within the three-second budget". Code: `SearchGroupBudget = 600 * time.Millisecond` per group (`search.go:25`).
6. **"Cursors are revision-fenced offsets"** in the `browseSelect` comment (`browse_engine.go:379-382`); the code seeks by key where it can (`browse_pages.go:95-113`).
7. **`architecture.md`** says cursors bind "locale/collation". `cursorScope` has no such field (`catalog/discovery.go:36-49`); there is one collation, so nothing is wrong, but the contract promises more than exists.
8. **"No time-based caches of derived results."** The facet cache entry carries `expires: time.Now().Add(time.Minute)` (`browse_facets.go:233`). It is also keyed by revision, so the minute only evicts; it does not serve stale data. Worth a comment so it is not mistaken for a violation.
9. **`ErrSchemaResetRequired`** message and README agree; noted as correct.

## Open questions for Justin

1. Should a viewer's history survive a title being deleted and re-added, or a library being moved to another disk (F-07.1)? If yes, keyed by Portico's own identity only, or also by provider id?
2. Is it acceptable that browsing or search shows "being prepared" for hours after an upgrade that changes a derivation, or should old rows serve until the new ones are ready (B-07.2)? The second needs either versioned rows or a shadow table per rebuild.
3. How coarse should revisions be? Should a playback position tick invalidate that profile's lists, and should one artwork fetch invalidate a library's cursors (B-07.4)?
4. Is `VACUUM`-style space reclamation wanted, and if so by `auto_vacuum=INCREMENTAL` (must be decided before the first release database exists) or by an explicit owner action (F-07.15)?
5. Should the database have any owner-visible tuning (cache size at least), or stay fixed (A-07.21)?
6. Search scope: title only by design, or should original titles, artists and years match (F-07.5)? And is CJK substring search in scope (F-07.7)?
7. Sort keys: add number padding and leading-punctuation skipping before release (F-07.10, F-07.11)? Changing them later rebuilds every library's rows and blocks.
8. Are missing/unaired episodes in the product (F-07.12)?
9. Should collections be able to span libraries (A-07.11)?
10. When will "Remove library" exist, and what should happen to viewers' state for its titles (F-07.14)?
11. Should the server copy the database automatically before applying a migration (F-07.16)?
12. Should the integrity check be on by default and visible in the console (F-07.21)?
13. Is the 30-minute cursor lifetime intended as a product limit (A-07.49)?
14. Do the migration rules apply now or from the first release? If from release, say so in the README; if now, 0003/0007/0008/0010 are exceptions worth a line each.
15. Is "browse by folder" permanently out of scope (A-07.9)?

## Evidence boundary

- **Nothing was executed.** No server, test, build or query was run. Every statement about behaviour is a reading of source.
- **Performance and scale are not established.** I did not measure derivation rates, rebuild times, gate hold times, WAL sizes, database sizes or query plans. Numbers such as "hours" and "gigabytes" are estimates from row shapes and the docs' own per-entity figures.
- **Trigger correctness was sampled, not proven.** I read the title-ordering block triggers and listed every trigger that queues derived work or bumps a revision; I did not verify the generated block arithmetic for the other four orderings or for visibility classes, and did not look for fact changes that fail to dirty a derivation beyond checking that fact-table writes go through the write API (they do, except two `parent_id` updates in `catalog/audio_repository.go:429,522` that I did not follow).
- **FTS behaviour** in B-07.6 and B-07.7 is reasoned from SQLite's documented tokenizer rules and from Navidrome's comments about the same tokenizer, not from running a query.
- **Client behaviour** on stale cursors was read in `client-core` only; I did not check what each screen does with a `refresh-required` state.
- **Jellyfin** was read at the repository layer; I did not read `DtoService`, the item cache, `ItemCountService` or the migration routines' bodies, and did not verify the public reports about 10.11 against its issue tracker. Statements about Plex are from general public knowledge and are marked as such.
- **Not covered:** the non-catalogue 430 tables; `statement_cache.go`; `planner_stats.go` internals; `visibility_classes.go`; `derive_facet_counts.go`; `internal/worker`, `internal/supervise`, `internal/catalogtest`; the OpenAPI files; Navidrome's `criteria_sql.go`; Plezy.

## Outside my domain

- **English baked into derived data and SQL.** Season browse rows are titled `<show> · Season N` in the derivation (`compactcatalog/derive_browse.go:54-60`, `refinedRowSQL`), and search subtitles are built in SQL as "Season N · Episode N", "Specials · Episode N", "Absolute episode N (unassigned season)" (`catalog/search.go:703-707`). If the server is meant to compose localised labels, these cannot be localised without a rebuild or a query change.
- **Incremental scan and replaced files, second file as second title**: already in `Backlog.md`; they interact with B-07.3.
- **Hosted (PostgreSQL)** persistence was not looked at.

Sources consulted on the web (public reports about Jellyfin 10.11, not verified against the issue tracker): [AlternativeTo news](https://alternativeto.net/news/2025/10/jellyfin-10-11-migrates-to-ef-core-boosts-speed-drops-32-bit-arm-support), [Jellyfin forum: "does 10.11.2 fix the slow database"](https://forum.jellyfin.org/t-does-10-11-2-fix-the-slow-database?pid=66635), [Jellywatch blog on the EF Core migration](https://jellywatch.app/blog/jellyfin-10-11-ef-core-migration-chaos-lessons-learned-2026), [GeekNews summary](https://news.hada.io/topic?id=24087).
