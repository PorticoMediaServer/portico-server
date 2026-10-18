# Outcome

Completed the evening owner report refresh, all fifteen acceptance plans and Hosted toolkit/contract alignment. The report now records the final server suites, actual automated Chrome checks, the owner’s four answers, later phone/tablet panels, removed TV actions, functional Vega with accepted redirect risk, disk recovery and precise remaining work. Later STATE evidence records 655 native/87 Vega tests after the final shared refresh; the earlier native-owner receipt was before that refresh.

Acceptance plans contain 250 numbered cases (203 before this lane). They include every supported client’s group controls, real receipt Undo, actual Play on and owner transport, phone/tablet owner panels, read-only TV Dashboard, whole-view capture, Vega authentication/redirect containment, 428/412 rebasing, activation outage, retained housekeeping and final-baseline/restore behavior. Obsolete blanket Vega refusal and 500-cap-as-success expectations are removed.

Hosted apikit is byte-identical to the observed main-server copy. Regenerated OpenAPI/TypeScript includes typed route builders, current declarations and explicit union validation. Hosted build, relevant focused tests, strict generated TypeScript, toolkit tests and selected actual PostgreSQL tests pass. No runtime deployment, push, commit, native build or full suite was performed.

Execution host: personal mini-1, ehler-homelab-mini-1. Context-sync failed because its lock directory is read-only; relevant canonical guidance and the Justin writing-style skill were read directly. No helper or parallel agent was used. Disk was about 100 GB free at final checks.

# Decisions I made

- Current owning-lane final receipts supersede midday text. Keep earlier results as dated evidence in their lane paragraphs, not as current blockers.
- Justin’s owner-panel, Vega-risk, restore-key and wide-grouping answers are settled decisions. No question or approval was requested.
- Use the later final STATE native-suite receipt instead of implying only pre-refresh suites exist. Explicitly distinguish that log receipt from the native-owner report and from physical evidence.
- Count the current Bugs-section evidence across all 588 IDs. Owning remaining-bug reports override out-of-area “not fixed here”; later delivered screen/transport evidence overrides stale owning gaps. Compound gaps remain open. Categorisation uses selected lane-base language, so fixed/already/not-present counts differ from the midday table; every selected sentence is below.
- Classify B-22.19 as intentional under the explicitly accepted fallback, not strict redirect prevention. Keep B-22.21 intentional for removal of required-HTTPS policy; the Vega transport’s same-ID PIN change is narrower, so its sentence does not define that original finding.
- B-18.27 remains partial despite the native-owner lane calling its new panels fixed: the original finding is wider than those panels. B-21.11 remains open despite completed server capture: screens still stop at loaded/500 entries. Do not equate the new APIs with complete product parity.
- Selected inherited status ambiguities were adjudicated from their full sentence: B-06.5/16 retain predecessor fixes (already); B-08.11/13.22 have actual documentation/fan-out fixes despite a separate “not present” subclaim; B-13.17 fixes the remaining owned cases; B-20.19 has a source recovery fix despite unexecuted device reproduction; B-09.34 removes startup disclosure. B-18.13’s actual preview/typed-confirmation code supersedes the contradictory no-confirmation note.
- Test missing candidate implementation as Fail against promised scope; missing fixture/platform prerequisite is Blocked. Existing server explicit-item 500 bound remains valid and is distinct from complete query capture.
- Use one owned, socket-only PostgreSQL cluster with reject host authentication, generated throwaway database and a private URL file. Delete all owned runtime data afterwards. Initial helper used a nonexistent createdb location; corrected to the installed createdb and reran successfully. No production/shared cluster was touched.

# Bugs

No domain bug list was assigned for implementation. All current report Bugs sections supply evidence for the owner report’s 588 unique register IDs. Recomputed counts: **359 reported fixed, 173 already fixed on the selected lane base, 7 reported not present, 11 intentional, 38 open/partial**. Open reviewed severities: **0 High, 20 Medium, 18 Low**. These are source-report dispositions, not runtime reproduction of every claim.

New source closures since the midday open set: B-02.16, B-11.8, B-13.18, B-16.18, B-18.4 and B-21.6. B-22.19 moves to accepted/intentional. B-18.27 and B-21.11 deliberately remain partial. Completed independent reviews total **137 inventory items: 1 blocker, 42 high, 75 medium, 19 low**, including Hosted’s one coverage addition. Browser nine-row findings/server three repairs are separate, not added twice.

# API changes

No Hosted route added/removed and no Hosted wire request/response shape changed. Hosted still has 99 OpenAPI operations; final server inventory has 622. Toolkit-generated contracts gain safe raw/encoded path builders and current declared metadata; previously unconstrained nullable/union types now reflect and validate their alternatives. Generated files were regenerated, not hand-edited. No Go handler adaptation was required: current Hosted registration/build and contract-current tests already use the revised toolkit successfully.

# Schema changes

None. Focused PostgreSQL tests initialized disposable format-21 state. No compatibility code or persistent database migration was added.

# Owner or viewer settings added or wired

None by this lane. Owner-report Appendix B now shows current phone/tablet reachability and late actions; quiet-list clocks and query/retention behavior are documented as implementation choices rather than new user switches. Appendix C retains existing component/licence inventory and states that no late frontend/transport dependency was added.

# Needed from others

- Orchestrator reviews the owner report line by line, integrates only the final cumulative snapshot including its new-file archive, then retains the shared lane reports with the internal handoff. Do not stack cumulative snapshots.
- Scale-check-2 is still running: final-code 10k matrix is complete, 200k/one-million results and final integration/cleanup receipt pending. After scale/contracts integration, regenerate/copy packages and rerun the necessary health checks.
- Client/server follow-ups are explicit in owner section 4: query selector screen forwarding, Home/personal/book/cross-library/group-order/person-role fields, native paged quiet refresh and shared adapters/catalogue, local subtitle fonts, occurrence/subtitle identities, native playback-problems/episode-order panels, other owner contracts, platform download/audio/Cast gaps, release/rights/locks/signing and physical acceptance.

# Test notes for other environments

Actually run here, GOFLAGS=-p=2 for all Go commands:

- Hosted `go build ./...`: pass.
- Apikit `go test . ./precondition ./cursor ./apierror -count=1`: all four packages pass.
- Hosted `go test ./internal/contract ./internal/httpapi ./internal/oidclogin -count=1`: all three packages pass. Includes current-contract/golden responses, served-route equality, consent/current-origin and HTTP guards.
- Apikit `go vet . ./apierror ./cursor ./precondition`: pass.
- Hosted `go vet ./internal/contract ./internal/httpapi ./internal/oidclogin`: pass.
- Strict generated TypeScript: existing server tsc, `--noEmit --strict --skipLibCheck --target es2022 --module nodenext --allowImportingTsExtensions hosted-services/hosted/contracts/src/hosted/*.ts`: pass; nothing installed.
- Current main-native `PORTICO_SERVER_REPO=…/main-server node scripts/shared-packages.mjs --check`: identical. Read-only check only; no base tree changed.
- Actual isolated PostgreSQL 18.6 `go test ./internal/postgres -run 'Test(AppAuthorization|CheckInDeliversLatestCertificate|OlderReplicaRefusesRevokedConsent|HostedRejects|MemberConsentRechecksRevocation|DeviceDecisionRejects)' -count=1 -v`: **13 top-level tests pass, 11.470 s**, including expiry subcases, fresh-only schema, PKCE single-use/concurrency, cookie/consent, stale replicas/certificates and revoked device approver. Zero skipped selected tests. Cluster stopped and scratch removed.
- Whole-toolkit diff: identical. OpenAPI operation counts read directly: Hosted 99, server 622.
- Document checks: exact 588-ID register equality/no duplicates, statuses equal compact Appendix D, 38 open rows and reviewed severities, all current lane report files represented, 331 local links resolve, all 250 plan case numbers sequential, README environment totals match. Owner wording/placeholder scan and `git diff --check` pass. The word Opus appears as an audio codec, not an attribution.

No selected database-backed test was unavailable. Other PostgreSQL tests were deliberately not run under the programme’s full-suite prohibition; the exact unrun inventory follows. Earlier e2e-hosted/review-hosted database evidence is retained as earlier evidence and is not advertised as this lane’s full test pass. Physical Windows/macOS/Apple/Android/TV/Vega/GPU/tuner/provider/public-network/callback acceptance remains unexecuted by this lane. The numbered plans give cold steps, readback and pass/fail symptoms.

# Not finished

No assigned handoff or toolkit implementation remains unfinished. Scale and the programme’s named client/server/release/physical acceptance work remain unfinished; no release approval is implied. Final cumulative snapshot 003 is complete and readback matches its tracked diff/new-file inventory. No worktree edits follow it.

## Statements not verified against a lane report or source code

- **Post-refresh native 655/655 and Vega 87/87 test passes:** the final STATE line is the only later completed receipt available. Native-owner supplies the same figures before refresh, not that later run; this lane did not rerun suites or inspect that runner’s raw logs. Owner report names the running-log source explicitly. Final package equality is independently verified by the read-only shared-package check.
- **Post-refresh native type checks:** also the final STATE receipt; no later native-owner report proves those particular commands. Do not describe these as commands this lane executed.
- **Disk-full recovery and no repository work lost:** STATE is the incident record for 83 GB cache, 18 GB scratch, connectivity checks and untouched main checkouts. This lane observed current space and readable heads, but did not reproduce the incident or independently prove absence of every lost temporary artifact. Owner wording is limited to repository work; synthetic databases were deliberately deleted.
- **Exact cumulative delivery/publication history:** the programme log and lane reports state no push/deployment or merge into Justin’s working branches. This lane did not independently audit every remote/deployment environment. No publishing operation was performed here.

No other unsupported positive claim was knowingly retained. Source-report claims are attributed, not promoted to independently rerun results. The report expressly withholds device builds/playback, installer/GPU/tuner acceptance, native visual quality, sustained large-library capacity, legal/distribution rights and blanket security assurance. Those are unknown outcomes, not asserted successes.

## Source-backed claims not independently exercised here

Owner sections 1–2 and 5–7 use base-health-3/native-owner/the later STATE receipt and the real-server/browser/Hosted reports. Section 3 uses PROGRAM-BRIEF answers and the owning lane Decisions sections; alternative designs are recommendations, not claims of implementation. Section 4 uses later Needed/Not finished sections; Appendix A names each source report. Appendix B’s exact stored keys/defaults use the original owning settings inventories plus web-owner/native-owner/native-screens/queue-and-retention reachability; this lane did not run every setting. Appendix C uses apple-mpv/android-mpv/web-subtitles/server-docs notices, not a legal decision. Appendix E preserves earlier owner-choice evidence with late session/Undo/scope/screen corrections.

Explicit current code readbacks: actual bulk-delete preview/typed confirmation, TV-only TelevisionDashboard guard in native ServerPage.tsx, exact-owner web management gate, web album English disc parser, Hosted generator/current-contract tests, toolkit/TypeScript output, current operation counts, shared-package equality and observed base heads. Remaining broad behavior descriptions are supported by their lane reports rather than a new exhaustive code pass.

Conservative unresolved claim scopes: B-16.10 wider presentation salvage and B-18.27 the original compound dormant-control/catalogue entry are not completely proven by later screen reports. They remain open; no original whole-entry closure is inferred. Whole-view screen parity, local-font rendering, owner transport receiver obedience and native quiet paged refresh are likewise not inferred from their implemented service/rule pieces.

## Database tests not run in this lane

| Test file | Unrun top-level tests |
|---|---|
| account_email_link_test.go | `TestRegistrationLinkConfirmsFromAnotherDevice`, `TestVerificationTokenRoundTrip`, `TestEmailChangeLinkCompletesFromAnotherDevice` |
| account_management_proof_test.go | `TestAccountManagementNeedsNoProofButCredentialChangesDo` |
| account_receipt_authority_test.go | `TestSensitiveReceiptsRequireCurrentAuthority`, `TestTerminalMFAReceiptOnlySurvivesItsOwnRevocation`, `TestLegacyUnboundReceiptFailsClosed` |
| account_security_test.go | `TestVerifiedRegistrationAtomicRetryAndMailEligibility`, `TestMFAEnrollmentRevocationAndSingleUseRecovery`, `TestContactChangeAndRecoveryStayAccountBound` |
| apikit_idempotency_test.go | `TestAPIIdempotencyConcurrentPostgresReplay` |
| apple_notifications_test.go | `TestAppleNotificationRevocationAndDuplicates`, `TestAppleRelayOrderingAndDeletion`, `TestAppleRevocationCompletionFenceAndFreshConsent`, `TestAppleUnknownSubjectAndManualUnlink`, `TestAppleNotificationHTTPBoundary` |
| burst_test.go | `TestRecoveryBurstIsShedFastNotQueuedToTimeout` |
| certificate_admission_test.go | `TestCertificateOrderBudgetIsSharedAndNeverExceeded` |
| certificate_queue_test.go | `TestCertificateIdleQueueDoesNotProbeTerminalHistory`, `TestCertificateAuthorityReconciliationCommitsEvenWhenIdle`, `TestCertificateCleanupDueIsReservedAndRetiredTransactionally`, `TestAmbiguousCertificateRemainsQuarantined` |
| claim_routes_test.go | `TestCurrentHostedClaimMiddlewareJourney` |
| claim_web_test.go | `TestWebClaimApprovalPushAndAccountIsolation`, `TestUnboundWebClaimBindsTheApprover`, `TestWebClaimsAreCappedPerServer`, `TestWebClaimNewerRequestReplacesAnUndecidedOne` |
| current_family_test.go | `TestCurrentFamilyAuthorizationBindingsAndExpiry`, `TestCurrentFamilyMalformedInputAndRequiredFamily`, `TestCurrentFamilyLockWaitUsesFreshClockAndCancellation`, `TestCurrentFamilyGenerationAndRevocationSerialization`, `TestCurrentFamilyActualRefreshRetainsAccessButInvalidatesApproval`, `TestCurrentFamilyAuthorizationUsesBoundedPrimaryKeyReads` |
| deletion_test.go | `TestDeletionNoHistoryRevokesCredentialsAndConcurrentIdempotence`, `TestDeletionChangedPreviewAndPasswordBlock`, `TestDeletionSignedErasureRequiresExactServerAck`, `TestDeletionHTTPPreviewReplayAndServerRouteAuthorization`, `TestDeletionTransactionFailureRestoresAuthorityAndPolicy` |
| device_test.go | `TestDeviceApprovalRequiresExplicitAccountAndZeroServers`, `TestDeviceCreateRetryAndLostRedemptionRecoverOneSession`, `TestDevicePollingSlowdownExpiryDenialAndPrivateSecret`, `TestDeviceCancelFencesConsumedSessionAndPasswordChangeFencesApproval`, `TestConcurrentDeviceConsumptionOneEffect`, `TestDeviceHTTPApprovalAuthenticationAndTrustedProxyQuotas`, `TestDeviceApprovalRaceRecoveryExpiryAndCodeGuessQuota` |
| direct_address_test.go | `TestDirectAddressFeedOnlyMovesOnAChange`, `TestWithdrawingAFamilyIsAChangeTheFeedCarries` |
| endpoint_test.go | `TestCurrentEndpointClaimAckDiscoveryAndLateCancellation` |
| hosted_audit_test.go | `TestCancelledClaimWithdrawsBothFamiliesAndDeletionPreview`, `TestDeletionCompletesWithOwnedCancelledAndMissingClaim`, `TestOriginalKeyCleanupAfterOwnerDeletion`, `TestMailDiscardProgress` |
| i01_integration_test.go | `TestI01FreshAndRepeatedSchemaIncludesBothGroups`, `TestI01FailedOrderWithoutTXTStillRetainsIssuerAndCleanupLease`, `TestI01ProviderMFACompletesOIDCAndSharesExactReplayReceipt` |
| identity_schema_test.go | `TestCurrentIdentitySchemaRequiresCurrentFormatAndRestart`, `TestCurrentIdentitySchemaAuthorityTuplesAndTTL`, `TestCurrentIdentitySchemaDeletionOwnershipAndCascades`, `TestPasswordAbsenceDeniesEveryCurrentPasswordBoundary`, `TestCurrentPasswordDeletionAccountFirstAndExactReceiptReplay`, `TestCurrentPasswordDeletionFailureRollsBackAuthority`, `TestCurrentIdentitySchemaInitializationIsAtomic` |
| integration_test.go | `TestPasswordSessionsPersistAndRevoke`, `TestEarlyAckAndLeaseCompletionCannotLoseAcknowledgement`, `TestWake429BackoffAndRestartDurability`, `TestHTTPRealDatabaseJourneyAndOriginBoundaries`, `TestCheckInAcknowledgesWakesMonotonically`, `TestStaleWakeFinisherDoesNotOverwriteNewLease`, `TestDatabaseOutageIsNotBadPassword` |
| legacy_adoption_test.go | `TestMigrateDryRunOnAnEmptyDatabase` |
| lifecycle_test.go | `TestLifecycleHTTPRoutes` |
| member_label_test.go | `TestMemberRemovalRotatesMemberLabel`, `TestMemberLabelRotationKeepsRemainingMembersResolving` |
| notify_test.go | `TestQueueEnqueuesAnnounceThemselves`, `TestARolledBackEnqueueAnnouncesNothing`, `TestNextDueReportsTheEarliestDeliverableDeadline`, `TestAccountExportRequestsAnnounceThemselves`, `TestOfflineScanReportsEachSilenceOnce`, `TestSigningKeyAlertReachesTheOperator`, `TestOfflineScanDrainsAndReportsTheNextDeadline`, `TestReadinessSeesTheRunningListener` |
| oidc_completion_security_test.go | `TestNativeCompletionProof` |
| oidc_login_test.go | `TestOIDCBindingCompletionAndRevocation`, `TestNativeSSOInitiatorCannotTakeVictimSession`, `TestCurrentHostedRestartNeedsNoDDL`, `TestNativeSignInRequiresTheCompletionCode` |
| provider_link_test.go | `TestProviderLinkNeedsNoProofAndUnlinkKeepsTheLastMethod` |
| refresh_test.go | `TestRefreshRotationInputOutputRetryAndReuseRevocation`, `TestConcurrentSameRefreshRequestHasOneRotation`, `TestRefreshExpirySupersessionAndLogoutAll`, `TestRefreshFamilyLifetimeAndPasswordLogoutBoundaries`, `TestBrowserRefreshCookieModeAndLostHeaderBoundary`, `TestDeviceRefreshFamilyCancelAndLogoutAllFencesApprovals`, `TestConcurrentDifferentRefreshRequestsRevokeFamily`, `TestConcurrentRefreshAndLogoutAllCannotLeaveCredentials`, `TestBrowserExpectedFamilyCannotMutateNewLogin`, `TestRefreshReceiptOutlivesAnOutageUntilTheSuccessorIsUsed` |
| remediation_test.go | `TestAccountExportProofIsolationExpiryAndRetention`, `TestPasswordSessionCreatesRevocableDeviceAndNotificationPreferences`, `TestPasswordBackoffIsPerAccountAndClearsOnSuccess`, `TestPasswordBackoffCeilingNeverRefusesACorrectPassword`, `TestPasswordBackoffCountsParallelGuessesAtomically`, `TestSecurityEventsAreInAppNotifications` |
| routes_candidates_test.go | `TestRouteSetsDeduplicateNormalizedOrigins` |
| routes_review_test.go | `TestW2I02RouteHistoryDeduplicatesAndCannotResurrectMismatch` |
| routes_secure_lan_test.go | `TestSecureLANRouteDocument` |
| rpo_marker_test.go | `TestRPOMarkerOnlyAfterWrites` |
| scale_test.go | `TestScale200kAuthenticatedHeartbeat` |
| server_members_test.go | `TestMemberPushIndexesServersForDiscovery`, `TestServerListVersionCheckFromMemory`, `TestIdentityAssertionIsShortLivedAndAudienceBound`, `TestOwnerChangeMovesCustodyOnlyWithTheNewOwnersConsent`, `TestMemberPushNeedsConsentAndHonoursDepartures`, `TestMemberPushReplayDigestAndPages`, `TestCustodyIsReleasedWhenTheOwnerIsNotListed`, `TestServerListVersionMovesOnRenameAndVariesByList` |
| server_presence_test.go | `TestServerListLastSeen` |
| server_retirement_test.go | `TestServerRetirementQueuesCleanupWithoutAnAccountDeletion` |
| session_review_test.go | `TestAccountSessionReviewOwnershipFactsAndRevocation`, `TestAccountSessionReviewExpiryAndBoundedPages`, `TestAccountSessionReviewRevokeRacesRefreshAndMutualRevocation`, `TestAccountSessionReviewHTTPRevokeDoesNotTouchCookies`, `TestAccountSessionReviewPasswordChangeRaceFencesCaller`, `TestAccountSessionReviewCallerExpiresWhileWaitingForAccountLock` |
| signing_delivery_test.go | `TestRevocationFilteringDoesNotSelectAnUnlockedCustodian` |
| signing_rollout_test.go | `TestSigningRolloutCheckpointsOnceAndRejectsRollback` |
| timestamp_contract_test.go | `TestStoreTimestampsUseUTCContract` |

## Per-ID disposition evidence

F = reported fixed; A = inherited/already fixed on selected lane base; N = not present; I = intentional; O = open/partial. A selected source sentence can describe a partial unit; adjudication above and owner section 6 govern compound entries. Later unnumbered screen Outcomes/Decisions/Needed sections supply B-02.16, B-13.18 and B-21.6 closures. These are source-backed counts, not acceptance results.

| ID | Status | Selected report | Evidence / adjudication |
|---|---|---|---|
| B-01.1 | A | review-accounts-security.md |  /  B-01.1  /  Already fixed: closed-hours renewal refuses without revocation; pending-device bypass additionally fixed by R5.  /  |
| B-01.2 | F | review-accounts-security.md |  /  B-01.2  /  Fixed: existing issuance guard retained; R5 closes the pending-device path.  /  |
| B-01.3 | A | decision-audit.md |  /  B-01.3  /  already fixed in source  /  identity direct membership/device/session paths; httpapi/connection_policy.go and api_credentials_routes.go; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-01.4 | F | accounts-security.md |  /  B-01.4  /  Fixed. The switch is the identity policy; the legacy device table and header path are deleted.  /  |
| B-01.5 | A | accounts-security.md |  /  B-01.5  /  Already fixed on the base branch.  /  |
| B-01.6 | F | accounts-security.md |  /  B-01.6  /  Fixed. The waiting television's record is kept and the same code works once approved.  /  |
| B-01.7 | F | accounts-security.md |  /  B-01.7  /  Fixed. The owner is exempt; the owner and administrators can list members' devices.  /  |
| B-01.8 | F | accounts-security.md |  /  B-01.8  /  Fixed. "Hosted attached" is whether the claim is installed.  /  |
| B-01.9 | A | accounts-security.md |  /  B-01.9  /  Already fixed for 403; now no 403 ends a sign-in at all. A 401 from a gateway still does.  /  |
| B-01.10 | F | accounts-security.md |  /  B-01.10  /  Fixed. One rating rule.  /  |
| B-01.11 | F | accounts-security.md |  /  B-01.11  /  Fixed. Punctuation and spacing no longer matter; more countries' ratings are known. "A" stays 18 on purpose.  /  |
| B-01.12 | F | accounts-security.md |  /  B-01.12  /  Fixed. Names are blanked on member deletion, Portico Account erasure and profile deletion.  /  |
| B-01.13 | F | accounts-security.md |  /  B-01.13  /  Fixed. Invite-only sign-up spends the invitation inside the sign-up, after the username check.  /  |
| B-01.14 | A | leftovers-server-a.md |  /  B-01.14  /  Already fixed: httpapi/onboarding.go delegates privateSetupPeer to the same configured requestIsRemote used by playback; the networkpolicy parser remains shared.  /  |
| B-01.15 | F | accounts-security.md |  /  B-01.15  /  Fixed. A short bounded wait replaces the instant "busy".  /  |
| B-01.16 | F | accounts-security.md |  /  B-01.16  /  Fixed. The PIN is hashed before the transaction opens.  /  |
| B-01.17 | F | accounts-security.md |  /  B-01.17  /  Fixed. Installation ids no longer travel in addresses; another device's id is not shown.  /  |
| B-01.18 | F | accounts-security.md |  /  B-01.18  /  Fixed for session records (90 days). The per-account failed-attempt ledger is still unpruned.  /  |
| B-01.19 | F | accounts-security.md |  /  B-01.19  /  Fixed. Refresh reuse and Portico erasure go through the clean-up hook; the schedule, remote-policy and tier-change revocations no longer exist.  /  |
| B-01.20 | F | accounts-security.md |  /  B-01.20  /  Fixed at every site the audit listed.  /  |
| B-01.21 | F | leftovers-server-a.md |  /  B-01.21  /  Fixed: Devices/EditDevice and Remember/ForgetBrowserAccount require account management; secondary-profile regression covers all four. TV approval and PIN-free profile switching remain deliberate programme decisions.  /  |
| B-01.22 | F | leftovers-server-a.md |  /  B-01.22  /  Fixed for session admission: removed Hosted family issuance/renewal/horizon, fail-open restrictions and authority branches, viewer_only declaration/propagation, and legacy per-profile PIN counters. Retired-authority, quick-connect and local-family tests pass. Development-only policy fixtures and central erasure namespaces remain separate from session admission.  /  |
| B-01.23 | O | leftovers-server-a.md |  /  B-01.23  /  Not fixed: historical simulator incident has no correlated request trace or established cause. V01 refutes the guessed single-profile-cleanup bug and the current native source gates cleanup on accountGrant. Added/tested shouldRetireAccountSession defensive same-family check; native adoption and actual incident reproduction remain external work.  /  |
| B-02.1 | F | playback-core.md |  /  B-02.1  /  Fixed. No translation; a device's own per-transport lists are used, and HEVC/HDR prefer fragmented MP4.  /  |
| B-02.2 | F | playback-core.md |  /  B-02.2 = B-20.1  /  Fixed. Passthrough list plus output route in the one document.  /  |
| B-02.3 | F | playback-core.md |  /  B-02.3  /  Fixed. embeddedAudioSwitching is an engine field.  /  |
| B-02.4 | F | playback-core.md |  /  B-02.4 = B-20.2  /  Fixed. toneMapsHdr is declared; appleProfile sets it with hardware HEVC.  /  |
| B-02.5 | F | playback-core.md |  /  B-02.5  /  Fixed (per-container direct lists kept, interlaced kept, codec names folded, subtitle kinds are subtitleRendering).  /  |
| B-02.6 | F | playback-core.md |  /  B-02.6 = B-17.1  /  Server side and shared-client plumbing fixed (one PATCH; V1QueuePlayer.changePlayback). The player screens still have to call it: see section 7.  /  |
| B-02.7 | F | playback-core.md |  /  B-02.7  /  Fixed. Legacy start removed; no key is 428 idempotency_key_required.  /  |
| B-02.8 | I | review-playback-core.md | - B-02.8 — Not fixed: sustained unfinished rewatch clearing watched is deliberate under the current contract, as V02 confirms. |
| B-02.9 | F | playback-core.md |  /  B-02.9  /  Fixed. One classifier (requestIsRemote, the owner's home networks).  /  |
| B-02.10 | F | playback-core.md |  /  B-02.10  /  Fixed. No per-lane HDR switch; HDR is refused only by the owner or by the viewer's disableHdr.  /  |
| B-02.11 | F | playback-core.md |  /  B-02.11  /  Fixed (best version for this device).  /  |
| B-02.12 | A | review-playback-core.md | - B-02.12 — Already fixed on base. Stored/current ceilings remain enforced; the strengthened subtitle route test now also holds MaxConversions=1. |
| B-02.13 | F | playback-core.md |  /  B-02.13  /  Fixed: a timeline error with an engine-failure code is recorded for the file; the re-plan itself is PATCH … failure.  /  |
| B-02.14 | F | conversion-outputs.md | - B-02.14 — producer fMP4 conversion is implemented; planner admission for a device with only fMP4 belongs to playback-core and is not yet wired here. |
| B-02.15 | F | playback-core.md |  /  B-02.15 = B-20.9  /  Fixed by declaration (codecTags); device behaviour unverified.  /  |
| B-02.16 | F | web-owner.md | Unnumbered Outcome / Decisions / Settings evidence; recorded in reconciliation. |
| B-02.17 | A | playback-core.md |  /  B-02.17  /  Already fixed.  /  |
| B-02.18 | F | playback-core.md |  /  B-02.18  /  Fixed.  /  |
| B-02.19 | F | playback-core.md |  /  B-02.19  /  Fixed.  /  |
| B-02.20 | F | playback-core.md |  /  B-02.20  /  Fixed (80 % guard at the v1 timeline).  /  |
| B-02.21 | F | playback-core.md |  /  B-02.21  /  Fixed (positionMs on PATCH; the shared client sends it automatically).  /  |
| B-02.22 | F | leftovers-server-a.md |  /  B-02.22  /  Fixed production exposure: startup no longer reads PORTICO_FINITE_VOD; an environment variable cannot force all HLS streams through that experiment. Internal experiment limits remain.  /  |
| B-03.1 | A | decision-audit.md |  /  B-03.1  /  already fixed in source  /  decoder qualified target construction and playback conversion_targets/HLS admission/retention; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-03.2 | F | conversion-outputs.md | - B-03.2 — fixed: SDR scale qualification is independent of unavailable CUDA tone mapping. |
| B-03.3 | A | decision-audit.md |  /  B-03.3  /  already fixed in source  /  decoder qualified target construction and playback conversion_targets/HLS admission/retention; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-03.4 | A | decision-audit.md |  /  B-03.4  /  already fixed in source  /  decoder qualified target construction and playback conversion_targets/HLS admission/retention; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-03.5 | A | decision-audit.md |  /  B-03.5  /  already fixed in source  /  decoder qualified target construction and playback conversion_targets/HLS admission/retention; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-03.6 | A | conversion-outputs.md | - B-03.6 — already fixed on base; only a demonstrated successful software retry counts toward the hardware circuit. Source failures do not. |
| B-03.7 | N | conversion-outputs.md | - B-03.7 — not present as claimed: V03 refutes the source-clock burst claim. Removed redundant conversion options; retain one final relative prev_forced_t cadence expression. |
| B-03.8 | F | conversion-outputs.md | - B-03.8 — fixed: owner-adjustable default 4 GiB session cap, scaled file limit, played-segment reclamation before stopping. |
| B-03.9 | F | conversion-outputs.md | - B-03.9 — fixed for bounded unindexed sources: streaming packet index fallback retains source keyframes. Tiny 19-second MPEG-TS indexing and repeated copied seeks pass, including decoded source-picture hash equality. A source that cannot be indexed inside the existing 20-second bound still converts. |
| B-03.10 | F | conversion-outputs.md | - B-03.10 — fixed: working folder and background conversion limit are consumed; restart paths distinguish requested/effective values. |
| B-03.11 | F | conversion-outputs.md | - B-03.11 — fixed: bounded history outlives conversion artifact/session cleanup; successful hardware fallback is recorded too. |
| B-03.12 | F | conversion-outputs.md | - B-03.12 — fixed in command construction: discard copied-audio packets before the rebased seek origin. Apple/Cast audible preroll acceptance still requires devices. |
| B-03.13 | F | conversion-outputs.md | - B-03.13 — fixed: uncapped software output also scales to even dimensions and square pixels; rotation/anamorphic sources use software decode/filtering. |
| B-03.14 | F | conversion-outputs.md | - B-03.14 — fixed: prepared recipes preserve source cadence with a 60 fps ceiling instead of forcing 30; validation accepts it. All shipped recipe dimensions/rates are accepted. |
| B-03.15 | A | conversion-outputs.md | - B-03.15 — already fixed on base for high-rate AAC; extended with explicit supported rates and an explicit unknown-source fallback. |
| B-03.16 | F | conversion-outputs.md | - B-03.16 — fixed for converted H.264/HEVC: matching forced profile/level, display geometry and HDR range. Omit unsupported sequence/subtype guesses. |
| B-03.17 | N | conversion-outputs.md | - B-03.17 — not reproduced: ordinary FFmpeg rebases decoded seek timestamps. Relative expression also remains on the source-clock linear producer. Separately fixed a real fMP4 issue: muxer offsets alone did not preserve fragment clocks; closed fragments are now shifted before publication. |
| B-03.18 | F | conversion-outputs.md | - B-03.18 — fixed: bounded, cursor-based aged-orphan cleanup also runs below disk pressure, protecting live session/artifact/reservation rows and producers. |
| B-03.19 | F | conversion-outputs.md | - B-03.19 — fixed configuration: linear video decode uses two threads instead of one. Actual 4K live throughput requires tuner/device acceptance. |
| B-04.1 | F | subtitles-text.md | - B-04.1 — Fixed: text sidecars and embedded discoveries become selectable and eligible for automatic choice immediately, including remote discoveries whose evidence matches the session. Image selection uses the device/burn path. |
| B-04.2 | F | subtitles-text.md | - B-04.2 — Fixed: removed arbitrary eight-track scan imports; manifests rank up to 32 usable text tracks, selected first, then flags/language. |
| B-04.3 | A | limits-settings.md | - B-04.3 — already fixed on this base: discovery no longer couples every subtitle extraction to basic/deep analysis success. |
| B-04.4 | F | subtitles-text.md | - B-04.4 — Fixed: no scan-wide 90-second extraction loop; extraction uses independent progress and overall budgets per track. |
| B-04.5 | F | limits-settings.md | - B-04.5 — fixed: useful tracks are retained and the owner can raise the per-file inventory limit (128 default, 4096 maximum). |
| B-04.6 | F | subtitles-text.md | - B-04.6 — Fixed: language names/aliases, regional tags, forced/foreign/default/SDH/CC/HI and descriptive titles are recognized. |
| B-04.7 | F | subtitles-text.md | - B-04.7 — Fixed: matching sidecars attach to both same-stem video versions. |
| B-04.8 | F | subtitles-text.md | - B-04.8 — Fixed: canonical subtitle reasons use string-catalogue messages, including ambiguous association. |
| B-04.9 | F | subtitles-text.md | - B-04.9 — Fixed: BOM, UTF-8, UTF-16 and scored legacy charset decoding replaces blanket Windows-1252 fallback. |
| B-04.10 | A | leftovers-server-a.md |  /  B-04.10  /  Already fixed: CanonicalAsset text-only fallback salvages inline-attachment ASS without rich/burn admission; tolerant event processing retains usable dialogue and original bytes. Inline-attachment/tolerant regressions pass.  /  |
| B-04.11 | F | limits-settings.md | - B-04.11 — fixed: image-subtitle acquisition/extraction/sidecar readers follow the owner size limit (128 MiB default, 512 MiB maximum); bounded metadata/text/image-geometry checks remain. |
| B-04.12 | F | subtitles-text.md | - B-04.12 — Fixed cancellation/restart defect: discoveries select immediately, and cache fills survive first-reader cancellation. Full-source extraction is still required for cold embedded tracks. |
| B-04.13 | F | conversion-outputs.md | - B-04.13 — fixed overlay geometry: aspect-preserving fit and transparent padding. Tiny cropped PGS glyph test passes; real VobSub/DVB placement remains a manual acceptance item. |
| B-04.14 | F | web-subtitles.md |  /  B-04.14  /  Web part done (attached fonts used; fallback fonts for seven scripts). Burn-in fonts are conversion-outputs'.  /  |
| B-04.15 | A | subtitles-text.md | - B-04.15 — Already fixed on base: shared complete ISO language table; not redone. |
| B-04.16 | A | subtitles-text.md | - B-04.16 — Already fixed on base: same ISO language normalization for forced matching; not redone. |
| B-04.17 | F | subtitles-text.md | - B-04.17 — Fixed: probe and sidecar hearing-impaired facts survive inventory/revisions and reach choices, ranking and HLS. |
| B-04.18 | F | leftovers-server-a.md |  /  B-04.18  /  Fixed: one untagged forced track fallback and sole default selection with explicitly enabled/no-language preference; explicit off and preferred-language tests pass.  /  |
| B-04.19 | F | subtitles-text.md | - B-04.19 — Fixed: episode/provider/series/season identity, regional language and profile language are supplied; the web search no longer sends the UI locale. |
| B-04.20 | F | subtitles-text.md | - B-04.20 — Fixed server authentication lifecycle: username/password login, renewal, rate handling and rejected-login suppression replace startup-token environment input. Owner credential screen remains a client/owner-console task. |
| B-04.21 | F | subtitles-text.md | - B-04.21 — Fixed viewer exposure of weak unreviewed hypotheses; the underlying heuristic is not replaced by cross-episode fingerprinting. |
| B-04.22 | F | subtitles-text.md | - B-04.22 — Fixed: identical intro/recap guess is no longer emitted twice. |
| B-04.23 | A | leftovers-server-a.md |  /  B-04.23  /  Already fixed: assets.CleanChapters isolates invalid entries and retains a bounded timeline with partial warnings; chapter regression passes. No unsupported origin subtraction was invented.  /  |
| B-04.24 | F | subtitles-text.md | - B-04.24 — Fixed server resource gap: matching pending choices can address discovered text and image resources. Client title-page choices still need integrated platform verification. |
| B-04.25 | F | subtitles-text.md | - B-04.25 — Fixed: literal curly-brace text survives; only ASS override blocks beginning with a backslash are stripped. |
| B-04.26 | F | web-subtitles.md |  /  B-04.26  /  Fixed on web for styled tracks drawn as originals (libass lays out overlapping signs). Text cues unchanged.  /  |
| B-04.27 | F | subtitles-text.md | - B-04.27 — Fixed: scans retain indexed subtitle directory pages for local and remote sources; queries filter association before candidate bounds. Explicit refresh uses filtered helper enumeration, so thousands of unrelated entries do not consume the subtitle candidate budget. |
| B-04.28 | F | leftovers-server-a.md |  /  B-04.28  /  Fixed for optional visual analysis: recoverable corruption no longer aborts on its first error; 5% ceiling and publication bounds remain. Exact audio recipes stay strict; real damaged-file qualification remains in test notes.  /  |
| B-04.29 | F | subtitles-text.md | - B-04.29 — Fixed: trickplay stores sheets and descriptors, not duplicate per-frame JPEG artifacts. |
| B-04.30 | F | subtitles-text.md | - B-04.30 — Fixed: preview tiles and chapter images honor authorized conditional requests with 304. |
| B-04.31 | A | decision-audit.md |  /  B-04.31  /  already fixed in source  /  subtitles inventory/cache/original/device delivery and bounded parsers; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-04.32 | A | limits-settings.md | - B-04.32 — already fixed by the base’s snapshot/foreground subtitle changes (listed in the fix log). |
| B-04.33 | F | subtitles-text.md | - B-04.33 — Fixed: removed dead duplicate subtitle-plan implementation/tables and obsolete parser/clock/style helpers. |
| B-04.34 | F | subtitles-text.md | - B-04.34 — Fixed: subtitle extraction accepts the MPEG demuxer used by VOB/MPG/MPEG sources. |
| B-04.35 | O | leftovers-server-a.md |  /  B-04.35  /  Reduced default Complete cost: checksum/diagnostic fingerprint passes are explicit Custom choices; actual persisted Complete profile and Custom override regression passes. Shared-decode optimization remains a future improvement, not a measured capacity claim.  /  |
| B-04.36 | F | subtitles-text.md | - B-04.36 — Fixed: v1 advertises a real .vtt endpoint, including direct sessions; JSON documents remain separately identified. |
| B-04.37 | F | subtitles-text.md | - B-04.37 — Fixed: HLS carries selected/default/autoselect, forced and SDH signalling; Off does not default a container track. |
| B-04.38 | F | leftovers-server-a.md |  /  B-04.38  /  Fixed command/fact wiring: preview source colour facts, HDR tone mapping, qualified Dolby Vision reshaping and display-aspect correction reach chapter/trickplay/scene graphs. Closed-recipe tests pass; visual/GPU acceptance remains external.  /  |
| B-04.39 | A | leftovers-server-a.md |  /  B-04.39  /  Already fixed: current web-media-player.ts ignores HLS subtitle offset changes below 20 ms; the former bridge path is gone and an existing fragment-jitter regression covers it. No browser-flicker reproduction is claimed.  /  |
| B-04.40 | F | subtitles-text.md | - B-04.40 — Fixed owner-facing setting description: chapter/scene suggestions and review, without promising cross-episode comparison. Private architecture/spec prose still needs reconciliation. |
| B-05.1 | A | naming.md | - B-05.1 — already fixed on the base according to the fix log; scan cadence is outside this lane. |
| B-05.2 | F | scan-loop.md | - B-05.2 — fixed: changed coverage is protected, diagnosed and retried; analysis continues. |
| B-05.3 | F | scan-loop.md | - B-05.3 — fixed in scanning: local file facts are reobserved, including in-place writes; changed-during-analysis paths are queued again. Playback’s strict source pin is retained. |
| B-05.4 | F | scan-loop.md | - B-05.4 — fixed: unreadable subtree isolated, diagnosed, kept and tested under a non-root user. |
| B-05.5 | A | leftovers-server-a.md |  /  B-05.5  /  Already fixed: storage/inventory_page.go skips unreadable/out-of-root symlinks with protected warning issues; root containment remains enforced. Inventory regression passes.  /  |
| B-05.6 | F | naming.md | - B-05.6 — fixed for newly enumerated local and managed publication paths through built-in exclusions. Existing cached/catalogued exclusions need the scanner's reconciliation/invalidation work. |
| B-05.7 | F | naming.md | - B-05.7 — fixed in the shared reader and catalogue display adapter, extending the base's already-fixed unambiguous cases. Ambiguous bare scene years retain title alternatives. The matcher must adopt this reader for the end-to-end fix. |
| B-05.8 | F | identity.md |  /  B-05.8 second file is a second movie; upgrade loses state  /  Fixed (films, tracks, parts).  /  |
| B-05.9 | F | identity.md |  /  B-05.9 renaming does not change the reading  /  Fixed for films and episodes. Audio is identified by tags, not names.  /  |
| B-05.10 | F | naming.md | - B-05.10 — fixed in the episode adapter: flat folders, folder years, translated/abbreviated seasons and intervening disc folders are read. |
| B-05.11 | F | naming.md | - B-05.11 — fixed in the episode adapter: folder equality is no longer a gate; filename identity/season wins, with folder alternatives retained. |
| B-05.12 | A | decision-audit.md |  /  B-05.12  /  already fixed in source  /  naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-05.13 | F | naming.md | - B-05.13 — fixed in the reader for dates, title text ending in Part N, x-ranges/lists and marker text in episode titles. Eight-link/numeric bounds remain. Episode zero and real stacks are read but still require identity/schema support for attachment. |
| B-05.14 | F | naming.md | - B-05.14 — fixed in the extras adapter: top-level show/category names are not swallowed and a season hierarchy interrupts accidental extras inheritance. |
| B-05.15 | F | scan-loop.md | - B-05.15 — fixed as one unsupported disc-folder entry; no disc playback claimed. |
| B-05.16 | F | naming.md | - B-05.16 — fixed for explicit samples and bare/suffixed trailers in media-owner context. Well-named films called Sample or Trailer are retained. Arbitrary backdrops layouts are not newly classified. |
| B-05.17 | F | naming.md | - B-05.17 — fixed for newly read display names: release tags, provider hints, editions, hashes, parts and whitespace are separated from titles. Old accepted catalogue rows are not rewritten here. |
| B-05.18 | F | identity.md |  /  B-05.18 device number in file identity  /  Fixed and tested by rewriting the recorded numbers and rescanning; not exercised on a real NFS/SMB/FUSE remount (section 8).  /  |
| B-05.19 | A | decision-audit.md |  /  B-05.19  /  already fixed in source  /  Ordinary scans list file facts regardless of unchanged directory mtime; bounded ignore configuration is independently read.  /  |
| B-05.20 | F | scan-loop.md | - B-05.20 — fixed: bounded terminal work/history/change-feed/cache pruning and durable integrity summaries. |
| B-05.21 | F | scan-loop.md | - B-05.21 — fixed: selective queue rows; policy/operation/algorithm/head/STRM evidence invalidation retained. |
| B-05.22 | A | naming.md | - B-05.22 — already fixed for local sidecar lookups on the base according to the fix log; unchanged-sidecar invalidation belongs to scanning. |
| B-05.23 | F | scan-loop.md | - B-05.23 — fixed: default monitoring, startup scan and periodic fallback; owner may disable monitoring. |
| B-05.24 | F | identity.md |  /  B-05.24 deleted files stay available for a day, never removed  /  Fixed.  /  |
| B-05.25 | F | review-identity.md |  /  B-05.25  /  Old state-folder copy mechanism removed; disk unlink now occurs. Physical deletion has R2, R3 and R8.  /  |
| B-05.26 | F | scan-loop.md | - B-05.26 — fixed: persistent naming attention scoped through the affected source’s current inventory. |
| B-05.27 | F | identity.md |  /  B-05.27 show identity is title and year  /  Fixed.  /  |
| B-05.28 | A | decision-audit.md |  /  B-05.28  /  already fixed in source  /  naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-05.29 | F | scan-loop.md | - B-05.29 — predecessor local-metadata tolerance retained; scanner now notices sidecar changes/removal while preserving failed observations. |
| B-05.30 | A | naming.md | - B-05.30 — already fixed for administrator folder notices on the base according to the fix log; Jobs projection is outside this lane. |
| B-05.31 | A | leftovers-server-a.md |  /  B-05.31  /  Already fixed: commitAudio calls naming.DiscFolder, recognizes labeled disc folders, and release/tag grouping joins equivalent albums across folders. Existing grouping regression passes.  /  |
| B-05.32 | F | review-scan.md | - B-05.32 — fixed audio remainder as S6: NFC/NFD tags share artist/album identity. Name-reader Unicode controls pass. |
| B-05.33 | F | scan-loop.md | - B-05.33 — fixed: directory aliases are detected before visited-identity updates and protect their old subtree; two-scan regression passes. |
| B-06.1 | A | decision-audit.md |  /  B-06.1  /  already fixed in source  /  metadata provider scoring/publish/freshness/attention and local-artwork quota; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-06.2 | F | providers-matching.md | - B-06.2: fixed; confident yearless/fuzzy matching and paged needs-attention. |
| B-06.3 | A | decision-audit.md |  /  B-06.3  /  already fixed in source  /  metadata provider scoring/publish/freshness/attention and local-artwork quota; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-06.4 | F | providers-matching.md | - B-06.4 / B-10.2: fixed intake quota; configurable 2 GiB default, cached byte accounting, visible capacity log, raise-and-retry test. Automatic local-original pruning is not added. |
| B-06.5 | A | scan-loop.md | - B-06.5 — predecessor season artwork routing retained; changed/removed sidecars are reobserved. |
| B-06.6 | A | scan-loop.md | - B-06.6 — already fixed by providers-matching. |
| B-06.7 | F | providers-matching.md | - B-06.7: implemented TMDB show alternatives/logos/seasons/portraits and TVDB film artworks; show-artwork locale/season/portrait fixture passed; live-provider acceptance remains outstanding. |
| B-06.8 | F | providers-matching.md | - B-06.8: fixed by appending and parsing series aggregate credits; fixture passed; current official docs settle the static scope question. |
| B-06.9 | F | providers-matching.md | - B-06.9: fixed stale locale reuse through candidate locale/region predicates and freshness variants; known field locales are retained and unreported fallback locales are explicitly unknown. |
| B-06.10 | A | scan-loop.md | - B-06.10 — already fixed by providers-matching. |
| B-06.11 | A | scan-loop.md | - B-06.11 — provider rates/concurrency already fixed; scanner pacing/selective work completed. No live first-import timing claim. |
| B-06.12 | A | scan-loop.md | - B-06.12 — already fixed provider recovery retained. |
| B-06.13 | A | scan-loop.md | - B-06.13 — already fixed owner-search priority retained. |
| B-06.14 | A | scan-loop.md | - B-06.14 — already fixed provider region default retained. |
| B-06.15 | F | providers-matching.md | - B-06.15: fixed unlocked field ownership and complete observed NFO evidence retirement, with regression-tested owner relationship lock preservation. Unchanged-file sidecar detection remains a scanner integration dependency. |
| B-06.16 | A | scan-loop.md | - B-06.16 — predecessor tolerant NFO parser retained; no safety bounds removed. |
| B-06.17 | A | scan-loop.md | - B-06.17 — already fixed optional provider-field isolation retained. |
| B-06.18 | A | scan-loop.md | - B-06.18 — already fixed diversity ordering retained. |
| B-06.19 | A | scan-loop.md | - B-06.19 — already fixed artwork language ordering retained. |
| B-06.20 | A | leftovers-server-a.md |  /  B-06.20  /  Already fixed: TMDB portrait acquisition uses h632 rather than original and automatic retained source variants are shared; TestRetainedSourceArtworkIsAcquiredOnlyOnce passes. Explicit owner refresh still fetches current content.  /  |
| B-06.21 | F | leftovers-server-a.md |  /  B-06.21  /  Fixed: official/alternate-group absolute mapping plus signed explicit offsets, owner selection validation, original-coordinate provenance, local-number preservation and per-file needs-order isolation. Provider and full selection/publication regressions pass.  /  |
| B-06.22 | A | leftovers-server-a.md |  /  B-06.22  /  Already fixed: TMDB includeAdult defaults false and SetIncludeAdult is wired through the owner metadata settings.  /  |
| B-06.23 | A | leftovers-server-a.md |  /  B-06.23  /  Already fixed main bounded-cost findings: provider watcher is one second, artwork watcher two seconds, and artwork-history GC streams rows. No claim that image auth/selection queries are eliminated or measured at scale.  /  |
| B-06.24 | A | decision-audit.md |  /  B-06.24  /  already fixed in source  /  metadata provider scoring/publish/freshness/attention and local-artwork quota; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-07.1 | A | search-sort.md | - **B-07.1** — Already fixed in the base for steady state per fix log; rebuild/failure machinery belongs to freshness-server. |
| B-07.2 | F | freshness-server.md |  /  B-07.2  /  Fixed: serving browse generation and independent search generation; first build is readable as it progresses.  /  |
| B-07.3 | F | identity.md |  /  B-07.3 new file at a moved file's old path joins the moved title  /  Fixed.  /  |
| B-07.4 | F | freshness-server.md |  /  B-07.4  /  Fixed for this lane: order/semantic/observation counters, silent continuation/range recovery.  /  |
| B-07.5 | A | scale-check-2.md | - B-07.5 — already fixed on base: search revision reads its snapshot. |
| B-07.6 | F | search-sort.md | - **B-07.6** — Fixed: normalized punctuated abbreviations, prefix/component tokens and one-character searches. |
| B-07.7 | F | search-sort.md | - **B-07.7** — Fixed: indexed CJK unigrams/bigrams followed by actual substring verification, including single characters. |
| B-07.8 | A | scale-check-2.md | - B-07.8 — already fixed on base: date periods are compiled, not raw text comparisons. |
| B-07.9 | F | freshness-server.md |  /  B-07.9  /  Fixed: seek uses the server's Unicode-folded letter function, rather than the first UTF-8 byte.  /  |
| B-07.10 | O | web-viewer.md | - **B-07.10** (letter jumps for article-stripped titles): not touched. |
| B-07.11 | F | identity.md |  /  B-07.11 raw item delete route  /  Fixed: route removed.  /  |
| B-07.12 | A | scale-check-2.md | - B-07.12 — already fixed on base: text predicates use Unicode folding. |
| B-07.13 | F | leftovers-server-a.md |  /  B-07.13  /  Fixed budget/release gap: bounded snapshot context, allocated-WAL growth/disk-reserve checks and clean abort. Injected-pressure release and concurrent-write backup regressions pass; multi-gigabyte live pressure requires separate qualification.  /  |
| B-07.14 | F | leftovers-server-a.md |  /  B-07.14  /  Fixed: folded every post-baseline development migration into localized final CREATE definitions and deleted 0002–0011. No startup personal-history aggregation, facet backfill or anime rematch is shipped. Schema sync/checks pass.  /  |
| B-07.15 | F | freshness-server.md |  /  B-07.15  /  Fixed: bounded dictionary/identity/generation/event sweeps; FTS lifecycle follows document deletion. Identity already removes a title's ledger at final removal.  /  |
| B-07.16 | F | leftovers-server-a.md |  /  B-07.16  /  Fixed: watchdog checks actual heldSince in ordinary production; one log per stuck hold names class/entry point and health becomes degraded. No waiter/strict mode is needed; release recovers. Regression passes.  /  |
| B-07.17 | F | search-sort.md | - **B-07.17** — Fixed: title search uses the stored natural sort key; shows/books release-year sorts use their real year rather than a constant. |
| B-07.18 | A | leftovers-server-a.md |  /  B-07.18  /  Old 64-unit default is already mitigated: current compactcatalog.BatchLimit is 16, domains checkpoint/fan out boundedly and foreground pacing runs after release. No current excessive hold was established; representative weak-host measurement is prescribed below.  /  |
| B-07.19 | I | leftovers-server-a.md |  /  B-07.19  /  Deliberate owner choice, not a demonstrated defect: keep SQLite concurrency controls and add the quiet README note to inspect a backup copy. No external-program detection or blocking.  /  |
| B-08.1 | F | api-platform.md |  /  B-08.1 / B-15.8 web app refused behind TLS proxy  /  **Fixed by the networking lane** (now merged).  /  |
| B-08.2 | F | leftovers-server-b.md | - B-08.2 — Fixed. httpapi.failure defaults to logged 500 internal; caller-validation errors use requestinput, including wrapped errors, legacy JSON decoders and query/service validators. Internal stored-document, dependency and database faults remain server errors. The full per-route hostile-input battery and dedicated logging/redaction-to-response tests pass. |
| B-08.3 | F | api-platform.md |  /  B-08.3 reconnect demoted to long-poll  /  **Fixed** server side: the newcomer takes over. Client-core still treats a 409 as permanent, but the server no longer sends one.  /  |
| B-08.4 | F | api-platform.md |  /  B-08.4 new stream has no cursor  /  **Fixed**: a stream that opens with nothing waiting sends stream.opened carrying its position.  /  |
| B-08.5 | F | api-platform.md |  /  B-08.5 advertised 55 s, accepted 25 s  /  **Fixed**: both say 25 (server capability and client-core clamp).  /  |
| B-08.6 | F | freshness-server.md |  /  B-08.6  /  Fixed for private hints and idle feed work: audience-filtered wakes, coalescing, no idle SSE authentication queries or feed safety polls. Library hints still wake subscribers for live per-library permission checks. Capacity under 200 real devices remains unmeasured.  /  |
| B-08.7 | F | api-platform.md |  /  B-08.7 / B-15.3 limits shared behind a proxy  /  **Fixed by the networking lane** (now merged). The per-credential limits are keyed by credential, not address.  /  |
| B-08.8 | F | freshness-server.md |  /  B-08.8  /  Fixed: grid validators use resource catalogue/semantic/settings revisions and stable viewer identity, without token hash or process-wide write count.  /  |
| B-08.9 | N | leftovers-server-b.md | - B-08.9 — Not present as an identified production response defect. Guards retained per V01; required-list, nullable-pointer, omitted-list and oversized-response conformance fixtures added and pass. Root-null requests are now refused before registered and legacy handlers. Missing optional fixture dependencies were corrected rather than weakening guards. |
| B-08.10 | A | registry.md | - B-08.10: already fixed: shared API errors recognize the registry retry directive. Generated OpenAPI now includes a default envelope for retained handler errors as well. |
| B-08.11 | F | leftovers-server-b.md | - B-08.11 — Fixed in writable documentation. server/api/README.md now distinguishes optional toolkit packages from actual catalogue cursor, family-specific receipt and revision behavior. Current catalogue cursors already retain their traversal position across expiry/revisions and refresh totals (catalog/discovery.go); the original expired-cursor rejection is not present. External architecture wording still needs c… |
| B-08.12 | O | review-freshness.md |  /  B-08.12  /  Base retains gzip/asset/ETag fixes; generic registry post-composition 304 cost remains API-platform work.  /  |
| B-08.13 | F | api-platform.md |  /  B-08.13 registry access label not enforced  /  **Fixed**: a viewer_item route must name {itemId} and a viewer_library route {libraryId} or it cannot be registered, and the authorizer now checks the library.  /  |
| B-08.14 | A | server-docs.md | - B-08.14: already addressed by base title query; guide documents its revision-independent, 30-minute cursor and concurrent-walk limits. |
| B-08.15 | A | registry.md | - B-08.15: already fixed: original media MIME comes from the actual container in media_original.go. |
| B-08.16 | F | api-platform.md |  /  B-08.16 / B-15.15 public build fingerprint  /  **Fixed** (see decision 18).  /  |
| B-08.17 | F | freshness-clients.md |  /  B-08.17 (server report)  /  Client half done: one feed per device on the apps, shared between tabs on the web, and the receiver shares it.  /  |
| B-08.18 | F | api-platform.md |  /  B-08.18 slow uploads cut at 15 s  /  **Fixed by the networking lane** for the large JSON upload routes.  /  |
| B-09.1 | F | ops-packaging.md | - B-09.1: fixed further: detached restore receipt, copy byte progress, integrity phase, retained result; prior long integrity lane preserved. |
| B-09.2 | F | ops-packaging.md | - B-09.2: fixed: supervising parent restarts exit 75 on every target; owner Restart route added. |
| B-09.3 | A | ops-packaging.md | - B-09.3: already fixed: fresh install start and running-state-preserving upgrades verified in maintainer scripts. |
| B-09.4 | F | ops-packaging.md | - B-09.4: fixed through SCM replacement; NSIS compilation passed on Linux with a fixture payload. Windows runtime acceptance outstanding. |
| B-09.5 | F | ops-packaging.md | - B-09.5: fixed service account, scoped state ACL, firewall provisioning and explicit UNC/account guidance; Windows runtime acceptance outstanding. |
| B-09.6 | A | hosted.md | - **B-09.6** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-09.7 | F | networking.md | - B-09.7 — Fixed. Removed the hidden 256-socket ceiling; the owner can raise the ordinary ceiling, with 32 reserved connections and independent console/sign-in admission. |
| B-09.8 | F | networking.md | - B-09.8 — Fixed. Public, overlay and global-IPv6 peers can use HTTP; connectivity status uses the authoritative home list. |
| B-09.9 | F | ops-packaging.md | - B-09.9: fixed: saved conversion folder used at startup; requested/effective state and pending Restart projection wired. |
| B-09.10 | F | networking.md | - B-09.10 — Fixed by retiring the incompatible secure-connections choice. The programme accepts HTTP everywhere and never forces TLS. |
| B-09.11 | A | ops-packaging.md | - B-09.11: manifest/parser mismatch already fixed; default feed, periodic owner notice and clear-to-opt-out now wired. |
| B-09.12 | F | ops-packaging.md | - B-09.12: fixed: owner storage/certificate notices run on startup and every five minutes at maintenance priority. |
| B-09.13 | F | identity.md |  /  B-05.25 / B-09.13 delete copies into the state folder  /  Fixed: the trash no longer exists.  /  |
| B-09.14 | A | hosted.md | - **B-09.14** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-09.15 | A | hosted.md | - **B-09.15** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-09.16 | A | hosted.md | - **B-09.16** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-09.17 | F | ops-packaging.md | - B-09.17: fixed scheduled failure owner alert, shared running job admission, Windows free-space check, manifest durability, manual retention and DST scheduling. Directory sync on Windows uses flushed files plus NTFS rename journaling; no unsupported directory flush call. |
| B-09.18 | F | ops-packaging.md | - B-09.18: fixed: separate backup path and explicit same-disk caveat in installation guidance/catalogue. |
| B-09.19 | F | accounts-security.md |  /  B-09.19  /  Fixed.  /  |
| B-09.20 | F | ops-packaging.md | - B-09.20: fixed: restore admission refuses staging when the single-instance lock is unavailable, instead of falsely restarting successfully, with restore_lock_unavailable and catalogue copy. |
| B-09.21 | F | networking.md | - B-09.21 — Fixed in the server default (:32500), including empty-host topology/discovery parsing. Packaged launchers still explicitly bind IPv4; exact changes are assigned to ops-packaging below. |
| B-09.22 | F | networking.md | - B-09.22 — Fixed. run waits for HTTP Shutdown and the Hosted shutdown notice before returning into deferred database/service cleanup. |
| B-09.23 | F | review-ops.md |  /  B-09.23  /  Fixed further: uploaded-client age retention wired (R9). Early/history/redaction/file retention checks pass. Source-wide log-level instrumentation remains each domain's responsibility.  /  |
| B-09.24 | F | networking.md | - B-09.24 — Fixed. One bounded parser consumes X-Forwarded-For or Forwarded only behind the owner-trusted peer; policy/status/rate-limit consumers agree. |
| B-09.25 | F | networking.md | - B-09.25 — Fixed in this lane: the obsolete proxy environment variable is no longer read, owner proxy settings accept addresses or ranges, and :port/localhost bind values work through net.Listen. |
| B-09.26 | F | ops-packaging.md | - B-09.26: fixed image driver/timezone/discovery configuration and build-stage-only Python; Docker/GPU runtime acceptance outstanding. |
| B-09.27 | A | hosted.md | - **B-09.27** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-09.28 | F | limits-settings.md | - B-09.28 — fixed: schedule windows/defaults/accepted tasks offer backups only, and the catalogue says so. Other work follows Background work speed or the explicit one-hour pause. |
| B-09.29 | F | limits-settings.md | - B-09.29 — fixed: both corrupt/unopenable database and earlier state-folder failures serve a plain recovery page plus failed readiness until the service is restarted or stopped. Cold-process checks passed. |
| B-09.30 | F | leftovers-server-b.md | - B-09.30 — Fixed. backup/copy.go bounds pinned snapshot lifetime and WAL growth; pressure and timeout tests pass, including immediate WAL truncation after abort. |
| B-09.31 | F | ops-packaging.md | - B-09.31: fixed through package/system-daemon installation and explicit uninstall; app opening does not restart or relocate the daemon. Console URL can be supplied when opening from a shell. |
| B-09.32 | F | ops-packaging.md | - B-09.32: fixed: recover-owner requires the instance lock before opening/migrating the database. |
| B-09.33 | F | review-networking.md | - B-09.33 — Mandatory HTTPS conflict removed. HTTP readiness route can use reserved paths; F1/F3 compromise transport availability generally. |
| B-09.35 | A | hosted.md | - **B-09.35** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-10.1 | F | music-audiobooks.md | - B-10.1 — fixed: time-remaining completion; resume survives both the threshold and ended. The earlier resume-only fix was preserved and completed. |
| B-10.2 | F | music-audiobooks.md | - B-10.2 — fixed: removed the permanent artwork store's global cap and directory-size scan; per-image safety limits remain. |
| B-10.3 | F | music-audiobooks.md | - B-10.3 — fixed: play-next inserts immediately after current under shuffle, including ordered client batches. |
| B-10.4 | F | music-audiobooks.md | - B-10.4 — fixed: sparse permutation spans preserve played/current/upcoming order through removal and moves. Stable entry IDs survive shuffled moves. |
| B-10.5 | A | music-audiobooks.md | - B-10.5 — already fixed: existing provider pause/cursor behavior retained and its targeted regression passed. |
| B-10.6 | F | music-audiobooks.md | - B-10.6 — fixed: bulk candidates require title/artist/duration identity, plus timing validation. |
| B-10.7 | A | audio-engines.md | - B-10.7 — already fixed on supplied base: timing-based lyric identity; preserved. |
| B-10.8 | A | audio-engines.md | - B-10.8 — already fixed on supplied base: tolerant bounded LRC; preserved. |
| B-10.9 | F | music-audiobooks.md | - B-10.9 — fixed: end discard is summed across all packets. Canned regression covers a split discard region. |
| B-10.10 | F | audio-engines.md | - B-10.10 — fixed in code: supplied server profile qualification plus web SBR/PS clock and raw AAC packet counting; Apple packet decoder implements the raw window. Real HE-AAC conformance remains pending. |
| B-10.11 | O | music-audiobooks.md | - B-10.11 — not fixed: a bitrate-limited Apple audio renderer still needs a supported compressed conversion target. The misleading reason is corrected; implementing and qualifying AAC/MP4 across conversion and the native renderer belongs to the playback/native lanes. |
| B-10.12 | F | music-audiobooks.md | - B-10.12 — fixed: foreground starts enqueue persistent facts; requested files run first and are not discarded at a five-second playback budget. Facts still use a full background decode; a general packets-first/header measurement optimization is not implemented. |
| B-10.13 | F | music-audiobooks.md | - B-10.13 — fixed for tested conversions: playback priority and bounded input seeking with residual sample trimming. FLAC/MP3 samples were identical; AAC was aligned and differed by at most one 16-bit unit of decoder rounding. Other codecs need qualification below. |
| B-10.14 | F | music-audiobooks.md | - B-10.14 — fixed: narrator/composer fallback and plural narrator/composer values. |
| B-10.15 | F | music-audiobooks.md | - B-10.15 — fixed: prior part-number parsing retained; unnumbered files have natural filename fallback in selection, queues, paging and chapter listing. |
| B-10.16 | A | decision-audit.md |  /  B-10.16  /  already fixed in source  /  audiotags/catalog audio identity/listeningrules/lyrics and playbackv1 shuffled queues; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-10.17 | A | decision-audit.md |  /  B-10.17  /  already fixed in source  /  audiotags/catalog audio identity/listeningrules/lyrics and playbackv1 shuffled queues; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-10.18 | F | music-audiobooks.md | - B-10.18 — fixed: own APEv2 reader sees MP3 ReplayGain; ID3 RVA2 and R128 forms are also read. |
| B-10.19 | F | audio-engines.md | - B-10.19 — fixed for remaining WEBP support. Existing case/name/parent/front-cover fixes are preserved. Byte/dimension limits remain intentional. |
| B-10.20 | F | audio-engines.md | - B-10.20 — fixed: shared filter history, phase and cumulative output rounding across compatible committed joins; cancellation/deadline regressions pass. |
| B-10.21 | F | audio-engines.md | - B-10.21 — fixed in code: bounded real decoder probes admit Gecko/Vorbis when output agrees. Actual Firefox conformance is not executed here; unsupported configurations retain fallback. |
| B-10.22 | F | audio-engines.md | - B-10.22 — fixed: Android PCM amplification, peak limits and ramp; isolated real-Media3 JVM test passed. |
| B-10.23 | F | audio-engines.md | - B-10.23 — fixed in code: supplied server padding plus web raw AAC counts and Apple raw packet trimming/seek. Apple LC/HE-AAC/edit-list acceptance is still required. |
| B-10.24 | F | music-audiobooks.md | - B-10.24 — fixed: bitrate derives from audio packets/stream facts rather than embedded cover bytes. |
| B-10.25 | F | music-audiobooks.md | - B-10.25 — fixed: seeded, bounded library candidate walk starts at varying pivots and wraps; normalized genre relations replace raw tag equality. Candidate budget remains 640. |
| B-10.26 | F | music-audiobooks.md | - B-10.26 — fixed: missing disc sorts as disc one in queue/selection/paging and owned listening indexes. |
| B-10.27 | F | audio-engines.md | - B-10.27 — fixed policy: unity when gain is absent, no unsafe boost without a peak, album-to-track fallback. Analysis coverage can still vary; equal perceived loudness cannot be promised without measurements. |
| B-10.28 | O | music-audiobooks.md | - B-10.28 — not fully fixed: added hostile-input/container/chapter/seek/profile/padding regressions and fuzzing; the full device gapless fixture matrix is not qualified. |
| B-10.29 | F | music-audiobooks.md | - B-10.29 — fixed for the formats named in the finding: audio intake now also admits AIF, OGA, MKA, WMA, APE, WV, TTA, CAF, W64, AMR, DSF, DFF, MPC and MP4; closed decoder format lists were aligned. Encrypted/unknown formats and generic unsupported-file notices are not new features here. |
| B-10.30 | F | audio-engines.md | - B-10.30 — fixed: bounded repeated-tag/padding synchronization, regression passed; optional decode errors retain ordinary fallback. |
| B-10.31 | N | audio-engines.md | - B-10.31 — not present as prohibited behavior under the programme: owner TXT lyric sidecars are deliberate; supplied rip-log exclusions preserved. |
| B-10.32 | F | audio-engines.md | - B-10.32 — fixed: author/narrator/book/chapter Media Session projection, with book chapter paging for deep positions. |
| B-10.33 | F | music-audiobooks.md | - B-10.33 — fixed: Previous wraps under repeat-all; repeat laps retain one permutation and order history. |
| B-10.34 | F | audio-engines.md | - B-10.34 — fixed for engine-owned fallback: supplied shared foreground fallback retained; native owners now fall back after the first reported optional-renderer error. Ordinary-player conversion escalation remains with playback owners. |
| B-11.1 | A | decision-audit.md |  /  B-11.1  /  already fixed in source  /  linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-11.2 | A | decision-audit.md |  /  B-11.2  /  already fixed in source  /  linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-11.3 | F | livetv.md | - B-11.3: Fixed common XMLTV short dates, missing UTC offset, inferred stops and supported legacy text encodings. |
| B-11.4 | F | livetv.md | - B-11.4: Fixed tolerant programme rows, overlap trimming, consistent new/repeat facts and safe unknown-element skipping. |
| B-11.5 | F | livetv.md | - B-11.5: Fixed raised adjustable limits, streamed guide fetch/parsing, bounded spool and chunked publication. |
| B-11.6 | F | livetv.md | - B-11.6: Fixed gzip by magic bytes as well as content encoding, with compressed and expanded bounds. |
| B-11.7 | F | livetv.md | - B-11.7: Fixed per-row playlist tolerance, common EXTINF attributes, fallback IDs and duplicate variants. |
| B-11.8 | F | native-owner.md | - **B-11.8** (HDHomeRun add form hides the guide address): fixed on native. |
| B-11.9 | F | livetv.md | - B-11.9: Fixed runtime wiring for stream buffer, retry window, user agent, filters, numbering, mapping, hidden channels and horizon. Standalone guide-source creation is not implemented; attachment can reference another configured source's guide. |
| B-11.10 | F | livetv.md | - B-11.10: Fixed distinct-channel reservation sweeps and soft padding yielding to programme cores; same-channel records do not compete for another tuner. |
| B-11.11 | F | livetv.md | - B-11.11: Fixed upstream retries and capture continuation within the planned window, with gaps recorded. |
| B-11.12 | A | leftovers-server-b.md | - B-11.12 — Already fixed in recordingmedia.Driver.Recover: retained captures resume while the original live window remains open, increment gap evidence, and publish explicitly partial recovery. Sealed objects are validated/published. No real-tuner restart acceptance was run. |
| B-11.13 | F | livetv.md | - B-11.13: Fixed explicit commitments following a unique nearby correction with matching programme facts; ambiguous/missing guide data retains the accepted window. |
| B-11.14 | F | livetv.md | - B-11.14: Fixed HLS master selection by highest bandwidth with bounded shared acquisition/cache; device/provider fixture acceptance remains external. |
| B-11.15 | F | livetv.md | - B-11.15: Fixed admission through the advertised HLS transport tuple instead of codec-name whitelists, including HEVC/H.264/MPEG-2 and AC-3/E-AC-3/multichannel AAC when declared supported. Native Android/Fire TV acceptance remains external. |
| B-11.16 | F | livetv.md | - B-11.16: Fixed unknown live frame-rate/bitrate facts no longer forcing conversion when the declared tuple is compatible and no requested quality ceiling requires those facts. |
| B-11.17 | A | livetv.md | - B-11.17: Already fixed healthy recovery reset on the base; preserved. |
| B-11.18 | F | livetv.md | - B-11.18: Fixed segment publication retrying transient authority/database failures for up to three seconds; explicit policy refusal remains immediate. |
| B-11.19 | F | livetv.md | - B-11.19: Fixed dropping unsupported optional/low-latency and unknown URI-bearing tags; supported segment/key/map URLs remain validated and privately rewritten. |
| B-11.20 | F | livetv.md | - B-11.20: Fixed completeness against the programme core, rather than mandatory full padding. |
| B-11.21 | A | leftovers-server-b.md | - B-11.21 — Already fixed in recordingmedia/storage.go: traversal is under measureMu, not the capture writeMu; counters are reconciled briefly and concurrent writes counted conservatively. Hard-linked recording objects count once. Disk latency/drop acceptance remains a device test. |
| B-11.22 | A | livetv.md | - B-11.22: Already fixed retention-before-low-space refusal on the base; preserved. |
| B-11.23 | F | livetv.md | - B-11.23: Fixed removing an unused transport hash during validation. |
| B-11.24 | F | livetv.md | - B-11.24: Fixed dd_progid series recognition only for reliable EP identifiers; movie/special/generic ending-0000 identifiers do not create false series. |
| B-11.25 | F | livetv.md | - B-11.25: Fixed reliable episode deduplication using external episode identity or series+season+number. Unknown identities remain distinct. |
| B-11.26 | F | livetv.md | - B-11.26: Fixed the normal two-profile capture/storage path and last-owner physical deletion. Active leader cancellation does not yet transfer capture leadership seamlessly; a follower falls back to a new capture. |
| B-11.27 | O | livetv.md | - B-11.27: Backend fixed: session responses expose reservationRequiredAtMs and profile-scoped channel.reservation-required events carry it for every affected shared-channel viewer. Duplicate retries do not repeat events. Client countdown rendering remains to be wired. |
| B-11.28 | F | livetv.md | - B-11.28: Fixed discovered tuner descriptor reads and configured-device detection by stored locator host. |
| B-11.29 | F | livetv.md | - B-11.29: Fixed owner global hidden/number overlays in directory, guide and input admission; distinguish global vs personal hide in client screens. |
| B-11.30 | F | livetv.md | - B-11.30: Fixed programme rating gates at start, monitoring and media access, applying profile restrictions and the shared member rating ladder. Missing/unknown guide ratings follow the unrated policy. A narrow access.Enforcer helper was added to reuse account policy rather than duplicating it. |
| B-11.31 | N | livetv.md | - B-11.31: Not present in current baseline: directory candidates already use SQL paging. Preserved it; owner overlays are joined into the query. |
| B-11.32 | F | livetv.md | - B-11.32: Fixed superseded generation cleanup grace reduced from 24 hours to five minutes, with new per-generation tables pruned. |
| B-11.33 | F | livetv.md | - B-11.33: Fixed two-minute refresh lease renewal every 20 seconds, with bounded gated writes and cancellation on lost fencing. |
| B-11.34 | F | livetv.md | - B-11.34: Fixed time ruler alignment in quarter-hour-offset zones; Kathmandu/Chatham regression fixtures pass. |
| B-11.35 | I | review-server-late.md | - B-11.35 — Deliberate tuning choice: retain bounded timeshift defaults; not a new defect. |
| B-12.1 | F | personal-state.md | - B-12.1 — fixed: container marks supersede earlier member intent, clear member/book progress and fence earlier playback. |
| B-12.2 | F | personal-state.md | - B-12.2 — fixed on server: show-page continuation ignores Home age/dismissal rules and keeps meaningful rewatch progress. Client pages must use the supplied nextUp rather than an early-page guess. |
| B-12.3 | F | downloads.md |  /  B-12.3  /  Fixed (an explicit mark newer than the observation wins; the watched intent is dated to the play).  /  |
| B-12.4 | F | personal-state.md | - B-12.4 — fixed: an unwatched anchor itself is eligible for Next Up. |
| B-12.5 | F | downloads.md |  /  B-12.5  /  Fixed.  /  |
| B-12.6 | F | personal-state.md | - B-12.6 — fixed: member writes queue existing recommendation work, and RecSignalFor reads effective watched state. Season mark/Undo signal regression passes. |
| B-12.7 | F | personal-state.md | - B-12.7 — shared writer classifies completion on server, uses fixed millisecond activity/history stamps and stamps watched intent with occurrence time. Old downloads direct writes still need removal by downloads. |
| B-12.8 | F | leftovers-server-b.md | - B-12.8 — Fixed. Early known-duration EOF retains resume below completion policy; natural end, approved credits and percentage completion remain usable. TestEarlyEndKeepsResumeForKnownDuration passes. |
| B-12.9 | F | leftovers-server-b.md | - B-12.9 — Fixed. Credits mode falls back to percentage without an approved current marker; marker/no-marker/unapproved fixtures pass. server/api/home.md updated. |
| B-12.10 | A | review-server-late.md | - B-12.10 — Already fixed in source: effective viewer/library completion policy reaches the writer; native display acceptance remains external. |
| B-12.11 | F | personal-state.md | - B-12.11 — fixed: started percentage applies to long as well as short known-duration video. |
| B-12.12 | F | personal-state.md | - B-12.12 — fixed: unchanged paused reports and below-started replays do not move personal recency/anchor. History still receives accepted reports. |
| B-12.13 | F | personal-state.md | - B-12.13 — fixed: unknown-duration continuation and session resume no longer collapse to zero because of the missing duration; qualified terminal evidence can finish. |
| B-12.14 | F | personal-state.md | - B-12.14 — fixed: noncanonical completion may update watched/progress, while explicit fences and newer personal evidence remain authoritative. |
| B-12.15 | F | personal-state.md | - B-12.15 — fixed: premiere exemption needs a recent addition and recognizes the first available episode, not only episode number one. |
| B-12.16 | F | personal-state.md | - B-12.16 — fixed: local server date for recommendation rotation/fingerprints. |
| B-12.17 | A | personal-state.md | - B-12.17 — already fixed by web-app lane according to its report; server search-history preference enforcement already exists. Search text and screens were not changed here. |
| B-12.18 | F | identity.md |  /  B-12.18 state lost on rename or replace  /  Fixed.  /  |
| B-12.19 | F | identity.md |  /  B-12.19 show's Next Up anchor dangles  /  Fixed for removal: the marker moves to the nearest remaining episode.  /  |
| B-12.20 | F | freshness-server.md |  /  B-12.20  /  Fixed: pure observations preserve Home/command revisions and do not create personal snapshots. Progress itself remains current.  /  |
| B-12.21 | F | personal-state.md | - B-12.21 — fixed by choosing a three-profile minimum for public popularity. |
| B-12.22 | A | leftovers-server-b.md | - B-12.22 — Already fixed in catalog/personal_activity.go and play_history.go: shared counts require unpaused history and the activity opt-in; owner history and personal counts remain separate. Opt-out/history-pause focused tests pass. |
| B-12.23 | F | personal-state.md | - B-12.23 — fixed: continuation bounds use linked episode start/end, not whole-file duration. |
| B-12.24 | F | personal-state.md | - B-12.24 — fixed: merged maximum uses the largest participating library cap rather than the smallest. |
| B-12.25 | F | personal-state.md | - B-12.25 — fixed: explicit resume upserts activity for a never-played title without fabricating History or last-played activity. |
| B-12.26 | F | personal-state.md | - B-12.26 — fixed: separate short-lived Undo restores the previous completion mix and positions; unwatched remains an explicit reset. |
| B-13.1 | F | social.md | - **B-13.1 — fixed.** Playback/settings/governance/queue fences separated; routine member telemetry does not invalidate commands. Focused HTTP interleaving tests pass. |
| B-13.2 | A | social.md | - **B-13.2 — already fixed on base, extended here.** Existing seek settling/in-flight protection retained; current-revision bounded readiness waits added. Slow-seek follower tests pass. |
| B-13.3 | A | leftovers-server-b.md | - B-13.3 — Already fixed in shared GroupFollower, independently verified and extended with native-style simulations at 1.2, 2, 5, 12 and 19 seconds of seek latency and a no-op rate setter. All converge within the 750-ms band, issue at most three seeks over 120 seconds and never supersede a pending seek. Existing failed-seek, unknown-seek-state, host-clock-change and end-room tests also pass. Players lacking comple… |
| B-13.4 | F | social.md | - **B-13.4 — fixed.** Extrapolate elapsed playback before applying the new rate. Rate components and positions are bounded to avoid arithmetic overflow. |
| B-13.5 | F | social.md | - **B-13.5 — fixed.** Presence and readiness timestamps separated; stale non-host membership retired; disconnected members do not hold readiness/eligibility. |
| B-13.6 | F | native-sync.md | - B-13.6, B-13.8, B-13.9(a) — client side done on native (claim deadline, private members, hidden title). |
| B-13.7 | F | native-sync.md | - B-13.7 — native persistence done (controller storage, adoption); unverified on a device. |
| B-13.8 | F | native-sync.md | - B-13.6, B-13.8, B-13.9(a) — client side done on native (claim deadline, private members, hidden title). |
| B-13.9 | F | native-sync.md | - B-13.6, B-13.8, B-13.9(a) — client side done on native (claim deadline, private members, hidden title). |
| B-13.10 | O | social.md | - **B-13.10 — improved/fixed hot paths.** Non-host queue projections check only that viewer; readiness frames update client state without full group/queue rereads. Item visibility is memoized within event batches. No ceiling-scale performance benchmark was run. |
| B-13.11 | F | social.md | - **B-13.11 — fixed.** Receiver approvals/heartbeats/readiness bound to the actual authenticated device; session scope, device, item, state and lease checked; commit rechecks destination and source item/liveness. Request device claims removed from the wire. Fingerprint omission is refused. |
| B-13.12 | F | social.md | - **B-13.12 — fixed.** Active playback no longer suppresses inbox polls/hints. Taking one handoff does not block the heartbeat or grant inbox; overlapping inbox reads are single-flight. |
| B-13.13 | F | social.md | - **B-13.13 — fixed for owned paths.** Social creation/command/telemetry/receiver/handoff budgets and resource caps added. Existing global admission and Cast budgets retained. |
| B-13.14 | F | accounts-security.md |  /  B-13.14  /  Fixed.  /  |
| B-13.15 | F | social.md | - **B-13.15 — fixed.** Standard CAF LOAD/PLAY/SEEK require the controller, including after departure. Local-room pause/stop concession retained. |
| B-13.16 | F | social.md | - **B-13.16 — fixed.** Refused/disconnected/replaced pending takeover revokes the unused pairing and retires its directory entry. Cleanup carries the generation in If-Match, so delayed cleanup cannot revoke its replacement. Network-unreachable cleanup is best effort and still needs physical recovery acceptance. |
| B-13.17 | F | social.md | - **B-13.17 — (b/d) already fixed; remaining owned cases fixed.** Pre-play reconnect phases preserved, seek/title changes wait, anyone-control activity is not re-paused solely because the host is absent. Empty pause stays a lobby. |
| B-13.18 | F | native-screens.md | Unnumbered Outcome / Decisions / Settings evidence; recorded in reconciliation. |
| B-13.19 | F | social.md | - **B-13.19 — fixed in shared/server coordination.** Follower clamps to known local duration and handles an ended player as ready; fenced host completion pauses the shared clock once. No guessed global duration or new automatic-next feature was added. Client adapters must continue exposing duration after end. |
| B-13.20 | F | social.md | - **B-13.20 — fixed.** Lower revision/playback revision/event ordinal snapshots cannot overwrite newer state; transport receipts and event state carry the playback revision. |
| B-13.21 | F | accounts-security.md |  /  B-13.21 (Q-5)  /  Fixed.  /  |
| B-13.22 | F | leftovers-server-b.md | - B-13.22 — Fixed for large fan-out in operations.BroadcastNotice: paged, fenced and resumable, with a 522-profile interruption/replay test. The Hosted-audience subclaim is not present in current admission: identity.PorticoSignInTx resolves a local membership and calls directSignInTx, using local profile inboxes. Dormant Hosted-authority device helpers do not establish a current Hosted-issued viewer path. |
| B-13.23 | F | social.md | - **B-13.23 — fixed.** Group/member text rejects control characters and direction overrides; direct text validation rejects invalid UTF-8, and existing length bounds are retained. |
| B-14.1 | F | downloads.md |  /  B-14.1  /  Fixed (server receipts.go Revalidate, worker.go tidy; device removes only on a real refusal).  /  |
| B-14.2 | A | apple-native-review.md | - B-14.2 — Already fixed for unchanged file/grant on supplied source; extended here to preserve Wi-Fi policy. Address or policy changes still restart iOS partials. |
| B-14.3 | F | downloads.md |  /  B-14.3  /  Fixed (renewable 24 h address; the app renews hourly while a transfer runs and on every attempt).  /  |
| B-14.4 | F | downloads.md |  /  B-14.4  /  Fixed (menu, claims and Optimize list ask the encoder's own argument builder).  /  |
| B-14.5 | F | downloads.md |  /  B-14.5  /  Fixed (no copy, one read, shared digest, bounded windows; nothing holds the worker for a whole file).  /  |
| B-14.6 | F | downloads.md |  /  B-14.6  /  Fixed (receipt taken before the first byte; stored with the entry at start).  /  |
| B-14.7 | F | downloads.md |  /  B-14.7  /  Fixed (title on every claim; web and native lists use it; browser saves "Film (2000).mp4").  /  |
| B-14.8 | F | downloads.md |  /  B-14.8  /  Fixed (server lists a device's requests; the shared service finds them again on start).  /  |
| B-14.9 | F | downloads.md |  /  B-14.9  /  Fixed (daily renewal, one manifest write per batch; the provider was already keyed per viewer by the fix pass).  /  |
| B-14.10 | F | downloads.md |  /  B-14.10  /  Fixed (shared writer; history, counts, thresholds, credits, explicit-mark ordering, position cleared on completion, songs keep no resume point, writer ownership).  /  |
| B-14.11 | A | downloads.md |  /  B-14.11  /  Already fixed; behaviour kept and re-expressed through the shared writer (book tests pass, one assertion updated: the pointer is now held by the offline play rather than by nobody).  /  |
| B-14.12 | F | native-playback.md |  /  B-14.12  /  Fixed for playing: a download the platform player cannot open goes to mpv. No note in the download menu (not the player).  /  |
| B-14.13 | F | downloads.md |  /  B-14.13  /  Fixed (progress no longer moves the revision; the shared client re-reads and re-sends once on a conflict).  /  |
| B-14.14 | O | decision-audit.md |  /  B-14.14  /  already fixed in source  /  downloads claim/receipt/bundle paths; native offline-downloads storage/player paths; relevant observable decision paths are mapped in the two matrices above.  /  |
| B-14.15 | F | downloads.md |  /  B-14.15  /  Fixed (a ready claim whose converted copy is gone becomes unavailable, at tidy and at grant/transfer time; copies on devices are untouched).  /  |
| B-14.16 | F | downloads.md |  /  B-14.16  /  Fixed (unknown receipt keeps the copy and asks for a new receipt; unknown claim re-claims and adopts it when the digest matches).  /  |
| B-14.17 | F | downloads.md |  /  B-14.17  /  Fixed (decision 10).  /  |
| B-14.18 | F | downloads.md |  /  B-14.18  /  Fixed in code (the arrived file is kept in the download's folder and verified at next launch); needs the device test in section 8.  /  |
| B-14.19 | F | downloads.md |  /  B-14.19  /  Fixed in code (launch reconciliation of entries against live tasks; Pause works with no task).  /  |
| B-14.20 | O | leftovers-server-b.md | - B-14.20 — Not finished in the native platform. Durable server requests and DownloadsService.followDevice already recover claims on the next app run; the downloads lane reports Wi-Fi/retry/service-timeout improvements. Automatic recovery while the Android process is dead still needs an installation-owned platform job. No native files are writable here. |
| B-14.21 | O | leftovers-server-b.md | - B-14.21 — Partly fixed. Server future timestamps are clamped before comparison/history/watermark writes; a regression proves they cannot replace later online resume. Device receipt expiry still uses its wall clock through receiptUsable; monotonic persisted receipt-clock integration is not finished outside this server worktree. |
| B-14.22 | F | downloads.md |  /  B-14.22  /  Fixed (renewal carries the server's resume point; progress results set it).  /  |
| B-14.23 | F | downloads.md |  /  B-14.23  /  Fixed (asking again restarts a failed claim; "already in your downloads" when nothing new).  /  |
| B-14.24 | F | downloads.md |  /  B-14.24  /  Fixed (hourly bounded pruning of ended claims, finished requests, dead receipts, orphaned marks and digests).  /  |
| B-14.25 | F | downloads.md |  /  B-14.25  /  Fixed (housekeeping at most once a minute, reads first, no write when nothing is due).  /  |
| B-14.26 | F | downloads.md |  /  B-14.26  /  Fixed (versions download; parts refuse with several_files).  /  |
| B-14.27 | F | downloads.md |  /  B-14.27  /  (a) fixed; (b) the device withdraws its receipt on removal, the claim is left (decision 1); (c) fixed, reasons are a closed set; (d) fixed, a retried ended claim steps aside for a newer live one; (e) no longer applies (no first-use window); (f) Android still refuses redirects, deliberately.  /  |
| B-15.1 | F | accounts-security.md |  /  B-01.2  /  Fixed. Same as B-15.1.  /  |
| B-15.2 | A | accounts-security.md |  /  B-15.2  /  Already fixed on the base branch.  /  |
| B-15.3 | F | api-platform.md |  /  B-08.7 / B-15.3 limits shared behind a proxy  /  **Fixed by the networking lane** (now merged). The per-credential limits are keyed by credential, not address.  /  |
| B-15.4 | F | networking.md | - B-15.4 — Fixed for ordinary cross-site browser setup, including the separate /v1/setup entry point: JSON plus matching browser origin, and cross-site preflight refusal. Open remote first-owner setup is deliberate. DNS rebinding that is already same-origin remains an unverified residual risk; a hostname allow-list is expressly excluded. |
| B-15.5 | F | accounts-security.md |  /  B-15.5  /  Fixed as far as the trade-off allows: backups hold no sessions and a restore signs everyone out. Password hashes and second-factor secrets remain in the clear by decision.  /  |
| B-15.6 | F | networking.md | - B-15.6 — Fixed. One parser, a right-to-left trusted chain, one owner trust list; malformed chains are remote and rate-limited under the socket peer. |
| B-15.7 | F | leftovers-server-b.md | - B-15.7 — Documentation error; writable identity guide now explicitly says bearer credentials have installation/family association, not private-key proof. The external architecture's “registers a key” promise needs correction. Accepted HTTP policy retained. |
| B-15.8 | F | api-platform.md |  /  B-08.1 / B-15.8 web app refused behind TLS proxy  /  **Fixed by the networking lane** (now merged).  /  |
| B-15.9 | I | hosted.md | - **B-15.9** — Not changed: deliberate no-lockout policy; do not introduce it. Existing Hosted abuse budgets and password backoff remain. |
| B-15.10 | A | leftovers-server-b.md | - B-15.10 — Already fixed in identity.takeHashSlot: three-second bounded, cancellable wait, no writer held during hashing/admission. Existing focused admission/password/PIN tests are included in final verification. |
| B-15.11 | F | networking.md | - B-15.11 — Fixed for configurable locality and observed-address reporting. A transport that hides the real address cannot reveal it: trust a forwarding HTTP proxy, remove the gateway range, or set home networks to []. No proxy/container detection was added. |
| B-15.12 | F | review-accounts-security.md |  /  B-15.12  /  Fixed for learned-revocation persistence by R1; existing check-in certificate verification/revocation tests pass. Fleet revocation-delivery latency is not established here.  /  |
| B-15.13 | I | accounts-security.md |  /  B-15.13  /  Not changed. Deliberate (a TV code confers the approver's role).  /  |
| B-15.14 | A | leftovers-server-b.md | - B-15.14 — Already fixed transport: POST /v1/auth/remembered-accounts and IdentityClient.rememberedAccounts use a body rather than query string. Corrected the inaccurate 256-bit entropy claim in source comments/identity OpenAPI prose; grammar/length validation does not prove randomness. Descriptor lookup remains intentionally unauthenticated and reveals no credentials. |
| B-15.15 | F | accounts-security.md |  /  B-15.15  /  Fixed.  /  |
| B-15.16 | F | accounts-security.md |  /  B-15.16  /  Fixed, untested against a real Cast sender; see section 8.  /  |
| B-15.17 | F | leftovers-server-b.md | - B-15.17 — Fixed. Release feed uses remotemedia.Policy/client with bounded time/body, pinned addresses, metadata-service blocking and origin-confined redirects. safeUpdateURL already restricts rendered release links to credential-free HTTPS. Private configured feed, metadata and redirect tests pass. |
| B-15.18 | F | accounts-security.md |  /  B-15.18  /  Fixed.  /  |
| B-15.19 | O | leftovers-server-b.md | - B-15.19 — Partly fixed. Remaining tagged checkout/setup-go uses are pinned; Docker base manifests are digest-pinned after hash verification against Docker Registry responses; pinned govulncheck and npm vulnerability jobs added. Release key generation was already forbidden by build-server-release.py. OS installer signing is still a separate credential/qualification step. Live npm scan found four high advisory pat… |
| B-15.20 | A | leftovers-server-b.md | - B-15.20 — Already fixed server/API: metadata/runtime_settings.go stores owner TMDB/TheTVDB overrides, returns presence only, and hot runtime settings select those credentials. A key can be replaced without a release. Console integration/visibility needs client verification; public application credentials remain the intentional default. |
| B-15.21 | F | accounts-security.md |  /  B-15.21  /  Fixed. Wrong PINs are also counted across devices.  /  |
| B-15.22 | I | review-networking.md | - B-15.22 — Superseded by deliberate universal HTTP; Hosted TLS-only promise remains incomplete in F6. |
| B-15.23 | F | accounts-security.md |  /  B-15.23  /  Fixed for asking for a TV code and for both invitation routes. A television polling for its answer is deliberately not counted.  /  |
| B-15.24 | F | accounts-security.md |  /  B-15.24  /  The download file name is fixed. The CSV formula case was already handled (csvCell).  /  |
| B-16.1 | A | leftovers-native.md | - **B-16.1** — Already fixed: web/src/app/browse.ts retains the window through usePooled; the old parent-dispose/child-save ownership no longer applies. Browser Back acceptance remains device work. |
| B-16.2 | A | leftovers-native.md | - **B-16.2** — Already fixed: client-core viewer-selection uses conditional saved-family clearing; web selection storage coordinates shared writes. |
| B-16.3 | A | leftovers-native.md | - **B-16.3** — Already fixed: httpapi installation serves hashed assets immutably (also in FIX-LOG). |
| B-16.4 | O | leftovers-native.md | - **B-16.4** — Not fixed here: web/src/ui/WindowedGrid.tsx update still snaps at maxScrolled while small deltas move 1:1. Frontend owner must smooth the compressed end ranges. |
| B-16.5 | O | leftovers-native.md | - **B-16.5** — Already partly fixed: WindowedGrid measurement subscription is stable; Library.tsx EngineResults inline cell renderers still need frontend memoisation. |
| B-16.6 | A | leftovers-native.md | - **B-16.6** — Already fixed in the maintained locale helpers/reactive web catalogue; no new locale policy introduced. |
| B-16.7 | O | leftovers-native.md | - **B-16.7** — Not fixed here: plain-HTTP visible tabs can still own separate event requests. Web player/bridge needs an IndexedDB-coordinated feed owner or an equivalent HTTP-capable shared transport; secure-only sharing is insufficient. |
| B-16.8 | A | leftovers-native.md | - **B-16.8** — Already fixed: client-core index.ts requestDeadlineMs/run deadline includes ordinary requests; streams retain their distinct lifetimes. |
| B-16.9 | A | leftovers-native.md | - **B-16.9** — Already fixed: lazy-screen failure recovery reloads the failed bundle (FIX-LOG and web retry path). |
| B-16.10 | O | leftovers-native.md | - **B-16.10** — Already partly fixed: home/library presentation parsing has tolerant row handling. Older-client/newer-server compatibility is not a pre-release obligation; retain strict identity/security validation. |
| B-16.11 | A | leftovers-native.md | - **B-16.11** — Already fixed: web shell Connection keeps the routed page underneath its reconnect cover. |
| B-16.12 | O | leftovers-native.md | - **B-16.12** — Not fully fixed: web card focus fallback exists, but web/vite.config.ts still has no build.target/browser floor. Frontend/packaging owner must declare the supported engines and test CSS features. |
| B-16.13 | A | leftovers-native.md | - **B-16.13** — Already fixed: signed-in web shell has the main target and skip link. |
| B-16.14 | O | leftovers-native.md | - **B-16.14** — Already partly fixed: route titles/announcements exist. The previously reported rapid-heading and identical-heading announcement cases need web navigation acceptance, outside native non-screen scope. |
| B-16.15 | A | leftovers-native.md | - **B-16.15** — Already fixed: web Card names viewing/progress state at its focus target. |
| B-16.16 | A | leftovers-native.md | - **B-16.16** — Already fixed: web WindowedGrid position semantics and focus fallback are present; screen-reader acceptance remains unrun. |
| B-16.17 | O | leftovers-native.md | - **B-16.17** — Already partly fixed: Field help association and named options are present. Pixel type sizes and forced-colour styling are frontend work in web/src/ui/Field.tsx, Tabs.tsx, tokens.css and base.css. |
| B-16.18 | F | e2e-web.md |  /  B-16.18: dialog scroll-lock stylesheet blocked by CSP  /  Fixed with a fresh per-document style nonce; trusted sheet accepted and arbitrary inline sheet refused in Chrome.  /  |
| B-16.19 | A | leftovers-native.md | - **B-16.19** — Already fixed: web browse.ts RevalidatingCache is viewer-scoped, bounded to 32 entries and rechecks each use. No server vocabulary revision exists. |
| B-16.20 | A | leftovers-native.md | - **B-16.20** — Already fixed: client-core collections/windowed.ts respects per-page server retry guidance. |
| B-16.21 | A | leftovers-native.md | - **B-16.21** — Already fixed: web/admin/cursor-read.ts resets ownership with its dependency identity. |
| B-16.22 | A | leftovers-native.md | - **B-16.22** — Already fixed: web/app/detail.ts credits paging has subject generation, per-flight ownership and surfaced error state. |
| B-16.23 | O | leftovers-native.md | - **B-16.23** — Not fixed here: web/package.json has no hook-rule enforcement. This is web tooling outside the permitted native/shared-code area; adding a new lint dependency also needs the programme dependency decision. |
| B-16.24 | A | leftovers-native.md | - **B-16.24** — Already fixed: production web route wiring gates developer routes; no production build was run to recount emitted chunks. |
| B-16.25 | O | leftovers-native.md | - **B-16.25** — Not fixed here: server Dockerfile uses Node 24 while server CI specifies 22.22.2. Packaging owner must align and pin both; this lane does not own server build wiring. |
| B-17.1 | F | playback-core.md |  /  B-02.6 = B-17.1  /  Server side and shared-client plumbing fixed (one PATCH; V1QueuePlayer.changePlayback). The player screens still have to call it: see section 7.  /  |
| B-17.2 | F | playback-core.md |  /  B-17.2  /  Shared-client part fixed (in-place change and recovery); the web player still has to stop restarting the queue (section 7).  /  |
| B-17.3 | F | web-player.md |  /  B-17.3  /  Fixed. Unsolicited pause/play is intent; Media Session for all titles.  /  |
| B-17.4 | F | web-player.md |  /  B-17.4  /  Fixed. Progress-based loading and recovery; recovery ends at the end of a title or once the picture moves.  /  |
| B-17.5 | F | web-player.md |  /  B-17.5  /  Fixed.  /  |
| B-17.6 | F | web-player.md |  /  B-17.6  /  Fixed as honest gating (AirPlay not offered for media-source streams). Needs Safari + Apple TV to confirm (section 8).  /  |
| B-17.7 | F | web-player.md |  /  B-17.7  /  Fixed (pagehide keepalive DELETE with position; flush on hidden).  /  |
| B-17.8 | F | web-player.md |  /  B-17.8  /  Fixed.  /  |
| B-17.9 | A | review-playback-core.md | - B-17.9 — Already fixed in FIX-LOG (891cdec); shortcut UI is outside this lane and was not re-tested here. |
| B-17.10 | F | web-player.md |  /  B-17.10  /  Fixed.  /  |
| B-17.11 | F | web-player.md |  /  B-17.11  /  Fixed.  /  |
| B-17.12 | F | web-player.md |  /  B-17.12  /  Fixed.  /  |
| B-17.13 | F | web-player.md |  /  B-17.13  /  Fixed (system full-screen fallback, plain notice where none). Needs an iPhone.  /  |
| B-17.14 | F | web-player.md |  /  B-17.14  /  Fixed by sizing buffers from the stream's bit rate (120 MB budget, 40 MB on ≤2 GB devices). Not measured.  /  |
| B-17.15 | F | web-player.md |  /  B-17.15  /  Fixed (moves serialised, overtaken sessions stopped, retry position bound to its title).  /  |
| B-17.16 | F | web-player.md |  /  B-17.16  /  Fixed (real rate in every report; cadence from the reporter).  /  |
| B-17.17 | F | audio-engines.md | - B-17.17 — fixed: FLAC index/bisection; unindexed lossy deep seeks immediately use ordinary playback. Approximate MP3 indexing is not advertised as sample exact. |
| B-17.18 | F | audio-engines.md | - B-17.18 — fixed in code: optional Safari playback session and interrupted-context handling; lock/silent-switch/call acceptance still needs iPhone/iPad Safari. |
| B-17.19 | O | review-playback-core.md | - B-17.19 — Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review. |
| B-17.20 | F | web-player.md |  /  B-17.20  /  Fixed.  /  |
| B-17.21 | F | web-player.md |  /  B-17.21  /  Fixed (cues lifted by line where the cue box cannot be moved with CSS). Needs Firefox.  /  |
| B-17.22 | F | web-player.md |  /  B-17.22  /  Fixed (the range input takes no pointer events).  /  |
| B-17.23 | F | web-player.md |  /  B-17.23  /  Fixed (no size capping; a multi-rendition manifest is pinned to its best level).  /  |
| B-17.24 | F | web-player.md |  /  B-17.24  /  Fixed (20 ms tolerance).  /  |
| B-17.25 | F | web-player.md |  /  B-17.25  /  Fixed: the shipped path is the tested one.  /  |
| B-17.26 | F | web-subtitles.md |  /  B-17.26, A-17.39  /  Audiobook speed to 3×: fixed (server, client-core, settings, web player).  /  |
| B-18.1 | A | leftovers-native.md | - **B-18.1** — Already fixed: web detail group selection carries the selected group id. |
| B-18.2 | A | leftovers-native.md | - **B-18.2** — Already fixed: MetadataEditor includes list values when determining changed locked fields. |
| B-18.3 | A | leftovers-native.md | - **B-18.3** — Already fixed: web Search uses committed searches and server history with preference-aware clear/read. |
| B-18.4 | F | web-viewer.md | - **B-18.4** (Genres and Series stop at 200): fixed on the web. |
| B-18.5 | A | leftovers-native.md | - **B-18.5** — Already fixed on this base: DELETE /v1/admin/libraries/{id} is registered; shared administration consumers exist. This lane added no library route. |
| B-18.6 | A | leftovers-native.md | - **B-18.6** — Already fixed for invalid menu offers: EntryActions uses refreshMetadataPath(kind,id). Broader container refresh implementation belongs to its server owner. |
| B-18.7 | A | leftovers-native.md | - **B-18.7** — Already fixed in shared detail-actions/web Detail action composition; no new screen work here. |
| B-18.8 | N | leftovers-native.md | - **B-18.8** — Not present for current Portico Account admission: it receives local server authority, as the second review established. Do not resurrect a legacy hosted-authority route. |
| B-18.9 | O | leftovers-native.md | - **B-18.9** — Not fixed here: selectable admin tier still differs from exact-owner web management gates. Accounts/server and web owners must expose the actual delegated permissions in web/src/app/session.tsx and screens/server/People.tsx. |
| B-18.10 | A | leftovers-native.md | - **B-18.10** — Already fixed: profile action model gates management while preserving permitted read-only limits. |
| B-18.11 | A | leftovers-native.md | - **B-18.11** — Already fixed: check-preference-consumers.mjs points to apps/native rather than retired apps/apple. |
| B-18.12 | A | leftovers-native.md | - **B-18.12** — Already fixed: maintained web screens use viewer catalogue helpers rather than fixed module-scope en-US. |
| B-18.13 | F | identity.md |  /  B-18.13 bulk delete without confirmation  /  Fixed by decision 5.  /  |
| B-18.14 | A | leftovers-native.md | - **B-18.14** — Already fixed: compact shell has the profile/general menu; Help remains reachable through About. |
| B-18.15 | A | leftovers-native.md | - **B-18.15** — Already fixed at the frontend level: Show offers the supplied cast beyond twelve. This is not proof of server cast paging. |
| B-18.16 | A | leftovers-native.md | - **B-18.16** — Already fixed: episode grid has the action alternative. |
| B-18.17 | A | leftovers-native.md | - **B-18.17** — Already fixed: season routing preserves an actual season identity rather than substituting a season number into an id path. |
| B-18.18 | A | leftovers-native.md | - **B-18.18** — Already fixed: web internal Back helper supplies a Portico fallback. |
| B-18.19 | A | leftovers-native.md | - **B-18.19** — Already fixed: class-scoped settings use class wording, distinct from installation-local values. |
| B-18.20 | A | leftovers-native.md | - **B-18.20** — Already fixed: bulk personal collection actions follow permission availability rather than exact owner alone. |
| B-18.21 | A | leftovers-native.md | - **B-18.21** — Already fixed: People role selection opens askRole confirmation before setRole. |
| B-18.22 | O | leftovers-native.md | - **B-18.22** — Already partly fixed: web catalogue coverage improved. Metadata fallback field labels and any remaining player copy belong to their frontend owners; no screen text invented here. |
| B-18.23 | O | leftovers-native.md | - **B-18.23** — Not fixed in the web consumer: library-content already carries discNumber, but Entity.tsx album grouping still calls discNumberFromSubtitle(entry.subtitle). Frontend must use entry.discNumber and remove that English parsing path. |
| B-18.24 | A | leftovers-native.md | - **B-18.24** — Already fixed: maintained date/language formatters use the profile catalogue/region. |
| B-18.25 | A | leftovers-native.md | - **B-18.25** — Already fixed: Saved media cards supply navigation addresses. |
| B-18.26 | A | leftovers-native.md | - **B-18.26** — Already fixed: ordinary web Search uses scoped memory rather than writing private query text to route search. |
| B-18.27 | O | leftovers-native.md | - **B-18.27** — Not fully fixed: dead catalogue cleanup exists, but pinned resource controls, provider API-key applicability, remembered-device handler and registry-only choices require their owning screens/server pieces. See exact handoffs below. |
| B-18.28 | A | leftovers-native.md | - **B-18.28** — Already fixed on the client: Home uses homeRowSeeAll and row.seeAll destinations. Server must author the correct destination for each row; a library fallback cannot represent arbitrary future subgroups. |
| B-18.29 | O | leftovers-native.md | - **B-18.29** — Not fixed here: web/screens/saved/Resource.tsx immediate entry removal still needs a visible Undo action and an occurrence-safe restore operation. |
| B-18.30 | O | leftovers-native.md | - **B-18.30** — Not fixed here: web/screens/detail/EpisodePanel.tsx needs server-backed air date on direct visits and navigable guest/director/writer entries. |
| B-19.1 | A | leftovers-native.md | - **B-19.1** — Already fixed: app-level link-intents owns initial/warm links above Shell; pending-link fences/coalesces them. Scene adoption here preserves cold URL delivery. |
| B-19.2 | A | leftovers-native.md | - **B-19.2** — Already fixed: app/viewer-scope.ts caches by viewer/server value, so token renewal preserves service scope. |
| B-19.3 | A | leftovers-native.md | - **B-19.3** — Already fixed: Android Menu emits its own intent, and Focusable/useMenuKey routes it only to a menu action; no synthetic Select remains. |
| B-19.4 | A | leftovers-native.md | - **B-19.4** — Already fixed: secure-records preparation/recovery distinguishes temporary failure and confirmed key loss; older Android/Fire uses explicit reset confirmation. |
| B-19.5 | A | leftovers-native.md | - **B-19.5** — Already fixed for failure/retry and unbounded shown-set: ArtworkPipeline bounds shown to 4000 and has reload backoff. Remaining retry-URL cache duplication is a low-cost performance followup; not claimed eliminated. |
| B-19.6 | A | leftovers-native.md | - **B-19.6** — Already fixed: index.js installs error-capture before App; bounded redacted error-journal persists a fatal record and preserves framework fatal handling. Five journal tests passed; injected device fatal timing is untested. |
| B-19.7 | A | leftovers-native.md | - **B-19.7** — Already fixed: iPad plist has all four orientations. Archive/upload validation remains unrun. |
| B-19.8 | O | leftovers-native.md | - **B-19.8** — Not fixed: TelevisionDisplay still relies on RN internal flags and process-level density; rotation/font-scale behaviour needs Android TV hardware. The detection change here also supplies the same TV hint to density setup, but does not claim to repair RN rotation metric overwrites. |
| B-19.9 | F | leftovers-native.md | - **B-19.9** — Fixed here: Keychain, profile PIN and record work run on PorticoRecordQueue; transport submission stays on the module queue. |
| B-19.10 | O | leftovers-native.md | - **B-19.10** — Partly fixed here: a Fire TV flavor or Leanback feature supplies a TV hint before the app loads, and TelevisionDisplay uses the same hint. Generic boxes that declare neither television UI mode nor Leanback cannot safely be inferred as TVs; an explicit layout choice remains product/frontend work. |
| B-19.11 | I | vega-transport.md | - **B-19.11 — Not changed, deliberate sandboxed files.** Programme policy does not require encryption at rest; no keychain is fabricated. |
| B-19.12 | F | leftovers-native.md | - **B-19.12** — Fixed here: a missing Apple installation marker clears only six exact Portico services, after protected data is available on iOS. Cleanup failures remain retryable; established installations retain background access. This is local reinstall cleanup, not server revocation on uninstall. |
| B-19.13 | A | leftovers-native.md | - **B-19.13** — Already fixed for row geometry: Card/Grid reserves measured caption geometry. Large-offset float precision remains a hardware/large-library acceptance item, not reproduced here. |
| B-19.14 | F | leftovers-native.md | - **B-19.14** — Fixed in source here: inactive roots suspend their render tree and covered native stack screens set freezeOnBlur; visited roots/history remain retained. This reduces rendering, not a claim of bounded retained-history memory. TV focus and Vega acceptance remain unrun. |
| B-19.15 | O | leftovers-native.md | - **B-19.15** — Partly fixed here: hook driver fails explicitly when the expected React dispatcher seam is absent; native test config is checked in CI; macOS CI compiles/runs actual URL, download-owner and installation-storage contract tests. React-internal hook imitation and missing Vega SDK CI remain explicit gaps. |
| B-19.16 | F | leftovers-native.md | - **B-19.16** — Fixed for the identified non-screen paths: session cleanup/local-profile messages now use the catalogue; unused English fallback parameters were removed. direct-connect and browse already use active catalogue labels; Artwork’s label was fixed on base. Native string lint passes. No screen restyling performed. |
| B-19.17 | N | leftovers-native.md | - **B-19.17** — Not present for shipped catalogues: only regional English is shipped and its fallback rules agree. A future non-English launch requires actual Hermes/Vega plural-rule acceptance; no new polyfill dependency or unsupported-language claim added. |
| B-19.18 | F | leftovers-native.md | - **B-19.18** — Fixed in source here: iOS/tvOS plists declare a single application scene; scene-owned windows start React and preserve cold/warm allowed links. iOS forwards universal links/Handoff/Spotlight too. Xcode 27/device acceptance remains outstanding. |
| B-19.19 | O | leftovers-native.md | - **B-19.19** — Partly fixed here: PiP uses the exported runtime capability, including Android API/feature detection; Vega stays false. Unmounted native search keyboard and inert Android adjustResize are exact frontend/native followups, not removed blindly. |
| B-20.1 | F | native-playback.md |  /  B-20.1  /  Fixed on the client: passthrough is its own list beside decoding, with the active route; DTS-HD naming corrected. Server side was playback-core's.  /  |
| B-20.2 | F | android-mpv.md |  /  B-20.2  /  Apple; fixed by the Apple lane.  /  |
| B-20.3 | A | review-playback-core.md | - B-20.3 — Already fixed in FIX-LOG; native implementation/physical acceptance is outside this review and was not re-tested here. |
| B-20.4 | F | android-mpv.md |  /  B-20.4  /  **Fixed** for both Android engines: a video is paused when the app leaves the screen without Picture in Picture, and JavaScript is told (systemPaused). Needs a device to confirm.  /  |
| B-20.5 | A | decision-audit.md |  /  B-20.5  /  already fixed in source, runtime unqualified  /  Native ready/display-switch gating exists; delayed-window and HDR mode-switch timing require actual Apple TV acceptance.  /  |
| B-20.6 | A | review-playback-core.md | - B-20.6 — Already fixed in FIX-LOG; native implementation/physical acceptance is outside this review and was not re-tested here. |
| B-20.7 | F | android-mpv.md |  /  B-20.7  /  Client side fixed by the Apple lane; Android now sends the fact too.  /  |
| B-20.8 | A | review-playback-core.md | - B-20.8 — Already fixed in shared canonical names and FIX-LOG native probe; native acceptance was not run. |
| B-20.9 | F | apple-mpv.md |  /  B-20.9  /  Client side fixed: the native profile states the HEVC tags AVPlayer takes; the server's planner has the matching field.  /  |
| B-20.10 | F | android-mpv.md |  /  B-20.10  /  **Fixed**: an output that lists no channel counts is eight channels on a digital output and two elsewhere, not always two.  /  |
| B-20.11 | F | android-mpv.md |  /  B-20.11  /  **Fixed** where this repository can: the published native profile declares text only (now stated and tested for Android), and the Android module's features.subtitleRendering is ['text']. The false table in packages/client-core (androidDeclaredProfile) is a copy here; see section 7.  /  |
| B-20.12 | F | android-mpv.md |  /  B-20.12  /  **Fixed on Android** for both engines: the app-drawn line and mpv's own subtitles scale with the system caption size. Colour and background are not read.  /  |
| B-20.13 | A | review-playback-core.md | - B-20.13 — Already fixed in FIX-LOG; native implementation/physical acceptance is outside this review and was not re-tested here. |
| B-20.14 | F | android-mpv.md |  /  B-20.14  /  **Fixed** in the published document: a codec with only a software decoder is declared at the software estimate for the device's class, or left out. The shared package still ignores the flag.  /  |
| B-20.15 | F | android-mpv.md |  /  B-20.15  /  Fixed by the Apple lane in the document; mpv on Android supplies chapters.  /  |
| B-20.16 | A | review-playback-core.md | - B-20.16 — Already fixed in FIX-LOG; native implementation/physical acceptance is outside this review and was not re-tested here. |
| B-20.17 | F | apple-mpv.md |  /  B-20.17  /  Fixed: the profile's default video and audiobook speeds are applied.  /  |
| B-20.18 | F | apple-mpv.md |  /  B-20.18  /  Fixed: the pre-play check waits 1.5 s at most.  /  |
| B-20.19 | F | native-playback.md |  /  B-20.19  /  **Fixed** by recovery (decision 6) plus a wider transport-stream seek search. Not reproduced: needs a cue-less MKV on a device.  /  |
| B-20.20 | F | android-mpv.md |  /  B-20.20  /  **Fixed**: passthrough is reported only when sound is going to the digital output, and the published document keeps the device's own decoder beside a bitstream.  /  |
| B-20.21 | F | audio-engines.md | - B-20.21 — fixed: music/crossfade players are discarded on owner retirement, and crossfade buffering respects low-memory limits. Following-video acceptance remains a device test. |
| B-20.22 | F | apple-mpv.md |  /  B-20.22  /  Fixed: both engines give the audio session back when a video is closed.  /  |
| B-20.23 | F | apple-mpv.md |  /  B-20.23  /  Fixed: the observer comes off the item it was put on.  /  |
| B-21.1 | A | leftovers-native.md | - **B-21.1** — Already fixed: Destinations has the permanent manual TV-code action. |
| B-21.2 | A | leftovers-native.md | - **B-21.2** — Already fixed: ProgramSheet cancellation uses confirmation. |
| B-21.3 | A | leftovers-native.md | - **B-21.3** — Already fixed: non-TV wide Rail offers Downloads with the same capability gate. |
| B-21.4 | A | leftovers-native.md | - **B-21.4** — Already fixed: independent bounded Saved indexing feeds Spotlight/Siri instead of depending on an absent Home My List shelf. |
| B-21.5 | A | leftovers-native.md | - **B-21.5** — Already fixed: Saved history/playlist/resource rendering uses virtualized PageList/windowed owners. |
| B-21.6 | F | native-screens.md | Unnumbered Outcome / Decisions / Settings evidence; recorded in reconciliation. |
| B-21.7 | A | leftovers-native.md | - **B-21.7** — Already fixed: SettingsPage class-scoped rows use the class labels. |
| B-21.8 | O | leftovers-native.md | - **B-21.8** — Not fixed: PlaylistScreen still sends an item-id container anchor. Existing QueueAnchor.position has no playlist revision/occurrence fence, so using a loaded-page index would be wrong after authorization filtering or a concurrent reorder. Needs the server-owned occurrence/revision selector, then native container-play and SavedPages wiring. |
| B-21.9 | O | leftovers-native.md | - **B-21.9** — Not fixed: PlaybackChoices still collapses subtitle languages and omits unknown language; PendingTrackChoice still accepts language/resource id only. Frontend must list catalogued resources/discoveries by source and stable identity; server/shared owner must carry a discovered file track through session materialisation without silently choosing a same-language alternative. |
| B-21.10 | A | leftovers-native.md | - **B-21.10** — Already fixed: SettingsPage supplies playerSettingsSupport playbackSpeed/audioEffects, and Vega’s override disables unsupported controls. |
| B-21.11 | O | leftovers-native.md | - **B-21.11** — Partly fixed on base: Saved view gathers up to 500 titles before playing, avoiding merely the loaded page. It is still not a full server query selector for larger views; no claim of whole-view playback beyond the bound. |
| B-21.12 | A | leftovers-native.md | - **B-21.12** — Already fixed: shared detail/menu edit permission follows actual owner permissions; current Hosted admission issues local server authority. |
| B-21.13 | A | leftovers-native.md | - **B-21.13** — Already fixed: Saved hook selection guard includes service identity as well as route. |
| B-21.14 | A | leftovers-native.md | - **B-21.14** — Already fixed for late results and duplicate navigation: pending-link/LinkFence binds viewer and intent; the underlying lookup need not finish before stale results are dropped. Lookup abortion remains an efficiency followup. |
| B-21.15 | I | native-app.md | - B-21.15 — not changed, by decision 10. |
| B-21.16 | A | leftovers-native.md | - **B-21.16** — Already fixed semantically: Focusable offers a named accessibility menu action. Visible More affordances on recommendation/playlist cards and physical remote/VoiceOver/TalkBack validation remain frontend acceptance work. |
| B-22.1 | A | leftovers-native.md | - **B-22.1** — Already fixed: route-identity accepts the signed secure LAN alias while retaining pin/signature validation. |
| B-22.2 | A | leftovers-native.md | - **B-22.2** — Already fixed in Hosted’s route origin de-duplication per FIX-LOG; Hosted source is outside these worktrees and was not rebuilt here. |
| B-22.3 | A | leftovers-native.md | - **B-22.3** — Already fixed in maintained browser credential coordination: shared selected-store operations use the IndexedDB coordination path rather than trusting a tab-local fallback lock alone. Two-tab HTTP race acceptance remains unrun. |
| B-22.4 | A | leftovers-native.md | - **B-22.4** — Already fixed: shared refresh/storage restore retains a temporarily unreadable sign-in; permanent record damage has its own native code. |
| B-22.5 | A | leftovers-native.md | - **B-22.5** — Already fixed: unsigned reset hints cannot discard the selected credential family; retirement needs authorized evidence. |
| B-22.6 | I | vega-transport.md | - **B-22.6 — Deliberate trade-off retained.** Signed server proof does not encrypt later plain-HTTP exchanges. |
| B-22.7 | A | leftovers-native.md | - **B-22.7** — Already fixed: shared manual address candidates preserve explicit choices and apply Portico’s bare-local default port. |
| B-22.8 | A | leftovers-native.md | - **B-22.8** — Already fixed: ordinary HttpLocalApi operation deadlines, distinct from deliberate streams/long polls. |
| B-22.9 | A | leftovers-native.md | - **B-22.9** — Already fixed: PorticoServerListWatcher owns a bounded request and releases its shared flight on timeout. |
| B-22.10 | A | leftovers-native.md | - **B-22.10** — Already fixed: server-connections retains retry floors; session-refresh proactive/reactive paths share not-before deadlines and positive jitter. |
| B-22.11 | A | leftovers-native.md | - **B-22.11** — Already fixed: playback/watch paths use the shared HTTP-date-aware Retry-After parser. |
| B-22.12 | A | leftovers-native.md | - **B-22.12** — Already fixed: web account adapter preserves retry floors. Browser auxiliary adapters belong to the web/Hosted owners. |
| B-22.13 | A | leftovers-native.md | - **B-22.13** — Already fixed: SSE parser bounds aggregate frame data and handles complete frames/CRLF splits. |
| B-22.14 | A | leftovers-native.md | - **B-22.14** — Already fixed for the concrete application/problem+json case: gatewayFailure distinguishes Portico application/json from proxy JSON dialects. A proxy deliberately imitating application/json remains an ambiguous-provenance case; no source-only guarantee added. |
| B-22.15 | F | leftovers-native.md | - **B-22.15** — Fixed in source here on Apple/Android: ten read slots, reserved control dispatch/admission and end-to-end queued deadline. Apple uses separate read/control socket pools so HTTP connection admission cannot undo the reservation. Vega sensitive control is unavailable under the redirect safety gate, not claimed equivalent. |
| B-22.16 | A | leftovers-native.md | - **B-22.16** — Already fixed: signed nonce proof uses symmetric two-minute clock allowance after signature validation and a clocks-disagree outcome. |
| B-22.17 | A | leftovers-native.md | - **B-22.17** — Already fixed: native session subscribes to ready route origins and writes KnownDirectServers.setAddress independently from logical session ownership. |
| B-22.18 | A | leftovers-native.md | - **B-22.18** — Already fixed on Apple/Android: auxiliary native streams open through the pinned selected route. Vega credential streams now fail before SDK dispatch because of its redirect boundary. |
| B-22.19 | I | vega-transport.md | - **B-22.19 — Fixed to the authorized fallback standard, not strict pre-follow parity.** The functional blanket refusal is removed; origin binding, response rejection, credential retirement, revocation attempts, live/durable sign-out and regressions are implemented. SDK automatic following remains the stated residual risk. |
| B-22.20 | A | leftovers-native.md | - **B-22.20** — Already fixed and retained: Android mutation client disables retryOnConnectionFailure and request bodies are one-shot. |
| B-22.21 | I | leftovers-native.md | - **B-22.21** — Contradicted by current policy: required-HTTPS server middleware was removed; plain HTTP must remain accepted from every address. Secure aliases remain additional candidates, not a mandatory discovery bootstrap. |
| B-22.22 | F | leftovers-native.md | - **B-22.22** — Fixed here: bounded local promotion on foreground/request activity, at most once a minute, with no Hosted chatter or ready-screen blanking; four candidate bound and five-second lifetime. |
| B-22.23 | A | leftovers-native.md | - **B-22.23** — Already fixed: CORS exposes Report-Every-Ms. |
| B-22.24 | A | leftovers-native.md | - **B-22.24** — Already fixed structurally: server discoveryInterfaceSuffix gives each interface distinct names. Same-VLAN multi-interface mDNS conformance/device acceptance remains unrun; not a claim of complete DNS-SD conformance. |
| B-22.25 | A | leftovers-native.md | - **B-22.25** — Already fixed: event retry floor gets additional positive spread and long delays are chunked safely. |
| B-22.26 | A | leftovers-native.md | - **B-22.26** — Already fixed: native activationGate shares Hosted outage guidance while preserving valid codes/protocol intervals and explicit manual actions. |
| B-25.1 | A | ops-packaging.md | - B-25.1: already fixed writer/reader shape; default feed now configured. |
| B-25.2 | F | ops-packaging.md | - B-25.2: fixed shared vet driver exception used by CI, Makefile and release builder. |
| B-25.3 | A | hosted.md | - **B-25.3** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-25.4 | A | hosted.md | - **B-25.4** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-25.5 | F | hosted-followups.md | - B-25.5 — Fixed here: copied toolkit and 99 scopes updated; complete canonical comparison and CI gate preserved. |
| B-25.6 | F | ops-packaging.md | - B-25.6: fixed: missing or unpinned signing keys fail before builds; no generated release key. |
| B-25.7 | F | ops-packaging.md | - B-25.7: fixed final-artifact authenticated inventory in release publishing, covering packages and embedded web/assets. |
| B-25.8 | A | hosted.md | - **B-25.8** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-25.9 | F | ops-packaging.md | - B-25.9: fixed release workflow gate on exact source SHA, full media tier, release performance tier and canonical qualified Linux FFmpeg bundle. Definition only: workflow was not run. |
| B-25.10 | O | leftovers-server-b.md | - B-25.10 — Not finished: Apple/Vega release-build and physical acceptance gates are in the native repository, outside writable scope. Native CI's source omission was confirmed read-only. Existing bridge tests do not prove device delivery; exact workflow/acceptance work below. |
| B-25.11 | F | ops-packaging.md | - B-25.11: fixed this tree's Manrope/Lucide notices and explicit package/container notices for scaling, SQLite, decoder fonts and hls.js. Native font/notice distribution and exhaustive dependency-license inventory require the native/release integrators. |
| B-25.12 | O | hosted-followups.md | - B-25.12 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews. |
| B-25.13 | F | ops-packaging.md | - B-25.13: fixed AST goroutine guard; found and corrected one previously missed inline storage expiry goroutine. |
| B-25.14 | F | leftovers-server-b.md | - B-25.14 — Fixed in writable API guide and extended the SQL guard to prepare the private Live TV spool against its own actual literal DDL: literal-only SQL preparation scope and the separate responsibility for runtime query constructors are explicit. statements_prepare_test.go already documents that boundary. External 02 Server.md claim needs narrowing. |
| B-25.15 | F | leftovers-server-b.md | - B-25.15 — Fixed. Every route receives every hostile body, every hostile query and every hostile wildcard separately, under both auth modes. Each request has a bounded deadline and a cancellation backstop. 503 is explicitly permitted for optional fixture subsystems/admission. No route-index rotation or exhaustive cross-product claim remains. The final authenticated-canary battery passes (11.5 seconds), verifies c… |
| B-25.16 | F | ops-packaging.md | - B-25.16: fixed 60-minute full runner/default release race timeout; workflow media/performance qualification has an explicit 120-minute test limit. Runtime duration not measured here. |
| B-25.17 | F | ops-packaging.md | - B-25.17: fixed by retiring the dormant Python harness referencing removed files; current Go schema tests remain. |
| B-25.18 | F | leftovers-server-b.md | - B-25.18 — Fixed. Migrations 2–11 folded into baseline; obsolete migration SQL and data backfills removed; one-step schema guard added, schema version 1 and digest/known objects synced. Fresh/reopen/index checks pass. |
| B-25.19 | A | hosted.md | - **B-25.19** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here. |
| B-25.20 | A | leftovers-server-b.md | - B-25.20 — Already fixed in Linux maintainer scripts, matching FIX-LOG: preremove distinguishes upgrade/removal; postinstall starts fresh install, restarts running upgrades, preserves stopped/disabled state and honors Debian policy through deb-systemd-invoke. Portable packaging fixtures pass. Actual DEB/RPM upgrade acceptance not run. |
| B-25.21 | O | leftovers-server-b.md | - B-25.21 — Not finished: native and Roku sibling checkouts still omit dependency refs, confirmed in read-only source. Those workflows and dependency locks are outside writable scope; no server-side dispatch pretending to validate a moving client branch was added. Roku is excluded from this programme. |
| B-09.34 | F | ops-packaging.md | - B-09.34: startup responses no longer disclose phases or raw failure paths. Public system build metadata remains for the API/client lanes to replace together. |

## Source checkpoint hashes

Current external report contents at final readback (UTC filesystem timestamps are not the programme’s narrative date labels). Scale is still changing; its pending state is deliberate.

| Report | SHA-256 |
|---|---|
| accounts-security.md | 591860712c5f486672331963b8e8be098d744c5940a2fa36830c3d11336c93aa |
| android-mpv.md | 50ccd71dfd624c328ee83518a8cba26204f8602c4b68b2c9e62136b07da0fab2 |
| android-native-review.md | 795ab6b7692a6ca93c908c3de67c9a50f11265ff93708ecd77a502fa25681b98 |
| api-platform.md | c37629a8cb2cf31c440ea30209dd99d091420ae741f33b5f17805b1aba9cb7e1 |
| apple-mpv-review.md | b54a4db55a54adcaaca477bb1c14c50e67a5091fc3424a6d2bb85792082f7a32 |
| apple-mpv.md | 30fbfe546afb64ab1eac79cd74d25de74f0dffceb8c3f20b0728da007f6378bf |
| apple-native-review.md | 77f7cbff9246989b638cb20b91adb5f2b73926029a5fc18b5fffb0386aa12fba |
| audio-engines.md | ecf11d1ef755df23fc76fc0f620842c7af2172042b8b5314c16896460990b8c5 |
| base-health-2.md | b597968e3778112088aff736ffbdea3ffaa81b6cded556f3fff07e848f91a139 |
| base-health-3.md | a7a7a4abe89f0b2ce35a0fd471648f14f37ca0226adb0397ac3ca1e7f1a5c1eb |
| base-health.md | 92774fb5a7e11546354456de490378ff844c3748695dbec78261928eea8464d4 |
| claim-and-gate.md | 8a23f0e6abf5d7f7b4edcf1d5bb6855592f99ca3adef1cc045c3c5bf91759eda |
| console.md | e1aa9e5f1cbf98fe2d3f29558475135152ba5b3af6fb1dbeebcf663a37d91506 |
| conversion-outputs.md | b1df0cd7abec53f9938850d28264bcfffac869f2aceb637de7c229633255de6e |
| decision-audit.md | 71a63eeec27088679f8802814efa5767105b5abe5b89d047f83df8703e2bc9bd |
| docs-internal.md | 21db7495aff8776d15f838ca397f863275cf77745ea948cc15533f70d84873fb |
| downloads.md | 26a728d9b087c5af303cec8bcdb62cf287073bd90037db0acca217a65f7da45e |
| e2e-hosted.md | 16aca4d266ccbbccd31ad05f50839ba122306d6613a7021c93abee404492294a |
| e2e-server.md | 947508237a09b0062897d926378e504863bc697a905bca1d959515a2833bab34 |
| e2e-web.md | cb96451c49ab64fe027260714306385cb24f1dcea63e60c096f190ea75554a10 |
| freshness-clients.md | c3fc70b2b3b882de070e7c2af7cb3c7d10ed5689f8843f16a444cad677c38b37 |
| freshness-server.md | ad1d3a4ebf4cc1c9625c394bc7e9da6425b2710de072611df5eb907bc6e49e3c |
| grouping-sorts.md | 075ac840646bd4193e20f2ef12b0ba7e40cf4ce9821f1f46a5e023e2a24e27b2 |
| handoff-draft.md | 6f6a8d9a619f54f6be737da48f08fff1413f74d89ef8196406156ec153de3b9f |
| hosted-followups.md | a643da04ce766f8388a0294481a00bc977f4f1f63c6b8bdc217b03b1ba629c88 |
| hosted.md | 8b4a2f46994b56b2d5059ac2d8e63c356a720807e903d8e96d2f217f28ed4a3e |
| identity.md | 42c6606eef99acf055564bbb729aba50cf67ae63e42ad785b18e15f0cca3f6a9 |
| integrator.md | 00fc78b2f681fd9fbd4857ed3e87ab7b985addba44d1d2c4a583591bd39d6076 |
| leftovers-native.md | 251f21a065be94070a4285514bbae381cb2a89dd38276b3a2aaa9ffa7b8c8050 |
| leftovers-server-a.md | 6d5206af506ebd8720ff167b5ddaa1e5144423cef750dc6c41992aacc9f535b4 |
| leftovers-server-b.md | bb1107c9a4843356946692d219f88e990bb03422c19224a944ba0e9470b3859a |
| limits-settings.md | 19715d7a5a857d853120a305d2e9f2515f8ae3e84c1231c7fa80ccfd89c8253d |
| livetv.md | 30eccf036c5bac31517e858bc3e9a50bf860ee053942a22cb490e71142f26f46 |
| music-audiobooks.md | 0a828c579dfe46af7afc7c663b097ff765fbddd77807c0ca9a3460d9bd4289aa |
| naming.md | 010631b783484a5ada19d8a22a3c12013cc36a46e1fc1c69e129ce0d8f68fbd5 |
| native-app.md | a9ada4a75ea86ab3dde4865bd9d7d5c21001a1a6351a588515afcafec126b5cd |
| native-health-2.md | 128a71c541aa4b95be1f00cd0ba869b66850cf72026f302ba5c952c66534dd62 |
| native-health.md | 6036161a237c960ddd7a4cd3dc2ec0d2dc6c63056bbe37a1379a32f2257e59bc |
| native-owner.md | a156b909f3d5df1a7afd76f3b2f6c2a504377e6ab132d9a7a79bc4387dcc4d91 |
| native-playback.md | f674ec9d6374599248d69c951e9342c0c9ee0740767e6c4e256410b423a19f7d |
| native-screens.md | b6004941ff8b89956b8d161696693f0920ea0807475f76073465f56a2ad25e10 |
| native-sync.md | 4782ea379301cad0c4ec4d1877bc82a1deabae3f3d0cbdb5ba9de2094e7e887e |
| networking.md | 1f3b57c15adff1ff1d28f8cfdf1cd8def0e1a704db5268bb242380272dd9ab09 |
| ops-packaging.md | c3c014571fb82cf3b741d2c5e5c143a66281b8b83bf5369b68bd8419f518f2b4 |
| personal-state.md | 6b7633cb428e16d5ed40087c0ac1aa68fa1684013daf97d13f054e1e3d14bea2 |
| planner-wiring.md | c5cddbec08340049478d0c05d67f9e56a3ef05e77939e3a00bcbcc3d348e32a7 |
| playback-core.md | 2f087e3a12569905f13477b8f2f6be0b5ab998ba715c7593a7c23cd199a22198 |
| providers-matching.md | 7e3a73589d3606e035a49e3782cd70cae57dd509d50a1528ad91ae500a1bdc0d |
| queue-and-retention.md | cca5e790beaef9110b0d44d13d910ee734e02555ef231457d860d956a44b3499 |
| registry.md | c0966b5f149f58a984e730de64256f2df4731a362caaf4f5ef802bd8110682a1 |
| review-accounts-security.md | 2660937a43b2ea4beaa83976f01aed9228997163ee07133b011b93c5f3fa1adc |
| review-api-platform.md | e6e5136decf9730d219ca47df25a61a9329c22fb9efc77cdc85ae7737f917b97 |
| review-downloads.md | c3ebe335e696fd280a7b62a4d2ed668d5d2ed13e10be798502ba1209e5f4e67a |
| review-freshness.md | 11f2e25bb1f5ce630b2f439e03f13ddfc204f3ee150ac5ee4f406ccd55a0cdaa |
| review-hosted.md | 72ad837a4700932fc70a76c173c548209ef717499e3b809dff9e47f82ed7b767 |
| review-identity.md | 323a6bc430e93f53fe0594a4fca1ee53500c282b9a6e069b4cf17b57e18d57c6 |
| review-livetv.md | c4c0cf18dbfcb7409260ca976279955ebd3372c80d203b0ad0cece3b278444be |
| review-native-js.md | 756faec9b6c931f202b78d338d3bcf9e924e9d8dc941793bf3ae74eb1e7297c5 |
| review-networking.md | a1ce703b15785f6a6582e54c4412f7f96008e3510902dd3c2561dc9a439a776c |
| review-ops.md | 09a9d8d159b81c7ee4e048b4af9e48c76ff654a946d6b7ff050e92d76cf6f2dd |
| review-playback-core.md | a93d176e378e4962275b054c60e86c013ed5482bc82db9128a999cb232dc449a |
| review-playback-pipeline.md | 6c6e89e24c6eaa10570bd1139c8f7efcf6ba9eaa19daca539f434e1f193d06ef |
| review-registry.md | 901ddbbb5c1408ca01d46f75c2b9e0ccf99eb312a0d399497fd0684fef783c75 |
| review-scan.md | adaf49fbf7846835423b89753d873cb19cb938c0d0c2032fc47ab20f3be9497c |
| review-server-late.md | 7409d689959ef9deaace624881aa8563acafba9e19c8a46af5fd9127fc262518 |
| review-web-logic.md | c4efa689e240e7f97bd45c9d4eb317312f1939507812c55f84ecd727b4546908 |
| scale-check-2.md | 1f3f696ef7572aaec652d70163209ea86b7313caf6a96f12ada8ce1dc77d4537 |
| scale-check.md | 89fa74533dc0212094bac7b868a50896c9b8fea9e4236b4a6f9af7c6d67b9a58 |
| scan-loop.md | a8b58858a9134db20a562b13cd28697112bbb535ba4154899493ecae55258839 |
| search-sort.md | 71894f5159ee2f1ee91afe0b053a8625db19c7c792118bef3c67c75e419e5469 |
| security-sweep.md | fed24c1244ee32a71e53b43082a3d39b123806b668c75e9d41513a65780c465c |
| server-docs.md | 02a9f47d96a4709ba2aaa4783c57913c2f509f086133964b2b93de1f2f9b5aa6 |
| server-followups.md | 51d9295ff06e8d7f35aa64a3804cdf85d818c1d9246b7e26d74811bae2239b46 |
| social.md | 8c79c6c96956bee4da46e96378976a2cf0a157ceb12b59b7feafb00bfdcb6d26 |
| subtitles-text.md | 9763d19a0e4e1b67d0a08405f33385abee23a719486187ec662e16cce2d4c969 |
| test-plans.md | 2961ec85ff3ca03af581fa193c7db3061446faaaba19f91fe8306c5356a86c6b |
| vega-transport.md | 2cdc542e82ec0ffa6b0dcbbee674f63d2f49f6588c602b98c3861ed60e9d6fac |
| vega.md | cb547c7a627079b9235577990b148da5b5612a714a0f7ae519b4cff19adf6c3f |
| web-app.md | 1ae3eb74496d0b92ba223822015acda5d35b89eb1c3eead00c3681eefe340287 |
| web-owner.md | 01113655b6bc79301894590787e11d4c5f812dd496ad949b3cc28862c56dca34 |
| web-player.md | a2fa66e3b85e4e10fc514edd013d5131ed6bd8c6c5b7b97184fcf2ef39312523 |
| web-subtitles.md | 3ff98f83140536567c309c5628c013d5d8e518a45055ffddbb037b428c0698cc |
| web-viewer.md | 1c59cbd8b2239a7a75d7968543d5549e1118f39cde20af1e1e6161af9cbcd95f |

Snapshots: 001 preserves toolkit/contracts; 002 preserves the first complete document pass. Both are cumulative milestones; only the final snapshot must be applied.

## Final snapshot receipt

Final cumulative snapshot: `_program/snapshots/handoff-update/003`. Subject: Keep the final handoff accurate and Hosted contracts current.

- Base: 48a0d7a539b3471323303718037652fa72131103.
- Tracked patch: 667315 bytes; SHA-256 `9a568cde3f1419d9940185fde5686a696354e7b62a872cf9899da7fcd16a8439`.
- Three new toolkit files are preserved in untracked.tar; apply them along with the patch.
- Apply only 003. Snapshots 001 and 002 are cumulative milestones, not additional patches.
- Final source-tree diff and untracked paths exactly match the receipt. No source-tree changes follow 003. Report updates are outside that worktree. Nothing committed, pushed or deployed.
