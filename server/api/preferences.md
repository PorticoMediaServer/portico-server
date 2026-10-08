
### Registry revision p39.B1

Every field declares its consumer. Integers declare `step: 1`. Locale uses the
semantic type `locale`, defaulting to `auto` for client environment resolution.
Language lists use `languageList`, canonical BCP-47 tags, and reject duplicates
after canonicalization.

There is no time zone preference (removed in registry revision p39.CD32.B4):
every client shows times in the device's own zone. `region.timeZone` is an
unknown key in a patch, and a value stored for it earlier is ignored on read
and dropped by the next save of that document.

Video bitrate choices are labelled objects in `allowedValues`: Original (0),
40, 20, 12, 8, 4, 2, 1 Mbps, with `maxVideoHeight` on each bounded choice.
Audio choices are Original (0), 320, 256, 192, 128, and 96 kbps. A zero bitrate
means unrestricted original quality, subject to explicit administrator caps.

The effective response also includes `audioEffects` (`gapless`,
`crossfadeSeconds`, `normalization`), derived from the three `music.*` registry
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
