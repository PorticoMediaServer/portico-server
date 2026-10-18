# Portico public alpha and beta launch readiness

> Remediation update, 18 October: all 19 findings now have source fixes and focused verification. The final Hosted PostgreSQL, HTTP and identity package run passes; independent review accepted the authentication corrections. Full release qualification remains incomplete: the mandatory full server race run did not finish, and broader test failures/limits are recorded in the remediation section at the end of this file. React Native changes remain isolated on branch `fix/launch-readiness-20261018` at `/home/justin/projects/Portico/.worktrees/launch-readiness-native-20261018`; the active native checkout is untouched. Nothing has been deployed or published. The original assessment below is retained as the before-fix baseline.

Assessment date: 18 October 2026. Scope: Hosted Services, the local server on Windows, macOS, Linux and Docker, the web UI, and the Apple and Android clients in the React Native repository.

**Original assessment recommendation, before remediation: hold the public launch in its current form.** Concrete defects affect recovery-account security, download revocation, Windows startup, native music and audiobook playback, and the default server-web account connection. The documented release pipelines also have independent failures. An alpha label reasonably accommodates the P2 functionality issues below; it does not resolve these P1 failures.

This assessment concerns the current source and release configuration. It does not equate the deployed demo, existing testing APKs or historical audit results with the release candidate. Missing Apple end-to-end or physical-device testing is not counted as a finding or launch blocker.

There are **19 retained findings: nine P1 and ten P2**. Hosted's own review found no additional P1; its remaining items concern abuse resistance and authentication hardening. The Apple audio blocker was established during cross-client reconciliation even though the Apple-specific review alone did not identify a P1.

## How to read the findings

- **P1:** fix before public distribution of the affected feature/platform. Release-pipeline findings block the documented production path; a separately produced, verified artifact could bypass a pipeline defect without fixing it.
- **P2:** a concrete beta defect or conditional security concern. Each finding states the workaround or remaining exposure so it can be accepted deliberately.
- **Test demonstrated:** exercised against the real implementation in a disposable fixture or adapter. This does not imply a physical-device or production-network reproduction.
- **Source established:** the implementation and its relevant platform/API contract establish the failure path. Platform-dependent assumptions are stated beside the finding.

No P0 finding has been established. The security findings below describe their required access; none is presented as an unauthenticated takeover of an already configured server.

## Revisions and concurrent work

Paths in this audit refer to the active workspace on ehler-homelab-mini-1, the personal Ubuntu host at /home/justin/projects/Portico.

| Repository | Starting revision | Revision used for final source reconciliation |
| --- | --- | --- |
| apps/portico-server | f6878ed1bfcf3af5872aa87dfc2f3d51ce9f5d2c | f6878ed1bfcf3af5872aa87dfc2f3d51ce9f5d2c |
| apps/portico-react-native | e4ce4ec720524876a3bbefb0c60d28ca87b2e2d3 | ec1d94145cecc7bee947fb3540cb6be9dcb10c40 |
| portico-internal | 981137e2b669ad0876c75f94b03e6dada79c0e51 | 981137e2b669ad0876c75f94b03e6dada79c0e51 |
| getportico-tv | 213da50307c867be36224aec8bfb5e7b932784c3 | 213da50307c867be36224aec8bfb5e7b932784c3 |

Concurrent native commit ec1d941 changed SavedServerUnavailable.tsx and CertificateNotice.tsx. It did not change the cited native findings. The canonical client packages and their native mirrors were byte-identical to server revision f6878ed1, including after that native commit. The Hosted and server apikit copies also matched.

## Findings at a glance

| ID | Priority | Defect | Release implication |
| --- | --- | --- | --- |
| SEC01 | P1 | Recovery locality is bypassed by direct account operations and refresh | Fix on all local-server distributions |
| SEC02 | P1 | Revoked downloads keep delivering an already open response | Fix before releasing downloads enabled |
| RUN01 | P1 | Directory synchronization prevents Windows startup | Windows release blocker |
| RUN02 | P1 | Native audio snapshots are rejected by the shared bridge | Apple and Android music/audiobook blocker |
| RUN03 | P1 | Server-web builds use the development Hosted origin for claim validation | Default web setup and secure-connection blocker |
| REL01 | P1 | Sealed server snapshot omits inputs required by mandatory tests | Official server release path fails |
| REL02 | P1 | Native CI assembles Android without its mandatory media engine | Native release gate fails on a clean runner |
| REL03 | P1 | Android artifact collection uses obsolete unflavored directories | Android release collection fails independently |
| DATA01 | P1 | Backup manifest omits the database checksum | Fix before relying on verified backup recovery |
| BETA01 | P2 | Default update feed cannot follow GitHub asset redirects | Manual release-page checks remain available |
| BETA02 | P2 | Claim UI times out after one minute of a ten-minute approval | Recoverable onboarding failure and misleading status |
| BETA03 | P2 | Administrator library visibility contradicts the tier policy | Explicit grants work around existing libraries |
| BETA04 | P2 | Administrator library/metadata management remains owner-only | Owner must perform the affected operations |
| BETA05 | P2 | Native profile pictures and upload bypass verified routing | Broken on alternate routes; inconsistent security boundary |
| BETA06 | P2 | iOS background download relaunch does not recreate its session | Completion handling waits for foreground initialization |
| SEC03 | P2 | First-run browser setup trusts arbitrary Host/Origin pairs | DNS-rebinding exposure before owner setup |
| HOST01 | P2 | One IPv6 delegation can occupy the Hosted claim-stream pool | Conditional onboarding denial of service |
| HOST02 | P2 | Second-factor guesses can continue on a replenishing budget | Hardening needed after first-factor compromise |
| HOST03 | P2 | Password cool-off distinguishes existing usernames | Limited username enumeration |

## Day 1 platform impact

| Release surface | Current assessment |
| --- | --- |
| Hosted API, web host and DNS | No additional P1 established in Hosted itself; HOST01 through HOST03 remain P2. The server-web integration failure is RUN03. |
| Windows local server | Blocked by RUN01 in addition to shared server security, backup and release-build findings. |
| macOS local server | Shared SEC01, SEC02, DATA01 and REL01 apply. No separate macOS-specific runtime P1 was established. |
| Linux local server | Shared SEC01, SEC02, DATA01 and REL01 apply. No separate Linux-specific runtime P1 was established. |
| Docker local server | Shared server findings apply; the bundled web has RUN03, and official publication depends on the release pipeline. |
| Web UI | Default bundled setup/settings claim is blocked by RUN03; BETA02 remains after its origin is corrected. Backend security findings apply to web users too. |
| iPhone, iPad and Apple TV clients | RUN02 blocks native audio-render listening. The native release also depends on the checks affected by REL02. Apple-only BETA06 concerns iPhone/iPad background downloads. |
| Android phones/tablets, Android TV and Fire TV | RUN02 blocks native listening; REL02 and REL03 independently break documented automated release production. |

## Security defects requiring fixes

### SEC01 Recovery locality is bypassed by direct account operations and refresh

**P1. All local-server platforms. Test demonstrated through the real HTTP router.** The owner can disable remote recovery, and ordinary viewer requests enforce that restriction, but direct account operations and refresh do not apply the same check.

The intended restriction is in [onboarding.go](/home/justin/projects/Portico/apps/portico-server/server/internal/identity/onboarding.go:215). Ordinary authentication calls CheckRecoveryRoute. The separate [directCallerTx authority path](/home/justin/projects/Portico/apps/portico-server/server/internal/identity/direct_accounts.go:340) validates family/account/profile management authority without checking recovery locality. [Refresh](/home/justin/projects/Portico/apps/portico-server/server/internal/identity/refresh.go:72) also rotates the recovery family without that rule; [direct HTTP dispatch](/home/justin/projects/Portico/apps/portico-server/server/internal/httpapi/direct_identity.go:115) reaches the bypass.

A disposable router fixture used Hosted onboarding state, a local recovery owner, and remote_recovery=0. The same bearer produced these results:

| Peer and operation | Result |
| --- | --- |
| Home-network GET /v1/me | 200 |
| Remote GET /v1/me | 401 |
| Remote GET /v1/direct/members | 200 |
| Remote POST /v1/direct/profiles with valid synthetic profile | 201 |
| Remote POST /v1/direct/members with valid synthetic account | 201 |
| Remote POST /v1/auth/refresh with its refresh credential | 200 |

The parent reran the reproduction and observed the same committed operations. A credential intended to be unusable remotely can enumerate members, create persistent account/profile changes and renew remotely. This requires possession of an existing recovery bearer or refresh credential; the test does not establish password-free or unauthenticated access.

**Remediation:** apply the transaction-capable recovery-locality check to direct account authority, refresh and refresh replay, using verified request locality. Preserve the intentional exemption for families admitted through Hosted. Verify identical locality outcomes across ordinary viewer, account-management and renewal routes.

### SEC02 Explicit download revocation does not stop an open transfer

**P1. Windows, macOS, Linux and Docker. Test demonstrated through the real HTTP router.** The [download transfer handler](/home/justin/projects/Portico/apps/portico-server/server/internal/httpapi/downloads_routes.go:355) gives an unguarded reader to http.ServeContent. OpenGrant checks current authority before opening the response, but subsequent reads do not recheck grant revocation or current permissions.

The reproduction served a 262,144-byte synthetic original. Immediately after the first 32 KiB write, the fixture completed the real POST /v1/admin/downloads/revoke route. It returned 200 with claims=1, receipts=0 and more=false. The already open response then delivered **229,376 additional bytes**, finishing the entire file. A new request using that grant returned 401. Disabling allow_downloads between writes similarly allowed the active response to finish while denying a new request with 403. The parent independently reran the explicit-revocation case.

This is continued acquisition after a successful revocation, not an attempt to reclaim bytes already copied. On a large or throttled transfer, the remaining response can outlive the owner's withdrawal substantially. Normal playback readers already have a continuing authority guard; the download path lacks it. The revocation mutation is in [profile_revoke.go](/home/justin/projects/Portico/apps/portico-server/server/internal/downloads/profile_revoke.go:54).

**Remediation:** use a guarded download reader that checks current grant/claim/profile authority during transfer, with the existing authority-generation invalidation pattern to keep the common case cheap. Close and stop the reader on revocation, cancellation or applicable permission loss. Verify revocation between successive writes, including Range responses.

## Core runtime and onboarding blockers

### RUN01 Windows cannot complete normal server startup

**P1. Windows x64 and ARM64. Source established and independently checked against Go and Windows API semantics; not executed on Windows.** Several untagged directory-sync implementations open a directory read-only and call File.Sync. On Windows this reaches FlushFileBuffers, which requires a handle with GENERIC_WRITE. The current os.Open handle does not have that access. See the [Microsoft API contract](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-flushfilebuffers).

On a fresh install, the first failure is [networking/claim_keys.go](/home/justin/projects/Portico/apps/portico-server/server/internal/networking/claim_keys.go:27). OpenCurrentKeys writes and flushes a new private key, then [synchronizes its directory](/home/justin/projects/Portico/apps/portico-server/server/internal/networking/claim_keys.go:218). [Startup returns the error](/home/justin/projects/Portico/apps/portico-server/server/cmd/server/main.go:430). The database transaction rolls back while the key file remains, so the next attempt encounters a nonempty unmarked key directory and refuses it. The predicted access-denied error and retry state follow from source, rather than a Windows observation.

Existing valid identity state does not avoid the blocker. [mediaartifact.New](/home/justin/projects/Portico/apps/portico-server/server/internal/mediaartifact/store.go:119) always synchronizes its directories and ancestors using the same [read-only sync implementation](/home/justin/projects/Portico/apps/portico-server/server/internal/mediaartifact/store.go:270). [Subtitle storage initialization](/home/justin/projects/Portico/apps/portico-server/server/internal/subtitles/service.go:63) is mandatory and [fatal on error](/home/justin/projects/Portico/apps/portico-server/server/cmd/server/main.go:603). Prepared media, analysis and recording stores also depend on mediaartifact. Runtime object publication repeats the same operation.

The repository already has Windows-specific directory-sync handling in backup, atomicfile and persistence. This is therefore a concrete inconsistency in mandatory paths, not a request to block release merely because Windows was not tested end to end.

**Reproduction target:** on Windows, construct these stores against a disposable empty state directory, or start the server with that directory. Expect failure at claim-keys; with valid preexisting identity, expect failure at artifact-store initialization. Do not delete keys from a real installation to work around this.

**Remediation:** introduce appropriate platform-specific directory durability handling consistently across networking, mediaartifact and the other affected publishers. Retain file flushing and atomic publication guarantees. Handle a failed fresh identity creation without leaving an unrecoverable orphan. Verify a fresh start, restart and object publication on Windows.

### RUN02 Native music and audiobook snapshots fail the shared bridge validator

**P1. Android phones/tablets, Android TV, Fire TV, and Apple native audio-render paths. Test demonstrated in the real TypeScript parser/adapter; native producers and wiring source established.** The native engines advertise speeds through 3×, but [NativeListeningAdapter](/home/justin/projects/Portico/apps/portico-react-native/apps/native/src/bridge/NativeListeningAdapter.ts:35) rejects more than seven supported rates, rejects 2.5 and 3 in that list, and rejects effectiveRate above 2.

Android [publishes RATES in every listening snapshot](/home/justin/projects/Portico/apps/portico-react-native/apps/native/android/app/src/main/java/tv/getportico/portico/native/PorticoPlaybackController.kt:1098). Its [RATES constant contains nine values](/home/justin/projects/Portico/apps/portico-react-native/apps/native/android/app/src/main/java/tv/getportico/portico/native/PorticoPlaybackController.kt:1412). Apple's [PorticoAudioRender snapshot](/home/justin/projects/Portico/apps/portico-react-native/apps/native/Native/PorticoAudioRender.m:861) publishes the same nine. The AVPlayer controller can also append 2.5/3 when fast-forward is supported. [useNativePlayback](/home/justin/projects/Portico/apps/portico-react-native/apps/native/src/bridge/useNativePlayback.ts:133) installs the shared adapter for music and audiobooks.

The Android review exercised parser and adoption with an otherwise valid snapshot. Adoption rejected and retired the native listening owner. The parent also extracted the rate arrays directly from both current producers and passed them to the real parser:

    Seven-speed control: accepted
    Android Kotlin rates [0.5,0.75,1,1.25,1.5,1.75,2,2.5,3]: Invalid native supported rates.
    Apple AudioRender rates [0.5,0.75,1,1.25,1.5,1.75,2,2.5,3]: Invalid native supported rates.

This happens at **1× playback** because the rejected field is the complete capability list. Once a snapshot is rejected, adoption fails or the event failure path detaches/stops its owner. Merely removing 2.5/3 from the menu would not fix the current producer/parser disagreement. The existing listening test snapshots omit supportedRates, explaining why they pass.

**Remediation:** align native producers, parser bounds, effective-rate validation, types and speed controls. Preserve the music-versus-audiobook policy where applicable. Test an actual producer-shaped snapshot at adoption, readiness and 2.5×/3× operation, rather than a fixture omitting the capability field.

### RUN03 Release server web builds reject the real Hosted approval page

**P1. Web UI shipped with server packages and Docker. Source established, with the actual approval validator exercised in Node.** [browserAccount](/home/justin/projects/Portico/apps/portico-server/web/src/bridge/account.ts:128) falls back to the development account origin http://127.0.0.1:19410 unless VITE_HOSTED_URL is set, the account-workspace build flag is set, or the page is on web.getportico.tv.

The [Docker web build](/home/justin/projects/Portico/apps/portico-server/Dockerfile:51) and [official release web job](/home/justin/projects/Portico/apps/portico-server/.github/workflows/release.yml:102) do not set VITE_HOSTED_URL. The release server itself correctly uses https://web.getportico.tv and returns an approval URL there. [Setup](/home/justin/projects/Portico/apps/portico-server/web/src/screens/auth/ServerSetup.tsx:120) and [settings claim connection](/home/justin/projects/Portico/apps/portico-server/web/src/app/claim-connect.ts:31) validate that URL against session.hostedUrl, which comes from the incorrect browser account origin.

The exact shared validator rejects the production approval URL when the expected origin is http://127.0.0.1:19410 and accepts it when the expected origin is https://web.getportico.tv. Consequently, default Hosted setup creates the recovery owner, then fails the account connection with claim_unavailable; Retry repeats the failure. Connect/Secure connection in server settings fails the same way. Local sign-in and the option to connect later preserve access to the installation.

The private demo build explicitly supplies the production Hosted URL, so a working demo does not cover this release-build difference. The Hosted website and native claim paths are not implicated by this defect.

**Remediation:** derive the account/claim authority consistently from supported production configuration, preferably the server's explicit Hosted authority, and wire both Docker and package builds accordingly. Keep exact approval-origin validation. Verify the actual clean release build's origin and its setup/settings claim path.

## Release production failures

### REL01 The isolated sealed server build omits mandatory test inputs

**P1. Official Windows, macOS and Linux packages; dependent Docker publication. Test demonstrated against the builder's snapshot layout.** [build-server-release.py](/home/justin/projects/Portico/apps/portico-server/scripts/build-server-release.py:128) snapshots the server module and apikit, then [runs the mandatory server tests](/home/justin/projects/Portico/apps/portico-server/scripts/build-server-release.py:150). Tests now depend on repository siblings that are absent from that snapshot.

In a disposable copy containing precisely those modules, TestPublishedFeedBytesDriveUpdateNotice failed because scripts/write-public-update-manifest.mjs was absent, and TestRecoveryCopyMatchesStringCatalogue failed because packages/i18n/src/catalog/en-US/settings.ts was absent. Both tests passed in the complete checkout. The dependencies are explicit in [public_feed_test.go](/home/justin/projects/Portico/apps/portico-server/server/internal/administration/public_feed_test.go:16) and [startup_recovery_test.go](/home/justin/projects/Portico/apps/portico-server/server/cmd/server/startup_recovery_test.go:50). [Every server release target invokes this builder](/home/justin/projects/Portico/apps/portico-server/.github/workflows/release.yml:165).

No complete signed release build was executed. The narrower reproduction establishes missing mandatory inputs independently of signing, race instrumentation or platform qualification.

**Remediation:** capture and digest all repository inputs needed by the mandatory snapshot tests, preserving their expected layout. Keep the tests mandatory and qualify the actual isolated layout; running them only in the full working tree would continue to conceal the failure.

### REL02 The native release gate assembles Android without the media engine

**P1. Clean-runner native release workflow, including jobs gated by its checks. Source established.** The [Android CI job](/home/justin/projects/Portico/apps/portico-react-native/.github/workflows/ci.yml:79) installs Java/Node packages and runs assembleDebug, but never builds or restores the generated media engine. Those libraries, headers and notices are not tracked in the checkout.

The mpv Gradle module's [mandatory check](/home/justin/projects/Portico/apps/portico-react-native/apps/native/android/portico-mpv/build.gradle:45) throws when the files are absent, and [preBuild depends on their staging](/home/justin/projects/Portico/apps/portico-react-native/apps/native/android/portico-mpv/build.gradle:121). The [release workflow invokes this CI workflow first](/home/justin/projects/Portico/apps/portico-react-native/.github/workflows/release.yml:31). Building the engine in a later release job cannot populate the earlier job's filesystem. A developer's existing local engine output can conceal this failure.

**Remediation:** build or restore a verified engine artifact, with its NDK and ABI set, before Android CI assembly/tests. Verify on a clean runner. This finding concerns release production, not the runtime validity of a separately built testing APK.

### REL03 Android release collection searches directories that flavors no longer produce

**P1. Google Android/Android TV and Fire TV automated release output. Test demonstrated for the collection commands.** The app has google and firetv product flavors, so APK output is under apk/google/release and apk/firetv/release; bundles likewise use flavor-specific variants. The [release Collect step](/home/justin/projects/Portico/apps/portico-react-native/.github/workflows/release.yml:57) instead searches apk/release and bundle/release.

The current local output layout confirms the flavor structure. Repeating the workflow search commands against representative disposable flavored APK/AAB trees returned exit status 1, reported nonexistent directories and collected no files. Thus collection fails even after REL02 is corrected and the flavor builds succeed.

**Remediation:** collect the explicit intended variants and assert the required APK/AAB set before upload. Existing development signing is a documented manual-distribution gate and is not counted again as a newly discovered defect.

## Backup integrity

### DATA01 Ordinary backup manifests do not cover the database bytes

**P1 before relying on advertised verified recovery. All server platforms. Test demonstrated and independently rerun.** Backup creation [copies and scrubs portico.db, then builds the manifest from copyStateFiles only](/home/justin/projects/Portico/apps/portico-server/server/internal/backup/backup.go:483). The [state-file list](/home/justin/projects/Portico/apps/portico-server/server/internal/backup/files.go:22) contains companion state, not the database. Consequently manifest.Files has no database size or SHA-256 entry.

Restore [checks the listed manifest hashes](/home/justin/projects/Portico/apps/portico-server/server/internal/backup/stage.go:280), then [inspects SQLite/schema integrity](/home/justin/projects/Portico/apps/portico-server/server/internal/backup/stage.go:139). A valid SQLite text payload can change without breaking structural integrity.

The disposable reproduction used the production session-scrubbing hook and a unique synthetic configuration value. After making a manual backup, it changed one database byte from AUDIT_ORIGINAL_VALUE to BUDIT_ORIGINAL_VALUE without changing the manifest. Manifest validation and restore staging succeeded:

    one-byte data corruption accepted: staged=true integrity=ok
    marker=BUDIT_ORIGINAL_VALUE; database absent from manifest.Files

The parent reran the same fixture with the same result. This does not show Portico spontaneously corrupting data, an installed-service restore, or authenticity against someone who can rewrite both database and manifest. It establishes that the principal backup payload lacks the integrity protection owners are led to rely on. Scheduled and manual backups share this path; pre-restore safety copies have a separate path.

**Remediation:** hash the finalized, scrubbed and checkpointed database into the ordinary backup manifest and require its entry during validation. Handle any required WAL consistently. Verify that payload corruption which still passes SQLite integrity checks is rejected before staging.

## Other concrete beta defects

### BETA01 The default update feed conflicts with the redirect policy

**P2. All server platforms. Source established, supported by redirect-policy tests and observed GitHub asset behavior.** The [default update URL](/home/justin/projects/Portico/apps/portico-server/server/internal/operations/settings.go:137) uses GitHub releases/latest/download. [FetchReleaseFeed](/home/justin/projects/Portico/apps/portico-server/server/internal/administration/update_feed.go:39) approves only that initial origin, and the [remote-media policy](/home/justin/projects/Portico/apps/portico-server/server/internal/remotemedia/policy.go:275) rejects a redirect to another origin. GitHub normally delivers release assets through a redirect to release-assets.githubusercontent.com; see [GitHub's release asset documentation](https://docs.github.com/en/rest/releases/assets).

A read-only request to an existing public GitHub asset observed that redirect, and the repository's cross-origin rejection test passed. The exact Portico manifest returned 404 during the review, so a successful Portico fetch was not observed; the 404 is not counted separately. Publishing the asset alone would leave the policy mismatch.

**Disposition:** manual release-page checks are a beta workaround. Serve the feed directly at a stable origin or implement a narrow HTTPS GitHub-asset policy while retaining DNS, metadata-address, credential and downgrade protections. Do not describe update checking as functioning until that path works.

### BETA02 The web claim UI gives up before the approval expires

**P2. Web setup/settings; the shared transport also underlies native retries. Test demonstrated with the real transport and an accelerated timer.** [requestDeadlineMs](/home/justin/projects/Portico/apps/portico-server/packages/client-core/src/index.ts:174) assigns the claim await POST a 60,000 ms deadline. [awaitWebClaim](/home/justin/projects/Portico/apps/portico-server/packages/client-core/src/claim-web.ts:58) uses that ordinary request path. The server's approval lifetime is [ten minutes](/home/justin/projects/Portico/apps/portico-server/server/internal/networking/claim_web.go:134), and its watch intentionally continues after a waiting page disconnects.

The parent exercised awaitWebClaim through HttpLocalApi with a held abort-aware transport, accelerating only the 60,000 ms timer. It returned TimeoutError/request_timeout at that deadline. The [web binding](/home/justin/projects/Portico/apps/portico-server/web/src/app/claim-connect.ts:36) then displays an error and leaves its waiting phase. A person who takes longer than one minute to sign in/approve can see failure while the server is still processing the valid approval. Native setup has bounded reattachment retries, so its exact experience differs.

**Disposition:** recoverable through retry/reconciliation or connecting later, but likely to confuse fresh users. Give this await operation its documented lifetime or reconnect/reconcile it until the actual decision/expiry. This is independent of RUN03: correcting the authority origin does not fix the timeout.

### BETA03 Administrators do not receive the promised library visibility

**P2. All server platforms and clients. Test demonstrated through the router and independently rerun.** The [tier policy](/home/justin/projects/Portico/apps/portico-server/server/internal/identity/access_tier.go:43) says administrators see all libraries, but [directLibraryAllowed](/home/justin/projects/Portico/apps/portico-server/server/internal/recordingaccess/local_policy.go:30) recognizes only owner as unrestricted before the main policy is reached.

With two ordinary libraries, the owner saw both; a member granted one saw exactly that library. An administrator with the default empty explicit allow-list received 200 with an empty libraries list and 404 for an otherwise accessible item. This is denial of promised functionality, not an authorization widening.

**Disposition:** explicit grants work around current libraries, but do not provide automatic access to future libraries. Use the shared administrative-tier predicate while retaining profile narrowing and private-recording ownership.

### BETA04 Administrator library and metadata tools remain owner-only

**P2. Principally web/console and API workflows. Test demonstrated for the named reads; mutation predicates source established.** Current tier definitions grant library/metadata management to administrators. [Library administration](/home/justin/projects/Portico/apps/portico-server/server/internal/httpapi/admin.go:17), [manual metadata](/home/justin/projects/Portico/apps/portico-server/server/internal/httpapi/manual_metadata.go:39) and the broader [administration wrapper](/home/justin/projects/Portico/apps/portico-server/server/internal/httpapi/administration.go:137) still require owner authority.

The fixture returned 200 for the owner and 403 for the administrator at GET /v1/admin/libraries, GET /v1/admin/libraries/allowed and GET /v1/items/{id}/metadata/manual. Explicit library grants do not satisfy these owner-only checks, so this is separate from BETA03. The result is not generalized to every administration endpoint.

**Disposition:** the owner can perform the operations during beta. Apply live administrator-tier checks to the operations the tier promises, at request and transaction boundaries; keep ownership, security, backup and other expressly owner-only functions separate.

### BETA05 Native profile pictures and upload bypass verified routing

**P2. Shared Apple and Android screens. Source established; alternate-route device behavior not reproduced.** [Avatar upload](/home/justin/projects/Portico/apps/portico-react-native/apps/native/src/screens/Profiles.tsx:67), [profile settings pictures](/home/justin/projects/Portico/apps/portico-react-native/apps/native/src/screens/Profiles.tsx:137) and [chooser pictures](/home/justin/projects/Portico/apps/portico-react-native/apps/native/src/screens/auth/ProfileAvatar.tsx:41) concatenate api.baseUrl and send the bearer directly. That is the logical origin, which can differ from the currently verified LAN/public route.

When the logical public name is unreachable from the current home network but RouteConnection has recovered a usable LAN route, normal API reads can work while these pictures/uploads fail. Upload also bypasses the API's 401 renewal path. These call sites skip the selected transport's redirect and identity handling, creating a security-sensitive inconsistency, but this audit did not demonstrate cross-host credential theft.

**Disposition:** acceptable as a documented beta functionality defect only with that narrower scope. Resolve image URLs through the verified media route and send uploads through a supported routed upload path with renewal. Merely replacing fetch with a different raw fetch does not add the API's authentication recovery automatically.

### BETA06 iOS background download relaunch does not reconnect the session

**P2. iPhone and iPad. Source established under the background-launch lifecycle; not device reproduced.** [AppDelegate's background-session callback](/home/justin/projects/Portico/apps/portico-react-native/apps/native/ios/Portico/AppDelegate.swift:55) stores the completion handler but does not recreate the background session. React Native starts from [scene connection](/home/justin/projects/Portico/apps/portico-react-native/apps/native/ios/Portico/AppDelegate.swift:82), while the download session is created inside [PorticoDownloads initialization](/home/justin/projects/Portico/apps/portico-react-native/apps/native/Native/PorticoDownloads.m:59).

After the OS terminates a suspended app and relaunches it solely for download events, a foreground scene need not connect. The native module therefore is not initialized to recreate its session, so completion verification, filing and manifest updates wait for foreground initialization. The handler is released only by the session delegate's finish-events callback. Apple's [background download guidance](https://developer.apple.com/documentation/foundation/downloading-files-in-the-background) requires recreating the session with the same identifier during relaunch so the system can reconnect its tasks.

**Disposition:** background completion is incomplete; this audit does not establish data loss. Initialize a native download owner independently of the React Native scene and attach the module to it later. Verify OS-driven termination/relaunch rather than treating a user force-quit as the same lifecycle.

## First-run browser exposure

### SEC03 Bootstrap accepts an arbitrary matching Host and Origin before setup

**P2. Unconfigured local servers. Test demonstrated for server acceptance; DNS-rebinding browser delivery not reproduced.** [sameRequestOrigin](/home/justin/projects/Portico/apps/portico-server/server/internal/httpapi/request_locality.go:62) compares Origin with an origin derived from Host, without an independently trusted hostname constraint. [The browser setup-token route](/home/justin/projects/Portico/apps/portico-server/server/internal/httpapi/setup_transport.go:76) relies on that comparison, and the subsequent setup request accepts the same pair.

An in-process test against the real router sent Host rebind.attacker.test:32500, matching Origin and same-origin fetch metadata, from a LAN peer. POST /v1/setup/browser returned 200; POST /v1/setup returned 201 with an owner access token and owner role. This proves the server accepts the authority pair. A malicious page that can rebind its origin to a fresh server could use that path to create the first owner.

The exposure is limited to the interval before owner setup, requires browser delivery to the server's port, and depends on the browser's private-network and DNS-rebinding controls. The test did not establish a working attack in a particular browser, a takeover of an already configured server, or subsequent Hosted claim. Those qualifications are why this is not promoted to an unconditional P1 exploit.

**Disposition:** fix before a broad public beta if practical. Protect first-run bootstrap with an authority check independent of attacker-supplied Host/Origin, while preserving explicitly supported owner hostnames and trusted proxies. The advice to finish setup before public exposure does not itself prevent a browser-mediated request from the private network.

## Hosted security hardening

### HOST01 Claim streams lack a concurrent limit for the wider client network

**P2. Hosted claim approval and every client that uses it. Source established; no saturation attack was run.** Hosted has a dedicated [global pool of 128 claim streams](/home/justin/projects/Portico/portico-internal/hosted-services/hosted/internal/httpapi/handler.go:82). The [per-peer limit is four](/home/justin/projects/Portico/portico-internal/hosted-services/hosted/internal/httpapi/claim_web.go:32), charged using [budgetIP](/home/justin/projects/Portico/portico-internal/hosted-services/hosted/internal/httpapi/claim_web.go:88), which groups IPv6 addresses by /64. It does not charge concurrent streams against the existing wider /48 network key.

An operator controlling at least 32 /64s inside one /48 can register four pending operations per /64 using freely generated server keys and hold all 128 streams. The three-pending-claims-per-server-key limit does not prevent creating more keys. Stream lifetimes are bounded at approximately ten minutes, but refreshing 128 operations per interval remains within the [minute request budgets](/home/justin/projects/Portico/portico-internal/hosted-services/hosted/internal/httpapi/handler.go:229). Other servers then receive capacity responses instead of joining their approval stream. No account data is exposed.

The store does check that a registered matching claim exists and has not expired before continuing to wait; this finding is about aggregate occupancy, not acceptance of an unregistered claim. An effective deployed edge/WAF concurrency rule could mitigate the application-level exposure; dashboard rules were not verified.

**Disposition:** bound concurrent claim streams by the wider network as well as /64, preserve a usable share of onboarding capacity, and align any edge rule with that limit. This is a conditional abuse concern, not evidence that Hosted is currently unavailable.

### HOST02 MFA permits sustained guessing after first-factor compromise

**P2. Hosted accounts with TOTP enabled. Source established policy/hardening concern; no account attack was run.** [CompleteMFA](/home/justin/projects/Portico/portico-internal/hosted-services/hosted/internal/postgres/account_mfa_login.go:157) permits 20 account-level attempts per fifteen-minute window, and [factorTx](/home/justin/projects/Portico/portico-internal/hosted-services/hosted/internal/postgres/account_security.go:368) applies the same replenishing factor budget. A single challenge is capped at five attempts, but a holder of the correct password can obtain fresh challenges. The [TOTP verifier](/home/justin/projects/Portico/portico-internal/hosted-services/hosted/internal/identity/totp.go:45) accepts a three-step drift window, subject to accepted-step replay protection.

The account cap therefore permits roughly 1,920 attempts per day if the attacker sustains fresh challenges and can use the available request budget. This is a budget calculation, not a measured bypass or a claim that the second factor is absent. It requires first-factor compromise; network limits, challenge expiry, five attempts per challenge and TOTP replay checks remain in force.

**Disposition:** consider an escalating account-level delay or stronger sustained-failure policy that does not let an unauthenticated attacker permanently lock out an owner. Preserve durable attempt charging, recovery-code handling and the existing concurrency fences. No email or consumer-push requirement is introduced by this audit.

### HOST03 Password cool-off reveals whether a username exists

**P2 with limited impact. Hosted password sign-in. Source established; no live enumeration was performed.** [PasswordLoginBound](/home/justin/projects/Portico/portico-internal/hosted-services/hosted/internal/postgres/account_mfa_login.go:26) looks up the account and applies account/network cool-off only when it exists. With the [four-free-failures setting](/home/justin/projects/Portico/portico-internal/hosted-services/hosted/internal/postgres/account_password_backoff.go:29), an existing username can progress to a cool-off/429 response while an unknown username continues returning invalid credentials until the separate generic network limit intervenes.

The dummy bcrypt check for unknown users addresses timing, but not this response distinction. It leaks username existence, not the password or account contents; the value is lower for usernames already made public elsewhere.

**Disposition:** keep responses and observable failure pacing consistent for existing and unknown usernames without creating an unbounded table keyed by attacker-supplied names. Retain the existing password-work and network bounds.

## Intentional boundaries and excluded leads

Public-client Sign in with Portico produces the normal server/challenge-bound identity assertion, and the local server applies that person's membership and role. For an owner, that can result in owner authority. This matches the [current public-client contract](</home/justin/projects/Portico/portico-internal/Project Architecture/07 Hosted Services.md:11>); the consent page already identifies the unverified app and exact return address, while PKCE and single-use exchange protect delivery. It is not an app-scoped API key. The distinction should remain clear in consent, but introducing app registration or a new scope model is not counted as a missing release implementation.

The Hosted origin allowlist was examined as a possible additional bundled-web failure. Normal bundled account sign-in deliberately hands off to the central site through [HostedHandOff](/home/justin/projects/Portico/apps/portico-server/web/src/screens/auth/SignIn.tsx:89), so arbitrary LAN-origin access to Hosted is not treated as a requirement. RUN03 is the separately established claim-origin defect. Android App Link association was also not counted as a blocker because the existing account-return page/custom-scheme path retains its private continuation binding.

## Review coverage and verification

The assessment combines the requested Sol and Opus 5.5 reviews across six release surfaces, parent validation of cross-surface findings, and an additional independent Windows startup check. All reproductions used disposable state; no live exploitation, installation, deployment or publication was performed.

| Surface | Material paths reviewed |
| --- | --- |
| Hosted API, web host and DNS | Password/OIDC/MFA/recovery/session authority, browser cookies/CSRF/origins, public-client consent and continuation, TV/device grants, claims and removals, signing/revocation, outbound route proofs/wakes, certificate issuance, DNS labels/feed/CAA, proxy and production-unit configuration |
| Local server | Setup/origin tokens, recovery authority, session families/refresh/reuse, profiles/PINs/device approval, role/library/content restrictions, scoped API credentials, media grants/downloads, filesystem confinement, remote-media policy, backup/restore |
| Windows/macOS/Linux/Docker distribution | Service identity and ACLs, lifecycle/restart parent, persistent state, package scripts, Docker non-root/init/health wiring, trust-root injection, source identity/sealing, FFmpeg bundle and release asset wiring |
| Web and client-core | XSS sinks/CSP, browser credential storage, cookie/CSRF separation, link fragments, account and server selection, claim setup, renewal/account switching, route proof and certificate binding, selected access-control handlers |
| Apple | Keychain/reinstall/locked-store behavior, TLS pinning/account-host exclusion, redirect-refusing transports, deep/universal links and provider callbacks, native playback/mpv relay, profile privacy, downloads/background launch, platform search/Top Shelf cleanup |
| Android and Fire TV | Keystore storage, manifest/backup policy, account/control/event redirects, TV approval cryptography, Media3/mpv resource scope, playback/listening bindings, download digest/ownership, Gradle variants and release pipeline |

Passing checks must be read alongside the failing targeted probes above:

- Hosted review: all Hosted package tests and DNS tests passed. PostgreSQL-backed tests passed against a disposable PostgreSQL 18 instance, including internal/postgres and internal/httpapi; internal/postgres took 225 seconds. An initial run with a 1 GB database-container cap failed when PostgreSQL entered recovery/unavailable state; the rerun with 2 GB and serialized package execution passed. Those initial environment failures are not counted as application defects. Release-tagged Hosted compilation with readonly module handling also passed after the manifest was restored.
- Web review: 589 web unit tests and 2,030 client-core tests passed.
- Apple review: 748 native JavaScript tests passed and the native TypeScript check passed. These tests did not catch the producer-shaped audio snapshot mismatch.
- Android review: 80 focused existing tests passed across codec/replay, approval, certificate trust, account/event transport, capabilities, mpv, offline playback and native listening/playback.
- Parent: 73 focused route identity, certificate binding, server assertion, remembered-connection, session-renewal and setup tests passed. The shared package parity check passed.
- Parent Go package checks passed for hostedtrust, hosted, networking, remotemedia, mediaartifact and mediaexec; hosted/networking included cached successful results.
- Server review: targeted identity/content/download/HTTP security tests and storage root/policy/confinement checks passed; full remotemedia, backup and recordingaccess package tests passed. A remotesources selection matched no tests and is not counted.
- Distribution review: two portable packaging fixtures, three release-seal fixtures and eight macOS seal fixtures passed, plus focused restore/crash/rollback/scrub/pressure/restart/feed tests. Mocked macOS tools do not establish an installed macOS package lifecycle.
- Web review: current browser/app setup origin tests passed. The additional Host/Origin bootstrap probe demonstrated SEC03.
- Additional probes demonstrated SEC01, SEC02, RUN02, REL01, REL03, DATA01 and BETA02. Security/administrator probes intentionally assert the desired protected behavior and fail on the current defects; these are findings, not unexplained baseline-suite failures.

The dependency check reported zero production npm advisories for the server/web workspace. The native npm production-tree scan reported 21 entries, including a critical shell-quote advisory, but the traced chains include Metro, Jest, launch-editor and React developer tooling. The audit did not establish those advisory paths as reachable in a shipped release-mode native app, so raw scanner severity is not promoted to a client launch vulnerability. This result is not a full native binary, Go, FFmpeg or mpv dependency certification.

Important reviewed protections remain present: Hosted identity assertions bind server/challenge/installation; signed route proofs bind server identity and nonce; native account TLS is kept separate from owner-certificate exceptions; control/event transports reject redirects; platform stores protect credentials; permissions and download grants are rechecked on new requests; diagnostics use bounded/redacted projections. These controls do not cancel the narrower bypasses identified above.

Hosted-specific controls reviewed include origin-checked, HttpOnly browser refresh cookies with CSRF binding; OIDC issuer/audience/nonce validation and provider-subject linking; durable MFA/recovery-code use; context-bound X25519 TV grants; generation-scoped claim/removal authority; monotonic signing revocations; pinned outbound resolution/TLS with no redirects; namespace-constrained certificate requests; and bounded DNS workers/feed authentication. No Hosted account takeover, claim-owner bypass, signing bypass or DNS credential leak was established in this pass.

## Workspace integrity

Only this audit file was added by the assessment. The server's pre-existing untracked dist directory was preserved, as was concurrent native work. One Hosted test invocation used GOFLAGS=-mod=mod and temporarily normalized hosted/go.mod; that accidental audit-generated change was reverted to the exact original before delivery. The final application trees have no audit-authored source or dependency changes. Disposable Hosted containers were removed. No fixes, commits, installations, deployments or publication were performed.

## Required release decisions

1. Resolve SEC01 and SEC02 before exposing real accounts or offering protected downloads publicly.
2. Resolve RUN01, RUN02 and RUN03 before presenting Windows, native music/audiobooks and the default server-web account setup as day-1 working paths.
3. Resolve REL01 through REL03 or provide an explicitly identified alternative artifact-production path with equivalent provenance and checks. Existing testing artifacts do not demonstrate that the documented workflows work.
4. Resolve DATA01 before asking testers to rely on the product's verified backup/restore behavior for valuable state.
5. Track the P2 items with their stated limitations. They do not justify a blanket demand for every unfinished feature or every physical-device test before an alpha.

No fixes are implemented by this audit. The findings are suitable for a separate remediation lane with targeted regression checks at the exact failing boundaries.


## Authorized remediation, 18 October 2026

Justin subsequently authorized implementation of all 19 fixes. This section records that separate implementation phase; the original findings, priorities and assessment evidence above are retained as the historical baseline. The original “no changes” statements apply to the assessment phase only.

**Current status: implementation of all 19 findings is complete, with focused verification and review. Final Hosted package verification passes. Release qualification remains incomplete.** A complete mandatory server race run has not passed. The fixes have not been deployed, published or installed over a tester's client.

### Isolation and provenance

All React Native changes are confined to `/home/justin/projects/Portico/.worktrees/launch-readiness-native-20261018`, branch `fix/launch-readiness-20261018`, based on `ec1d94145cecc7bee947fb3540cb6be9dcb10c40`. The active `apps/portico-react-native` checkout remains untouched and clean. The separate branch has not been merged into it. Android build outputs and dependencies used for qualification are also confined to that worktree.

Server fixes are in `apps/portico-server`; Hosted fixes are in `portico-internal/hosted-services/hosted`. Most implementation edits remain uncommitted for review. Three local server commits establish clean canonical shared-package revisions needed by the native synchronization tool:

- `2835f870f3a8fdaf27c45fa95dd96779d0d54b55`: claim approval request deadline.
- `13887817c9fc4e975bc10ae319a43cbee150d034`: browser setup address guidance.
- `5d5d02924766d0cdbf97bdac56cdd5b788309e82`: generated administrator route access metadata.

The isolated native shared-package lock and byte-parity check use the last revision. These local commits have not been pushed. The server's pre-existing untracked `dist/` directory is preserved. No PR, deployment, release upload or publication was performed.

### Finding-by-finding remediation

“Implemented” below means the defect has a source change and the stated focused evidence. It does not imply a complete hosted CI run, every-platform binary certification or deployment.

| ID | Implementation and resulting behavior | Verification and remaining limits |
| --- | --- | --- |
| SEC01 | Direct recovery-account operations, password proof and session refresh now check recovery locality inside the authority transaction. Replay handling checks locality before mutation or family revocation. HTTP admission supplies explicit locality; missing locality defaults to remote. Hosted-admitted session families retain their intended exemption. | Real-router and identity regressions exercise remote refusal without mutation, local success, explicit remote permission, proxy locality, refresh replay and Hosted-admitted families. Independent authority review found no remaining bypass in these paths. |
| SEC02 | Download response readers recheck current session, library, grant and exact artifact authority before and after bounded reads. They discard bytes read after withdrawal and close once. Database authority invalidation now advances after successful transaction commit, including prepared/RETURNING statements, closing the gap where a pre-commit reader could cache the old snapshot. | The original revocation probe now returns zero additional bytes after completed withdrawal. Deterministic tests cover reads between withdrawal SQL and commit, first post-commit refusal, rollback, connection reuse, prepared statements and ordinary cache-neutral progress writes. Focused race checks pass; a full database-work suite had a checkpoint-convergence failure under concurrent load, while that exact subtest subsequently passed alone. |
| RUN01 | A platform-specific directory synchronization helper retains Unix error handling and uses the existing Windows durability policy. Claim keys, server authority initialization, artwork/media publication and other untagged directory-sync callers use it. Claim-key initialization cleans up safe pre-commit failures and preserves material after an ambiguous commit attempt. | Linux storage/backup/networking/media suites pass. Windows server builds succeed for amd64 and arm64; eight affected package test binaries compile for Windows amd64. No Windows runtime execution is claimed. Ambiguous commit failures intentionally fail closed and may require recovery rather than deleting potentially committed key material. |
| RUN02 | The native listening adapter accepts all nine rates emitted by the real Android and Apple producers, including 2.5x and 3x, and validates the effective-rate range consistently. | Tests read producer declarations from the actual Kotlin and Apple source and exercise parse/adoption/event behavior at 1x, 2.5x and 3x, plus malformed input. Native JavaScript and TypeScript checks pass; Android sources compile in the build qualification below. |
| RUN03 | Packaged server-web builds default account claim validation to `https://web.getportico.tv`. Development retains its development origin; explicit `VITE_HOSTED_URL` and the Hosted account workspace retain their configured origins. | Origin-selection tests cover packaged localhost/LAN/custom server addresses, valid production approvals, attacker origins and explicit/development configuration. All 591 web tests and the production web build pass. Custom Hosted installations still need the explicit build configuration. |
| REL01 | The sealed snapshot includes the sibling scripts, shared catalogue/runtime source and checked-in web asset inventory consumed by mandatory tests, alongside the server module and apikit. Capture and later verification hash the same complete input set and reject concurrent edits or symlinked added inputs. Vet executes the captured script. | Fifteen Python release fixtures, four isolated sibling-input regressions under race instrumentation, dependency verification, helper build and full vet pass. The full unchanged mandatory race suite was attempted but terminated with status 143 before completion; it is not passed. The inspected snapshot predates two later SEC03 fixture corrections. |
| REL02 | Clean Android CI/release preparation now installs the declared host tools, locked SDK/NDK and Rust toolchain, builds the media engine for the declared ABI set, verifies it, and only then runs Gradle. Configuration extraction rejects lock/app drift. | Pipeline fixtures, shell/action syntax checks and 5,026 engine verification checks pass. A deliberately missing engine fails Gradle as expected. Both x86_64 debug flavors, 36 JVM tests, both release APKs and both release bundles build locally. Hosted GitHub Actions and a fresh engine rebuild from source were not run; local builds used verified existing engine output. |
| REL03 | Release collection uses exact Google and Fire TV variant paths, disables ABI splits for those publishable APKs and requires all six nonempty regular-file assets before copying anything. It includes both APKs, both bundles, the locked source bundle and notices. | Eight pipeline fixture tests include each missing/empty artifact. Collection of the real local release outputs preserves all six hashes. No artifact was published or installed. |
| DATA01 | New backups use manifest version 2 with the final scrubbed, checkpointed and closed database's size and SHA-256. Restore rejects missing/malformed database entries and unexpected nonempty WALs. Staging rechecks the private copied files against the originally captured manifest before inspection can modify them. | Full backup tests pass after the final staging correction (105.202 seconds). Regressions cover structurally valid database payload corruption, manual/scheduled backups, malformed manifests, WALs, invalid SQLite and a deterministic source/manifest replacement between validation and copy. Legacy version-1 backup compatibility is deliberately retained with its older integrity limits; raw database imports remain structural validation only. Hashes do not establish authenticity against an attacker able to rewrite a whole backup and manifest before validation. |
| BETA01 | The update reader manually validates each redirect and creates a freshly DNS-admitted client for each hop. Only the official Portico GitHub manifest path can cross to the narrow HTTPS GitHub release-asset host/path. Custom feeds remain within their original origin and retain pinned private-feed approval; redirected CDN hosts receive no private-network exemption. | Focused feed/redirect race tests cover allowed paths, hostile hosts/paths, DNS/private destinations, downgrade, credentials, loops and response limits. The live manifest returned 404 during verification, so a successful live feed fetch is not established. |
| BETA02 | Only the claim approval POST receives a 615-second deadline, covering the server's ten-minute approval window plus a margin. Other request deadlines retain their existing values. | Deadline and real HttpLocalApi/web-claim tests prove a 90-second approval succeeds, the claim remains live beyond one minute, and the extended deadline still terminates. Canonical packages are synchronized into the isolated native worktree. |
| BETA03 | Administrators receive the intended baseline library visibility while profile narrowing and private-recording rules remain effective. | Identity/recording-access tests cover owner/admin/profile tiers and narrowed libraries. Additional HTTP read tests cover private DVR and real library IDs; administrators do not inherit an empty-scope bypass. |
| BETA04 | Library and manual-metadata handlers and generated contracts permit administrator scopes, while writes recheck current tier, family and locality inside their authority transactions. List/inventory/detail/job reads use the administrator's actual permitted library set before pagination. Owner-only security and recovery controls remain owner-only. | Combined focused HTTP, catalog, administration, recording-access and identity checks pass. New tests cover demotion/revocation during a write and private/profile-narrowed read scope. Generated OpenAPI and shared route metadata were regenerated; 40 canonical contract/admin tests and 47 native role/owner/avatar tests pass. |
| BETA05 | Native avatar display subscribes to the currently verified selected route and withholds stale images during proof changes. Upload uses HttpLocalApi authentication/renewal/deadline/redirect rules; the platform bridge decodes a bounded binary envelope restricted to the avatar endpoint. | Real client tests cover alternate LAN routing, proactive and 401 renewal with unchanged bytes, bounded repeated 401, identity refusal without bearer release, network errors, path/size bounds and maximum payload. Kotlin compiles in Android qualification; Apple bridge wiring is source-reviewed. |
| BETA06 | A native singleton owns background download sessions independently of React Native initialization. AppDelegate stores the completion handler before recreating the exact session on a background wake. Serial filing finishes before completion, handlers complete once, and protected-data/manifest retries preserve queued finished files without overwriting unread state. | Nine source/lifecycle wiring checks plus native JavaScript and TypeScript checks pass. Independent review's retry and completion-order objections were corrected in a second implementation round. No Apple compiler/device lifecycle run is claimed. Missing Apple end-to-end testing is not promoted to a new audit finding. |
| SEC03 | First-run browser setup independently validates the requested authority against literal addresses, localhost/the machine's local names, configured access URLs or exact `PORTICO_SETUP_HOSTS` entries. Matching arbitrary Host and Origin alone no longer authorizes setup. Attacker DNS is not resolved to establish trust. Startup rejects malformed allowlists; setup guidance explains custom aliases. | Real-router tests cover forged matching authorities, setup start/write/resume, valid setup tokens on hostile origins, proxy mismatches, IP/IPv6/local names and explicitly configured aliases. Focused setup regressions pass. Custom DNS/Tailscale/router aliases require an explicit trusted setup hostname or address. |
| HOST01 | Hosted claim streams apply /48 IPv6 and /24 IPv4 wide-network caps in addition to existing per-address and global caps. Stream entries have unique identities and idempotent release so old cleanup cannot remove replacements. | Concurrency/replacement/accounting and race checks pass. Independent Hosted review confirmed limits and exact unexpired claim authorization remain enforced. Multiple unrelated networks can still exhaust a finite global pool; no edge service was deployed. |
| HOST02 | Invalid second factors accumulate durable, serialized account-level backoff across challenges, with bounded exponential delays; successful factors clear their scope. Existing challenge and account budgets remain. | PostgreSQL-backed focused tests pass for sustained second-factor backoff and verified password reset clearing sign-in backoff. The reset regression was also confirmed to fail without the correction. The challenge-expiry comment is corrected. Final PostgreSQL, HTTP and identity package tests pass; independent review accepted the correction. |
| HOST03 | Unknown usernames now use the same cool-off schedule in existing durable security-limit storage, selected through stable seal-key HMAC buckets. At most 3 × 65,536 rows are possible, with no attacker-supplied names stored. This replaces the rejected memory-only implementation. | Focused PostgreSQL tests cover matching known/unknown schedules, invalid-name exclusion, full bucket capacity, parallel attempts, restart/replica continuity and successful real-account sign-in from a fresh network. Bucket collisions may cause an unknown name to cool off early, creating a residual probabilistic hint under a large distributed flood. Dummy bcrypt does not guarantee identical timing. The verified-email registration availability response remains intentional. Final PostgreSQL, HTTP and identity package tests pass; independent review accepted the correction. |

### Integrated checks and explicit limits

The current combined server builds successfully. The web production build and 591 web unit tests pass. Full native JavaScript testing passes all 767 tests; direct app and test TypeScript checks pass. Canonical/native shared-package parity passes, and the active native checkout remains clean. Android qualification establishes local compilation and real artifact collection as described above, without replacing the active client's build or emulator.

The complete client-core run was attempted twice: each run passed 2,031 of 2,032 tests, with different very short wall-clock expectations in `playback-progress-windows.test.ts` failing under concurrent load. That file passed all three tests alone. A full HTTP package run also did not pass: it timed out at 600 seconds and included superseded setup expectations, OpenAPI drift subsequently regenerated, and an unrelated missing listening-helper path. Focused final setup/authority/download checks pass, but those results are not relabelled as a green full suite. The release snapshot's mandatory full race suite ended before completion. These are outstanding verification limits, not additional invented security findings or evidence that every failure is environmental.

Hosted verification used disposable PostgreSQL 18 with a 2 GB limit and serialized execution. The final HOST02/HOST03 run passed `internal/postgres` in 234.513 seconds, `internal/httpapi` in 0.289 seconds and `internal/identity` in 0.666 seconds, with exit 0. The PostgreSQL package exercised the real database; the HTTP and identity packages passing does not establish database-backed HTTP integration because their tests do not consume that database setting. Focused new database regressions also passed with race instrumentation. Vet and the combined Hosted build pass. Both the implementation agent and independent reviewer observed the completed full-run log before temporary-file cleanup; its result is preserved here. The disposable database and credential file were removed. No production database was used.

Windows has cross-build and test-compilation evidence; Apple changes have source, wiring and shared JavaScript/TypeScript evidence. Neither is a claim of a native platform runtime pass. The user explicitly excluded already-known missing Apple end-to-end testing as a standalone blocker.

A release candidate should be built from a settled combined revision and run through its mandatory checks before publication. This implementation lane has neither waived those checks nor carried out a release. All 19 findings, including the original nine P1 items, have source fixes with the focused evidence above; the broader release-check limitations remain open here. No delegated implementation or review task remains active.


### Final review disposition

Independent reviews covered the web/setup boundary, server authority and download revocation, native bridge/background lifecycle, and Hosted abuse/authentication changes. Follow-up work closed the objections about transaction-commit invalidation, administrator private-library reads, iOS protected-data retries and completion-handler order, unknown-username capacity/persistence, and password-reset clearing MFA sign-in backoff. The final independent Hosted review found no material new security regression in its scope.

Residual HOST03 tradeoffs are explicit: bucket collisions can delay an unknown name early and create occasional probabilistic hints under a large distributed flood; work is equalized with dummy bcrypt without promising identical timing; verified-email registration retains its intentional availability response. The implementation review also noted a possible lock-order collision between stale-row maintenance deletion and an unknown-name transaction. This was not reproduced; PostgreSQL would abort one participant, potentially failing a request rather than bypassing authentication. It is recorded as a concurrency limit, not an established new release blocker.

The remaining release gate is qualification of the settled combined candidate under the mandatory build/test checks, together with real distribution signing and store preparation when authorized. Those steps have not been claimed as completed by the 19-finding remediation. Apple/Google identity consolidation discussed subsequently is separate release preparation; no bundle identifiers, store records or signing identities were changed for that discussion.
