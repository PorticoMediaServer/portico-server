
# Profile preferences

Every field declares its consumer. Integers declare `step: 1`. Locale uses the
semantic type `locale`, defaulting to `auto` for client environment resolution.
Language lists use `languageList`, canonical BCP-47 tags, and reject duplicates
after canonicalization.

There is no time zone preference:
every client shows times in the device's own zone. `region.timeZone` is an
unknown key in a patch, and a value stored for it earlier is ignored on read
and dropped by the next save of that document.

Saved quality choices are `original` (default), `automatic`, `2160p`, `1440p`,
`1080p`, `720p`, `480p`, `360p`, in `quality.home` and `quality.remote`.
Automatic starts at Original and adapts; it is an explicit choice. Use the
registry's allowed values instead of keeping an independent quality ladder.

The effective response also includes `audioEffects` (`gapless`,
`crossfadeMs`, `normalization`), derived from the three `music.*` registry
values. Clients should apply this object to the renderer and persist changes
through the preference endpoint. The registry no longer includes
`music.shuffleDefault`, `music.repeatDefault`, or `navigation.sidebarCollapsed`.
Watch Together excludes opted-out profiles from presence/readiness projections
and suppresses their corresponding activity events, including retained replays.

Listening preference reads and writes use this same profile document. The
`/v1/listening/preferences` response is a projection: `musicRate` maps to
`music.defaultSpeed`, `bookRate` to `audiobooks.defaultSpeed`, `autoplayNext` to
`playback.autoplayNext`, and `passoutMinutes` to `playback.sleepTimerMinutes`.
Its revision is the profile document revision (1 for defaults); a change through
either API invalidates a stale write through the other. There is no separate
listening-preferences table. Listening writes revalidate authority in the write
transaction.
