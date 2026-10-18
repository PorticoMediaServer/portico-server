# Outcome

Third-base verification has repaired fresh-server startup, transaction principal metadata and restore seed detection, plus stale audio/sign-in/queue fixtures. Web check passes all 545 tests, server E2E passes all 18 groups, and browser E2E passes all 13 groups including the desktop/phone walk. Final full Go suite passes (86 tested packages, 18 with no tests). Both final-binary E2E drivers pass. Disposable credentials/state/media/Chrome profiles were deleted. No lane implementation gap remains. Host: ehler-homelab-mini-1. Initial working tree clean.

# Decisions I made

- Follow this lane's full-suite exception and GOFLAGS=-p=4. No agents, commits, installs, pushes or deployments.
- Context sync failed because its lock directory is read-only; existing canonical project/standards/verification guidance was read directly.
- Keep disposable runtime state beneath `_program/scratch/base-health-3`, remove it at completion, and retain compact non-secret command evidence outside that disposable directory.

# Bugs

No domain bug-ID list assigned. Initial apikit, client-core and cast suites passed (exit 0). Final server and web suites pass; complete initial failures and repairs follow.

| Failure group | Cause | Repair/status |
| --- | --- | --- |
| Real server driver startup; browser driver first-run startup; isolated networking `TestClaimIdentityStoreRefusesDamagedMarkedSchema` (all four subcases) | Networking exact-schema verification × queue-and-retention table bounds: new `networking_server_identities_bound_*` triggers are returned by the verifier query but absent from its expected baseline extraction. Every fresh server stops in recovery. | Include the server-identity trigger namespace in exact baseline extraction; add fresh-schema acceptance plus missing/altered/accounting/unexpected-trigger refusal regressions. Focused schema regression passes; cmd/server package passes alone after repair (129.453 s) and in the final full suite (98.815 s). Both real-server startup paths pass. No guard weakened. |
| Web `gate 3: a device the owner denied or has yet to approve is held inside the shell with Check again and Sign out` | e2e-web session management fix × old component fixture: stub omits `currentAccessToken`, causing an asynchronous TypeError before device admission completes. | Add a stable synthetic test token to the session stub. Isolated sign-in tests pass (exit 0). |
| Server `go vet ./...` | Known vendored SQLite `unsafe.Pointer` diagnostics only. | Explicitly outside scope; unchanged. |

Additional web failure: `audio-render.test.ts` cases (e), (f), (b)(i) reject the fake HTTP 206 response because it lacks Content-Range (review-web-logic stricter audio ranges × old web render fixture). Failed assertions leave the renderer interval running, stalling ordered whole-suite output. Corrected fixture headers to the actual requested range and total, and registered disposal with `ctx.after` so it runs on failure too. Initial web run and its isolated failing replay were interrupted (exit 130) after this was established; second full check passed all 545 tests. No production range validation changed.

Snapshots 001 and 002 saved after compilation and focused schema/sign-in/audio checks. Windows and macOS full server cross-builds both pass (exit 0).

New isolated failures confirmed:

- identity `TestAuthenticatedDeviceMetadataIsVerifiedWithoutExtraStatements`, `TestReauthorizeTxUsesOwnedTransactionAndSeesRevocation`; httpapi `TestPlaybackAuthorityUsesOneTransactionForSessionAndItemPolicy`: security-sweep authentication deadline metadata × transaction authentication seam. `ReauthorizeTx` reconstructs a Principal but omits its new `ExpiresAt` (and existing narrowed Grant). Repair re-reads token/family deadline and grant in the same SQL statement, retains device metadata, refuses expired family horizons, and validates grant equality. New regression covers shortened/expired horizons and refusal to widen a Cast grant. No extra statement or new DB handle.
- persistence `TestSeededTablesMatchAFreshInstall`: restore verification × queue-and-retention bounds: seed extraction recognized VALUES only, omitting the populated maintenance_table_bounds table seeded by INSERT SELECT. Repair derives SELECT seeds too; original fresh-install equality and each-missing-table guards remain.
- httpapi `TestPlaybackV1QueueAuthorization`: whole-view queues × old playback selector expectation. Query selectors are now supported; the fixture used obsolete library syntax and expected unsupported_selector. Repair sends the actual typed query for the owner's inaccessible library and requires 404 not_found, retaining the permission boundary.

Now-playing and web-playback reported as failing in the brief both pass in isolated runs; no change made to them.


## Complete initial Go failure inventory

All failing Go packages were rerun separately. Networking, Hosted and cmd/server passed after the exact-schema repair. Identity, httpapi and seed failures were reproduced alone before their repairs. The following includes all failed subcases emitted by the initial full run.

| Test | Cause and change |
| --- | --- |
| `cmd/server.TestCurrentNetworkingStartupOwnerAndRestart` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreOfAnotherServersBareDatabaseIsRolledBack` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreOfThisServersBareDatabaseIsKept` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_rec_data_revision` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/authority_file_has_the_wrong_length` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_notification_revision` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/schema:_a_table_is_missing` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_download_settings` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/schema:_a_view_is_missing` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_admin_revision` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_kinds` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_networking_certificate_settings` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_directory_revision` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_screen_metadata_consent` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/subtitles_folder_is_a_file` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/claim_schema:_the_version_marker_is_missing` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_hosted_trust_state` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_content_eligible_state` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_book_group_visible_state` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_saved_resource_generation` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_metadata_runtime_settings` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_hosted_membership_sync` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_movie_category_visible_state` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/mount_configuration_folder_is_a_file` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_attribute_fields` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_dvr_storage_policy` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_networking_remote_state` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/schema:_a_column_start-up_writes_is_missing` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_playback_owner_policy` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_console_settings_revision` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_generations` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/subtitle_objects_folder_is_a_file` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_collection_visible_state` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/schema:_an_index_is_missing` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/claim_schema:_an_identity_protection_is_missing` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_networking_claim_schema` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_configuration` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_dvr_defaults` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_live_allocation_reconcile_cursor` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_database_maintenance_status` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/authority_file_is_not_a_valid_value` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_maintenance_policy` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_download_receipt_sequence` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_personal_sort_state` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/signing_key_is_another_server's` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_book_evidence_clock` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_mount_revision` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_catalog_visibility_sort_state` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreIsRefusedForEveryMissingStartupRequirement/seeded_row_is_missing:_lc_maintenance_cursor` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreOfThisServersBackupFolderIsKept` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreRefusesABackgroundPriorityStartupWouldNotAccept` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestRestoreOfAHealthyBackupDoesNotNeedTheTemporaryFolder` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestSIGTERMDrainsInsideItsBudgetAndLeavesNoChildren` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `cmd/server.TestHealthCheckAgreesWithReadiness` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimAcknowledgmentImmediatelyAdmitsOwner` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimInstallationPreservesExistingAccountLinks` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimInstallationPreservesExistingAccountLinks/owner` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimInstallationPreservesExistingAccountLinks/another_member` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupErasesBeforeRetireAndAcknowledgesAfterRestart` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupFailureRollsBackRetirementAndNeverAcknowledges` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupRejectsForgedStaleOrCrossClaimResults` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupRejectsForgedStaleOrCrossClaimResults/signature` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupRejectsForgedStaleOrCrossClaimResults/nonce` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupRejectsForgedStaleOrCrossClaimResults/generation` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupRejectsForgedStaleOrCrossClaimResults/operation` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupRejectsForgedStaleOrCrossClaimResults/expiry` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupRejectsForgedStaleOrCrossClaimResults/disposition` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupRejectsForgedStaleOrCrossClaimResults/profile` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupAcknowledgementOutageAndSuccessorIsolation` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupMemberErasureDoesNotRetireOwnerClaim` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupCancelledLeaseCannotEraseOrRetire` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestClaimCleanupRejectsAckWithoutCommittedReceipt` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestAnEmptyEventOutboxIsNothingDue` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestServerEventsBackOffAndDropStaleClaims` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestOnlyANameChangeQueuesARename` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestANetworkChangeAloneNeverCallsHosted` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestNetworkChangeBurstsCoalesceIntoOneWake` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestADueCheckInCallsHostedExactlyOnce` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestPorticoIdentityAdmitsOnlyMembers` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestMigratedFamilyRefreshSaysMigrated` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestMembershipPushDeltaAndGapRecovery` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestMembershipPushBacksOffWhileHostedIsDown` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestPorticoOwnerTransferReachesHosted` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestIdentityAssertionChallengeSkewAndConsent` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestPorticoSignInKeepsLocalSecondFactorAndRecoveryScope` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestMembershipPushConsentDeparturesAndPages` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestMembershipDeltasArePagedAndFallBackToTheFullList` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestChallengeProofIsSignedByTheServerIdentity` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestPorticoOwnerAcceptsCustody` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestADepartureOlderThanARejoinIsNotApplied` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestRootRevocationSurvivesRestartAndOldCertificateReplay` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestRootRevocationEndsSignedAdmissionsAndLeavesPasswordFamilies` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestLearningASigningKeyWasRevokedEndsTheSessionsItAdmitted` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestTheCheckInCertificateTeachesRevocationsAndNothingElseDoes` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestOversizedCertificateRevisionNeverReachesPersistence` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/hosted.TestLargestSharedCertificateRevisionPersists` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/httpapi.TestPlaybackAuthorityUsesOneTransactionForSessionAndItemPolicy` | Fixed: security-sweep deadline metadata × transaction seam; re-read expiry/horizon and narrowed grant in the existing statement. |
| `internal/httpapi.TestPlaybackV1QueueAuthorization` | Fixed: whole-view queries × stale queue expectation; verify 404 for an inaccessible library using the supported query contract. |
| `internal/identity.TestAuthenticatedDeviceMetadataIsVerifiedWithoutExtraStatements` | Fixed: security-sweep deadline metadata × transaction seam; re-read expiry/horizon and narrowed grant in the existing statement. |
| `internal/identity.TestReauthorizeTxUsesOwnedTransactionAndSeesRevocation` | Fixed: security-sweep deadline metadata × transaction seam; re-read expiry/horizon and narrowed grant in the existing statement. |
| `internal/networking.TestCertificateRouteAcceptsTheMemberLabelName` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestCertificateConnectedLifecycleRestartAndAuthorityFencing` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestCertificateServingReadPathAndTLSResumption` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestCertificatePauseAndCurrentRevisionGuard` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestCertificateKeyFilePlainAndStable` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestCertificateMaterialScopeAndRenewalMargin` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestCertificatePauseResumeCannotInstallLatePendingResponse` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimAuthorityCaptureTerminalWithoutBearer` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimAuthorityStickyRevisionAndDigest` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimAuthorityRevocationRollbackAndStaleTuple` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimAuthorityBootstrapAndLeaseFence` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimHandlerIntentCancelWithoutHostedOperationClearsIntent` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimCurrentInstalledCaptureAndCancellation` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimEndpointPendingProofAndOwnerFence` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimEndpointPendingProofAndOwnerFence/success` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimEndpointPendingProofAndOwnerFence/wrong-origin` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimEndpointPendingProofAndOwnerFence/revoked-owner` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimEndpointPendingProofAndOwnerFence/stale-claim` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimEndpointPendingProofAndOwnerFence/quiesced` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimEndpointProofWithoutPendingDenied` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimHandlerOwnerJourney` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimHandlerSessionRevokedDuringRemote` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimHandlerMandatoryAdmissionAndCommitGuard` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimHandlerPreparesWithoutAnAccount` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestUnboundWebClaimRequestIsSigned` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreFreshRestartAndImmutability` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreReplacementRetainsHistory` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreRejectsWrongPointerAndChosenID` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStorePendingAndHostileOperation` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreOverflowAndCancellation` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreRefusesDamagedMarkedSchema` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreRefusesDamagedMarkedSchema/table` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreRefusesDamagedMarkedSchema/trigger` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreRefusesDamagedMarkedSchema/index` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreRefusesDamagedMarkedSchema/foreign_key` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreReplaceCannotRewriteHistory` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreReplaceCannotRewriteHistory/OFF` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimIdentityStoreReplaceCannotRewriteHistory/ON` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimLifecycleUngatedCallsRefused` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimLifecycleLateInstallRejectedBeforeDrain` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimLifecycleLateSignatureCleared` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteInstallRestartAndSchema` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteCancelWinsLateInstallAndRestart` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteInstallThenCancelRevokesProtectedReference` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLitePrecommitCancelAndSuccessorIsolation` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteApprovalRevisionAndExpiry` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteGuardRollbackAndDenyOnlyCancellation` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteOutboxFailureRollsBackFence` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLitePlaintextCredentialAndNoKey` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLitePlaintextRoundTripAndTamper` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteCancellationWhileConnectionBlocked` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteGenerationOverflowAndNoKeyAdoption` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteInstalledAuthorityIndependentOfOwnerEpoch` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteConcurrentInstallThenCancel` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimSQLiteCancellationProofBindings` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimCredentialsMigrateToPlain` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestClaimInstallationReadinessAndWakeCommitTogether` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestUnboundClaimAdoptsTheApproversAccount` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestAccessURLsPublishedAsManualCandidates` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestManualCandidatesAreNeverProven` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestAccessURLChangePublishesOnce` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestSettingsReadFailureKeepsLastList` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestAccessURLsDedupedOrderedAndCapped` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestW2I02RemoteIntentAndPublicationRecheckInstalledAuthority` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestW2I02RemoteIntentAndPublicationRejectStaleConfigRevision` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestATopologyChangeThatChangesNoRouteDoesNotCallHosted` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestAnAddressWithNoNameIsTheOnlyReasonToAskForOne` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestUnverifiedIPv6IsNotPublished` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestIPv6PinholeOpensPublishesAndCloses` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestPinholeProbeBacksOffWithoutFirewallControl` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestRouteLabelDropsStaleCachedNames` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestMappingProbeBacksOffWithoutGatewaySupport` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestManagerIPv4LossDoesNotResurrectRetainedObservation` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestALostFamilyIsWithdrawnOnlyAfterRepeatedObservationsOverTime` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestAnInconclusiveObservationNeitherWithdrawsNorResets` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestTheLastFamilyIsNeverWithdrawn` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestAFailedWithdrawalKeepsTheNameAndIsRetried` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestAFamilyThatWasNeverReportedIsNeverTracked` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestRouteIdentityDescriptorExhaustion` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestRouteIdentityFailureClassesAreLogged` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestRouteIdentityFailureLogIsRateLimitedPerClass` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestRouteMutationSerializesCertificateNamingAndFamilyWithdrawal` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestRouteMutationSerializesCertificateNamingAndFamilyWithdrawal/naming` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestRouteMutationSerializesCertificateNamingAndFamilyWithdrawal/withdrawal` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestRouteMutationWaitIsCancelledWithoutRunningEffect` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestHostedCandidatesStayTLS` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestHostedCandidatesAreBoundedTogether` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/networking.TestConfiguredNetworkingControllerReportsUnconfiguredReadsBeforeClaim` | Fixed: table bounds × exact claim schema; compare the declared identity bound triggers too. |
| `internal/persistence.TestSeededTablesMatchAFreshInstall` | Fixed: table bounds × restore seed extraction; recognize INSERT SELECT seeds. |

# API changes

None.

# Schema changes

None.

# Owner or viewer settings added or wired

None.

# Needed from others

No new client screens, routes, schema edits or settings are required. Physical/native acceptance remains with the owning environments; the lane does not wait for later capacity merges.

# Test notes for other environments

Required: helper build; server vet/test; apikit tests; core/web/cast checks; Windows/macOS builds; server and browser fresh-state drivers.

Server driver second run passed 18 groups with 1446 HTTP requests, including generated media/scans, browse/search, playback/engines, restrictions, API scopes/MCP, webhook/SSE, deletion/missing/unreachable roots, capacity, backup/restore/restart and library removal. Known driver limitations remain: no PGS source fixture and no provider-backed canonical person identity. Final server replay also passed 18 groups with 687 HTTP requests. Final browser replay passed all 13 groups and all 114 screen observations (57 per viewport); it covers PGS separately. No physical engine/hardware/Hosted/provider/network deployment acceptance is claimed.

Cold replay on Ubuntu: use the existing node_modules symlink (no install), Go, Node 24, Python, FFmpeg/ffprobe and `/usr/bin/google-chrome`. Set `GOFLAGS=-p=4`, build the helper from server as specified in the command table, and export `PORTICO_PLAYBACK_TEST_HELPER=/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/playback-test-helper`. Run the two Go module commands in server/apikit respectively and the npm commands from the repository root. Production bundle: `npm run build --workspace portico-web -- --configLoader runner`.

Driver replay from the repository root:

```sh
GOFLAGS=-p=4 python3 scripts/e2e-server.py --scratch /absolute/private/new-server-run --binary /absolute/current-server-binary
PORTICO_SERVER_BIN=/absolute/current-server-binary PORTICO_BROWSER_OUTPUT=/absolute/private/new-browser-run node web/test/integration/browser-smoke.mjs
```

Representative final-run desktop movie browse and phone movie sort/filter screenshots were visually inspected: controls and cards render within their viewports; generated fixtures intentionally have no poster artwork. Screenshots stay disposable and will be removed with state.

Use a new throwaway directory every time. Pass means 18 server scenario groups and all 13 browser groups, desktop and phone walk, no unexpected failed action/uncaught exception, plus all unit suites. The browser driver uses generated credentials, loopback only, a separate disposable Chrome profile and blocks external traffic. Server startup/restore must accept the current bounds but refuse missing or altered identity triggers. Authentication tests must preserve verified device/deadline/grant metadata and still refuse uncommitted revocations, expired horizons and forged widened grants. Do not discard these guards to accommodate load.

Cross-platform commands: `GOOS=windows CGO_ENABLED=0 GOFLAGS=-p=4 go build ./...` and darwin equivalent from server both exit 0. These establish compilation, not native execution.

# Not finished

Nothing in the assigned lane remains unfinished. All required suites, both final-server E2E runs and both cross-platform builds pass. Plain server vet retains only the explicitly excluded upstream SQLite unsafe.Pointer warnings; the application-vet wrapper passes. Hardware/native/provider/Hosted/network acceptance and the later scale-check-2 merge remain outside this lane. Initial Go run found 122 top-level failed tests (190 including subcases) in six packages; all are inventoried above. No tests deleted or security bounds weakened.

## Command results

All Go commands use GOFLAGS=-p=4. Tests use PORTICO_PLAYBACK_TEST_HELPER at the required shared scratch path.

| Command | Initial | Latest/final |
| --- | --- | --- |
| server `go build -o …/scratch/playback-test-helper ./cmd/server` | Exit 0 | Exit 0 after all production repairs |
| server `go vet ./...` | Exit 1, SQLite unsafe.Pointer only | Exit 1, same known SQLite-only diagnostics |
| server `go test ./...` | Exit 1, six failing packages listed above | Exit 0; 86 tested packages, 18 with no tests; zero failures |
| apikit `go test ./...` | Exit 0, four tested packages | Same passing result; unchanged |
| root `npm run test:core` | Exit 0 | 1937/1937 pass; unchanged |
| root `npm run check --workspace portico-web` | Interrupted at stalled invalid audio fixture (130) | Exit 0, 545/545 tests, all lint/TS checks pass |
| root `npm run test:cast` | Exit 0 | 20/20 pass; unchanged |
| root production web build with `--configLoader runner` | Exit 0 | Used for both browser driver replays |
| server Windows `CGO_ENABLED=0 go build ./...` | Exit 0 | Exit 0 after production repairs |
| server macOS `CGO_ENABLED=0 go build ./...` | Exit 0 | Exit 0 after production repairs |
| server `GOFLAGS=-p=4 ../scripts/vet-go.sh` | — | Exit 0; all application analyzers, only the known upstream unsafeptr exception |
| server E2E `python3 scripts/e2e-server.py --scratch … --binary …` | Exit 1, fresh-server recovery startup | Final replay exit 0, 18 groups/687 HTTP requests |
| browser E2E `node web/test/integration/browser-smoke.mjs` | Exit 1, same fresh-server recovery | Final replay exit 0, all 13 groups; 57 screens at each of desktop and phone widths |

Snapshots: 001 schema/sign-in repair; 002 audio fixture/cleanup; 003 transaction deadline/grant, seed extraction and query authorization expectation. Final cumulative snapshot: `_program/snapshots/base-health-3/004/` (13,752-byte tracked patch plus one new regression file). Worktree has not changed after it. Compact logs retained in `_program/scratch/base-health-3-logs`; all disposable state/media/browser data beneath `_program/scratch/base-health-3` was deleted after the drivers finished (693671268 file bytes, below 3 GB). The directory no longer exists. Non-secret driver group summaries are retained beside command logs.

Verification ended 2026-10-13 20:29 UTC. `git diff --check` passed. No commits, pushes, deployments, installed packages or weakened guards. The two named load-sensitive Go cases did not fail in the final suite.
