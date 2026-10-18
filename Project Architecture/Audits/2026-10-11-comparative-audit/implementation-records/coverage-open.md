| B-01.14 | Medium | open | hosted | - **B-01.14** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another |
| B-01.21 | Low | open | hosted | - **B-01.21** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another |
| B-01.22 | Low | partly | accounts-security | / B-01.22 / Partly. The three retired Hosted routes and the legacy device table are removed. The "hosted" session authority, the `viewer_only` grant t |
| B-01.23 | Medium | open | hosted | - **B-01.23** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another |
| B-02.8 | Medium | open | playback-core | / B-02.8 / Not fixed: deliberate (contract says a sustained re-watch clears watched). / |
| B-02.16 | Low | partly | playback-core | / B-02.16 / Narrowed: uncertain failures lapse after 6 hours; failures are per engine. Still not visible or clearable by the owner. / |
| B-02.22 | Low | open | playback-core | / B-02.22 / Not changed (latent, off by default, belongs to the conversion pipeline). / |
| B-04.10 | Medium | open | subtitles-text | - B-04.10 — Fixed: malformed ASS events are skipped individually; three-digit fractions and tolerant SRT/VTT/SMI/TTML parsing preserve good cues. Inli |
| B-04.18 | Low | open | subtitles-text | - B-04.18 — Not fixed here: unknown-language forced/default automatic-selection policy belongs to playback-core. Flags are preserved for it. |
| B-04.23 | Low | open | subtitles-text | - B-04.23 — Not fixed here: chapter admission belongs to music-audiobooks. |
| B-04.28 | Low | open | subtitles-text | - B-04.28 — Not fixed here: analysis FFmpeg command tolerance belongs to conversion-outputs/library scanning. Subtitle cue tolerance is independent of |
| B-04.35 | Medium | open | subtitles-text | - B-04.35 — Not fixed here: Complete-tier stage cost/profile design belongs to library scanning. Subtitle scan demux cost is removed. |
| B-04.38 | Low | open | subtitles-text | - B-04.38 — Not fixed here: HDR tone mapping and anamorphic preview command construction belong to conversion-outputs. |
| B-04.39 | Low | open | server-followups | - B-04.39 — Not fixed here: outside the assigned server follow-ups; see web-subtitles report(s) for the owning lane’s status. Not re-verified in this  |
| B-05.5 | Medium | open | naming | - B-05.5 — not fixed here; dangling-link recovery belongs to scanning. Existing root/symlink safeguards are retained. |
| B-05.29 | Medium | open | naming | - B-05.29 — not fixed here; tolerant bounded sidecar parsing belongs to local metadata. |
| B-05.31 | Low | open | naming | - B-05.31 — not fixed end-to-end; `Disc 1 - Studio`/`Disc 2 - Live` and separator variants are read by the pure package, but existing audio album asso |
| B-06.20 | Medium | mentioned | providers-matching | - B-06.20: automatic retained source variants are reused without another download/encode; explicit owner artwork choices still fetch the current sourc |
| B-06.21 | High | mentioned | providers-matching | - B-06.21: official absolute numbering now maps to TMDB seasons and receives title/still metadata; alternate offsets remain explicit needs-order. |
| B-06.22 | Low | mentioned | providers-matching | - B-06.22: adult search is now an owner setting, default false. |
| B-06.23 | Low | mentioned | providers-matching | - B-06.23: provider watcher reduced to one second, artwork watcher to two seconds; owner artwork history streams instead of one concatenated allocatio |
| B-06.24 | High | partly | fix-pass | Partly fixed |
| B-07.13 | Medium | open | freshness-server | / B-07.13 / Not fixed here: backup snapshot/WAL-pressure work belongs to ops-packaging. No runtime backup-pressure claim made. / |
| B-07.14 | Low | open | freshness-server | / B-07.14 / Not fixed here: existing post-baseline migrations remain; squashing/removing those is a persistence/ops integration change outside this la |
| B-07.16 | Low | open | freshness-server | / B-07.16 / Not fixed here: write-gate holder alarms belong to dbwork/operations. Existing diagnostics expose held duration; no real leaked gate repro |
| B-07.18 | Low | open | search-sort | - **B-07.18** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes |
| B-07.19 | Low | open | search-sort | - **B-07.19** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes |
| B-08.2 | Medium | open | api-platform | / B-08.2 unknown errors become `400`, unlogged / Not fixed: ~590 legacy call sites; belongs with the registry migration (wave 3). New routes here are  |
| B-08.9 | Medium | open | hosted | - **B-08.9** — Not fixed here — outside Hosted code; belongs to API/freshness. This status does not imply the finding is still present in another impl |
| B-08.11 | Low | partly | api-platform | / B-08.11 unused toolkit pieces described in docs / Partly: the `AuthenticateKey` part is gone; the cursor/idempotency/precondition drift is for the d |
| B-09.30 | Low | open | networking | - B-09.30 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed. |
| B-11.12 | Medium | open | livetv | - B-11.12: Fixed retained partial capture continuation when its live window remains open; sealed captures validate/publish normally. |
| B-11.21 | Medium | open | livetv | - B-11.21: Fixed storage measurement outside the capture write lock; concurrent writes are included conservatively and hard-linked objects count once. |
| B-11.35 | Low | mentioned | livetv | - B-11.35: Deliberate bounded timeshift policy retained; readiness/live delay is now configurable and actual span is reported. Larger viewer timeshift |
| B-12.8 | Low | open | search-sort | - **B-12.8** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lane |
| B-12.9 | Medium | open | search-sort | - **B-12.9** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lane |
| B-12.10 | Low | open | search-sort | - **B-12.10** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lan |
| B-12.22 | Low | open | personal-state | - B-12.22 — fixed: shared counts exclude privacy opt-out and paused history; owner record remains complete, and private per-viewer counts remain disti |
| B-13.3 | High | mentioned | social | - **B-13.3 — base mitigation retained, extended here.** Existing measured lead/settling behavior remains; new seek/load waits reduce residual drift. P |
| B-13.18 | Low | mentioned | native-sync | - B-13.18 — improved, not closed: the pause is set the moment the player has the title. A guaranteed silent first frame needs the engine to start paus |
| B-13.22 | Low | open | social | - **B-13.22 — not fixed: outside owned social/casting paths.** Owner broadcast capacity/audience handling is operations/notification work. The audit’s |
| B-14.14 | Medium | partly | downloads | / B-14.14 / Partly fixed: other profiles' space is shown and can be removed; files are still kept at sign-out (decision 2). Settings stay device-wide. |
| B-14.20 | Low | partly | downloads | / B-14.20 / Partly fixed: automatic retry, Wi-Fi return, restart after process death on next run, service time-out handling. No restart while the proc |
| B-14.21 | Low | open | downloads | / B-14.21 / Not fixed (decision 15). / |
| B-15.7 | Medium | open | api-platform | / B-15.7 tokens not device-key bound though docs say so / Not fixed: accounts-security lane / documents. / |
| B-15.9 | Low | open | networking | - B-15.9 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to i |
| B-15.10 | Low | open | networking | - B-15.10 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to  |
| B-15.13 | Medium | open | networking | - B-15.13 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to  |
| B-15.14 | Low | open | hosted | - **B-15.14** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another |
| B-15.17 | Low | open | hosted | - **B-15.17** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another |
| B-15.19 | Low | open | hosted | - **B-15.19** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another |
| B-15.20 | Low | open | hosted | - **B-15.20** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another |
| B-16.4 | Low | open | server-followups | - B-16.4 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane. |
| B-16.7 | Medium | partly | freshness-clients | / B-16.7 / Partly fixed. Tabs share one stream on a secure page; a hidden tab stops its feed. Over plain HTTP from another machine each *visible* tab  |
| B-16.12 | Low | partly | web-app | - B-16.12 partly fixed: cards keep a focus ring without `:has()`. No browser floor is declared and no build target set. |
| B-16.18 | Low | open | server-followups | - B-16.18 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane. |
| B-16.23 | Medium | open | server-followups | - B-16.23 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane. |
| B-16.25 | Low | open | server-followups | - B-16.25 — Not fixed here: outside the assigned server follow-ups; see web-app report(s) for the owning lane’s status. Not re-verified in this lane. |
| B-18.9 | Medium | open | search-sort | - **B-18.9** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not as |
| B-18.29 | Low | open | search-sort | - **B-18.29** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not a |
| B-18.30 | Low | open | search-sort | - **B-18.30** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not a |
| B-19.5 | Low | open | apple-mpv-review | / B-19.5 / Not fixed — artwork retry behavior. / |
| B-19.6 | Medium | open | apple-mpv-review | / B-19.6 / Not fixed — global JavaScript error handling. / |
| B-19.8 | Low | open | apple-mpv-review | / B-19.8 / Not fixed — Android TV density. / |
| B-19.9 | Low | open | apple-mpv-review | / B-19.9 / Not fixed — unrelated Apple storage/network queue ownership. / |
| B-19.10 | Low | open | apple-mpv-review | / B-19.10 / Not fixed — device form detection. / |
| B-19.11 | Low | open | apple-mpv-review | / B-19.11 / Not fixed — Vega storage. / |
| B-19.12 | Low | open | apple-mpv-review | / B-19.12 / Not fixed — app deletion/sign-in lifecycle. / |
| B-19.14 | Low | open | apple-mpv-review | / B-19.14 / Not fixed — mounted-screen lifetime. / |
| B-19.15 | Low | open | apple-mpv-review | / B-19.15 / Not fixed — broader native test harness; this lane supplies a Swift test file but does not claim a native run. / |
| B-19.16 | Low | partly | native-app | - B-19.16 — partly fixed: the artwork label and the saved-sign-in message. About 70 literals remain in `src/app/session.tsx`, `src/app/direct-connect. |
| B-19.17 | Low | open | apple-mpv-review | / B-19.17 / Not fixed — Hermes plural behavior. / |
| B-19.18 | High | open | apple-mpv-review | / B-19.18 / Not fixed — scene manifest/SDK launch acceptance. / |
| B-19.19 | Low | open | apple-mpv-review | / B-19.19 / Not fixed — unrelated native-app contract leftovers. / |
| B-21.6 | Medium | open | limits-settings | - B-21.6 — not changed: native preference control reachability belongs to native-app; newly added owner controls have shared structure rows where appr |
| B-21.8 | Medium | open | native-app | - B-21.8 — not fixed. Needs a server occurrence anchor (unchanged from the fix pass). |
| B-21.9 | Medium | open | native-app | - B-21.9 — not fixed. A subtitle choice can only be carried by language or by a session resource id that does not exist before play; needs a shared ch |
| B-21.15 | Low | mentioned | native-app | 10. **Back from filters keeps applying changes live** (B-21.15), as Plex does. Contract NAV-6.7 says Back restores; the contract needs correcting, not |
| B-22.6 | High | open | hosted | - **B-22.6** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in anoth |
| B-22.15 | Medium | open | hosted | - **B-22.15** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in anot |
| B-22.19 | High | open | hosted | - **B-22.19** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in anot |
| B-22.22 | Medium | open | hosted | - **B-22.22** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in anot |
| B-22.24 | Medium | partly | networking | - B-22.24 — Fixed structurally: each interface uses a distinct stable DNS-SD suffix, so its partial address RRset cannot conflict with another interfa |
| B-25.10 | High | partly | vega | / B-25.10 (no automated checks for Vega) / Partly: the bridge now has 61 tests that need no device. Device behaviour is still unautomated. / |
| B-25.12 | High | open | hosted | - **B-25.12** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in ano |
| B-25.14 | Medium | open | hosted | - **B-25.14** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in ano |
| B-25.15 | Low | open | hosted | - **B-25.15** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in ano |
| B-25.18 | Medium | open | hosted | - **B-25.18** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in ano |
| B-25.20 | Medium | open | hosted | - **B-25.20** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-25.21 | Medium | open | hosted | - **B-25.21** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in ano |
