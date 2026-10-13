# Portico Media Presentation Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


**Contract ID:** `PC-MEDIA-PRESENTATION`  
**Contract revision:** `portico.reconciled.1`  
**Approval state:** Approved  
**Applies to:** Home, libraries, search, Saved, media detail, people, Live TV/DVR, audio Now Playing, downloads, and equivalent first-party surfaces

This contract defines the content elements, artwork roles, action hierarchy, metadata, rows, and state treatments that make the same media item recognizable across Portico clients. It is intentionally platform-agnostic. Layout mechanics may adapt to screen and input; content meaning may not.

## 1. Authority and composition

1. The server Product Contract owns entity kinds, hierarchy, default destination, primary artwork role, filter/sort vocabulary, action IDs, permissions, and capabilities.
2. This contract owns first-party presentation composition: which visual roles and information appear on Home, browse, detail, search, Saved, player-adjacent, and administration surfaces.
3. `../visual/contract.md` owns material, atmosphere, typography, focus, motion, form, and state styling.
4. Product Language owns user-facing labels and recovery copy.
5. A client MUST render only actions authorized for the current resource. A surface matrix can prescribe placement for an action; it cannot invent availability.
6. Missing optional metadata removes that fact without collapsing the stable surface scaffold or leaving punctuation artifacts.
7. API projections MUST remain surface-appropriate. A client MUST NOT fetch a full administrative media record merely to render a card.

## 2. Media-first principles

1. Artwork and title establish identity; concise metadata establishes the viewing/listening decision; actions follow.
2. Media detail surfaces MUST privilege the media over navigation chrome and technical evidence.
3. The same entity kind MUST use the same default artwork role, destination, title hierarchy, progress semantics, and action meaning across clients.
4. Home is an ordered set of server-advertised, viewer-authorized rows. Clients MUST preserve row order and presentation descriptors; they MUST NOT invent a separate per-platform catalogue.
5. One surface may contain different media families, but an ordinary Home, discovery, recommendation, search, Saved, or library shelf uses one poster or square geometry. Landscape/thumb cards are not an ordinary exploratory presentation.
6. Media type changes hierarchy, not design language. Movies, music, audiobooks, Live TV, and recordings remain recognizably Portico.
7. Consumer surfaces MUST NOT expose file paths, codecs, provider URLs, match evidence, scan internals, or transcode jargon unless the viewer deliberately opens Playback Info or an authorized administrative editor.

## 3. Artwork vocabulary

The Product Contract publishes these roles. Clients reserve geometry before bytes arrive.

| Role | Ratio | Fit | Purpose |
| --- | ---: | --- | --- |
| `poster` | 2:3 | cover | primary identity for movies, television/anime, audiobooks, recordings, people, and video/book collections |
| `square` | 1:1 | cover | primary identity for music artists, albums, tracks, and music collections |
| `backdrop` | 16:9 | cover | hero/detail/Now Playing environmental artwork |
| `thumb` | 16:9 | cover | episode/special/extra rows inside the containing show or anime detail workspace only |
| `logo` | 3:1 | contain | transparent title/channel mark; never an ordinary primary-card substitute |
| `banner` | 5.4:1 | cover | optional extra-wide promotion, not a default browse card |

Rules:

1. The entity's published `primaryArtworkRole` controls ordinary browse/search card geometry. Movies, television/anime, and audiobooks resolve to poster; music resolves to square.
2. A relationship surface MAY request a contextual role only inside the owning detail context: episode/special/extra rows inside a show or anime detail workspace use thumbnails when available; chapter rows use a contained inherited book cover or no-art list treatment; guide/programme cells use a separate time-grid treatment that is not a media-card artwork role; live-channel cells use contained channel logos; a media-led hero uses backdrop; an audio Now Playing surface uses square cover plus optional backdrop.
3. Outside a show detail workspace, an episode/special/extra uses the season poster when available and otherwise the show poster. Outside a book detail workspace, a chapter uses its book poster. This inheritance changes artwork only; the card retains the child entity's title, number, progress, action, and destination.
4. Contextual artwork MUST remain attached to the correct entity or declared inheritance relationship. A show backdrop may support an episode only when the episode has no backdrop and the relationship is explicit.
5. Fallback order is: requested local derivative, requested local original, permitted related-entity artwork for the same role, deterministic Portico fallback. Provider URLs never render directly.
6. A logo uses `contain` on a quiet surface with safe padding. It MUST NOT be cropped, stretched, or used as a full-bleed poster.
7. Artwork failure preserves aspect ratio and layout. A broken-image glyph is prohibited.
8. Artwork edited by the server owner replaces only the relevant role. Choosing a poster must not silently replace a backdrop.
9. Backdrop visibility obeys the viewer's appearance/privacy preference and the Visual Contract's media precedence rules.

## 4. Global page scaffold

Every consumer destination composes from these layers as applicable:

1. **Persistent context:** product navigation, profile, selected server, destination identity, and player reservation.
2. **Media context:** hero/backdrop or atmospheric fallback, title, parent context, essential metadata, progress/live state.
3. **Primary action group:** the most likely authorized play/resume/watch/listen action plus a bounded number of immediate state actions.
4. **Peer navigation:** pivots/tabs, season selector, library selector, or other same-level context.
5. **Content body:** ordered shelves, grid/list/table, episodes/tracks/chapters, cast/credits, related items, extras, or administrative sections.
6. **Transient tools:** filters, sorts, context actions, selection bar, playback choices, editor, or diagnostics.

A platform may move these layers, but MUST preserve their order of importance.

## 5. Home

### 5.1 Composition

1. Home displays the server-authoritative ordered row descriptors for the current viewer/server/profile.
2. Continue Watching/On Deck and Continue Listening retain their server-defined priority. A client MUST NOT fabricate one from stale unrelated history.
3. Each advertised row reserves its final header and card geometry before resolution.
4. First-viewport rows load eagerly; later rows load through bounded viewport activation.
5. One row failure remains local. Successful rows remain visible.
6. A resolved-empty row collapses only when doing so will not disrupt current scroll/focus; otherwise it retains neutral geometry for the visit.
7. Row customization is an explicit action. Reordering is never activated by ordinary focus or hover.

### 5.2 Hero behavior

1. A Home hero is appropriate when the server supplies an eligible hero or a strong resume/featured candidate.
2. A hero includes media identity, one concise metadata line, a bounded summary, primary play/resume action, and only immediate authorized secondary actions.
3. Television MAY update the hero to match deliberate shelf focus after a stable delay; casual focus movement MUST NOT cause flashing, repeated network work, or inaccessible copy.
4. Handheld MAY keep a stable initial hero instead of following horizontal-card focus.
5. When no hero exists, Home MAY begin directly with content rows. The absent hero MUST NOT leave a pure-black void; the Portico atmosphere fills the page.
6. An empty-history Home SHOULD lead with useful discovery/recent content rather than a large apology panel when authorized rows exist.

### 5.3 Row headers and card copy

- A row header states the row's user-facing purpose and MAY include one trailing “See all” action when a full destination exists.
- Card copy uses title plus at most two decision-useful lines.
- Source/library context appears only when ambiguity exists, such as a cross-server or mixed-source row.
- Row-level loading/error copy MUST NOT narrate transport or scanner implementation.

## 6. Library workspaces

### 6.1 Canonical pivots

The selected library's resolved capabilities determine available pivots. Typical order is:

| Library | Typical peer views |
| --- | --- |
| Movies | Discover, Movies, Collections, Categories |
| TV / Anime | Discover, Shows, Episodes, Collections, Categories |
| Music | Discover, Artists, Albums, Tracks, Playlists, Genres |
| Audiobooks | Discover, Books, Authors, Series, Collections, Genres |
| Recorded TV | Discover, Recordings, Shows, Collections, Categories |

Clients MUST use canonical pivot identifiers and MUST hide or disable unavailable pivots honestly. A client MUST NOT show the same generic tab list for every library.

### 6.2 Query controls

1. Search-within-library, filter, sort, direction, and view are peer controls over the current result set.
2. Available fields, operators, values, and sorts come from resolved browse capabilities. Clients MUST NOT invent filters from labels.
3. The current pivot, filters, sort, direction, and view remain visible and restorable.
4. Active filters receive a selected treatment and an accessible count/summary. Clear/reset remains reachable.
5. View choices MAY include grid, compact grid, list, table, and hierarchy only where that representation adds information.
6. A list/table MUST retain the same entity identity, action availability, and artwork role as the grid; it is not a lesser semantic object.
7. Empty filtered results retain the query controls and provide a clear path to remove constraints.

### 6.3 Result composition

- Poster grids are the normal video/book/person browse presentation.
- Square grids are the normal music presentation.
- The media-card `thumb` role is confined to episode/special/extra relationship rows inside the containing show or anime detail workspace. Programme-guide time cells, trickplay strips, and video-player previews are separate interaction geometry, not landscape media cards. Landscape media cards are not used in Home, discovery, recommendation, search, Saved, or ordinary library shelves.
- Mixed results group by canonical Product Contract group or use an explicitly contracted mixed presentation. They MUST NOT force every entity into one arbitrary shape.

## 7. Detail scaffold

Every detail page MUST provide, when available and authorized:

1. contextual backdrop over atmospheric fallback;
2. title and parent/series context;
3. concise facts needed for a play/listen/record decision;
4. progress, watched/live/recording/download state;
5. primary play/resume/watch/listen action;
6. immediate personal-state actions such as Saved/favorite where supported;
7. a contextual More action for lower-frequency operations;
8. an entity-appropriate body—episodes, tracks, chapters, programmes, credits, related items, extras, versions, or membership;
9. a route to technical/admin evidence only for authorized viewers and only through deliberate disclosure.

### 7.1 Fact order

The normal metadata order is:

1. relationship identifier (season/episode, disc/track, chapter, channel/time) when necessary;
2. release year/date or live time;
3. content rating/advisory;
4. runtime/duration;
5. genres or compact descriptive context.

Missing values disappear cleanly. Facts MUST NOT be filled with `Unknown` unless the absence itself is decision-critical.

### 7.2 Summary and credits

- The hero summary is bounded; complete text is reachable through expansion or the body on small screens.
- Spoiler-sensitive episode summaries follow the profile's spoiler policy.
- Cast/crew uses person entities with portrait art where available and stable role labels.
- Long credit lists use pagination/virtualization and deterministic focus identity.

### 7.3 Detail density floor

A media detail page is a complete decision surface before it becomes an exploration surface. Sparse presentation is a defect when the server projection contains useful data.

The first detail region MUST expose, when available and authorized:

1. backdrop/environmental artwork plus logo or title identity;
2. relationship context and compact facts: year/date, runtime/duration, content rating/advisory, genres, and progress/live state;
3. provider/community/editorial ratings only when their provenance and attribution are valid;
4. the current primary transport action—Play, Resume with useful remaining context, Watch Live, or Listen—and a clear path to Play from Beginning when resume is active;
5. immediate personal actions relevant to the entity: Saved/watchlist, favorite, Add to Playlist, rating/review, watched/unwatched, download, record/cancel, Cast/output, and More as capabilities permit;
6. a bounded synopsis and creator/director/author/artist context;
7. before planning, the selected version, quality intent, audio preference, and subtitle preference, each opening its contracted chooser when change is available; after a delivery plan exists, the player may separately disclose effective output without relabeling a preference as an observed result.

Rules:

- The page MUST NOT hide every useful action inside More merely to appear minimal.
- Saved/watchlist, Favorite, Add to Playlist, and Rate/Review are canonical immediate actions when authorized. They MUST NOT remain inside More merely because an implementation began with a smaller action row.
- Immediate actions MUST have labels on touch and television unless a universally understood platform transport/output icon remains unambiguous and accessible.
- More contains lower-frequency, administrative, destructive, version-specific, sharing, and technical actions; it does not duplicate the primary transport.
- Technical playback facts are concise consumer choices, not raw codecs or diagnostics. A compact value such as `4K HDR`, `English · 5.1`, or `Subtitles Off` is appropriate; complete stream evidence remains in Playback Info.
- Missing data closes its own space cleanly. It MUST NOT create a large empty hero or push all exploratory content several viewports away.
- Handheld hero height is content- and viewport-aware. A large fixed minimum, oversized top chrome, or action geometry that pushes the immediate decision surface below the fold is non-conformant. Long titles receive usable width, bounded responsive type, and multiple lines before decorative empty space is preserved.
- At normal text size on a common phone, the identity/facts, primary transport, immediate-action row, and beginning of synopsis SHOULD be visible in the initial viewport. When authorized immediate actions exceed the width, the action row scrolls horizontally or uses an equally discoverable compact treatment; the page MUST NOT require vertical scrolling merely to reach those actions. Accessibility text scaling may require more space.
- On a 1080p television, identity, essential facts, bounded synopsis, primary transport, all canonical immediate actions that fit the available width, and a visible cue or leading edge of the first body row SHOULD fit within the safe-area composition. Available horizontal space is used before overflow is moved to More.

### 7.4 Detail body order

Once the decision region is satisfied, sections follow entity-specific value rather than one generic stack:

| Entity family | Body order after the decision region |
| --- | --- |
| Movie | cast and crew → ratings/reviews where supported → trailers and extras → related movies → versions/technical disclosure |
| Show / anime | season selector and episodes → cast and crew → trailers/extras → related shows → versions/technical disclosure |
| Season / episode / special | containing show/season context → adjacent/remaining episodes → cast/credits where useful → extras → related titles |
| Artist | popular tracks → albums → appearances/compilations → related artists |
| Album | ordered disc/track list → credits → related releases/artists |
| Track | queue/album context → lyrics → credits → related listening |
| Audiobook / book | ordered chapters → author/series context → description/credits → related books |
| Live programme | now/next and live/recording state → recording options → show/channel context → related upcoming airings |
| Recording | programme/show context → playback/recording facts → related recordings/episodes → failure evidence when needed |

If a high-priority section has no data, the next section advances; clients MUST NOT insert an empty placeholder row. Exploratory sections SHOULD begin quickly after the decision region. A long synopsis or technical table MUST NOT separate the user from episodes, cast, extras, or related media unnecessarily.

### 7.5 Form-factor composition

Handheld detail normally uses a dense backdrop-led vertical composition with compact persistent Back and available Cast/output controls, followed by identity, facts, a full-width primary transport, a labeled compact horizontally scrollable immediate-action row when needed, synopsis/creator context, playback-choice summary, then horizontally scrollable or stacked body sections. Backdrop prominence comes from image composition and scrim continuity, not an oversized empty hero. The backdrop and scrims flow into the fixed Portico atmosphere; the page MUST NOT become a stack of unrelated cards.

Television detail normally uses the backdrop as a prominent environmental field, a readable decision column within the safe area, a large primary action plus bounded immediate actions, and the first body row visible or clearly cued below. Focus movement may expand a body row but MUST NOT erase the decision context unexpectedly.

Web adapts between these patterns based on width and input. A wide desktop detail MAY place the decision column beside artwork and expose more facts simultaneously; a phone-width Web detail follows the handheld hierarchy. Web MUST NOT use its administrative capability as a reason to clutter the consumer decision region with file evidence.

## 8. Entity presentation matrix

The machine-readable companion is `presentation-matrix.json`. The following table is the human authority for composition. “Available actions” always means actions authorized by the resource contract.

| Entity | Ordinary card | Detail primary decision | Required/priority facts | Primary body |
| --- | --- | --- | --- | --- |
| movie | poster | Play or Resume | year, rating, runtime; edition/version only when decision-relevant | cast/crew, extras, related, versions through disclosure |
| show / anime | poster | Resume/Play Next; otherwise Play when defined | years/status, rating, genre | one canonical Episodes workspace, cast/crew, extras, related |
| season | poster in season chooser; landscape is not implied | Play/Resume season only if contracted | season number/title, episode count, year | episode list within the show workspace |
| episode | inherited season/show poster outside show detail; thumbnail inside the containing show detail episode row | Play or Resume | `Sxx Exx`, title, air date/year, runtime | summary, parent show/season, extras/credits as available |
| special / extra | inherited season/show poster outside show detail; thumbnail inside the containing show detail relationship row | Play or Resume | relationship, type, runtime | parent context and related extras |
| artist | square | Play or Shuffle when available | artist identity; genres only when useful | popular tracks, albums, appearances/related artists |
| album | square | Play; Shuffle as secondary | artist, year, album type; disc count when relevant | ordered disc/track list |
| track | square in grids; compact square in lists | Play | artist, album, disc/track number, duration | queue/context, credits, lyrics when available |
| author | poster | no fabricated play action; play/resume next book only when contracted | author identity | books, series, related authors |
| audiobook series | poster | Play/Resume next book only when contracted | author, book count, series status | books ordered by series index |
| book / audiobook | poster | Play or Resume | author, series/index, year, duration | ordered chapters, description, related books |
| chapter | inherited book poster outside book detail; contained inherited cover or no-art list treatment inside book detail | Play or Resume | chapter number/title, duration | parent book context |
| recording | inherited show/programme poster; accepted programme poster or server-generated focal-safe poster derivative for a one-off; deterministic poster fallback last | Play or Resume | programme time/date, channel, duration, recording state | programme/show context, playback/recording facts, related recordings/episodes, failure evidence when applicable |
| live channel | contained logo | Watch Live | channel number/name, current programme, time remaining | now/next programme list and guide link |
| live programme | show/programme poster on ordinary shelves; dedicated non-card time-grid cell in the guide | Watch Live if airing; Record/Cancel as authorized; Play only if a recording exists | start/end, channel, live/upcoming state | programme detail, recording options, show context |
| person | poster | no play action unless a valid collection action is supplied | name, known-for context | credits grouped meaningfully by role/type |
| playlist | server-declared family: music square; video/audiobook/mixed poster | Play; Shuffle when meaningful | owner/visibility, item count, duration if known | stable ordered entries, duplicates preserved |
| collection | server-declared/library family: music square; video/audiobook/mixed poster | Play only if contracted for compatible members | owner/visibility, item count | semantic members and collection description |
| category | poster under current contract | none unless a collection action is supplied | category name/count | child results |
| unsupported | deterministic fallback | no play | clear unsupported reason | safe recovery/admin route if available |

## 9. Show and episode workspace

1. A show has one canonical detail workspace. Season and episode deep links resolve into it with durable selection state.
2. The hero/show identity and show actions remain stable while season/episode selection changes.
3. One season selector controls the episode list. A client MUST NOT create competing season controls.
4. Default season selection contains the Resume/Next episode when one exists; otherwise it uses the contract's initial season.
5. Episode rows expose thumbnail/fallback, numbering, title, bounded summary where space permits, runtime, watched/progress, play, and contextual actions.
6. Selecting an episode may open its detail context or play according to the semantic target; focus alone MUST NOT start playback.
7. Specials remain reachable and correctly related without being silently renumbered as ordinary episodes.

## 10. Music presentation

1. Artist, album, and track identity uses square art.
2. Album detail keeps artist and album identity visible above the track list.
3. Track lists preserve disc and track order, duplicate titles, version/explicit indicators when available, duration, playback state, and a contextual action target.
4. Multi-disc albums show disc boundaries without repeating a full hero for each disc.
5. Lyrics are a playback/detail companion, not a replacement for title/artist/album identity.
6. Synchronized lyrics follow current playback time; unsynchronized lyrics remain readable and scrollable. Missing lyrics is not an error.
7. Audio Now Playing uses cover art and optional backdrop/derived atmospheric treatment. It is never the black video-player canvas.
8. Queue, repeat, shuffle, output/Cast, quality, lyrics, and track selection remain accessible according to capability without crowding the primary transport.
9. On handheld, persistent audio uses a compact horizontal mini-player: small square art/fallback, title and useful secondary identity, play/pause, and at most the immediately useful next action. It is a doorway back to Now Playing, not a second full transport surface.
10. On television, persistent audio uses a compact focusable rectangular now-playing indicator in the top-right shell safe area. Selecting it restores Now Playing. No television audio mini-player card appears in content rows or along the bottom edge.
11. Video MUST NOT reuse either audio presentation. Handheld continuation uses native Picture in Picture when available; television video ends when its full-screen player closes unless an explicit receiver handoff owns continuation.

## 11. Audiobook presentation

1. Books and series use portrait cover art; chapters use a compact contained inherited cover or a no-art list treatment.
2. Progress is time- and chapter-aware. Resume identity includes book, chapter when known, and useful remaining context.
3. Series order uses the canonical series index; missing indices sort by the Product Contract fallback, not client guesswork.
4. Book detail prioritizes Resume/Play, author/series, total duration, description, and chapter navigation.
5. Playback speed, sleep timer, chapter navigation, bookmarks if supported, and output selection are first-class audio utilities.
6. Chapter artwork MUST NOT be fabricated by cropping a book cover into landscape when a contained cover/list treatment would be clearer.

## 12. Live TV and DVR

### 12.1 Live TV

1. Guide, Channels, and DVR are distinct peer workspaces.
2. Channel identity uses a contained logo plus readable channel name/number; the logo alone is insufficient.
3. Programme cells preserve channel row identity, start/end geometry, current-time position, focused state, selected/active channel, recording state, and availability.
4. The current programme and tuned channel are visually clear without conflating focus, playback, and selection.
5. Programme detail exposes Watch Live only while playable, and Record/Cancel/Series actions only when authorized.
6. Missing guide data, offline source, tuner exhaustion, and permission denial are different states with different language/actions.

### 12.2 DVR

1. Recordings, rules, schedule/upcoming, conflicts, failures, and capacity are distinct views or explicit sections.
2. Recording status uses icon/text plus semantic color; color alone is insufficient.
3. A failed recording explains that the recording failed without exposing raw tuner/storage exceptions on the consumer surface.
4. Conflict resolution presents the programmes, times, tuner demand/capacity, and the consequence of each choice.
5. Destructive cancellation/deletion requires confirmed intent and preserves focus/context after completion.

## 13. Search and mixed results

1. Search groups and group order come from the Product Contract.
2. Each result uses the entity's canonical destination, hierarchy, and artwork role; a client MUST NOT collapse all results into `movie`, `show`, or `music`.
3. Quick Search uses bounded grouped results and transfers the complete query to Full Search.
4. Full Search exposes only contracted scopes/sorts and independent group continuation.
5. A mixed result group may use compact rows to accommodate multiple artwork roles, but each item retains correct aspect/fit and kind label.
6. Recent queries are private viewer-scoped presentation data and clearable through an explicit action.
7. No-results, no-access, server-offline, and query failure are distinct.

## 14. Saved, playlists, collections, and downloads

1. Saved is a consumer destination with explicit peers such as My List, Playlists, Collections, Favorites, and saved views where enabled.
2. **My List** is the canonical name for the save-for-later feature (renamed from Watchlist on 2 Oct 2026 so it reads for any media type: a film or show to watch, a book to listen to) and its actions are **Add to My List** and **Remove from My List**. The stored flag and API field stay `watchlisted`. **Saved** names the containing destination; it is not a synonym for My List and MUST NOT appear in those action labels. My List is a destination only (Saved › My List): Home has no My List row (2 Oct 2026).
3. My List/favorite states are personal state and update consistently on card, detail, context menu, and Saved surfaces.
4. Playlist entries use stable entry identity; duplicate media remains intentionally visible and reorderable.
5. Collections are semantic grouping, not playlists with a different name.
6. Owner/editor/viewer differences affect available actions, not the core visual identity of the resource.
7. Downloads show media identity, quality/version where decision-relevant, size/progress/state, expiry/offline availability, and a clear manage/remove action.
8. A download error does not make the online media item appear unavailable.

## 15. Actions and overflow

1. Each detail surface has one primary playback/listening/watch decision when one is authorized.
2. Resume and Play from Beginning are distinct intents. A confirmation/choice is required only when both are meaningful and the platform's established behavior calls for it.
3. Applicable high-frequency personal-state actions MUST be in the decision region beside or immediately below the primary action: Saved/watchlist, favorite, Add to Playlist, rating/review, watched/unwatched, download, record/cancel, Cast/output, or equivalent. Saved/watchlist, Favorite, Add to Playlist, and Rate/Review are the first canonical personal actions when authorized. A constrained platform may move only genuine overflow beyond its bounded immediate-action capacity into More.
4. Lower-frequency actions belong in a contextual More surface in canonical action-group order.
5. Administration actions never displace the consumer primary action. They live under Manage/Edit/Playback Info or Server Console disclosure.
6. Context menus and multi-select use the same action IDs and permission results as detail.
7. A later authorization denial is shown honestly and reconciles stale visible actions.
8. Optimistic personal state is visually reversible and announces failure without losing the user's route.

### 15.1 Playback choice presentation

1. Quality selection MUST always expose the current choice and include `Automatic` plus the explicit quality choices supplied by the playback contract.
2. `Automatic` communicates that Portico may choose the compatible delivery needed to keep playback working.
3. An explicit choice such as a bitrate/resolution cap communicates literal user intent. The UI MUST NOT imply that Portico may silently replace it with another quality.
4. Audio, subtitle, version, and quality selections remain distinct controls with their current value visible before commit.
5. If an explicit choice cannot be honored, the player presents the contracted unavailable/failure explanation and lets the user choose again; it MUST NOT display the requested choice while playing an undisclosed substitute.
6. Recommendations and warnings MAY explain that a choice could buffer, transcode heavily, lose HDR, or fail. They MUST NOT block an authorized choice merely because Portico considers it unwise.
7. The active-player settings surface MUST expose the current quality intent and all current server-issued quality offers. `Automatic` and explicit offers are separate choices; Direct Play/remux/transcode is an effective plan fact, not a substitute quality selector.
8. Audio track and subtitle/off selection are required whenever multiple choices exist. Playback speed, sleep timer, chapters, queue, and output controls appear when applicable and supported. Autoplay-next remains in Settings. Clients MUST capability-gate these controls rather than show dead or decorative actions.
9. The current Web player capability set is the minimum functional target for handheld and television players: queue inspection and mutation, reorder, repeat, shuffle, video quality/type, audio quality/type and track, subtitles, lyrics, sleep timer, playback speed, and platform-appropriate volume/mute. Autoplay remains a Settings preference rather than a player-chrome control. Platform layouts MAY differ; supported operations and their canonical results MUST remain reachable, but they need not all be simultaneously visible on a compact viewport.
10. Touch and television queue reordering expose visible Move Earlier and Move Later actions with truthful disabled boundaries. Platform-native direct drag/reorder MAY supplement them when it does not steal scrolling or assistive navigation. Every path preserves stable queue-entry identity and uses the Playback Contract's queue revision/conflict law.

### 15.2 Television in-player discovery

1. Television video players MAY extend downward from the five immediate option boxes into a deliberately entered discovery region. Handheld video players MUST NOT expose this region.
2. The first discovery shelf is Cast & Crew using canonical person entities, portrait art, and role labels. Recommendation and related-media shelves follow using the ordinary poster/square geometry and inheritance rules; landscape episode thumbnails remain confined to the containing show detail workspace.
3. Entering discovery does not pause, replace, or obscure the current media merely because focus moved. Person activation uses the supported person/detail behavior and never starts arbitrary media.
4. Activating a playable media card requests deliberate playback replacement. The current player, session, image, and underlying detail context remain intact until the replacement is authorized and prepared. Success atomically adopts the replacement and changes the underlying media-detail identity so closing the player reveals the newly playing item. Failure preserves the current playback/detail context and presents an honest recoverable error.
5. Back while focus is inside discovery dismisses the discovery region and restores the current media/player context. The next ordinary player-chrome engagement starts in the transport/control region rather than restoring the last discovery-card focus.
6. The discovery region reuses the same server projection and authorization rules as media detail. It MUST NOT create a Hosted Services media-metadata or recommendation dependency.

## 16. Progress, watched, live, and availability state

1. Progress is computed from authoritative position/duration and clamped only to valid display bounds. It MUST NOT enforce an artificial visible minimum.
2. Watched/completed is a durable state distinct from `99%` progress.
3. Resume copy and position use the canonical timeline; clips/extras and multi-version items must not borrow another version's duration.
4. Live progress represents programme time only when guide times are trustworthy; otherwise use a live indicator without a false ratio.
5. Recording, scheduled, conflict, failed, downloaded, downloading, unavailable, and offline-only are separate semantic states.
6. State badges remain sparse. A card should not become a pile of every known fact.
7. State changes patch visible copies of the entity across Home, detail, Saved, search, and player-adjacent surfaces.

## 17. Loading and continuity

1. Card shape, row origin, header, and primary action location are reserved before network data/artwork.
2. Successful data remains visible while compatible data refreshes.
3. New artwork replaces old artwork only after decode and authorization validation.
4. A metadata update changes content in place without resetting route, scroll, focus, queue, or active media transport.
5. Viewer/profile/server transitions fence old media immediately. Retention never crosses an unverified viewer scope.
6. Unavailable/offline state preserves the persistent product frame and offers server selection/account actions without pretending the account is signed out.

## 18. Administration and evidence

1. Technical media details, versions, streams, codecs, analysis, match provenance, file information, and optimization tools are deliberate administrative disclosures.
2. Consumer cards, Home, and normal detail MUST NOT annotate Direct Play/Remux/Transcode predictions.
3. Playback Info MAY show the active delivery method and relevant facts after playback begins.
4. An editor presents field provenance/locks and artwork roles without exposing metadata-provider URLs to ordinary users.
5. Destructive file deletion is visually and semantically distinct from removing an item from Saved, a playlist, or a collection.

## 19. Responsive and ten-foot adaptation

- Handheld may shorten summaries, reduce hero height, move low-frequency actions into More, and use sheets for filters/track choices.
- Web may expose more simultaneous pivots, filters, metadata, and administration while preserving the consumer hierarchy.
- Television increases type/targets, limits simultaneous copy, uses focus-trapped panels, and keeps critical actions above the fold/safe area.
- Constrained clients may paginate or virtualize aggressively but cannot omit the current state, primary decision, canonical destination, or recovery path.
- A platform must not change entity artwork role because its generic card component only supports one ratio.

## 20. Comparative rationale: Plex and Jellyfin

The detailed source record is `../visual/comparative-rationale.md`. The enforceable conclusions are:

1. Portico adopts the useful precedent of a familiar Home/Library/Live/Saved hierarchy, customizable ordered recommendation rows, and content-type-specific library peers.
2. Portico rejects allowing source pinning or local app customization to silently redefine cross-client product meaning; the server/viewer composition is authoritative and explicit.
3. Portico adopts backdrop-led detail and fixed artwork-role geometry, but rejects black/flat fallback pages and arbitrary theme divergence.
4. Portico adopts accessible in-player quality, audio, subtitle, chapter, and queue controls, but keeps video visually primary and avoids persistent opaque chrome.
5. Portico adopts explicit user choices and manual override precedence. Automatic behavior is a user-selected mode, not permission to override an explicit quality/track choice.
6. Portico rejects CSS/theme extensibility as a first-party semantic contract; customization cannot compromise focus, contrast, state meaning, or cross-client consistency.

## 21. Conformance fixtures

Every first-party client MUST be tested against the same fixture meanings:

- each entity in `presentation-matrix.json` with full metadata, sparse metadata, missing art, and failed art;
- Home with hero, no hero/history, partial row failure, empty row, and retained refresh;
- poster, square, thumb, logo, and mixed search results;
- show workspace deep-linked to season and episode;
- multi-disc album, long track names, audiobook with many chapters;
- live channel with/without logo, guide data unavailable, tuner exhausted, scheduled/conflicting/failed recording;
- progress near zero, near complete, completed, invalid duration, and live programme;
- action availability changed between read and activation;
- large text/zoom, reduced motion, keyboard/remote focus, and offline retained content.

Conformance compares semantic elements, order, state, aspect/fit, accessible identity, focus behavior, and recovery—not pixel equality.

## 22. Product decisions

- Video, television, anime, and audiobooks are poster-first; music is square-first; episode thumbnails exist only inside the containing show detail workspace.
- Playlist/collection presentation family is server-declared. Genuinely mixed containers use poster geometry.
- One-off recordings use accepted poster art, then a server-generated focal-safe poster derivative, then the deterministic fallback.
- Home renders a hero only when the server supplies a relevant, authorized hero projection. Without one, rows begin immediately over the fixed Portico atmosphere; a client MUST NOT fabricate an empty or generic hero merely to occupy space.
- Television detail reserves one primary action and a responsive immediate-action row that uses the safe-area width before overflowing. Saved/watchlist, Favorite, Add to Playlist, and Rate/Review remain directly exposed when authorized; Play from Beginning, watched state, download, Cast/output, and other high-frequency actions follow canonical order. `More` appears only for lower-frequency or genuine overflow actions. Capability/permission absence closes positions without inserting dead controls.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.

## October scope and evidence

Original is the quality default; Automatic is an explicit connection-adaptive choice. Missing cards and server-authored See all destinations have later follow-ups. Every subgroup must expose the main library sort/filter vocabulary; grouping implementation is still in progress. Physical layout/accessibility parity remains unverified.

Sources: [web-app](../../../_program/reports/web-app.md), [native-app](../../../_program/reports/native-app.md), [personal-state](../../../_program/reports/personal-state.md), [grouping-sorts](../../../_program/reports/grouping-sorts.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
