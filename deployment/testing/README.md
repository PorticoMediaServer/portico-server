# Interactive testing on the personal Ubuntu PC

Active setup: 9 October 2026. Compute runs on `ehler-personal` (`100.120.50.24`), with visible emulators in Justin's existing Ubuntu desktop session. The server and web UI run at **https://demo.getportico.tv** on the existing resource-limited VPS. Apple builds still require the Mac. Emulator smoke checks establish a starting point; they do not establish visual parity with Apple or physical-device acceptance.

## Start testing

From `/home/justin/projects/Portico/portico-internal`:

```sh
scripts/test-clients.sh status
scripts/test-clients.sh start phone
scripts/test-clients.sh install phone google
scripts/test-clients.sh start tv
scripts/test-clients.sh install tv firetv
scripts/test-clients.sh start vega
scripts/test-clients.sh install vega
```

Start the devices you need. Three emulators plus the existing Windows VM can cause memory pressure during compilation; compile with the emulators stopped when practical. The launcher uses four CPU cores and 3 GB for newly started Android devices after a 2 GB phone cold boot produced a System UI ANR under resource pressure. `stop phone`, `stop tv`, and `stop vega` stop only those testing devices. Android commands verify the AVD identity before installing or stopping anything. Vega runs in a detached process group and startup requires both a running virtual device and a connected SDK device adaptor; metadata alone is insufficient.

Open https://demo.getportico.tv in regular Ubuntu Chrome. Chrome's `chrome://inspect/#remote-debugging` setting, **Allow remote debugging for this browser instance**, was enabled on 9 October. After the desktop restart the CDP connector did not attach, but the browser extension connection successfully controlled the same visible regular Chrome tab. Use a verified current connection rather than treating the toggle alone as proof of automation access. Use direct server sign-in on every client. For a TV, select **Sign in directly to a server**, enter `https://demo.getportico.tv`, and approve its code at https://demo.getportico.tv/device while signed in as the demo owner. If the owner session has ended, sign in directly at the server Home page first; the unauthenticated `/device` fallback form is for Hosted accounts. Wait for the local owner session to load before reviewing the code. Vega also supports **Use a password instead**.

The username is `demo`. The refreshed password and operator details are in `/home/justin/projects/Portico/.private/demo-testing/active-credentials.json` (directory 0700, file 0600), with a root-only copy at `/etc/portico/demo-testing-credentials.json` on `portico-demo`. Keep these files, session envelopes and emulator authentication state outside Git. The password was changed using `recover-owner` followed by the supported direct-password endpoint; old sessions were revoked. No account or library records were replaced.

## Installed build components

| Component | Verified installation |
| --- | --- |
| Node / npm | 24.21.0 / 11.19.0 |
| Go | 1.27.1; server module requests 1.26.2 |
| Java for Android | OpenJDK 17, `/usr/lib/jvm/java-17-openjdk-amd64` |
| Android SDK | `/home/justin/Android/Sdk`: platform 36, build-tools 36.0.0, NDK 27.1.12297006, CMake 3.22.1, platform-tools, emulator, command-line tools, licenses |
| Android devices | `Portico_Phone_API36` at `emulator-5554`; `Portico_TV_API36` at `emulator-5556`; x86_64 images, KVM accessible |
| Vega | CLI 1.4.2, SDK 0.24.12112 under `/home/justin/vega/sdk/vega-sdk/main/0.24.12112` |
| Vega virtual device | Visible window; console 5700, device 5701, separate device-tool server 5039 |

Build both APK variants and all Vega packages with:

```sh
scripts/test-clients.sh build
```

The default Android ABIs are `x86_64,arm64-v8a`, suitable for the local emulators and 64-bit hardware. For a 32-bit Fire OS target, rebuild with `PORTICO_ANDROID_ABIS=armeabi-v7a,arm64-v8a,x86_64`. The APK variants have the same application ID and development signing key; installing one replaces the other and preserves app data. These are release-mode **testing** packages with embedded JavaScript, not store-signing releases. Metro is unnecessary.

Vega packages include `aarch64`, `armv7`, and `x86_64`. The build wrapper restores the source manifest after the SDK expands native dependencies. This was verified with a second successful build.

Vega interaction helpers run from `apps/portico-react-native/apps/vega`: `node scripts/vega.mjs key left select`, `shot /absolute/path.png`, and `logs 200`. Logs query the manifest's package ID, which differs from the `.main` launch component. Android logs use `adb -s emulator-5554 logcat`; select `emulator-5556` for TV. Screen recording and screenshots can contain account or library information; keep raw evidence under ignored `.scratch/`.

## Current artifacts and source

Active server/web source: `portico-server` **82aaa54f89efdc2e029b635746627cd8c8b71960**, including the [Home artwork fix](2026-10-09-home-performance.md) and [server/client/API performance changes](2026-10-09-performance-audit.md). Google/Fire TV packages were rebuilt from **a3d8e7cd14c16229715a07c67f43cb8364bae743**, including the Android keyboard-collapse correction. Vega packages remain **f0ed9115e6f20470c5aa06dab16f5f27a328251c**; the keyboard change does not affect Vega. All six repository baseline heads were verified against GitHub main before setup. Draft reviews: [server PR 14](https://github.com/PorticoMediaServer/portico-server/pull/14), [native PR 18](https://github.com/PorticoMediaServer/portico-react-native/pull/18), and [Roku PR 3](https://github.com/PorticoMediaServer/portico-roku/pull/3). The tested catalogue-source reuse, browsing-queue, measured statement-cache, checkpoint controller, startup memory default and bounded artwork-scaler changes are deployed.

| Artifact | Location relative to `/home/justin/projects/Portico` |
| --- | --- |
| Current Linux server binary | `apps/portico-server/.scratch/demo-releases/82aaa54f89efdc2e029b635746627cd8c8b71960/20261009T191717Z/portico-server` |
| Google Android APK | `apps/portico-react-native/apps/native/android/app/build/outputs/apk/google/release/app-google-release.apk` |
| Fire OS APK | `apps/portico-react-native/apps/native/android/app/build/outputs/apk/firetv/release/app-firetv-release.apk` |
| Vega packages | `apps/portico-react-native/apps/vega/build/{aarch64,armv7,x86_64}-release/portico-vega_<architecture>.vpkg` |

Checksums of the current files are recorded in [2026-10-09-setup.md](2026-10-09-setup.md). Build logs and screenshots are in the repositories' ignored `.scratch/` directories.

## Demo deployments and preservation

```sh
scripts/deploy-demo.sh <committed-server-sha> --stage-only
scripts/deploy-demo.sh <committed-server-sha> --activate-only
```

Or omit the mode to stage and activate together. The script builds a committed server/web snapshot on Ubuntu, embeds the existing official Hosted trust root, transfers it through `portico-demo`, verifies the binary checksum, then switches the service. This is an identified demo build; it does not claim a signed/sealed production release.

Activation stops the service, copies the full state and service/environment configuration into a root-only backup, verifies SQLite, and preserves all media files and previous releases. Failed activation restores the old configuration and state, retaining failed state for investigation. No empty-state option or automatic backup pruning is provided. A retained release cannot be restaged over itself.

Current release: `/opt/portico/demo-releases/portico-82aaa54f`; unit `portico-demo-current`; state `/var/lib/portico-demo-20260923`; fixtures `/srv/portico-demo-20260919`. The latest deployment backup is `/var/backups/portico/demo-20261009T191717Z`, with the populated state. Credentials were rotated again after this backup during interactive testing; restoring this or any earlier state requires supported owner recovery/password refresh and reconciling the protected active credential record. The original pre-upgrade backup is `/var/backups/portico/demo-20261009T145648Z`. Its owner credential predates the password refresh; restoring it requires another supported credential refresh. Check https://demo.getportico.tv/portico-build.json and `/v1/readiness` after any deployment.

The existing database was compatible: the original ten migrations matched the setup source; the later recommendation-signal index migration was applied normally. No manual schema edits were needed. There are 356 catalogue entities and 256 available playable files, all verified to exist. Movies, TV, Anime, Music and Audiobooks retain their metadata. Fixtures contain synthetic media matched to real titles, not the titled films or episodes themselves.

**Transcoding is disabled server-side** through the owner settings API and verified in the persisted playback policy. Preserve that setting on this single-core, 1 GB VPS. Direct playback and stream-copy synthetic live feeds are the testing scope; conversion performance requires different hardware.

## Next acceptance work

Use [the setup results](2026-10-09-setup.md) as the starting evidence. The Home timeout was traced to artwork query planning; see [the investigation and live validation](2026-10-09-home-performance.md). Review phone/TV navigation and layout against the Apple builds, then test real Android and Fire OS/Vega hardware, codecs, audio, seeking, resume, subtitles and library categories. The Android TV AVD runs Google's Android TV image; installing the Fire OS flavor there does not make it a Fire OS device.

## Current verification and access state

On active 82aaa54f, a 30-request VPS loopback check returned all 200 responses. Home averaged 105.84 ms across five samples (115.29 ms first sample, 103.48 ms mean of subsequent four). Discover averaged 35.90 ms across five requests per library category. The historical 8f92fd3d warm Home mean was 159 ms and Discover mean 70 ms. These measurements include server HTTP/authentication/serialization and exclude Germany-to-client network latency. They are sequential checks, not concurrent capacity acceptance.

The final local 200-browser/100-direct-stream test passes the stream and physical WAL gates but still fails browsing latency/refusal targets. Earlier runs also failed the physical WAL gate. Each sustained 100 actual media response bodies. Raw browsing refusals improved from the baseline 45.3% to 10.5% on four Go execution threads and 6.93% using all 12 threads on the identical updated build; successful logical throughput barely improved between four and 12 threads. The final canonical-fixture run on the local Ubuntu PC, with all twelve Go execution threads, sustained 100 direct media bodies and passed the physical WAL gate at 239.27 MiB peak; 639/22,681 clean requests were initially refused (2.82%), clean p95 was 2.625 seconds, and normal retry left 17/21,654 logical calls unresolved. It still fails strict browsing capacity acceptance. Historical fixtures differ, so this is not a percentage-only equivalent-input comparison. The large load tests run against a local test server, never the weak VPS. Recommendations retain their candidate selection, scoring, history and privacy behavior.

Justin restored the regular Ubuntu desktop and GitHub authentication on 9 October. The native a3d8e7c head is pushed and PR 18 is updated. Fresh regular Chrome renders the populated Home and recommendations without the former Home error, and a 45-second movie fixture reaches Finished without an HTML media error. Fresh Vega direct-device approval, Home and 45-second Solo Leveling episode playback also pass, reaching Finished at 0:45. Fresh Google Android phone direct sign-in, Home/Discover/detail, keyboard hide/reopen and 45-second playback pass. The fresh Fire TV flavor on Google Android TV passes direct device approval, Home/Discover/detail and episode playback through Finished. Detailed evidence and limitations are in the setup report. The visible devices are restarted for interactive testing; they were stopped during the completed local capacity run. Earlier smoke results remain historical, and these checks do not establish Apple parity or physical-device acceptance.
