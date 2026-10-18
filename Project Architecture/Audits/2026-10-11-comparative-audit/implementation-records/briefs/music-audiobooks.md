# Lane: music-audiobooks — tags, albums, lyrics, chapters and the listening rules

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-music-audiobooks` (branch `audit-implementation/music-audiobooks`). This is your working directory and, with `/home/justin/projects/Portico/.worktrees/audit-implementation/_program`, the only place you can write.
**You cannot commit** (the sandbox keeps git metadata read-only). At each milestone, when the tree compiles, run from the worktree root:
`bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh music-audiobooks "<one-line commit subject in plain language>"`
Snapshots are cumulative; each becomes one commit. Take one at least every hour of work and always one at the very end, after which you must not change the tree. Go environment: `GOCACHE=/home/justin/projects/Portico/.worktrees/audit-implementation/_program/gocache` and `GOFLAGS=-p=2` are already set.
Audit documents: 10 (all); verification V07, G2. Decision register D-44, D-31, D-02.

Justin has given this lane leeway: put sane defaults in place for the defects below. CarPlay and Android Auto are out of scope.

## What to deliver
1. **Our own tag reader** (D-31; no new dependency). Tags are read through ffprobe and a whitelist today, so multi-value artists, sort tags and some ReplayGain forms are missed (A-10.1). Read ID3v2.2, 2.3 and 2.4 (and v1 as a last resort), Vorbis comments in FLAC, Ogg and Opus, MP4 atoms, APEv2, and WAV/AIFF chunks: title, artists and album artists as lists, sort names, album, disc and track numbers with totals, dates (original and release), genres as a list, composer, compilation flag, MusicBrainz ids, ReplayGain and R128 values, embedded pictures with their type, embedded lyrics (plain and timed), and for audiobooks narrator, series and position, publisher, description, and chapters in MP4 and ID3. Bounded and safe against hostile files. Keep ffprobe for stream facts only.
2. **Albums group correctly** (B-10.17): by MusicBrainz release id when present; otherwise by album artist, album and year, across disc folders (`CD1`, `Disc 2`), with compilations kept together. Use the shared attach point the identity lane is building where available.
3. **Lyrics**: local lyrics (sidecar `.lrc` and `.txt`, embedded) appear on their own without a manual step (A-10.34); one odd line must not reject an LRC file (B-10.8); the bulk lyrics run must not publish the first search hit unverified (B-10.6) — verify by artist, title and duration.
4. **Audiobooks**: finished is decided by time remaining, not 95% (B-10.1, check the fix log first); one bad chapter must not reject all of a book's chapters (B-10.16); narrator from every tag form in use; multi-file books ordered correctly; resume a little before the stopping point after a long pause.
5. **Queue rules**: "Play next" goes next when shuffle is on (B-10.3); the other queue defects in document 10.
6. Every other confirmed bug in document 10 that is still present on the server or in `packages/client-core`.

## Boundaries
- You own `audiofacts`, `lyrics`, the music and audiobook parts of `mediaanalysis` and `ingestion`, and the music parts of `client-core` that are not screens.
- The **identity** lane owns the catalogue and schema; the **naming** lane owns path reading; the **providers-matching** lane owns providers (including music and audiobook sources). Keep to tags, grouping, lyrics, chapters and rules.
- The native gapless audio engines are not yours.
