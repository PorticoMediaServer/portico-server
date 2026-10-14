# Hosted services acceptance

## Purpose and evidence

Updated for the evening implementation and owner decisions. New cases remain qualification work; known missing candidate implementation is Fail, while an unavailable test prerequisite is Blocked. Native-owner recorded 655 native/87 Vega before the mirror; the later running log confirms 655/87 again after the mirror with clean type checks and equal packages. Physical acceptance remains unrun.

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Owned isolated PostgreSQL17/18, Go/Node, new private test trust root/signing/sealing keys, isolated same-hostname HTTPS reverse proxy, disposable Hosted password/provider/MFA accounts, linked private local server and physical TV/phone for callback/Cast. Optional sandbox DNS/ACME issuer and domain under tester control. Format21 candidate must include final hosted-followups integration; older format notes are historical.

## Media and accounts

For ordinary playback, make synthetic samples with the candidate's qualified FFmpeg/ffprobe pair. Run in a new writable fixture folder (never over existing media):

```sh
ffmpeg -f lavfi -i testsrc2=size=1280x720:rate=24 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 19 -c:v libx264 -g 96 -keyint_min 96 -sc_threshold 0 -c:a aac -movflags +faststart 'Fixture (2026).mp4'
ffmpeg -i 'Fixture (2026).mp4' -c copy 'Fixture (2026).mkv'
ffmpeg -f lavfi -i sine=frequency=440:sample_rate=44100 -t 120 -c:a flac -metadata artist='Fixture Artist' -metadata album='Fixture Album' -metadata title='Track 01' -metadata track=1 'Track 01.flac'
```

Make three TV copies named `Fixture Show/Season 1/Fixture Show - S01E01.mkv` through `S01E03.mkv`; make a second version/edition copy and a two-part film. Write a UTF-8 sidecar `Fixture (2026).en.srt` containing `1`, `00:00:02,000 --> 00:00:05,000`, and `Fixture caption`, separated by newlines and followed by a blank line. For long buffering/background/download cases generate a 20-minute/2+GB synthetic file by raising duration/bitrate and verify its hash before testing; a 19-second clip cannot establish those cases.

For HDR10/HLG/Dolby Vision 5/7/8, TrueHD/DTS-HD/Atmos, PGS/VobSub/DVB, HE-AAC/SBR and attached-font ASS use legally held test clips/disc or broadcast extracts. Log provenance, ffprobe facts and a correctly rendered reference frame/audio/channel map. SDR bars with HDR tags are not HDR-fidelity evidence. The server checkout has `fixtures/audio-gapless/` and `fixtures/audio-gapless/generate-portable.py`; preserve its original corpus and generate extras with `python3 fixtures/audio-gapless/generate-portable.py --out /tmp/portico-audio-extra`. Obtain measured HE-AAC/PS fixtures separately. Missing real formats block their cases. Make bad-input copies separately: truncate a copy, corrupt a metadata row, or insert a malformed subtitle cue between good cues. Record which data changed.

## Build or install

Use an integrated internal candidate that includes hosted-followups, with the format-21 current schema and matching shared apikit; the internal-test-plans checkout’s Hosted code is an older baseline and is not the candidate. Source verification used `/home/justin/projects/Portico/.worktrees/audit-implementation/internal-hosted-followups`; record the eventual integrated internal SHA. Need Go version from module, Node, PostgreSQL17/18 server tools (not only psql), private test root/signing/sealing files and provider/issuer sandboxes for the corresponding cases. Never connect these tests to an existing business/production database.

Owned test-cluster preparation on Linux, from a writable qualification directory (choose an unused port):

```sh
umask 077
work=$(mktemp -d /tmp/portico-hosted-accept.XXXXXX)
mkdir "$work/socket"
initdb -D "$work/db" --auth-local=trust --auth-host=reject
pg_ctl -D "$work/db" -o "-k $work/socket -p 55473 -c listen_addresses=''" -w start
createdb -h "$work/socket" -p 55473 portico_hosted_test_accept
printf 'postgres:///portico_hosted_test_accept?host=%s&port=55473&sslmode=disable
' "$work/socket" > "$work/dsn"
export PORTICO_TEST_DATABASE_URL_FILE="$work/dsn"
export GOFLAGS=-p=2
```

From hosted-services/hosted run focused tests only:

```sh
go test ./internal/postgres ./internal/contract -run 'Test(AppAuthorization|SigningRollout|CheckInDelivers|OlderReplica|MemberConsent|MemberPush|OwnerChange|Device|RefreshRotation|ConcurrentSameRefresh|CurrentIdentitySchema|HostedRejects|MigrateDryRun|SecureLANRouteDocument|RouteSets|Contract|Generated|Golden|Definitions)' -count=1 -v
go test ./internal/httpapi ./internal/appauth ./internal/security ./internal/directory ./internal/wake ./internal/claims -run 'Test(App|Consent|ServerIssued|Redirect|PKCE|Signer|KeysSetup|RevokedSigner|ExpiredSigner|Credential|CleanupIs)' -count=1 -v
```

No selected PostgreSQL test may SKIP. Build `GOFLAGS=-p=2 go build -o /private/bin/portico-hosted ./cmd/hosted`. Use the key CLI `portico-hosted keys setup --private-out /private/test-root.pem --public-out /private/test-root-public.pem --staging /private/test-signing` only for a NEW isolated root; inspect its help/KEYS.md and retain all three private signing/certificate/seal files together. Start loopback candidate with `--database-url-file "$work/dsn" --key-file /private/test-signing/signing.json --root-public-key-file /private/test-root-public.pem --listen 127.0.0.1:19410 --claim-audience https://hosted.test.example --origins https://hosted.test.example`; proxy its own trusted HTTPS hostname, configure matching issuer and private OIDC/ACME sandbox files only for dependent cases. For local HTTP enrollment use explicit development loopback flag and matching issuer; never broaden production origins. Stop only this process/owned cluster at end: `pg_ctl -D "$work/db" -m fast -w stop` and preserve required redacted evidence first.

Common first failures: uncached pgx/OIDC/DNS/oauth dependencies require locked module download/cache; absent initdb requires PostgreSQL server package. Previous schema intentionally refuses: make a fresh test database, preserve old fixture and key files. Missing/expired/revoked signer/certificate/seal is a startup refusal; install the matching approved test files, do not auto-generate a replacement sealing key. Empty/unusable check-in certificate preserves presence but grants no new signing authority. Provider/cookie/browser/OS callback acceptance remains distinct from stand-in page tests.

## Smoke subset

Run cases 1, 2, 8 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. Schema, private keys and actual database tests — smoke

1. Prepare owned empty format21 cluster/build/keys as above; run named DB/contract tests and record cases executed/skipped, then restart current candidate on same database.
2. Present an old format database and run migration dry-run; compare schema/data before/after refusal. Test missing/wrong/expired/revoked certificate or seal and exact maximum key revision/next value using named key tests.
3. Compare hosted-services/apikit to canonical candidate via scripts/shared-apikit.sh and regenerate contract; fork/scratch CI canonical SHA follows configured source.

**Expected:** All selected tests really run, current restart works; previous format refuses unchanged without compatibility migration. Exact usable signer/root/seal only, unsafe revision refuses without writing file. Copied toolkit byte-identical; intentional drift/extra file fails.

**Failure:** SKIP treated as pass, schema rewritten on refusal, generated new sealing key, unsafe signer acceptance or toolkit drift bypass.

**Route to:** hosted, hosted-followups, review-hosted, registry; fix-pass hosted.

### 2. Consent, keyboard, cookies and real callbacks — smoke

1. With disposable member open app authorization signed out and signed in; password/provider/MFA, account switch and long/RTL app/server/return names at desktop375px keyboard/screen reader.
2. Hold Enter/double-click Continue, then inspect reviewed question focus and Tab to Allow/Cancel only after review; app named X {serverName}. Cancel and exact typed HTTPS/loopback/custom-scheme callback.
3. At private proxy return HTML503 Retry-After date10s; press Continue immediately then after floor. Alter state/PKCE/redirect, expire/replay code and run concurrent12 exchanges.

**Expected:** No accidental consent/hidden account switch, real cookie behavior and exact destination preserved; Cancel only access_denied, no OS protocol launch from unapproved page. Gate suppresses early retry; wrong verifier/URI does not consume code, exactly one concurrent exchange wins.

**Failure:** Held key grants consent, app controls sentence/authority, callback normalized to different destination, state/PKCE ignored or retry hammers.

**Route to:** hosted, review-hosted, hosted-followups.

### 3. Identity assertion and local admission

1. Complete code-flow on actual receiver app with nonce/server challenge; inspect returned assertion content and local admission for invited/existing/nonmember plus profile/MFA policy.
2. Switch Hosted account/family after review, revoke consent/account/server/family, test request/code expiration/denial/quota. Approve TV code with member and owner roles.
3. During temporary-profile selection/cancel/restart correlate current-session deletion with token-family IDs privately and accepted family.

**Expected:** Only identity assertion returned, no Hosted takeover of local membership/role/profile/MFA; single-use fences and inherited TV approver role. Temporary cleanup cannot revoke accepted viewer family; auth survives server restart unless actually revoked.

**Failure:** App receives unrelated server credential, wrong membership/role, pending approved code survives expiry or cleanup signs out accepted family.

**Route to:** hosted, accounts-security, leftovers-server-a, third-party API consumer.

### 4. Signing rollout across new/old replicas and offline server

1. Admit signerA sessions/local factor/approved uncollected TV/Cast bootstrap/paired reconnect, plus password/signerB controls. Root-certify B revoking A with same sealing key and next revision.
2. Run old/new Hosted replicas on same format21 DB; observe only B signs wakes, old replica returns B on check-in, acknowledgements drain once. Disconnect one local server then restore reachability/check-in.
3. Revoke exact family/account consent and inspect discovery/custody/admissions; replay old envelope or queued consent after rollout.

**Expected:** Local server learns terminal root-proven revocation before later admission, all A descendant/pending/reconnect authority refuses, password/B survives; bounded one sweep, old replica cannot starve wakes and offline server learns on existing check-in. Same revision no repeated sweep; null certificate no forged authority.

**Failure:** Revoked factor/TV/Cast still redeems, broad password sign-out, sweep repeats or stale signer takes wake leases.

**Route to:** hosted-followups, hosted, review-accounts-security, accounts-security; fix-pass hosted.

### 5. Routes, custody and browser trust

1. Publish IPv4/IPv6 privateDNS and external HTTPS candidates; verify signed route document dedup/caps/preferred external origin and route identity proof from actual clients.
2. Revoke member consent/reassign server custody with ambiguous owner accounts and locks; replay old candidate/cross-family directory read. Cut local route/reconnect and inspect retained current UI.
3. Exercise trusted-proxy client IP/buckets and narrow CORS/CSP with actual CastSDK on registered receiver.

**Expected:** Unique current authorized TLS route set, no HTTP Hosted route; wrong family/custody excluded, ambiguity never selects unlocked unrelated owner. Credentials sent only after pinned proof, no gateway 403 sign-out or credential CORS leak.

**Failure:** Duplicate/HTTP route, privateDNS family lost, stale custody re-admitted, proxy buckets shared or Cast blocked by narrowing.

**Route to:** hosted, hosted-followups, networking, native-sync, accounts-security; fix-pass hosted.

### 6. Refresh, code approval, account erasure and retries

1. Run concurrent same refresh, browser refresh cookie, TV approval/collection/device races; leave approved code uncollected until expiry and run maintenance.
2. Perform exact signed test account erasure on linked local server, interrupt/restart before acknowledgement, inspect sessions/saved/grants/preparations and unrelated local recovery owner.
3. Send forged/expired/cross-claim/canceled-lease cleanup and PKCE/callback misuse plus idle Hosted outage; compare device jitter/Retry-After behavior.

**Expected:** Single-winner rotation/reuse/family fences, expired approved uncollected rows cleaned without revoking approver. Erasure commits exact account before ack, resumes idempotently and preserves unrelated owner/data; invalid requests change nothing.

**Failure:** Replay token usable, cleanup endless NULL failure, acknowledgement before commit or broad unrelated erasure.

**Route to:** hosted, hosted-followups, accounts-security, base-health-2; fix-pass identity-ops-live.

### 7. DNS, ACME, restore drill and external providers

1. On isolated domain/issuer sandbox configure real DNS feed and ACME account, request certificate and rotate; use real OIDC accounts/MFA and OS app callback on actual phone/TV.
2. Take complete Hosted DB+sealed key backup, restore into new private instance using restore-drill so outbound/queue workers suppressed; verify read/admission boundaries then remove only disposable resources.
3. Test changed/extra toolkit file gate and pinned release public key/artifact/feed tamper in a scratch pipeline, never live publishing.

**Expected:** Provider/DNS/TLS/OS boundaries match actual observed result, restored rows readable only with original seal/root identity and drill sends no wakes/ACME/export/mail. Signed packages/feeds fail on tampered bytes/source/tag; no live service affected.

**Failure:** Database-only restore claimed complete, external traffic during drill, wrong seal silently repairs rows, unqualified provider or release deployed during acceptance.

**Route to:** hosted, hosted-followups, review-hosted, ops-packaging.

### 8. Real Hosted/local-server driver and immediate claim admission — smoke

1. On qualification internal candidate containing e2e-hosted run `python3 hosted-services/hosted/dev/e2ehosted/run.py --server-tree /absolute/candidate-server --scratch /private/e2e-hosted --pg-bin /absolute/postgresql/bin`. Need Go cached modules, PostgreSQL17/18 server tools, Node TypeScript stripping and Python cryptography; inspect driver ownership/private teardown first.
2. Require16 backend checks pass, inspect its real activation-adapter networkRequestsWhileGateOpen result separately. With claim installed/acknowledged IMMEDIATELY try correctly signed owner identity admission before any owner-link readback wait; record0/403 versus accepted session.
3. Use public isolated HTTPS endpoint to prove automatic queue-leased signed wake/endpoint proof without loopback fixture injection; shipping TV grant decryption/OS callback and provider acceptance remain physical cases.

**Expected:** Driver owns fresh cluster/processes/private keys and tears down only its children; admitted sessions survive Hosted outage, terminal signer revocation arrives through actual check-in. Desired activation shared gate prevents automatic network attempt; current e2e report confirms B-22.26 bypass and an owner-link admission race, so record their actual failure rather than passing because gate unit test/readback wait succeeds. Public automatic wake needs separate real proof.

**Failure:** Private/loopback egress denial bypassed, injected wake claimed public delivery, adapter gap hidden in16-pass headline or delayed admission claimed immediate readiness.

**Route to:** e2e-hosted, hosted-followups, native-sync, accounts-security.

### 9. Immediate claim, shared activation gate and certified bounds

1. Use separate empty format-21 PostgreSQL and the integrated actual server. Let an unclaimed server become idle, install/ack its claim, then sign in immediately. Do not reuse the older loopback driver’s three-second owner-link wait or manual injected wake as proof.
2. Cause one Hosted feature to open an origin gate for HTTP-date/HTML 503 Retry-After 120 seconds; activation automatic poll/create recovery/cleanup sends zero at 30/60/90 seconds. Interactive review/cancel is allowed and local Quick Connect stays independent.
3. Using fixture root keys only, issue revision 9007199254740991 and the next/uint64 maximum. Largest supported verifies/persists; oversized correctly signed artifacts refuse at trust/persistence before SQL. Verify fresh check-in delivery and revoked consent on older replicas.
4. Compare refreshed hosted-services/apikit byte-for-byte to the final server copy, regenerate Hosted contracts, run strict generated TypeScript and contract-current test. Run database-backed PKCE/consent/signer checks with an explicit isolated test URL file, not a silently skipping invocation.

**Expected:** Prompt committed owner readiness, shared long-outage floor, preserved operation receipts, exact supported revision and current same-toolkit artifacts. Loopback backend proof does not establish public endpoint/automatic wake/provider/OS callback.

**Failure:** Claim succeeds but owner access initially refuses, deadline shortened, oversized revision reaches SQL, stale toolkit contracts or database tests silently skipped.

**Route to:** claim-and-gate, e2e-hosted, hosted-followups, handoff-update.
