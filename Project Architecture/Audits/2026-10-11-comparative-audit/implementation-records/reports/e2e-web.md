# Outcome

Production server and web builds run in real headless Chrome on mini-1 (`ehler-homelab-mini-1`), using loopback, disposable state, generated credentials and synthetic media. No Hosted or external service was used. The reusable direct-CDP runner needs no browser package installation.

All interaction groups pass in the final fresh-state replay at `_program/scratch/e2e-web/verified/`. All 114 screen captures (57 routes at both widths) rendered; all 57 action checks passed. The cold runner exited successfully and stopped its children; the exploratory server, Chrome and webhook receiver were also stopped. Fixed dialog CSP, image-subtitle reads and selection, capped AV1 conversion, stale version choices, author navigation and loading heroes, account-control permissions, and developer chunks in the production bundle.

Original video advances; a fixed 480p conversion advances; SRT has an active native cue; ASS loads the vendored WASM renderer and paints styled pixels; PGS selects client rendering and paints pixels. Text, styled and image subtitles work at 1440×1000 and 390×844. Automatic quality was additionally selected and played in the exploratory run (`screenshots/desktop-quality-automatic.png`). Next episode changes the source and advances. Music position advances. Downloads prepare and save the actual 18,152,637-byte original MKV.

API-triggered scanning adds a third version to an already-open title within 1746 ms in the final replay, without navigation, page reload or blanking the main content. Keys and member tokens read with their selected scopes; keys are refused after revocation, and the owner revokes the member token. A signed webhook test reaches a loopback receiver. Backup and support-export preparation complete.

# Decisions I made

- Kept fixtures local and deterministic: movie, TV, anime, FLAC music and M4B audiobook, local metadata, named SRT/ASS/PGS tracks and multiple film versions. Every cold run creates its own owner, server, Chrome profile and webhook receiver and stops its processes afterward.
- Authorized only the Radix scroll-lock stylesheet with a fresh document nonce. Nonced HTML is not cached; hashed assets retain immutable caching. Existing script, WASM, root-confinement and frame protections remain.
- Aligned rooted subtitle reads with the existing 512 MiB binary ceiling; metadata bounds remain separate. The owner's default is still 128 MiB.
- Used capped CRF for SVT AV1 because FFmpeg/SVT rejects the former constrained CBR arguments. The requested bitrate and buffer ceilings remain.
- Quiet refresh retains playback choices only within the same target and authority. Removed versions reset dependent track choices; permission loss clears retained data.
- Author cards, content views and book-context links use the normalized catalog public ID. Obsolete group tokens are rejected for author navigation. Series retain their own existing identity.
- Account security/device controls follow the server's `canManage` answer, bound to the session family that answered. A primary profile name alone does not confer a managing grant. Server authorization was not relaxed.
- Kept design changes out of this lane; precise visual and grouping-control gaps are below.
- Context sync could not acquire its read-only lock directory. Relevant canonical project, standards and verification guidance was read directly. No commits, push or deployment; cumulative snapshots are in `_program/snapshots/e2e-web/`.

# Bugs

No domain audit documents were assigned to this lane. The relevant confirmed register entries and additional browser findings are:

| Finding | Status and repair |
| --- | --- |
| B-16.18: dialog scroll-lock stylesheet blocked by CSP | Fixed with a fresh per-document style nonce; trusted sheet accepted and arbitrary inline sheet refused in Chrome. |
| B-16.24: disabled developer screens emitted into production | Fixed with direct build-time development branches; Gallery, Features and the three labs are absent from built assets. |
| Rooted PGS reads rejected even for a tiny file | Fixed inconsistent 32 MiB inventory bound versus the configured 128 MiB request bound; existing 512 MiB ceiling retained. |
| Unselected PGS wrongly advertised as conversion and selected same-language text | Fixed recognition of known bare delivery offers before the server pins a media URL; fetching and selected plans still require valid confined URLs. |
| Fixed-quality SVT AV1 playback failed | Fixed capped bitrate arguments; actual FFmpeg encode and advancing 480p browser playback pass. |
| New version missing from open pre-play picker | Fixed content-feed refresh and quiet retention; API-triggered update passes without reload or blanking. |
| Authors card opened an empty page | Fixed catalog public ID versus group-token mismatch, including book context and internal visibility references; cross-library and obsolete IDs refused. |
| Author/series hero remained in loading geometry | Fixed existing entity descriptor publication and exact shared-client parsing for these views. |
| Secondary/restricted profile sent forbidden account requests | Fixed account settings, device naming/listing and background remembered-account write using the session-bound managing grant; both switch directions produce zero forbidden requests. |

# API changes

No routes were added or removed, and no request/response schema shapes changed. Existing typed content responses now populate their optional entity descriptor for author/series heroes. Author entity IDs now consistently use the normalized catalog public ID in Authors and book context, matching the browse pivot; the old author group token is refused. All changed consumers in the shared content parser are included. Static HTML delivery adds a matching style nonce to its existing CSP and changes that document's cache policy to `no-store`.

# Schema changes

None. Fresh fixtures use the existing baseline.

# Owner or viewer settings added or wired

No persisted settings were added and no defaults changed. Existing per-session quality, subtitle choices, profile switching and download controls were exercised. Existing account-management controls now appear only when `/v1/direct.canManage` authorizes the current session family. Shared presentation adds an optional `manageDevices` capability alongside the existing sign-in capability; web supplies the authoritative value.

# Needed from others

- **Grouping sort/filter parity:** artist, album and author entity pages render their members but have no main-library sort/filter toolbar. Entity projections also expose narrower sorts. The content/frontend lane needs the same wide server-authored choices and query behavior here. Evidence: `verified/screenshots/desktop-artists-group.png`, `desktop-releases-group.png`, `desktop-authors-group.png` and phone equivalents. Genre grouping opens a filtered browse view with controls and passes. Do not interpret the successful navigation checks as grouping-wide sort/filter parity.
- **Unnamed subtitle labels:** two same-language unnamed SRT/ASS sidecars both read “English / File”, making the choice ambiguous. Exploratory screenshot: `screenshots/desktop-subtitle-menu.png`. Named fixtures distinguish and render correctly; frontend wording/format display remains.
- **Indistinguishable version labels:** same-resolution H.264 MP4 copies have the same “SD · H.264 · MP4” label. Exploratory screenshots: `screenshots/desktop-download-dialog.png` and `desktop-version-freshness.png`. Selection works; frontend needs a meaningful discriminator.
- Native lanes must sync the shared client changes, including author/series entity parsing, bare image delivery recognition, quiet playback-offer refresh and account-device capability support. Native UI and real-device acceptance were outside this browser lane.

All screenshot paths above are relative to `_program/scratch/e2e-web/`. No shared knowledge or audit documents were edited.

# Test notes for other environments

Final cumulative snapshot: `_program/snapshots/e2e-web/008/` (no commit was possible in this lane).

The final acceptance folder is `_program/scratch/e2e-web/verified/`: `summary.json`, `actions-*.json`, `walk-results.json`, `browser-events.jsonl`, `server.log`, screenshots and the saved original. Credentials and one-time secret screenshots are disposable scratch artifacts and must stay outside Git. Earlier `replay*`, `final-replay` and `acceptance` folders are investigation history, not the final pass: their driver/fixture races and the profile-permission finding were corrected.

Cold replay, from the worktree root (absolute fresh output; never reuse meaningful state):

```sh
npm run build --workspace portico-web -- --configLoader runner
GOFLAGS=-p=2 make build-server SERVER_OUTPUT=/absolute/scratch/portico-server
PORTICO_SERVER_BIN=/absolute/scratch/portico-server \
PORTICO_BROWSER_OUTPUT=/absolute/fresh-scratch \
node web/test/integration/browser-smoke.mjs
```

The runner generates media with Python/FFmpeg, boots a private loopback server, sets external proxies to an unusable loopback address, disables Hosted, blocks external Chrome requests, creates its owner through the UI, and tears down its children. Requires Node 24, Python 3, FFmpeg/FFprobe and Chrome; no npm install. Vite's default loader cannot write its config cache through this worktree's read-only node_modules symlink, hence `--configLoader runner`.

Verified on Chrome 154.0.8037.97, Linux software decode/encode. Production Go build and web TypeScript/build pass. Focused checks passed:

- `internal/httpapi`: nonce/HEAD/cache/protections and public-root confinement tests.
- `internal/storage`: subtitle binary versus metadata limit and candidate inventory tests.
- `internal/decoder`: two fixed-quality SVT tests, including a real tiny FFmpeg encode.
- `internal/catalog`: new author browse/context/hero regression, cross-library and obsolete-ID rejection; the focused `TestListening.*` slice also passed.
- Shared client individual test files: playback offers (18), subtitle originals (6), library content (29), settings scope (4).
- Script syntax checks and `git diff --check` pass. No whole-repo suites were run.

Three expected/recovered HTTP responses remain recorded: no setup-resume record on a fresh server (404), a conversion manifest requested before preparation finishes (503, HLS retries and video advances), and one offers read racing retirement of the previous episode session (404, next source advances). Navigation-aborted requests are retained in evidence. There are no uncaught application exceptions or external requests. The intentional negative CSP test logs its expected violation. Libass emits “Failed to load fonctconfig fonts!” while its shipped fallback font paints successfully; canvas pixel probes also produce a performance warning. These are recorded, not hidden. This host reports unavailable bubblewrap namespaces for FFmpeg sandboxing as an existing environment limitation; no protection was changed.

For other browsers/OSes, repeat the fresh-state run or the same user actions with a browser-native driver: pass means time advances at Original and fixed quality, active SRT cues, styled/image pixels, the next episode and music advance, the download bytes are saved, quiet version refresh stays visible, and all screen results render without exceptions or unexpected failed requests. Confirm cached HTML never pairs a document with a different CSP nonce. Validate fonts and device subtitle composition visually on Safari/Firefox and actual phones; Chrome viewport emulation establishes responsive web behavior only.


## Screen inventory

Every row below passed at desktop 1440×1000 and phone 390×844. Screenshots and per-screen text, controls, errors and route paths are in the linked files and `verified/walk-results.json`. Empty tabs are recorded as empty states, not populated-content acceptance.

| Screen | Desktop | Phone |
| --- | --- | --- |
| home | [Pass](../scratch/e2e-web/verified/screenshots/desktop-home.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-home.png) |
| search | [Pass](../scratch/e2e-web/verified/screenshots/desktop-search.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-search.png) |
| saved | [Pass](../scratch/e2e-web/verified/screenshots/desktop-saved.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-saved.png) |
| downloads | [Pass](../scratch/e2e-web/verified/screenshots/desktop-downloads.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-downloads.png) |
| settings | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings.png) |
| settings-profile | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings-profile.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings-profile.png) |
| settings-playback | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings-playback.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings-playback.png) |
| settings-languages | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings-languages.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings-languages.png) |
| settings-quality | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings-quality.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings-quality.png) |
| settings-music | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings-music.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings-music.png) |
| settings-appearance | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings-appearance.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings-appearance.png) |
| settings-region | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings-region.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings-region.png) |
| settings-privacy | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings-privacy.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings-privacy.png) |
| settings-device | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings-device.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings-device.png) |
| settings-about | [Pass](../scratch/e2e-web/verified/screenshots/desktop-settings-about.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-settings-about.png) |
| server-dashboard | [Pass](../scratch/e2e-web/verified/screenshots/desktop-server-dashboard.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-server-dashboard.png) |
| server-history | [Pass](../scratch/e2e-web/verified/screenshots/desktop-server-history.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-server-history.png) |
| server-libraries | [Pass](../scratch/e2e-web/verified/screenshots/desktop-server-libraries.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-server-libraries.png) |
| server-metadata | [Pass](../scratch/e2e-web/verified/screenshots/desktop-server-metadata.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-server-metadata.png) |
| server-streaming | [Pass](../scratch/e2e-web/verified/screenshots/desktop-server-streaming.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-server-streaming.png) |
| server-remote | [Pass](../scratch/e2e-web/verified/screenshots/desktop-server-remote.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-server-remote.png) |
| server-storage | [Pass](../scratch/e2e-web/verified/screenshots/desktop-server-storage.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-server-storage.png) |
| server-schedule | [Pass](../scratch/e2e-web/verified/screenshots/desktop-server-schedule.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-server-schedule.png) |
| server-troubleshooting | [Pass](../scratch/e2e-web/verified/screenshots/desktop-server-troubleshooting.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-server-troubleshooting.png) |
| server-general | [Pass](../scratch/e2e-web/verified/screenshots/desktop-server-general.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-server-general.png) |
| integrations-keys | [Pass](../scratch/e2e-web/verified/screenshots/desktop-integrations-keys.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-integrations-keys.png) |
| integrations-tokens | [Pass](../scratch/e2e-web/verified/screenshots/desktop-integrations-tokens.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-integrations-tokens.png) |
| integrations-webhooks | [Pass](../scratch/e2e-web/verified/screenshots/desktop-integrations-webhooks.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-integrations-webhooks.png) |
| integrations-assistants | [Pass](../scratch/e2e-web/verified/screenshots/desktop-integrations-assistants.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-integrations-assistants.png) |
| people-accounts | [Pass](../scratch/e2e-web/verified/screenshots/desktop-people-accounts.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-people-accounts.png) |
| people-invitations | [Pass](../scratch/e2e-web/verified/screenshots/desktop-people-invitations.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-people-invitations.png) |
| people-devices | [Pass](../scratch/e2e-web/verified/screenshots/desktop-people-devices.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-people-devices.png) |
| live-sources | [Pass](../scratch/e2e-web/verified/screenshots/desktop-live-sources.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-live-sources.png) |
| live-recording | [Pass](../scratch/e2e-web/verified/screenshots/desktop-live-recording.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-live-recording.png) |
| live-channels | [Pass](../scratch/e2e-web/verified/screenshots/desktop-live-channels.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-live-channels.png) |
| movie-discover | [Pass](../scratch/e2e-web/verified/screenshots/desktop-movie-discover.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-movie-discover.png) |
| movie-browse | [Pass](../scratch/e2e-web/verified/screenshots/desktop-movie-browse.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-movie-browse.png) |
| movie-collections | [Pass](../scratch/e2e-web/verified/screenshots/desktop-movie-collections.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-movie-collections.png) |
| tv-discover | [Pass](../scratch/e2e-web/verified/screenshots/desktop-tv-discover.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-tv-discover.png) |
| tv-browse | [Pass](../scratch/e2e-web/verified/screenshots/desktop-tv-browse.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-tv-browse.png) |
| tv-aired | [Pass](../scratch/e2e-web/verified/screenshots/desktop-tv-aired.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-tv-aired.png) |
| tv-collections | [Pass](../scratch/e2e-web/verified/screenshots/desktop-tv-collections.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-tv-collections.png) |
| anime-discover | [Pass](../scratch/e2e-web/verified/screenshots/desktop-anime-discover.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-anime-discover.png) |
| anime-browse | [Pass](../scratch/e2e-web/verified/screenshots/desktop-anime-browse.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-anime-browse.png) |
| anime-aired | [Pass](../scratch/e2e-web/verified/screenshots/desktop-anime-aired.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-anime-aired.png) |
| anime-collections | [Pass](../scratch/e2e-web/verified/screenshots/desktop-anime-collections.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-anime-collections.png) |
| music-discover | [Pass](../scratch/e2e-web/verified/screenshots/desktop-music-discover.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-music-discover.png) |
| music-browse | [Pass](../scratch/e2e-web/verified/screenshots/desktop-music-browse.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-music-browse.png) |
| music-releases | [Pass](../scratch/e2e-web/verified/screenshots/desktop-music-releases.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-music-releases.png) |
| music-songs | [Pass](../scratch/e2e-web/verified/screenshots/desktop-music-songs.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-music-songs.png) |
| music-genres | [Pass](../scratch/e2e-web/verified/screenshots/desktop-music-genres.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-music-genres.png) |
| music-playlists | [Pass](../scratch/e2e-web/verified/screenshots/desktop-music-playlists.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-music-playlists.png) |
| music-unmatched | [Pass](../scratch/e2e-web/verified/screenshots/desktop-music-unmatched.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-music-unmatched.png) |
| audiobook-discover | [Pass](../scratch/e2e-web/verified/screenshots/desktop-audiobook-discover.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-audiobook-discover.png) |
| audiobook-browse | [Pass](../scratch/e2e-web/verified/screenshots/desktop-audiobook-browse.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-audiobook-browse.png) |
| audiobook-authors | [Pass](../scratch/e2e-web/verified/screenshots/desktop-audiobook-authors.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-audiobook-authors.png) |
| audiobook-series | [Pass](../scratch/e2e-web/verified/screenshots/desktop-audiobook-series.png) | [Pass](../scratch/e2e-web/verified/screenshots/phone-audiobook-series.png) |

First-run setup was also captured at both widths (`desktop-first-run.png`, `phone-first-run.png`), followed by successful owner creation. Profile/HTTP sign-in and sign-out screenshots are in the same folder. The separate action files cover title/version selection, both-width subtitles, fixed quality, episode/music playback, group navigation, sorting/filtering, all five library configuration tabs, backup/export, webhook delivery, profiles, tokens, saved downloads and CSP/key checks. All 57 pass.

# Not finished

No verified runtime defect remains unresolved in the exercised paths. The grouping-control and two label issues above remain for the frontend/content lane, as required by the no-redesign boundary. Live TV sources/channels/recording settings and empty states were walked; tuner ingest and DVR playback need real/local stream fixtures and were not exercised. No external metadata/provider lookup, Hosted flow, certificate issuance, remote TLS/reverse proxy, physical casting/Watch Together peer, hardware conversion, HDR or non-PGS image-subtitle format was exercised. Audiobook Authors and book context were populated; the Series tab was an empty-state check. Backups and support exports were created; destructive restore was not attempted. These limitations do not establish release-wide acceptance.
