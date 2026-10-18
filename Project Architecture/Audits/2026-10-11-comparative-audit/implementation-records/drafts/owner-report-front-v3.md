# Portico implementation report — 14–16 October 2026

This is the owner's summary of the implementation programme that followed the comparative audit, updated after the completion passes of 15 and 16 October. Sections 1–6 are the short version. The appendices hold the per-decision detail, every lane's record and all 588 audit bug dispositions; lane reports for 15–16 October are in `_program/reports/` (`cp-*`, `cp2-*`, `cp3-*`, `cp4-*`, `cp5-*`, `verify-*`).

## 1. Where things stand

- **Every decision you made is built in source,** including everything you decided on 15 October (section 3). A full sweep of every lane report, ledger and decision on 15 October found 69 codeable items still open; all of them were then built, along with the follow-ups those lanes left (section 4 lists the little that remains).
- **Audit bugs:** of the 588, 386 are fixed, 173 were already fixed, 7 were not present and 11 are intentional under your decisions. **11 remain open (7 Medium, 4 Low, none High)**; nearly all need real devices, real media or a release to settle (section 4).
- **Independent reviews of the new work found 137 problems, and all of them are fixed.**
- **Everything passes on Ubuntu after the final verification (16 October):** @@VERIFY@@
- **It is not ready to release.** Nothing has been compiled for Apple; the Swift and Objective-C code is unbuilt. Nothing has run on a real phone, TV, Fire TV or Vega device (only the Android emulator). Nothing has been installed on Windows, macOS or Docker. No real GPU, receiver or tuner has been used.
- **Nothing was pushed, merged into your own branches or deployed.** All work sits on `audit-implementation/main` in each repository (section 6). Hosted was not redeployed.

## 2. Performance

You asked for per-title cost to stay roughly constant as a library grows, and for a library built from scratch to be quick. Figures come from single runs of the synthetic-library benchmark on this Ubuntu host while it was busy with other work; ratios matter more than absolute times.

**Building and keeping a library up to date**

| Area | Before | After |
|---|---|---|
| New 50,000-file library, empty → fully browsable | 45.9 min | 20.7 min (a third of the processor time) |
| New 200,000-file library, empty → fully browsable | didn't finish (under 5% after 65 min, slowing) | 67.8 min |
| Titles visible during a first scan | only when the scan ends | within seconds of the scan reaching them |
| Adding one new file to a 12,000-title library | 74.8 s | 13 s |
| Adding a film, 50,000 → 200,000 titles | — | flat (11.3 ms) |
| Full rebuild of 200,000 titles' derived data | 44–59 min | about 30–53 min, flat per title to 1 million |
| One show's metadata update / worst artist update | 257 ms / 8.1 s | 11 ms / 7.8 ms |
| "Complete" scan of a film / episode / song | 53 s / 28 s / 4.8 s | 19 s / 10.6 s / 2.3 s |
| Longest database write lock | about 2 s | 0.3–0.7 s |

**Reading at 200,000 titles** (database steps or time)

| Read | Before | After |
|---|---|---|
| Requests over 1 s / over 500 ms | 69 / 327 | 4 / 57 (before the 16 Oct read work) |
| Broad search (median) | 858 ms | 14 ms |
| Broad multi-word search prefixes | 1.1–1.3 M steps | 14 k (same as at 50,000) |
| Search sorted by year or date added | 12–13 M (some timed out) | ~0.35 M |
| Songs/episodes sorted ascending by duration or rating | 1.1 M | 24 k |
| Filters on versions, duration, resolution, cast/crew, subtitle language | 0.2–0.4 M | under 10 k |
| Cold Home at 10,000 titles | 14.5 s | 0.1 s |

@@READS2@@

- Large totals are exact up to 10,000 and then read "10,000+", on every client.
- Scanning never waits on metadata providers; metadata fills in afterwards within each provider's rate limit.
- Watch Together at its 32-member limit costs 1,333 µs per member (was 2,272) and stays flat with room size.
- Million-title runs were stopped early, as you asked.

## 3. Your decisions on 15 October, and things you should know

**Settled on 15 October and built as you asked:**
- **Tuner conflicts:** an owner setting; by default live viewing is never interrupted — the recording waits, starts late as a partial recording if a tuner frees up, or fails with a clear reason and an owner notice. The other option lets the recording take the tuner, with a countdown on every player (counted on the server's clock).
- **Next episode during the credits:** a viewer setting, off by default; the credits card shows what's next on every player, but nothing starts before the title ends.
- **Player choices:** the Settings menu decides how every playback starts; changes made in the player last for that playback only (carried through autoplay and the queue). No per-title memory, no "Forget last choice".
- **Picture-in-picture:** no subtitle support; burned-in subtitles stay, drawn ones disappear until the picture returns.
- **Exact resume in MP3, ADTS and Ogg:** a small per-file seek index, no conversion (web, Android and Apple).
- **Collections:** personal and library collections have a manual order; reorder on web and phones/tablets.
- **Administrator role:** what the server allows that tier; confirmed.
- **API keys are Plex-like:** with the right level a key can manage profiles, members, devices and sessions; it works from any tool and any website, in a header or as `?apiKey=` on GET links (stripped from logs). Only sign-in, passwords and two-factor, creating keys, ownership and the server claim stay sign-in-only.
- **Owners' own certificates are trusted** by the Android and Apple apps once the server proves its Portico identity over them; a different server is never trusted silently. Settings › Servers lists trusted certificates. Vega's SDK can't do this.
- **Apple allows plain HTTP only on the local network;** remote addresses need HTTPS.
- **Any browser reaching the web UI works with no setup;** the `PORTICO_ALLOWED_ORIGINS` variable is gone.
- **Secure connection by default:** creating a server with a Portico Account connects it and requests its certificate as soon as the person approves it in their account (one click — automatic approval would let anyone attach a server to someone else's account). Server Settings has a plain "Secure connection" section. Servers can now also be set up from a phone or tablet.
- **Building from scratch is quick** (section 2).
- **Volume and mute** are the system's everywhere except web.
- **Missing titles** are kept for a server-configured time (30 days by default), with "Never".
- **Dropped from the plans:** spoiler controls, writing NFO/artwork beside media, country rating tables, per-library unrated/kids allow-lists, unified label rules.
- **Accepted defaults:** full-size media engine; Cast burns styled subtitles in; Hosted sends account emails; local-metadata people get person pages; Vega has manual HDR and surround settings; no online audiobook provider or disc playback before release.

**Things you should know:**
- **Every time value on the wire is now in milliseconds** (`…Ms`). A few settings were renamed, so a value saved under an old name returns to its default once. **The server and Hosted must be deployed together** (Hosted's check-in interval changed with it).
- **The apps' mpv engine is Portico's own build** from pinned source (FFmpeg at the server's 8.1.2 configured LGPL 2.1, OpenSSL 3, mpv, libass with a small patch so mixed-script subtitles draw every letter, and the HDR libraries), each a separate dynamic library. Android is built and verified; the Apple build runs on the Mac and is the first step of the Apple test plans. Each release must publish the engine's source bundle (about 133 MB).
- **Every client shows its open-source licences** under Settings › About.
- **Live TV through mpv is written but switched off** (`LIVE_CHANNELS` / `liveChannels`) until a device check; Live TV can otherwise be delivered as fragmented MP4 with HEVC, HDR and lossless audio.
- **Release builds contain no build-machine folder names** (server, Android). The macOS server's bundled FFmpeg still does until its toolchain is rebuilt on the Mac (you approved).
- **Loudness is measured on a stereo mix** of each file's audio (to share the decode); a true surround setup may differ slightly.
- **The TV owner Dashboard is read-only.**
- **A 24 MB benchmark binary** was committed and then removed; it is still in the branch history — squash it out before landing.

## 4. What is not finished

**Open audit bugs (11), mostly for devices, real media or release:** Apple's old unexplained sign-out incident (B-01.23); real-world cost of "Complete" scanning on real files (B-04.35, though it is now 2–3× cheaper); gapless fixtures needing real HE-AAC media (B-10.28); Watch Together's remaining per-member cost at the ceiling (B-13.10, now flat); device cleanup after sign-out on real devices (B-14.14); Android TV density on hardware (B-19.8); Apple and Vega release builds and device acceptance (B-25.10); Apple distribution rights review (B-25.12); release installer signing (B-15.19); library pins are done but the original catalogue-cleanup claim (B-18.27) needs a final read; the native test helper still imitates React internals (B-19.15, you said leave it).

**Needs the Mac or devices:** Apple builds (and the Apple media engine), the iOS build-path fix, the macOS FFmpeg toolchain rebuild, licence texts for nine iOS libraries, turning on Live TV through mpv, compressed DVB subtitles in mpv and in server burn-in (an FFmpeg change), the Cast receiver trusting owner certificates (not possible on Chromecast), anything listed in each lane report's device checklist.

**Small gaps left:** @@GAPS@@

## 5. Testing: done, and still to do

**Done on Ubuntu:** section 1, the performance runs above, and Android emulator checks (sign-in, mpv playback, MP3 audiobook resume, the credits card with the setting off, @@EMU@@).

**Still to do (the [test plans](test-plans/README.md) hold step-by-step cases; build the media engine first on each platform):**
1. On the Mac: build the Apple media engine, then compile, archive and run the apps on iPhone, iPad and Apple TV.
2. Build Android, Android TV and Fire TV and Vega packages and run them on devices.
3. Install the server on Windows, macOS and Docker; upgrade, restart, restore.
4. Real media hardware: HDR, surround receivers, GPUs, a tuner.
5. Firefox, Safari and Edge, and a human walk-through of the web app.
6. Stage Hosted together with the server on an empty PostgreSQL database (Hosted's database tests didn't run here).

Codex will review and test everything again.

## 6. How to land it

| Repository | Programme branch | Head |
|---|---|---|
| portico-server | `audit-implementation/main` (worktree `main-server`) | @@SERVER@@ |
| portico-react-native | `audit-implementation/main` (worktree `main-native`) | @@NATIVE@@ |
| portico-internal | `audit-implementation/main` (worktree `main-internal`) | the commit containing this report |

Worktrees are under `/home/justin/projects/Portico/.worktrees/audit-implementation/`; your own checkouts are untouched. Land the server first (it holds the shared packages and contracts), then native at the matching package revision (its lock file names the server commit), then internal. Start each test server from empty state (one fresh baseline schema, no migrations), and deploy Hosted together with the server.
