# Lane: identity — titles, versions, missing files, deleting, removing a library

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-identity` (branch `audit-implementation/identity`). You can commit.
Audit documents: 05 (identity parts), 07, 09 (deletion and the state-folder trash), 12 (where personal state attaches); verification V04, V05. Decision register D-01, D-33, D-53.

## What to deliver
1. **A title is separate from its files.** Today a film is the catalogue row its file was scanned into, and personal state hangs off that. Change the model so that a title (film, episode, album track, book…) is its own thing, files attach to it as versions, and all personal state (progress, watched, ratings, history, My List, playlists) attaches to the title. A second file that resolves to the same title (same provider identity, or the same parsed identity where unmatched) becomes another version of it, not a second title. Replacing a file with a better encode under a new name keeps the title and its state (B-05.8). Shows must not split or merge on title-and-year alone (B-05.27).
2. **Versions** are listed per title with enough facts for a picker (resolution, codec, dynamic range, audio, size, edition label where known). Playback can name a version; with none named the server picks the best playable one.
3. **Missing files ("trash", the Plex model).** A file that disappears from disk is removed as a version. A title left with no files stays, marked missing with the time it went missing, and is shown as unavailable rather than removed. A replacement file re-attaches. An owner setting chooses when missing titles are emptied: immediately, after a number of days (default 30), or never. Add an "empty now" action. **When a library root is unreachable or looks unmounted (missing, unreadable, or empty where it previously had content), nothing under it is marked missing.**
4. **Deleting.** Deleting from Portico deletes the file on disk. Remove the code that moves deleted media into a folder inside the state directory, its settings and its maintenance (B-09.13, B-09.18, A-09.13). Keep the existing preview of what a delete will remove and the confirmation.
5. **State that outlives removal.** When a title is finally removed, keep each profile's watch state keyed by provider identity (and season and episode numbers for episodes) in a small table, and re-apply it when the same title is added again. Bound it sensibly.
6. **Removing a library** (D-53): a route and the work behind it. Titles go; personal state is kept under rule 5.
7. Check B-05.18 (the device number in file identity changing across reboots on network mounts) and make file identity robust to it.

## Boundaries
- You own `catalog`, `compactcatalog`, `entityid`, `personalstate`, `administration` (deletion, file operations), `persistence` and the baseline schema for these tables.
- The **naming** lane is rewriting how names are read; the **providers-matching** lane owns matching and providers; a later lane reworks the scan loop and wires all three together. Do not rewrite name reading, matching or the scan loop. Change only the point where a scanned file is attached to a title, and make that point a clear function others can call with "this file, this parsed identity, this provider identity (if any)".
- The **music-audiobooks** lane owns how tracks group into albums. Give it the same attach point.
- A later lane reworks derived-data rebuilds and revision counters. Keep derived tables working; do not redesign them.
- Update `packages/client-core` parsers and types for anything whose shape you change. List the screens needed (version picker, missing badge, trash list and settings, remove-library) in your report.
