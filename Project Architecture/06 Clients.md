# Clients

The web app is React DOM. The shared native app is React Native with platform adapters for iPhone, iPad, Apple TV, Android, Android TV and Fire TV. Vega imports shared screens and supplies its own player, storage and capability bridge. Roku is outside this programme.

## Shared authority

Canonical client-core, design and i18n packages live in portico-server and are mirrored into the native repository. Headless services own requests, viewer scope, playback coordination and parsing; screens render server-composed content and own focus/drafts. Tolerant document parsing keeps usable rows and counts skips without weakening exact credentials, ownership, roles, limits or hostile-input bounds. Complete generated-type migration remains unfinished. [Web app](../../_program/reports/web-app.md) and [native sync](../../_program/reports/native-sync.md) record mirror and parser status.

## Playback

Web and native use PlaybackService → session controller → platform engine. The web port owns one video element and HLS integration. Native selects the presentation's native/mpv engine from the single capability document. Player and device overrides are shared settings. Source/quality/audio/subtitle changes retain the queue; native Player-setting changes still replace the current session.

Web subtitles have canvas/worker renderers for ASS/SSA, PGS, VobSub and DVB. Unsupported web paths, PiP and casting need fallback integration. Native caption size/background are implemented in the player but some Settings exposure is pending. Apple/Android mpv limitations are declared; package builds and real device rendering are unproven. Vega reports SDR, stereo and text subtitles and hides downloads, Cast, PiP and unsupported listening features.

Automatic quality is explicit and uses engine observations. Original remains the default. Direct streams without fragment measurements react to stalls. Sources: [web player](../../_program/reports/web-player.md), [native playback](../../_program/reports/native-playback.md), [web subtitles](../../_program/reports/web-subtitles.md), [Vega](../../_program/reports/vega.md).

## Freshness, lists and navigation

A stable viewer identity survives token renewal. One freshness hub per viewer/server follows named dependencies. It gathers event hints, waits briefly for idle input and rereads visible documents without blanking the screen. Covered screens catch up on reveal; background apps stop reads. A document pool retains selected previous pages. Browser tabs can share a stream where locks/channels are available; plain HTTP from another computer still has transport limitations.

History, playlists, collections and Saved views are windowed on native. Libraries remember each profile's tab, sort and filters on the device. Explicit links take precedence without changing that memory. Search text remains scoped in memory, outside normal route/history/restoration payloads. Home See all destinations come from the server. Appended lists, paged Saved data, some special screens, row focus and several error notices still need integration or acceptance. [Freshness clients](../../_program/reports/freshness-clients.md), [native app](../../_program/reports/native-app.md) and [personal state](../../_program/reports/personal-state.md) distinguish those limits.

Every subgroup must expose the main library sort/filter vocabulary. The server grouping implementation is still in progress; no all-platform parity claim is made.

## Sign-in and settings

Manual address entry tries HTTPS and HTTP candidates, including public addresses and IPv6. Apple transport configuration no longer limits HTTP to private addresses. A quiet note explains plaintext outside home networks. Server identity verification remains necessary; a wrong address must not erase saved credentials. Hosted activation uses a shared outage gate, with Retry-After floors separate from code polling cadence. Physical address/TV activation acceptance is still required.

Personal preferences and applicable consumer actions belong on all clients. Broad server administration remains web-first: credentials, providers/roots, restore, logs and global owner tools use a web handoff. Console follow-ups implement many owner screens, but native title actions, handoff scrub/skip, staged report strings and remaining localisation/focus work are open. [Console](../../_program/reports/console.md) records its later passes; [native sync](../../_program/reports/native-sync.md) records what reaches native.

## Acceptance

Type-checks and unit stand-ins do not establish browser layout, Apple scene lifecycle, native SDK linkage, media clocks, Cast, HDR, accessibility or television focus. Follow each source report's cold procedure. No Gradle/Xcode/full build was run by this documentation lane.
