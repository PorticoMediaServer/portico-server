# Reverse proxy and networks acceptance

## Purpose and evidence

Updated for the evening implementation and owner decisions. New cases remain qualification work; known missing candidate implementation is Fail, while an unavailable test prerequisite is Blocked. Native-owner recorded 655 native/87 Vega before the mirror; the later running log confirms 655/87 again after the mirror with clean type checks and equal packages. Physical acceptance remains unrun.

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Private server, two client machines with distinct addresses, proxy with its own hostname and trusted TLS, configurable forwarding headers, Tailscale addresses, dual-stack/global IPv6 and a link-local segment, owner PEM chain/key, a second service for redirect capture. A public/remote HTTP origin may be a controlled routable test hostname; never use a URL sub-path. Use the candidate package, matching web and two profiles; network tests need no production Hosted claim except the publication case.

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

Install the server on a disposable host with wildcard `[::]:32500`; open /v1/readiness and /v1/server from both address families. Put an HTTPS proxy on an origin such as https://media.test.example, forwarding / to the plain HTTP backend. Preserve Host initially. For nginx use proxy_http_version 1.1 and proxy_buffering off for /v1/events; keep timeouts beyond the feed heartbeat. Add only the proxy’s actual peer CIDR to Server > Remote access > trusted proxies when it rewrites forwarding/Host. Home networks is the only locality authority. Use an actual owner certificate/key readable to the service; record public serial/expiry, never key bytes. A 400 setup-origin failure needs same-origin Host/trust correction; buffered SSE needs proxy flushing or its existing long-poll fallback; neither calls for relaxing origin/authentication checks.

## Smoke subset

Run cases 1, 2, 4 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. First setup through own hostname and spoofed origins — smoke

1. With fresh state visit proxy /, finish setup and sign in; save a setting without an origin environment override.
2. Configure trust for the exact rewriting proxy peer and test X-Forwarded-* plus RFC Forwarded from two clients. Send forged forwarded headers directly from an untrusted peer.
3. Send setup/write requests from another site’s form/fetch/preflight and malformed Forwarded input.

**Expected:** Same-origin setup/write works; genuine client addresses/rate subjects differ; untrusted headers do not override socket peer. Foreign-site owner creation fails and malformed input cannot panic.

**Failure:** Proxy-origin refusal, all users classified as proxy/home or spoofed headers create owner/alter identity.

**Route to:** networking, review-networking, accounts-security.

### 2. HTTP, Tailscale, IPv4 and IPv6 identity — smoke

1. Connect over local, Tailscale and controlled global HTTP and trusted HTTPS; perform the normal /v1/server nonce identity proof before signing in.
2. Test bare/bracketed IPv6 entry in native apps, wildcard bind from both families and a link-local URL using the correct interface zone. Give an invalid DNS percent-escape address.
3. Change the address to another server with a different identity while the app is signed in.

**Expected:** HTTP admitted from every address subject to ordinary authorization; both families work, scoped literals validate correctly and invalid DNS escapes refuse. Identity mismatch preserves sign-in but sends no bearer to the wrong server.

**Failure:** Private-only HTTP, hidden overlay detection, one family unreachable, scoped parse error or credentials sent before identity proof.

**Route to:** networking, review-networking, native-sync, leftovers-native.

### 3. Home networks and remote policy

1. Observe connection badge; remove the current range, save and re-read, then add it back. Test [] through bridged/rootless/tunnel routes without inventing original IP.
2. Put Tailscale CIDR in home networks and compare quality.home/quality.remote plus remote bitrate limits. Set remote sign-in off and test trusted-proxy home/away clients.
3. Remove proxy trust then replay an older full settings write; send forged home XFF from removed peer.

**Expected:** Only current committed home ranges define locality; empty list makes all addresses remote. Home/away quality and remote sign-in apply consistently; stale replay cannot reinstate trust/revision.

**Failure:** Gateway makes everyone local, stale settings overwrite current trust, remote sign-in wrongly allowed or policy inferred from overlay.

**Route to:** networking, review-networking, playback-core, accounts-security.

### 4. Owner certificates, replacement and expiry — smoke

1. Set owner certificate with and without a Hosted claim, connect via SNI/h2 and inspect status.
2. Atomically replace the PEM pair with same-size/timestamp new valid bytes; wait ≤30 seconds. Corrupt/mismatch replacement and inspect serving/error status.
3. On isolated-clock clients advance past expiry and try a resumed TLS session, then use HTTP console to repair.

**Expected:** New bytes loaded; corrupt replacement reports error while previous valid certificate serves; expired new/resumed handshakes refuse. HTTP/console remains usable.

**Failure:** TLS silently trusts bad/expired material, certificate detection only follows mtime, broken replacement shuts console down or leaked private key.

**Route to:** networking, review-networking, ops-packaging.

### 5. Console reserve under combined load and HTTP/2

1. Set small connectionLimit, save and verify Requested versus Effective/pending restart; restart and confirm new capacity.
2. Fill ordinary authenticated media sockets including owner streams, all conversion slots and an active scan. Cold-open console sign-in/bootstrap via HTTP, HTTPS and negotiated h2; pause/resume/terminate/restart work.
3. Hold idle reserve/header-stall connections, then release; test API-key traffic and normal first h2 handshake.

**Expected:** Human console authorization/render/control remains reachable; excess ordinary traffic gets honest retryable busy, API scripts do not take human reserve; idle stalls release without breaking first h2/active streams.

**Failure:** Login/control blocked, owners bypass all ordinary limits, h2 killed before first request, unbounded reserve flood or no recovery.

**Route to:** networking, review-networking, limits-settings, review-livetv.

### 6. Freshness through proxy and network changes

1. Open Home/library at deep scroll in one client; edit watched state/title/settings from another. Observe one /v1/events stream, cursor and visible/focused card.
2. Idle beyond heartbeat, renew token, disconnect/reconnect and route-switch. Turn proxy buffering on for a separate run and measure fallback.
3. Expire retained event history through maintenance, reconnect an old nonzero cursor, then revoke current session.

**Expected:** Updates arrive quietly within about 30s with no blank/scroll/focus movement; comments flush, old stream retires. Buffered failure uses bounded long poll; old cursor signals resync, revoked feed ends.

**Failure:** Stale open screen, repeated idle DB reads/polls on healthy native SSE, lost resume cursor or stream survives authority loss.

**Route to:** freshness-server, freshness-clients, review-freshness, native-app.

### 7. Discovery across interfaces and route promotion

1. Enable discovery on two interfaces same VLAN plus separate v4/v6 segments; browse from Apple/Android/Vega capable clients. Change interface and disable discovery via full settings.
2. Connect remotely while LAN unavailable, restore LAN without a path event, foreground or request a read; prove the candidate LAN identity before use. Repeat failed promotion frequently.
3. For Hosted claimed server capture route publication with broken Hosted certificate and valid manual HTTPS domain; supply eight LAN/eight manual candidates.

**Expected:** Unique per-interface names group by server identity, disable withdraws. Bounded proof promotes LAN without Hosted polling/screen reset; failed checks no more than once/minute. Hosted final routes all HTTPS, ≤12 retaining preferred external route.

**Failure:** Self-withdraw loop, inferred localhost trust, promotion sends bearer before proof, HTTP publication or preferred external route dropped.

**Route to:** networking, review-networking, leftovers-native, hosted-followups.

### 8. Slow body, transfer progress and redirect bounds

1. Upload permitted Live-source JSON in >15 but <60 seconds with progressing bytes; run two uploads then a third; keep console active.
2. Send oversized/stalled body; stream provider artwork/guide/subtitle/live segment slowly beyond old total deadlines, then halt progress.
3. For media/HLS/subtitle endpoints return redirect to a capture origin, 401/403/410 and out-of-grant absolute references from a private fault proxy; inspect both origins.

**Expected:** Progressing bounded bodies finish; only admitted upload count runs, true idle/size bounds refuse. No redirected credentials or grant escape, authority failures remain distinct from transport retries.

**Failure:** Whole 15s upload cutoff, unbounded progressing hostile body, stalled sockets never close or capture host gets credentials.

**Route to:** networking, review-networking, limits-settings, subtitles-text, native-playback.
