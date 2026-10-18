# Bug register — all 588 first-pass claims with the second review's verdicts

Generated from the 23 domain documents that list bugs and the ten verification documents in `verification/`. **First pass** is the original auditor's confidence and severity. **Second review** is GPT 6.1 Sol's independent verdict; "not re-checked" means the claim was Low severity and was left as written. **Class** separates genuine defects from deliberate trade-offs, unfinished features and latent design risks. Sizes: S hours, M days, L a week or more. All findings are from reading source; nothing was run on a device. Full text and code references: the domain document for the first pass, the `verification/V..` document for the second.


## 01 — Identity and access

Source: `01-identity-users-access.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-01.1 | An access schedule signs people out instead of pausing them | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-01.2 | "Remote sign-in: owner only / off" is not applied on the sign-in routes the apps use | Confirmed by reading / High (a security setting that silently does not ho | Confirmed | Defect | High | M | No |
| B-01.3 | API keys can be created but never authenticate | Confirmed by reading / Medium (advertised function does nothing; owners w | Confirmed | Unfinished feature | Medium | M | No |
| B-01.4 | "Require approval before a device can play" does nothing | Confirmed by reading / Medium (false sense of security) | Confirmed | Defect | Medium | M | No |
| B-01.5 | One approved-but-never-collected TV code stops identity clean-up for good | Confirmed by reading (driver behaviour for NULL into string  / Low to Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-01.6 | A TV code cannot complete when new devices need approval (latent) | Confirmed by reading / Medium, latent (no first-party screen sets the pol | Confirmed | Defect | Medium | S | No |
| B-01.7 | The approve-new-devices policy can lock the owner out, and members' devices cannot be approved (latent) | Confirmed by reading / Medium, latent (same policy as B-01.6) | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-01.8 | hostedAttached is always false in release builds | Probable (I did not confirm what the web page shows for a cl / Medium for servers set up with a Portico Account;  | Confirmed, narrower | Defect | Medium | S | No |
| B-01.9 | Any 403 on refresh deletes the stored sign-in | Confirmed code path; whether a real deployment produces such / Low to Medium | Confirmed | Defect | Medium | S | No |
| B-01.10 | The two "highest rating" controls disagree | Confirmed by reading / Medium (the per-account control is the one the web | Confirmed | Defect | Medium | M | No |
| B-01.11 | Some common rating spellings are read as "unrated" | Confirmed by reading / Low (errs towards hiding when unrated is blocked,  | Not re-checked | — | Low (first pass) | — | — |
| B-01.12 | Erasing a person leaves their name in the owner's play history | Confirmed by reading / Low to Medium (two written rules conflict; a priva | Confirmed, narrower | Deliberate trade-off | Low | S | No |
| B-01.13 | Invite-only self-registration can never succeed (latent) | Confirmed by reading / Low, latent | Not re-checked | — | Low (first pass) | — | — |
| B-01.14 | Two different tests for "is this request local?" | the inconsistency is Confirmed by reading; the container exp / Medium for servers set up with a Portico Account b | Confirmed, narrower | Defect | Medium | M | No |
| B-01.15 | Two hashing slots, no queue | Speculative (depends on hardware and timing) / Low to Medium | Confirmed, narrower | Design risk | Low | S | No |
| B-01.16 | Profile PIN is checked while holding the database's single writer | Confirmed by reading / Low (cost 8 is tens of milliseconds, longer on a w | Not re-checked | — | Low (first pass) | — | — |
| B-01.17 | The installation id is treated as a secret but is shown and sent in URLs | Confirmed by reading / Low (what leaks is a list of names; refresh still  | Not re-checked | — | Low (first pass) | — | — |
| B-01.18 | Ended session records are never deleted | Confirmed by reading / Low (slow growth; the sessions list also returns e | Not re-checked | — | Low (first pass) | — | — |
| B-01.19 | Some revocations skip the playback clean-up hook | Probable / Low | Not re-checked | — | Low (first pass) | — | — |
| B-01.20 | Refusals that answer 401 where the project's own rule says 403 | Confirmed by reading; client reaction not traced / Low | Not re-checked | — | Low (first pass) | — | — |
| B-01.21 | A restricted profile can do account-level things | Confirmed by reading / Low (nuisance, mostly by design) | Not re-checked | — | Low (first pass) | — | — |
| B-01.22 | Left-over code from earlier identity designs | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-01.23 | Related Backlog item: two Apple sign-ins ended with explicit_signout | Speculative / as | Unsettled — needs runtime | Design risk | Medium | M | Yes (incident) |

## 02 — Playback negotiation

Source: `02-playback-decision-and-protocol.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-02.1 | Native devices are told HEVC may travel in MPEG-TS HLS | the planning path is confirmed by reading; the device failur / High (Apple is the reference client; HEVC MKV is t | Confirmed, narrower | Defect | High | M | No |
| B-02.2 | Passthrough codecs are always rejected for v1-publishing devices | Confirmed by reading / Medium to High for home-theatre users on Android T | Confirmed, narrower | Defect | High | M | No |
| B-02.3 | "Can switch audio tracks inside a file" is lost for native devices | Confirmed by reading / Medium (server work and loss of in-file extras, no | Confirmed | Defect | Medium | S | No |
| B-02.4 | "The device tone-maps HDR itself" is never sent | Confirmed by reading / Medium | Confirmed | Defect | High | M | No |
| B-02.5 | Other facts lost or bent in the v1 translation | Confirmed by reading / Low individually; together with B-02.1 to B-02.4 t | Not re-checked | — | Low (first pass) | — | — |
| B-02.6 | The player's Quality menu, version choice and audio re-plan never reach the server | the dropped parameters are confirmed by reading; the visible / High (user-visible controls that do nothing; also  | Confirmed, narrower | Defect | High | M | Related source merge only |
| B-02.7 | A start without an Idempotency-Key takes the legacy path and skips caps | Confirmed by reading / Medium to High (any signed-in member can bypass a  | Confirmed, narrower | Defect | High | M | No |
| B-02.8 | Re-watching a watched title marks it unwatched | Confirmed by reading; whether it is intended is unknown (the / Medium | Confirmed | Deliberate trade-off | Low | S | Specified behavior |
| B-02.9 | Two "remote" classifiers disagree | Confirmed by reading / Medium for VPN users (LAN-quality expectations, re | Confirmed | Defect | Medium | M | No |
| B-02.10 | On the cellular lane, HDR titles are converted even at "original" quality | Confirmed by reading / Low to Medium (surprising, expensive, and invisibl | Confirmed | Defect | Medium | M | No |
| B-02.11 | The default version is arbitrary | Confirmed by reading / Low today (the backlog says same-title files are n | Confirmed | Design risk | Low | M | Related source merge only |
| B-02.12 | Turning on a burned-in subtitle re-plans without the session's ceilings | Confirmed by reading / Medium (cap bypass on burn-in today; burn-in broke | Confirmed | Defect | High | M | No |
| B-02.13 | The timeline's error is accepted and ignored | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-02.14 | A device without MPEG-TS HLS can never receive a conversion | Confirmed by reading / Low for first-party clients (all declare TS), Medi | Confirmed | Unfinished feature | Low | M | No |
| B-02.15 | hev1-tagged HEVC in MP4 is direct-played to AVPlayer | Probable (device behaviour is well documented; not run) / Low | Not re-checked | — | Low (first pass) | — | — |
| B-02.16 | Route-failure memory: risks of the design | Confirmed by reading / Low to Medium (design risk, not a defect) | Confirmed, narrower | Design risk | Low | M | No |
| B-02.17 | Two language tables have drifted apart | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-02.18 | A file of unknown bitrate passes every bitrate ceiling | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-02.19 | "Transcoding off" also blocks copy-only remux | Confirmed by reading / Low to Medium (an owner who disables conversion on | Confirmed | Defect | Medium | S | No |
| B-02.20 | ended marks a title watched at any position | Speculative; depends on each engine never reporting ended sh / Low to Medium | Confirmed, narrower | Design risk | Low | M | No |
| B-02.21 | A re-plan resumes from the last reported position unless the client sends one | Speculative (I did not trace whether clients include seek wi / Low | Not re-checked | — | Low (first pass) | — | — |
| B-02.22 | Latent: the experimental "finite VOD" switch converts everything | Confirmed by reading that the switch exists and what it rewr / Low (off by default) | Not re-checked | — | Low (first pass) | — | — |

## 03 — Transcoding

Source: `03-transcoding-pipeline.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-03.1 | NVIDIA, Quick Sync and AMD encodes probably do not cut on the six-second grid | Probable — from FFmpeg encoder behaviour I know well plus Je / High | Confirmed, narrower | Defect | High | M | No |
| B-03.2 | The NVIDIA "native" path can never be enabled with Portico's own FFmpeg | Confirmed by reading (and against FFmpeg's public filter doc / Medium (works, but a GPU owner gets a fraction of  | Confirmed | Unfinished feature | Medium | M | No |
| B-03.3 | The VA-API "native" probe very likely fails on every host | Probable (FFmpeg behaviour from source; Portico's probe from / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-03.4 | Software HDR conversions fail inside the Linux sandbox when Vulkan (or OpenCL) was detected | Probable (straightforward from reading; needs one run to con / High where the conditions hold | Confirmed, narrower | Defect | High | M | No |
| B-03.5 | Remote-source conversions get no GPU access and no error text | Probable / Medium (only for owners using remote sources on a  | Confirmed, narrower | Defect | Medium | M | No |
| B-03.6 | The hardware circuit breaker cannot tell a bad GPU from a bad file | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-03.7 | Live/Library-Channel conversions: the safe keyframe expression is overridden by the unsafe one | Confirmed by reading for the argument order; Probable for th / Medium | Refuted | Design risk | Low | S | No |
| B-03.8 | The fixed 4 GiB per-session ceiling fights the owner's retention setting | Confirmed by reading for the mechanism; the user-visible eff / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-03.9 | MPEG-TS and other non-indexed sources can probably never be direct-streamed | Probable (demuxer behaviour from knowledge of FFmpeg; needs  / Medium | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-03.10 | The temporaryDirectory and maxBackgroundSessions owner settings do nothing | Confirmed by reading (searched every non-test Go file for bo / Medium (misleading setting; disk-wear and capacity | Confirmed | Unfinished feature | Medium | M | No |
| B-03.11 | A failed conversion's diagnostic is deleted almost immediately | Confirmed by reading / Medium (supportability) | Confirmed, narrower | Defect | Medium | S | No |
| B-03.12 | Converted video with copied audio: audio from before the seek point is included | Speculative — needs a runtime check on Apple and Cast player / Low to Medium | Unsettled — needs runtime | Design risk | Low | M | No |
| B-03.13 | Odd-sized sources fail to convert when no size limit applies | Confirmed by reading for the missing filter; Probable for th / Low | Not re-checked | — | Low (first pass) | — | — |
| B-03.14 | Prepared versions are forced to 30 fps | Confirmed by reading / Medium (every offline download and optimized versi | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-03.15 | Converted audio keeps a 96 kHz (or 88.2 kHz) sample rate | Confirmed by reading; device impact Probable / Low to Medium (rare sources, but silent audio when | Confirmed, narrower | Defect | Medium | M | No |
| B-03.16 | Declared codec string does not describe the encoded stream | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-03.17 | Latent trap: the keyframe expression depends on t starting at zero | Confirmed by reading / Low (latent) | Not re-checked | — | Low (first pass) | — | — |
| B-03.18 | Latent trap: folders with no database row are only removed under disk pressure | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-03.19 | Performance risk: 4K live channels decoded on one thread | Speculative on the numbers / Low to Medium | Unsettled — needs runtime | Design risk | Medium | S | No |

## 04 — Subtitles, markers, trickplay

Source: `04-subtitles-tracks-markers-trickplay.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-04.1 | Sidecar subtitle files are never imported automatically, so they are never auto-selected | Confirmed by reading / High (core expectation of a media server) | Confirmed, narrower | Unfinished feature | High | M | No (related Cast gap only) |
| B-04.2 | The scan imports an arbitrary eight text tracks, not the useful ones | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-04.3 | One bad or empty text track blocks the rest of the file's subtitles and all deep analysis | Confirmed by reading (logic) / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-04.4 | The 90-second budget covers up to eight full reads of the file | Probable (arithmetic on typical disk speeds; needs a timed r / High for remux libraries | Confirmed, narrower | Defect | High | M | No |
| B-04.5 | A file with more than 32 subtitle tracks gets none | Confirmed by reading / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-04.6 | Common sidecar names are disabled as "ambiguous" | Confirmed by reading / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-04.7 | A second video with the same base name disables every sidecar for both | Confirmed by reading / Low–Medium | Confirmed | Deliberate trade-off | Low | S | Related version-grouping item only |
| B-04.8 | The client has no message for the server's "ambiguous" reason | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-04.9 | Non-Western legacy encodings are shown as garbage, silently | Confirmed by reading / Medium (High for affected languages) | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-04.10 | One odd line makes a whole ASS script unusable | Confirmed by reading; prevalence Probable / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-04.11 | Image subtitle tracks over 32 MiB cannot be used | Probable (size expectation from typical discs; the cap itsel / Medium–High for Blu-ray remux libraries | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-04.12 | First use of an image track must read the whole file inside a 110-second request | Probable / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-04.13 | Burned image subtitles may be squashed or clipped when the video is not the subtitle's shape | Speculative — depends on how FFmpeg's bitmap pipeline treats / Medium if real | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-04.14 | Burned-in styled subtitles can lose their glyphs or fonts | Probable / Medium for anime/CJK | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-04.15 | Audio language preferences fail for languages outside a 57-entry table | Confirmed by reading / Medium for those languages | Confirmed | Defect | Medium | S | No |
| B-04.16 | Forced-track matching uses a second, smaller table | Confirmed by reading / Low–Medium | Confirmed | Defect | Medium | S | No |
| B-04.17 | The hearing-impaired flag from the file is ignored | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-04.18 | Forced tracks with no language tag are never auto-selected; file "default" flags are ignored | Confirmed by reading (design gap) / Low–Medium | Confirmed, narrower | Deliberate trade-off | Low | S | No |
| B-04.19 | Online search for an episode sends only the episode's title | Confirmed by reading / Medium (feature mostly unusable for TV) | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-04.20 | The OpenSubtitles sign-in is a static environment variable | Probable (relies on the provider's documented behaviour, not / Low–Medium | Confirmed, narrower | Unfinished feature | Low | M | No |
| B-04.21 | Unreviewed luminance guesses are shown to viewers as Skip buttons, and the intro guess is often backwards | Confirmed by reading / Medium (owners who enable "Complete" will see wron | Confirmed, narrower | Design risk | Medium | M | No |
| B-04.22 | The same stretch is emitted as both "intro" and "recap" | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-04.23 | One imperfect chapter discards all chapters | Confirmed by reading (rule); the non-zero-start case is Spec / Low–Medium | Confirmed, narrower | Deliberate trade-off | Low | S | No |
| B-04.24 | A title-page subtitle choice can silently do nothing | Confirmed by reading / Medium (looks like a broken control) | Confirmed, narrower | Defect | Medium | M | No |
| B-04.25 | Curly-brace text is deleted from SRT and WebVTT | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-04.26 | Overlapping cues share italic and top placement on the native overlay | Confirmed by reading for Apple; Android header suggests the  / Low | Not re-checked | — | Low (first pass) | — | — |
| B-04.27 | Flat folders with more than 4,096 entries lose all sidecars, and discovery re-lists the folder for every video | Confirmed by reading (cap); Probable (cost — depends on whet / Medium for flat libraries | Confirmed, narrower | Design risk | Medium | M | Related metadata cache item only |
| B-04.28 | Any decode error aborts an analysis stage | Probable / Low–Medium | Confirmed, narrower | Deliberate trade-off | Low | S | No |
| B-04.29 | Every trickplay frame is stored twice | Confirmed by reading (unless an owner-facing view uses the s / Low | Not re-checked | — | Low (first pass) | — | — |
| B-04.30 | Preview images are re-downloaded every five minutes | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-04.31 | On a Windows server no embedded subtitle can be used | Confirmed by reading / High if Windows servers are in scope, otherwise no | Confirmed, narrower | Defect | High | S | No |
| B-04.32 | Subtitle reads and a viewer's subtitle selection queue for the single write slot at background priority | Confirmed by reading (classes and call sites); runtime effec / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-04.33 | A vestigial second subtitle-plan system still writes rows, and several functions are dead | Confirmed by reading (grep for callers across internal/ and  / Low | Not re-checked | — | Low (first pass) | — | — |
| B-04.34 | Embedded subtitles in some containers cannot be extracted | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-04.35 | "Complete" scanning is far heavier than its label suggests (design risk) | Confirmed by reading / Medium on weak hardware (weeks of background load  | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-04.36 | The v1 session says "webvtt" but the URL returns JSON | Confirmed by reading / Medium for the "implementable by a third party" pr | Confirmed | Defect | Medium | M | No |
| B-04.37 | HLS subtitle renditions carry no default, auto-select or forced signalling (design risk) | Confirmed by reading (playback/hls_renditions.go:190) / Low–Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-04.38 | HDR and anamorphic sources produce poor preview images | Probable / Low–Medium | Confirmed, narrower | Defect | Low | M | No |
| B-04.39 | The web renderer rebuilds every cue whenever the HLS mapping changes | Speculative — needs observation in a browser / Low | Not re-checked | — | Low (first pass) | — | — |
| B-04.40 | The owner-facing description of segment detection does not match what runs | conf: / sev: | Confirmed | Documentation error | Medium | S | No |

## 05 — Libraries, scanning, naming

Source: `05-libraries-scanning-naming.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-05.1 | Scan speed has a hard ceiling of about three steps per second per folder source | Confirmed by reading (the cadence); the table is derived / High — it collides with the "5–10 million items" t | Confirmed, narrower | Defect | High | S | No |
| B-05.2 | Any folder changing during a scan fails the whole scan — and then nothing gets analysed | Confirmed by reading / High on busy libraries | Confirmed, narrower | Deliberate trade-off | High | M | No |
| B-05.3 | A file that changes after it was first seen stays stale — and probably unplayable — for up to a week | Confirmed by reading for the stale record; Probable for "unp / High | Confirmed, narrower | Defect | High | M | Yes — stale facts; consequence adds deta |
| B-05.4 | One unreadable sub-folder stops every scan of that source | Confirmed by reading / High when it applies (a library whose root is a mo | Confirmed | Defect | High | M | No |
| B-05.5 | With "follow symlinks" on, one dangling or out-of-root link stops the scan | Confirmed by reading / Medium (only when the option is on, but symlink-bu | Confirmed | Deliberate trade-off | Medium | M | No |
| B-05.6 | Hidden and system files are catalogued as media | Confirmed by reading / High (any library on a NAS share used from a Mac) | Confirmed | Defect | Medium | M | No |
| B-05.7 | A year inside a title is taken as the release year | Confirmed by reading / Medium-High (well-known titles) | Confirmed, narrower | Defect | Medium | S | No |
| B-05.8 | A second file for a movie is a second movie; an upgraded file loses the title's personal state | Confirmed by reading / High | Confirmed, narrower | Unfinished feature | High | L | Yes — second-file grouping |
| B-05.9 | Renaming a file to fix it does not change how Portico read it | Confirmed by reading / Medium (it is the first thing an owner tries) | Confirmed, narrower | Defect | Medium | M | No |
| B-05.10 | Flat show folders, and the show folder's year, are only understood inside a Season N folder | Confirmed by reading (illustrated by the port) / High — flat folders are common for miniseries and  | Confirmed | Defect | High | M | No |
| B-05.11 | The folder-must-match rule rejects correctly organised shows | Confirmed by reading / High | Confirmed, narrower | Defect | High | M | No |
| B-05.12 | Resolution tags such as 1920x1080 are read as season × episode | Confirmed by reading / Medium (fansub releases often carry resolution tag | Confirmed | Defect | Medium | S | No |
| B-05.13 | Other correct names the episode reader refuses or shortens | Confirmed by reading / Medium (daily shows), Low (the rest) | Confirmed, narrower | Defect | Medium | M | No |
| B-05.14 | An extras folder name anywhere in the path swallows everything under it | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-05.15 | A ripped Blu-ray or DVD folder becomes dozens of titles | Confirmed by reading / Medium | Confirmed, narrower | Missing feature | Medium | M | No |
| B-05.16 | Samples and bare trailers become titles | Confirmed by reading / Medium (scene releases ship samples) | Confirmed | Defect | Medium | S | No |
| B-05.17 | Titles are left mangled until (and unless) a provider match replaces them | Confirmed by reading / Low-Medium | Confirmed, narrower | Defect | Low | S | No |
| B-05.18 | File identity includes the device number, which is not stable across reboots on many setups | Probable (the mechanism is confirmed by reading; that st_dev / High where it applies | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-05.19 | A directory's timestamp is not always evidence that nothing was added | Probable (mechanism confirmed; file-system behaviour from ge / Medium-High for mergerfs and cloud-mount users, wh | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-05.20 | Scan bookkeeping is never pruned | Confirmed by reading (absence of a delete, by search) / Medium-High over months; it works against "open da | Confirmed, narrower | Design risk | High | M | No |
| B-05.21 | Every scan re-queues every file | Confirmed by reading / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-05.22 | Looking for sidecars that are not there costs dozens of process starts per file | Confirmed by reading / Medium (it multiplies the first-import time of lar | Confirmed, narrower | Defect | Medium | M | Yes — missing-sidecar cache |
| B-05.23 | A new library never rescans by default | Confirmed by reading (not checked in the running web form) / Medium — a first-run impression problem | Confirmed | Deliberate trade-off | Medium | S | No |
| B-05.24 | Deleted files stay "available" for a day and are never removed on their own | Confirmed by reading / Medium (design risk; deliberate) | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-05.25 | Deleting media on another volume copies it into the server's state folder | Probable (read the move code; did not trace every caller) / Medium — a full state disk is a server outage | Confirmed, narrower | Design risk | Medium | M | No |
| B-05.26 | One unplaceable file makes a TV library permanently "degraded" | Confirmed by reading / Low-Medium (alarm fatigue) | Confirmed, narrower | Defect | Medium | S | No |
| B-05.27 | Show identity is "title + year", which both splits and merges wrongly | Confirmed by reading / Medium | Confirmed, narrower | Design risk | Medium | L | No |
| B-05.28 | On Windows the server cannot add a library | Probable (by reading; build tags followed, nothing run) / Critical if a Windows server is intended — the tre | Confirmed, narrower | Defect | Critical | M | No |
| B-05.29 | Local metadata files are rejected whole when they exceed a budget | Confirmed by reading for the limits; Probable for the encodi / Medium for local-only libraries | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-05.30 | A stopped scan does not say where it stopped | Confirmed by reading / Low-Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-05.31 | Multi-disc albums split when disc folders are not named exactly CD1 / Disc 1 | Confirmed by reading / Low-Medium | Confirmed, narrower | Missing feature | Low | S | No |
| B-05.32 | Accented names written two ways may not be recognised as equal | Speculative — needs a runtime check with real NFD and NFC na / Low-Medium | Confirmed | Defect | Medium | S | No |
| B-05.33 | With "follow symlinks" on, a folder alias inside the root may make every incremental scan fail on a database constraint | Probable (traced by reading; depends on nothing unusual, but / Medium (needs the symlink option and a directory a | Confirmed | Defect | Medium | S | No |

## 06 — Metadata and artwork

Source: `06-metadata-providers-artwork.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-06.1 | Titles that are or contain a year are parsed wrongly and end up unmatched | Confirmed by reading / High (well-known titles; silent) | Confirmed, narrower | Defect | Medium | M | No |
| B-06.2 | A title without a year never matches on its own, and nothing lists the leftovers | Confirmed by reading (the rule); the size of the effect depe / High as a first-run experience | Confirmed, narrower | Deliberate trade-off | Medium | M | No; broader Fix Match note |
| B-06.3 | The "is this slot already taken?" check for provider posters never matches | Confirmed by reading for the dead guard; Probable for the lo / Medium to High (owner's local posters silently rep | Confirmed | Defect | High | S | No |
| B-06.4 | Local artwork stops being accepted after 128 MB in total | Confirmed by reading / High for music and for any library relying on loca | Confirmed, narrower | Defect | High | M | No |
| B-06.5 | A season folder's poster.jpg becomes the show's poster; the usual season-poster name is not recognised | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-06.6 | TheTVDB titles get no studio and no network | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-06.7 | Whole categories of artwork are missing depending on the provider | Confirmed by reading / High (default configuration for TV) | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-06.8 | A TMDB show's cast is the latest season only | Probable (relies on TMDB's documented meaning of credits for / Medium | Unsettled — needs runtime | Design risk | Medium | S | No |
| B-06.9 | Changing a library's language or region does not refetch, and old text is relabelled as the new language | Confirmed by reading / Medium to High (the setting appears not to work; v | Confirmed, narrower | Defect | Medium | M | No |
| B-06.10 | Only a movie can be force-refreshed | Confirmed by reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-06.11 | First import of a large library takes days to weeks | Confirmed by reading (the limits); the timings are estimates / High against the stated scale principle | Confirmed, narrower | Design risk | Medium | L | Related re-walks only |
| B-06.12 | A provider or internet outage of about a quarter of an hour parks titles permanently | Confirmed by reading / Medium to High (a router reboot during first impor | Confirmed, narrower | Defect | Medium | M | No |
| B-06.13 | An owner's Fix Match search waits behind the background backlog | Confirmed by reading (the ordering); Probable for the user-v / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-06.14 | With the default settings no title has a content rating | Confirmed by reading for the metadata side; how restricted p / Medium (High if unrated titles are shown to restri | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-06.15 | A field once set by an NFO stays NFO-owned after the NFO is gone | Probable (I did not trace every path that could clear the ro / Low to Medium | Confirmed | Defect | Medium | M | Related NFO credits only |
| B-06.16 | One odd detail makes the whole NFO unreadable | Confirmed by reading; how common each trigger is in real lib / Medium | Confirmed | Defect | Medium | M | No |
| B-06.17 | A provider record with one over-long list is rejected whole | Speculative (needs a title that actually exceeds a limit; th / Low, but it would hit famous titles | Not re-checked | — | Low (first pass) | — | — |
| B-06.18 | TheTVDB artwork keeps the first 80 images in the provider's order | Probable (depends on TheTVDB's response order) / Low to Medium | Confirmed, narrower | Design risk | Low | S | No |
| B-06.19 | Logos and alternative images are chosen without regard to language | Probable / Medium (visible on hero banners) | Confirmed, narrower | Defect | Medium | S | No |
| B-06.20 | Cast photos are downloaded per title, at original size | Confirmed by reading; volume is an estimate / Medium (time, bandwidth, load on TMDB's image CDN  | Confirmed, narrower | Design risk | Medium | M | Related NFO photos only |
| B-06.21 | Absolute-numbered anime gets no episode data on the default provider | Confirmed by reading / High for anime libraries | Confirmed, narrower | Unfinished feature | Medium | L | No |
| B-06.22 | Every TMDB search includes adult titles | Confirmed by reading; whether it is intended is unknown / Low | Not re-checked | — | Low (first pass) | — | — |
| B-06.23 | Small costs that add up on weak hardware (design notes) | Confirmed by reading; impact unmeasured / Low | Not re-checked | — | Low (first pass) | — | — |
| B-06.24 | Adding one episode re-fetches every episode of the show; the freshness rules do not cover episodes | Confirmed by reading for the unguarded episode calls; Probab / High (it is the common weekly event for any airing | Confirmed | Defect | High | M | Related TVDB credits only |

## 07 — Data model and queries

Source: `07-data-model-persistence-query.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-07.1 | One permanently failing entity stops all second-tier derived workers | Confirmed by reading (control flow) / High if triggered, because nothing self-heals; Med | Confirmed | Defect | High | M | No |
| B-07.2 | Raising a derivation version takes browsing or search offline for the whole rebuild | Confirmed by reading for the mechanism; duration is an estim / High at target scale (it is the upgrade experience | Confirmed, narrower | Deliberate trade-off | High | L | No |
| B-07.3 | A new file at a moved movie's old path is attached to the moved movie | Probable (each step read; the interaction not run) / Medium | Confirmed | Defect | Medium | M | No |
| B-07.4 | Cursors and range revisions go stale on every progress report and every scan commit | Confirmed by reading for the mechanism; how visible it is de / Medium (design risk) | Confirmed, narrower | Design risk | Medium | M | No |
| B-07.5 | A search can fail with "the library changed" although it read one consistent snapshot | Confirmed by reading; frequency depends on commit rate durin / Medium | Confirmed | Defect | Medium | S | No |
| B-07.6 | Some titles cannot be found by search at all | Confirmed by reading (tokenizer behaviour reasoned from FTS5 / Medium | Confirmed, narrower | Missing feature | Medium | M | No |
| B-07.7 | Search finds Chinese, Japanese and Korean titles only from their first character | Confirmed by reading plus Navidrome's documented experience  / Medium for libraries with native-script titles (an | Confirmed, narrower | Missing feature | Medium | L | No |
| B-07.8 | Date filters compare raw text | Confirmed by reading / Low to Medium (wrong results, quietly; affects sav | Confirmed, narrower | Defect | Medium | S | No |
| B-07.9 | The seek-by-letter request breaks on non-ASCII letters | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-07.10 | The client's letter-bucket match disagrees with the server's letters | Confirmed by reading the function; I did not trace what valu / Low | Not re-checked | — | Low (first pass) | — | — |
| B-07.11 | DELETE /v1/items/{id} is a raw entity delete | Confirmed by reading for the delete and the FK; Probable for / Low to Medium (owner-only; the proper path is Purg | Confirmed, narrower | Defect | Medium | M | No |
| B-07.12 | Case and accent handling is ASCII-only in filters | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-07.13 | The backup pins one snapshot for the whole copy | Speculative; needs a runtime check at scale (the mechanism i / Medium at target scale | Confirmed, narrower | Design risk | Medium | M | No |
| B-07.14 | Migrations 0003, 0007, 0008 and 0010 do what the migration rules forbid | Confirmed by reading / Low now | Not re-checked | — | Low (first pass) | — | — |
| B-07.15 | Insert-only tables grow without bound | Confirmed by reading; sizes estimated / Low | Not re-checked | — | Low (first pass) | — | — |
| B-07.16 | Nothing reports a stuck write gate in production | Speculative as to occurrence (I found no such call path; cat / Low to Medium (latent trap) | Confirmed, narrower | Design risk | Low | S | No |
| B-07.17 | Search sort quirks | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-07.18 | Derived batches may hold the write gate for long stretches | Speculative; needs measurement (GateStats.MaxHeldMilli by cl / Low to Medium | Unsettled — needs runtime | Design risk | Low | M | No |
| B-07.19 | Opening the live database in another program | Speculative (standard SQLite behaviour) / Low | Not re-checked | — | Low (first pass) | — | — |

## 08 — API and real-time

Source: `08-api-realtime-extensibility.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-08.1 | Behind a TLS-terminating reverse proxy, the bundled web app's writes are refused as "origin not allowed" | Confirmed by reading (static) / High for anyone using a TLS-terminating proxy (the | Confirmed, narrower | Defect | Medium | S | No |
| B-08.2 | Any error a legacy handler does not recognise becomes 400 invalid_request, unlogged | Confirmed by reading / Medium (diagnosability; wrong retry behaviour) | Confirmed | Defect | Medium | M | No |
| B-08.3 | A reconnect that arrives before the server notices the old stream is dead demotes the client to long-polling for good | Confirmed by reading both sides / Low today, Medium once SSE is used | Confirmed, narrower | Defect | Low | S | No |
| B-08.4 | An SSE client that has not yet received an event has no cursor, so a drop loses whatever happened in the gap, silently | Confirmed by reading / Low today (SSE unused), Medium for streaming clien | Confirmed | Defect | Low | S | No |
| B-08.5 | The advertised long-poll maximum is 55 seconds; the server accepts 25 | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-08.6 | Every event wakes every connected client into several database reads (design risk at 200 browsers) | Confirmed by reading that the work happens; whether it matte / Medium as a design risk | Confirmed, narrower | Design risk | Medium | M | No |
| B-08.7 | Behind a reverse proxy, sign-in and device-code limits are shared by everyone | Confirmed by reading; the budget windows inside identity.Set / Medium for proxy deployments, nil otherwise | Confirmed, narrower | Defect | Medium | M | No |
| B-08.8 | Grid ETags are invalidated by any write anywhere and by token renewal | Confirmed by reading / Low (lost efficiency, never staleness) | Not re-checked | — | Low (first pass) | — | — |
| B-08.9 | Registry safety checks fail whole responses at run time | Confirmed by reading the mechanism; no failing handler was i / Medium as a latent trap | Confirmed, narrower | Design risk | Low | S | No |
| B-08.10 | The mismatch between error envelopes changes client retry behaviour | Confirmed by reading; which screens are affected was not tra / Low to Medium | Confirmed | Defect | Medium | S | Related only |
| B-08.11 | Toolkit pieces the documents describe are not what the server runs | Confirmed by search (no call sites outside tests) / Low | Not re-checked | — | Low (first pass) | — | — |
| B-08.12 | If-None-Match, ETag and encoding nits | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-08.13 | The registry's access label is not what is enforced | Confirmed by reading / Medium as a latent trap | Confirmed, narrower | Design risk | Low | M | No |
| B-08.14 | A full-library walk may not finish while the library is changing (needs a runtime check) | Speculative — depends on how often revisions move / Medium for integrators, Low for first-party screen | Confirmed, narrower | Deliberate trade-off | Medium | L | No |
| B-08.15 | Original files are always labelled video/mp4 | Confirmed by reading; impact on real players not tested / Low to Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-08.16 | Unauthenticated build fingerprint | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-08.17 | The realtime lane is one pool for every long-lived connection | Confirmed limits; reaching them is speculative / Low | Not re-checked | — | Low (first pass) | — | — |
| B-08.18 | Slow uploads other than avatars and artwork are cut at 15 seconds | Probable (Go applies ReadTimeout to the whole body); body si / Low (remote owners on slow uplinks) | Not re-checked | — | Low (first pass) | — | — |

## 09 — Server operations

Source: `09-server-operations-admin.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-09.1 | Restore, and the on-demand integrity check, run under a 10-second request budget | Confirmed by reading / High (restore is unusable at the scale Portico is  | Confirmed, narrower | Defect | High | M | No |
| B-09.2 | After a restore the server exits and nothing may start it again | Confirmed by reading (the packaging has no restart path) / High on Windows, Medium elsewhere | Confirmed, narrower | Defect | High | M | No |
| B-09.3 | Linux packages: a fresh install is not started, and an upgrade leaves the service stopped (deb) or stopped and disabled (rpm) | Confirmed by reading the scripts; the script ordering is sta / High for anyone updating a packaged install | Confirmed, narrower | Defect | High | S | No |
| B-09.4 | Windows: Scheduled Task defaults will stop the server after three days and on battery | Probable; needs a check on a Windows machine (the defaults a / High on Windows | Unsettled — needs runtime | Defect | High | S | No |
| B-09.5 | Windows: running as SYSTEM breaks network shares and has no firewall rule | Probable (standard Windows behaviour, not tested) / High for first-run success on Windows | Confirmed, narrower | Defect | Medium | M | No |
| B-09.6 | Two log files that grow without limit on Windows and macOS | Confirmed by reading / Medium (slow, but it is the state volume that fill | Confirmed | Defect | Medium | S | No |
| B-09.7 | A fixed 256-connection ceiling sits under the memory-based one, and it drops rather than queues | Confirmed by reading for the cap and the close; the user-vis / High against the "couple of hundred concurrent bro | Confirmed, narrower | Defect | Medium | S | No |
| B-09.8 | Plain HTTP is refused to Tailscale, carrier-grade-NAT and global-IPv6 peers, and the console disagrees with itself about them | Confirmed by reading / Medium to High for the self-hosting audience (Tail | Confirmed, narrower | Defect | Medium | M | No |
| B-09.9 | The "Folder for converted video" setting is saved, validated and reported, but never used | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-09.10 | The console offers a secure-connections choice the server rejects | Confirmed by reading / Low to Medium | Confirmed | Defect | Low | S | No |
| B-09.11 | The update check cannot work with the manifest the release pipeline publishes | Confirmed by reading both files / Medium (owners are never told a newer build exists | Confirmed | Defect | Medium | S | No; signing integration broadly known |
| B-09.12 | Low-storage and certificate-expiry notifications have no default trigger | Probable (I found no trigger in the server, the web app, cli / Medium; a silently filling disk is the classic way | Confirmed | Unfinished feature | Medium | S | No |
| B-09.13 | Deleted media is moved into the state folder and never leaves on its own | Confirmed by reading / High as a design risk (it converts a routine actio | Confirmed, narrower | Defect | High | M | No |
| B-09.14 | The systemd sandbox blocks things the console lets the owner do | Confirmed by reading the unit; each consequence follows from / Medium to High (features appear broken on the most | Confirmed, narrower | Defect | Medium | M | No |
| B-09.15 | The state-folder permission check walks everything, misfires on Windows and Docker, and its fix can touch media | Confirmed by reading for the walk and the missing guard; Pro / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-09.16 | Undoing a restore loads the whole database into memory | Confirmed by reading / Medium (it breaks the "put it back" path exactly w | Confirmed | Defect | Medium | S | No |
| B-09.17 | Scheduled backups can fail quietly, and several small robustness gaps | Confirmed by reading / Low individually; Medium together because backups  | Confirmed, narrower | Defect | Medium | M | No |
| B-09.18 | Backups can only live on the database's own volume | Confirmed by reading / Medium (the disk failure that destroys the databas | Confirmed, narrower | Design risk | Medium | S | No |
| B-09.19 | A restore brings back old sessions, API keys and passwords | Confirmed by reading / Medium as a risk an owner should be told about (re | Confirmed, narrower | Deliberate trade-off | Medium | S | No; Backups spec supersedes older comple |
| B-09.20 | On a filesystem without advisory locks, a restore restarts the server and then does nothing | Confirmed by reading / Low (narrow), but confusing when hit | Not re-checked | — | Low (first pass) | — | — |
| B-09.21 | The server listens on IPv4 only unless the service definition is edited | Confirmed by reading / Medium (it silently disables a remote-access path  | Confirmed | Unfinished feature | Medium | S | No |
| B-09.22 | Shutdown does not wait for in-flight HTTP requests | Confirmed by reading / Low (no data risk; clients retry), but it makes "g | Not re-checked | — | Low (first pass) | — | — |
| B-09.23 | The log an owner can read is thinner than the design suggests | Confirmed by reading / Medium for supportability | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-09.24 | Four different answers to "who is the client behind this proxy?" | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-09.25 | Unfriendly failures on two environment variables | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-09.26 | The container image: likely no hardware transcoding, no discovery on the default network, UTC schedules | Probable for hardware transcoding (I did not inspect the bun / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-09.27 | The portable archives do not run as unpacked | Confirmed by reading / Medium for those artifacts | Confirmed, narrower | Defect | Medium | S | No |
| B-09.28 | The Schedule page says jobs run in maintenance windows; only backups do | Confirmed by reading / Low to Medium (truthfulness of a settings surface; | Confirmed, narrower | Documentation error | Low | S | No |
| B-09.29 | A failed start is invisible outside the service log | Confirmed by reading / Low to Medium; see F-09.10 | Confirmed, narrower | Missing feature | Low | M | No |
| B-09.30 | A long backup holds back the write-ahead log | Speculative; needs a runtime check with a large database und / Low | Not re-checked | — | Low (first pass) | — | — |
| B-09.31 | macOS launcher leftovers | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-09.32 | recover-owner opens the live database without the instance lock | Confirmed by reading; the scenario needs mismatched binaries / Low | Not re-checked | — | Low (first pass) | — | — |
| B-09.33 | Health check versus "require HTTPS" on a specific bind address | Speculative / Low | Not re-checked | — | Low (first pass) | — | — |
| B-09.34 | Public endpoints say a little more than the contract allows | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-09.35 | Moving only the database file does not work, and a mismatched restore is not rolled back | Confirmed by reading for the first part; Speculative for the / Medium if the second part is real | Confirmed, narrower | Defect | High | M | No |

## 10 — Music and audiobooks

Source: `10-music-audiobooks-books.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-10.1 | An audiobook file is marked finished, and its resume point reset to zero, at 95% | Confirmed by reading / High for audiobooks | Confirmed | Defect | High | S | No |
| B-10.2 | All local artwork shares one 128 MiB folder; after that, covers are silently dropped | Confirmed by reading (I found no pruning of this folder othe / High; it contradicts the 5–10 million item goal | Confirmed, narrower | Defect | High | M | No |
| B-10.3 | "Play next" on a shuffled queue adds to the very end | Confirmed by reading / Medium (a visible everyday surprise) | Confirmed | Deliberate trade-off | Medium | S | No |
| B-10.4 | Removing or moving one entry in a shuffled queue reshuffles everything and forgets what was played | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-10.5 | A library-wide lyrics run that is rate-limited once races through the rest of the library as "missing" | Confirmed by reading / Medium (the feature quietly does nothing for large | Confirmed, narrower | Defect | Medium | S | No |
| B-10.6 | The bulk lyrics run publishes the first search hit without checking it is the same song | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-10.7 | Retagging a file hides its lyrics, and the bulk run will not fetch them again | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-10.8 | One unexpected line makes a whole LRC file invalid | Confirmed by reading; how often it bites is Probable / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-10.9 | End padding longer than one frame is under-counted | Probable (the Portico side is confirmed; the FFmpeg side is  / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.10 | HE-AAC is planned as plain "AAC" and will not decode on the web engine | Probable / Medium (with B-10.34 it is a playback failure, not | Confirmed, narrower | Defect | Medium | M | No |
| B-10.11 | A bitrate limit on an Apple device turns the audio plan off | Confirmed by reading / Medium | Confirmed | Unfinished feature | Medium | M | No |
| B-10.12 | Long files cannot be measured in the 5-second play-time budget, and each attempt is thrown away | Probable (decode speed is an estimate) / Medium on first import, Low afterwards | Confirmed, narrower | Defect | Medium | M | No |
| B-10.13 | A listener's conversion runs as background work and restarts from the file's beginning on every seek | Confirmed by reading / Low to Medium (weak boxes, long files) | Confirmed | Defect | Medium | M | Related only: web MP3/FLAC seek tables ( |
| B-10.14 | Narrators are read only from a narrator tag | Confirmed by reading / Medium for audiobook libraries | Confirmed, narrower | Missing feature | Medium | S | Related only: unmerged Audiobooks agent  |
| B-10.15 | Audiobook parts without a track tag or a leading number play in catalogue order, not file-name order | Probable (I did not confirm the scan's visiting order) / Medium | Confirmed | Defect | Medium | M | No |
| B-10.16 | One bad chapter discards all chapters | Confirmed by reading / Low to Medium | Confirmed | Deliberate trade-off | Medium | S | No |
| B-10.17 | Common album layouts split into several albums | Confirmed by reading / Medium (the most common complaint about tag-keyed  | Confirmed, narrower | Design risk | Medium | M | Related only: “Untagged albums in the ma |
| B-10.18 | ReplayGain stored in APEv2 tags on MP3 files is not seen | Probable / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.19 | Covers with other names, capitalisation, size or location are missed | Confirmed by reading for names and limits; Probable for case / Medium | Confirmed, narrower | Missing feature | Medium | M | No |
| B-10.20 | On the web, a gapless join between tracks that need resampling is two filtered edges | Confirmed by reading; audibility is Speculative (needs a lis / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.21 | In Firefox every lossy track is converted to FLAC | Confirmed by reading / Medium as a design risk | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-10.22 | Android cannot raise quiet tracks | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.23 | AAC with only an edit list: the end is not trimmed, and Apple may cut the start | Speculative for the Apple part (depends on Core Audio); Prob / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.24 | Embedded cover art inflates the "bitrate" used for the limit test | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.25 | The instant mix pads from the same 192 songs every time | Confirmed by reading / Low today (no screen calls it); it matters when ra | Not re-checked | — | Low (first pass) | — | — |
| B-10.26 | Tracks with no disc number sort before disc 1 | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.27 | Normalisation comes and goes while analysis catches up | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.28 | The gapless fixtures leave whole classes of file untested (design risk) | conf: / Medium as a process gap | Confirmed, narrower | Design risk | Medium | M | No |
| B-10.29 | Files with unlisted extensions vanish without a word | Confirmed by reading for the gate; Probable for the lack of  / Medium | Confirmed | Missing feature | Medium | M | No |
| B-10.30 | The web MP3 reader gives up if audio does not start within 8 KB after the ID3 tag | Probable / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.31 | Any <track name>.txt is offered as lyrics | Confirmed by reading / Low (it is a candidate, not auto-published) | Not re-checked | — | Low (first pass) | — | — |
| B-10.32 | The lock-screen entry for an audiobook has no author | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.33 | Small queue edge cases | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-10.34 | When the device's engine cannot play a direct plan, playback fails instead of falling back | Probable (I did not trace what fail leads to in every client / Medium; it turns each decode gap (B-10.10, B-10.30 | Confirmed, narrower | Defect | Medium | M | No |

## 11 — Live TV and DVR

Source: `11-live-tv-dvr-channels.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-11.1 | Every guide refresh retunes every live viewer of that source | Confirmed by reading (the retune) / High | Confirmed | Defect | High | M | No |
| B-11.2 | A standard XMLTV file with a DOCTYPE line is rejected | Confirmed by reading / High | Confirmed | Defect | High | S | No |
| B-11.3 | Valid XMLTV time forms, a missing stop, or a non-UTF-8 encoding reject the whole guide | Confirmed by reading / High | Confirmed | Missing feature | Medium | M | No |
| B-11.4 | One odd programme rejects the whole guide | Confirmed by reading / High for real-world guides | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-11.5 | Size limits fail the whole source | Confirmed by reading / High for IPTV line-ups, low for an antenna | Confirmed, narrower | Design risk | Medium | M | No |
| B-11.6 | Gzip guide files are not decompressed | Confirmed by reading / Medium (most public guide feeds are .gz) | Confirmed, narrower | Missing feature | Medium | S | No |
| B-11.7 | One imperfect playlist entry rejects the whole playlist | Confirmed by reading / High | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-11.8 | HDHomeRun sources cannot have a guide, so they cannot record | Confirmed by reading / High (the most common home tuner gets channel name | Confirmed, narrower | Unfinished feature | High | M | No |
| B-11.9 | Owner settings that are saved but do nothing | Confirmed by reading / Medium (misleading; and it hides the absence of th | Confirmed | Unfinished feature | Medium | M | No |
| B-11.10 | A few minutes of padding overlap fails an entire recording | Confirmed by reading / High where a limit is set (every HDHomeRun using t | Confirmed, narrower | Deliberate trade-off | High | M | No |
| B-11.11 | The first upstream error ends a recording for good | Confirmed by reading / High | Confirmed, narrower | Unfinished feature | High | L | No |
| B-11.12 | A server restart during a recording loses the rest of the programme | Confirmed by reading / Medium | Confirmed | Deliberate trade-off | Medium | L | No |
| B-11.13 | A one-time recording is lost if the provider corrects the start time | Confirmed by reading / High | Confirmed, narrower | Deliberate trade-off | High | M | No |
| B-11.14 | HLS sources with several quality variants: recordings take all of them, live takes the first listed | Probable (it rests on FFmpeg's stream mapping for HLS, which / High for recordings of such channels (several time | Unsettled — needs runtime | Design risk | High | M | No |
| B-11.15 | On Android and Fire TV, live channels are probably always converted | Probable — needs a runtime check on an Android device / Medium to High on those platforms (CPU per viewer, | Confirmed, narrower | Defect | Medium | S | No |
| B-11.16 | An unknown frame rate forces a conversion | Speculative — needs a runtime check with real sources / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-11.17 | Live recovery stops after five failures and the counter does not reset | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-11.18 | The segment writer has no tolerance for a slow database | Speculative — needs a load test / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-11.19 | The HLS gateway refuses playlists with common extra tags | Probable for the rule; which providers trigger it needs real / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-11.20 | "Complete" is judged against the padded window with a four-second tolerance | Probable — needs timing on real sources / Low to Medium (trust in the label) | Confirmed, narrower | Deliberate trade-off | Low | S | No |
| B-11.21 | Measuring storage stalls every running capture | Confirmed by reading (the locking); the size of the stall is / Medium, latent | Confirmed | Design risk | Medium | M | No |
| B-11.22 | A full disk at start time fails the recording before retention can help | Confirmed by reading / Low to Medium | Confirmed | Defect | Medium | S | No |
| B-11.23 | Each finished recording is hashed twice, one result discarded | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-11.24 | dd_progid gives movies and one-off programmes a "series" | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-11.25 | An "all episodes" rule records every airing of every episode | Confirmed by reading / Medium | Confirmed | Missing feature | Medium | M | No |
| B-11.26 | Two profiles recording the same programme use two tuners and store it twice | Confirmed by reading / Medium on a tuner-limited source | Confirmed | Deliberate trade-off | Medium | L | No |
| B-11.27 | The pre-emption countdown is not delivered | Confirmed by reading / Medium (the contract makes the countdown a conditi | Confirmed | Unfinished feature | Medium | M | No |
| B-11.28 | HDHomeRun discovery returns addresses only, and "already configured" is never true | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-11.29 | "Hide channel" is offered to owners but hides only for the owner's own profile | Confirmed by reading / Low to Medium | Confirmed | Defect | Low | S | No |
| B-11.30 | A programme the profile may not see can still be watched | Confirmed by reading / Medium for a parental-control promise; arguably in | Confirmed | Defect | Medium | M | No |
| B-11.31 | The channel list sorts the whole line-up for every page | Confirmed by reading; performance not measured / Low now, a design trap for A-11.7 | Not re-checked | — | Low (first pass) | — | — |
| B-11.32 | Hourly refresh keeps up to a day of full guide copies | Confirmed by reading / Low to Medium (database size and backup time) | Confirmed | Deliberate trade-off | Low | S | No |
| B-11.33 | A slow import can outlive its refresh lease | Speculative — needs a timing test with a guide near the limi / Low to Medium | Unsettled — needs runtime | Design risk | Medium | S | No |
| B-11.34 | Time ruler labels in zones offset by 45 minutes | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-11.35 | The default timeshift window is shorter than it sounds at broadcast bitrates | Confirmed by arithmetic / Low (a tuning decision) | Not re-checked | — | Low (first pass) | — | — |

## 12 — Personal state and Home

Source: `12-personal-state-home-recommendations.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-12.1 | "Mark season / show / album / book as watched" leaves in-progress members unwatched | Confirmed by reading (it is also tested as intended behaviou / High as a user-visible wrong result; the audiobook | Confirmed | Defect | Medium | M | Related container saves only |
| B-12.2 | A show's page loses its "Next up" when the show is old, dismissed, or being rewatched; clients then offer the wrong episode | Confirmed by reading for the server; Probable for the exact  / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-12.3 | A stale offline observation can undo a newer explicit "Mark watched / unwatched" | Confirmed by reading / Medium (needs downloads plus a second device) | Confirmed | Defect | Medium | M | No |
| B-12.4 | Marking the last-played episode unwatched makes Next Up skip it | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-12.5 | Plays of downloads while offline never reach either history or the play counts | Confirmed by reading (grep finds no other writer of these ta / Medium for the owner's audit trail; Low otherwise | Confirmed | Unfinished feature | Medium | M | No |
| B-12.6 | Container "watched" marks are invisible to the recommender, and season-level marks do not stop a show being recommended | Confirmed by reading / Low to Medium (matters most right after someone ti | Confirmed, narrower | Defect | Medium | M | No |
| B-12.7 | The offline path lets the client decide "watched", and writes times in a different text layout | Confirmed by reading / Low (ordering error is under one second); a latent | Not re-checked | — | Low (first pass) | — | — |
| B-12.8 | A report of state ended marks the title watched at any position | Speculative — needs a runtime check of when each client repo / Low to Medium | Unsettled — needs runtime | Design risk | Low | M | No |
| B-12.9 | credits completion mode never completes a title that has no approved credits marker, short of playing to the very end | Confirmed by reading / Medium for an owner who selects the mode (not the  | Confirmed | Deliberate trade-off | Low | S | No |
| B-12.10 | The played threshold a viewer sees is not the one applied to video | Confirmed by reading (server); I did not check how the clien / Low | Not re-checked | — | Low (first pass) | — | — |
| B-12.11 | The "started" percentage preference does almost nothing | Confirmed by reading / Low (a control that does not do what its name says | Not re-checked | — | Low (first pass) | — | — |
| B-12.12 | A paused or momentary play moves "last active", which reorders Continue Watching and re-anchors Next Up | Confirmed by reading for the writes; Probable for the visibl / Low to Medium | Confirmed, narrower | Defect | Low | M | No |
| B-12.13 | Titles with unknown duration cannot resume or complete normally | Confirmed by reading; how often a playable asset lacks a dur / Low | Not re-checked | — | Low (first pass) | — | — |
| B-12.14 | A completion by a session that is not the "canonical" one does not mark the title watched | Confirmed by reading / Low (unusual), but completion is the one piece of  | Not re-checked | — | Low (first pass) | — | — |
| B-12.15 | The season-premiere exemption has no "recently arrived" test | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-12.16 | "Today" for recommendations is the UTC date | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-12.17 | Search history is built on the server and not used; the web client ignores the "remember searches" preference and puts the query in the address | Confirmed by reading / Low (a privacy preference that one client does not | Not re-checked | — | Low (first pass) | — | — |
| B-12.18 | Watched state, ratings and lists appear to be lost when a film's file is renamed, moved or replaced under a new name | Probable — I did not trace the scanner for move detection or / High if it holds, because automated upgraders rena | Confirmed, narrower | Missing feature | Medium | L | No |
| B-12.19 | A show's Next Up anchor is a bare id with no foreign key | Probable (depends on when episodes are purged) / Low | Not re-checked | — | Low (first pass) | — | — |
| B-12.20 | Each progress report rewrites the profile's personal row and invalidates its Home (design risk) | Confirmed by reading; cost not measured / Low at household size; worth a look against the "c | Not re-checked | — | Low (first pass) | — | — |
| B-12.21 | "Popular on this server" needs only two viewers | Confirmed by reading / Low (both opted in) | Not re-checked | — | Low (first pass) | — | — |
| B-12.22 | Server-wide play counts ignore "pause history" and the activity opt-out | Confirmed by reading / Low (aggregate only) | Not re-checked | — | Low (first pass) | — | — |
| B-12.23 | Multi-episode files: "unfinished" is tested against the whole file's length | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-12.24 | One library's small "maximum items" shrinks everyone's merged row | Confirmed by reading / Low (surprising, not wrong) | Not re-checked | — | Low (first pass) | — | — |
| B-12.25 | An explicit resume position on a never-played title does not appear in Continue Watching | Confirmed by reading / Low (the request shape is allowed by the contract  | Not re-checked | — | Low (first pass) | — | — |
| B-12.26 | "Mark unwatched" is not an undo for an accidental "Mark show watched": it erases the episodes' real state | Confirmed by reading; I did not check whether clients offer  / Low to Medium (one mis-tap on a long-running show) | Confirmed, narrower | Missing feature | Low | M | No |

## 13 — Watch Together and casting

Source: `13-social-sync-remote-casting.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-13.1 | Heartbeats and readiness reports invalidate the host's commands | Confirmed by reading / High | Confirmed | Defect | High | M | No |
| B-13.2 | A slow seek is restarted every three seconds and never finishes | Confirmed by reading; whether real seeks exceed 3 s needs a  / High | Confirmed, narrower | Defect | High | M | No |
| B-13.3 | After a seek, members can loop: wait four seconds, seek again | Probable (depends on real seek times, which were not measure / High on native, Medium on web | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-13.4 | Changing the group's speed jumps the position | Confirmed by reading / Low today (no first-party client sends rate); it i | Not re-checked | — | Low (first pass) | — | — |
| B-13.5 | Members who disappear stay in the group forever, and the lobby shows everyone as "away" | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-13.6 | Any member can take the host role after 30 seconds of host silence | Confirmed by reading / Medium (design risk; it defeats host-only) | Confirmed | Defect | Medium | S/M | No |
| B-13.7 | Casting stops when the phone disconnects | Confirmed by reading; how quickly real senders are reported  / High for Cast | Confirmed, narrower | Defect | High | S | Generic Cast acceptance |
| B-13.8 | A profile with the Watch Together privacy switch off is an invisible participant | Confirmed by reading / Medium (the people in a room cannot see everyone w | Confirmed | Defect | Medium | M | No |
| B-13.9 | Small leaks around a group | Confirmed by reading / Low (a), Medium (b), Low (c) | Confirmed | Defect | Medium | S/M | No |
| B-13.10 | Cost grows with the square of the group size | Confirmed by reading (design risk; not measured) / Medium at the ceiling, negligible for a handful of | Confirmed, narrower | Design risk | Medium | M | No |
| B-13.11 | "Play on" approvals and readiness are not tied to the TV | Confirmed by reading / Low (everything is inside one profile's trust), bu | Not re-checked | — | Low (first pass) | — | — |
| B-13.12 | A TV that is already playing never sees a "Play on" request | Confirmed by reading / Medium (looks like a hang) | Confirmed | Defect | Medium | S/M | No |
| B-13.13 | No limits on creating groups or sending commands | Confirmed by reading / Low to Medium (needs a signed-in member) | Confirmed, narrower | Design risk | Medium | M | No |
| B-13.14 | The Cast TV holds a full session with the pairing person's role | Confirmed by reading that the role is passed to IssueTx; wha / Medium (a shared screen is the least trustworthy d | Confirmed | Defect | High | M/L | Generic Cast acceptance |
| B-13.15 | After the caster leaves, the TV still has their session and accepts an unpaired load | Probable (depends on Cast's idle behaviour) / Low to Medium | Confirmed, narrower | Defect | Medium | S | Generic Cast acceptance |
| B-13.16 | A refused takeover leaves a paired device behind | Confirmed by reading / Low (the token is discarded; it is clutter and a s | Not re-checked | — | Low (first pass) | — | — |
| B-13.17 | Group state-machine slips | Confirmed by reading / Low to Medium | Confirmed | Defect | Medium | M | No |
| B-13.18 | Loading a title in a paused group plays it for a moment | Confirmed by reading / Low (a blip of sound at each load) | Not re-checked | — | Low (first pass) | — | — |
| B-13.19 | At the end of a title the group clock keeps running | Confirmed by reading / Low to Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-13.20 | The client can adopt an older snapshot over a newer one | Probable (a short race) / Low (self-heals on the next event) | Not re-checked | — | Low (first pass) | — | — |
| B-13.21 | An invitation accepted by an existing Portico Account member rewrites that member | Confirmed by reading / Medium for the overwrite, Low for the email (codes | Confirmed | Defect | Medium | M | No |
| B-13.22 | Owner broadcast edge cases | (a) Confirmed by reading; (b) Speculative, needs someone who / Low | Not re-checked | — | Low (first pass) | — | — |
| B-13.23 | Names are not filtered | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |

## 14 — Downloads and offline

Source: `14-downloads-offline.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-14.1 | Downloads delete themselves from the device after the server's retention period | Confirmed by reading / High | Confirmed | Defect | High | M | No |
| B-14.2 | "Continue" and "Try again" restart from byte zero on both platforms | Confirmed by reading / High for large files and unreliable links | Confirmed, narrower | Defect | High | M | No |
| B-14.3 | Any pause longer than ten minutes inside the OS downloader fails the transfer | Probable (server side confirmed; the iOS behaviours are docu / High | Confirmed, narrower | Defect | High | M | No |
| B-14.4 | Four of the six offered quality rungs can never be produced | Confirmed by reading (I did not find another argument builde / High for the feature, contained (nothing is damage | Confirmed | Unfinished feature | High | M | No |
| B-14.5 | Original downloads: full copy, double read, per viewer, serial, on the state volume | Confirmed by reading / High as a design risk on small servers (disk exhau | Confirmed, narrower | Design risk | Medium | L | No |
| B-14.6 | A download that finishes while the app is not running online cannot be played offline | Confirmed by reading for the control flow; whether JavaScrip / High (it defeats the main use case) | Confirmed, narrower | Defect | High | M | No |
| B-14.7 | Season, show, album, playlist and auto-next downloads are listed by an internal id, without artwork | Confirmed by reading / Medium (the feature works but is unusable to navig | Confirmed | Unfinished feature | Medium | M | No |
| B-14.8 | Following a whole-show request is lost when the access token renews or the app restarts | Confirmed by reading for the mechanism; Probable for how oft / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-14.9 | Every token renewal re-runs the whole offline handshake | Probable (mechanism confirmed; cost depends on library size) / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-14.10 | Offline viewing is recorded differently from online viewing | Confirmed by reading for each difference; the last two are P / Medium | Confirmed, narrower | Unfinished feature | Medium | L | No; R09 future integration |
| B-14.11 | Audiobook position does not follow the listener to a later part | Confirmed by reading / Medium for audiobook users | Confirmed | Defect | Medium | M | No |
| B-14.12 | "Original" is offered to devices that cannot play it | Probable (no MKV handling exists in the Apple code; not run) / High on Apple devices | Unsettled — needs runtime | Unfinished feature | Medium | L | No |
| B-14.13 | Pause and Cancel on a working preparation almost always answer "conflict" | Probable / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-14.14 | Files of a signed-out account or another profile stay on the device, invisible and unremovable | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-14.15 | A ready claim can point at a prepared file that no longer exists | Probable / Medium (a dead end for anyone mid-transfer or arri | Confirmed, narrower | Defect | Medium | M | No |
| B-14.16 | Receipt dead ends | Confirmed by reading / Low-to-Medium (needs a restore or data loss, but t | Confirmed | Defect | Medium | M | No |
| B-14.17 | A whole-container request fails if anything in the library or the viewer's state changes during capture | Probable / Medium | Confirmed, narrower | Design risk | Medium | L | No |
| B-14.18 | iOS may lose a completed large download during background verification | Speculative — needs a device test with a file of tens of gig / Medium | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-14.19 | Entries can be stuck in "downloading" | Probable / Low (Remove still works) | Not re-checked | — | Low (first pass) | — | — |
| B-14.20 | Android transfer lifetime | Speculative for the platform limits (not tested); Confirmed  / Low-to-Medium | Confirmed, narrower | Unfinished feature | Medium | L | No |
| B-14.21 | Clocks | Confirmed by reading / Low (this is not DRM by decision) | Not re-checked | — | Low (first pass) | — | — |
| B-14.22 | A download resumes from the device's own stale position | Confirmed by reading / Low-to-Medium | Confirmed | Defect | Medium | S/M | No |
| B-14.23 | Re-requesting something already held says "Nothing could be added"; a failed claim blocks a fresh one | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-14.24 | Finished rows are never removed | Confirmed by reading (grep for DELETE FROM download_) / Low | Not re-checked | — | Low (first pass) | — | — |
| B-14.25 | The downloads worker does a write transaction on every wake | Probable / Low | Not re-checked | — | Low (first pass) | — | — |
| B-14.26 | Multi-file and multi-version titles are refused with a misleading reason | Confirmed by reading / Low now, Medium later | Confirmed | Unfinished feature | Low | M/L | Version merge only |
| B-14.27 | Small ones | conf: / sev: | Confirmed, narrower | Design risk | Low | M tota | No |

## 15 — Security posture

Source: `15-security-posture.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-15.1 | The remote sign-in policy is enforced on only one of the sign-in routes | Confirmed by reading (no test exercises enforcement; admin_a / High | Confirmed | Defect | High | M | No |
| B-15.2 | One approved-but-uncollected TV code permanently stops identity maintenance | Confirmed by reading; a one-line runtime check (approve a co / Medium (silent; grows into disk and secret-retenti | Confirmed, narrower | Defect | Medium | S | No |
| B-15.3 | Behind a reverse proxy, the database-backed limiter treats all clients as one | Confirmed by reading / Medium (availability of sign-in on proxied deploym | Confirmed, narrower | Defect | Medium | M | No |
| B-15.4 | An un-set-up server can be claimed by anyone who can reach it, and by a web page | Confirmed by reading for the open window (deliberate) / Medium overall; High for a server that is port-for | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-15.5 | Live credentials and two-factor seeds are stored and backed up in the clear | Confirmed by reading / Medium (needs file access; the same access already | Confirmed | Deliberate trade-off | Medium | S | No |
| B-15.6 | Three different readings of X-Forwarded-For | Confirmed by reading / Low to Medium (availability and consistency; I fou | Confirmed | Defect | Medium | M | No |
| B-15.7 | Tokens are not bound to a device key, and the documentation says they are | Confirmed by reading / Medium on HTTP over untrusted networks; Low on a h | Confirmed | Documentation error | Low | S | No |
| B-15.8 | The server's own web app is refused behind a TLS-terminating proxy unless an environment variable is set | Probable (depends on the proxy forwarding Host unchanged, wh / Low (fails closed; works once the origin is added  | Not re-checked | — | Low (first pass) | — | — |
| B-15.9 | The second factor can be worn down by an attacker who already has the password | Confirmed by reading (arithmetic, not a test) / Low to Medium | Confirmed | Deliberate trade-off | Low | S | No |
| B-15.10 | Two hashing slots serve every password check | Confirmed by reading; the threshold depends on hardware / Low to Medium for internet-facing servers | Confirmed, narrower | Design risk | Low | S | No |
| B-15.11 | "Private peer" can be wrong when something in front hides the client address | Confirmed by reading for the logic; which deployments trigge / Medium for affected deployments | Confirmed, narrower | Design risk | Medium | M | No |
| B-15.12 | A stolen Hosted signing key vouches for any Portico Account until each server hears otherwise | Confirmed by reading for the verification rule; how quickly  / High impact, low likelihood | Confirmed, narrower | Design risk | High | L | No |
| B-15.13 | Approving a code makes the other device you, and codes are advertised on the LAN | Confirmed by reading; inherent to device-code sign-in (Jelly / Medium | Confirmed | Deliberate trade-off | Low | S | No |
| B-15.14 | Remembered-account lookup is unauthenticated and keyed by a value sent in every request | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-15.15 | Version and build fingerprint are public | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-15.16 | The main CSP allows scripts from all of www.gstatic.com | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-15.17 | The release-feed fetch has no network policy | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-15.18 | The folder picker checks the path before resolving symlinks, not after | Probable / Low | Not re-checked | — | Low (first pass) | — | — |
| B-15.19 | Supply-chain loose ends | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-15.20 | The built-in provider credentials are a shared dependency | Confirmed by reading / Low (availability) | Not re-checked | — | Low (first pass) | — | — |
| B-15.21 | Profile PIN attempts are counted per device, and a new sign-in is a new device | Confirmed by reading / Low; the contract calls PINs "convenience locks, n | Not re-checked | — | Low (first pass) | — | — |
| B-15.22 | "Secure connections: required" has quiet exceptions | Confirmed by reading / Low to Medium (expectation gap) | Confirmed | Deliberate trade-off | Low | S | No |
| B-15.23 | TV-code and invitation routes sit outside the cheap limiter | Confirmed by reading; the impact needs measuring / Low | Not re-checked | — | Low (first pass) | — | — |
| B-15.24 | Small output-encoding nits | Speculative (neither traced to an input an attacker controls / Low | Not re-checked | — | Low (first pass) | — | — |

## 16 — Web architecture

Source: `16-web-architecture.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-16.1 | The Back-restore memory for library lists is probably never filled when you leave the library | Probable / Medium (a headline scale feature silently off; use | Unsettled — needs runtime | Defect | Medium | S | No |
| B-16.2 | Switching profile (or signing in as someone else) in one tab signs other tabs out and erases the browser's saved sign-in | Probable (traced end to end by reading; needs a two-tab runt / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-16.3 | Only two of about seventy-five hashed assets are cached as immutable | Confirmed by reading (server code and build output) / Medium (performance on every load, worst on HTTP/1 | Confirmed, narrower | Defect | Medium | S | No |
| B-16.4 | WindowedGrid's mapped-scroll mode has discontinuities near the ends | Probable (arithmetic by reading; the dev WindowedLab exists  / Low (only very large flat lists; nothing is unreac | Not re-checked | — | Low (first pass) | — | — |
| B-16.5 | WindowedGrid re-creates its ResizeObserver and forces layout on every row change; inline renderers defeat per-page memoisation | Confirmed by reading that the work happens; its cost is unme / Low to Medium on slow devices | Confirmed | Design risk | Low | S | No |
| B-16.6 | Regional language and time preferences are applied inconsistently | Confirmed by reading; visible effect today is limited to spe / Low today, High the day a second language ships (m | Confirmed, narrower | Defect | Low | M | No |
| B-16.7 | Each open tab holds a long-poll; on plain HTTP several tabs can starve the connection pool | Probable (well-known browser behaviour; not measured here) / Medium for the "plain HTTP stays first-class" audi | Confirmed, narrower | Design risk | Medium | M | No |
| B-16.8 | Most requests have no deadline | Confirmed by reading that no deadline exists; how often serv / Medium (design risk; the backlog's own notes menti | Confirmed, narrower | Design risk | Medium | M | No |
| B-16.9 | After a server upgrade, an open tab cannot load new screens and "Try again" cannot fix it | Probable (React caches a rejected lazy import; not reproduce / Medium (every self-hoster upgrades with tabs open) | Confirmed, narrower | Defect | Medium | S | No |
| B-16.10 | Strict parsers reject whole documents for unknown enum values | Confirmed by reading (latent; no skew exists yet) / Medium as a design risk for the first post-release | Confirmed | Design risk | Medium | M | No |
| B-16.11 | A short connection wobble unmounts the current page and discards its state | Probable / Low to Medium (laptops that roam between Wi-Fi acc | Confirmed, narrower | Defect | Medium | S | No |
| B-16.12 | No declared browser floor; older engines degrade silently | Confirmed by reading (CSS semantics); which real devices are / Low if the matrix is "current evergreen"; Medium i | Confirmed, narrower | Design risk | Low | S | No |
| B-16.13 | The signed-in shell has no main landmark and no skip link | Confirmed by reading / Medium (WCAG 2.4.1) | Confirmed | Defect | Medium | S | No |
| B-16.14 | Every page has the same document title; navigation is not announced | Confirmed by reading / Low to Medium (WCAG 2.4.2; also hurts tabs, histor | Confirmed | Defect | Medium | S | No |
| B-16.15 | A card's accessible name is the title only; watched and progress state are not conveyed with it | Confirmed by reading / Medium for screen-reader users (contract A11Y-2.1  | Confirmed, narrower | Defect | Medium | S | No |
| B-16.16 | Virtualised grids do not tell assistive technology their size or position, and focus can fall out | Confirmed by reading (attributes absent); focus loss is Prob / Low to Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-16.17 | Smaller accessibility defects in primitives | conf: / sev: | Confirmed, narrower | Design risk | Low | S | No |
| B-16.18 | The strict style CSP probably blocks the scroll-lock stylesheet that Radix dialogs inject | Probable (needs a look at the console on a server-served bui / Low | Not re-checked | — | Low (first pass) | — | — |
| B-16.19 | The browse-capabilities cache never expires and is not keyed by a revision | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-16.20 | Failed list pages ignore Retry-After and retry on a fixed 5 s clock | Confirmed by reading / Low (retries happen only as the user scrolls) | Not re-checked | — | Low (first pass) | — | — |
| B-16.21 | Paged admin lists keep an old cursor when their inputs change | Speculative (depends on whether the component is remounted p / Low | Not re-checked | — | Low (first pass) | — | — |
| B-16.22 | Credits paging can double-append and hides failures | Probable for the silent failure (by reading); Speculative fo / Low | Not re-checked | — | Low (first pass) | — | — |
| B-16.23 | React hook dependency rules are not enforced by any tool | Confirmed by reading / Medium as a process risk | Confirmed | Design risk | Low | S | No |
| B-16.24 | Dev-only screens are emitted into the production web folder | Confirmed (source + local build) / Low | Not re-checked | — | Low (first pass) | — | — |
| B-16.25 | CI and the shipped image build the web with different Node versions | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |

## 17 — Web player

Source: `17-web-player.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-17.1 | Quality, prepared-version and audio-track choices never reach the server | Confirmed by reading / High | Confirmed | Defect | High | M | No |
| B-17.2 | Changing quality, version or audio, pressing Replay, or an automatic fallback throws away the queue | Confirmed by reading / High for series viewing | Confirmed, narrower | Defect | Medium | M | No |
| B-17.3 | A pause that does not come from Portico's own buttons is undone at once (video) | Confirmed by reading the logic; the per-browser effect is Pr / High (media keys and the PiP pause button are ever | Confirmed, narrower | Defect | Medium | M | No |
| B-17.4 | Slow delivery is turned into failure: the stall watchdog and a 15-second fragment limit abort downloads that would have finished | Confirmed by reading (both sides of the interaction are in s / High for remote and weak-server use, which Portico | Confirmed, narrower | Defect | High | M | No |
| B-17.5 | Resuming an HLS title loads the start of the title first | Confirmed by reading on the client; the server-side double s / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-17.6 | AirPlay is offered in Safari but cannot work for HLS sessions | Probable (needs a Safari check) / Medium | Unsettled — needs runtime | Unfinished feature | Medium | M | No |
| B-17.7 | Closing or reloading the tab leaves the session running on the server | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-17.8 | A lapsed lease (laptop sleep, long suspend) ends in an error card | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-17.9 | Player shortcuts fire with Ctrl, Cmd or Alt held | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-17.10 | Shortcuts stop working after using a slider | Confirmed by reading (first part); Speculative (Firefox doub / Low | Not re-checked | — | Low (first pass) | — | — |
| B-17.11 | On a touch screen, tapping the picture pauses | Confirmed by reading / Medium on phones and tablets | Confirmed | Defect | Medium | S | No |
| B-17.12 | Double-click for fullscreen also pauses and resumes | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-17.13 | The Fullscreen button does nothing on iPhone | Probable (needs an iPhone) / Medium on iPhone | Unsettled — needs runtime | Unfinished feature | Medium | M | No |
| B-17.14 | Buffer targets ignore bitrate | Speculative, needs a run with a high-bitrate remux / Medium | Unsettled — needs runtime | Design risk | Medium | S | No |
| B-17.15 | A session that a newer action supersedes is never stopped; a fast double "Next" can end in an error | Confirmed by reading for the orphan session; Probable for th / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-17.16 | Reported speed is always 1; other speeds cause extra reports | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-17.17 | Resuming deep into a long MP3 or FLAC reads everything before the position | Confirmed by reading for the mechanism; Probable for the fai / Medium (audiobooks are first-class) | Confirmed, narrower | Unfinished feature | Medium | M | Yes — audio seek mechanism |
| B-17.18 | The music engine on iPhone and iPad Safari | Speculative, needs devices / Medium (music that stops when the phone locks) | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-17.19 | Media keys and lock-screen controls for music depend on Web Audio counting as "media" | Speculative / Medium if it fails | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-17.20 | Subtitles disappear during an outage the video survives | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-17.21 | In Firefox, subtitles can sit under the controls | Confirmed by reading; visual effect Probable / Low | Not re-checked | — | Low (first pass) | — | — |
| B-17.22 | The seek bar has two overlapping ways to seek | Speculative / Low (each extra seek on a conversion can move the  | Not re-checked | — | Low (first pass) | — | — |
| B-17.23 | Automatic level switching in hls.js is left on | Confirmed by reading (as a latent condition) / Low | Not re-checked | — | Low (first pass) | — | — |
| B-17.24 | The subtitle time mapping can rebuild every cue on a fragment change | Speculative; depends on what hls.js reports for these stream / Low to Medium | Unsettled — needs runtime | Design risk | Low | S | No |
| B-17.25 | Design risk: the adapter that ships has no behavioural test | conf: / Medium as a risk multiplier | Confirmed | Design risk | Medium | M | No |
| B-17.26 | Small items | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |

## 18 — Web UX

Source: `18-web-ux-feature-parity.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-18.1 | On a show with seasons hidden or split into ranges, the group tabs do nothing on the web | Confirmed by reading / High for the feature (ranges and hidden seasons ar | Confirmed | Defect | Medium | S | No |
| B-18.2 | The metadata editor silently drops a Tags or Labels edit when the field is already locked | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-18.3 | "Remember search history" does nothing on the web, and "Clear" does not clear what the server stored | Confirmed by reading / Medium (a privacy control that does not do what it | Confirmed, narrower | Defect | Medium | M | No |
| B-18.4 | The Music › Genres tab and the Audiobooks › Series tab stop at 200 entries | Confirmed by reading (which 200 are kept depends on the face / Medium; it contradicts "correctness over limits" | Confirmed | Defect | Medium | M | No |
| B-18.5 | A library cannot be removed | Confirmed by reading / High as a product gap (not a malfunction) | Confirmed | Missing feature | Medium | L | No |
| B-18.6 | "Refresh metadata" is offered on shows, seasons, albums, artists and books, where the server refuses it | Probable (I did not trace how a show's public id resolves in / Medium: refreshing a show is the most common refre | Confirmed | Unfinished feature | Medium | S | No |
| B-18.7 | A movie's page offers fewer and different actions than the same movie's card | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-18.8 | Owner tools depend on how the owner signed in, inconsistently | Confirmed by reading that the conditions differ; whether a h / Medium | Refuted | Design risk | Low | S | No |
| B-18.9 | The "Administrator" role can be granted but gives nothing in the web app | Confirmed by reading for the web; server coverage of the tie / Medium (a control that promises something the prod | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-18.10 | The profile menu offers "Limits and permissions…" and "Delete profile…" to someone who cannot use them | Probable (the model is clear; I did not trace the server's r / Medium for a restricted child profile seeing "Limi | Confirmed, narrower | Defect | Low | S | No |
| B-18.11 | The check that every preference has a consumer no longer runs | Confirmed by reading / Medium (a safety net silently disabled) | Confirmed | Defect | Medium | S | No |
| B-18.12 | About fifteen screens ignore the profile's language choice | Confirmed by reading / Low today, Medium the day a second language is add | Confirmed | Defect | Low | M | No |
| B-18.13 | Bulk Delete has no confirmation, including for "Select all N" (deliberate; design risk) | Confirmed by reading that no confirmation exists; what "tras / High potential impact, low likelihood | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-18.14 | On a phone-width browser there is no way to switch profile or server, send feedback or open Help | Confirmed by reading / Medium for households using phone browsers | Confirmed, narrower | Defect | Medium | S | No |
| B-18.15 | A show's cast stops at twelve with no "See all" | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-18.16 | In the episode grid layout an episode has no ⋯ menu | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-18.17 | A season card's address stops working after switching season | Probable; it needs a season card to exist somewhere in the w / Low | Not re-checked | — | Low (first pass) | — | — |
| B-18.18 | Back buttons behave differently per page and can leave Portico | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-18.19 | "This device" badges sit on settings that apply to every browser the profile uses | Confirmed by reading / Low (a label that misleads) | Not re-checked | — | Low (first pass) | — | — |
| B-18.20 | Non-owners can add one title to a collection but not several | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-18.21 | Changing someone's role takes effect on selection, with no confirmation | Confirmed by reading / Low (and moot while B-18.9 stands) | Not re-checked | — | Low (first pass) | — | — |
| B-18.22 | English text written straight into components | Confirmed by reading / Low at an English-only launch | Not re-checked | — | Low (first pass) | — | — |
| B-18.23 | Disc headings depend on an English phrase inside a subtitle | Confirmed by reading (latent) / Low | Not re-checked | — | Low (first pass) | — | — |
| B-18.24 | Dates and language names follow the browser, not the profile's Language & region | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-18.25 | Cards in Saved are not links | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-18.26 | Search words are written into the address | Confirmed by reading / Low to Medium (privacy on a shared computer; the p | Confirmed, narrower | Defect | Medium | S | No |
| B-18.27 | Parts that exist but cannot be reached or do nothing | Confirmed by reading (the string count is an estimate) / Low (maintenance weight, and translators would be  | Not re-checked | — | Low (first pass) | — | — |
| B-18.28 | Home's "See all" guesses the destination from the row's id | Confirmed by reading (latent) / Low | Not re-checked | — | Low (first pass) | — | — |
| B-18.29 | Removing an entry from a playlist or collection is immediate, with no Undo | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-18.30 | The episode panel loses facts on a direct visit and does not link people | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |

## 19 — Native architecture

Source: `19-native-architecture.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-19.1 | A link that launches the app from cold is thrown away | Confirmed by reading / Medium (home-screen cards are a primary TV entry p | Confirmed, narrower | Defect | Medium | M | No |
| B-19.2 | Every access-token renewal rebuilds the data services of every mounted screen | Confirmed by reading for the re-creation; the visible effect / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-19.3 | On Android TV and Fire TV the remote's Menu key presses the focused control | Confirmed by reading (both files) / Medium (unexpected activation of destructive butto | Unsettled — needs runtime | Defect | Medium | M | No |
| B-19.4 | If the Android Keystore key is lost, the app can never read or save most of its records again | Confirmed by reading for the logic; how often the trigger ha / Medium (rare, but permanent and invisible) | Confirmed, narrower | Defect | Medium | M | No |
| B-19.5 | An image that has loaded once is never retried and gets no placeholder if it later fails | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-19.6 | An uncaught JavaScript error outside rendering ends the app with no record | Confirmed by reading (absence of a handler) / Medium as a risk multiplier: it turns any small bu | Confirmed, narrower | Missing feature | Low | M | No |
| B-19.7 | The iPad build lists three orientations, which App Store validation normally rejects for a multitasking app | Probable (not uploaded) / Medium only because it blocks an upload; the fix i | Unsettled — needs runtime | Design risk | Medium | S | No |
| B-19.8 | The Android TV density override has gaps and wide side effects | the rotation gap is Speculative (needs a device that reports / Low | Not re-checked | — | Low (first pass) | — | — |
| B-19.9 | On Apple, storage, PIN hashing and request submission share one queue | Probable (queue behaviour of interop modules not verified at / Low | Not re-checked | — | Low (first pass) | — | — |
| B-19.10 | A television that does not report "television" gets a touch layout with no visible focus | Confirmed by reading; which devices do this is known in gene / Low to Medium depending on how many owners use suc | Confirmed, narrower | Design risk | Medium | M | No |
| B-19.11 | Vega keeps sign-in records as plain files | Confirmed by reading and by the port log / Low (sandboxed consumer device) | Not re-checked | — | Low (first pass) | — | — |
| B-19.12 | On Apple, deleting the app does not sign it out | Confirmed by reading (standard platform behaviour; not exerc / Low to Medium (privacy on a device that changes ha | Confirmed, narrower | Design risk | Low | M | No |
| B-19.13 | The windowed grid assumes equal row heights and exact pixel arithmetic at any size | Speculative for both (needs a library with long titles, and  / Low today; Medium against the stated 5–10 million  | Confirmed, narrower | Defect | Medium | M | No |
| B-19.14 | Mounted screens are never released or frozen | Confirmed by reading; impact on memory is unmeasured / Low on Apple TV, potentially Medium on 1–1.5 GB st | Confirmed, narrower | Design risk | Medium | M | No |
| B-19.15 | The test harness leans on a React internal, and native tests are not run | Confirmed by reading / Low (process risk) | Not re-checked | — | Low (first pass) | — | — |
| B-19.16 | User-visible English outside the catalogue | Confirmed by reading / Low while the app is English-only; they also bypas | Not re-checked | — | Low (first pass) | — | — |
| B-19.17 | Plural rules may silently fall back to English on devices | Speculative (depends on the Hermes build; not checked) / Low now, Medium when the first non-English languag | Unsettled — needs runtime | Design risk | Low | M | No |
| B-19.18 | No scene manifest in the Apple apps | Speculative — I could not verify what the current SDK enforc / High if it applies (the app would not launch when  | Unsettled — needs runtime | Unfinished feature | High | M | No |
| B-19.19 | Small contract leftovers | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |

## 20 — Native players

Source: `20-native-players.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-20.1 | Audio a receiver could take as a bitstream is refused and converted, on every device that publishes v1 capabilities | Confirmed by reading (planner logic) / High for Android TV / Fire TV / Vega with external | Confirmed, narrower | Defect | High | M | No |
| B-20.2 | HDR titles are tone-mapped on the server for Apple devices whose screen is SDR at that moment | Confirmed by reading / High for the weak-server target | Confirmed, narrower | Defect | High | M | No |
| B-20.3 | App-drawn subtitles are refused when the server is reached over plain HTTP | Confirmed by reading / High where a server is used without a certificate, | Confirmed | Defect | High | S | No |
| B-20.4 | On Android a video keeps playing after the app is left when Picture in Picture does not take over | Probable (the absence of a pause is confirmed; that ExoPlaye / High on Android TV and Fire TV | Unsettled — needs runtime | Defect | High | M | No |
| B-20.5 | On Apple TV the display switches mode after playback has started | Probable (ordering confirmed by reading; the visible effect  / Medium | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-20.6 | Ordinary network and server errors are reported as engine failures, which downgrades the file for a week | Confirmed by reading (classification and consequence) / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-20.7 | When the system pauses the player, the app still believes it is playing | Probable (the logic is confirmed; the system pause is standa / Medium | Unsettled — needs runtime | Defect | Medium | M | No |
| B-20.8 | Android declares its MPEG-2 decoder under a name the planner never matches; VC-1 is not declared at all | Confirmed by reading / Medium (Live TV and DVR on Android TV) | Confirmed | Defect | Medium | S | No |
| B-20.9 | An MP4 whose HEVC track is tagged hev1 (or Dolby Vision dvhe) is offered to AVPlayer as a direct play | Probable (the missing check is confirmed; AVPlayer's behavio / Medium; hev1 is what FFmpeg writes by default, so  | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-20.10 | Android reads "any channel count" as stereo | Confirmed by reading / Low now; latent | Not re-checked | — | Low (first pass) | — | — |
| B-20.11 | Android declares that it renders ASS, SSA and PGS; it renders none of them | Confirmed by reading / Low now; latent High | Confirmed, narrower | Design risk | Low | S | No |
| B-20.12 | App-drawn subtitles ignore both Portico's appearance settings and the system's caption style | Confirmed by reading / Low to Medium (accessibility) | Confirmed | Defect | Medium | M | No |
| B-20.13 | One failed re-check blanks subtitles the device already has, silently | Confirmed by reading / Medium-Low | Confirmed | Defect | Medium | S | No |
| B-20.14 | A device with only a software decoder for a codec declares that codec at the software decoder's limits | Confirmed by reading (flag unused); effect Probable / Medium on low-end Android and older Fire TV | Confirmed, narrower | Design risk | Medium | M | Related diagnostics only |
| B-20.15 | The device claims trick-play and chapters; the native player has neither | Confirmed by reading / Low (a claim the server may act on later; a dead n | Not re-checked | — | Low (first pass) | — | — |
| B-20.16 | The Next and Previous buttons in the video player call the music owner and do nothing for video | Probable (traced in client-core and the player; I did not fi / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-20.17 | The profile's default video speed is offered in Settings but never applied by the native player | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-20.18 | A slow answer to the pre-play Dolby Vision check delays every video start | Confirmed by reading / Low to Medium (start-up time on weak servers) | Confirmed, narrower | Design risk | Low | S | No |
| B-20.19 | A file the engine cannot seek exactly turns every seek, including resume, into a failure | Speculative (needs such a file on Android) / Low to Medium; affects direct play only, and only  | Unsettled — needs runtime | Design risk | Low | M | No |
| B-20.20 | Passthrough is declared from every HDMI device even when sound is going elsewhere, and replaces the decode capability | Confirmed by reading / Low (and masked by B-20.1 today) | Not re-checked | — | Low (first pass) | — | — |
| B-20.21 | After a music crossfade, the surviving ExoPlayer is the app's video player too | Confirmed by reading / Low (wrong buffer limits on low-memory devices) | Not re-checked | — | Low (first pass) | — | — |
| B-20.22 | The video audio session is never released on Apple | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-20.23 | Observer removal relies on the player's current item rather than the item observed (latent) | Speculative (no failing path found) / Low | Not re-checked | — | Low (first pass) | — | — |

## 21 — Native UX

Source: `21-native-ux-feature-parity.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-21.1 | Play on has no permanent Enter a TV Code action | Confirmed by reading / Medium | Confirmed | Unfinished feature | Medium | S | No |
| B-21.2 | Programme-sheet Cancel Recording bypasses confirmation | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-21.3 | Wide-tablet Downloads loses its primary navigation entry | Confirmed by reading / Medium | Confirmed | Unfinished feature | Medium | S | No |
| B-21.4 | Saved Spotlight and Siri index reads a deliberately absent Home row | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-21.5 | History and adjacent appended saved lists grow unwindowed | Confirmed by reading / High | Confirmed, narrower | Defect | Medium | M | Yes, History only |
| B-21.6 | Published native preferences can be consumed or stored without a reachable control | Confirmed by reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | Partial, card size |
| B-21.7 | This device wording actually controls the whole device class | Confirmed by reading / Medium | Confirmed | Documentation error | Medium | S | No |
| B-21.8 | Playing a duplicate playlist occurrence starts the first matching item | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-21.9 | Pre-play subtitles omit unknown-language tracks and merge distinct same-language choices | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-21.10 | Vega exposes unsupported speed and audio-effect preferences | Confirmed by reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-21.11 | Saved View Play and Shuffle only act on the loaded prefix | Confirmed by reading / Medium | Confirmed | Unfinished feature | Medium | M | No |
| B-21.12 | Hosted owners are blocked from native metadata edit by authority gate | Probable / Medium | Confirmed | Defect | Medium | S | No |
| B-21.13 | Saved hook can skip selecting a replacement service with the same route | Probable / Medium | Confirmed | Defect | Medium | S | No |
| B-21.14 | Async item deep links have no visible intent/viewer fence | Probable / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-21.15 | Back from filters commits live changes rather than restoring a draft | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-21.16 | TV long-press menus lack a proven visible accessible alternative on some card paths | Speculative — needs a runtime check / Medium | Unsettled — needs runtime | Unfinished feature | Medium | M | Yes, recommendation path |

## 22 — Client connectivity

Source: `22-client-connectivity-resilience.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-22.1 | Secure LAN direct names invalidate signed route documents | Confirmed by reading / High | Confirmed | Defect | High | M | No |
| B-22.2 | Multiple secure LAN addresses collapse to duplicate signed origins | Confirmed by reading / High | Confirmed | Defect | High | S/M | No |
| B-22.3 | Plain-HTTP tab-local locks cannot protect a shared rotating credential | Confirmed by reading for missing cross-tab coordination; the / High | Confirmed | Defect | High | M | No |
| B-22.4 | A temporary protected-store read failure is classified as sign-out | Confirmed by reading / High | Confirmed | Defect | High | M | Related symptom only |
| B-22.5 | Unsigned replacement detection can discard a valid saved family | Confirmed by reading / High | Confirmed, narrower | Defect | Medium | M | No |
| B-22.6 | A ready HTTP route is not bound to future sockets | Confirmed by reading for the verification lifetime; actual c / High | Confirmed, narrower | Deliberate trade-off | Medium | S/L | No |
| B-22.7 | Manual scheme guessing omits the default port and misclassifies hosts | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-22.8 | General browser requests can remain pending without a deadline | Confirmed by reading / Medium | Confirmed | Design risk | Medium | M | No |
| B-22.9 | Server-list watch single-flight can wedge on a hung fetch | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-22.10 | Local refresh ignores server retry floors and can bypass its own timer | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | Related symptom only |
| B-22.11 | Playback and watch parsing ignore HTTP-date Retry-After | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-22.12 | Browser Hosted errors discard Retry-After | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S/M | No |
| B-22.13 | SSE frame cap does not bound accumulated parsed data | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-22.14 | JSON proxy errors can leave a dead route classified ready | Probable, deployment/proxy format needs a runtime check / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-22.15 | Native FIFO can starve renewal/timeline behind reads | Probable, actual burst/starvation needs load/device evidence / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-22.16 | Browser clock ahead by roughly 30 seconds rejects fresh route proof | Confirmed by reading for asymmetry; real-device clock behavi / Medium | Confirmed | Design risk | Medium | M | No |
| B-22.17 | Known-server labels can retain obsolete logical addresses | Probable, exact chooser path/device behavior needs verificat / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-22.18 | Auxiliary native streams bypass route selection and verification | Confirmed by reading / High | Confirmed | Defect | High | M | No |
| B-22.19 | Vega rejects redirects after credential transmission can occur | Confirmed by reading for post-follow enforcement; exact SDK  / High | Confirmed, narrower | Defect | High | M/L | No |
| B-22.20 | Android inherits implicit retries of replayable mutation bodies | Probable for duplicate side effects; confirmed source policy / High | Confirmed, narrower | Defect | Medium | M | No |
| B-22.21 | HTTP-only discovery cannot bootstrap a required-HTTPS server | Confirmed by reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-22.22 | A successful remote route may never return to LAN without a path event | Confirmed by reading for policy; impact needs network eviden / Medium | Confirmed | Design risk | Medium | M | No |
| B-22.23 | Cross-origin web clients cannot read Report-Every-Ms | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-22.24 | Same-link multi-interface mDNS advertisements may conflict with each other | Speculative — needs a runtime check / Medium | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-22.25 | Event retries can synchronize on a large server-provided floor | Confirmed by reading / Medium | Confirmed | Design risk | Medium | S | No |
| B-22.26 | Native Hosted activation polling bypasses the shared outage gate | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |

## 25 — Engineering and release

Source: `25-engineering-practices-project-model.md`

| ID | Claim | First pass | Second review | Class | Severity after review | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-25.1 | Public update manifest cannot be read by the server | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-25.2 | Release and Makefile vet still reject the maintained driver | Probable / High | Unsettled — needs runtime | Defect | High | S | No |
| B-25.3 | Windows portable ZIP output is relative to its payload directory | Confirmed by reading / High | Confirmed | Defect | High | S | No |
| B-25.4 | Docker release job omits its required FFmpeg input | Confirmed by reading / High | Confirmed | Defect | High | M | No |
| B-25.5 | Hosted apikit has actual drift and its checker is not in CI | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-25.6 | Official release script can silently invent a signing key | Confirmed by reading / High | Confirmed, narrower | Unfinished feature | Medium | S | No; explicitly known in release-signing. |
| B-25.7 | Server seal does not authenticate the later web/package payload | Confirmed by reading / Medium | Confirmed, narrower | Design risk | Medium | M | No; broad signing enforcement is known |
| B-25.8 | macOS codesigning changes bytes after the manifest is signed | Confirmed by reading / High | Confirmed, narrower | Defect | Medium | M | No |
| B-25.9 | Release does not require the full media/performance tiers | Confirmed by reading / High | Confirmed, narrower | Design risk | Medium | M | Partly: test-runtime/full-timeout work |
| B-25.10 | Apple/Vega and physical native behavior are not automated release gates | Confirmed by reading / High | Confirmed, narrower | Design risk | Medium | L | Partly: ports/device/UI acceptance |
| B-25.11 | Font/icon notices promised in docs are missing from source distribution | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-25.12 | GPL Apple distribution rights remain unresolved | Speculative — needs a runtime check (and a distribution-righ / High | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-25.13 | Bare-goroutine guard is a line-regex gate, not a complete AST gate | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-25.14 | SQL guards explicitly exclude runtime-assembled statements | Confirmed by reading / Medium | Confirmed, narrower | Documentation error | Low | S | No; specific scale costs are known |
| B-25.15 | Hostile-input test rotates cases despite exhaustive-sounding prose | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-25.16 | All-tier runner timeout remains below recorded package cost | Probable / Medium | Confirmed, narrower | Design risk | Medium | S | Yes: runner-test.sh --full cannot finish |
| B-25.17 | Python schema integration test references removed files | Confirmed by reading / Low | Not re-checked | — | Low (first pass) | — | — |
| B-25.18 | Migration chain violates the documented no-startup-backfill rule | Confirmed by reading / Medium | Confirmed | Design risk | Medium | M | No explicit migration-policy item |
| B-25.19 | Manual release can test caller branch rather than requested tag | Confirmed by reading / High | Confirmed | Defect | High | S | No |
| B-25.20 | Linux upgrade lifecycle stops but does not restart the service | Probable / Medium | Confirmed, narrower | Defect | High | S | No |
| B-25.21 | Cross-repo CI checks moving default branches, not recorded dependency revisions | Confirmed by reading / Medium | Confirmed | Design risk | Medium | M | No |
