# base-health-2 — combined server verification

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-base-health-2`; branch `audit-implementation/base-health-2`; host `ehler-homelab-mini-1` (personal mini-1). The starting tree was clean. All work remains local; no commits, installs, pushes or deployments. Logs: `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/base-health-2/`.

## 1. Outcome

Every required suite passes on the finished tree: server 85 tested packages, apikit 4 tested packages, shared client 1848 tests, web 512 tests and Cast 20 tests. Server vet has only its 63 known vendored SQLite findings. Linux, Windows amd64, Darwin arm64 and devtrust builds pass. The initial complete run found 56 failing tests in nine packages; isolated checks also found a duplicate settings key and a stale operations capability fixture. All real failures are fixed. No load-sensitive startup test was relaxed.

Real defects repaired: missing retention keys; PIN reset SQL naming removed columns; malformed title IDs reaching SQL; manual metadata writes missing live owner/profile predicates; retired Hosted erasure blocking linked local account cleanup; recording-owner cursors naming retired authority; incomplete playback geometry reducing fixed quality height to zero; malformed batch/share/mount requests returning 500. Development builds compile again and development routes have typed contracts. The generic playback refusal now explains server/device support through the string catalogue.

## 2. Decisions I made

- Portico Account users use this server's local membership, profiles, restrictions, sessions and grants. Tests now exercise that model. Retired Hosted-positive assertions became valid local-positive assertions plus explicit retired-authority-negative assertions. No old authority path or schema column was restored.
- Signed account-erasure jobs resolve the exact Portico link and use `EraseMemberTx`; unrelated local accounts and the recovery owner survive. Download preparation cancellation and its authority fences follow the existing erasure implementation.
- Recording owners come only from current local profiles, including linked Portico Accounts. A legacy Hosted policy entry cannot become a recording target or cursor.
- Oversized XML text remains bounded and usable: keep 4096 bytes and the partial marker. Entity/directive, node/depth, attribute, multiple-root and total-size guards remain.
- Catalogue plan guards distinguish bounded CTE/constant rows and the UPDATE target from the batch-driven subquery. All base-table seeks remain checked. Renaming ambiguous CTEs avoids a dropped-table allowance.
- The golden file was reviewed, not blindly refreshed: 422 cases; 56 changed cases contain grouping capability metadata, catalogue revision 631→633, or two newly supported year sorts (books and songs, 400→200). Apart from these two new successful results, result membership, order and restricted totals are unchanged. `initial/golden-differences.json` records the leaf diff.
- Keep existing dormant client behavior. Register three remote-control and nine absent Hosted profile request protocols in a separate consumer contract (`packages/contracts/src/optional`), generated from apikit by `cmd/clientcontractgen`. These do not enter production OpenAPI or add handlers. Development's two real routes have a separate generated module and compile only under `devtrust && !release`. Production remains 622 operations.
- A generic unavailable playback route is a server/device support failure, not a permission failure. Its catalogue message is permanent rather than offering a futile retry.
- No test was deleted. No restriction matrix was allow-listed or weakened. Both full HTTP restriction matrices and the route/scope/credential/registry guards are included in the complete HTTP suite.
- Context sync was attempted; the sandbox prevents writing its lock. Relevant existing canonical project/standards/verification guidance was read directly.

## 3. Bugs

This health lane has no assigned B-number list. Every reproduced command failure is recorded here. All nine initially failing Go packages were rerun alone before diagnosis (playback selected its sole failing test). Both named load-sensitive startup tests passed; neither was chased or relaxed.

| Original failing test | Cause and change |
|---|---|
| `compactcatalog.TestRetentionKeepsReferencedTerms` | Fixed: review-freshness × grouping-sorts: new retained serving tables lacked pruning keys; added both key definitions. |
| `compactcatalog.TestPerItemDerivationStatementsSeek` | Fixed: grouping-sorts × catalogue guard: compact CTE spacing, a constant-only row and outer UPDATE confused plan classification; recognize exactly these bounded structures and continue checking base-table seeks. |
| `downloads.TestReceiptSignVerifyRevalidateAndRevoke` | Fixed: identity × downloads: fixture had no live membership, so renewal correctly refused; seed its real account/profile/membership. |
| `downloads.TestACopyOnADeviceOutlivesServerTidying` | Fixed: identity × downloads: missing membership fixture looked disabled; add live membership, retain both offline-copy survival cases. |
| `downloads.TestPolicyRefusesAViewerWithoutDownloads` | Fixed: identity × downloads: retired Hosted restriction fixture; exercise the actual local allow_downloads switch. |
| `downloads.TestPolicyRevocationStopsAnAdmittedClaimAndItsReceipts` | Fixed: identity × downloads: retired Hosted authority cannot issue a receipt; admit locally, then disable the membership and retain refusal/revocation assertions. |
| `downloads.TestDownloadPolicySeparatesLocalAndHostedProfileIDs` | Fixed: identity × downloads: retired authority is now refused; assert that refusal and that old restriction rows do not affect valid local profiles. |
| `downloads.TestOlderOfflinePlayKeepsLatestPlayDate` | Fixed: review-playback-pipeline × leftovers-server-b: early end at 20/60 seconds is deliberately not completion; finish at 60 seconds to test completed-play recency. |
| `downloads.TestReceiptReplayDistinguishesGuardDenialFromFailure` | Fixed: identity × downloads: absent membership caused account-disabled before temporary failure; provide live membership and retain denial-versus-retry coverage. |
| `hosted.TestClaimCleanupErasesBeforeRetireAndAcknowledgesAfterRestart` | Fixed: identity × networking cleanup: illegal Hosted grant fixtures; use exact linked local membership plus an unrelated account. Remove retired Hosted erasure branch that rejected before local erasure; retain all signature/claim/lease/rollback/restart/acknowledgement/owner checks. |
| `hosted.TestClaimCleanupFailureRollsBackRetirementAndNeverAcknowledges` | Fixed: identity × networking cleanup: illegal Hosted grant fixtures; use exact linked local membership plus an unrelated account. Remove retired Hosted erasure branch that rejected before local erasure; retain all signature/claim/lease/rollback/restart/acknowledgement/owner checks. |
| `hosted.TestClaimCleanupRejectsForgedStaleOrCrossClaimResults` | Fixed: identity × networking cleanup: illegal Hosted grant fixtures; use exact linked local membership plus an unrelated account. Remove retired Hosted erasure branch that rejected before local erasure; retain all signature/claim/lease/rollback/restart/acknowledgement/owner checks. |
| `hosted.TestClaimCleanupAcknowledgementOutageAndSuccessorIsolation` | Fixed: identity × networking cleanup: illegal Hosted grant fixtures; use exact linked local membership plus an unrelated account. Remove retired Hosted erasure branch that rejected before local erasure; retain all signature/claim/lease/rollback/restart/acknowledgement/owner checks. |
| `hosted.TestClaimCleanupMemberErasureDoesNotRetireOwnerClaim` | Fixed: identity × networking cleanup: illegal Hosted grant fixtures; use exact linked local membership plus an unrelated account. Remove retired Hosted erasure branch that rejected before local erasure; retain all signature/claim/lease/rollback/restart/acknowledgement/owner checks. |
| `hosted.TestClaimCleanupCancelledLeaseCannotEraseOrRetire` | Fixed: identity × networking cleanup: illegal Hosted grant fixtures; use exact linked local membership plus an unrelated account. Remove retired Hosted erasure branch that rejected before local erasure; retain all signature/claim/lease/rollback/restart/acknowledgement/owner checks. |
| `hosted.TestRootRevocationLeavesLocalFamilies` | Fixed: identity × networking: impossible Hosted family fixture; use local Portico admission with signer provenance and a password family, assert only the signer-dependent family ends. |
| `httpapi.TestSignInWithoutInstallationHeaderReturnsRefreshBinding` | Fixed: identity PIN rules × PIN recovery: UPDATE named removed pin_attempts/pin_locked_until columns; remove assignments, keep attempt/trust erasure and revocation. |
| `httpapi.TestBackgroundPauseAndResourceUsageOwnerContracts` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestConversionSettingsTypedContractAndOwnerAccess` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestDeliveryPolicyRouteResolvesTheViewersLane` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestDeliveryPolicyRouteRefusesOptionsAndAnonymousReads` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestTranscodeCapacityRouteRequiresOwner` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestLibraryDiscoveryAndCollectionPermissions` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestTypedPINRecoveryAndResetAuthorityAndContract` | Fixed: identity PIN rules × PIN recovery: same stale reset SQL; corrected without restoring lockout. |
| `httpapi.TestEveryRouteSurvivesHostileInput` | Fixed: metadata × catalogue: oversized malformed public ID reached SQL; decode before query and return absent-title 404. |
| `httpapi.TestViewerRestrictionCacheUsesAuthorityQualifiedProfile` | Fixed: identity × HTTP restriction cache test: retired authority must be refused rather than treated as unrestricted; assert unauthorized and no active borrowed policy. |
| `httpapi.TestManualMetadataTransactionAuthorizationPredicates` | Fixed: identity × manual metadata guard: schema now refuses storing Hosted authority; assert storage refusal and forged-principal refusal, retain every other stale-session mutation. |
| `httpapi.TestStorageOwnerProjectionReceiptsAndClock` | Fixed: review-ops × HTTP storage test: fake mount start consulted real missing FUSE; exercise universally supported stop receipt and conflicting replay; unsupported start now returns caller-input 400, with a separate no-mutation guard. |
| `httpapi.TestHostedOwnerCanReadAdministrationButMemberCannot` | Fixed: identity × administration test: Portico Account session is local membership authority; seed linked local rows and preserve owner/member and transaction boundaries. |
| `httpapi.TestHostedOwnerRechecksPolicyAndFamilyInsideTransaction` | Fixed: identity × owner guards: legacy policy mutations no longer govern local membership; migrate tests. Deeper probe found manual metadata omitted live role/disabled/deleted/primary checks; add all predicates. |
| `httpapi.TestHostedOwnerLateRevocationDropsReadResponse` | Fixed: identity × owner guard: mutate the current membership instead of legacy policy; still drop the response after late revocation. |
| `httpapi.TestHostedOwnerSettingsChangePersistsAndDowngradeBlocksFurtherWrites` | Fixed: identity × owner settings test: downgrade actual membership; the old owner session now answers 401 because its live role changed, with no write. |
| `httpapi.TestPeopleBatchAndCollectionRoutesAreAuthorizedAndBounded` | Fixed: identity × personal state: update the session fixture; deeper test found empty/oversized batches returned ordinary errors (500). Classify as caller input (400), preserve 1–200 bound. |
| `httpapi.TestPlaybackAuthorityUsesOneTransactionForSessionAndItemPolicy` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestPlaybackAuthorityRejectsCachedRestrictionAndPolicyChanges` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestPlaybackAuthorityFailsClosedWithoutDependenciesAndOnCancellation` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestPlaylistHTTPShareNeverGrantsMedia` | Fixed: identity × playlists: local linked session and share target required. Reject retired share authority before SQL and return caller-input errors; add valid-owner 400 assertion, retain hidden media/order/CAS/sharing checks. |
| `httpapi.TestPreferencesPublishRegistryAndPatchSemantics` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestSearchHistoryRecordsAndRespectsThePreference` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestReadPathGoldenEquivalence` | Fixed: grouping-sorts × read-path golden: new capability document, two newly supported year sorts and catalogue revision advance; review all 422 cases and record the intentional differences. |
| `httpapi.TestRecommendationFeedbackRoutes` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestRelatedMoviesDetailPermissionAndRevocation` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestRecordingOwnerDirectoryCurrentMembershipAndPagination` | Fixed: identity × DVR administration: legacy Hosted candidates produced a cursor the local-only validator rejected; list only current local profiles (including linked accounts), preserve pagination and all bounds. |
| `httpapi.TestSearchAuthorizationAndRevocation` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `httpapi.TestShowWorkspaceHTTPPermissionAndParentResolution` | Fixed: identity × this HTTP domain: fixture minted retired Hosted authority; install linked local account/profile/membership, and apply current local library/profile/session changes while preserving access, restriction and revocation assertions. |
| `identity.TestDeviceControlCharactersAndLiveFamilyContract` | Fixed: registry × identity schema guard: old YAML document removed; validate device rows against the generated JSON device response schema. |
| `identity.TestDirectProfileErasureRemovesOnlyExactRecordingGrant` | Fixed: identity × erasure guard: schema forbids Hosted recording grants; test two local accounts, delete only the exact profile grant, retain inheritance and other-account grants. |
| `identity.TestPINRecoveryUsesAccountSessionAndRetiresTrust` | Fixed: identity PIN rules × PIN recovery: same removed-column UPDATE; corrected, retaining account-session authority and trust retirement. |
| `identity.TestReauthorizeTxRejectsStalePrincipalAndAccountChanges` | Fixed: identity × transaction guard: impossible stored Hosted authority; assert schema refusal and reject a forged Hosted principal, retain all other stale mutations. |
| `identity.TestReauthorizeTxCancellationAndHostedIdentityBoundary` | Fixed: identity × transaction guard: IssueTx also forbids retired authority; assert both issuance refusals, and check valid local reauthorization, cancellation and wrong-server refusal. |
| `localmetadata.TestAudioSidecarsRejectExternalEntitiesDepthAndWrongKinds` | Fixed: review-scan × local metadata guard: bounded oversized text is deliberately salvaged; assert 4096-byte truncation and partial marker, retain entity/directive/depth/root/kind rejection. |
| `mounts.TestCommandReceiptsReplayCASAndRemovalRecovery` | Fixed: review-ops × mounts: fake mounting executable needs simulated host support; inject a private predicate defaulting to the real check, preserve receipt/CAS/removal recovery. |
| `mounts.TestReceiptRestartUsesOneOwnedReplacement` | Fixed: review-ops × mounts: same simulated support fixture; restart still replaces exactly one owned process. |
| `persistence.TestNoProductionSQLNamesADroppedCatalogueTable` | Fixed: grouping-sorts × persistence guard: albums/items CTE names matched dropped table names; rename CTEs without changing queries or allow-listing tables. |
| `persistence.TestProductionStatementsPrepareAgainstTheBaseline` | Fixed: identity PIN rules × persistence guard: reset names removed columns; corrected SQL prepares against unchanged baseline. |
| `playback.TestDeliveryCopyRemuxSelection` | Fixed: review-playback-pipeline × quality: incomplete geometry reduced fixed conversion height to zero; use existing bounded fallback geometry before applying device and quality limits. |


Additional failures reached after fixing the first blockers:

| Failure | Cause and change |
|---|---|
| Settings-consumer test did not compile; server vet | First base-health × review-scan added the same `monitorChanges` map key twice. Removed the weaker duplicate and retained the precise runtime JSON-path witness. |
| `operations.TestFeedbackCapabilities` | Identity × operations: no-op fixture authorization could not simulate membership revocation; assert actual authorization-boundary refusal, while old restrictions do not interfere with valid local profiles. |
| `downloads.TestPreparationReadsConcealRestrictedTitle` | The new common live account seed collided with an old per-test duplicate. Reproduced alone; removed only the duplicate seed. |
| Updated show-workspace fixture panicked on empty members | A removed member list needs a disable operation rather than indexing member zero; fixed that test helper. |
| `catalog.TestPlaylistOrderRightsSnapshotAndCAS` and `TestPlaylistOccurrencesPermissionsCASAndRestart` | New local-only share validation exposed Hosted actors in old catalogue fixtures. Reproduced alone; use valid local editor/viewer actors and retain a Hosted impostor in cross-authority probes. Capacity fixture also uses local targets. |
| Release-tag HTTP test compilation | Ten fixtures called development-only `Hosted.Service.Apply`. Their policy-shaped descriptions now seed fixture data directly, then establish actual local memberships. These tests exercise media/owner authority; real signed admission/certificate/revocation remains covered by `hosted`. |
| Release test startup without linker root | Intentional release fence, not a defect: supplied the existing official **public** root from `portico-internal/deployment/hosted/root-public-key.pem` through `hostedtrust.OfficialRoot`. No key/fence changed. Release-plus-devtrust absence checks pass. |
| Post-change web TypeScript | Newly registered optional `setTracks` request omitted meaningful null subtitles. Declared its string-or-null schema; TypeScript and full web check pass. |

The initially mentioned historical failures (both restriction matrices, OpenAPI coverage, administration tiers/settings, transaction gate, subtitle class, discovery, listening, delivery decision matrices, indexed container order and transport seek) were checked by the current full runs; do not treat the old brief list as new failures.

## 4. API changes

Existing production routes keep their request/response shapes and remain typed on the registry. Production generation has no drift.

| Route/behavior | Before | Now |
|---|---|---|
| `GET /v1/items/{id}/metadata/manual`, malformed public ID | 500 from SQL | 404, same as absent title |
| Manual metadata mutation, owner downgraded/disabled or profile deleted | Transaction query could retain stale owner permission | Live membership and primary-profile predicates refuse it |
| `PUT /v1/items/personal-state:batch`, zero or more than 200 items | 500 ordinary error | 400 caller-input error; same bound |
| Playlist share/unshare, retired authority | 500 or schema write attempt | 400 before SQL; valid target is local |
| Saved-resource share target, retired authority | 500 ordinary error | 400 caller-input error |
| `GET /v1/admin/dvr/recording-owners` | Included retired Hosted candidates; cursor could reject itself | Current local profiles only, including linked accounts, with valid bounded cursors |
| Storage mount start/restart on an unsupported host | 500 ordinary error | 400 caller-input error; no intent mutation |
| `GET /v1/dev/e2e`, `POST /v1/dev/e2e/sign-in` | Devtrust did not compile; unregistered handlers | Same development behavior, typed contracts, socket-loopback gate, never release routes |
| Retained client protocols | Six hand-written path allowances, plus profile suffix construction | Generated development/consumer builders; no allowances and no new production handlers |

The delivery reason code stays `no_admissible_delivery_route`; its diagnostic and shared catalogue text now say: “This server cannot prepare a version of this title that this device can play.”

## 5. Schema changes

None. Removed SQL references to columns that no longer exist; did not add them back. `python3 scripts/schema-sync.py` passes and causes no schema/artifact diff.

## 6. Owner or viewer settings added or wired

None. Existing settings, defaults and bounds remain. The `monitorChanges` inventory still proves its existing runtime consumer. Download tests use the actual `allow_downloads` profile switch and membership disable operation.

## 7. Needed from others

- Native/TV/Vega package synchronization must include `packages/contracts/src/development` and `src/optional` along with the current shared-client imports. Only this lane's named server worktree was writable.
- The dormant remote-control protocol and Hosted profile APIs remain unavailable production APIs, as on the starting base. Their requested wire protocols are registered and their client interfaces/behavior retained; choosing to implement or remove those interfaces is later owning-lane product work. This lane does not revive retired Hosted profile authority.
- Other lanes merged after this fixed worktree are for the programme's next combined-base pass. No merge/deployment performed here.

## 8. Test notes for other environments

All Go commands use `GOFLAGS=-p=4`. Full server tests use a freshly built `_program/scratch/playback-test-helper` as `PORTICO_PLAYBACK_TEST_HELPER`. `node_modules` remains the existing symlink; nothing was installed.

- **Windows/macOS:** cross-compilation is verified (Windows amd64, Darwin arm64, CGO disabled), not native execution. On a fresh state directory, reset a secondary profile's PIN with an authorized account session; confirm profile selection works, old trust/session credentials end, and a revoked/downgraded owner cannot edit metadata. Hidden and absent title IDs must answer identically.
- **Metadata/playback:** use a source with incomplete observed geometry and request a fixed quality; output height must remain positive and within the device/quality ceilings. With a server lacking Dolby Vision reshaping tools, refusal should describe server/device support rather than permission. Real codecs/GPU/device presentation are not established by these tests.
- **FUSE/macFUSE:** without host support, start/restart refuses with 400 and unchanged intent. With real support, run start/restart/stop and retry the same operation; one intent/owned process and identical receipt should result. Unit fixtures simulate mount support and mount observation; they do not prove FUSE behavior.
- **Portico account cleanup:** signed exact-account erasure removes the linked member, sessions, saved resources and grants, cancels preparation authority, preserves unrelated local data and the recovery owner, and acknowledges only after commit, including after restart. Forged/expired/cross-claim/cancelled-lease requests must change nothing.
- **Development builds:** generate with `GOFLAGS=-p=4 go run -tags devtrust ./cmd/apigen -development`; run peer/contract tests and verify external peers with spoofed forwarded headers are refused. Release-tag tests require the official public root linker setting and must expose neither development route/hook. No production root or signature check was changed.
- No physical tuner, GPU, Apple/Android/Vega device, browser visual or production capacity acceptance is claimed. The test host lacks bubblewrap namespace permission; the existing decoder's documented fallback/bounds remain in force.

## 9. Not finished

Nothing in this lane's implementation or requested suite list remains unfinished. The known vendored SQLite vet warnings remain by explicit scope. Native package synchronization, actual platform/runtime acceptance and deliberately unavailable dormant APIs are the follow-ups in sections 7–8. They are not claimed as deployed or implemented APIs.

## 10. Final command results

Logs: `_program/scratch/base-health-2/final/`. Earlier failing runs and isolated reproductions remain under `initial/`; interim full-run logs are retained under descriptive names in `final/`. The progression log/report is preserved as `scratch/base-health-2/report-progress.md`.

| Command | Latest result |
|---|---|
| Playback helper build | Pass, exit 0 |
| Server `go vet ./...` | Exit 1: only the 63 known vendored SQLite unsafe.Pointer findings; no Portico findings |
| Server `go test -count=1 ./...` | Pass, exit 0: 85 packages ok, 18 with no tests, 0 failures; both named load-sensitive startup tests passed |
| apikit `go test -count=1 ./...` | Pass, 4 tested packages |
| `npm run test:core` | Pass, 1848/1848 |
| `npm run check --workspace portico-web` | Pass, exit 0, 512/512; existing pending lint notices only |
| `npm run test:cast` | Pass, 20/20 |
| Server Linux `go build ./...` | Pass |
| Server `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...` | Pass, exit 0 |
| Server `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` | Pass, exit 0 |
| Server `go build -tags devtrust ./...` | Pass |
| Devtrust `httpapi`, `identity` tests matching `DevE2E` | Pass |
| Release + devtrust `DevE2E` absence tests, with official public root linked | Pass |
| Production `go run ./cmd/apigen` | Pass, no production artifact diff; 622 operations |
| Development `go run -tags devtrust ./cmd/apigen -development` | Pass; 2 operations generated separately |
| `go run ./cmd/clientcontractgen` | Pass; 12 retained consumer protocols generated separately |
| `python3 scripts/schema-sync.py` | Pass, persistence checks pass, no schema/artifact diff |
| `git diff --check` | Pass |

Cumulative snapshots: `003` through `007` are complete root-level snapshots; `001`/`002` were taken from `server/` and are superseded. Final snapshot `007` captures the finished verified tree. No worktree changes follow it.

Final full-server run used the finished tree captured in snapshot `006`; snapshot `007` is the identical final delivery snapshot after report completion. The earlier full logs are retained as `server-test-before-last-fixes.log` and `server-test-before-playlist-fixtures.log`. No code was changed during the final full run.
