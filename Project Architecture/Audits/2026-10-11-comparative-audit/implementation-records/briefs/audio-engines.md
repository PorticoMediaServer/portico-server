# Lane: audio-engines — the music and audiobook playback engines on web and native

Two worktrees, both writable; you cannot commit in either (git metadata is read-only):
- `/home/justin/projects/Portico/.worktrees/audit-implementation/server-audio-engines` (your working directory; repository portico-server: the web app in `web/`, the canonical shared packages in `packages/`, the server in `server/`). Snapshot from its root with `bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh audio-engines "<commit subject>"`.
- `/home/justin/projects/Portico/.worktrees/audit-implementation/native-audio-engines` (repository portico-react-native: `apps/native`, `apps/vega`). Snapshot from its root with `bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh audio-engines-native "<commit subject>"`.
`packages/*` in the native repository are copies: change the canonical package in the server worktree, then run `PORTICO_SERVER_REPO=/home/justin/projects/Portico/.worktrees/audit-implementation/server-audio-engines node scripts/shared-packages.mjs --write` in the native worktree. Snapshots are cumulative; each becomes one commit; take one per milestone and a final one in each worktree, after which do not change the tree. `node_modules` are symlinks; do not install. Your report is `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/audio-engines.md`.

Audit documents: 10 (the playback-engine entries), 17 and 20 where they concern music; verification V07. Read `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/music-audiobooks.md` first: that lane just finished the server side (tags, albums, lyrics, chapters, listening rules, queue rules) and its "Needed from others" and "Not finished" sections list what it left for the engines. Its work is already in your worktree.

Portico plays music through its own decode engines to get gapless, Plexamp-grade playback: one on the web, one on Apple platforms, one on Android. CarPlay and Android Auto are out of scope.

## What to deliver
1. **Web engine**: resampler continuity across track boundaries, MP3 synchronisation search, the codec and browser gaps the audit and the music lane list (Firefox and Vorbis), and Media Session facts for audiobooks (author, narrator, chapter).
2. **Native engines (Swift and Kotlin)**: volume normalisation gain applied correctly including amplification within safe limits; accurate priming and trimming for AAC and HE-AAC (edit lists) so gapless is sample-accurate; a clean fall back to the ordinary platform player when the engine cannot decode a file; the audiobook comfort rewind after a long pause enforced even while JavaScript is suspended (same thresholds the server lane chose).
3. **Apple compressed target** (B-10.11): the bitrate-limited compressed render target the music lane describes.
4. **Deep seeking** that is accurate across the codecs the engines support, with fixtures generated in tests where possible.
5. WEBP embedded artwork if it is still unsupported, and a clear policy for when normalisation data is absent.
6. Every other confirmed engine-side bug in document 10 that is still present. List each in your report.

## Boundaries
- Video playback is not yours: the **web-player** lane owns the web video player and the **apple-mpv** lane owns the native video engine modules and the player screen. Touch only the music and audiobook engines and their JavaScript controllers.
- The **web-app** lane is working across `web/` and `client-core` outside playback. Keep to the audio modules.
- Server-side decoding, analysis and conversion command construction belong to other lanes; if the server must change, describe exactly what is needed.
- Nothing can be run on a device and no Xcode or Gradle exists here. Type-checks and single test files only. Write exact device test notes per platform (what to play, what to listen for, what pass and fail sound like).
