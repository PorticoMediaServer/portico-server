# Settings, Administration, and Operations Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


Contract ID: `PC-SETTINGS-OPERATIONS`
Revision: `portico.october.1`
Status: **approved**
Owners: Product, Security, Server Operations, Hosted Operations, Client Foundations
Imports: `PC-FOUNDATION` viewer scope, permissions, and resource lanes
Coordinates with: `PC-IDENTITY`, `PC-NETWORKING`, `PC-LIBRARY-SCANNING`, `PC-METADATA`, `PC-PLAYBACK`, `PC-LIVE-DVR`, `PC-ACCESSIBILITY-LANGUAGE`
Applies to: Hosted Services operator tooling, Portico Server, bundled and Hosted Web, Client Core, and every first-party client

## Scope and non-goals

This contract owns preference scope and resolution, the boundary between Settings and Server Console, administrative capabilities, save and validation behavior, persistent operational changes, jobs, activity and resource dashboards, logs and diagnostic export, retention, operational notifications, backup/restore, recovery mode, maintenance and update policy, providers, remote storage configuration, secrets, auditability, and constrained-client behavior.

It does not define provider-specific metadata selection, scan algorithms, playback planning, network route selection, deployment commands, infrastructure topology, or internal package ownership. Those contracts and architecture documents may add domain-specific constraints but MUST use the settings, operations, job, recovery, and audit semantics here.

Portico is pre-release. This revision defines one current behavior. No implementation may preserve a hidden legacy settings document, last-write-wins write path, fake operational status, or second administration model for compatibility.

There are no intentionally unresolved product decisions in this revision. Implementations MUST NOT replace an approved rule with client-specific behavior.

## 1. Canonical concepts and authority

### 1.1 Terms

- A **preference** is a durable user choice whose authority is an account, profile, library, device class, or exact installation.
- A **server setting** is owner-controlled desired state for one Portico Server.
- A **runtime override** is an explicit session-only choice. It is never silently promoted to durable state.
- **Effective state** is the resolved value currently governing behavior, with its source and any policy restriction.
- An **operation** is an owner- or system-initiated intent that may contain one or more durable attempts/jobs.
- A **job** is a durable, observable unit of background work.
- **Activity** is a current or recent factual event. Activity is not configuration and not necessarily audit evidence.
- An **operational alert** is an actionable, deduplicated condition with severity and lifecycle.
- **Server Console** is the owner-facing observation and intervention surface for one server.
- **Hosted Operator Console** is the separate service-operator surface for Hosted infrastructure. It is not a server-owner or consumer feature.

### 1.2 Authority table

| Fact or action | Authority | Ordinary editor |
| --- | --- | --- |
| Portico Account security, global Devices, contact and sign-in settings | Hosted Services | authenticated account owner through the Hosted account surface |
| Local Auth account security and sessions | selected Portico Server | that local account through the server surface |
| Profile viewing preferences | selected Portico Server for profile-server preferences; Hosted only owns global identity/profile security facts | current profile; primary profile only where `PC-IDENTITY` grants profile management |
| Exact-installation and device-class preferences | selected server plus client installation metadata | current interactive installation |
| Server settings, providers, secrets, maintenance, backups, restore, updates, remote access | selected Portico Server | server owner only |
| Library/source settings and library operations | selected Portico Server | Server Owner; an explicitly bounded `library.manage` delegate has only the non-source operation set below |
| Current activity, streams, jobs, resource state, logs, diagnostics and audit | selected Portico Server | server owner only, except a delegate's own initiated library operation summary |
| Hosted infrastructure configuration, deployment, service jobs and service audit | Hosted operations authority | authorized Portico service operators only |
| Session override | current authenticated session | current viewer within permission and capability policy |

There is exactly one broad Server Owner authority. Portico does not create a second all-powerful `administrator` role. A consumer with playback, download, DVR, feedback, metadata-edit, or library-management capabilities does not inherit server settings, People management, logs, other viewers' activity, backup, restore, provider-secret, update, or operator access.

The `library.manage` permission permits the delegate to view configuration and start/cancel/retry scan, metadata-refresh, match-review and analysis work only for explicitly granted libraries; edit that library's display/descriptive, scan-tier and metadata policy; and view the safe operation summaries produced by those actions. It does not permit creating/removing libraries, adding/editing/removing sources or paths, configuring remote storage/provider credentials, deleting media or Portico records, emptying retained missing titles, changing global schedules/resource/retention policy, or accessing logs and server-wide diagnostics. Those remain Server Owner actions.

An API key never qualifies as an interactive owner session for restore, account-security changes, secret export, server transfer, or another high-risk operation.

## 2. Preference ownership and resolution

### 2.1 Scope taxonomy

Every durable setting or preference key MUST declare exactly one authoritative scope and MAY declare narrower override scopes. Valid scopes are:

| Scope | Canonical key | Meaning and examples |
| --- | --- | --- |
| `account` | Hosted: `{authority, accountId}`; Local: `{authority, accountId, serverId}` | account security and account-wide service choices; not viewing history |
| `profile-server` | canonical server-bound `ViewerScope` | viewing, language, privacy, Home, music and playback intent for that profile on that server |
| `server` | `{serverId}` | friendly name, security, playback capacity, maintenance and retention policy |
| `library` | `{serverId, libraryId}` | sources, scan tier, metadata policy, DVR-library behavior |
| `profile-device-class` | canonical server-bound `ViewerScope` + `deviceClass` | navigation density, card size, device-family playback intent |
| `account-server-installation` | `{authority, accountId, serverId, installationId}` | remembered profile and trusted installation behavior |
| `installation` | `{installationId}` | strictly local UI/runtime preferences such as a diagnostic overlay; never authorization |
| `session` | `{sessionId}` | temporary playback/menu/diagnostic choice; never durable by implication |

`deviceClass` is one of the contract-advertised classes, initially `web`, `mobile`, or `television`. A client cannot invent a durable scope because it has a novel form factor.

Library configuration is not a profile preference. Server policy is not a profile preference. An installation identifier is not a credential or proof of possession.

### 2.2 Field registry

Every writable field MUST be present in a server-published, revisioned registry containing:

- stable field and group IDs;
- owner scope and permitted override scopes;
- type, enum/range/format and cross-field validation;
- canonical default and the authority that supplies it;
- save mode: `autosave`, `explicit-apply`, `command`, or `destructive-confirmation`;
- application mode: `hot`, `next-operation`, `scheduled`, or `restart-required`;
- permission and capability requirements;
- runtime consumer and operational-health source;
- secret classification and redaction behavior;
- audit class and retention class;
- contract revision.

A visible writable field without a named runtime consumer and outcome test is prohibited. `Stored`, `defaulted`, `configured`, `valid`, `effective`, `healthy`, and `active` are distinct states and MUST NOT be represented by one boolean.

### 2.3 Resolution law

Resolution is field-specific and deterministic. Implementations MUST NOT apply one generic object merge to unrelated scopes. For a field that permits overrides, the usual order is:

1. security, authorization, account membership, and server resource policy constrain what is allowed;
2. an explicit session override governs only that session;
3. an exact-installation override, when the registry permits it;
4. a profile-device-class preference, when permitted;
5. a profile-server preference;
6. a library or server default named by the field registry;
7. the canonical product default.

The response MUST preserve `requestedValue`, `effectiveValue`, `effectiveSource`, `policyLimit`, and `restrictionReason` when they differ. A policy limit may constrain execution; it MUST NOT overwrite or silently "correct" the user's saved choice. If a viewer explicitly chooses a poor but supported playback quality, the preference remains their choice. Only impossibility, authorization, safety, or an explicit automatic mode may change the executed outcome, as defined by `PC-PLAYBACK`.

Unknown fields are rejected on writes and ignored only in a read projection whose version explicitly permits forward-compatible observation. Unknown enum members never fall back to a seemingly similar choice.

### 2.4 Session and local state

A session override expires with the session or explicit reset. A client MUST label **Use for this session** separately from **Make this my default** when both exist.

Strictly local installation preferences MAY be saved in protected client storage when no server round trip is appropriate. They MUST be namespaced by installation and, if viewer-sensitive, the complete viewer scope. Signing out, switching profile/server, or resetting the app fences and clears the applicable state according to `PC-IDENTITY`.

## 3. Settings versus Server Console

### 3.1 Settings

Settings answers **what should Portico do in the future?** It contains durable desired state, schedules, policies, credentials, library/source definitions, account/profile choices, and explicit maintenance configuration.

Settings MUST show:

- the authoritative scope being edited;
- current saved value and unsaved draft independently;
- capability/permission state;
- when and how a change takes effect;
- validation, conflicts, partial availability, and recovery action;
- active versus pending generation for staged or restart-required changes.

### 3.2 Server Console

Server Console answers **what is happening now, what happened, and what needs attention?** It contains current health, streams, resource use, connectivity, jobs, scans, transcodes, DVR work, alerts, logs, diagnostic checks, release state, and audit views.

Console intervention commands such as **Cancel job**, **Retry**, **Stop playback**, **Acknowledge alert**, or **Export diagnostics** are operations, not settings. They MUST NOT imply a persistent change unless the confirmation explicitly says so.

The Console MUST NOT become a second editor for the same server settings. It MAY link to the owning Settings field and show the effective value or policy that explains a condition.

### 3.3 Hosted Operator Console

Hosted Operator Console is isolated from Server Console. It MAY expose aggregate Hosted service health, certificate/DNS jobs, queues, backup verification, deployment versions, and de-identified failure metrics. It MUST NOT expose library names, media titles, viewer history, server-local settings, provider queries, playback URLs, or server secrets.

Operator access requires strong authentication, least privilege, short sessions, and a separate tamper-evident audit. A Portico Account owner, Server Owner, or client app never receives operator capability merely because they own a server.

## 4. Capability and permission gating

Every settings/operations response MUST expose facts sufficient for a client to distinguish:

- `unsupported` — this server/client contract cannot perform it;
- `unavailable` — a required runtime dependency or platform facility is absent;
- `unconfigured` — required owner input is missing;
- `invalid` — saved/candidate input fails validation;
- `degraded` — usable with a known impairment;
- `available` — action can start;
- `busy` — conflicting work currently prevents it;
- `denied` — the viewer lacks authority;
- `offline` — the owning authority cannot be reached;
- `recovery-only` — normal operation is fenced.

The server is the authority for permission and capability enforcement. Clients use advertised state to explain and suppress impossible actions, but hiding a control is never authorization.

Unavailable, denied, and unsupported are not interchangeable. A missing FFmpeg build, an unclaimed server, a read-only account, and an unimplemented feature require different messages and recovery actions. A client MUST NOT show a dead control, fake success, or `Coming soon` as if the current server accepted the setting.

## 5. Save, validation, revision, and application behavior

### 5.1 Save modes

Low-risk, single-field personal preferences MAY autosave. Autosave MUST show pending state, commit acknowledgement, and failure; on failure it restores the last authoritative value or keeps an explicitly marked unsynced draft. It never silently loses the user's choice.

The following require explicit **Save** or **Apply**:

- server, library, provider, networking, retention, schedule, update, and remote-storage settings;
- a change spanning multiple dependent fields;
- secrets or credential references;
- a change that starts work, allocates storage, changes exposure, or requires restart;
- any candidate configuration that must be tested before activation.

Commands use action language such as **Run now**, **Test connection**, or **Create backup**. Destructive work uses the confirmation rules in §8.

### 5.2 Drafts and navigation

An explicit-save surface MUST keep a local draft separate from the authoritative document. Navigation, Back, profile/server switching, sign-out, reload, or app termination while dirty MUST either preserve a resumable same-scope draft or ask the user to discard/stay. A draft never crosses authority, account, server, profile, library, or installation scope.

### 5.3 Validation

Client validation is immediate guidance; server/Hosted validation is authoritative. The authority MUST validate:

- JSON type, enum, numeric range and step, length, encoding, and URL/path grammar;
- unknown and read-only fields;
- cross-field dependencies and mutually exclusive choices;
- capability, permission, ownership, quota, and runtime prerequisites;
- source/provider reachability only when activation requires it;
- dangerous transitions and required confirmation/reauthentication.

Invalid fields return stable field paths and product error codes while preserving the draft. Silent clamping, coercion, trimming that changes meaning, or partial application is prohibited. Canonical whitespace normalization MAY occur when it is lossless and returned to the client.

### 5.4 Concurrency and revisions

Every durable settings/preference document has a monotonically increasing revision and digest. A mutation carries `expectedRevision` and an idempotency key. The authority atomically compares, validates, commits, audits, and returns the complete affected document.

A stale write returns conflict plus the current revision; it never overwrites the newer value. Clients reload and reapply only the user's still-intended field changes. They MUST NOT blindly resubmit an old whole document. Multiple editors for one document serialize through one coordinator or merge into one patch.

A lost success response is reconciled with the idempotency receipt or a fresh read. Repeating a request with the same key and same body returns the same committed outcome. Reusing a key with different intent is rejected.

### 5.5 Application modes

| Mode | Required behavior |
| --- | --- |
| `hot` | Commit and apply atomically, or roll back the commit. Response identifies the active revision. |
| `next-operation` | Commit now; new operations use the new revision while already-admitted work keeps its pinned revision. |
| `scheduled` | Commit desired state and create/show the operation that will activate it. |
| `restart-required` | Commit a pending generation, retain the active generation, and show exact restart impact. |

`restartRequired=true` without fields, active/pending revisions, or instructions is insufficient. Restart-required changes MUST survive reload and reboot. Restart does not imply successful activation: startup validates the pending generation, publishes it only after health checks, and otherwise retains or rolls back to the last known-good generation with an alert.

## 6. Candidate configuration and secrets

Provider, remote-storage, network, certificate, and other external integrations use a candidate-generation transaction:

`editing → validating-locally → testing-candidate → ready-to-activate → activating → active | failed`

The active generation remains usable until the candidate passes validation and activates atomically. A failed candidate cannot corrupt, clear, or partially replace the active generation. Late work from an older generation cannot publish into a newer one.

Secrets are write-only values represented as `absent`, `present`, `replace-pending`, or `clear-pending`. APIs and UIs never return placeholder text that could be resubmitted as a real secret. Clear and replace are explicit operations. Secret material:

- is accepted only over an authenticated protected channel;
- is stored as plain server state protected by filesystem permissions; native authentication storage follows its platform facility;
- is excluded from query strings, client persistence, logs, traces, analytics, audit diffs, error bodies, diagnostics, screenshots, and clipboard actions;
- is never sent to a third party until the user starts a bounded test or activation for that provider;
- is revoked or rendered unreachable when its configuration is removed.

Provider and remote-storage tests are bounded, cancellable, rate-limited, and report capability/reachability without listing private remote contents by default. Arbitrary shell commands, command-line fragments, environment-variable editors, and unrestricted filesystem browsing are prohibited product controls.

## 7. Jobs, operations, and activity

### 7.1 Durable model

Long-running, retryable, resource-intensive, externally ambiguous, or restart-sensitive work MUST be a durable operation/job before execution. This includes scans, metadata refresh, analysis, optimization, backups, restores, storage cleanup, provider imports, guide refresh, recording maintenance, certificate work, and updates.

An operation contains:

- stable operation ID, kind, scope/resource, actor/trigger, creation time and idempotency key;
- policy/settings/source revisions pinned at admission;
- current state, phase, factual progress and updated time;
- attempt count, next attempt, priority and resource lane;
- cancellation/retry eligibility and reason;
- safe result reference or stable error code;
- child jobs and aggregate outcome when work fans out;
- retention expiry.

### 7.2 State machine

The normative job states are serialized in `state-machines.yaml`. Key guarantees are:

- `cancellation-requested` is not `cancelled`; cancellation completes only after the worker acknowledges and fenced work can no longer publish;
- progress is determinate only when current/total are factual; otherwise the UI shows indeterminate phase, never invented percentages;
- automatic retry creates a new attempt under the same user operation; an explicit Retry of terminal work may create a linked new operation and retains the predecessor. Either representation must expose one coherent history and must not reopen a cancelled terminal intent implicitly;
- an interrupted lease returns to reconciliation before retry;
- duplicate active work for the same exclusive key returns the existing operation or an explicit conflict;
- terminal success means all required publication/cleanup committed, not merely that a process exited zero.

Jobs use the single PC-FOUNDATION lane order and the durable mechanics in architecture.md. Background pressure changes execution scheduling; it never adds predictive playback admission vetoes. Storage checks may constrain background artifacts and owner-configured DVR reserves. Playback session caps and transcoding policy belong to document 32.

Consumer jobs expose only their own safe operation projection, never Server Console or another viewer's operational detail.

## 8. Destructive and high-risk actions

Destructive actions include server/account/profile deletion, library/source removal, media deletion, missing-title cleanup that makes recovery harder, credential/API-key revocation, trust reset, backup deletion, restore, storage relocation, server transfer, shutdown, and update/rollback when data compatibility may change.

Before execution, the authority MUST resolve the exact target and impact. The confirmation shows:

- target name and authority/scope;
- data and active work affected;
- whether media files, Portico records, generated data, or credentials are removed;
- recoverability and retention window, if any;
- prerequisite backup or safety copy;
- conflicts such as active streams, recordings, scans, or restore;
- the exact confirmation and recent-authentication requirement.

High-risk operations require the interactive principal that owns the affected authority, recent reauthentication, an operation-bound single-use proof, and a typed or exact-target confirmation. A generic `Are you sure?` is insufficient. CSRF and idempotency protections apply.

The proof authority is action-specific:

- Hosted account deletion, sign-in-method changes, global Device revocation and Hosted-profile deletion require the Portico Account owner or primary-profile authority defined by `PC-IDENTITY`;
- Local account/profile deletion and credential changes require that Local authority's authenticated account owner or primary-profile authority as defined by `PC-IDENTITY`;
- server, library/source, media-file, API-key, backup/restore, update, shutdown, transfer and server-secret operations require the Server Owner; a `library.manage` delegate may perform only the exact non-source, non-destructive operation set in §1.2;
- Hosted service operations require a separately authorized Hosted operator.

Server ownership never grants control over another account's Hosted profile, and profile primacy never grants server administration.

An accepted destructive operation becomes durable before side effects. Partial failure reports completed, pending, preserved, and recovery-required outcomes. Portico never claims success while unknown references or unverified cleanup remain.

## 9. Status, streams, resources, and capacity

Server Console MUST independently load and timestamp these panels so one failure does not blank the rest:

- service/database/readiness and dependency health;
- active streams and sessions;
- current/queued jobs, scans, transcodes and recordings;
- CPU, memory, storage, temporary/transcode space and database size;
- disk/source I/O pressure and remote-source health;
- network/remote-access, route and certificate state;
- DVR tuner and recording capacity;
- alerts, release/update state, backup health and last restore drill where available.

Every snapshot includes `generatedAt`, freshness/expiry, and partial-failure details. Cached data is labelled stale. Zero is never substituted for unavailable measurement.

Each active stream shows only owner-authorized facts: viewer display identity, client/device class, item display title, progress, local/remote route class, requested quality, actual delivery mode, per-stream actions, bandwidth, transcoder/hardware use, and reason summary. **Stop playback** is owner-only, requires target confirmation, records an audit event, and may include a bounded user-visible reason. It is never hidden behind a premium entitlement.

Historical aggregate views obey retention and privacy policy. They do not become social activity or expose contact email, IP addresses, precise route data, or private profile state.

## 10. Logs, diagnostics, and support export

Server runtime, client-uploaded, audit, job, playback/transcode, and Hosted operator records are separate lanes with separate permissions, capacity and retention. Untrusted client uploads cannot evict server evidence.

Structured records use stable time, severity, component, event code, correlation/request/operation ID, safe fields, and redaction classification. Redaction occurs before persistence and again before display/export. By default, remove or tokenize:

- credentials, cookies, authorization headers, setup/claim codes and secret settings;
- account/contact identifiers, installation IDs and IP addresses;
- private routes, hostnames, filesystem paths and remote locators;
- media titles/filenames where they are unnecessary to the investigation;
- provider queries, signed URLs, subtitle text and user-entered message bodies.

Debug logging is owner-enabled, time-bounded, component-scoped, states its storage/privacy cost, and automatically returns to the normal level. Verbose or trace capture is never a permanent default.

Console filtering and copying apply to the visible, already-redacted projection. Support export requires an owner-selected time window and components, produces a manifest of included/excluded classes and redaction policy, enforces size limits, and expires the private artifact. Raw databases, environment files, secret stores, media, and unrestricted data directories are never included. An owner can inspect the manifest before download or upload. Portico does not upload diagnostics automatically.

## 11. Retention, alerts, and notifications

### 11.1 Retention

Each record class has an explicit policy. `0` MUST have one documented meaning per field (`disabled` or `retain until manual deletion`); it never ambiguously means both.

Initial default maximums are:

| Record class | Default |
| --- | --- |
| server diagnostic/log history | 30 days, plus bounded rotating size |
| accepted client diagnostic uploads | 30 days, separately bounded |
| job and operation history | 30 days, terminal summaries may outlive detailed attempts only when declared |
| security/administrative audit | 90 days minimum by default |
| operational notifications | 180 days maximum unless acknowledged/pruned sooner |
| exact device IP evidence | 30 days maximum |
| expired auth/setup request evidence | 14 days maximum |

Security law may require a de-identified minimum audit record after related personal data is erased. Retention changes apply prospectively and schedule prompt pruning when shortened. A failed prune is an operational alert; it does not silently redefine policy.

### 11.2 Alerts and notifications

Operational alerts have stable IDs, severity (`info`, `warning`, `critical`), condition code, scope, first/last seen, occurrence count, status (`open`, `acknowledged`, `resolved`), impact, and next action. Repeated identical conditions deduplicate rather than spam.

Owner notifications summarize important alerts such as backup failure, restore/recovery state, low storage, database degradation, repeated job failure, remote-access/certificate expiry risk, recording failure, update action required, or a secret/provider becoming invalid. Notification delivery is an acceleration; the Console remains authoritative.

Consumers receive only notifications belonging to their own viewer state. Server operational details, other viewers' activity, paths, IPs, logs, and secrets never enter consumer notifications or Hosted payloads. Consumer apps refresh this server-owned inbox when open; Hosted does not deliver background device notifications.

## 12. Backup, restore, and recovery mode

### 12.1 Backup

Portico provides online, application-consistent server backups. A backup contains the server database and the Portico-owned configuration/evidence required to restore accounts, profiles, permissions, libraries, metadata decisions, settings, progress, and other canonical state. It excludes original media, ephemeral transcodes/cache/temp files, ordinary logs, active session credentials, and unbounded generated artifacts unless an explicitly selected component says otherwise.

Every backup has an opaque ID, creation operation, Portico/schema version, component manifest, byte size, checksum, consistency result, verification time, required plain configuration components, retention status, and `restoreReady` result. The manifest classifies Server-authoritative Local identity/configuration separately from cached Hosted membership/profile/policy projections. A file's presence is not proof that it is restore-ready.

Scheduled and manual backups use the same verified mechanism. Backup creation reserves space, uses the database backup API/snapshot protocol, writes privately, fsyncs as appropriate, validates the result, and publishes atomically. Backups on the same disk do not satisfy the recommendation for an independent copy. Portico MUST make that limitation visible.

### 12.2 Restore

Restore is an owner-confirmed supervised operation. Validation/staging is detached and pollable with phase/byte progress. Validate the source, exact current pre-release schema/baseline, integrity and space before installation. Retain safety copies and durable staged/result evidence; install at restart and roll back on validated failure. Failed session scrubbing must be retried safely, without corrupting the rollback copy.

Active sign-ins are excluded from backups and invalidated after restore. API keys/tokens are the implemented exception: keys in the backup return, including keys revoked later. The owner must review them after restore. This exception is reported by api-platform; it is not a fresh-credential guarantee.

No older baseline/database compatibility is promised. The earlier status-capability, universal external security-incarnation and all-secret-component design is not certified as implemented. Startup recovery is minimal; the full recovery console remains unfinished. Hosted authority/signing revocation must be reconciled before relying on restored projections. Follow the current backup specification and source reports rather than the retired state-machines wire shapes.

### 12.3 Recovery mode

Recovery mode is fail-closed. Normal API mutations, background work, playback and ordinary sign-in are unavailable. A minimal local/verified-owner surface shows operation ID, safe phase/error, last known-good backup/safety-copy facts, diagnostics export, retry-rollback/reopen actions that are actually safe, and documented manual recovery guidance.

Recovery mode never auto-deletes the failed database, chooses an older backup by filename, or loops restore/restart. Leaving recovery requires a verified healthy database and explicit audited transition.

## 13. Maintenance and updates

Backup timing uses the configured local window. General background work is paced continuously with Gentle/Normal/Fast rather than restricted to that window. An owner may explicitly pause background work for one hour or resume it. Scheduler hints coalesce durable work; job state is the authority. Remaining worker pause boundaries are implementation follow-ups, not proof that every worker stops immediately.

Disabling a schedule stops new scheduled admission. It does not falsely label an already-running job cancelled. The UI links to that job and offers cancellation only when safe.

The default update policy is notify and manual install. A setting for channel, window, automatic download, or install is visible only when the installed distribution has a named updater that consumes it. Portico MUST NOT render an updater setting that is merely stored.

Before installation, Portico checks package authenticity, compatibility, disk space, active streams/recordings/jobs, backup health, migration implications, and rollback compatibility. Automatic installation, if later approved and supported, is opt-in, never interrupts active playback/recording, and requires a fresh verified backup. Database downgrade is never implied by reinstalling an older executable.

Release state distinguishes `current`, `available`, `downloaded`, `ready`, `installing`, `restart-required`, `verifying`, `failed`, `rollback-available`, and `manual-action-required`. Installation method and platform-specific instructions remain truthful.

## 14. Platform and client boundaries

### 14.1 Web and Server Console only

The bundled/Hosted Web administration experience is the canonical first-release surface for:

- server identity, network and remote-access configuration;
- libraries, sources, provider policy, remote storage and secrets;
- server playback/transcode capacity and generated-media policy;
- Live TV/DVR source and storage administration;
- People/access, Devices, API keys and server-wide retention;
- schedules, jobs, storage cleanup, backups, restore, recovery and updates;
- full stream/resource dashboards, alerts, logs, diagnostics export and audit.

Hosted Web performs these operations directly against the verified selected server; Hosted Services does not proxy server administration or receive server-local settings/media evidence.

### 14.2 Consumer clients

Mobile, television, Cast and other consumer clients MUST provide applicable personal/profile playback, language, subtitle, accessibility, privacy, download and device preferences. They MAY show a compact owner-authorized server-health/alert summary when product-approved, but MUST deep-link or device-code hand off to Web for the administrative controls in §14.1.

Consumer clients MUST NOT duplicate provider credential entry, filesystem/remote-source setup, retention editors, raw logs, restore, server updates, arbitrary cleanup, user/API-key administration, or other high-risk Console operations.

### 14.3 Limited-input and constrained clients

Television and limited-input clients optimize for focus, remote control, large targets and short decisions. They never require entry of provider secrets, paths, shell-like values, typed destructive confirmations, or long diagnostic filters. A QR/device handoff includes only an opaque, expiring, purpose-bound continuation; the Web surface reauthenticates and revalidates the action.

SSE, long polling, push and periodic refresh are transport adaptations for the same settings/operation revisions. A constrained client does not invent a second job state machine or treat a missed event as final state; it refetches the authoritative snapshot.

## 15. Offline, partial, stale, and failure behavior

- Personal cached preferences MAY remain usable offline with their last-authoritative revision. An edit is either an explicitly unsynced same-scope draft or unavailable; it is never shown as saved.
- Server/admin mutations, commands, destructive work, secret changes, tests, and restore are never queued blindly while offline.
- A settings workspace loads groups independently. One malformed/unavailable group is quarantined and explained without making healthy groups disappear.
- A Console snapshot keeps healthy panels usable and identifies failed/stale panels with their own retry.
- Hosted outage does not disable direct local Server Settings for a current server-native owner session. It does disable Hosted account/operator operations that require Hosted authority.
- Server outage does not sign out the Portico Account. Account settings and server selection remain available; cached server settings are read-only and labelled stale.
- Authorization revocation fences drafts, events, cached diagnostics and pending mutations for the revoked scope immediately.
- Unknown commit outcome triggers receipt/read reconciliation. It never becomes a success toast or automatic duplicate operation.

## 16. Auditability

Every administrative, security-relevant, destructive, secret-state, job-control, stream-stop, backup/restore, maintenance/update, provider/source, retention and operator action records a tamper-evident audit event in the owning authority.

An audit event contains stable event ID/time, authority, actor type and opaque actor ID, authenticated session class, action, target type/opaque ID, request/correlation/operation ID, prior and resulting revision, result, safe reason, and redacted changed-field names. Secret values, contact email, raw paths/routes, media titles, provider query values, and free-form diagnostic bodies are excluded.

Audit writes for a successful sensitive mutation commit in the same transaction or durable outbox boundary. Failure to secure the audit record fails the sensitive mutation. Read-only consumer preference changes need revision history but do not require a permanent security audit event unless their privacy impact or policy registry marks them auditable.

Audit access and export are themselves audited. Owners cannot alter audit rows. Hosted operators cannot read server-local audit; Server Owners cannot read Hosted operator audit.

## 17. Security and privacy invariants

- Reauthenticate and bind proofs to exact high-risk purpose, actor, target, expiry and one-time use.
- Never trust client-provided role, capability, effective value, job terminal state, path safety, backup readiness or redaction claims.
- Prevent path traversal, symlink escape, SSRF, command injection, archive bombs and credential leakage in provider/storage/backup flows.
- Use bounded request bodies, result pages, log fields, exports, uploads, job metadata and retention.
- Keep public liveness/readiness endpoints free of operational, identity, library, route and dependency detail.
- Do not include private operational state in analytics. Product telemetry, if approved separately, uses aggregate/de-identified events and cannot replace owner-visible audit.
- A diagnostic mode, support export, backup, or operator action never weakens media authorization or the Hosted no-relay boundary.

## 18. Conformance and governance

The canonical envelope validates against `settings-operation.schema.json`. Ordinary and secret field projections are disjoint schema variants; conformance MUST also prove that each projected classification matches the authoritative field registry so a secret cannot be relabelled as ordinary to expose its value. Implementations run `conformance.json` and the state transitions in `state-machines.yaml`. `plex-jellyfin-precedent-review.md` is informative evidence, not authority. `exceptions.yaml` is the exception registry and is empty in this revision.

Minimum applicability matrix:

| Rule family | Server | Hosted | Web | Client Core | Apple/Android | TV/Roku | Cast |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Scope/resolution/revisions | required | account/operator scope | required | required | applicable personal scopes | applicable personal scopes | session scope |
| Server Settings/Console | required | direct route only | required | contracts/types | handoff/status only | handoff only | not applicable |
| Jobs/activity/resources | required | Hosted jobs separately | required | required projections | own-state only | own-state only | receiver state only |
| Secrets/providers/storage | required | Hosted-owned secrets only | server administration | redacted types only | prohibited | prohibited | prohibited |
| Backup/restore/recovery | required | Hosted runbook separately | required | status types only | prohibited | prohibited | prohibited |
| Logs/diagnostics/audit | required | operator boundary | required | redaction/transport | own diagnostics only | own diagnostics only | own diagnostics only |

Before approval, adversarial evidence MUST cover abandonment/resumption, concurrent writers, lost success responses, invalid and partially corrupt documents, missing runtime dependencies, unsupported clients, offline Hosted/server states, stale events, duplicate jobs, cancel races, resource contention with playback, secret leakage, low disk, backup corruption, restore interruption at every phase, failed rollback, operator/server boundary violations, and explicit user choices that policy constrains but does not erase.

Approved deviations MUST use the exact nearest-code-comment format from `../governance.md`:

```text
PORTICO-CONTRACT-EXCEPTION
Contract: <contract-id>#<section-id>
Scope: <platform, surface, or capability>
Decision: <the exact deviation>
Reason: <why the canonical rule cannot or should not apply>
Approved-by: <name or recorded decision reference>
Review-when: <expiry date, capability change, or other objective trigger>
Evidence: <test or fixture that prevents accidental expansion>
```

For this contract, `<contract-id>` MUST be `PC-SETTINGS-OPERATIONS`. The same exception MUST appear in `exceptions.yaml`. An unapproved branch, UI omission, or implementation gap is drift, not an exception.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.

## October operations and API settings

Keep existing useful settings wired. Prefer named profiles and plain numeric limits. The registry carries requested/effective values, bounds, defaults and application mode. Original is the viewer quality default everywhere; a session may override its starting value. Player Automatic/Native/mpv and stereo/passthrough/HDR overrides feed the one capability document. Home networks are the single locality setting; mandatory HTTPS and WAN-as-LAN are removed.

Scoped owner keys and personal tokens are active. Webhooks and MCP run locally. Creation secrets are shown once; credential management is session-only. These features have web screens and shared transports, but broad native administration remains web-first. No MCP toggle is required.

Optional cache/backup paths are startup variables. A restart parent supports owner Restart and asynchronous restore. Windows uses SCM/LocalService by default; macOS uses a system `_portico` daemon and package. Manual backups survive scheduled retention. New databases use bounded incremental reclamation; pre-migration copies stay beside the database. Actual package/service lifecycle and signed release execution are unverified.

Client reports have bounded redacted upload and owner read/download/delete. Resource/status routes publish known usage and report unknown measurements honestly. Later console passes render these, Restart, restore progress, version deletion and exact needs-attention totals. Native report strings/UI and physical acceptance remain open. A minimal startup recovery page is implemented; full recovery console is not.

State and backups are plain at rest. No encrypted-secret backup requirement applies. No consumer push or email is sent under the release decision; residual Hosted mail flows require reconciliation. Quiet HTTP guidance may be shown; unsafe-choice detection/permission enforcement flows are not added.

## October scope and evidence

Later console passes implement many initial screen handoffs. Native broad owner administration remains web-first. Physical OS packaging, full recovery console, release signing and residual pause boundaries remain open.

Sources: [ops-packaging](../../../_program/reports/ops-packaging.md), [limits-settings](../../../_program/reports/limits-settings.md), [server-followups](../../../_program/reports/server-followups.md), [console](../../../_program/reports/console.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
