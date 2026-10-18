# 25 — Engineering practices and project model

## Scope and sources

Audit date: 11 October 2026. This is a static, read-only review of the working trees on personal Ubuntu mini-1. No build, install, test, formatter, Go tool, server launch, browser, device, Hosted endpoint or private credential store was used. Independent Python text/file measurements and read-only Git history inspection supplied the counts below. Context-sync could not refresh its lock on the read-only filesystem; existing project orientation and verification guidance were read.

Portico paths below are relative to /home/justin/projects/Portico. Comparison paths start with the cloned repository name. Sources actually inspected:

- Architecture: 01 Overview, 02 Server, 08 Operations and Backlog; Product Contracts governance, conformance, scope-completion, coverage and registry. These distinguish required behavior from tested completion.
- Public server: README, BUILDING, INSTALLING, RELEASING, CONTRIBUTING, SECURITY, SUPPORT, TRADEMARKS, THIRD-PARTY-NOTICES; Go/npm manifests; Makefile, Dockerfile, dockerignore, all workflow definitions; release builder, package builders, container identity writer, public update writer, runner tier handling, FFmpeg locks/notices and hls.js provenance.
- Server internals: package/file measurements and imports across cmd/internal; composition root; router dependencies and error mapping; catalog service/context wrapper; persistence migration and SQL guards; maintained SQLite provenance/cancellation patch; supervise guard; API contract assertions; test-tier and decoder-helper behavior; hostile-input and fuzz entry points; update feed fetching/parsing; local telemetry. Representative tests were read, not all 815 test files.
- Native/Roku: shared-package copy script, native and Vega manifests, native CI/security/dependency jobs, Apple Podfiles and Android dependency declarations, native test harness/examples; Roku manifest and CI/build/test approach. No feature audit of Roku was attempted.
- Private engineering: Hosted module layout, CI, shared-apikit script and actual copy comparison, build/deploy script structure. Public website documentation tree, update guidance and privacy source were read locally; no Portico endpoint was contacted.
- Jellyfin: solution/project references, central NuGet inventory, build/analyzer defaults, banned symbols, application DI registrations, Items controller signatures, integration host factory, fuzz project guidance, test/ABI/OpenAPI/format/security workflow definitions and documentation release/governance/AI/contribution guidance. jellyfin-web quality workflow and contribution/legacy architecture were inspected.
- Plezy: license, analyzer rules, CI and release job structure, contributions and Maestro guidance, dependency manifest, updater/signing paths and Sentry startup controls. Navidrome: Go dependencies, provider sets, Makefile, build/release pipeline, Docker cross-build structure, GoReleaser, embedded resources, insights defaults and collector data. Audiobookshelf: version, CI job inventory, migration manager and public releases. Swiftfin and Android TV licenses were checked.
- Public primary sources were consulted for licensing, privacy, funding and release communication: FFmpeg legal documentation, VideoLAN iOS COPYING, GNU license guidance, Jellyfin documentation, Plezy privacy/site, Immich upgrade/migration/funding statements, Plex Pass, Emby Premiere and Firecore product information. Links appear with the related finding.

Snapshot identities: portico-server 82aaa54f89efdc2e029b635746627cd8c8b71960; native a3d8e7cd14c16229715a07c67f43cb8364bae743; Roku 3637b7c0bcb6839e3803614db2a108a2604071af; internal 19cc2a7eb6fb40ce675decd65036769c95b42c5d. Working trees had no tracked changes shown by git status at inspection; the audit directory was untracked in internal. Comparator clones are shallow: their commit counts cannot establish lifetime activity or team size.

“Confirmed by reading” means a deterministic source relationship, not a reproduced runtime failure. Security, legal, delivery and required-check settings outside the checkout remain unverified. Absence findings are limited to the inspected tree. No claim that a workflow has successfully run is inferred from its YAML.

## How Portico does it today

Portico is a **split-repository product with a partial monorepo inside the server repository**. Server, web UI, canonical client-core/design/i18n packages and Go apikit live together. Native, Roku, public website, Hosted/internal operations and recommendation enrichment have distinct repositories. Native copies selected canonical files so it can build independently; Hosted copies apikit. The native checker compares filenames and SHA-256 contents; it does not store an upstream commit identity. It covers 215 client-core files, six design files and 16 i18n files in the declared selection. All those selected files matched during this review. Hosted's 19-file apikit copy differed in generate.go. Sources: apps/portico-react-native/scripts/shared-packages.mjs:16; portico-internal/scripts/shared-apikit.sh:9; Project Architecture/01 Overview.md:21.

The server is one Go application module with a sibling module replacement for apikit. It uses net/http, database/sql, explicit constructors and function callbacks rather than a general dependency-injection framework. cmd/server/main.go is the composition root: it opens state, supplies trust/configuration/tool paths, wires services, starts workers, composes the HTTP dependencies and drains on signal. Go internal directories keep these packages private to the application module. Most server code is handwritten; no standard “Code generated” or “DO NOT EDIT” marker was found in the counted Go source. known_schema.go is mechanically maintained despite lacking that conventional marker. The roughly 360,000-line description includes tests; it is not 360,000 lines of production application code.

### Measured server shape

Physical line counts include comments and blank lines. Counted .go files under server, excluding scratch/worktrees and build artifacts; imported upstream generated modernc library source outside this tree is not counted. There are 93 Go-bearing directories, 89 under internal, 1,109 non-test Go files and 815 test Go files: **232,280 production lines and 122,022 test lines**, 354,302 combined. Test/production ratio is **0.525**; tests are 34.4% of the counted total. These are neither code coverage nor a quality score. The maintained SQLite driver accounts for 7,522 production lines.

| Package | Production lines | Test lines | Production/test files | Test/production |
|---|---:|---:|---:|---:|
| catalog | 30,857 | 18,648 | 121 / 114 | 0.60 |
| httpapi | 23,193 | 29,564 | 120 / 162 | 1.27 |
| metadata | 16,650 | 7,808 | 69 / 38 | 0.47 |
| playback | 13,290 | 7,237 | 54 / 42 | 0.54 |
| networking | 10,955 | 6,906 | 55 / 47 | 0.63 |
| playbackv1 | 8,205 | 644 | 26 / 8 | 0.08 |
| identity | 7,687 | 4,362 | 32 / 27 | 0.57 |
| operations | 6,875 | 2,722 | 23 / 19 | 0.40 |
| compactcatalog | 6,670 | 1,705 | 24 / 17 | 0.26 |
| administration | 6,666 | 2,165 | 27 / 14 | 0.32 |
| dbwork | 5,222 | 4,288 | 27 / 22 | 0.82 |
| social | 4,521 | 540 | 12 / 4 | 0.12 |
| persistence | 3,741 | 2,703 | 13 / 24 | 0.72 |

Low local ratios can reflect testing through httpapi; they do not establish that playbackv1/social lack behavioral coverage. Conversely, a large httpapi test suite can make a small domain change expensive to verify.

Largest files: persistence/known_schema.go 1,727 lines; httpapi/router.go 1,407; maintained sqlite/conn.go 1,380 and sqlite/sqlite.go 1,336; playbackv1/queue_commands.go 1,245; catalog/home_rows.go 1,224; metadata/screen_publish.go 1,142; playbackruntime/linear.go 1,094; cmd/server/main.go 1,067; playback/hls.go 1,060; networking/remote_manager.go 1,050; operations/notifications.go 1,028; compactcatalog/write.go 1,004; catalog/rec_engine.go 999; httpapi/social_routes.go 996. These are file lengths, not function lengths. Splitting files alone would not change cohesion.

Production-import scanning gives cmd/server 50 distinct internal dependencies, httpapi 45, playback 21, catalog 18, playbackruntime 17, preparedmedia 15, metadata and ingestion 14 each. No direct reciprocal import pair appeared in the filtered application source. That is a useful sign, but Go already disallows compiled import cycles, and a text scan does not resolve platform build conditions or runtime callbacks. catalog.OwnerMetadataRevision explicitly avoids a package cycle (catalog/catalog.go:75). The graph is closer to a broad dependency DAG than a strict controller/service/repository stack.

The most consequential import hubs are below. Production imports include compiled-source candidates and production test helpers, exclude scratch and _test.go, and do not imply that every caller is user-facing.

| Hub | Distinct importing package directories | Direction / maintenance implication |
|---|---:|---|
| dbwork | 40 | Shared persistence/control substrate; changes can affect almost all domains. |
| identity | 27 | Authentication/authority and utility identity types are a broad dependency. |
| compactcatalog | 15 | Low fan-out (four dependencies), high reuse; centralized catalog representation/publication is a sensitive seam. |
| catalog | 11 | High fan-out (18) plus many service callers; strong candidate for cohesive area boundaries. |
| persistence | 11 | Low fan-out (two), but schema ownership is shared with direct SQL callers. |
| operations | 10 | Also imported by domain services, so owner-console/job policy is not confined above the domain layer. |
| httpapi | 2 | Composed by cmd/server and cmd/apigen; broad outward dependencies but no reverse domain imports in this scan. |

That last property is worth preserving: domains do not import HTTP transport. The cycle risks are **semantic**, such as callbacks calling back into services that share a write gate or startup constructors waiting on each other, rather than an existing directly reciprocal Go import. No deadlock or longer import cycle was demonstrated.

SQLite is the pure-Go modernc.org/sqlite v1.58.0 library with a **locally maintained handwritten driver** at internal/thirdparty/sqlite, not mattn/go-sqlite3. Its progress callbacks close a cancellation gap and account for prepared-statement memory. Portico uses CGO_ENABLED=0 for shipped server builds, which simplifies cross-compilation. FFmpeg remains a separate native executable bundle, so a pure-Go server does not make the entire distribution platform independent. The module declares five external direct requirements in its first block, one local direct apikit module and nine indirect modules. Sources: server/go.mod:5; internal/thirdparty/sqlite/PORTICO.md:3; Makefile:22.

Persistence owns migrations/schema integrity; dbwork owns snapshots, write priority, retries, prepared statements and WAL maintenance. Domain packages also write SQL directly, often taking authorization callbacks into the same write transaction. catalog binds a request context by copying a small service wrapper while sharing synchronized state (catalog/reads.go:85). The request deadline therefore reaches query execution without changing every older method signature. Older context-free APIs remain callable, so this is a convention that must continue to be policed.

The engineering defense is unusually product-specific: panic supervision, SQL preparation and query-plan guards, revision-aware caches, hostile request cases, API response-schema assertions, deterministic synthetic catalogues, FFmpeg qualification, string/design linting and preference-consumer checks. Default tests skip media-tier tests; media/all enable them, and performance proofs have a separate environment selector. Five Go fuzz entry points exist under httpapi. Ordinary go test executes their seeds, not an extended fuzz campaign. Most client tests run under node:test and custom loaders/component harnesses; those exercise behavior but do not prove browser layout, React Native rendering or device media output.

Public server CI runs repository checks, Go tests/vet, contract generation checks, shared-core tests, string/design/type checks and a web build. Security YAML adds gitleaks and CodeQL. Native CI checks copied packages against a separately checked-out server default branch, typechecks/tests native JS and assembles Android debug variants; Apple and Vega builds are absent there. Roku generates/checks/tests/packages with sibling server/native checkouts. Hosted CI tests three modules and regenerates its contract, without running the apikit copy check.

Releases use SemVer tags, a separate manually qualified FFmpeg component, snapshot source digests, embedded build identity and Ed25519 server manifests. Public installers are previews, unsigned by default. Docker has an unprivileged user, tini and a healthcheck. The public update writer produces an unsigned flat stable-channel manifest; the running server's notice checker expects a different channels-map shape. Automatic application is deliberately false. The signing-custody document explicitly says enforcement is unfinished (server/api/release-signing.md:9).

Hosted engineering is a related but separate build model: hosted-services/hosted (API plus web-host commands), dns and copied apikit are three Go modules. Hosted uses pgx/PostgreSQL and established OIDC/OAuth libraries rather than extending the local SQLite/identity implementation (hosted/go.mod:5; dns/go.mod:5). build-hosted-release.py captures private modules plus public web/core/design/i18n/hls.js into a checked snapshot, records includesWorkingTree:true/proof:unsealed and builds artifacts. Its own body runs module verification/builds and a Vite web build, not the CI test suite; CI exists separately. Accordingly a private release needs evidence tied to the captured digest, not just the nominal current commit (build-hosted-release.py:99,105,112).

Deploy scripts verify digests and switch versioned paths with rollback/readiness checks; demo deployment archives a specific committed server SHA and backs up stopped state before activation, whereas Hosted build explicitly permits a captured working tree (deploy-demo.sh:17,78,81; deploy-hosted-release.py:48,86; deploy-dns-release.py:31,55). These are useful release-engineering safeguards. A Hosted binary readiness check does not prove account/mail/ACME/DNS/SSO journeys, and the demo rollback path does not establish every public installer update path. None of these scripts was executed.

Governance is sole-owner and deliberately closed to external code pull requests. Issues, feature/compatibility reports and documentation corrections as issues are welcome. Product contracts preserve intent, require independent review and refuse to label documentary coverage as implementation qualification. The actual independent human review rate is not observable from these trees. Public Git history starts with an 8 October baseline for server/native/Roku/internal; older provenance is available only through referenced archives, not normal current-branch blame.

## How the others do it

### Jellyfin

Jellyfin organizes the server as multiple .NET projects: MediaBrowser.Model/Data hold types, MediaBrowser.Controller/Common expose interfaces, Emby.Server.Implementations and Jellyfin.Server.Implementations implement services, Jellyfin.Api holds controllers and Jellyfin.Server composes the host. Project references make parts of the dependency direction explicit. ApplicationHost registers services through Microsoft dependency injection; controller constructors receive interfaces and ILogger rather than one application-wide dependency object. This is real layering, but not a pristine clean architecture: legacy Emby/MediaBrowser names, both old/new implementation projects and broad model/framework references remain. Sources: jellyfin/Jellyfin.sln:1; MediaBrowser.Controller.csproj:24; Jellyfin.Api.csproj:20; Emby.Server.Implementations/ApplicationHost.cs:510.

Directory.Packages.props centrally pins NuGet versions. Nullable types and warnings-as-errors are enabled, with explicit vulnerability-warning exceptions; custom analyzers and several dependency/threading/StyleCop analyzers are Debug-conditional. Its unit/integration test workflow uses Release builds on Linux, macOS and Windows and collects coverage. Thus “all analyzers gate every test build” would overstate the source. Integration tests boot the real application with WebApplicationFactory, temporary state and targeted mocks; naming tests and SharpFuzz projects preserve parsing scars. ABI comparison reports plugin binary-interface changes; importantly its diff command tolerates failures to make a report, so it is visibility rather than an unconditional breaking-change veto. OpenAPI jobs generate base/head artifacts and a diff report. CodeQL, formatting and issue/PR automation are separate jobs.

Jellyfin is itself split across server, web, SDKs and platform-client repositories. Shared protocols and released SDKs provide common behavior, not a single cross-platform presentation implementation. jellyfin-web explicitly has legacy/modern/dashboard/wizard apps, migration debt and TV limitations; this is direct evidence that separate UI implementations age differently, not a claim that every Jellyfin client is poor. Its quality matrix runs JavaScript/CSS lint, target-syntax/build checks and tests, plus dependency review. Weblate owns community translations.

The core team has merge authority and platform responsibilities, with open contribution/review guidance. Release procedure describes golden nightlies, a freeze, at least 48 hours of testing and stable branch backports. The inspected 2026 document has moved to major/hotfix X.Y release labels while project package versions remain three-component .NET versions; older 10.X.Y descriptions should not be projected onto this checkout. Public forum/Matrix channels distribute support/ownership beyond one person. OpenCollective donations fund infrastructure rather than paying developers, while individual developer donations are separately described (jellyfin-docs/docs/general/faq.md:51). Sources: jellyfin-docs/docs/general/about.md:21; contributing/release-procedure.md:12,33,81; jellyfin-web/CONTRIBUTING.md:22.

### Plezy

Plezy is a Flutter application monorepo for a client rather than a replacement media server. Shared Dart behavior is surrounded by native Android/iOS/tvOS/macOS/Windows/Linux code, local plugin forks, relay protocol/server and website tooling. It supports Plex/Jellyfin/Emby, so server-specific adapters create a compatibility dimension Portico avoids. Its license file is GPLv3; that file by itself does not establish a store-distribution exception.

The inspected CI has analysis/code generation/format checks, Dart and widget tests, Android JVM/C++ tests/lint, Linux sanitizer/native tests, Apple native xcodebuild tests, Windows native tests, dependency checks, relay and website checks. CONTRIBUTING documents Maestro Android journeys against a disposable pre-seeded Jellyfin fixture and manual classification of destructive profile cases. This is more than a cross-platform compile matrix.

Its build workflow creates signed Android packages, an unsigned iOS build, signed/notarized macOS distributions when credentials are available, Windows/Linux packages and Flatpak verification; macOS/Windows updater feeds carry Ed25519 signatures. Store delivery is split between that workflow and a resumable maintainer script: scripts/release/deploy.py:14,23,25,26 handles Google Play, signed iOS and tvOS App Store Connect uploads and draft-release publication. .github/workflows/update-packages.yml:63,102 updates Homebrew cask and WinGet after releases. tvOS has no dedicated build.yml job, but does have an explicit archive/export/store phase in deploy.py. This is a useful small-team model: one shared codebase plus platform-native build/signing and resumable delivery state, rather than pretending one Flutter build delivers every store. Presence in a store is not legal proof and neither six directories nor one language establishes testing parity.

Plezy official GitHub release definitions enable update checks, donations and Sentry. Its Sentry controls disable performance sampling, session tracking and HTTP breadcrumbs, install redaction callbacks and expose a user crash-report preference, defaulting on when enabled in the build (lib/services/settings_service.dart:557; lib/main.dart:1096). Update checking also defaults on for eligible official builds; packaged Windows installations delegate updates to the Store (services/update_service.dart:34). Portico's local diagnostics avoid that automatic external crash channel. Sources: plezy/.github/workflows/ci.yml:166,255,325,456; build.yml:36,154,222,541,656; CONTRIBUTING.md:48; lib/main.dart:277; analysis_options.yaml:15.

### Navidrome

Navidrome is the closest implementation peer: Go, SQLite and an included web UI, but primarily music/OpenSubsonic. It separates core, server, persistence, scanner, model, configuration and adapters, with Wire provider sets and generated DI. It uses mattn/go-sqlite3, so cgo and native cross-toolchains are still part of packaging. The prompt's historical TagLib/cgo starting point is no longer accurate for this clone: adapters/gotaglib/gotaglib.go:1 uses a pure-Go WebAssembly (WASM) TagLib implementation with a forked module. WASM runs compiled parser code inside a runtime; it is different from linking TagLib C++ through cgo. The adapter explicitly contains parser/runtime panics (gotaglib.go:61), and no old adapters/taglib directory is present. The Docker build/xx wrappers produce target binaries, and GoReleaser packages those binaries rather than compiling twice. UI/resources are embedded for a simpler standalone deployment. Sources: navidrome/go.mod:5,33,40,62; core/wire_providers.go:18; release/goreleaser.yml:5; Dockerfile; resources/embed.go:13.

The inspected 0.x Navidrome source has a breaking OpenAPI diff gate and migration validation, but this review did not establish a formal multi-version/LTS or major-only-breaking guarantee; it should not be inferred merely from SemVer-shaped tags. Its ecosystem use is much broader: chi, Viper/Cobra, goose, SQL builders, Ginkgo/Gomega, logrus, Prometheus, web/format/tag libraries and WebAssembly plugin support. Its pipeline includes race/shuffle/coverage tests, Windows tests, JS lint/tests, migration validation, API lint/breaking diff and cross-binary smoke tests. The Makefile exposes the same operations. Insights collection defaults on with an opt-out, while development Makefile disables it. It collects aggregate installation/environment/library/player/plugin information; this is a materially different stance from Portico's local owner telemetry, even without sending song titles. Sources: Makefile:8,65,68,123; .github/workflows/pipeline.yml:126,164,314,438; conf/configuration.go:1046; core/metrics/insights.go:168.

### Audiobookshelf, Immich and commercial references

Audiobookshelf's repository has separate unit, component, integration, OpenAPI, translation and CodeQL workflows. Its MigrationManager tracks database/current/max server versions, so upgrade and downgrade handling is an explicit application concern. Its 2.x release practice is a useful model for visible bug-fix and migration notes, but release frequency is not a reliability score.

Immich's current published upgrade policy limits intended breaking API/deployment changes to major versions and has a dedicated v3 migration page enumerating removed/deprecated APIs and environment changes. That is a communication practice worth copying; it does not prove effortless upgrades or rollback. [Immich upgrading](https://docs.immich.app/install/upgrading/), [v3 migration](https://immich.app/blog/v3-migration).

Funding differs from architecture: Plex Pass, Emby Premiere and Infuse Pro monetize product capabilities; Jellyfin/other community projects use voluntary support; Immich offers optional financial support without feature restrictions. Portico deliberately reserves a future separate paid relay while keeping ordinary server capabilities free. Those models imply different support budgets and incentives, not automatic engineering superiority. [Plex Pass](https://support.plex.tv/articles/201751006-plex-pass-feature-overview/), [Emby Premiere](https://support.emby.media/support/articles/Premiere-Feature-Matrix.html), [Infuse](https://firecore.com/infuse), [Immich funding statement](https://github.com/immich-app/immich/discussions/11186).



### Telemetry comparison at the actual collection boundary

| Project | Source-backed stance | Assessment for Portico |
|---|---|---|
| Portico | Host/GPU and operation metrics stay on owner server; Hosted privacy source describes account/network/security operational processing and excludes central media/playback collection. | A useful privacy distinction; do not turn it into the inaccurate claim that optional account/certificate services process no personal/network data. |
| Jellyfin | Official site says no tracking/phone-home/central collection. Public history includes an Expo build containing unwanted analytics-capable code despite no tracking calls, illustrating why final native dependencies matter. [Jellyfin](https://jellyfin.org/), [dated Expo explanation](https://jellyfin.org/posts/a-note-about-privacy-and-expo/) | Similar privacy aspiration; Portico's optional central account service is an additional boundary. The 2021 Expo note is dated engineering evidence, not a current finding against Jellyfin. |
| Plezy | Optional crash reports and explicit support uploads are separate from direct streaming; official builds enable Sentry with redaction and default-on crash preference. See source above and [privacy](https://plezy.app/privacy). | More automatic diagnostic visibility, more external disclosure; do not equate optional with opt-in by default. |
| Navidrome | Default-on installation insights include pseudonymous installation ID, version/environment and aggregate activity/configuration; can opt out or disable external services (conf/configuration.go:806,1046; core/metrics/insights/data.go:4). | Different from local metrics; Portico may keep owner control without adopting installation reporting. |
| Audiobookshelf | Published policy says personal data stays locally on the user's server; inspected app/server manifests contain no Sentry/analytics package hit. [Privacy](https://audiobookshelf.org/privacy-policy/) | Similar no-central-collection position, but source keyword absence is not a complete network/privacy audit. |
| Immich | Prometheus monitoring is explicitly enabled and self-hosted metrics can be collected locally. [Monitoring](https://docs.immich.app/features/monitoring/) | More ecosystem-integrated owner observability; the word telemetry does not inherently mean reporting to the vendor. Full external-call audit was not done. |
| Infuse / Firecore | Current policy describes usage/diagnostic identifiers, website Google Analytics and optional Firebase crash data; Infuse has a Share Crash Data switch. [Privacy](https://firecore.com/privacy) | A user-visible diagnostic choice is useful; Portico's automatic external reporting surface is smaller in inspected manifests. |
| Plex / Emby | Product account/payment/remote services have additional privacy boundaries; a complete current collection/setting comparison was not completed in this domain review. | No unsupported claim that private media titles are collected or that telemetry is absent. Funding comparisons above do not establish privacy. |

No platform privacy label, no dependency grep and no public policy alone proves that final binaries contain no analytics/diagnostic-capable third-party code. A release inventory should validate SDK behavior and permissions as well as direct application calls.

### Observed release cadence, not a release-quality ranking

A read-only GitHub Releases API sample on 11 October returned Plezy 2.19.1 (7 September), 2.20.0 (15 September), 2.21.0 (22 September), 2.22.0 (29 September) and 2.23.0 (11 October); roughly weekly-to-fortnightly in this short window. Navidrome's sample had 0.63.1/0.63.2 in July and 0.64.0/0.64.1/0.64.2 on 12/21/24 September. Audiobookshelf had 2.35.1 in May, 2.36.0 in July, 2.36.1 on 16 September and 2.37.0/2.37.1 on 28/29 September. These small samples show batching followed by hotfixes; they do not establish long-term cadence, regressions, maintainer count or an SLA. [Plezy 2.23.0](https://github.com/edde746/plezy/releases/tag/2.23.0), [Navidrome 0.64.2](https://github.com/navidrome/navidrome/releases/tag/v0.64.2), [Audiobookshelf 2.37.1](https://github.com/advplyr/audiobookshelf/releases/tag/v2.37.1).

Portico has no established stable public cadence visible in these checkouts; its newest-only security policy and preview release process fit pre-release development. It should choose a readiness/patch/support policy rather than imitate another project's interval.

### License inventory caveat for comparison

Jellyfin's root LICENSE/README identify GPLv2, while inspected Model/Controller package metadata says GPL-3.0-only. This review records that mixed metadata rather than resolving the entire project's licensing from one badge. Jellyfin Android TV has GPLv2 text; Swiftfin has MPL-2.0; Plezy has GPLv3 text and README GPL-3.0, not a verified Apple exception. Licenses must be checked at the actual component/distribution boundary.

## Architectural differences

### A-25.1 Split product repositories, colocated server and web

- **Portico:** Server/web/core share one npm workspace and source tree; native, Roku, Hosted/internal and website remain separate (apps/portico-server/package.json:5; Project Architecture/01 Overview.md:9).
- **Jellyfin / Plezy / other:** Jellyfin separates server/web/platform clients and SDKs. Plezy keeps its Flutter/native adapters, relay and website together.
- **Why it matters:** Portico can review server/web behavior atomically, but cannot make a server/native/Hosted change one commit. A product release needs a tuple of repository identities, not just a server tag. Private Hosted visibility is a real boundary a universal public monorepo would have to preserve.
- **Assessment:** **Trade-off (subjective)** — Deliberate split. Keep public/private separation; a public client monorepo would be a large migration touching release, imports, CI and history. The present layout is reasonable if cross-repo versions are pinned and checked.

### A-25.2 Copied shared packages instead of published versions

- **Portico:** Native hashes selected files against a sibling canonical checkout; source ships independently and matched at inspection. Tests and contracts are not in the copy selection (apps/portico-react-native/scripts/shared-packages.mjs:16).
- **Jellyfin / Plezy / other:** Jellyfin clients consume released SDK/protocol dependencies; Plezy's shared Dart code does not require a cross-repository copy.
- **Why it matters:** Copying avoids a package registry and release choreography, but replaces dependency resolution with human synchronization. A branch can match today's main and drift tomorrow. Alternatives: publish immutable npm packages with compatible versions; vendor by git subtree with upstream SHA; use a submodule; or colocate clients in one public monorepo. None is free: publishing adds release work, submodules add checkout friction and a monorepo increases build scope.
- **Assessment:** **Trade-off (subjective)** — Deliberate ownership rule, good checksum checker. Medium change to record a canonical commit/content manifest and test the selected version; medium/large to move to published packages or a monorepo.

### A-25.3 Shared Go toolkit is vendored by copying

- **Portico:** Hosted copies 19 apikit files and a diff script checks them; current generate.go is not identical (portico-internal/scripts/shared-apikit.sh:9; apps/portico-server/apikit/generate.go:65).
- **Jellyfin / Plezy / other:** Jellyfin project references/NuGet and Navidrome Go modules identify shared dependencies declaratively.
- **Why it matters:** The copy allows a private service to compile without a public sibling but its schema generation can differ silently. A Go module tag would give update intent and checksum provenance, although public/private publication policy still needs care.
- **Assessment:** **Portico behind** — The deliberate independent-build goal is valid; enforcement is incomplete. Small: gate current diff; medium: pin apikit as a versioned module or embed an upstream digest. B-25.5 records actual drift.

### A-25.4 Broad package DAG instead of formal layers

- **Portico:** cmd/server imports 50 internal packages and httpapi 45. Domain services import other domain packages and dbwork directly. catalog imports operations; metadata imports catalog (server/internal/catalog/catalog.go:9; httpapi/router.go:10).
- **Jellyfin / Plezy / other:** Jellyfin uses Model/Controller interfaces/Implementations/Api/Server project references; Navidrome has core/server/persistence/model and Wire sets.
- **Why it matters:** Portico's direction is acyclic in the text scan but not strictly persistence → domain → transport. Internal packages prevent external consumers from depending on them; they do not stop HTTP from knowing every service or a catalog change from affecting operations.
- **Assessment:** **Trade-off (subjective)** — No architectural violation follows from package count. Medium: define a dependency map and enforce forbidden imports; large: extract cohesive read/state/application seams. Prefer contracts at actual change boundaries over interfaces for every type.

### A-25.5 HTTP composition and catalog are god-package candidates

- **Portico:** httpapi has 23,193 production lines/120 files and catalog 30,857/121, together 23.3% of production Go. router.go is 1,407 lines; social_routes.go 996; home_rows.go 1,224.
- **Jellyfin / Plezy / other:** Jellyfin spreads controllers/services across projects but retains substantial legacy managers; Navidrome separates scanner/persistence/core and smaller media scope.
- **Why it matters:** These packages combine routing, authorization, error translation, browsing composition, personalization, SQL and cross-domain orchestration. Review breadth and test fixture costs grow together. compactcatalog is smaller and low-fan-out, but its 1,004-line write.go is an important shared mutation seam.
- **Assessment:** **Portico behind** — Maintainability risk, not proof of defective behavior. Medium/large: extract cohesive areas with explicit transactions and ownership, then move tests closer to them. Merely slicing files by length would hide rather than resolve the coupling.

### A-25.6 Explicit wiring instead of framework DI

- **Portico:** main.go wires concrete services/callbacks in one visible 1,067-line composition root; httpapi.Dependencies exposes concrete service pointers (router.go:67).
- **Jellyfin / Plezy / other:** Jellyfin registers interface implementations with Microsoft DI; Navidrome's Wire provider sets generate constructors (core/wire_providers.go:18).
- **Why it matters:** Explicit wiring is easy to trace and has no runtime container magic. It can become difficult to reason about initialization order and optional nil dependencies as capabilities grow. DI can improve substitution and lifetime descriptions, but also create indirection and service-locator mistakes.
- **Assessment:** **Trade-off (subjective)** — Deliberate small dependency footprint. Medium: extract startup sections and narrow option types, validate required dependencies and document lifetimes. A DI framework is not necessary unless that makes the actual wiring easier to review.

### A-25.7 Narrow callbacks fence cross-domain authority

- **Portico:** Authorization callbacks run inside the SQL transaction; catalog.OwnerMetadataRevision avoids a package cycle (catalog/catalog.go:75,134; administration/service.go:58).
- **Jellyfin / Plezy / other:** Jellyfin exposes service interfaces and DI; Navidrome uses bound interfaces/provider sets.
- **Why it matters:** Callbacks can preserve atomic permissions/publication without a large central interface. They also turn dependency edges into runtime relationships invisible to an import scan; unclear callback order or nil behavior can weaken invariants even when the graph is acyclic.
- **Assessment:** **Portico ahead** — For transactional authorization this is a useful narrow seam. Keep it; small/medium improvement is documenting callback preconditions, rollback/after-commit behavior and cancellation, with seam tests. Reconsider if callbacks start serving as a hidden general event bus.

### A-25.8 SQL is owned by domains, not only repositories

- **Portico:** Domain packages issue SQL through dbwork; persistence mostly owns schema/integrity. Real authorization and publication often share transactions (catalog/catalog.go:130; catalog/reads.go:27).
- **Jellyfin / Plezy / other:** Jellyfin has EF Core/repository interfaces alongside legacy persistence; Navidrome has a named persistence/repository layer and SQL builder ecosystem.
- **Why it matters:** Direct SQL makes exact indexes, query work and write ordering visible. It makes schema changes cross many domain packages and asks reviewers to understand both business semantics and SQL. An ORM would not automatically preserve bounded work or transaction fencing.
- **Assessment:** **Trade-off (subjective)** — Deliberate scale-first choice. Keep SQL and guards; medium: maintain table-owner and transaction-owner maps, discourage arbitrary cross-domain writes and centralize repeated queries. Large to impose a repository layer throughout.

### A-25.9 Request context is a service view

- **Portico:** WithContext clones the catalog wrapper and shares mutable state; read resolves a request snapshot/deadline, otherwise background context (catalog/reads.go:64,85).
- **Jellyfin / Plezy / other:** Jellyfin commonly accepts CancellationToken per async operation; Navidrome passes Go context into services/repositories.
- **Why it matters:** The Portico wrapper avoids changing hundreds of signatures, but callers can forget WithContext and silently lose cancellation/snapshot accounting. The source still has 245 production context.Background occurrences; that count is an inspection signal, not 245 bugs, because startup and detached maintenance legitimately need them.
- **Assessment:** **Trade-off (subjective)** — Practical retrofit. Medium: prefer explicit context on new APIs and guard interactive entry points. A call-site audit matters more than banning background contexts globally.

### A-25.10 Error conventions combine typed sentinels and a large transport map

- **Portico:** Domains return sentinel/typed errors; failure() translates many package errors with errors.Is into public codes, retry headers and concealed 404s (httpapi/router.go:234). API schema assertions validate envelopes (spec_contract_test.go:13).
- **Jellyfin / Plezy / other:** Jellyfin relies on controller result types, exception filters and framework validation; Navidrome has middleware/REST error helpers.
- **Why it matters:** Stable error codes help thin clients and i18n. One broad mapper must be updated whenever a domain adds an error, and generic errors can become incorrect client errors or conceal operational detail.
- **Assessment:** **Trade-off (subjective)** — Useful public boundary, substantial maintenance load. Medium: per-area mappings and table-driven complete error tests; keep private causes separate from public strings. Do not replace the map with raw err.Error responses.

### A-25.11 Plain logs plus local operational records

- **Portico:** Production text scan found 116 log.Printf occurrences and no slog usage. HTTP failures are bounded/rate-collapsed; servicelog/operations records supply owner-facing diagnostics (router.go:163,229; supervise/supervise.go:48).
- **Jellyfin / Plezy / other:** Jellyfin uses ILogger/Serilog with structured sinks and analyzers; Navidrome uses contextual logrus and Prometheus.
- **Why it matters:** Portico is dependency-light and bounds repeated failures, but text messages are harder to correlate by operation/build/authority. Local structured records partially compensate; not every log line follows the same privacy/key conventions.
- **Assessment:** **Trade-off (subjective)** — Medium: establish a small structured local logging wrapper with request/operation/build IDs and redaction, keeping sensitive fields out. No hosted collector is required. Reconsider if current support exports already provide sufficient correlation in real failures.

### A-25.12 Configuration uses typed owner documents and environment plumbing

- **Portico:** Operations publishes typed fields/defaults/effective values and update settings; main supplies environment paths/tool options (operations/settings.go:46,93,150; cmd/server/main.go:523).
- **Jellyfin / Plezy / other:** Jellyfin has framework configuration/options plus persisted server settings; Navidrome uses Viper/Cobra with many env/TOML options (go.mod:55).
- **Why it matters:** Portico's settings-document approach supports UI explanations and defaults without a migration for every setting. Environment and constructor plumbing can still duplicate defaults or blur restart/hot effects.
- **Assessment:** **Trade-off (subjective)** — Deliberate owner-choice model. Medium: generate a complete effective configuration reference and verify env ↔ persisted precedence/restart semantics. Adopting Viper alone would not solve duplication.

### A-25.13 Pure-Go SQLite enables cross-compilation

- **Portico:** modernc-generated SQLite plus maintained Go driver; CGO_ENABLED=0 for server packages (server/go.mod:10; Makefile:22; release.yml:75).
- **Jellyfin / Plezy / other:** Navidrome uses mattn/go-sqlite3/cgo; its current TagLib adapter is Go/WASM, not the former C++/cgo wrapper (adapters/gotaglib/gotaglib.go:1). Docker/native cross-toolchains still build SQLite-linked target binaries (Dockerfile:118). Jellyfin bundles a .NET/native SQLite stack.
- **Why it matters:** Portico can cross-build server targets on Ubuntu and avoids native linker/glibc requirements for the Go binary. Translation brings its own memory, compiler and unsafe-pointer behavior. FFmpeg and decoder sandbox tools remain platform-specific.
- **Assessment:** **Portico ahead** — For portable deployment/build simplicity, ahead; not a proven speed advantage. Preserve unless profiling establishes a compelling alternative. Switching SQLite drivers is large and touches cancellation, custom functions, backup, diagnostics and tests.

### A-25.14 A local SQLite fork is now a critical maintained dependency

- **Portico:** Only handwritten modernc driver files are copied; generated SQLite remains upstream. PORTICO.md records exact version/commit/checksum, patches, duplicate-registration constraint and upgrade tests (internal/thirdparty/sqlite/PORTICO.md:3,18,22).
- **Jellyfin / Plezy / other:** Navidrome forks TagLib explicitly in go.mod; Jellyfin consumes centralized maintained packages. Plezy also keeps local native/plugin forks and upstream-contract tests.
- **Why it matters:** Portico owns a database cancellation/memory fix that ordinary dependency bots cannot safely rebase. The split between local driver and upstream library must remain compatible, and security updates cannot be a blind version bump.
- **Assessment:** **Trade-off (subjective)** — The narrow provenance record is strong. Medium ongoing work: automated upstream diff, preserved upstream tests and patch qualification. Keep only while upstream lacks the needed fix; B-25.2 records inconsistent vet policy.

### A-25.15 Minimal Go module inventory, substantial owned behavior

- **Portico:** Five external direct modules plus local apikit; nine indirect modules. Routing, JWT/session policy, scheduling, retry, supervision, schema validation and much media orchestration are in-tree.
- **Jellyfin / Plezy / other:** Navidrome declares 63 direct/72 indirect module entries in this snapshot; Jellyfin centrally lists 82 package versions, including test/analyzer packages. Counts span different product scopes and are not an apples-to-apples risk score.
- **Why it matters:** Fewer dependencies reduce licensing/security/update surface and opaque defaults. They also transfer mature-library edge cases, documentation and maintenance to one owner.
- **Assessment:** **Trade-off (subjective)** — Deliberate minimalism looks useful for core invariants. Prefer small proven ecosystem pieces for commodity parsers/tools when they lower total owned complexity. Medium policy work: require rationale and maintenance ownership for both new dependencies and new replacements.

### A-25.16 Web/native dependency policy is less uniform

- **Portico:** Web and native mix exact versions, caret ranges and local forks; lockfiles fix actual installs. Native CI uses legacy-peer-deps; React Native is an npm alias to react-native-tvos (web/package.json:17; native/package.json:18; native CI:29).
- **Jellyfin / Plezy / other:** Jellyfin centrally manages NuGet and web uses SDK conventions; Plezy pins Flutter in workflows, lockfiles and a mixture of local/git plugin overrides.
- **Why it matters:** Locking gives repeatable installs but does not explain why peer checks are bypassed or how framework/native ABI upgrades are qualified. A dependency-minimal server still sits beside a large JavaScript/native transitive graph.
- **Assessment:** **Trade-off (subjective)** — Medium: document dependency-upgrade policy per stack and test framework forks against all target adapters. Remove peer bypass only when compatibility permits; do not claim dependency minimalism for the whole product.

### A-25.17 Manually approved FFmpeg is a separate component release

- **Portico:** sources.lock pins FFmpeg 8.1.2, recipe commits/patches, feature/license rules and component releaseTag. Workflow manually builds/qualifies six targets (third_party/ffmpeg/sources.lock.json:3; ffmpeg-toolchain.yml:31).
- **Jellyfin / Plezy / other:** Jellyfin has a separate jellyfin-ffmpeg ecosystem; Navidrome often relies on external FFmpeg and native cross-build packaging. Plezy similarly owns media native-build inputs.
- **Why it matters:** Separating codec/toolchain changes from application changes reduces unexplained media regressions and gives corresponding-source identity. It increases release dependency management and does not test actual GPU/HDMI/device paths just because software verification passed.
- **Assessment:** **Portico ahead** — Keep deliberate qualification. Medium: publish approval evidence and component compatibility/security replacement policy. A feature manifest should record hardware acceptance separately from compile/codec presence.

### A-25.18 hls.js patch provenance is explicit

- **Portico:** Local 1.7.2-portico.1 package records upstream commit, source/patch/artifact/distribution hashes and build Node/npm, with deterministic rebuild claim and preserved license (third_party/hls.js/README.md:3; provenance.json:1).
- **Jellyfin / Plezy / other:** Jellyfin-web normally manages player dependencies through npm; Plezy owns native player/plugin forks rather than this same browser patch.
- **Why it matters:** A narrowly explained patch is reviewable and better than an untracked minified edit. The tgz is still a binary-like vendored artifact and source-level drift can escape a simple package-version update.
- **Assessment:** **Portico ahead** — Keep provenance and reproducible rebuild procedure. Small/medium: CI verify artifact/hash and rerun boundary regression on upgrades. Reconsider maintaining the fork when upstream fixes the exact problem.

### A-25.19 Product-specific guards versus general analyzers

- **Portico:** Go vet plus panic/write/SQL guards, web lint-design/lint-strings and preference-consumer checks express actual product rules (supervise/discipline_test.go:63; web/package.json:12).
- **Jellyfin / Plezy / other:** Jellyfin enables nullable/warnings-as-errors, banned symbols and multiple analyzers; Plezy enforces formatting, asynchronous-resource rules and native lint; Navidrome runs golangci-lint.
- **Why it matters:** Portico catches regressions general linters cannot see, such as unbounded catalog scans or abandoned preference fields. It lacks equivalent broad formatting/static-analysis coverage in some lanes, and text guards can overclaim completeness.
- **Assessment:** **Portico ahead** — Ahead on domain invariants; not uniformly ahead on generic hygiene. Medium: keep domain guards, add cheap targeted general checks and make AST guards complete. B-25.13/14 limit what current guards prove.

### A-25.20 Panic containment is a process policy

- **Portico:** supervise.Go/Loop contains and records panics; source guard rejects most bare go statements with one exact upstream driver exception (supervise/discipline_test.go:27,63).
- **Jellyfin / Plezy / other:** Jellyfin uses host/task/exception conventions; Navidrome's standard goroutines and test leak tooling do not impose this same in-tree supervision rule.
- **Why it matters:** One background fault need not kill unrelated playback. Containment can leave a job half-published or a component dead unless durable state/restart semantics are correct; Loop explicitly assumes restartable state.
- **Assessment:** **Trade-off (subjective)** — Deliberate availability policy. Keep containment and visible panic counts, but qualify publication/restart safety separately. Small: strengthen AST check. Medium: documented restart ownership and unhealthy-component reporting.

### A-25.21 Real database and contract tests are first-class

- **Portico:** API tests open real SQLite, wire handlers and validate real responses against schemas; guards prepare literal SQL against current schema (httpapi/hostile_input_test.go:120; spec_contract_test.go:13; persistence/statements_prepare_test.go:98).
- **Jellyfin / Plezy / other:** Jellyfin WebApplicationFactory boots the real app with selected mocks; Navidrome Ginkgo repositories use database tests. All mix isolation and integration.
- **Why it matters:** Portico can catch schema/handler mismatches that mocked repositories miss. Integration fixtures are expensive and may hide a small unit regression behind slow setup. Schema-valid answers can still have wrong authority/ordering, as the contracts explicitly acknowledge.
- **Assessment:** **Portico ahead** — Strong foundation worth preserving. Medium: maintain a semantic API conformance corpus and separate cheap domain tests from broad composition tests. Do not use the test-line ratio as release qualification.

### A-25.22 Scale is encoded in test design

- **Portico:** catalogtest synthesizes deterministic large catalogues; SQL-plan/statement guards and performance-tier proofs police bounded work (persistence/statements_scan_test.go:148; internal/catalogtest/catalogtest.go; catalog/rec_scale_test.go).
- **Jellyfin / Plezy / other:** Jellyfin has naming/property/integration tests and Navidrome scanner benchmarks, but neither inspected CI asserts Portico's 5–10-million-item O(visible) target as a product contract.
- **Why it matters:** Portico treats asymptotic behavior as architecture. Query-plan tests depend on statistics and allowed scans; wall-clock proofs depend on fixture realism, hardware, concurrent writers and cold/warm state.
- **Assessment:** **Portico ahead** — Deliberate and valuable. Medium: publish reproducible workload/hardware/result evidence and make release-tier invocation explicit. Goals and synthetic proofs remain distinct from household and concurrency acceptance.

### A-25.23 Media tests are tiered away from default CI

- **Portico:** 28 testtier.Media call sites were counted; unset/default skips them, and eight test files refer to performance selector. CI and app release do not set those selectors (testtier/testtier.go:11; ci.yml:49; build-server-release.py:148).
- **Jellyfin / Plezy / other:** Plezy's CI includes native platform suites; Jellyfin's server tests run three OSes; Navidrome races/shuffles and smoke-tests cross-binaries.
- **Why it matters:** Default feedback is cheaper, but codec/live/startup/shutdown/scale regressions can pass ordinary green checks. A manual integrator tier can work only if qualification results are bound to the exact release source and cannot be forgotten.
- **Assessment:** **Portico behind** — Deliberate speed/cost trade-off, incomplete gate. Medium: scheduled and release-required media/performance lanes with explicit helpers; keep the everyday fast tier.

### A-25.24 Race detector is release-only in the principal public server workflow

- **Portico:** Regular CI executes go test without race/shuffle/coverage; snapshot release builder runs -race -timeout=30m (ci.yml:49; build-server-release.py:148).
- **Jellyfin / Plezy / other:** Navidrome runs -race -shuffle=on and coverage in its ordinary Go pipeline; Jellyfin collects coverage across OSes.
- **Why it matters:** Concurrency mistakes surface late, and testing the same package suite five times for release targets increases compute cost without testing target runtime behavior. Coverage reporting would expose unexercised areas but never proves behavior.
- **Assessment:** **Portico behind** — Medium: separate one race job on host from cross-build jobs, run it on meaningful change/merge gates and store results. Keep targeted race exemptions narrow and explained.

### A-25.25 Fuzz seeds exist, continuous fuzzing does not

- **Portico:** Five Go fuzz functions cover browse expressions, body decoding, fairness keys, client address and proxies (httpapi/fuzz_test.go:34,63,86,118,152). No reviewed Portico workflow runs -fuzz.
- **Jellyfin / Plezy / other:** Jellyfin has SharpFuzz projects and testcases/fuzz scripts; their presence also does not prove continuous campaigns.
- **Why it matters:** Seeds under go test are valuable regression cases but do not explore mutations over time. Parsers for media/metadata/playlists/subtitles and signed data merit different corpora and timeout/allocation assertions.
- **Assessment:** **Unclear** — Portico has the capability; neither tree establishes operational fuzzing history. Medium: schedule bounded campaigns and retain minimized crash corpora, without putting unpredictable runs on every developer edit.

### A-25.26 Client tests are mostly headless behavior harnesses

- **Portico:** Node test loaders and custom component/hook harnesses exercise commands/state with mocked UI elements (web/test/detail-actions.test.ts:7,15; native/test/register.mjs:1).
- **Jellyfin / Plezy / other:** Plezy has Flutter widget tests, C++/JVM/Apple native tests and Maestro device journeys. Jellyfin-web has Vitest and a legacy stack it explicitly calls difficult to test.
- **Why it matters:** Portico tests valuable lost/duplicate response and navigation logic cheaply. A fake Button or hook scheduler cannot catch focus geometry, keyboard avoidance, platform lifecycle, screen readers or actual React rendering bugs.
- **Assessment:** **Portico behind** — Medium/large: add real component-renderer and small E2E/device acceptance suites; preserve the cheap core tests. Visual/device proof is especially important for recent Android/Fire/Vega ports.

### A-25.27 Native CI is Android-centric, not absent

- **Portico:** Native JS/type/string tests and Android assembleDebug exist, but no iOS/tvOS xcodebuild or Vega build is in CI (native/.github/workflows/ci.yml:16,38).
- **Jellyfin / Plezy / other:** Plezy has Apple/Windows/Linux native suites; Swiftfin has Apple CI/TestFlight workflows. Platform CI costs money and needs different SDKs.
- **Why it matters:** Android compilation establishes Kotlin/Gradle source coherence, not signed release JavaScript packaging, device output or Apple reference-client validity. Broad claims that native has no CI would be wrong; broad claims of all-platform qualification would also be wrong.
- **Assessment:** **Portico behind** — Medium: Apple build/native tests and Vega type/build lane when tooling permits; medium/large for physical media/focus acceptance. Mark manual gates explicitly rather than hiding SDK constraints.

### A-25.28 Roku relies on generated parity and source-driven checks

- **Portico:** Roku CI reads server/native siblings, regenerates, checks diff, compiles/tests/packages and uploads a ZIP (Roku CI:22,31,36). Golden fixtures and a BrightScript VM help test translated behavior.
- **Jellyfin / Plezy / other:** Jellyfin TV clients have independent Kotlin/Swift/BrightScript implementations; Plezy shares Flutter presentation where platform support exists.
- **Why it matters:** This establishes deterministic translation/source parity, not physical SceneGraph focus, codec support, timing or store certification. Reading current sibling branches makes reproducibility and historical rebuilds dependent on foreign revisions.
- **Assessment:** **Trade-off (subjective)** — Useful deliberate shortcut, real device evidence still needed. Medium: pin source tuple/generator versions and add hardware smoke matrix. No broader Roku feature claims are made here.

### A-25.29 CI does not imply required checks

- **Portico:** Workflows and CODEOWNERS (* @justinehler) exist, but branch protection/rulesets are not present in the audited source. Release needs CI, not the separate Security workflow (release.yml:57; .github/CODEOWNERS:1).
- **Jellyfin / Plezy / other:** Jellyfin/Plezy publish contributor rules and review processes; remote required-check enforcement also cannot be proved from their YAML alone.
- **Why it matters:** A green job can be optional, stale or bypassed; a reviewed tag comment is not an enforced reviewer gate. A solo owner can choose lightweight review without pretending there is independent human approval.
- **Assessment:** **Unclear** — Small/medium: export/document required checks and tag/environment protections, distinguish human review from independent agent review, and bind release authorization to exact SHAs. No remote settings were queried.

### A-25.30 Security automation is present but incomplete as a release gate

- **Portico:** Public server/native security jobs run CodeQL and checksum-verified gitleaks; Dependabot covers Go/npm/Docker/Actions and native Gradle. No govulncheck, PR dependency-review or CocoaPods update lane was found (security.yml:18,31; dependabot.yml:2).
- **Jellyfin / Plezy / other:** Jellyfin CodeQL and jellyfin-web dependency review add different defenses; Plezy has dependency checks/native sanitizers.
- **Why it matters:** Secret/source scanning matters, but native JS CodeQL does not analyze Objective-C/Swift/Kotlin media bridges. Module vulnerability checks and supply-chain policy can fail early without executing app code.
- **Assessment:** **Portico behind** — Medium: add reachable Go vulnerability checks, dependency review and native static/sanitizer checks where useful, and make successful security evidence part of release. Do not infer GitHub private-report/secret-scanning settings from docs.


### A-25.31 Source sealing versus reproducible binary builds

- **Portico:** Snapshot builder captures all module/apikit inputs, checks source before/after copying/building, embeds digest/identity and signs artifact manifest; build ID uses current time and UUID (build-server-release.py:115,120,155).
- **Jellyfin / Plezy / other:** Navidrome uses containerized cross-builds and GoReleaser; Jellyfin publishes platform builds/provenance through its packaging ecosystem; Plezy pins native toolchain inputs.
- **Why it matters:** Sealing proves which source a controlled build recorded, not that an independent party can reproduce byte-identical binaries. Portico app binaries include variable identity/time and release build omits -trimpath, although Docker and Makefile use it. hls.js has a more specific reproducible-rebuild record.
- **Assessment:** **Trade-off (subjective)** — Strong traceability; incomplete reproducibility. Medium: separate deterministic content identity from invocation ID, pin base/toolchain digests, use path trimming and publish rebuild instructions/results.

### A-25.32 Signed server binary manifest is narrower than a signed product

- **Portico:** Manifest signs server artifact/source/root/toolchain-manifest digest. Web build is copied later; package-release rewrites web/portico-build.json after sealing (build-server-release.py:158; package-release.sh:21,26).
- **Jellyfin / Plezy / other:** Plezy signs updater payloads, optionally codesigns/notarizes app bundles; OS package signatures and full artifact attestations cover different trust boundaries.
- **Why it matters:** A verified server manifest is not an authenticated installed web UI, installer, service configuration or all third-party bytes. Release checksums and GitHub attestations add evidence, but updater/installer verification against a pinned key remains unfinished.
- **Assessment:** **Portico behind** — Medium: sign final per-platform artifacts/full inventory with web and packaging identity, verify at install/update. Preserve binary source sealing as one component rather than advertising it as whole-release trust.

### A-25.33 Release signer custody is documented but not enforced locally

- **Portico:** Docs forbid production key generation on development machines; signing_key generates one if missing and verifies against its own just-created public key (release-signing.md:3,5,9; build-server-release.py:35). CI checks an injected key exists.
- **Jellyfin / Plezy / other:** Plezy signing secrets and trusted-ref checks distinguish public official builds from arbitrary refs; more mature releases separate build/sign/approval concerns.
- **Why it matters:** The script can produce an apparently valid sealed artifact under a brand-new key. That is useful for development but unsafe as an official trust claim unless mode/key identity and authorization are enforced.
- **Assessment:** **Portico behind** — Explicitly known unfinished work. Medium: separate dev seal from official release, fail closed on absent approved signer, pin expected public identity and protect signing environments. This is B-25.6, not a claim of leaked keys.

### A-25.34 Container engineering has good defaults, separate release weakness

- **Portico:** Docker uses USER portico, a single state volume, tini, explicit qualified media binaries, CA certificates, bubblewrap and a server-native health probe (Dockerfile:63,80,87,94,100).
- **Jellyfin / Plezy / other:** Navidrome embeds UI in its binary and builds multi-architecture images through native cross-build machinery; Jellyfin supplies platform/container distributions.
- **Why it matters:** Common root/PID1/no-healthcheck accusations do not apply. Portico's Debian image also includes Python for bundle verification and mutable base tags; size cannot be determined without a build. Media permissions/hardware groups and host bind mounts still need install guidance.
- **Assessment:** **Portico ahead** — Good defensive default container design. Small/medium: fix bundle input per target, document UID/GID/mount behavior and publish image size/digest. Do not weaken init/nonroot to simplify packaging.

### A-25.35 OS installers implement different service models

- **Portico:** Linux runs a dedicated user under systemd sandboxing; macOS installs a per-user LaunchAgent; Windows schedules an ONSTART task as SYSTEM/HIGHEST (packaging/linux/portico-media-server.service:10; macos/launcher.sh:8; windows/installer.nsi:45).
- **Jellyfin / Plezy / other:** Jellyfin/Navidrome have established native service/package installers; Plezy desktop client lifecycle is user-application oriented rather than a media-server daemon.
- **Why it matters:** Linux restrictions protect application paths but can conflict with owner-selected external recording/cache roots. macOS dies with user-session lifecycle. Windows gets more authority and weaker service-style restart/readiness semantics. These are product behaviors, not just file extensions.
- **Assessment:** **Trade-off (subjective)** — Medium: document lifecycle/permissions per OS and qualify upgrade/stop/restart. Consider a least-privilege Windows service and explicit optional Linux writable-path overrides. Do not infer safety from a successful installer compile.

### A-25.36 Signing and notarization are preview-only promises

- **Portico:** README/INSTALLING openly say unsigned previews. macOS script optionally signs with timestamp=none; no notarytool/stapling/provisioning lane, Windows Authenticode or native store-release lane is in the reviewed Portico workflows (README:49; package-macos.sh:29).
- **Jellyfin / Plezy / other:** Plezy macOS workflow signs with timestamp/runtime, submits to notarytool and staples; updater signing is separate from Windows Store resigning.
- **Why it matters:** OS acceptance and artifact-authenticity signatures serve different purposes. Optional codesign without notarization is not Gatekeeper qualification; APK debug build is not production Android signing or Apple store submission.
- **Assessment:** **Portico behind** — Honest prerelease staging, appropriate automatic-update hold. Medium/large platform release work before marketing unattended/store distribution. No evidence signing accounts/secrets have been provisioned was collected.

### A-25.37 Native target runtime testing differs from cross-compilation

- **Portico:** Application Go binaries for five targets are built on Linux; regular app CI is Ubuntu. FFmpeg qualification runs target runners, but app package/runtime/installer smoke is not in release.yml (release.yml:74,82; ffmpeg-toolchain.yml:38).
- **Jellyfin / Plezy / other:** Jellyfin tests on Linux/macOS/Windows; Navidrome runs Windows tests and cross-binary QEMU smoke; Plezy native CI spans desktop and Apple.
- **Why it matters:** CGO off removes linker complexity but not path/case/signal/permissions/port differences. Target-specific installer/daemon behavior is untested by a Linux test helper.
- **Assessment:** **Portico behind** — Medium: startup/readiness/shutdown/package smoke on every shipped target; medium/large hardware qualification separately. Avoid rerunning only host tests and calling that platform validation.

### A-25.38 Update notices are owner-configured and background-fetched

- **Portico:** Empty feed means unconfigured; first request schedules bounded background fetch, successful data refreshes on later reads after six hours and failures after five minutes (administration/update_feed.go:61; maintenance.go:503).
- **Jellyfin / Plezy / other:** Plezy enables official update checks and desktop updater integration; Jellyfin normally updates via packaging/container channels; Navidrome exposes release/update information through its ecosystem.
- **Why it matters:** No periodic Hosted chatter and no interactive network wait fit Portico's principles. An unconfigured server may miss security updates; the feed cache is time-based external data, not derived catalogue data, so it is not the same freshness promise.
- **Assessment:** **Trade-off (subjective)** — Deliberate low-ceremony privacy behavior. Small/medium: provide an explicit official opt-in default and visible unconfigured warning. Fix schema mismatch before adding an installer.

### A-25.39 Automatic application and rollback are not implemented

- **Portico:** UpdateReport.AutoInstall stays false; official flat manifest also prohibits automatic installation (maintenance.go:484; write-public-update-manifest.mjs:26).
- **Jellyfin / Plezy / other:** Plezy macOS/Windows uses Sparkle/WinSparkle-style signed feeds; package managers/containers supply update delivery for peers.
- **Why it matters:** Manual updates preserve owner control and avoid unsigned automation. They impose backup/compatibility checking on users. Server update notification, package signature verification, binary swap and database rollback are different capabilities.
- **Assessment:** **Trade-off (subjective)** — Deliberate hold and a sensible pre-release boundary. Large to add safe auto-update: pinned trust, schema compatibility, health/backup/restore, rollback authorization and package-manager ownership. A good signed notice/manual workflow is an earlier medium win.

### A-25.40 Stable/beta labels exist before channel semantics are settled

- **Portico:** Settings permit stable/beta; public generator always says stable and release workflow marks previews prerelease:false/make_latest:true (operations/settings.go:47; release.yml:225; update writer:23).
- **Jellyfin / Plezy / other:** Jellyfin distinguishes stable/release branches and unstable/nightly/testing workflows; Plezy has public release/feed channels.
- **Why it matters:** Portico can deliberately make GitHub latest useful for preview downloads, but automated consumers and humans may interpret stable as production qualification. A label is a compatibility/support promise unless clearly defined.
- **Assessment:** **Portico behind** — Small/medium: define preview/beta/stable rules and separate development availability from stable acceptance. Preserve useful permanent download links without making warning text the only channel contract.

### A-25.41 SemVer validation does not define compatibility support

- **Portico:** Builder validates complete MAJOR.MINOR.PATCH; embedded version defaults 0.1.0-dev. API docs explicitly allow pre-release endpoint/client/database changes without compatibility (build-server-release.py:98; buildinfo/info.go:4; api/README.md:47).
- **Jellyfin / Plezy / other:** Jellyfin documents HTTP/plugin compatibility and backports; Immich publishes intended major-only breaking API/deployment policy.
- **Why it matters:** Valid version syntax alone cannot tell whether old clients survive a new server or a database can be downgraded. One version can describe app, API, protocol, schema and media component only if each relationship is documented.
- **Assessment:** **Portico behind** — Deliberate pre-release freedom, but medium release policy needed: supported client/server window, deprecation period, minimum compatible capabilities, security patch support and DB upgrade/downgrade rules.

### A-25.42 Compatibility checks validate current spec, not prior releases

- **Portico:** CI regenerates contracts and rejects dirty outputs; openapi coverage/schema tests check current routes/answers (ci.yml:51; httpapi/spec_contract_test.go:13).
- **Jellyfin / Plezy / other:** Jellyfin diffs base/head OpenAPI and reports plugin ABI changes; Navidrome's api-diff fails on selected breaking changes (Makefile:123).
- **Why it matters:** A renamed/removed endpoint and regenerated types can pass Portico's freshness gate while breaking installed clients. This is currently allowed, but becomes important immediately after first release.
- **Assessment:** **Portico behind** — Medium: diff public schema against supported baselines, require deliberate version/deprecation decisions, then test old client semantics. Portico has no plugin ABI promise to copy unless it introduces in-process plugins.

### A-25.43 Additive migrations favor large-library availability

- **Portico:** Persistence policy forbids large startup rewrites and permanent enum reuse; derived/backfill changes run in background. Actual embedded chain contains baseline plus 0002–0011 (persistence/README.md:29; migrations.go:20).
- **Jellyfin / Plezy / other:** Navidrome uses goose with migration validation; Jellyfin uses EF/legacy migration mechanisms; Audiobookshelf tracks database/max server versions.
- **Why it matters:** Portico avoids blocking huge catalogues at upgrade. Purely additive evolution can accumulate obsolete schema and does not mean a new index is constant-time on a populated table. Long-term cleanup requires a separate maintenance plan and measured backfill publication behavior.
- **Assessment:** **Trade-off (subjective)** — Deliberate useful policy. Medium: explicitly distinguish bounded metadata migrations from data-dependent index creation, maintain derivation versions and publish compatibility/cleanup plan. A migration-rule doc is not a performance measurement.

### A-25.44 Baseline and squash reset provenance

- **Portico:** Server/native/Roku/internal current histories begin with 8 October baseline and contain 9/4/3/2 commits respectively. Persistence baseline replaced 265 pre-release migrations on 25 September (persistence/README.md:8; read-only git log).
- **Jellyfin / Plezy / other:** Jellyfin retains multi-contributor development/review history; comparator shallow clones cannot measure total history. Plezy requests focused reviewed changes and disclosure of AI use.
- **Why it matters:** A fresh public baseline is a legitimate publication choice, but current blame cannot explain most code's origin, why a regression was fixed or which old device evidence applies. SQL rebase and Git squash are separate events and should not be conflated.
- **Assessment:** **Trade-off (subjective)** — Deliberate pre-release cleanup. Small/medium: retain searchable private history/bundles and provenance links, keep future meaningful small commits. Do not equate a short public history with a young implementation or lack of past review.

### A-25.45 Release notes are generated, migration guidance is manual

- **Portico:** Public release uses generate_release_notes:true with static unsigned-preview body; docs tell owners to back up and read notes (release.yml:228; INSTALLING:3; public update docs:10).
- **Jellyfin / Plezy / other:** Jellyfin procedure explicitly requires platform notes/FFmpeg changes/repo changelogs; Immich publishes dedicated breaking-change migration instructions.
- **Why it matters:** Auto-generated commit lists can describe implementation without telling households about schema, minimum clients, changed defaults or rollback. Pre-release squashed histories make this worse.
- **Assessment:** **Portico behind** — Small/medium: required human-readable upgrade section, known issues/platform qualification and compatibility metadata before publish. No need for a large changelog bureaucracy.

### A-25.46 Public docs exist but architecture currency is weak

- **Portico:** Website has user/admin/install/security/developer Markdown; architecture overview still says Apple/web first and future Android ports, server doc says one migration, Backlog contains retired branch/deploy references.
- **Jellyfin / Plezy / other:** Jellyfin has an extensive docs repository and contribution workflow, but its source-tree page also lists old projects; Plezy's developer guide and codegen checks are more current in inspected areas.
- **Why it matters:** Documentation quantity is not currency. Portico's rich private contracts help implementers, but users/third-party clients need a reliable released reference rather than private planning and obsolete namespace descriptions.
- **Assessment:** **Portico behind** — Medium: release-bound doc checks and a single authoritative API/version/platform inventory. Prioritize misleading install/protocol/migration statements, not cosmetic rewriting.

### A-25.47 Public third-party API intent is unusually explicit

- **Portico:** scope-completion requires an API usable without first-party React state; published schemas and semantic fixtures belong with implementation, private plans stay private (Product Contracts/scope-completion.md, API section; server/api/README.md:15).
- **Jellyfin / Plezy / other:** Jellyfin has public API/SDKs and plugin ecosystem; Navidrome benefits from OpenSubsonic plus its own API; Plezy consumes several server protocols.
- **Why it matters:** Portico's authored presentation model could otherwise accidentally make client-core a mandatory proprietary-style dependency. Explicitly promising independent clients is valuable, but generated contracts not adopted by client-core and multiple protocol documents complicate delivery.
- **Assessment:** **Portico ahead** — Strong deliberate intent, not yet a verified compatibility promise. Medium: publish minimal third-party client examples and semantic vectors independent of UI packages. Treat capability negotiation as part of public contract.

### A-25.48 Internationalization plumbing precedes community workflow

- **Portico:** Typed message IDs, regional catalogs and string/glossary/raw-error linters exist; supportedLocales is only en-US/en-CA/en-GB (packages/i18n/src/catalog/index.ts:7).
- **Jellyfin / Plezy / other:** Jellyfin routes translations through Weblate/translation branches; Plezy uses generated locale catalogs/codegen and community localization.
- **Why it matters:** Portico can keep terminology consistent and prevent raw internal errors in UI. Community translation needs extraction/context/plural validation/review and an explicit non-code contribution policy. Regional English transformations are not evidence of arbitrary-language/layout readiness.
- **Assessment:** **Portico behind** — Medium: translation exchange format, review workflow and pseudolocale/RTL/long-string tests, with Justin deciding what submissions are accepted. No need to open general code PRs.

### A-25.49 GPL licensing and contribution governance are different freedoms

- **Portico:** Server/native GPL-3.0-or-later; external code PRs closed without review, feedback/issues accepted (CONTRIBUTING:5,17; README:90).
- **Jellyfin / Plezy / other:** Jellyfin opens code/docs/translations to community; Plezy accepts code with explicit AI-assisted PR disclosure (CONTRIBUTING:19).
- **Why it matters:** GPL gives recipients use/modify/redistribute rights, not a right to merge into upstream. A no-PR rule can protect product coherence and maintainer time, while making upstream bug fixes, ownership succession and translators harder to organize.
- **Assessment:** **Trade-off (subjective)** — Deliberate, valid model. Small: clarify translations/data/docs/security patches versus code; medium: create issue-based tested patch proposals if desired. Opening contributions is Justin's decision, not an audit requirement.

### A-25.50 Apple distribution needs a distinct license decision

- **Portico:** Native is GPL-3.0-or-later and no store exception/dual-license statement was found. iOS/tvOS use native Apple media APIs, but shared GPL code and third-party components still need a distribution-rights inventory.
- **Jellyfin / Plezy / other:** Swiftfin is MPL-2.0 (swiftfin/LICENSE.md:1); VLC iOS is explicitly GPLv2-or-later/MPLv2 dual-licensed. Jellyfin Android TV has GPLv2 text and Android distribution; Plezy has GPLv3 text and Apple availability.
- **Why it matters:** GPL and Apple distribution restrictions have a known compatibility issue; a store listing proves neither legal compatibility nor the rights of all contributors/dependencies. Sole ownership may permit an additional grant for original Portico code, not arbitrary relicense of third-party code.
- **Assessment:** **Unclear** — High-value pre-release legal decision. Medium: component-by-component rights review and explicit Apple-compatible grant/exception if chosen. This review is not a legal ruling. [VLC iOS license](https://github.com/videolan/vlc-ios/blob/master/COPYING); B-25.12 tracks the unresolved risk.

### A-25.51 FFmpeg distribution chooses GPLv3 and rejects nonfree

- **Portico:** Recipe requires --enable-gpl/--enable-version3, rejects nonfree/libfdk-aac and requires source/license bundle (third_party/ffmpeg/NOTICE.md:3; sources.lock.json:39).
- **Jellyfin / Plezy / other:** FFmpeg itself can have different licensing depending on enabled components; LGPL-only builds and GPL builds are not interchangeable. Native clients may use different media libraries than the server.
- **Why it matters:** Portico's separate-process GPL bundle is a deliberate redistributable choice, not an accidental LGPL claim. Corresponding source/build scripts/notices and actual configure verification must survive every package format. 'Nonfree' must not be enabled just to satisfy a codec request. [FFmpeg legal guidance](https://www.ffmpeg.org/legal.html).
- **Assessment:** **Portico ahead** — Explicit recipe/license guard is good. Medium ongoing work: audit dependency source completeness and package notices on each release; codec patent obligations are a separate jurisdictional question not settled by source licensing.

### A-25.52 Third-party notices describe inventories rather than ship all notices

- **Portico:** Server NOTICE points to go.mod/npm metadata, mentions FFmpeg/hls.js/Manrope/Lucide; package-release copies only root GPL and that summary plus component payload (THIRD-PARTY-NOTICES:7; package-release.sh:24). Native has no root THIRD-PARTY-NOTICES file.
- **Jellyfin / Plezy / other:** Mature app distributions commonly include acknowledgements/license inventory; Plezy local plugin forks preserve upstream material; VLC/Swiftfin show why component-level rights matter.
- **Why it matters:** Manifests identify dependencies, not every copyright/license obligation for compiled output. BSD/MIT/ISC/OFL texts and the maintained SQLite driver attribution need a final artifact audit; missing-summary wording alone is not proof of a legal violation.
- **Assessment:** **Portico behind** — Medium: generate and review complete runtime notices, including Go, JS, Pods, Gradle, fonts/icons/media libraries, then verify they are in packages/app acknowledgements. B-25.11 distinguishes probable recipe gap from unexamined shipped artifacts.

### A-25.53 Private Hosted services coexist with GPL public clients

- **Portico:** Public repo intentionally excludes Hosted implementation/operator plans; server can build without it (BUILDING:51). Hosted copies GPL/public toolkit code within a private service source tree.
- **Jellyfin / Plezy / other:** Jellyfin normally has no required central account service; Plezy has optional relay/hosted integration services. Immich uses AGPL for network-service source availability policy.
- **Why it matters:** GPL is not AGPL: operating a private service over a network does not by itself require public source. Distribution of covered combined binaries and license terms still matters. Public open-source positioning should describe the Hosted boundary honestly, including outage/self-hosting limits.
- **Assessment:** **Trade-off (subjective)** — Deliberate source/privacy boundary. Small/medium: publish complete protocol/offline guarantees and accurate component licensing map. Large to open/self-host Hosted; no evidence requires that choice.

### A-25.54 Trademark rules preserve official release identity

- **Portico:** TRADEMARKS separates GPL source rights from name/logos and asks modified builds to use distinct identity while retaining notices (TRADEMARKS.md:5,9).
- **Jellyfin / Plezy / other:** Jellyfin publishes branding/contribution guidance; community forks/clients use independent names. Proprietary peers also control official branding.
- **Why it matters:** Forking remains allowed; confusing users about official support/trust is discouraged. Official Hosted root/signing identity should not become a hidden requirement that falsely makes any independently branded fork unusable locally.
- **Assessment:** **Trade-off (subjective)** — Deliberate and reasonable. Small: document fork build/root/server-local behavior and descriptive attribution. Legal enforceability/registration was not evaluated.

### A-25.55 Sole-owner AI-assisted development centralizes review

- **Portico:** Justin is the sole decision maker/author identity and CODEOWNER. Contracts require independent review and behavioral evidence; current baseline hides most historic implementation review (governance.md, review paragraph; .github/CODEOWNERS:1).
- **Jellyfin / Plezy / other:** Jellyfin has a multi-person core team and policies requiring authors understand/explain AI changes; Plezy requires AI model disclosure in PR descriptions.
- **Why it matters:** AI can extend implementation/review throughput, but independent agents are not independent human accountability and can share assumptions. Bus factor means how many people can disappear before maintenance stops; Portico's decision/release/operations bus factor is currently one. This is a capacity risk, not criticism of effort or competence.
- **Assessment:** **Trade-off (subjective)** — Deliberate owner model. Medium ongoing discipline: small reviewed changes, decision rationale, critical-boundary human review and release evidence; succession/recovery contacts and maintainership policy need Justin's choice.

### A-25.56 Private contracts are knowledge capture, not completion certificates

- **Portico:** registry/coverage mark reconciled-required behavior and not-certified implementation; conformance lists 64 hostile cross-domain outcomes (Product Contracts/registry.json:1; conformance.md:1; coverage.json:1).
- **Jellyfin / Plezy / other:** Jellyfin's contributor/test/release docs and regression history capture lessons; Plezy's tests and manual/E2E guidance capture device scars.
- **Why it matters:** Portico preserves intent/authority better than undocumented agent sessions. A large prose corpus can become a second implementation that no longer matches source; its explicit no-certification rule prevents that confusion only if completion reporting obeys it.
- **Assessment:** **Portico ahead** — Keep the separation and exact-source evidence. Medium: map every required outcome to executable/manual evidence with owner/platform/build identity and expiration triggers. Do not publish private provenance by accident.

### A-25.57 Free core plus future relay has distinct sustainability economics

- **Portico:** All ordinary capabilities remain free; a future separate paid relay is the only paid feature, not current billing authorization (decisions.md:P22; scope-completion.md, relay section).
- **Jellyfin / Plezy / other:** Plex Pass/Emby Premiere/Infuse Pro charge for capabilities. Jellyfin accepts voluntary financial/community support; Plezy release enables donations. Immich offers optional financial support while keeping functionality available.
- **Why it matters:** Relay subscribers incur bandwidth/support cost; nonrelay users still create server/client/security maintenance load. Funding only transport may not pay for the large free product. A donation/support model can help without changing access rights.
- **Assessment:** **Trade-off (subjective)** — Deliberate product promise worth preserving. Medium planning: recurring costs, maintainer hours and security/support budget, with donation/sponsorship if desired. No evidence establishes commercial viability or a relay launch.

### A-25.58 Local telemetry versus automatic crash/installation reporting

- **Portico:** internal/telemetry collects owner host/GPU facts; public privacy source excludes central library/playback/search collection and describes necessary Hosted operational logs (telemetry/telemetry.go:1; getportico-tv/src/pages/privacy.astro:27,34). No Sentry/analytics dependency appeared in reviewed public manifests.
- **Jellyfin / Plezy / other:** Plezy official builds enable optional Sentry, redaction and user disabling; Navidrome insights defaults on with opt-out; Jellyfin emphasizes self-hosted community service.
- **Why it matters:** Portico has less automatic external disclosure, but less visibility into device crashes without user reports. Local telemetry is not the same as vendor analytics, and Hosted network/account operations are not literally zero data collection. Plezy also distinguishes voluntary log uploads from crash reports. [Plezy privacy](https://plezy.app/privacy).
- **Assessment:** **Portico ahead** — For privacy/control, ahead. Small/medium: clear local-versus-Hosted data inventory, bounded user-reviewed support exports and crash-log instructions; any opt-in remote reporting would need explicit product approval.

### A-25.59 Issue-led support versus a staffed community

- **Portico:** SUPPORT directs users to repository issues/private email, with no emergency/paid/response guarantee; security policy handles private reports and newest-build fixes only (SUPPORT:5,9; SECURITY:5,11).
- **Jellyfin / Plezy / other:** Jellyfin docs/forum/Matrix and multi-person contributors distribute troubleshooting; Plezy GitHub and community contributions widen feedback paths.
- **Why it matters:** Issue templates can produce useful exact-version/OS/log reports, but one maintainer triages every app/platform, security report and feature request. Public discussions can reduce duplicate support without promising a service level.
- **Assessment:** **Trade-off (subjective)** — Deliberate prerelease capacity model. Small/medium: version-specific known-issues/support matrix and community discussion moderation if demand warrants. Do not offer an unsupported SLA or imply a forum exists.

### A-25.60 Extension model trades ecosystem reach for bounded ownership

- **Portico:** Go internal packages hide implementation; first-party thin clients and public HTTP contracts are extension boundaries. No comparable in-process plugin ABI/repository lifecycle was found in the examined public tree.
- **Jellyfin / Plezy / other:** Jellyfin plugins have repository manifests/version compatibility and ABI change reports; Navidrome now has plugin/WebAssembly tooling and OpenSubsonic clients.
- **Why it matters:** Portico owns fewer unstable in-process contracts and plugin security/support dependencies. Users cannot readily add custom integrations without a fork or external API service, increasing pressure on the sole-owner backlog.
- **Assessment:** **Trade-off (subjective)** — Deliberate minimal public boundary looks sensible pre-release. Large to add plugins; medium to improve external API/webhook/adapter tooling without in-process code loading. Adopt only with a concrete extension demand.

## Missing features and capabilities to consider

### F-25.1 A release compatibility contract

Publish server/client minimums and supported windows, API capability/version negotiation rules, deprecation periods and schema downgrade/backup rules. Jellyfin's API/ABI reports and release guidance, plus Immich's major-change policy, show the user value: updating without guessing whether an installed TV app still works.

**Size:** Medium. **Principles and opinion:** No principle conflict. Highest-value architectural decision before v1; SemVer syntax and a current OpenAPI file are insufficient. Start with a conservative explicitly supported window rather than indefinite compatibility.

### F-25.2 One enforceable release qualification ledger

Bind every required check, media/performance result, platform build, manual physical-device acceptance and known exception to exact source/artifact identities. Portico contracts already demand this; Jellyfin's golden nightly/freeze and Plezy native CI show practical routes.

**Size:** Medium/large. **Principles and opinion:** Fits existing governance. My preference is a small machine-readable release checklist plus evidence links, not a new abstract test framework. Missing evidence must produce preview/unqualified status, not silent completion.

### F-25.3 Pinned cross-repository source tuples

Record server/native/Roku/Hosted apikit upstream SHAs and selected-file digest, and check those versions in CI. Published workspace packages, subtree or a public monorepo are alternatives to copying.

**Size:** Small/medium. **Principles and opinion:** Preserves standalone builds and ownership. Best near-term improvement to current mechanism; changing repository strategy is larger and should follow measured pain.

### F-25.4 Media and scale jobs required for release

Set PORTICO_TEST_TIER=all and the explicit performance tier on qualified hardware with the intended bundle and helpers, and store test/skip summaries. Plezy native suites and Navidrome race/smoke gates demonstrate broader automated qualification.

**Size:** Medium. **Principles and opinion:** No conflict; costs runner time. Schedule expensive runs and reuse evidence for unchanged components, but require release-source applicability. Fix the known httpapi timeout before making it a gate.

### F-25.5 Apple and Vega CI, release-mode Android validation

Add Apple xcodebuild/native tests, a Vega type/build lane with documented SDK conditions, and signed/release-mode Android packaging checks. Plezy's platform jobs and Swiftfin's Apple workflows are concrete examples.

**Size:** Medium/large. **Principles and opinion:** Matches the reference-client/port model. Compile, lifecycle, media output and store acceptance should remain separate result fields.

### F-25.6 Installed-package smoke matrix

Install/uninstall/upgrade, preserve state, start/readiness/shutdown and restart every supported OS package; inspect archive paths and permissions. Navidrome cross-binary smoke and Plezy Linux/Flatpak checks are useful models.

**Size:** Medium. **Principles and opinion:** Fits low ceremony. This catches release-path defects that handler tests cannot, including relative ZIP paths and service restart behavior. No extensive device lab is needed for initial installer smoke.

### F-25.7 Complete signed final-artifact inventory

Authenticate server, web, media bundle, service configuration and final installers/archives with a pinned public trust root; produce the manifest after any OS signing mutation. Add digest/key rotation and failure tests.

**Size:** Medium. **Principles and opinion:** Fits release-signing.md. It should precede automatic installation. A signing key fetched from the same feed cannot establish independent trust.

### F-25.8 Official signer mode with protected authorization

Require an approved signing identity in official mode, refuse fallback-generated keys, separate build from protected signer and document rotation/revocation/recovery. Public custody instructions already describe the goal.

**Size:** Medium. **Principles and opinion:** No conflict. Keep development sealing clearly marked and convenient; do not make it indistinguishable from official authenticity.

### F-25.9 One update-feed schema and channels contract

Choose a schema shared by the public generator and server parser, add generator-to-consumer contract fixtures, stable/beta/preview rules, latest download/digest/notes fields and explicit opt-in official feed configuration.

**Size:** Small/medium. **Principles and opinion:** Immediate user value: correct security/update notice without auto-install. Current generated feed cannot be consumed by the current server. No periodic Hosted telemetry is needed.

### F-25.10 Public upgrade and breaking-change notes

Require platform-specific changes, schema/minimum-client impact, known issues, backup expectations and compatible rollback instructions in every release. Jellyfin and Immich provide models; Audiobookshelf's release notes offer smaller-project examples.

**Size:** Small/medium. **Principles and opinion:** No conflict. Prefer concise curated notes supplemented by generated commits. A global changelog file is optional; usable per-release upgrade instructions are not.

### F-25.11 Prior-version API/semantic conformance gate

Diff OpenAPI against the oldest supported release; run old client request/response vectors and critical semantic scenarios. Jellyfin base/head diff and Navidrome oasdiff offer implementation options.

**Size:** Medium. **Principles and opinion:** Needed only once compatibility is promised; deliberate pre-release breaks remain allowed. Include auth/error/cursor/idempotency semantics that a schema diff cannot detect.

### F-25.12 Broader vulnerability and dependency qualification

Add govulncheck/reachable-vulnerability reports, PR dependency review, native static analysis and reviewed exemptions. Jellyfin-web dependency-review and Plezy native sanitizers are practical reference points.

**Size:** Small/medium. **Principles and opinion:** No policy conflict. Avoid a raw dependency-count ban; prioritize reachable issues and fixes. CI security jobs also need actual required-check/release wiring.

### F-25.13 Scheduled parser fuzzing and minimized corpus ownership

Run bounded Go/native parser fuzz jobs, retain minimal failing seeds, cover subtitles/metadata/playlist/range and signed document parsers with allocation/time assertions. Portico's five fuzz functions and Jellyfin SharpFuzz seeds are starting infrastructure.

**Size:** Medium. **Principles and opinion:** Fits hostile-input goals. An extended campaign can be nightly rather than unpredictable per-edit testing. A test owner should triage crashes and expiration of exemptions.

### F-25.14 Real rendered UI and small end-to-end journeys

Supplement custom headless component harnesses with browser/React Native rendering and a few disposable setup/sign-in/profile/browse/play/retry/download tests. Plezy's widget/native/Maestro approach shows how to connect a real fixture server to UI.

**Size:** Medium/large. **Principles and opinion:** Strongly aligned with user-facing correctness. Focus/accessibility/keyboard tests matter more than a large screenshot snapshot count. Avoid mistaking one happy-path emulator run for device acceptance.

### F-25.15 Physical-device acceptance matrix

Record specific OS/device/output combinations for HDR/audio/HDMI/subtitles/remote focus/background tasks and weak-host behavior. Shared code or software decode tests cannot certify them.

**Size:** Large ongoing. **Principles and opinion:** Contracts already require boundary evidence. Stage platform availability honestly if hardware tests are deferred; do not silently reduce required product scope.

### F-25.16 An architecture dependency/cohesion map

Declare allowed imports, package/table/transaction owners, mutable state lifetimes and callbacks. Enforce forbidden edges with a read-only AST/import guard and keep a small diagram with the source.

**Size:** Medium. **Principles and opinion:** Improves 230k production-line maintainability without forcing a framework rewrite. Focus catalog/httpapi/playback/operations cross-seams and compactcatalog publication ownership.

### F-25.17 Test cost and flakiness observability

Record package/test durations, skips, retries and run conditions; isolate real-time/large-fixture tests, use fake clocks where semantics permit and fix unreliable assertions. Navidrome shuffle/race/coverage and Portico runner flags support it.

**Size:** Small/medium. **Principles and opinion:** Supports fast feedback. Never convert flaky correctness tests into permanently optional checks merely to make CI green. No runtime claims are made by this audit.

### F-25.18 Release SBOM and notice completeness verification

Generate source/dependency SBOMs before packaging and verify final artifact contents, including Go, JS, native libraries, copied driver/fonts/icons and FFmpeg dependencies. Do not assume scanning a directory of compressed installers discovers every embedded module.

**Size:** Medium. **Principles and opinion:** No principle conflict. Portico already invokes SBOM/provenance actions; verify their actual inventory rather than claiming absence. Include separate component versions and notices.

### F-25.19 A maintained third-party patch register

Unify SQLite/hls.js/FFmpeg/native fork upstream IDs, rationale, patches, license, upgrade owner, supported targets and retirement conditions; compare upstream contracts automatically where possible.

**Size:** Small/medium. **Principles and opinion:** hls.js and SQLite already do much of this well. The value is preventing pinned security debt and forgotten nominal-type/cancellation differences.

### F-25.20 Community translation intake without general code PRs

Provide export/import format, translator context/screenshots, plurals/placeholder validation, review owner and license permission. Weblate is Jellyfin's implementation; Plezy codegen shows another route.

**Size:** Medium. **Principles and opinion:** Justin must decide whether translation submissions are accepted and by what channel. This can preserve the deliberate no-code-contribution rule; regional English alone does not demonstrate readiness.

### F-25.21 An explicit Apple distribution-rights policy

Inventory first-party/shared/third-party code and choose an Apple-compatible additional license/exception where permitted. Swiftfin MPL and VLC iOS dual licensing are concrete alternatives to assuming a GPL store listing resolves everything.

**Size:** Medium. **Principles and opinion:** Material licensing/product decision, not just a build task. Original code under sole ownership may be grantable differently; dependency rights must be evaluated independently.

### F-25.22 Release-bound documentation verification

Generate current routes/platforms/version/install prerequisites from release inventories; check links and validate commands against supported package layouts. Mark old architecture/backlog entries as superseded instead of leaving them current.

**Size:** Small/medium. **Principles and opinion:** No conflict. Public docs site already exists. Highest value is removing wrong protocol/migration/platform guidance, not replacing it with more prose.

### F-25.23 Succession and review capacity plan

Document who can maintain trust keys/releases/security response/Hosted ops if Justin is unavailable, which boundary changes need independent human review and how agents' evidence is reviewed. Jellyfin's roles offer one larger-project model.

**Size:** Medium. **Principles and opinion:** Requires Justin's governance choice. A backup operator need not gain media access, general code merge rights or product authority. AI review can remain useful without being counted as a second accountable maintainer.

### F-25.24 Voluntary funding and community support routes

Consider donations/sponsorship/transparent maintenance costs, a known-issues guide and community discussions with moderation. Compare Jellyfin/individual support, Plezy donations and Immich optional purchase support.

**Size:** Small/medium. **Principles and opinion:** No requirement to paywall features or launch relay. A bandwidth service may not cover free-product maintenance; make support expectations sustainable before users depend on it.

### F-25.25 Externally usable integration SDK/examples

Publish a minimal independently built authenticated client, API examples, semantic fixtures and optional typed SDK package releases. Jellyfin's SDK ecosystem and OpenSubsonic make third-party support easier.

**Size:** Medium. **Principles and opinion:** Fits Portico's public API promise and thin-client design. Prefer stable external API adapters before an in-process plugin ABI, which is a large additional governance/security commitment.


## Possible bugs and risks in Portico

### B-25.1 Public update manifest cannot be read by the server

Concrete integration defect. The public writer emits top-level schemaVersion/channel/version/tag/assets, but the server decodes a channels map and rejects any feed with zero channels. If an owner sets updates.feedUrl to the official generated asset, background fetching can succeed and the report still becomes unavailable with “shape this server understands” failure; no new-version notice is produced.

**Evidence:** apps/portico-server/scripts/write-public-update-manifest.mjs:21; server/internal/administration/maintenance.go:489,525; administration/update_feed.go:109; operations/settings.go:151.

**How comparison surfaced it:** Plezy's updater feed made it necessary to trace the generator into the consumer rather than assume a file named update-manifest was usable.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already in Backlog:** Not listed. The signing doc mentions unfinished update-feed integration broadly, but not this schema mismatch.

**Suggested consideration:** Small/medium: one shared schema and a test of generated bytes through the current parser. Auto-install can stay disabled.

### B-25.2 Release and Makefile vet still reject the maintained driver

Probable build blocker. CI explicitly excludes internal/thirdparty/sqlite from normal go vet and reruns it with unsafeptr=false, because maintained upstream driver arithmetic generates known diagnostics. Both build-server-release.py and Makefile still invoke unqualified go vet ./.... Thus the regular CI policy and official release/local-build policy disagree; the release can stop before building/sealing even after green CI.

**Evidence:** apps/portico-server/.github/workflows/ci.yml:57; server/internal/thirdparty/sqlite/PORTICO.md:24; scripts/build-server-release.py:149; Makefile:12.

**How comparison surfaced it:** Jellyfin/Plezy centralized quality conventions prompted checking all entry points, not just workflow names.

**Confidence:** Probable. **Severity:** High. **Already in Backlog:** Not listed. PORTICO.md documents the warnings; Backlog's test-runtime item is separate.

**Suggested consideration:** Small: share one vetted analyzer invocation, retaining every analyzer for application code. No Go tool was run to reproduce diagnostics.

### B-25.3 Windows portable ZIP output is relative to its payload directory

Concrete packaging path defect. Release invokes package-release.sh with OUT=dist. The script creates PAYLOAD=dist/payload-windows-x64 (or arm64), then cd's into it and asks zip to write dist/Portico-...-Portable.zip. That nested dist directory is not created by this script, so a clean workflow is expected to fail opening the archive output. An absolute OUT would avoid it, but the actual workflow passes a relative one.

**Evidence:** apps/portico-server/scripts/package-release.sh:10,14,38; .github/workflows/release.yml:132.

**How comparison surfaced it:** Navidrome/Plezy target packaging smoke led to following working-directory changes through archive commands.

**Confidence:** Confirmed by reading. **Severity:** High. **Already in Backlog:** Not listed.

**Suggested consideration:** Small: resolve output/stage paths before cd; installer/archive smoke should use exactly the workflow arguments. No ZIP command was executed.

### B-25.4 Docker release job omits its required FFmpeg input

Concrete workflow-input defect. Dockerfile COPY requires third_party/ffmpeg/bundle, which is absent in a clean source checkout and ignored as a generated bundle. The native server jobs download .toolchain/ffmpeg in their isolated jobs; Docker gets a fresh checkout and has no download/extract/copy step. It requests both amd64 and arm64 from one context, so merely adding one architecture's bundle is insufficient. The release's Docker dependency blocks final publication after native artifacts complete.

**Evidence:** apps/portico-server/Dockerfile:75; .github/workflows/release.yml:113,156,160,167; third_party/ffmpeg/.gitignore:1. Static directory listing showed no bundle.

**How comparison surfaced it:** Navidrome's architecture-aware cross-build inputs prompted checking how media tools reach the multi-arch image.

**Confidence:** Confirmed by reading. **Severity:** High. **Already in Backlog:** Not listed.

**Suggested consideration:** Medium: target-aware bundle stages/contexts or artifacts, verified on both architectures. Docker build was not run; this is a missing dependency in the source graph.

### B-25.5 Hosted apikit has actual drift and its checker is not in CI

Concrete source-policy drift. generate.go in Hosted uses untyped large integer constants assigned into map[string]any; canonical apikit casts them to int64 after a 32-bit fix. The trees are no longer identical, despite the ownership documentation. Hosted's declared release target is amd64, so this is not a claim its present deployment fails; a 32-bit consumer would hit integer representation/compile limits, and future schema behavior can drift the same way.

**Evidence:** apps/portico-server/apikit/generate.go:65; portico-internal/hosted-services/apikit/generate.go:65; portico-internal/scripts/shared-apikit.sh:16; .github/workflows/ci.yml:18,40; scripts/build-hosted-release.py:94.

**How comparison surfaced it:** Published/shared module identities in Jellyfin/Navidrome highlighted missing actual enforcement of the copy promise.

**Confidence:** Confirmed by reading. **Severity:** Low. **Already in Backlog:** Not listed.

**Suggested consideration:** Small: synchronize canonical copy and gate it; pin the upstream source tuple rather than compare whatever sibling happens to be present.

### B-25.6 Official release script can silently invent a signing key

Latent authenticity risk, explicitly unfinished. If runtime/release-signing/private-key is absent, signing_key generates a key on that development/build machine, signs the release and self-verifies against its own public key. That does not establish official identity for independent users. GitHub release checks an injected secret is nonempty, so the fallback chiefly affects local/manual invocations and any future caller that assumes the builder enforces production custody.

**Evidence:** apps/portico-server/scripts/build-server-release.py:35,73,167; server/api/release-signing.md:3,5,9; .github/workflows/release.yml:53,119.

**How comparison surfaced it:** Plezy trusted-build/signing separation surfaced the distinction between cryptographic validity and approved release identity.

**Confidence:** Confirmed by reading. **Severity:** High. **Already in Backlog:** Known in release-signing.md:9, not a distinct Backlog item.

**Suggested consideration:** Medium: official/dev modes, approved pinned signer and fail-closed custody. No key material was inspected and no key leak is alleged.

### B-25.7 Server seal does not authenticate the later web/package payload

Design risk. Signed artifact inventory includes only the Go binary plus source/root/toolchain-manifest identity. Packaging copies independently built web files afterwards and rewrites portico-build.json. A valid manifest.sig therefore does not prove web bytes, launch scripts or the final installer are untouched. Because auto-install is disabled, this is not a current automatic remote-code-install exploit; it becomes critical if verification of this narrower manifest is used to trust the whole app.

**Evidence:** apps/portico-server/scripts/build-server-release.py:158,166; scripts/package-release.sh:21,23,26; Dockerfile:74; server/api/release-signing.md:5.

**How comparison surfaced it:** Plezy whole updater-payload signatures distinguish component sealing from distribution trust.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already in Backlog:** Signing enforcement/update client is broadly acknowledged in release-signing.md; full-web scope is not in Backlog.

**Suggested consideration:** Medium: final artifact inventory and pinned verification, with server source identity retained. Validate attestations/checksums as separate trust mechanisms.

### B-25.8 macOS codesigning changes bytes after the manifest is signed

Conditional concrete integrity defect. When PORTICO_MACOS_SIGNING_IDENTITY is supplied and codesign succeeds, package-macos.sh signs the copied server and FFmpeg binaries after build-server-release.py recorded their digests and signatures. Mach-O signing changes file bytes; the preserved server artifact hash and toolchain hashes describe the pre-codesign binaries. Default unsigned previews avoid this path. Future verification of the packaged signed mode will disagree with those manifests.

**Evidence:** apps/portico-server/scripts/package-macos.sh:14,29; scripts/build-server-release.py:82,94,158,167; .github/workflows/release.yml:151.

**How comparison surfaced it:** Plezy signing/notarization ordering prompted tracing the Portico seal through platform mutation.

**Confidence:** Confirmed by reading. **Severity:** High. **Already in Backlog:** Not listed.

**Suggested consideration:** Medium: OS-sign before final digest/manifest seal, or explicitly model signed derivative identity. Qualify on macOS; no signing or verification was run.

### B-25.9 Release does not require the full media/performance tiers

Qualification gap. Public CI and application release builder leave PORTICO_TEST_TIER and PORTICO_PERFORMANCE_TIER unset. Tests using testtier.Media skip by default; decoder helpers also skip missing tools unless required. The separate FFmpeg workflow runs two targeted playback/subtitle tests with required toolchain, which is valuable, but neither that pair nor a green default app suite proves the changed server's full media/live/scale behavior.

The selected target bundle is validated before tests but copied into the snapshot's third_party/ffmpeg only after go test/vet (build-server-release.py:110,148,165). decodertest discovers the canonical third_party/ffmpeg/bundle path or falls back to PATH; it does not read the --ffmpeg-bundle argument (mediatools/resolve.go:139,168; decodertest/tools.go:26,50). Thus app-release host tests are not guaranteed to exercise the exact downloaded media bundle, and target bundles cannot simply be executed in Linux-host tests for Windows/macOS. Separate qualification evidence is essential.

**Evidence:** apps/portico-server/server/internal/testtier/testtier.go:11,44; internal/decodertest/tools.go:13,47; .github/workflows/ci.yml:49; scripts/build-server-release.py:148; .github/workflows/ffmpeg-toolchain.yml:32,70.

**How comparison surfaced it:** Plezy native CI/Navidrome broad test gates prompted testing the gate definition rather than counting available tests.

**Confidence:** Confirmed by reading. **Severity:** High. **Already in Backlog:** Backlog already notes default runtime and --full timeout; manual integrator all-tier convention is in testtier docs. It does not establish required release execution.

**Suggested consideration:** Medium: explicit qualified tier evidence for each app source/release. This audit cannot claim the tests have never been run manually.

### B-25.10 Apple/Vega and physical native behavior are not automated release gates

Design/qualification risk. Native CI builds Android debug and runs JS tests, but cannot catch a broken Swift/Obj-C/tvOS bridge or Vega package. No native release workflow in this tree validates stores or signed release-mode builds. Apple is the reference client and new ports widen maintenance load. This matters when platform readiness is claimed; it is not proof a current client is broken.

**Evidence:** apps/portico-react-native/.github/workflows/ci.yml:16,38,49; apps/native/package.json:9; apps/vega/package.json:6; Project Architecture/08 Operations.md, Testing; Product Contracts/governance.md, qualification paragraph.

**How comparison surfaced it:** Plezy Apple/native/desktop CI and Swiftfin build lanes provide the comparison.

**Confidence:** Confirmed by reading. **Severity:** High. **Already in Backlog:** Backlog lists port/device/UI work and scope-completion keeps qualification explicit. It is partly known, though stale entries obscure status.

**Suggested consideration:** Medium/large: Apple/Vega builds and platform/native/E2E suites with separate real-device evidence. Source and emulator tests cannot substitute for HDMI/audio/focus acceptance.

### B-25.11 Font/icon notices promised in docs are missing from source distribution

Concrete source notice/doc mismatch and probable binary notice gap. THIRD-PARTY-NOTICES says Manrope's font license is beside distributed files and Lucide license/provenance is retained with generated assets. The inspected web fonts directory has only manrope-var-latin.woff2; native assets contain four Manrope TTFs but no font license. design/src/icons.ts declares Lucide-derived artwork, while design directories have no ISC license/provenance file. Native has no root third-party notices. The separate server mediaexec/fonts/LICENSE is for Bitstream/DejaVu, not Manrope. Broad binary Go/JS notice completeness also remains unproved: package-release copies only the root GPL/summary and FFmpeg payload, not SCALE_LICENSE or the SQLite driver BSD text.

**Evidence:** apps/portico-server/THIRD-PARTY-NOTICES.md:7,30,33; web/src/ui/fonts/manrope-var-latin.woff2; packages/design/src/icons.ts:5; server/internal/imagework/SCALE_LICENSE:9; server/internal/thirdparty/sqlite/LICENSE:1; scripts/package-release.sh:24; apps/portico-react-native/apps/native/assets/manrope-400.ttf.

**How comparison surfaced it:** VLC/Swiftfin component licensing and Plezy vendored plugin practice motivated actual asset/notice inspection.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already in Backlog:** Not listed.

**Suggested consideration:** Small/medium: correct notices/provenance and verify final packages/acknowledgements. This finding establishes missing source notices and recipe omissions, not a final legal ruling about an unseen published artifact.

### B-25.12 GPL Apple distribution rights remain unresolved

Licensing/distribution risk. Native and copied shared packages are GPL-3.0-or-later, with no Apple-specific exception or alternate grant found. Apple availability requires assessing restrictions/rights for the exact components and distribution terms. Sole ownership can simplify a grant for original code; it cannot change third-party rights. Plezy's GPLv3 file and store availability cannot serve as legal precedent. Swiftfin uses MPL, and VLC iOS explicitly dual-licenses.

**Evidence:** apps/portico-react-native/LICENSE:1; apps/portico-server/README.md:90; swiftfin/LICENSE.md:1; plezy/LICENSE:1; [VLC iOS COPYING](https://github.com/videolan/vlc-ios/blob/master/COPYING).

**How comparison surfaced it:** The different Swiftfin/VLC licenses revealed a question a successful iOS build does not answer.

**Confidence:** Speculative — needs a runtime check (and a distribution-rights review; runtime tests alone cannot resolve licensing). **Severity:** High. **Already in Backlog:** Not found in Backlog.

**Suggested consideration:** Medium: obtain a component-specific rights/distribution decision before store release. No infringement or store rejection is asserted.

### B-25.13 Bare-goroutine guard is a line-regex gate, not a complete AST gate

Latent guard bypass. The ordinary application rule matches only lines beginning with go plus a simple function call/literal. A legal inline Go statement after another statement on the same line, such as an if-body with go worker(), is not matched. The code already parses AST for its one upstream exception, but not for the universal rule. No existing offending production goroutine was identified; the defect is the test's completeness claim.

**Evidence:** apps/portico-server/server/internal/supervise/discipline_test.go:25,39,95.

**How comparison surfaced it:** Jellyfin analyzer/forbidden-symbol controls prompted inspecting how Portico's claimed architectural guards parse source.

**Confidence:** Confirmed by reading. **Severity:** Low. **Already in Backlog:** Not listed.

**Suggested consideration:** Small: inspect every ast.GoStmt and apply exact-path/function exceptions; keep tests for inline/multiline syntax. This is a guard weakness, not a demonstrated panic exploit.

### B-25.14 SQL guards explicitly exclude runtime-assembled statements

Guard coverage risk. productionStatements folds complete literal expressions but returns false for dynamically assembled SQL so their fragments are not individually prepared. That is correct for fragments, but the docs' “prepare every SQL statement” wording is broader than the actual gate. Query-plan allowlists and synthetic statistics also constrain what is proven. Dynamic filters/sorts still require semantic/plan tests; a green guard cannot establish universal O(visible) behavior.

**Evidence:** apps/portico-server/server/internal/persistence/statements_prepare_test.go:25,70; statements_scan_test.go:132,148; Project Architecture/02 Server.md, Database section.

**How comparison surfaced it:** Comparing ORM/API analyzer claims against real scope revealed this limitation.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already in Backlog:** Backlog acknowledges sparse-filter/search/shuffle costs, not the general extraction boundary.

**Suggested consideration:** Medium: enumerate query constructors and test representative/all allowed shapes; narrow docs. Do not classify every runtime-built SQL string as unsafe.

### B-25.15 Hostile-input test rotates cases despite exhaustive-sounding prose

Test blind-spot risk. TestEveryRouteSurvivesHostileInput selects body/query/path by route index modulo each list. Each route receives one combined hostile shape per auth mode, not every shape; adding a route changes later coverage. A later test runs every hostile body over a smaller selected route list. The broad test also accepts 503 and has no per-request timeout for ordinary routes, so its comment about every request's own deadline overstates it.

**Evidence:** apps/portico-server/server/internal/httpapi/hostile_input_test.go:28,181,186,198,208,222.

**How comparison surfaced it:** Parser/fuzz practices in Jellyfin/Plezy prompted reading the loops rather than relying on test names.

**Confidence:** Confirmed by reading. **Severity:** Low. **Already in Backlog:** Yes: Backlog's Hostile-input test rotation item. It remains present in current source.

**Suggested consideration:** Small/medium: stable per-route batteries, explicit deadlines and allowed 503 justification. Keep runtime manageable by separating parser/route families.

### B-25.16 All-tier runner timeout remains below recorded package cost

Known test-delivery risk. Backlog records httpapi all-tier at 2,082 seconds and preset timeout 30 minutes. runner-test.sh still defaults -timeout 30m, with --full selecting all unless overridden. That means the documented full preset is expected to terminate the recorded workload before completion on equivalent hardware; expensive queue scale fixtures distort feedback. Current runtime may differ after October changes, so failure was not reproduced.

**Evidence:** apps/portico-server/scripts/runner-test.sh:36,42,211,219; portico-internal/Project Architecture/Backlog.md, Server section.

**How comparison surfaced it:** Peer test-matrix/runtime design highlighted whether expensive Portico tests can actually finish under advertised commands.

**Confidence:** Probable. **Severity:** Medium. **Already in Backlog:** Yes, explicitly. Also default-under-five-minutes remains a goal, not an achieved metric.

**Suggested consideration:** Small/medium: move truly scale workloads to the performance lane or use a justified timeout, then record durations. Do not merely skip difficult tests.

### B-25.17 Python schema integration test references removed files

Concrete dormant test defect. server/tests/i02_schema_test.py's DDL list reads persistence/revisions.go, admin.go, library_inventory.go and console.go, which are absent. setUp executes literal() for that list, so invoking the documented command cannot reach its assertions. persistence/database.go also no longer contains the stated PRAGMA literal. It is outside go test/npm and is not referenced by the examined CI or Makefile.

**Evidence:** apps/portico-server/server/tests/i02_schema_test.py:4,14,21,33; current server/internal/persistence directory; .github/workflows/ci.yml:49; Makefile:10.

**How comparison surfaced it:** Comparison test inventory distinguished reachable suites from historical fixtures left behind.

**Confidence:** Confirmed by reading. **Severity:** Low. **Already in Backlog:** Not listed.

**Suggested consideration:** Small: retire as dated evidence or port it to current schema/invariants and invoke intentionally. It is not evidence of present integration coverage.

### B-25.18 Migration chain violates the documented no-startup-backfill rule

Concrete policy/code disagreement with scale risk. Migration 0008 performs three INSERT...SELECT passes over existing show metadata/genres during migrate; 0003 drops a personal_items column and deletes old evidence; 0011 creates an index over existing recommendation signals. migrate executes each pending SQL inside the upgrade path. 0008's cost grows with stored shows/genres, contrary to “never rewrite a large table at startup”/background backfill. Index creation is also data-dependent even when additive. This is permitted to break pre-release compatibility, but not bounded-time by construction.

**Evidence:** apps/portico-server/server/internal/persistence/migrations/0008_show_facets.sql:17,21,26; 0003_drop_reaction.sql:6; 0011_recommendation_positive_seeds.sql:5; persistence/schema.go:130,135; persistence/README.md:29,33,41.

**How comparison surfaced it:** Navidrome migration validation and large-library release comparison led to examining actual SQL, not just the policy.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already in Backlog:** Backlog discusses show facets and schema state, not this startup cost/policy exception. Some feature entries are stale because migration 0008 already exists.

**Suggested consideration:** Medium: classify historical pre-release exceptions, measure upgrade work and use resumable post-ready backfills/derivation versions for future large changes. No timing or data-loss claim is made.

### B-25.19 Manual release can test caller branch rather than requested tag

Concrete gate-scope risk. workflow_dispatch accepts an arbitrary existing tag, identity/web/server jobs checkout that tag, but the reused CI workflow has no ref input and its checkouts use the caller context's default github ref/SHA. When a maintainer dispatches from main while requesting an older/different tag, the CI web/core/generated-contract gate tests the dispatch revision, while packaging uses the tag. Snapshot builder tests tagged server code, but does not replace tagged web/core/contract CI. Tag-push releases do not have this mismatch.

**Evidence:** apps/portico-server/.github/workflows/release.yml:16,41,57,66,99; .github/workflows/ci.yml:7,32,73.

**How comparison surfaced it:** Plezy trusted-ref release controls motivated checking all artifact/check revision identities.

**Confidence:** Confirmed by reading. **Severity:** High. **Already in Backlog:** Not listed.

**Suggested consideration:** Small/medium: pass exact resolved tag SHA into every reusable workflow and verify artifact/check metadata matches. Remote dispatch/run logs were not queried.

### B-25.20 Linux upgrade lifecycle stops but does not restart the service

Probable packaging behavior defect requiring artifact lifecycle qualification. The package preremove script stops and disables the service without checking removal-versus-upgrade arguments. postinstall only daemon-reloads/enables it, never starts/restarts it. Under ordinary DEB upgrade ordering the old process is stopped and no new one starts; RPM upgrade ordering can additionally interact with old-package removal after new install. Fresh-install enable-at-boot may be deliberate, but an upgrade remaining down is not documented as a required owner step.

**Evidence:** apps/portico-server/packaging/linux/preremove.sh:3; postinstall.sh:6; nfpm.yaml:31; getportico-tv/src/content/docs/administration/update-portico.md:15.

**How comparison surfaced it:** Navidrome/Plezy package smoke coverage prompted reading maintainer script lifecycle, not assuming systemd packaging handles it.

**Confidence:** Probable. **Severity:** Medium. **Already in Backlog:** Not listed.

**Suggested consideration:** Small/medium: honor package lifecycle arguments, preserve enable/running state, start/restart after safe upgrade and test both DEB/RPM. No installer was built or installed.

### B-25.21 Cross-repo CI checks moving default branches, not recorded dependency revisions

Design risk with concrete workflow scope. Native's shared check and Roku generator check out sibling repositories without ref, so their truth changes whenever the foreign default branch changes. A valid native PR with synchronized copied code can fail because server main has advanced, and a historical rebuild cannot reconstruct its inputs from the native/Roku SHA alone. Server CI does not trigger native/Roku on a canonical package change. Current native contents match, so this is not actual selected-package drift today.

**Evidence:** apps/portico-react-native/.github/workflows/ci.yml:23; apps/portico-roku/.github/workflows/ci.yml:23,25; apps/portico-react-native/scripts/shared-packages.mjs:16; apps/portico-server/.github/workflows/ci.yml:70.

**How comparison surfaced it:** SDK/versioned dependency practices and Plezy single-tree sharing highlight atomicity and rebuild differences.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already in Backlog:** Not listed.

**Suggested consideration:** Medium: pin upstream tuple, trigger dependent validation and update intentionally. The source-only report cannot establish current green/red remote jobs.


## Where Portico is stronger

Portico has several engineering choices worth keeping. None below substitutes for platform/release qualification.

1. **Scale invariants have executable defenses.** Real SQLite statement preparation, query-plan checks, deterministic catalogue fixtures and revision-aware state are substantially stronger than a vague “fast server” claim. They directly reflect the 5–10-million-item principle. Keep the actual SQL-visible approach and be precise about extraction/allowlist limits (A-25.22/23; B-25.14).
2. **Portable server builds avoid cgo.** A pure-Go server plus explicit media tools is easier to cross-compile and install than a linked native database stack. Navidrome still uses cgo for SQLite even though its current TagLib extractor has moved to Go/WASM. Do not market the entire media distribution as a single dependency-free binary: FFmpeg/sandbox/runtime assets remain.
3. **Private causes and public errors are separated.** Stable error codes, retry headers, localized client strings, authority-concealing 404s and response-contract tests are a useful thin-client boundary. Preserve those instead of exposing arbitrary raw errors.
4. **Cancellation and memory accounting were taken into the database boundary.** The local driver patch is narrow, attributed and documented with exact upstream identity and upgrade obligations. That is more maintainable than a broad undocumented fork. Its ongoing cost should be visible.
5. **Forked browser/media assets have meaningful provenance.** hls.js pins source/patch/distribution hashes and describes the narrow change. FFmpeg has a separate manual approval/feature/license/source policy. These mechanisms are worth extending consistently to native dependencies.
6. **Copy ownership is explicit and currently native copies match.** There are not two silently competing authors of client-core. The checksum checker is useful; it needs version pinning and dependent CI, not dismissal as inherently bad.
7. **Local diagnostics preserve privacy.** Owner telemetry is deliberately local, unavailable metrics stay unavailable, and diagnostics reports have bounded sizes and whitelisted fields. internal/diagnostics/report.go:15,49,105 does not indiscriminately serialize logs or playback tables. A user-controlled evidence export is a better fit than automatic third-party uploads.
8. **Container basics are thoughtfully handled.** Nonroot, tini, explicit FFmpeg and a credential-free health probe are present. The Docker input/release defect should be fixed without losing these safeguards.
9. **Contracts distinguish intended behavior from actual evidence.** coverage/registry explicitly refuse to certify implementation just because prose is complete. Cross-domain hostile scenarios preserve knowledge independent of any one agent session. Maintain those documents as authority/qualification maps, not a second untested codebase.
10. **Manual update restrictions are honest.** Unsigned preview warnings and AutoInstall=false protect owners from premature unattended rollout. The mismatched feed is an integration bug, but automatic-install restraint itself is sound.
11. **No-contribution policy is stated plainly.** Users know where feedback belongs and security reports stay private. A deliberate governance boundary is more honest than soliciting PRs a sole maintainer cannot review.
12. **State downgrade refusal exists.** persistence/schema.go:117 rejects unknown applied migrations and checks applied digests. That is safer than silently running an old binary against a newer schema. Migration/backfill cost and rollback policy still need work.

The first release work I would prioritize is F-25.1–9: compatible upgrade behavior, exact qualification identities, complete target packaging and artifact trust. The first maintainability work is F-25.16/19/22/23: coherent boundaries, patch ownership, current docs and review/succession capacity. This is an opinion about consequences, not permission to change the product.

## Doc/code disagreements

| Document statement | Current source evidence | Consequence / opinion |
|---|---|---|
| Overview describes web/Apple users, future Android ports, and omits Roku from its repository table (01 Overview.md:3,14,42,54). | Native has Android/Fire variants and Vega app; Roku is a separate active repo with CI. | Update platform/repository inventory and distinguish implemented ports from qualified releases. “Apple reference experience” can remain true without saying ports are merely future. |
| “Copies are identical” (01 Overview.md, shared ownership section). | Selected native files match; Hosted apikit/generate.go does not (B-25.5). | Keep ownership rule, correct current copy and make equality a real gate. |
| Server doc says one baseline migration and additive/no-startup rewrites (02 Server.md:25). | Eleven embedded migration files, schemaVersion=11; 0003 drops a column and 0008 backfills metadata synchronously. | Separate baseline history from current chain, record pre-release exceptions and future policy (B-25.18). |
| Persistence README tells pre-first-release schema changes to edit baseline (README.md:17). | Later numbered migrations already exist and are validated immutably. | Define current schema evolution rules; do not encourage editing a baseline already applied to active test databases. |
| Every state-owned file is referenced relatively (02 Server.md:18). | persistence/rebase.go:21,45 explicitly stores/rebases absolute paths across multiple state-owned columns. | Portability is implemented with startup rebasing, not universal relative storage. Copying the folder across different OS/path conventions remains unqualified here. |
| Every goroutine starts through supervise.Go (02 Server.md:9). | Exact upstream SQLite interrupt watcher exception is allowed; Loop/Supervise exist; ordinary guard is regex-based (discipline_test.go:27,95). | Document the exception and strengthen the checker; do not infer all goroutines use one spelling or complete static enforcement. |
| SQL guards prepare every statement (02 Server.md:32). | Runtime-assembled expressions are deliberately excluded by statements_prepare_test.go:70. | State the guard's literal-query scope and require query-constructor tests. |
| BUILDING requires Node 24 (BUILDING.md:9); release/web Docker uses 24. | Public server client CI uses Node 22.22.2 (ci.yml:76); native uses 24 and package engine thresholds vary. | Set supported/tool-qualification matrix or unify versions. Different versions may be deliberate, but the current guide does not explain them. |
| Public release process says tests/build/seal/packages complete from reviewed tag (RELEASING.md:17). | Manual dispatch CI lacks the requested-ref parameter; source builder/Makefile vet differs from CI; Windows/Docker inputs are defective (B-25.2/3/4/19). | Documentation describes intended pipeline, not established success. Fix and verify actual clean-checkout runs before claiming readiness. |
| Each development release provides all planned packages (INSTALLING.md:10). | Release YAML intends them, but clean-path packaging contains blockers; actual release execution/assets were not inspected. | Phrase unreleased inventory as planned until artifacts exist and install-smoke evidence is current. |
| Every server build is sealed (RELEASING.md:26), alongside Docker in package list. | Docker container-release identity expressly uses proof='unsealed' (container-release.mjs:30); Docker has its own provenance and no same manifest.sig builder path. | Explain binary/source seal versus container provenance accurately. Neither should masquerade as the other. |
| Font/ISC license/provenance is retained beside generated assets (THIRD-PARTY-NOTICES.md:30,33). | Web/native Manrope directories and design icon source lack those records in inspected trees (B-25.11). | This is a concrete notice gap, not just outdated prose. Include actual texts in source and final distributions. |
| API README presents /v2 as playback control plane and /v1 as original session model (server/api/README.md:25,30). | Current registry mounts Playback Protocol v1; v1OrLegacy dispatches old versus new formats and emits successor Link to /v1/playback/sessions (httpapi/playback_v1.go:314,329,343). | Multiple protocol generations coexist; README's overview is not a safe client-version map. This audit does not resolve every endpoint; publish one current route/capability/deprecation inventory. |
| API docs say pre-release routes are reshaped/deleted rather than deprecated (api/README.md:47). | markDeprecated returns Deprecation:true and successor-version Link (playback_v1.go:343). | Existing transitional behavior is useful, but current compatibility intent needs clarification. Do not infer a post-release deprecation duration. |
| Backlog is “as of 2 October,” says ports in flight, old branches unmerged and old demo/build/schema state. | Current histories start at 8 October baseline; migrations reach 11, ports/CI and later fixes are in current trees; 08 Operations was partly refreshed. | Backlog is dated evidence, not a current completion ledger. Reconcile entries against current source/evidence and retain historical IDs. No current demo request was made. |
| BUILDING says main verification is make test; server's Python schema test documents a direct invocation. | Makefile does not run contract tests/generation like CI, omits Python/E2E setup checks; Python test references removed files. | Document the check hierarchy and retire/port unreachable tests (B-25.17). There need not be one all-purpose command, but each gate must be clear. |
| Release custody says installer verifies pinned public key and procedure needs enforcement (release-signing.md:5,9). | Key fallback self-verifies; public feed is unsigned; installers do not perform that verification. | The document correctly flags unfinished intent. Keep it explicit and do not describe existing preview packages as implementing the future trust model. |

Comparison docs also drift. jellyfin-docs/source-tree.md lists several retired .ci/Emby projects, and its release-procedure links mix X.Y and older X.Y.Z examples. jellyfin-web candidly documents legacy/modern migration debt. Peers should provide questions and patterns, not unquestioned specification authority.

## Open questions for Justin

No response is needed for this audit run. These decisions remain open in the document.

1. What exactly makes the first release stable: which platforms/features are advertised, which physical outputs/devices are accepted, and which evidence must be current at the tagged source?
2. What server/client support window will v1 promise, especially for TV/store clients that cannot update immediately? Is compatibility capability-based, version-based, or both?
3. What API/schema/default changes require a major release, and what deprecation/security backport period is sustainable for a solo owner? Is any LTS promised, or explicitly no LTS?
4. Which database upgrade paths and rollback combinations are supported? Is a newer-schema refusal enough, or must a verified pre-upgrade backup/restore path be supplied by installers?
5. Should historical pre-release SQL migrations remain documented exceptions, or should the release baseline/upgrade contract be reconciled before freezing v1?
6. Are private archive bundles/history indexed and preserved well enough for future regression blame, attribution and past device/approval evidence, and who can access them if Justin is unavailable?
7. Keep the split/copy model with pinned tuples, publish shared workspace/apikit packages, or move public clients into a monorepo? What independence/privacy/build ergonomics matter most?
8. Should shared copies include generated contracts and reusable tests? How should native adopt the canonical contracts types without coupling clients to every server implementation detail?
9. Does an official local release require a protected signer/approved public key, or can local developer-sealed builds exist under a separately named identity?
10. Will final web/media/package bytes be part of the authenticated release manifest? How will platform codesigning/notarization interact with those digests?
11. Is “stable” intentionally a preview channel until release, or should preview/beta/stable be distinct for machines as well as humans? Should the official update notice feed be an explicit first-run opt-in?
12. Is automatic server installation a desired feature after signing/rollback qualification, or should package managers/manual owner action remain the permanent delivery model?
13. What Apple-compatible licensing grant/distribution policy is intended for original native/shared code, and who will review the dependency rights before App Store submission?
14. Are community translations, documentation text and test/media fixtures considered external code contributions? If accepted, by issue, translation service or reviewed non-code PR, and under what rights grant?
15. Which maintainability boundaries should become mandatory: forbidden imports, table/transaction owners, maximum handler responsibility, shared error mappings, structured local log keys?
16. For AI-assisted changes, which critical boundaries get independent human review, what constitutes an independent agent review, and which assertions need runtime/device proof before acceptance?
17. What default/race/media/performance/fuzz cadence can fit available runner capacity while still being a release gate? Which expensive tests should move out of httpapi fixtures into dedicated tiers?
18. What public support capacity is realistic across server, web, Apple, Android/Fire, Vega and Roku? Should community discussions supplement issue reports, and who moderates/triages?
19. Can voluntary donations/sponsorship cover free-product maintenance while preserving all-free core behavior? What assumptions make future paid relay economics support, rather than distract from, the product?
20. Is there a named successor/recovery operator for signing/security/Hosted releases, and how can that role remain least-privilege without transferring product authority or private media access?
21. Should third-party integrations be HTTP/SDK-only at release, or is an eventual plugin/adapter ABI intended? What real unmet extension use case justifies its long-term compatibility burden?
22. How should documentation authority be published: released API guides, generated schemas, semantic vectors and user docs, with private contracts retained as internal decision records?
23. Is an unconfigured update feed an intentional permanent default for privacy, and how will owners learn of security fixes without central usage/crash telemetry?
24. What are the promised startup/upgrade scale budgets on specified hardware? Are data-dependent index creation and schema backfills excluded from the “seconds at millions of items” promise or redesigned?

## Evidence boundary

This document contains 60 architectural comparisons, 25 capability considerations and 21 source defects/design risks. It is a one-time snapshot, not a certification.

**What was established:** current file contents; source/reference relationships; package/file/test physical sizes; local import direction; exact selected shared-copy equality/difference; workflow triggers/jobs/arguments; missing clean-checkout inputs; incompatible JSON schemas; preserved/absent source notices; current reachable Git history; small public peer-release date samples. Those findings can be independently reread without running Portico.

**What was not established:**

- No builds/tests/installations were run. No Go tooling, dependency install, formatter, generator, fuzz execution, benchmark, FFmpeg qualification or code coverage was executed. Reported runtime figures come from dated Backlog evidence, not this run.
- No physical device, emulator, browser, demo server or Hosted endpoint was contacted. Android/Fire/Vega/Apple/Roku media/focus/accessibility/output acceptance, weak-host scale, concurrency and flakiness remain unestablished.
- No Portico private credential/key folder, native authentication state or production user data was inspected. Public-root presence is not evidence of private-key custody or trust-root operational recovery.
- GitHub branch rules, required checks, release environments, installed private-report settings, actual workflow logs, signer/notarization credentials, published Portico packages/images and store submissions were not queried. YAML is implementation intent; several deterministic source defects explain why that intent may not execute.
- No released installer/container was extracted to verify licenses or SBOM inventory. The font/icon source omissions and packaging recipe are concrete; a full legal/binary compliance conclusion would require exact distributed bytes and rights/terms review.
- GPL/App Store analysis is a flagged distribution decision, not legal advice or a finding of infringement. Public FSF/GNU FAQ fetches were attempted but timed out; local GPL text and actual Swiftfin/Plezy/VLC component licenses support the comparison, not a complete Apple terms analysis.
- Comparison repositories were read as untrusted data and never executed. Their shallow histories and public labels do not prove maintainer counts, audit quality, contributor competence, complete compatibility or future support.
- Immich was assessed from current primary public upgrade/migration/funding material, not a source clone. Closed-source Plex/Emby/Infuse comparisons are confined to public funding/features; detailed internal engineering and full telemetry collection cannot be inferred.
- Only representative HTTP/catalog/error/context/SQL/test/release paths were deeply read. It would be inaccurate to claim manual semantic review of all 232,280 production Go lines or 815 test files. Hosted backup/deploy scripts were structurally reviewed; no operational correctness/restore/HA certification is included.
- Current revision-keyed database/read fences, media authorization and broad product functions belong to other audit domains. This review assesses their engineering boundaries/test/maintenance model, not all feature semantics.

Several apparent gaps are intentional pre-release staging: newest-build-only security fixes, manual updates, no external code PRs, private Hosted implementation and unqualified new platforms. Their deliberate status does not erase architectural consequences. Conversely, existence of a policy, test, fixture, build job, store listing or signed component does not establish that the whole product meets the contract.

Outside my domain: absolute state-path rebasing, legacy/new playback namespace coexistence and local-owner retention/privacy details may deserve follow-up in the storage/playback/identity audits. Only their documentation, release and maintenance implications were pursued here.
