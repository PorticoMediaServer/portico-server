# Lane: downloads — offline copies that are complete, resumable and durable

Two worktrees, both yours, both committable:
- Server, web and the canonical shared packages: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-downloads` (branch `audit-implementation/downloads`, repository portico-server). `node_modules` is a symlink; do not install.
- Native apps: `/home/justin/projects/Portico/.worktrees/audit-implementation/native-downloads` (branch `audit-implementation/downloads`, repository portico-react-native). `packages/*` there are copies of the server repository's packages: change the canonical package in the server worktree, then copy it across with `PORTICO_SERVER_REPO=/home/justin/projects/Portico/.worktrees/audit-implementation/server-downloads node scripts/shared-packages.mjs --write` run in the native worktree, and commit both.
Your report is `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/downloads.md`.

Audit documents: 14 (all), 12 (B-12.5 offline plays), 22 (offline and reconnection entries); verification V08, G3. Decision register D-43. `fix-pass/lane-reports/report-social-downloads.md` and `FIX-LOG-2026-10-12.md` say what the fix pass already did.

## What to deliver
1. **A device copy outlives server tidying** (B-14.1): a downloaded file must not be deleted from the device because the server's retention for the claim lapsed, the title's file was replaced, or the title was removed from the server. The copy stays playable offline until the person removes it or the owner revokes that person's access. Show plainly when a copy's title no longer exists on the server.
2. **Everything a download needs travels with it** (A-14.21): title, description, artwork (poster and backdrop at device-appropriate size), chapters and markers, every text subtitle track and the styled or image originals the device can render, the audio tracks chosen, and for episodes the show and season facts; for albums and audiobooks the track list, cover and chapters. Offline browsing of downloads shows real titles and artwork with no request to the server.
3. **Resume instead of restart** (B-14.2): a failed or paused transfer continues from where it stopped using the range support the server already has, across app restarts and network changes, with integrity still verified at the end.
4. **Addresses that outlive an OS pause** (B-14.3): a download address valid for ten minutes is shorter than a suspended app; renew transparently.
5. **Originals are served, not copied** (A-14.2): an "original" download is copied into the state folder and hashed twice before a byte is sent. Serve the original file directly with a verifiable identity for the bytes, without the copy.
6. **Quality options that are real** (B-14.4): four of the six offered qualities cannot be encoded. Offer exactly what the server can produce. Another lane is adding HEVC, HDR-preserving and multichannel targets to the conversion pipeline right now; build the option list from what the conversion code reports it can produce, so new targets appear without further work here, and until then offer only what works.
7. **Offline plays count** (B-12.5, B-14.10, A-14.29): progress and plays made offline are sent when the device reconnects and go through the same ordering rules as online progress, so they reach history and Continue Watching without overwriting newer state from another device.
8. Every other confirmed bug in document 14 that is still present, on server, in `client-core`, on native and (where the web app has downloads) on web.

## Boundaries
- You own the `downloads` and `preparedmedia` server packages, the downloads modules of `client-core`, and the native download manager, storage and download screens (JavaScript, Swift and Kotlin).
- The **identity** lane is changing how titles and files relate (a title has versions; personal state attaches to the title) and owns personal state storage. Use the personal-state write path as it exists and keep your calls to it in one place.
- The **playback-core** lane owns playback sessions and the planner; the **conversion-outputs** lane owns FFmpeg; the **subtitles-text** lane owns subtitle delivery routes (it is adding routes that deliver styled and image originals). The **apple-mpv** lane owns the native player engine modules: do not edit them; an offline file is played through the existing player path.
- The **web-app** lane is working in `web/` and in the non-playback parts of `client-core` right now; keep to the downloads modules.
- Nothing can be run on a device. Type-checks and single test files only; no Gradle, no Xcode. Write exact device test notes: iPhone, iPad, Android phone, Fire tablet if relevant.
