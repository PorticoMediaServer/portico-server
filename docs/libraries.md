# Libraries, files and matching

Add a library in Settings → Server → Libraries. Choose its kind and one or
more media folders. Paths must be readable by the account running Portico; in
Docker they are paths inside the container. Keep film, TV, anime, music and
audiobook libraries separate so numbers and tags are interpreted in context.

## Video names

Folders help, but a useful filename and embedded metadata take precedence over
conflicting folder evidence. `Show/Season 1/Show - S02E01.mkv` belongs to season
2. A generic filename can use the title in its parent folder. Years and provider
IDs help distinguish titles, but a year is not required.

Examples recognised by the reader:

```text
Movies/
  The Matrix (1999)/The Matrix (1999).mkv
  The Matrix (1999)/The Matrix (1999) {edition-Extended Cut} - 1080p.mkv
  The Matrix (1999)/The Matrix (1999) - 2160p.mkv
  The Matrix (1999)/Behind the Scenes/Making the film.mkv
  The Matrix (1999)/The Matrix-trailer.mp4
  Arrival (2016) {tmdb-329865}.mkv
TV/
  Show (2020)/Season 2/Show - S02E01.mkv
  Show (2020)/Season 2/E02.mkv
  Show (2020)/Season 2/03.mkv
  Show (2020)/Show - 2x04.mkv
  Show (2020)/Show - S02E05-E06.mkv
  Show (2020)/Specials/Show - S00E01.mkv
  Daily Show/Daily Show - 2026-10-01.mkv
Anime/
  Series/[Group] Series - 01 [1080p].mkv
```

Season folders are optional. `S02E01`, `2x01`, repeated episode markers and
ascending episode ranges are understood; a file may link to at most eight
episodes. Bare numbers and `E01` use a season folder when present. Anime can
use absolute episode numbers; mapping them to seasons needs matching data.
Air-date names can identify daily episodes when the provider knows the date.
Contradictory or overlarge ranges become scan warnings instead of being guessed
as the first episode. Loose files can use their own title and episode markers.

An edition is a named cut, such as `{edition-Director's Cut}`. Versions are
alternative files of the same title; technical labels such as resolution and
codec do not create a different title by themselves. `-part1`, `-part2`, `-cd1`
and `-cd2` indicate parts rather than two unrelated films. Inspect the Files
list after a scan if grouping differs from what you intended.

Extras are attached to their owning title: use a subfolder such as `Extras`,
`Trailers`, `Featurettes`, `Behind the Scenes`, `Deleted Scenes`, `Interviews`,
`Scenes` or `Shorts`, or a filename suffix such as `-trailer`, `-featurette`,
`-deleted`, `-behindthescenes`, `-interview`, `-scene`, `-short`, `-other`.
For TV and anime, `Specials` means season zero. A top-level show called
`Extras` is still a show. A disc folder such as `BDMV` or `VIDEO_TS` provides
naming evidence; it does not guarantee that every disc structure can play.

## Music and audiobooks

Embedded title, artist, album, disc and track tags are useful. A conventional
layout also works as evidence:

```text
Music/Artist/Album/Disc 1/01 - Song.flac
Audiobooks/Author/Book/Chapter 01.m4b
```

Keep album/book files together. Embedded book chapters and separate chapter
files are distinct inputs; do not rename every track to the book title.

## Ignore files

Dot-prefixed entries, system/recycle folders, `.grab` entries, sample folders
and sample filename tokens are skipped. A title named `Sample (2020)` is kept.
Use `.plexignore` or `.ignore` in a library folder or a subfolder for your own
exclusions:

```text
# Patterns relative to this folder
*.tmp
Unsorted/
*sample*
```

Blank lines and `#` comments are ignored. `*`, `?` and character classes are
supported. At the library root a pattern without a slash matches basenames
recursively; in a subfolder it matches that folder's immediate entries. A
slash pattern is relative to the folder holding the ignore file. There is no
Git-style `!` re-inclusion syntax. Malformed lines are skipped and reported;
valid lines remain active. Each file is limited to 64 KiB, 512 patterns and
1,024 bytes per pattern; combined reads are also bounded.

## Choose a metadata source

A library's Metadata tab chooses its provider and language. TMDB is the default
for film, TV and anime; the registry also offers TheTVDB for film/TV/anime and
AniList for anime. Music uses MusicBrainz. Audiobook metadata is local in the current provider registry. Choose
local metadata only (`metadataAgent: "local"`) to use filenames, embedded tags,
NFO files and local artwork without online matching. The available choices are
published by `GET /v1/metadata/providers`; use the server's list in an API client.
Owner-supplied TMDB/TheTVDB credentials are server-wide Metadata settings.

Matching weighs names, aliases, year, provider IDs and other available evidence.
Minor spelling differences and a missing year can still match. There is no
promise that an ambiguous filename will choose the title you meant.

Open a library's Needs attention tab for unmatched or uncertain titles and
placement issues. It offers candidates and reasons, search, a provider ID or
provider title URL, and dismiss where applicable. Work in progress can be
included separately. Fix a show's match to give its episodes the correct
series identity; an episode that cannot be placed remains an attention item.
Manual metadata locks survive refreshes; restoring an automatic value releases
the lock. A scan warning about one file or folder does not invalidate usable
neighbours. Consult scan history for skipped entries and inaccessible folders.

## Missing files and deleting

Titles and files are separate. A file disappearing leaves its title marked
missing; a replacement can attach to that same title. An unreachable library
root does not mark its contents missing. Missing-title cleanup is an owner
choice in Storage & backups: immediately, after the chosen period, or never.
Provider-linked watch state survives title removal and can return on re-add.

Deleting media in Portico deletes the selected media on disk. It is not a move
into a trash folder. Keep independent media backups. Emptying missing titles
removes their catalogue entries rather than deleting an existing media file.
