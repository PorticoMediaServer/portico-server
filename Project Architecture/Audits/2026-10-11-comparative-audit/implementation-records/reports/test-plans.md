## Outcome
Completed fifteen environment plans and README under `Project Architecture/Audits/2026-10-11-comparative-audit/test-plans/`. They contain 203 numbered acceptance cases with prerequisites, OS/device variants, candidate/build/install steps, media preparation, smoke subsets, expected results, failure symptoms and owning lanes. README gives suggested order, the minimum end-to-end core proof, whole-product limits and source-note coverage.

Coverage intake contains 72 programme reports plus ten dated fix-pass reports. Duplicates are merged into case variants; historical executed checks are not presented as future device passes. No application code, API or schema was changed. No full build/test suite, device/browser/provider run, installation, deployment, push or Git commit was performed by this lane.

## Decisions I made
- Use isolated copied media/fresh candidate-baseline state; data-loss and first-build gates precede routine interaction. Upgrade fixtures share the current baseline digest; no legacy API/database compatibility is promised.
- Repeat essential setup/media/account instructions per environment so the tester can begin cold. Platform plans explicitly name build errors and remedies, signed Release versus Debug/Metro, and actual hardware versus simulator/fixture evidence.
- Use programme decisions over old private-only HTTP, Windows scheduled-task, title-only grouping and old Hosted-format expectations. Current Windows SCM, unrestricted ordinary HTTP, server-advertised broad sort/filter and Hosted format 21 are the tested expectations.
- Keep unsupported features hidden: TV downloads are excluded by the current native downloader; current Vega refuses protected SDK exchanges. Vega sign-in/playback cases are conditional and Blocked until a qualified transport exists, not a claimed working client.
- Treat absent hardware/formats/hooks as Blocked. Desired result is retained; passing a related source/helper regression does not close the physical case. No source acceptance target is weakened.
- Context-sync failed because its lock directory is read-only. Existing canonical project/standards/verification/attribution guidance was readable. Updated shared guidance revision 903b1fbea30aec5db763f43747827250650b340b was incorporated; relevant standards/verification were refreshed.
- The requested fix-pass/lane-reports folder is absent in this worktree. Read all ten reports from the read-only canonical internal repository instead. Source intake hashes below identify the reviewed inputs; later report changes need a further documentation pass.

## Bugs
This documentation lane has no assigned implementation bug list and cannot edit the read-only implementation trees. No bug is claimed fixed by this work. Relevant source-review findings are routed in acceptance cases.
- B-22.26 — not fixed here: e2e-hosted confirms the actual activation adapter sends one automatic request while the shared outage gate is open. Hosted real-driver and native code-screen cases preserve this failure boundary.
- B-01.23 — not fixed here: historical Apple explicit-signout causation remains unproved. Native restored-family/temporary-selection/late-restore cases require correlated family IDs and readback, not inference from old sign-out records.
- Immediate claim-to-owner admission race (e2e-hosted, no assigned B ID) — not fixed here: Hosted driver case explicitly tests admission immediately after installed/acknowledged, separately from the existing owner-link readback wait.
- Other inherited B entries in source reports are exercised by their case destinations in README. Static source presence and another lane’s pass are not relabeled as physical acceptance.

## API changes
None. Plans reference existing typed routes/current schemas, including API keys, paths-changed, webhooks, MCP, code flow, playback capabilities/replans, events, readiness and analysis job unions. No routes were added, removed or changed.

## Schema changes
None. Plans require fresh candidate baseline and explicitly preserve/reject incompatible pre-release fixtures rather than inventing migrations.

## Owner or viewer settings added or wired
None by this lane. Acceptance cases cover wired background speed/pause/windows, media/conversion/cache limits, custom FFmpeg, home networks/proxy/certificate/capacity, providers/artwork, missing cleanup/deletion, downloads, approval/ratings, quality/player/caption/audio, autoplay/protection, report retention and integration scope/rate settings.

## Needed from others
- Supply exact integrated server/native/internal candidate SHAs and matching verified artifacts/copied packages. Main-server was observed at 7488da82d5803a63777f3f78d38d3e98e9db5c69; main-native at 261783468807925f45257ad9fdce5a831227372c; Hosted followups at b35f96220c2beb160412c2a16049c4f63fd8f732. These are source observations during verification, not a release freeze or proof all snapshots have landed.
- Tester needs actual Windows/macOS/Linux package hosts, Docker architectures, GPUs/vendors/real sandbox, browsers/screen readers, physical Apple/Android/Fire/Vega devices, tuner/lawful IPTV, private provider accounts, test callback/receiver domain and lawful measured media/reference fixtures.
- Native test targets may need qualification-only XCTest wiring and fault instrumentation. Keep missing hooks blocked; do not weaken production fences to run a test.
- Integration must close Vega protected transport, the e2e-hosted client gate/admission gaps, any remaining live-fMP4/Apple compressed-audio handoff and runtime failures before claiming all-client completeness.
- Refreshed final notes from still-active e2e-server/e2e-web/scale/decision/native review lanes need a subsequent plan update if they add requirements beyond this intake. Their recorded current scope is included; unfinished evidence is not silently made final.

## Test notes for other environments
The fifteen environment documents are the deliverable; README indexes all of them and routes each source report to merged case numbers.

Documentation verification on mini-1 completed 2026-10-13T16:21:37.404997+00:00:
- All fifteen exact requested environment filenames plus README present; 203 sequential case numbers, expected/failure/routing fields, nonempty smoke subsets and Intel/AMD/NVIDIA/Apple sections validated.
- Every local Markdown file link in the plans/index resolves. All fences balanced, no TODO/TBD/FIXME placeholders. All 27 shell fences pass bash -n. No shell snippet was executed as an environment test.
- Twenty-two referenced build/fixture/helper/API source paths exist. All seven named Windows storage-test prefixes match real tests; current Swift mpv helper source has eight methods including the conditional controller test. Actual native scene manifests, TV downloader exclusion and Vega pre-dispatch refusal were inspected.
- Verified package script argument order/artifact names, service users/paths, Docker init/health/volumes, exact native build pins/platform floors, and current typed API guides/source where reports were vague. git diff --check passes. No source test suite was run for documentation.
- Snapshots 001–004 captured drafts at logical milestones. Final cumulative snapshot 005 follows this final report/verification and includes all sixteen finished documents. No worktree edit follows it.

### Notes that could not become fully executable physical cases
- `e2e-server.md`: its test-note section has only the fresh-loopback reusable-runner scope, without a finalized runner command or cold environment script. Standard server/catalogue/playback cases cover that scope; the future runner itself cannot be handed off concretely yet.
- `scale-check.md`: exact broader measurements and final commands remain pending; there is no established mixed 200000/2000000-title capacity result. The actual scalebench CLI/source supplies a concrete independent procedure in Linux case 13, with no performance pass claimed.
- `decision-audit.md`: no finalized cold acceptance script in the reviewed intake. Authoritative programme requirements are already represented; no nonexistent case or finished evidence was invented.
- `e2e-web.md`: browser walk/runtime fixes/final snapshot were still in progress; test note names scratch, Chrome and production targets without a finalized future script. Browser cases are concrete; that lane’s unfinished walk cannot be treated as final coverage.
- `review-native-js.md`: its current Test notes report focused regression logs, not finalized device steps. Source findings were retained as delayed-restore/player-intent/grant/receipt/enrollment/viewer-fence variants. Deterministic physical invocation still needs instrumentation.
- Deterministic race hooks for post-unlink authority refusal/new attachments, writer gate hold, source listing-page replay, callback injection/Keystore removal, delayed player surface, poison/version changes and DVR crash-after-seal are not supplied as a ready physical harness. Plans specify expected boundary and existing helper/source regression, but cannot supply an absent production/debug control. They are Blocked unless tester has an explicit qualification hook; ordinary manual variants remain executable.
- HE-AAC/PS priming, DV7 enhancement fidelity, real disc DVB/VobSub/PGS glyphs, physical HDR/receiver mapping, external provider/ACME/public endpoint-wake behavior cannot use synthetic stand-ins. Plans state lawful fixture/prerequisite acquisition and exact observations; missing materials block the applicable variant.
- Vega’s protected network boundary prevents authenticating to execute its downstream cases. Its safe refusal is testable now; removing it is another lane’s implementation, not a test-plan step.

## Not finished
No documentation deliverable remains unfinished. Hardware/runtime execution is outside this lane and has not been done. Missing hooks/fixtures, Vega transport and still-active source reports are explicit acceptance gaps above, not passes. A later source/report update is required to absorb notes written after the coverage intake.

### Reviewed source-note intake

| Source | SHA-256 |
| --- | --- |
| accounts-security.md | `591860712c5f486672331963b8e8be098d744c5940a2fa36830c3d11336c93aa` |
| android-mpv.md | `50ccd71dfd624c328ee83518a8cba26204f8602c4b68b2c9e62136b07da0fab2` |
| android-native-review.md | `795ab6b7692a6ca93c908c3de67c9a50f11265ff93708ecd77a502fa25681b98` |
| api-platform.md | `c37629a8cb2cf31c440ea30209dd99d091420ae741f33b5f17805b1aba9cb7e1` |
| apple-mpv-review.md | `b54a4db55a54adcaaca477bb1c14c50e67a5091fc3424a6d2bb85792082f7a32` |
| apple-mpv.md | `30fbfe546afb64ab1eac79cd74d25de74f0dffceb8c3f20b0728da007f6378bf` |
| apple-native-review.md | `77f7cbff9246989b638cb20b91adb5f2b73926029a5fc18b5fffb0386aa12fba` |
| audio-engines.md | `ecf11d1ef755df23fc76fc0f620842c7af2172042b8b5314c16896460990b8c5` |
| base-health-2.md | `b597968e3778112088aff736ffbdea3ffaa81b6cded556f3fff07e848f91a139` |
| base-health.md | `92774fb5a7e11546354456de490378ff844c3748695dbec78261928eea8464d4` |
| console.md | `e1aa9e5f1cbf98fe2d3f29558475135152ba5b3af6fb1dbeebcf663a37d91506` |
| conversion-outputs.md | `b1df0cd7abec53f9938850d28264bcfffac869f2aceb637de7c229633255de6e` |
| decision-audit.md | `b37256c302358ab93e95d4a89c7a92ead26c60a4fe5a4918085816f4d3fb650e` |
| docs-internal.md | `21db7495aff8776d15f838ca397f863275cf77745ea948cc15533f70d84873fb` |
| downloads.md | `26a728d9b087c5af303cec8bcdb62cf287073bd90037db0acca217a65f7da45e` |
| e2e-hosted.md | `16aca4d266ccbbccd31ad05f50839ba122306d6613a7021c93abee404492294a` |
| e2e-server.md | `1da685d5c88f4456fabbc211c2d861f77e37e80f6bd0db30f2d8327f5ec7b650` |
| e2e-web.md | `a91c98fdf791f30b89bbb3ccd324b1900d0487116c3f9660da9b0923972ef580` |
| freshness-clients.md | `c3fc70b2b3b882de070e7c2af7cb3c7d10ed5689f8843f16a444cad677c38b37` |
| freshness-server.md | `ad1d3a4ebf4cc1c9625c394bc7e9da6425b2710de072611df5eb907bc6e49e3c` |
| grouping-sorts.md | `075ac840646bd4193e20f2ef12b0ba7e40cf4ce9821f1f46a5e023e2a24e27b2` |
| handoff-draft.md | `6f6a8d9a619f54f6be737da48f08fff1413f74d89ef8196406156ec153de3b9f` |
| hosted-followups.md | `a643da04ce766f8388a0294481a00bc977f4f1f63c6b8bdc217b03b1ba629c88` |
| hosted.md | `8b4a2f46994b56b2d5059ac2d8e63c356a720807e903d8e96d2f217f28ed4a3e` |
| identity.md | `42c6606eef99acf055564bbb729aba50cf67ae63e42ad785b18e15f0cca3f6a9` |
| integrator.md | `00fc78b2f681fd9fbd4857ed3e87ab7b985addba44d1d2c4a583591bd39d6076` |
| leftovers-native.md | `251f21a065be94070a4285514bbae381cb2a89dd38276b3a2aaa9ffa7b8c8050` |
| leftovers-server-a.md | `6d5206af506ebd8720ff167b5ddaa1e5144423cef750dc6c41992aacc9f535b4` |
| leftovers-server-b.md | `bb1107c9a4843356946692d219f88e990bb03422c19224a944ba0e9470b3859a` |
| limits-settings.md | `19715d7a5a857d853120a305d2e9f2515f8ae3e84c1231c7fa80ccfd89c8253d` |
| livetv.md | `30eccf036c5bac31517e858bc3e9a50bf860ee053942a22cb490e71142f26f46` |
| music-audiobooks.md | `0a828c579dfe46af7afc7c663b097ff765fbddd77807c0ca9a3460d9bd4289aa` |
| naming.md | `010631b783484a5ada19d8a22a3c12013cc36a46e1fc1c69e129ce0d8f68fbd5` |
| native-app.md | `a9ada4a75ea86ab3dde4865bd9d7d5c21001a1a6351a588515afcafec126b5cd` |
| native-health.md | `6036161a237c960ddd7a4cd3dc2ec0d2dc6c63056bbe37a1379a32f2257e59bc` |
| native-playback.md | `f674ec9d6374599248d69c951e9342c0c9ee0740767e6c4e256410b423a19f7d` |
| native-sync.md | `4782ea379301cad0c4ec4d1877bc82a1deabae3f3d0cbdb5ba9de2094e7e887e` |
| networking.md | `1f3b57c15adff1ff1d28f8cfdf1cd8def0e1a704db5268bb242380272dd9ab09` |
| ops-packaging.md | `c3c014571fb82cf3b741d2c5e5c143a66281b8b83bf5369b68bd8419f518f2b4` |
| personal-state.md | `6b7633cb428e16d5ed40087c0ac1aa68fa1684013daf97d13f054e1e3d14bea2` |
| planner-wiring.md | `c5cddbec08340049478d0c05d67f9e56a3ef05e77939e3a00bcbcc3d348e32a7` |
| playback-core.md | `2f087e3a12569905f13477b8f2f6be0b5ab998ba715c7593a7c23cd199a22198` |
| providers-matching.md | `7e3a73589d3606e035a49e3782cd70cae57dd509d50a1528ad91ae500a1bdc0d` |
| registry.md | `c0966b5f149f58a984e730de64256f2df4731a362caaf4f5ef802bd8110682a1` |
| review-accounts-security.md | `2660937a43b2ea4beaa83976f01aed9228997163ee07133b011b93c5f3fa1adc` |
| review-api-platform.md | `e6e5136decf9730d219ca47df25a61a9329c22fb9efc77cdc85ae7737f917b97` |
| review-downloads.md | `c3ebe335e696fd280a7b62a4d2ed668d5d2ed13e10be798502ba1209e5f4e67a` |
| review-freshness.md | `11f2e25bb1f5ce630b2f439e03f13ddfc204f3ee150ac5ee4f406ccd55a0cdaa` |
| review-hosted.md | `72ad837a4700932fc70a76c173c548209ef717499e3b809dff9e47f82ed7b767` |
| review-identity.md | `323a6bc430e93f53fe0594a4fca1ee53500c282b9a6e069b4cf17b57e18d57c6` |
| review-livetv.md | `c4c0cf18dbfcb7409260ca976279955ebd3372c80d203b0ad0cece3b278444be` |
| review-native-js.md | `c01c256ef7adeb8b1749cf2c2e4e3a5e39a774ffe9d76e48a313b1809a3f00ea` |
| review-networking.md | `a1ce703b15785f6a6582e54c4412f7f96008e3510902dd3c2561dc9a439a776c` |
| review-ops.md | `09a9d8d159b81c7ee4e048b4af9e48c76ff654a946d6b7ff050e92d76cf6f2dd` |
| review-playback-core.md | `a93d176e378e4962275b054c60e86c013ed5482bc82db9128a999cb232dc449a` |
| review-playback-pipeline.md | `6c6e89e24c6eaa10570bd1139c8f7efcf6ba9eaa19daca539f434e1f193d06ef` |
| review-registry.md | `901ddbbb5c1408ca01d46f75c2b9e0ccf99eb312a0d399497fd0684fef783c75` |
| review-scan.md | `adaf49fbf7846835423b89753d873cb19cb938c0d0c2032fc47ab20f3be9497c` |
| review-server-late.md | `7409d689959ef9deaace624881aa8563acafba9e19c8a46af5fd9127fc262518` |
| review-web-logic.md | `c4efa689e240e7f97bd45c9d4eb317312f1939507812c55f84ecd727b4546908` |
| scale-check.md | `96a65e0e2632ab6441a4fc853e1b87cb6c7685a03db0c60ddbed0a614783fb98` |
| scan-loop.md | `a8b58858a9134db20a562b13cd28697112bbb535ba4154899493ecae55258839` |
| search-sort.md | `71894f5159ee2f1ee91afe0b053a8625db19c7c792118bef3c67c75e419e5469` |
| security-sweep.md | `fed24c1244ee32a71e53b43082a3d39b123806b668c75e9d41513a65780c465c` |
| server-docs.md | `02a9f47d96a4709ba2aaa4783c57913c2f509f086133964b2b93de1f2f9b5aa6` |
| server-followups.md | `51d9295ff06e8d7f35aa64a3804cdf85d818c1d9246b7e26d74811bae2239b46` |
| social.md | `8c79c6c96956bee4da46e96378976a2cf0a157ceb12b59b7feafb00bfdcb6d26` |
| subtitles-text.md | `9763d19a0e4e1b67d0a08405f33385abee23a719486187ec662e16cce2d4c969` |
| vega.md | `cb547c7a627079b9235577990b148da5b5612a714a0f7ae519b4cff19adf6c3f` |
| web-app.md | `1ae3eb74496d0b92ba223822015acda5d35b89eb1c3eead00c3681eefe340287` |
| web-player.md | `a2fa66e3b85e4e10fc514edd013d5131ed6bd8c6c5b7b97184fcf2ef39312523` |
| web-subtitles.md | `3ff98f83140536567c309c5628c013d5d8e518a45055ffddbb037b428c0698cc` |
| fix-pass/report-catalogue-playback.md | `bf8c4a3f7c8c233d81c14e1e5d4693cddb7431284738e5ff0a93b8cd7a81025f` |
| fix-pass/report-hardware-conversion.md | `1ea7451a0fb6177c56f9777ef2670756ff99c9df885824139bced52f6f88bb94` |
| fix-pass/report-hosted.md | `728b7c59e9f49267cc9a4a915fd1feeb50f59d8b48391fc353cb095930ff33a7` |
| fix-pass/report-identity-ops-live.md | `cb4fe0876800dd95ef1f394e0029ca061ecc487bb6aa3d7151f0df9aaa80de4e` |
| fix-pass/report-native-platform.md | `37db31b8e11cf34385cfc81e314aaf43ad162898ad30d78a8c545850197b6247` |
| fix-pass/report-native.md | `5655aa0cf91ec3940c8839c488e662d056a0f21c2374cf8f93dc060f025001a9` |
| fix-pass/report-platform-packaging.md | `5e274926a0d96a0c553bddbfc058d2976716311a14a8c6779afa59c1b0d9c026` |
| fix-pass/report-scanning-metadata.md | `f75ddd65ada5d4333cff51bf248f02dc1dce6ac86a12dc92438da3ea08e757d0` |
| fix-pass/report-social-downloads.md | `0256e59b223f3bdcefefe4132433d12689e5b343f150d4c66cccb89ca5124a46` |
| fix-pass/report-web-client-core.md | `079eac0783ece64179e9269e351d926a3a9d96e39866365b1f632b024c040261` |
