# Cross-domain implementation architecture

Revision: portico.reconciled.1. This is normative architecture for the rebuild, not a prescribed package-by-package rewrite or a claim that these mechanisms already exist.

The [domain amendments](architecture-amendments.md) complete the family contracts and resolve their named cross-domain conflicts.

## Application boundaries

Retain the Go modular local server with normalized SQLite on reliable local storage, Go/PostgreSQL Hosted, headless TypeScript client services, React DOM web and React Native clients with native platform adapters. Storage/FFmpeg/provider integrations run through bounded adapters. A monolith is appropriate; one giant application owner is not. Separate deployable microservices are not required to enforce ownership.

| Owner | Durable facts / publication responsibility |
|---|---|
| Identity and access | Accounts, profile identity/security, session families, effective permission and revocation |
| Sources and inventory | Storage definitions, source configuration, root/object/revision evidence, inventory generations |
| Catalog | Stable logical items, versions and source links, hierarchy, catalog lifecycle and read projections |
| Metadata | Evidence, candidate/accepted provider identity, field decisions/locks, hierarchy reconciliation and artwork selection |
| Personal state | Watched/resume arbitration input, history, favorites/reactions/ratings, Saved resources and shares |
| Playback | Document 32 occurrence/control, planning, source delivery/producer lifecycle and progress authority |
| Offline preparation | Exact prepared artifact, transfer grant and signed authorization receipt; calls playback/media preparation through an internal interface |
| Live/DVR | Guide generations, programme identity, tuner leases, scheduled commitments, recording publication and retention |
| Library Channels | Programmed schedules and tune resolution over catalog identities; never a tuner allocator |
| Social/receivers | Group and controller authorization, queues and commands; calls playback's handoff and occurrence interfaces |
| Operations | Common job infrastructure, settings application, audit, backup/restore/update orchestration |

Application services call domain/repository interfaces. HTTP, React, native player objects and shell processes do not own durable intent. SQL storage may share a database, but each table family has an owner; other domains use that owner's transaction-capable methods or explicit projection events. The integration lead coordinates cross-domain migrations once. Avoid direct mutations to another domain's tables merely because they are accessible.

Data aggregation uses indexed projections, bounded queries and keyset continuations. Large catalog work does not load a whole library into memory or hold a transaction while users page. A single SQLite connection is acceptable only as a measured current integration limitation; production read concurrency needs a deliberate bounded pool/reader strategy and serialized short writes, with PRAGMAs and cancellation applied consistently. Do not change engines before measuring the actual bottleneck. Background saturation cannot hold the only control-plane connection indefinitely.

## Commands, receipts and current state

An external mutation has stable operation identity, canonical request digest, authenticated stable actor/resource, relevant expected revisions and a defined retry lifetime. Commit atomically records domain effects, immutable result receipt, necessary audit and durable outbox work. Same identity/body returns the original result; different body rejects. Permission is checked before disclosing a receipt, so replay cannot restore revoked access or leak old private results.

Receipts prove what committed, not what is current now. A response may separately include a fresh materialized state/revision. A historical accepted invitation does not prove current membership; an old settings receipt does not prove that configuration remains active. Domain snapshots determine current state.

Cancellation and timeout are different: timeout means unknown until reconciliation. Client abort immediately fences stale presentation but cannot roll back a committed server mutation. Reuse the same operation identity for reconciliation; never mint a new one automatically to escape ambiguity. An explicit later user action is a new intent with new identity.

Deduplication retention is published. After a receipt is pruned, a delayed old mutation cannot be treated as new: require a server-issued request epoch/window, a compact retired-operation fence or an equivalent persistent monotonic command rule. Expired requests return an explicit expired/unknown outcome and require fresh state before a genuinely new user intent. Keep sensitive operation fences across restore using the restore incarnation. Do not keep every receipt forever or introduce arbitrary lifetime user-action quotas to avoid designing retention.

## Jobs and fencing

A durable user operation owns attempts, child jobs, effective policy and a current cancellation/revision fence. Attempts carry a worker lease token, expiry and monotonically advancing attempt/lease generation. A process-local cancel map is an optimization, not ownership. Claim, heartbeat, cancellation and publication compare the exact token/generation in the database.

Worker flow: claim short transaction → perform bounded external work → revalidate source/policy/authority and cancellation → stage/validate output → fenced publication transaction → acknowledge completion. No network, filesystem walk, blocking media process, large parse or remote user interaction occurs inside the transaction. If authority changed, discard/quarantine staged results; do not silently retry with expanded permissions.

Cancellation durably prevents later publication immediately. UI may show cancellation-requested while the helper terminates or private cleanup remains. Only confirmed settlement is terminal cancelled. Automatic retries stay under the same intent and advance attempts; explicit Retry of a terminal scan may create a linked operation without reopening a cancelled identity. Cancelled/superseded attempts cannot publish merely because they reacquired a lease.

Parent completion uses durable child counts or a closed continuation frontier. A queued continuation is outstanding work, not completion. Report inventory-ready, enriching, complete-with-warnings, failed and cancelled distinctly. Bounded pages and per-source fair scheduling must eventually visit every eligible item; LIMIT N is a page size, not a scope truncation.

## Database plus filesystem publication

There is no general atomic transaction across SQLite/PostgreSQL and a filesystem/provider. Use a journaled publication protocol:

1. Record operation, desired artifact identity and unique private attempt location.
2. Write and validate outside the DB transaction; close/sync as required by the durability class.
3. Install an immutable content-identified artifact on the same filesystem using atomic rename, or record the platform-specific durable installation result.
4. Recheck all fences and transactionally commit the authoritative reference/selection and outbox. This is product publication.
5. On crash, a reconciler distinguishes installed-but-unreferenced, referenced-and-valid, missing/corrupt and stale-attempt artifacts. Retain or quarantine safely, repair explicitly, and delete only when unreferenced and unleased.

An orphan is tolerable temporarily; an authoritative reference to unverified bytes is not. Do not claim exactly-once external execution. Execute idempotently or reconcile ambiguous provider acceptance before repeating it. Disk-full/rename/permission errors preserve the last good selection. Object generation and content digest—not a filename or process exit code—bind publication.

## Events, read projections and client state

Outbox events are committed alongside domain state and emitted after commit. They have owner/resource identity, revision and bounded safe payload. Subscription transport may vary; semantics do not. Duplicate/out-of-order/gapped events trigger deduplication or a current snapshot, never blind application of old authority. Subscriptions are reauthorized and fenced when access changes. Slow consumers have bounded queues and resnapshot behavior.

Cursors bind stable owner, authorization fence, query/filter/sort, locale/collation, catalog/projection generation and stable tie-breaker. A stale cursor returns a typed restart-required outcome. Keep prior valid content visible while refreshing where still authorized; never carry stale private results across revoked scope. Bulk selection/query jobs capture intended membership as a paged immutable generation, not a live query whose membership can silently grow after confirmation.

Headless services own scoped request cancellation, stale-response checks after body parsing, deadlines, mutation receipts and coherent snapshots. UI owns drafts/focus/layout. Cached content and authentication are separate. Independent panel/row failures do not blank healthy data; unsupported, denied, offline, empty and failed are distinct. Localization and semantic action IDs are versioned without server-delivered arbitrary UI code.

## Versioning and scale

Each new domain defines a reviewed serialized API in its implementation package before dependent clients build against it. No old schema generator is imported. Internal DB revisions, catalog revisions, authorization revisions and playback generations have distinct types/meaning. Large identifiers/counters/timelines must survive Go/JSON/JavaScript without rounding; use the representation chosen by the owning reviewed schema, and document conversions at adapters.

Provider snapshots/receipts may store bounded immutable JSON as evidence. Queryable catalog, ownership, job state, references and permissions are normalized relations, not a serialized application object graph. Millions of items require representative query plans and bounded ingestion/continuation memory; initial 200,000-server Hosted capacity requires measured steady state and restart/storm workloads. These are targets, not achieved benchmarks.

## Architectural review gate

Before a domain is described as complete, review its full lifecycle: entry/UI command → authorization → durable intent → external work → publication → read/event/client projection → cancellation/retry → crash/restart → policy change/deletion. Enumerate every writer and reference consumer. The independent implementation reviewer challenges both contract sufficiency and code conformance before behavioral tests under document 33. This documentation review does not waive that independent review or physical-device evidence.
