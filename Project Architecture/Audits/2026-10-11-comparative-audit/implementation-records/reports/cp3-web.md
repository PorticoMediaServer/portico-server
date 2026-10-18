# cp3-web — web gaps (W1–W11)

Branch `audit-implementation/cp3-web` in `server-cp3-web` and `native-cp3-web`, merged with `audit-implementation/main` (server 2281643a, native 8d4e109) at the end. No subagents or helpers. Nothing pushed.

## 1 Outcome

All eleven items have code.

- **W1 Reorder.**
  - A library collection's reorder mode is now a windowed list over the collection itself, so a 5,000-title collection arranges like a 5-title one.
  - A move needs only the moved title and its new neighbour on hand, and it shows at once until the rows are read back.
  - A personal collection no longer jumps back to page one after a move.
- **W2 Deep refresh.**
  - Playlists, My List and Favorites, personal collections, saved views, history and the collection and view directories now re-read every page on screen when they refresh quietly or after a change made here. That is up to 25 pages of 40.
  - The pages come back as one list, so a change deep in the list shows and the viewer keeps their place.
  - Past 25 pages, the old behaviour stays.
- **W3 Language change.** Cards, search results, the password hint, the Not interested notice, the channel picker and the Library Channels field list redraw at once when the language changes.
- **W4 Font sizes.** The remaining inline px font sizes are now in rem (32 places). Initials in fixed circles and the repeat-one badge are left in px.
- **W5 CJK subtitles.**
  - The server serves its own Noto Sans CJK font (the one used for burn-in) at `GET /v1/subtitle-fonts/cjk`. The browser renderer fetches it only when a styled subtitle has Chinese, Japanese or Korean letters. Hangul now works.
  - Droid Sans Fallback is removed from the web bundle, which saves 4 MB.
- **W6 DVB and VobSub.**
  - DVB character-coded objects are drawn in the region's foreground and background codes, using an offscreen canvas in the browser's sans-serif.
  - VobSub `CHG_COLCON` colour and contrast changes within a line are painted.
- **W7 Join page (web and native).**
  - The join page asks the invitation preview for `forPorticoAccount`. When it is set, the username and password path is not offered.
  - On the server's own address, the page hands off to the Portico web app (`web.getportico.tv/join#code=…&server=…`), because accounts sign in only there.
  - Native hides the "Join with a username and password" browser button for these invitations and shows a short note.
- **W8 4K check.** The 4K decoding question now uses a level that covers 4K: H.264 5.1 (`avc1.640033`), HEVC L153, AV1 5.0 and VP9 5.1.
- **W9 Unused strings.** 396 unused catalogue strings were deleted, including every named stale console and web-owner key that nothing uses. None were wired; none looked like a missing feature.
- **W10 Skipped parts.** Server › Troubleshooting has a new panel, "Parts this app could not read", on web and phone/tablet. It shows the tolerant readers' record (document, part, count) and has a Clear all action.
- **W11 Render checks.** New harness-based render checks cover the windowed reorder list and the skipped-parts panel. Everything else new has pure-logic tests.

## 2 Decisions I made

- **W2 bound:** 25 pages of 40, read in order. Beyond that, a refresh reads only the page in view, as before. Saved cursors bind page size and revision, so the deep read keeps a page size of 40 and gives fresh cursors. After the deep read the list is shown as one long "first page" (cursor null, history empty). Web and native page accumulators therefore need no change.
- **W1 personal collections:** a move is followed by the same quiet deep re-read. There is no blind local re-ordering, because the server's order and fresh cursors are needed for the next page.
- **W5:** the font route is public (`ScopeOpen`, like `/health`). It is a font, and the libass worker fetches fonts with no auth headers. It is served from memory with an ETag and a 30-day cache, through the `media-body` lane, which has no deadline.
  - I removed Droid Sans Fallback rather than keep two CJK fonts.
  - A browser with a subtitle that has CJK letters downloads about 16 MB once.
- **W6:** DVB character codes are read as Unicode code units, with C0/C1 controls dropped. The text is one line high (at most 48 px), and the mask is thresholded at alpha 96. Progressive (zlib) DVB objects stay skipped.
- **W7:** on a bundled server page, the bound invitation hands off to web.getportico.tv. A self-hosted page cannot sign Portico Accounts in.
- **W9:** I deleted rather than wired. Kept:
  - families a screen builds from a prefix (job states, alerts, scan states and similar);
  - anything a test refers to.
  - Roku is outside this programme. Its golden snapshot JSON files were not counted as a use, so they will differ from the catalogue if Roku is ever regenerated.
- **W10:** the panel is for the owner (not the administrator tier) on web and phone/tablet. It sits in Troubleshooting, and the list is this device's own.
- **W11:** the harness renders component functions with hook stubs. A real DOM renderer would be a new dependency, so I did not add one.

## 3 Audit items closed

| Item | Change | Test |
| --- | --- | --- |
| W1 | `WindowedOrderList` (web), `moveRequestAt`/`placeBeforeMoves` (client-core), quiet deep re-read after a personal move | `manual-order.test.ts`, `personal-saved.test.ts` (move keeps 101 entries, no loading state), `reorder-diagnostics-render.test.ts` |
| W2 | `SavedService.readThrough`, `PersonalSavedService.readThrough` | `saved.test.ts`, `personal-saved.test.ts` |
| W3 | `useI18n` in memoised cards, PasswordStrength, NotInterestedNotice, Features; memo dependencies on `t`/`i18n` | tsc, oxlint |
| W4 | 32 inline sizes px→rem | design lint |
| W5 | `GET /v1/subtitle-fonts/cjk`, `mediaexec.CJKFont`, fallback-fonts/ass-renderer wiring, Droid removed, licences regenerated | `TestSubtitleCJKFontIsServedOnceAndRevalidates`, `subtitle-overlay.test.ts` |
| W6 | `dvb.ts` character objects (`canvasText`, `characterText`), `vobsub.ts` `readColourChanges` | `subtitle-images.test.ts` (two new tests) |
| W7 | web `DirectJoin.tsx`, native `Invitation.tsx` | tsc (web and native) |
| W8 | `client-profile.ts` `uhd`/`uhdTenBit` probes | `client-profile.test.ts` |
| W9 | 396 keys deleted from `packages/i18n/src/catalog` | i18n/catalogue core tests, web and native string lints |
| W10 | `tolerant.ts` `subscribeSkipped`/`skippedVersion`, `server.skippedParts` panel (web and native) | `tolerant.test.ts`, render test |
| W11 | render checks | as above |

## 4 API changes

- New `GET /v1/subtitle-fonts/cjk` (`get_subtitle_fonts_cjk`): Public, `ScopeOpen`, `font/otf`, ETag, media lane.
- Contracts and OpenAPI were regenerated, and the restriction matrix entry was added.

## 5 Schema changes

None.

## 6 Settings added

None. The new Troubleshooting panel `server.skippedParts` is not a setting.

## 7 Tests run

- Server:
  - `go build ./...`: OK.
  - `go test ./internal/httpapi -run 'TestSubtitleCJKFont|TestRestrictedProfileRouteMatrix|TestInstalledWebAssets'`: OK.
  - `go run ./cmd/apigen`: clean after the merge.
- client-core:
  - tsc: OK.
  - `personal-saved`, `saved`, `manual-order`, `client-profile`, `tolerant`, `settings-console-pages`, plus the catalogue, i18n and word tests: all pass (62 + 64 + others).
- Web:
  - tsc: OK.
  - `oxlint src` (lint:hooks): clean.
  - `lint-design --strict`: clean.
  - `lint:strings`: clean.
  - `subtitle-images`, `subtitle-overlay`, `direct-join` and `reorder-diagnostics-render` tests: 31 pass.
  - `licences.ts --check`: up to date.
- Native:
  - `tsc --noEmit`: OK.
  - `lint:hooks` (includes Vega source): clean.
  - `lint:strings`: clean.
  - Vega `tsc --noEmit`: OK.
- Not run: whole suites, drivers and emulators, per tonight's brief.

## 8 Device test notes

- **Web reorder:** open a library collection with more than 500 titles. Reorder, scroll to the bottom, move a deep title to the top, then drag one. Check that the move shows at once and the list settles in the server's order.
- **Personal collection:** scroll past page 2 and move a title. Check that the list does not jump back to the top.
- **Saved lists:** open a long playlist or My List and scroll deep. Remove an item on another device. It should vanish here within about 30 seconds without the list jumping.
- **Language:** change the language in Settings. Check that card captions and search results switch at once.
- **CJK subtitles:** play a styled (ASS) subtitle with Korean text and no attached fonts. Check that Hangul draws and that the font is fetched once from `/v1/subtitle-fonts/cjk` (about 16 MB) and then cached. Repeat on the hosted web app, where the font comes cross-origin from the server.
- **DVB:** if a broadcast with character-coded DVB subtitles is available, check the text. No real sample was decoded.
- **VobSub:** check a DVD with karaoke-style colour changes.
- **Join, bound invitation (web):** from the server's own address, check the hand-off button. On the hosted web app, check that there is no "username and password" link.
- **Join, bound invitation (phone):** check there is no browser button.
- **Owner, skipped parts:** Troubleshooting › "Parts this app could not read" should say "Nothing has been left out" against a matching server.

## 9 Not finished

- **W2 bound:** lists longer than 25 pages still refresh only the page in view.
- **W1 personal collections:** each move re-reads up to 25 pages.
- **W1 drag-and-drop:** drag-and-drop in the windowed list across rows that scroll out of view was not tried in a browser. Row menus still work.
- **W6:** progressive (compressed) DVB objects are still skipped. No real DVB or VobSub samples were decoded.
- **W9 other lanes:** a parallel lane that started using one of the deleted keys will fail type-check after merge. Restore the key from the list below.
- **W9 Roku:** the Roku golden snapshots still list the deleted keys.
- **W11:** there are no render checks for DirectJoin's bound-invitation path or for `usePlaybackChoice`/PiP.

## Deleted catalogue keys (396)

```
accountProfiles.createUncertain accountProfiles.created accountProfiles.error.timeout accountProfiles.error.update accountProfiles.renamed accountProfiles.savedNotRefreshed activation.startNewCode activation.waiting audio.cannotMeetBitrate audio.embeddedMetadataSkipped auth.changePassword auth.emailMeToken auth.noServersBody auth.noServersTitle auth.profileUnavailable auth.recoveryToken auth.verificationCodeHint builder.fromTemplate builder.group builder.templateUnavailable caption.episode channel.addToFavorites channel.removeFromFavorites channels.favorite channels.loadingThing count.atLeast count.newNotifications count.people detail.mediaInformation download.added filter.categoryIncomplete home.empty.body home.empty.title invitation.allLibraries invitation.alreadyAccepted invitation.declineBody invitation.declineTitle invitation.expired invitation.incompleteBody invitation.incompleteTitle invitation.libraries invitation.title launch.checkingSavedConnection launch.slow launch.starting lib.browseTab lib.collectionsHint lib.filtersCount lib.from lib.order lib.scanHint lib.search lib.to limits.confirmBody limits.confirmTitle live.allSources live.cantWatch live.cantWatchBody live.channelNumber live.clearFilters live.earlier live.favorite live.favorited live.favoritesOnly live.favoritesOnlyOn live.hidden live.hiddenShown live.hide live.hideChannel live.hideChannelHelp live.import.bytes live.import.channels live.import.enabledChannels live.import.enabledGroups live.import.programmes live.jumpToNow live.keepGoing live.keepGoingBody live.later live.loadFailed live.loadFailedBody live.loadMore live.moreFor live.new live.next live.noChannels live.noChannelsLibrary live.noChannelsSource live.noDescription live.noGuideData live.noGuideDataBody live.noInformation live.noLibraryChannels live.noLibraryChannelsBody live.noMatch live.noMatchBody live.noProgramNow live.noSourcesBody live.now live.nowShowing live.nowTitle live.onNow live.onNowPrefix live.recording live.recordingCanceled live.recordingScheduled live.repeat live.scheduled live.searchChannels live.searchChannelsPrograms live.seriesSetUp live.show live.showChannel live.showChannelHelp live.showHidden live.stale live.staleBody live.today live.tomorrow live.tunerReservation live.unavailable live.until live.watchChannel live.watchChannelNow live.watchNow mediaInfo.container mediaInfo.duration mediaInfo.resolution mediaInfo.video metadata.nfo.fieldsSkipped metadata.provider.fieldsSkipped metadata.settings.artworkFull network.cellular network.wifi personal.listChanged pin.wrong playOn.useOtherRemote player.opt.automaticConverted player.opt.connection player.opt.convertedByHardware player.opt.convertedBySoftware player.opt.convertedForDevice player.opt.hdrConverted player.opt.playingAsIs player.opt.qualityReason player.opt.sourceFile player.opt.stream player.opt.upTo player.quality.automaticHelp player.quality.convert player.quality.convertingAuto player.quality.prepared profile.pinRecoveryCodeOnly profile.pinRecoveryLede recordings.details recordings.seeConflict savedServer.endedTitle savedServer.noAccessTitle savedServer.resetBody savedServer.resetTitle scan.issues security.twoStep.codeHint server.cert.environment server.cert.listener server.cert.listenerBound server.cert.listenerUnbound server.cert.namespace server.connect server.metadata.artworkUsage server.people.scheduleKept server.people.serverTime server.remote.offHelp server.remote.offTitle server.reports.actions server.thing.avoidedRoutes server.thing.libraryChannels settings.manageOnWeb settings.manageOnWebHelp settings.openServerConsole settings.row.downloadsDevice settings.row.downloadsServer settings.row.notifications settings.row.preferences settings.row.together settings.save.leave settings.save.leaveBody settings.save.leaveTitle settings.save.stay settings.server.customCertDomain settings.server.ffmpegOwnHelp settings.server.ffmpegPathHelp settings.server.refresh settings.server.secure.lan-plain-allowed settings.server.secure.preferred settings.server.secure.required settings.server.sessionStorageMiB settings.server.sessionStorageMiBHelp settings.server.stateFolder settings.switchProfileOrServer shell.watchDownloads status.tryingAgain title.appearances title.back title.episodeGroups title.favorite title.firstAired title.moreLikeThis title.originalTitle title.runtime title.seasonTooLarge title.someNotUpdated title.status title.status.continuing title.status.ended title.versions title.watched title.years watched.marked watched.unmarked web.about.builtOn web.access.keyMeta web.access.limitsTitle web.access.maxDevices web.access.peopleLede web.account.notificationsUnread web.activity.now web.activity.playback web.activity.recent web.alerts.certificateWarningLabel web.alerts.storageCriticalLabel web.alerts.storageWarningLabel web.alerts.title web.connection.resetBody web.connectivity.advanced web.connectivity.advertiseUrlHelp web.connectivity.connectionLimitHelp web.connectivity.customCertHelp web.connectivity.encrypted web.connectivity.encryptedHelp web.connectivity.extraAddressesHelp web.connectivity.homeNetworksHelp web.connectivity.qualityLimitHelp web.connectivity.trustedProxiesHelp web.connectivity.uploadSpeedHelp web.connectivity.wanAsLan web.connectivity.wanAsLanHelp web.console.admin web.console.nav.access web.console.nav.accessLede web.console.nav.activity web.console.nav.activityLede web.console.nav.feedback web.console.nav.general web.console.nav.generalLede web.console.nav.liveTv web.console.nav.liveTvLede web.console.nav.logs web.console.nav.maintenance web.console.nav.maintenanceLede web.console.nav.network web.console.nav.networkLede web.console.nav.overview web.console.nav.overviewLede web.console.nav.people web.console.nav.peopleLede web.console.nav.schedules web.console.nav.schedulesLede web.console.nav.storage web.console.nav.transcoding web.console.nav.transcodingLede web.console.section web.dvrSettings.afterConvert web.dvrSettings.afterNone web.dvrSettings.afterRemux web.dvrSettings.folderTokens web.dvrSettings.keepUnwatched web.dvrSettings.keepUnwatchedHelp web.dvrSettings.noFolderTokens web.dvrSettings.savePermission web.dvrSettings.sidecar web.dvrSettings.sidecarFanart web.dvrSettings.sidecarHelp web.dvrSettings.sidecarNfo web.filters.titles web.general.updatesHelp web.general.version web.hostedAccount.asProfile web.hostedAccount.deleteBodyServers web.hostedAccount.expires web.hostedAccount.renamed web.join.accepted web.join.body web.join.declined web.join.libraries web.libraries.rowSubtitle web.libraries.saveScanDepth web.library.eyebrow web.library.kind web.librarySettings.apiKey web.librarySettings.apiKeyOptional web.librarySettings.apiKeySet web.librarySettings.hoverPreviews web.librarySettings.hoverPreviewsHelp web.librarySettings.title web.live.moreFor web.live.next web.live.nextAt web.live.startedLeft web.live.tomorrow web.live.watchChannel web.liveSource.channelsFailed web.liveSource.searchFailed web.logs.captureLede web.logs.captureTitle web.logs.component web.logs.detailKept web.logs.duration web.logs.durationChoice web.logs.recordForAWhile web.logs.retention web.logs.retentionLede web.logs.startCapture web.logs.startCaptureHelp web.logs.startDebug web.logs.stopDebug web.maintenance.backUp web.maintenance.backUpLede web.maintenance.backgroundLower web.maintenance.backgroundNormal web.maintenance.backgroundPriority web.maintenance.backgroundPriorityHelp web.maintenance.backgroundWork web.maintenance.backgroundWorkLede web.maintenance.backupFailed web.maintenance.backupSameDisk web.maintenance.keepLabel web.maintenance.keepMax web.maintenance.pauseHeavyWork web.maintenance.quietHours web.maintenance.quietHoursLede web.maintenance.storage web.metadata.noIdentity web.network.addresses web.network.advanced web.network.advancedHelp web.network.detecting web.network.hideAdvanced web.network.moreInterfaces web.network.protocols web.network.publicAddress web.network.publicAddressHelp web.network.showAdvanced web.nowPlaying.open web.overview.noRecentPlayback web.overview.notMeasured web.overview.recentPlayback web.overview.stopBody web.people.deleteBody web.people.deleteTitle web.people.editProfile web.people.invite web.people.managedHosted web.people.managedHostedHelp web.people.newPin web.people.pinHelp web.people.yourProfiles web.people.yourProfilesLede web.prefs.laneHelp web.prefs.laneMode web.prefs.laneResolution web.prefs.languages web.saved.inProgress web.saved.unwatched web.schedules.noSchedules web.schedules.noSchedulesBody web.search.albums web.search.artists web.search.books web.search.episodes web.search.people web.search.shows web.search.songs web.setup.alreadyComplete web.shell.profileSettings web.storage.actions web.storage.loading web.storage.webdavShares web.telemetry.period30d web.telemetry.period7d web.transcoding.accountCapHelp web.transcoding.addOverride web.transcoding.allowTranscoding web.transcoding.chooseAccount web.transcoding.conversion web.transcoding.overrideHelp web.transcoding.removeOverride
```

## Commits

portico-server (`audit-implementation/cp3-web`, first parent):

- b1a38d70 Arrange a library collection of any length in a windowed list, and keep long saved lists in place when they refresh or a title moves
- dcd13fd0 Redraw labels drawn from catalogue helpers when the viewer changes language
- 3370a240 Size inline text in rem so it follows the browser's font size
- ee4b7395 Draw Korean, Chinese and Japanese styled subtitles in the browser with the server's own CJK font
- 52b4cf7b Draw DVB character-coded subtitles and VobSub colour changes inside a line
- 65dd0cf9 The join page offers only Portico Account sign-in for an invitation made for one account
- 080d6172 Ask the browser about 4K at a codec level that covers 4K
- 98d45ef4 Show the owner what this app left out of the server's answers
- 3135a78b Render checks for the windowed collection reorder list and the left-out parts panel
- 9468d04b Merge of audit-implementation/main
- c6b56e6d Remove 242 catalogue strings no first-party source uses
- f531bc95 Remove 154 more catalogue strings that only the Roku golden snapshots still listed
- 497a30fa Draw DVB character objects without naming a colour

portico-react-native (`audit-implementation/cp3-web`):

- 3d51bc8 Join and owner diagnostics on phones and tablets (with the shared-package mirror)
- ce77876 Merge of audit-implementation/main
- a65ad8c Mirror the shared packages: unused catalogue strings removed
- 8577f5e Record the server commit the shared packages come from (497a30fa)
