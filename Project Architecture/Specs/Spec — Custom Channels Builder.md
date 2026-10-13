# Library Channels builder (design spec)

October scope and evidence: This remains a product design specification. Do not infer that its editor/API examples are the current wire contract. Shared live input/DVR changes do not redesign Library Channels. Verify editor/player behaviour against current registry and client code before marking conformance; physical acceptance remains open. Sources: [livetv](../../../_program/reports/livetv.md), [native-sync](../../../_program/reports/native-sync.md). Existing route examples must be checked against generated contracts before use.

Server package `server/internal/livechannels/library`; builder UI in the web console (`web/src/screens/server/ChannelBuilder.tsx`). The feature is called **Library Channels** (`channels.custom.name`).

Goal (Justin): any channel an owner can describe is easy to make. His three examples are acceptance tests:
1. "Only movies from 1985."
2. "Nothing but Malcolm in the Middle."
3. "Only bad movies that mention sharks, with a rating under 3.0."

## 1. What a rule selects

A channel has rules (up to 16) and blocks (the schedule, unchanged, §4). A rule's **selection** is:

```jsonc
{
  "libraryIds": ["movies", "tv"],   // required; never "all libraries" implicitly
  "kinds": ["movie", "episode"],    // movie titles, and/or episodes of shows
  "filter": { /* the browse expression: exactly the library browse and saved-view language */ },
  "text": "shark",                  // optional: title or summary mentions this (words, case-insensitive)
  "includeItemIds": ["show-malcolm"], // pinned in: a movie, an episode, or a show (all its episodes)
  "excludeItemIds": ["movie-jaws-3"], // pinned out, same id kinds
  "order": "title",                 // sequential order key (§3)
  "limit": 0                        // 0 = every match
}
```

- **`filter` is the browse expression** (`catalog.BrowseNode`: `all`, `any` and `not` groups with `{field, operator, value}` predicates), validated by the same parser. Every browse field works:
  - kind, year and decade, date added, release date;
  - genre, tag, label, collection, content rating;
  - community and critic rating;
  - resolution, dynamic range (HDR), audio language;
  - studio, network;
  - actor, director, writer;
  - duration (runtime).
- **Nesting:** groups nest as browse allows (5 levels). The builder UI offers two levels, "Match all/any of" plus one sub-group, and NOT on any condition or group.
- **Where the filter is evaluated:**
  - For movies, it applies to the movie.
  - For episodes, it applies to the **show**: "genre is Comedy" picks comedy shows and plays their episodes; "network is FOX" picks FOX shows.
  - An episode-level condition (a season range) is a follow-up. Shows are the unit people describe channels in.
- **Library Channels are one curated, shared lineup** (Justin, 23 Sep): every viewer with the same access sees the same schedule at the same time. So there are no personal fields in rules (watched, favorite, watchlist, personal rating, last played). The builder doesn't offer them, and the server refuses them with a clear message. Community and critic ratings are not personal and stay. There are no owner-only channels for personal lineups; the channel's existing visibility (owner only or server members) is unchanged.
- **`text` matches words in the title or summary**, case-insensitive: movie title and overview; for episodes, the show's title and overview and the episode's own title and overview. Several words mean all must appear. There are no wildcards.
- **Pinned lists:**
  - `includeItemIds` adds titles even if the filter would drop them;
  - `excludeItemIds` removes them even if pinned or matched;
  - a show id includes or excludes all of its episodes.
- **Legacy fields migrate forward on read and save:**

  | Legacy field | Becomes |
  |---|---|
  | `genres` | `filter` genre contains-any |
  | `yearFrom`/`yearThrough` | year at-least/at-most |
  | `recentDays` | dateAdded at-least |
  | `showIds` | `includeItemIds` with the episode kind |

  A migrated config selects exactly what it did before. There is a test for this.

The three examples:

| Channel | Selection |
|---|---|
| Movies from 1985 | `kinds:[movie]`, `filter:{field:year, operator:equals, value:1985}` |
| Nothing but Malcolm in the Middle | `kinds:[episode]`, `includeItemIds:[<show id>]` (or `filter:{field:title, operator:equals, value:"Malcolm in the Middle"}`), episode mode "in order" |
| Bad shark movies | `kinds:[movie]`, `text:"shark"`, `filter:{field:communityRating, operator:less-than, value:3}` |

## 2. Episodes

`episodeMode` values:

| Value | Behavior |
|---|---|
| `none` | Episodes are ordinary items. |
| `in-order` | Each show plays in episode order; shows interleave by the rule's mode. |
| `marathon` | One show in order until the max-consecutive limit or its end. |
| `randomized` | Random episodes. |
| `rotate` (new) | One episode per show in turn, each show in episode order: S1E1 of every show, then S1E2 of every show, and so on. |

## 3. Order

**`mode`:**
- `sequential`: in `order`.
- `shuffle-bag`: shuffle without repeats until every item has played (the existing no-repeat cycle, with the dedup window across cycles).
- `weighted-random`: random with repeats, weighted.
- `sequential-then-shuffle` (new): the first cycle plays in `order`, then later cycles shuffle.

**`order` keys:**
- `title`;
- `year`;
- `recent` (date added, newest first);
- `oldest` (date added, oldest first);
- `release` (new: release date, then year, oldest first, i.e. chronological);
- `rating` (new: community rating, highest first);
- `episode` (show, season, episode).

## 4. Scheduling (kept, verified)

Blocks by weekday and time, priorities, fallbacks, anchors (channel cursor or block start), overrun (finish, cut, fit), weights, max consecutive and the dedup window stay as they are. The tests cover a 7-day generation with overlapping blocks, a DST day, fit overrun and slate exhaustion.

## 5. Preview

`POST /v1/admin/library-channels/preview` returns, per rule, and for the whole channel:
- `eligible`: items that match and can be scheduled;
- `unresolved`: matches without a usable duration;
- `hours`: total programming in hours;
- `sample`: up to 8 matching titles;
- `firstDay`: the first day's schedule, generated by the real generator in a transaction that is rolled back. It is exactly what saving would produce, including slates. Its candidates are bounded (2,000 per rule) so preview stays fast on huge libraries; totals still count every match.

## 6. Restrictions

The schedule is shared, and each viewer sees only what their profile allows:
- A restricted entry shows in the guide as "Not available on this profile" (a slate for that viewer).
- Tuning into it is refused, and the next allowed entry plays.

This reuses the profile's content restrictions (the same predicate as browse, C37).

## 7. Builder UI (web console)

- **Empty state:** "Create a channel", with templates.
- **A template opens a review step**, the builder pre-filled, never an instant create.
- **The builder:**
  - name and logo;
  - one card per rule, with:
    - libraries;
    - "Movies / Shows" toggles;
    - the browse filter editor (the same field, operator and value controls as library filters);
    - "mentions" text;
    - pinned include/exclude pickers (search titles);
    - order and episode handling as plain-language choices;
  - the schedule blocks table (a simple "all day" default);
  - a live preview panel (count, hours, sample posters, first day), refreshed 500 ms after edits.
- **Errors** name the field that is wrong ("Year must be a number"), using the server's validation paths.
