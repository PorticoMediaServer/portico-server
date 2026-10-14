# Portico environment acceptance plans

These plans qualify the integrated implementation. They do not certify it. Each environment document includes its own prerequisites, candidate/build instructions, sample-media preparation, numbered steps, expected results, failure symptoms, responsible lanes and a smoke subset. Record **Pass / Fail / Blocked** per case and per OS/device/browser/vendor variant. A successful cross-compile, unit fixture or virtual-device run cannot close a physical acceptance case.

Use fresh pre-release baseline fixtures and isolated copied media. Back up complete state before destructive operations; do not use the demo VPS for conversion, load, restore or deletion. Private credentials/keys/raw grants belong outside Git. Follow the exact candidate SHAs supplied by integration, including matching native copied packages and format-21 Hosted. Do not use an old individual lane branch as a substitute.

## Suggested order

Get one end-to-end path working before expanding variants. Build/launch gates and irreversible cases come first within each plan; then codecs, authority and lifecycle; then presentation and scale. GPU vendor sections repeat the common output checks with their own backend/driver prerequisites. Use verified prebuilt packages while the implementation programme’s full-build prohibition applies; build instructions are for the later qualification runner.

| Order | Environment | Smoke cases | Full cases |
| --- | --- | --- | --- |
| 1 | [Linux packages](linux-packages.md) | 1, 2, 3, 4, 5 | 17 |
| 2 | [Web browsers](web-browsers.md) | 1, 2, 3, 6 | 31 |
| 3 | [Reverse proxy and networks](reverse-proxy-and-networks.md) | 1, 2, 4 | 8 |
| 4 | [Third-party API](third-party-api.md) | 1, 2, 4, 6, 7 | 15 |
| 5 | [GPU conversion](gpu-conversion.md) | 1, 2, 7, 8, 9, 10 | 10 |
| 6 | [Live TV, tuner and IPTV](live-tv-tuner-and-iptv.md) | 1, 2, 4 | 8 |
| 7 | [iPhone and iPad](iphone-ipad.md) | 1, 2, 7, 10 | 29 |
| 8 | [Apple TV](apple-tv.md) | 1, 5, 6, 10 | 26 |
| 9 | [Android phone and tablet](android-phone-tablet.md) | 1, 2, 6, 9 | 30 |
| 10 | [Android TV and Fire TV](android-tv-fire-tv.md) | 1, 4, 5, 8 | 25 |
| 11 | [Windows server](windows-server.md) | 1, 2, 3, 4 | 12 |
| 12 | [macOS server](macos-server.md) | 1, 2, 3, 4 | 11 |
| 13 | [Docker](docker.md) | 1, 2, 3, 4, 5 | 11 |
| 14 | [Hosted services](hosted.md) | 1, 2, 8 | 9 |
| 15 | [Vega](vega.md) | 1 | 8 |

## Smallest end-to-end set

This is the smallest practical **core product** proof, not release coverage. Use one Linux private server, production Chrome, one mpv-capable handset, one GPU host, one one-tuner source, two profiles and a real webhook/MCP receiver. The same server can host the first four if its hardware permits. Read each named case for all its steps; do not replace it with an HTTP status check.

- [linux-packages.md](linux-packages.md), cases 1, 3, 4, 5.
- [web-browsers.md](web-browsers.md), cases 3, 6, 9, 11.
- [iphone-ipad.md](iphone-ipad.md), cases 1, 2, 3, 7.
- [gpu-conversion.md](gpu-conversion.md), cases 1, 2.
- [live-tv-tuner-and-iptv.md](live-tv-tuner-and-iptv.md), cases 1, 2.
- [third-party-api.md](third-party-api.md), cases 1, 2, 4, 6.

Run the appropriate GPU vendor case as well. Substitute the Android handset plan for the Apple handset only for this initial core proof. Hosted-assisted sign-in adds Hosted schema/consent/signing-rollout and third-party code-flow cases. Native first-build gates can run before any functional acceptance; compile failure is a failing gate, never a reason to skip that platform.

To prove **the whole promised product**, extend to every plan’s smoke subset on actual Windows/macOS/Linux/Docker targets, each available GPU vendor, every browser, iPhone/iPad/Apple TV/Android/Fire TV, real tuner/provider, Hosted and third-party callback receiver. Features must then pass all non-smoke cases on their supported platforms. Vega authenticated transport is restored with the owner-accepted SDK redirect risk. Its sign-in/containment cases are required; full-client parity still cannot be declared complete before the remaining scopes and devices pass. TV downloads are hidden because current native downloader excludes TV. Actual fMP4-only live publication and qualified compressed Apple audio are checked honestly, including refusal/gaps. No small run can establish those missing boundaries or the 200-browser/100-stream goal.

## Run record and routing

For every result keep case number/name, environment variant, candidate/source SHA and artifact hash, fixture hash/provenance, exact steps/times, expected versus actual, relevant request/response readback, redacted logs, screenshot/video or receiver/driver trace, and reproducibility. Keep private raw evidence outside Git. A failing case routes to its **Route to** lanes; shared paths and mixed cases may need more than one lane. Stop only the destructive case if isolation is compromised and continue independent cases. Do not deploy/push/live-publish as part of this handoff.

Some deterministic tests require a qualification-only hook: storage/Keystore loss, command callback races, crash between durable seal and publication, poison generation/version, delayed player view or a held writer. Plans retain their desired result and identify existing source regressions. Without a suitable device/debug harness, mark the physical variant Blocked and run the named source/helper test separately. A source test must not be reported as the missing physical pass. Real HE-AAC/PS, Dolby Vision, disc bitmap subtitles, HDR display and receiver fixtures must be acquired lawfully with reference evidence; merely tagging synthetic SDR does not produce them.

## Source-note coverage

The initial intake was frozen at the midday handoff. This update reconciles the later running log and final review/screen/health reports available at the evening checkpoint. Source reports were read from `_program/reports/` (shared programme directory) and the ten dated fix-pass reports from the read-only canonical `portico-internal/Project Architecture/Audits/2026-10-11-comparative-audit/fix-pass/lane-reports/`. That fix-pass folder is absent in this documentation worktree. Source hashes and unresolved notes are recorded in the test-plans lane report. Scale-check-2 is still running; its final larger results and source snapshot will require candidate integration and recorded acceptance. These plans already include its deferred Home and mixed-matrix checks.

The table maps source notes to merged cases; duplicate notes are variants of one case, not separate test obligations. Historical compile/unit results are background evidence, not missing environment cases. Programme decisions override old private-only HTTP, scheduled-task Windows service, outdated Hosted format and title-only grouping expectations. Those superseded expectations are intentionally replaced by current source behavior.

| Source report | Concrete case destinations |
| --- | --- |
| accounts-security.md | [windows-server.md](windows-server.md), cases 9; [reverse-proxy-and-networks.md](reverse-proxy-and-networks.md), cases 3; [hosted.md](hosted.md), cases 4; [web-browsers.md](web-browsers.md), cases 2, 18, 23 |
| android-mpv.md | [android-tv-fire-tv.md](android-tv-fire-tv.md), cases 4, 5, 6, 7, 8, 10 |
| android-native-review.md | [android-phone-tablet.md](android-phone-tablet.md), cases 1, 2, 3, 4, 6, 9, 18, 19, 20; [android-tv-fire-tv.md](android-tv-fire-tv.md), cases 5 |
| api-platform.md | [third-party-api.md](third-party-api.md), cases 1, 2, 4, 5, 6, 11 |
| apple-mpv-review.md | [iphone-ipad.md](iphone-ipad.md), cases 1, 7, 8, 10, 11, 21; [apple-tv.md](apple-tv.md), cases 6 |
| apple-mpv.md | [iphone-ipad.md](iphone-ipad.md), cases 1, 7, 8, 10, 21; [apple-tv.md](apple-tv.md), cases 3, 6, 9, 12 |
| apple-native-review.md | [iphone-ipad.md](iphone-ipad.md), cases 2, 3, 4, 5, 19, 20; [apple-tv.md](apple-tv.md), cases 6, 7 |
| audio-engines.md | [web-browsers.md](web-browsers.md), cases 22; [iphone-ipad.md](iphone-ipad.md), cases 19; [android-phone-tablet.md](android-phone-tablet.md), cases 18 |
| base-health-2.md | [web-browsers.md](web-browsers.md), cases 13, 18; [macos-server.md](macos-server.md), cases 9; [hosted.md](hosted.md), cases 6 |
| base-health.md | [linux-packages.md](linux-packages.md), cases 5, 8, 11; [gpu-conversion.md](gpu-conversion.md), cases 3 |
| console.md | [web-browsers.md](web-browsers.md), cases 1, 2, 14, 15, 16, 17, 18, 19, 20, 21 |
| conversion-outputs.md | [gpu-conversion.md](gpu-conversion.md), cases 1, 2, 3, 4, 5, 7, 8, 9, 10 |
| decision-audit.md | No finalized concrete environment note at cutoff; generic scope retained in [linux-packages.md](linux-packages.md), cases 13 and [web-browsers.md](web-browsers.md), cases 3 |
| docs-internal.md | [third-party-api.md](third-party-api.md), cases 11; [linux-packages.md](linux-packages.md), cases 5; [hosted.md](hosted.md), cases 4 |
| downloads.md | [windows-server.md](windows-server.md), cases 8; [iphone-ipad.md](iphone-ipad.md), cases 2, 3; [android-phone-tablet.md](android-phone-tablet.md), cases 2, 3, 20; [web-browsers.md](web-browsers.md), cases 21 |
| e2e-hosted.md | [hosted.md](hosted.md), cases 2, 3, 4, 8 |
| e2e-server.md | No finalized concrete environment note at cutoff; generic scope retained in [linux-packages.md](linux-packages.md), cases 13 and [web-browsers.md](web-browsers.md), cases 3 |
| e2e-web.md | [web-browsers.md](web-browsers.md), cases 3, 9 |
| freshness-clients.md | [web-browsers.md](web-browsers.md), cases 9; [apple-tv.md](apple-tv.md), cases 13; [reverse-proxy-and-networks.md](reverse-proxy-and-networks.md), cases 6 |
| freshness-server.md | [linux-packages.md](linux-packages.md), cases 13; [web-browsers.md](web-browsers.md), cases 9; [reverse-proxy-and-networks.md](reverse-proxy-and-networks.md), cases 6 |
| grouping-sorts.md | [web-browsers.md](web-browsers.md), cases 11; [iphone-ipad.md](iphone-ipad.md), cases 14; [linux-packages.md](linux-packages.md), cases 13 |
| handoff-draft.md | [windows-server.md](windows-server.md), cases 2; [hosted.md](hosted.md), cases 1; [vega.md](vega.md), cases 1 |
| hosted-followups.md | [hosted.md](hosted.md), cases 1, 2, 4, 5, 6, 7 |
| hosted.md | [hosted.md](hosted.md), cases 1, 2, 3, 4, 5, 6 |
| identity.md | [linux-packages.md](linux-packages.md), cases 4, 5, 8; [windows-server.md](windows-server.md), cases 3 |
| integrator.md | [web-browsers.md](web-browsers.md), cases 3, 11, 22; [third-party-api.md](third-party-api.md), cases 9, 11 |
| leftovers-native.md | [iphone-ipad.md](iphone-ipad.md), cases 4, 5; [android-phone-tablet.md](android-phone-tablet.md), cases 19, 21, 23; [vega.md](vega.md), cases 1; [reverse-proxy-and-networks.md](reverse-proxy-and-networks.md), cases 7 |
| leftovers-server-a.md | [linux-packages.md](linux-packages.md), cases 6, 9, 12, 14; [hosted.md](hosted.md), cases 3 |
| leftovers-server-b.md | [linux-packages.md](linux-packages.md), cases 1, 6, 14; [web-browsers.md](web-browsers.md), cases 23; [live-tv-tuner-and-iptv.md](live-tv-tuner-and-iptv.md), cases 2 |
| limits-settings.md | [reverse-proxy-and-networks.md](reverse-proxy-and-networks.md), cases 5, 8; [linux-packages.md](linux-packages.md), cases 7, 9; [web-browsers.md](web-browsers.md), cases 25 |
| livetv.md | [live-tv-tuner-and-iptv.md](live-tv-tuner-and-iptv.md), cases 1, 2, 4, 5, 6, 7 |
| music-audiobooks.md | [linux-packages.md](linux-packages.md), cases 11; [web-browsers.md](web-browsers.md), cases 3, 22; [iphone-ipad.md](iphone-ipad.md), cases 19; [android-phone-tablet.md](android-phone-tablet.md), cases 18 |
| naming.md | [linux-packages.md](linux-packages.md), cases 11; [macos-server.md](macos-server.md), cases 9; [windows-server.md](windows-server.md), cases 7 |
| native-app.md | [iphone-ipad.md](iphone-ipad.md), cases 5, 6, 13, 14, 16, 18; [android-phone-tablet.md](android-phone-tablet.md), cases 4; [android-tv-fire-tv.md](android-tv-fire-tv.md), cases 19 |
| native-health.md | [iphone-ipad.md](iphone-ipad.md), cases 14, 15; [vega.md](vega.md), cases 3 |
| native-playback.md | [iphone-ipad.md](iphone-ipad.md), cases 1, 3, 7, 8, 10, 22; [android-tv-fire-tv.md](android-tv-fire-tv.md), cases 5; [apple-tv.md](apple-tv.md), cases 6 |
| native-sync.md | [iphone-ipad.md](iphone-ipad.md), cases 11, 13, 17, 18; [apple-tv.md](apple-tv.md), cases 16; [android-phone-tablet.md](android-phone-tablet.md), cases 23 |
| networking.md | [reverse-proxy-and-networks.md](reverse-proxy-and-networks.md), cases 1, 2, 3, 4, 5, 7, 8 |
| ops-packaging.md | [windows-server.md](windows-server.md), cases 1, 2; [macos-server.md](macos-server.md), cases 1, 2; [linux-packages.md](linux-packages.md), cases 1, 2, 6; [docker.md](docker.md), cases 2, 9 |
| personal-state.md | [web-browsers.md](web-browsers.md), cases 14; [iphone-ipad.md](iphone-ipad.md), cases 3, 8, 15 |
| planner-wiring.md | [gpu-conversion.md](gpu-conversion.md), cases 1, 2, 3, 4, 7, 8, 9, 10; [third-party-api.md](third-party-api.md), cases 8 |
| playback-core.md | [web-browsers.md](web-browsers.md), cases 3; [iphone-ipad.md](iphone-ipad.md), cases 7, 9; [third-party-api.md](third-party-api.md), cases 8; [gpu-conversion.md](gpu-conversion.md), cases 4 |
| providers-matching.md | [linux-packages.md](linux-packages.md), cases 11, 12; [web-browsers.md](web-browsers.md), cases 17 |
| registry.md | [third-party-api.md](third-party-api.md), cases 9, 11; [iphone-ipad.md](iphone-ipad.md), cases 15 |
| review-accounts-security.md | [windows-server.md](windows-server.md), cases 2; [hosted.md](hosted.md), cases 4; [web-browsers.md](web-browsers.md), cases 18 |
| review-api-platform.md | [third-party-api.md](third-party-api.md), cases 1, 3, 4, 6, 9 |
| review-downloads.md | [web-browsers.md](web-browsers.md), cases 21; [iphone-ipad.md](iphone-ipad.md), cases 3; [android-phone-tablet.md](android-phone-tablet.md), cases 2, 20 |
| review-freshness.md | [linux-packages.md](linux-packages.md), cases 13; [web-browsers.md](web-browsers.md), cases 9, 14 |
| review-hosted.md | [hosted.md](hosted.md), cases 1, 2, 4, 5 |
| review-identity.md | [linux-packages.md](linux-packages.md), cases 4, 5, 8, 13; [macos-server.md](macos-server.md), cases 3 |
| review-livetv.md | [live-tv-tuner-and-iptv.md](live-tv-tuner-and-iptv.md), cases 1, 2, 3, 4, 7 |
| review-native-js.md | [iphone-ipad.md](iphone-ipad.md), cases 2, 6, 7; [android-phone-tablet.md](android-phone-tablet.md), cases 2, 5; [vega.md](vega.md), cases 4 |
| review-networking.md | [reverse-proxy-and-networks.md](reverse-proxy-and-networks.md), cases 1, 3, 4, 5, 7, 8 |
| review-ops.md | [windows-server.md](windows-server.md), cases 2, 5, 6; [macos-server.md](macos-server.md), cases 2; [docker.md](docker.md), cases 9 |
| review-playback-core.md | [iphone-ipad.md](iphone-ipad.md), cases 7, 8; [third-party-api.md](third-party-api.md), cases 8 |
| review-playback-pipeline.md | [gpu-conversion.md](gpu-conversion.md), cases 2, 3, 4, 6, 7, 8, 9, 10 |
| review-registry.md | [third-party-api.md](third-party-api.md), cases 11 |
| review-scan.md | [linux-packages.md](linux-packages.md), cases 5, 8, 11, 13 |
| review-server-late.md | [windows-server.md](windows-server.md), cases 3; [third-party-api.md](third-party-api.md), cases 10; [live-tv-tuner-and-iptv.md](live-tv-tuner-and-iptv.md), cases 4 |
| review-web-logic.md | [web-browsers.md](web-browsers.md), cases 3, 7, 9, 19, 22 |
| scale-check.md | [linux-packages.md](linux-packages.md), cases 13, 17; [web-browsers.md](web-browsers.md), case 31 |
| scan-loop.md | [linux-packages.md](linux-packages.md), cases 8, 11, 13; [macos-server.md](macos-server.md), cases 9; [windows-server.md](windows-server.md), cases 9 |
| search-sort.md | [web-browsers.md](web-browsers.md), cases 11; [iphone-ipad.md](iphone-ipad.md), cases 14 |
| security-sweep.md | [windows-server.md](windows-server.md), cases 3; [reverse-proxy-and-networks.md](reverse-proxy-and-networks.md), cases 1; [third-party-api.md](third-party-api.md), cases 9, 12; [hosted.md](hosted.md), cases 4 |
| server-docs.md | [windows-server.md](windows-server.md), cases 1, 2; [macos-server.md](macos-server.md), cases 1; [linux-packages.md](linux-packages.md), cases 1; [third-party-api.md](third-party-api.md), cases 2, 4, 6, 11 |
| server-followups.md | [web-browsers.md](web-browsers.md), cases 17, 19, 21; [windows-server.md](windows-server.md), cases 3; [third-party-api.md](third-party-api.md), cases 2, 6, 11 |
| social.md | [web-browsers.md](web-browsers.md), cases 23; [iphone-ipad.md](iphone-ipad.md), cases 17, 20; [android-phone-tablet.md](android-phone-tablet.md), cases 16, 23 |
| subtitles-text.md | [windows-server.md](windows-server.md), cases 8; [web-browsers.md](web-browsers.md), cases 6, 7, 8; [third-party-api.md](third-party-api.md), cases 9; [linux-packages.md](linux-packages.md), cases 9 |
| vega.md | [vega.md](vega.md), cases 1, 2, 3, 4, 5 |
| web-app.md | [web-browsers.md](web-browsers.md), cases 9, 11, 12, 13, 24 |
| web-player.md | [web-browsers.md](web-browsers.md), cases 2, 3, 4, 5, 8, 20, 22, 23 |
| web-subtitles.md | [web-browsers.md](web-browsers.md), cases 3, 5, 6, 7, 8, 22, 25 |
| fix-pass/report-catalogue-playback.md | [linux-packages.md](linux-packages.md), cases 11, 13; [gpu-conversion.md](gpu-conversion.md), cases 3, 4; [web-browsers.md](web-browsers.md), cases 3, 22, 24; [third-party-api.md](third-party-api.md), cases 9 |
| fix-pass/report-hardware-conversion.md | [gpu-conversion.md](gpu-conversion.md), cases 1, 2, 3, 5, 7, 8, 9 |
| fix-pass/report-hosted.md | [hosted.md](hosted.md), cases 1, 4, 5 |
| fix-pass/report-identity-ops-live.md | [windows-server.md](windows-server.md), cases 2; [linux-packages.md](linux-packages.md), cases 1, 6; [docker.md](docker.md), cases 9; [live-tv-tuner-and-iptv.md](live-tv-tuner-and-iptv.md), cases 2, 4, 6 |
| fix-pass/report-native-platform.md | [iphone-ipad.md](iphone-ipad.md), cases 8, 10; [android-phone-tablet.md](android-phone-tablet.md), cases 4, 19; [android-tv-fire-tv.md](android-tv-fire-tv.md), cases 5 |
| fix-pass/report-native.md | [iphone-ipad.md](iphone-ipad.md), cases 5, 6, 12, 13; [android-phone-tablet.md](android-phone-tablet.md), cases 5; [web-browsers.md](web-browsers.md), cases 20 |
| fix-pass/report-platform-packaging.md | [windows-server.md](windows-server.md), cases 1, 6, 9, 10; [macos-server.md](macos-server.md), cases 1; [linux-packages.md](linux-packages.md), cases 2 |
| fix-pass/report-scanning-metadata.md | [linux-packages.md](linux-packages.md), cases 11, 12, 13; [macos-server.md](macos-server.md), cases 9 |
| fix-pass/report-social-downloads.md | [iphone-ipad.md](iphone-ipad.md), cases 3, 17; [web-browsers.md](web-browsers.md), cases 21, 23 |
| fix-pass/report-web-client-core.md | [web-browsers.md](web-browsers.md), cases 5, 12, 13, 23, 24; [iphone-ipad.md](iphone-ipad.md), cases 18 |

## Evening additions and known gaps

The numbered cases above include group scope on every client, one receipt Undo, real Play on/owner control, 428/412 rebasing, phone/tablet owner panels, TV read-only behavior, whole-view queues, Vega authentication/redirect containment, origin-wide 120-second activation backoff, table retention, final-baseline restore and deferred Home. Run these alongside the original smoke cases. The known screen/server gaps must be reported individually; passing server query capture does not pass a UI that enrolls only 500. A group’s own order under filters, missing Home/personal/book surfaces, native paged quiet updates and local fonts remain open.

| Later source | Added acceptance coverage |
|---|---|
| base-health-3.md | linux-packages.md cases 15–17; Windows/macOS/Docker final-baseline cases |
| native-owner.md | iphone-ipad.md cases 25–27; android-phone-tablet.md cases 26–28; every TV Dashboard case |
| native-health-2.md | native grouping and revision cases; post-mirror suites passed per the later running log; repeat after any later scale merge |
| native-screens.md | each native grouping/Undo/Play on/common offline/queue case |
| web-owner.md | web-browsers.md cases 28–29 |
| web-viewer.md | web-browsers.md cases 26–27, 30 |
| queue-and-retention.md | third-party-api.md cases 13, 15; Linux case 16; all client whole-view queue cases |
| claim-and-gate.md | hosted.md case 9; Apple/Android TV origin-gate cases; Vega case 2 |
| vega-transport.md | vega.md cases 1–5, including accepted fallback capture/retirement |
| scale-check-2.md | linux-packages.md case 17; web-browsers.md case 31; integration prerequisite explicit |
| handoff-update.md | updated source/toolkit and owner-decision coverage in all plans |

There are **250 numbered cases across 15 environment plans** at this checkpoint. None of this update executes physical qualification.
