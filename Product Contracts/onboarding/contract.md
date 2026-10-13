# Account, Server, and Device Onboarding Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


Contract ID: `PC-ONBOARDING`
Revision: `portico.october.1`
Status: **approved**
Owners: Product, Hosted Identity, Server Setup, Client Foundations
Imports: `PC-FOUNDATION` viewer scope and permissions; `PC-GOVERNANCE` lifecycle/exception law
Coordinates with: `PC-IDENTITY`, `PC-NETWORKING`, `PC-VISUAL`, `PC-ACCESSIBILITY-LANGUAGE`
Applies to: Hosted Services, Portico Server, Web, Client Core, and every first-party client

## 1. Purpose

Onboarding must remain correct when a user changes devices, moves between `localhost` and `web.getportico.tv`, refreshes, closes a page, loses a response after a successful commit, repeats a click, or returns later. A UI step is not the authority for whether a durable operation succeeded.

This contract covers:

- Portico Account registration and SSO completion;
- first server initialization and sign-in-mode choice;
- server claiming/attachment to a Portico Account;
- TV and limited-input device authorization;
- Nearby TV approval and local Quick Connect;
- continuation, abandonment, recovery, and idempotency.

It does not preserve pre-release six-character codes, generated SSO usernames, `api.getportico.tv` user links, or legacy partial-account records.

Non-goals are organization provisioning, enterprise device management, bulk server enrollment, encrypted disc import, and preserving any unreleased setup protocol. There are no unresolved onboarding decisions in this revision; new device families must select one of the existing authority/secret models or change this contract.

## 2. Universal rules

1. **Durable truth wins.** Every continuation begins by reconciling authoritative state before offering a new mutation.
2. **Create once, resume many.** A client persists its opaque operation/session ID and high-entropy recovery secret before starting a mutation. Repeating the same intent returns the committed result or current state; it does not create a duplicate account, Device, server, membership, credential family, or certificate order.
3. **No destructive ambiguity.** Timeout, disconnect, lost response, tab close, app suspension, `408`, `429`, or `5xx` means `unknown/retryable`, never `failed and rolled back` unless the authority proves rollback.
4. **No half-published success.** UI completion follows durable credential/result persistence and final identity verification. A committed remote result remains recoverable through a bounded receipt until acknowledged.
5. **Independent authorities remain independent.** Portico Account authentication can succeed with zero servers. Server setup can complete in direct mode without Hosted Services. Device account authorization does not imply server access.
6. **One visible code.** QR, manual entry, Nearby discovery, and Cast approval refer to the same current protocol-v1 `XXXX-XXXX` setup session where their security requirements permit it. A separate high-entropy secret authenticates the requesting device.
7. **Hosted links are web links.** User instructions, QR destinations, callbacks, and continuation pages use `https://web.getportico.tv/...`. `api.getportico.tv` and `direct.getportico.tv` are never shown as destinations.

## 3. Account onboarding

### 3.1 Password registration

Username/password and supported provider onboarding retain exact authority and credential-publication checks. The October no-email decision supersedes mandatory contact verification; existing Hosted mail-dependent onboarding still needs reconciliation. The rules come from `../identity/contract.md`. A repeated request with the same idempotency key returns the same account/result or a deterministic conflict.

States:

`start → editing → validating → committing → credentials_persisting → signed_in`

Possible side states are `validation_error`, `retryable_failure`, `expired`, and `cancelled`. Validation errors preserve non-secret input. Passwords and email tokens are never written to ordinary draft storage.

### 3.2 SSO onboarding

Existing linked identities skip onboarding. New Google or Apple identities must choose a username. Provider verification creates a single-use, ten-minute onboarding transaction containing server-sealed claims. Callback URLs and deep links carry only an opaque onboarding token.

States:

`provider_start → provider_authorized → identity_verified → onboarding_required → username_selection → committing → credentials_persisting → signed_in`

- Refreshing or reopening the canonical onboarding URL resumes while the token is valid.
- Username collision returns a replacement onboarding token and preserves provider proof.
- Expiry gives a clear **Start sign-in again** action.
- Provider cancellation returns to sign-in without an error banner.
- A new account needs its chosen username and valid sign-in proof; mail delivery is not a release prerequisite. Existing Hosted mail paths remain unfinished reconciliation.
- Native clients finish through the native account container; they do not display an app-authored imitation of the provider page.

## 4. First server initialization

An uninitialized Portico Server creates its durable server identity and key once, then presents exactly two sign-in choices:

1. **Use a Portico Account** (recommended): initialize locally, authenticate at canonical Hosted web, claim/attach the exact server, install the returned credential, and enter the owner session.
2. **Sign in directly to a server** (advanced): create the server-local owner account and enter the owner session with no Hosted dependency.

The server name draft is local. Choosing Hosted must not create multiple server identities or records when the user retries. Choosing direct mode must not contact Hosted Services.

### 4.1 Server-setup state machine

`uninitialized → draft → initialization_committing → local_identity_ready → auth_mode_selected`

Hosted branch:

`auth_mode_selected → claim_creating → waiting_for_web → web_account_authenticated → claim_approving → claim_committed → result_installing → result_acknowledged → recovery_owner_credential_confirmed → owner_session_verified → ready`

Direct branch:

`auth_mode_selected → local_owner_editing → local_owner_committing → owner_session_persisting → owner_session_verified → ready`

`claim_committed` is the point of no return for the Hosted ownership association. A local page that loses the response must query/redeem the existing claim result; it must not display the initial choice again or make a second server record. A claim result remains recoverable through its receipt until the server durably installs and acknowledges it.

### 4.2 Moving between localhost and Hosted web

- Local setup owns the server draft, persistent server identity, claim ID, result receipt, and final installed credential.
- Hosted web owns account authentication and approval of the displayed server identity.
- The browser may navigate away or switch origins at any point. Returning to local setup loads and reconciles the persisted local state.
- Returning to the Hosted approval URL shows current claim state: pending, approved, completed, cancelled, expired, or already used. It never creates a replacement claim implicitly.
- If Hosted reports success but local install has not completed, local setup continues result recovery automatically.
- If local install succeeded but the browser missed the response, `/api/auth/me` and server initialization state take the user to the product, not back to setup.
- Expired pre-commit claims can be replaced explicitly while reusing the same server identity. Cancelled claims cannot later complete.

### 4.3 Setup failure presentation

Automatic/background coordination retries without a **Try again** button, using the networking contract. The UI distinguishes:

- device offline: `This device isn't connected to the internet. Portico will continue when the connection returns.`
- Hosted unavailable/unknown: `Portico Account services are unavailable. Portico will keep trying automatically.`
- claim expired: `This setup link expired. Create a new link to continue.`
- identity mismatch: terminal security error with restart guidance;
- result installed but UI stale: reconcile and continue without an error.

Raw transport strings such as `Load failed`, `Failed to fetch`, `502 Bad Gateway`, and `Portico transport failed` are prohibited.

### 4.4 Local recovery owner

Every server initialized with a Portico Account creates a separate Local recovery-owner authority before setup becomes `ready`. It is not another Portico Account and is not used during ordinary Hosted sign-in.

- Setup generates a high-entropy one-time recovery code or accepts a strong recovery password, confirms durable hashed storage, and requires the owner to save/confirm it before completion.
- Recovery/sign-in locality uses the owner home-network list, never an independent private-address test. Enabling remote recovery requires a separate warned owner setting and does not weaken rate limits or TLS/identity checks.
- Successful recovery opens a clearly labeled **This Server · Recovery Owner** session with `server.manage`; it cannot manage the Hosted account, other servers, Hosted profiles, invitations, or Hosted memberships.
- Recovery can repair networking/certificates, export diagnostics/backups, restore service, transfer/unclaim the server when Hosted returns, or deliberately convert the server to Local authority through a separately confirmed destructive operation.
- Hosted account deletion may unclaim into this authority only after the owner proves both fresh Hosted account control and the Local recovery credential.
- Losing this credential does not create a Hosted backdoor. A future physical-console reset requires a separately contracted proof and data-preservation policy.

The credential is server-local, write-only after creation, governed by the plain-state backup and session-scrubbing policy, and never sent to Hosted Services.

## 5. Protocol-v1 human setup code

All first-release device/TV/Quick Connect human codes use:

- format `XXXX-XXXX`;
- alphabet `ABCDEFGHJKMNPQRSTUVWXYZ23456789` (31 symbols; excludes I, L, O, 0, 1);
- cryptographically secure unbiased sampling;
- active uniqueness across the relevant authority's compatible code families;
- case-insensitive input with whitespace/hyphen normalization, canonical uppercase display;
- a separate 256-bit device/poll secret that never appears in the QR verification URL;
- keyed, domain-separated hashes at rest; no raw code or secret in logs.

The normal validity period is **10 minutes**. The response supplies authoritative `expiresAt` and a polling interval of at least five seconds. Clients derive countdowns from `expiresAt`, tolerate clock skew, and never extend validity locally.

### 5.1 Rotation and display continuity

Code refresh is proactive and atomic:

1. Keep the current valid code, QR, and instructions visible.
2. Begin replacement in the background near expiry or after a terminal expiry response.
3. Persist the replacement session and secret.
4. Swap code and QR in one render transaction only after the replacement is complete.
5. Cancel the superseded pending session best-effort after the swap.

A replacement failure does not blank the old code, remove its QR, show a spinner instead of it, or emit a red error while it remains valid. Continue retrying with bounded backoff. Once the old code is truly expired and no replacement exists, show a stable `Creating a new activation code…` state and explain whether the device is offline; never flash an expired code as valid.

QR and visible code must always identify the same session. A QR payload contains the canonical verification URI or opaque setup ticket, not account credentials, raw poll secrets, server credentials, contact email, or private network metadata.

## 6. Hosted device authorization

This flow signs a limited-input device into a Portico Account and succeeds with zero servers.

Device states:

`creating → pending → approved → redeeming → account_credentials_persisting → account_authenticated`

Terminal states are `denied`, `expired`, `cancelled`, and `redeemed`. `retryable_failure` is not terminal.

Approver states:

`code_entry_or_qr → authenticated_account → device_preview → explicit_approve_or_deny → decision_committed`

Requirements:

- Preview reveals only device label, platform, app version, and expiry.
- Approval is a separate confirmation and binds the current account and requesting session.
- Poll/redeem authenticate with the high-entropy device secret in a header or protected body, never the human code alone.
- Redemption atomically transitions state and inserts or recovers exactly one Hosted Device and one account credential family.
- The exact family may be recovered for five minutes after a committed lost response, but no second family is minted.
- Account credential delivery does not inspect server memberships, routes, heartbeat, or remote-access state.
- After account authentication, the client enters zero/one/multiple-server behavior from the identity contract.

## 7. Nearby TV setup

Apple TV uses the stronger X25519/HKDF/AES-GCM setup envelope. Other TVs may use generic device authorization. They share the protocol-v1 human code and ten-minute lifetime, but not secret material or cryptographic payloads.

Despite the section name, **manual TV-code approval is not proximity-bound**. Any authenticated Portico mobile or Web client may enter the current eight-character `XXXX-XXXX` code from any network. Security comes from the ten-minute expiry, single-use state transition, bounded attempt rate, authenticated approver, explicit device preview/confirmation, and the requesting TV's separate high-entropy poll/device proof. The human code alone never authorizes, polls, or redeems a device.

Mobile playback-destination surfaces always expose a full-size **Enter a TV Code** action. It remains available when Local Network permission is denied, discovery is unsupported, discovery fails, no TV is found, or the phone and TV are on different networks. Activating it opens manual entry without first requesting Local Network permission. Input is case-insensitive, accepts pasted whitespace or omitted hyphens, normalizes to canonical uppercase `XXXX-XXXX`, remains visible after a retryable failure, and never logs the entered value.

Nearby discovery is a same-LAN convenience layered into that action, not an authorization requirement and not a separate code system. When compatible TVs are discovered, **Enter a TV Code** may show a nearby indicator and an expandable list. Selecting a discovered TV uses that TV's displayed code/session and still follows the same authenticated approval and device-preview requirements. `No nearby Portico TVs found` is secondary discovery status; it never replaces or disables manual entry. Simulator isolation is treated like any other zero-result discovery state.

Nearby TV approval produces two outputs atomically:

1. a durable Hosted-account Device and access/refresh family, allowing the TV to list and switch among all eligible servers;
2. an optional short-lived, single-use bootstrap for the initially selected server.

The server bootstrap is not required for account authorization. If no server is selected, no server exists, or the chosen server is offline, the TV still becomes account-authenticated and shows the appropriate server chooser/empty state. A bootstrap is exchanged once with the verified server for a server-native family.

The grant is stored in plain server state before commit and remains recoverable to the authenticated poller. Credential collision or state conflict rolls back Device, credentials, bootstrap, and grant transition together.

The TV page contains the QR, code, `web.getportico.tv/device`, **Sign in with a Portico Account**, and **Sign in directly to a server**. Legal URLs are printed rather than rendered as misleading focusable links. Tiny hyperlink-style TV actions are prohibited.

## 8. Local Quick Connect

Local Quick Connect uses the same code grammar and display rules but remains server-coupled Local Auth. The high-entropy secret authenticates polling and exchange. Approval and durable creation of the server-native refresh family commit in one server transaction. A recovery receipt returns only that exact still-valid family after a lost response.

Hosted device authorization and Local Quick Connect are different authorities. A code lookup must not disclose which authority contains a matching code. Cross-family active uniqueness is required within one issuing authority when a shared entry surface accepts both. Independently operated local servers cannot promise global code uniqueness without Hosted coordination. The shared UI selects the authority/server context before lookup; it never searches every server or silently interprets a local code as a Hosted code.

## 9. Concurrency, idempotency, and abandonment

- Every create/complete/redeem/install mutation defines an idempotency key and a stable result lookup.
- Duplicate concurrent completion has one winner; followers receive the same committed result or a deterministic already-completed state.
- A newer explicit user choice cancels the older candidate through an operation generation/abort signal, prevents late publication immediately, and reconciles bounded cleanup independently. An unreachable old operation cannot indefinitely block a newer visible choice; unresolved security material remains quarantined.
- Abandoning a draft leaves no credential or membership. Abandoning after remote commit leaves a resumable operation until acknowledgement/expiry policy completes.
- Background expiry releases active human-code reservations transactionally while retaining minimum audited history.
- Cleanup is best-effort only after the durable security boundary is safe; inability to delete uncertain credential material triggers fail-closed quarantine.

## 10. Conformance

Implementations validate state records against `setup-state.schema.json` and run `conformance.json`. Required adversarial cases include double-clicks, lost commit responses, refresh during every phase, closing either origin, returning after expiry, out-of-order Hosted/local navigation, a newly created account with zero servers, selected server offline, concurrent device redemption, code rotation failure, mismatched QR/code, stale claims, and storage failure during credential publication.

Minimum applicability matrix:

| Rule family | Server | Hosted | Web | Client Core | Apple | Android | Roku | Cast |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Account onboarding | projection only | required | required | required | required | required | device approval only | not applicable |
| Server initialization | required | claim side | bundled required | required | management entry | management entry | not applicable | not applicable |
| Device authorization | Local Quick Connect | required | approval required | required | required | required | required | receiver-specific |
| TV cryptographic setup | bootstrap exchange | required | approval required | envelope helpers | X25519 required | generic device auth | generic device auth | receiver-specific |
| Resume/idempotency | required | required | required | required | required | required | required | required |
| Code continuity | required where shown | required | required where shown | required | required | required | required | required where shown |

Platform adaptations may replace layout and use stronger native cryptography, but cannot change code grammar/lifetime, make account sign-in depend on a server, omit durable recovery, or turn retryable interruption into terminal failure.

Approved exceptions use the full governance record with `Contract: PC-ONBOARDING#<section>` and conformance evidence.

The comparative evidence behind device-link and setup decisions is recorded in `../networking/plex-jellyfin-precedent-review.md`. It is informative, not normative.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.

## October implementation boundary

Direct setup/sign-in is independent of Hosted and accepts HTTP from any address. Setup is deliberately open before an owner exists; same-origin/JSON checks are not a DNS-rebinding guarantee. TV-code sign-in carries the approver's role, with separate device-approval pending/denied states. No device possession key exists.

Outside apps can use direct credentials, device codes or Hosted public-client Sign in with Portico. Hosted consent uses S256 PKCE/state/exact return binding and exchanges a short code for a server/challenge-bound assertion only. Local membership and admission still decide access. Real browser/provider/app-scheme acceptance remains unverified.

Activation retries preserve the current code and must not run before the larger of protocol due time, Retry-After and shared Hosted outage floor. Manual and automatic actions use their respective admission classes. Native/source integration and physical TV flows need the reported acceptance checks.

## October scope and evidence

Hosted app consent/source fixes and shared activation gating are reported, but provider/cookie/OS handoff/TV acceptance remains unverified. The no-email decision still needs residual Hosted reconciliation.

Sources: [hosted](../../../_program/reports/hosted.md), [review-hosted](../../../_program/reports/review-hosted.md), [native-sync](../../../_program/reports/native-sync.md), [accounts-security](../../../_program/reports/accounts-security.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
