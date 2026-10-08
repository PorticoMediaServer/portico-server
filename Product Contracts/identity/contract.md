# Identity, Accounts, Profiles, and Sessions Contract

> Current Portico product contract, reconciled 5 September 2026.


Contract ID: `PC-IDENTITY`  
Revision: `portico.reconciled.1`  
Status: **approved**  
Owners: Product, Security, Hosted Identity, Server Identity, Client Foundations  
Imports: `PC-FOUNDATION` viewer scope and permissions; `PC-GOVERNANCE` lifecycle/exception law; `PC-GLOSSARY` vocabulary  
Coordinates with: `PC-ONBOARDING`, `PC-NETWORKING`, `PC-ACCESSIBILITY-LANGUAGE`  
Applies to: Hosted Services, Portico Server, Web, Client Core, and every first-party client

## Scope and non-goals

This contract owns user/account/profile identity, authentication methods, credential/session boundaries, identity switching, email privacy, and account-level server selection semantics. It does not specify server discovery transports, setup-code protocol mechanics, visual layout, media permissions, or deployment topology except where needed to establish an authority boundary. Enterprise federation, organization tenants, anonymous guest accounts, and provider-specific account recovery beyond Google and Apple are not first-release commitments.

There are no unresolved identity product decisions in this revision. Adding another SSO provider, changing profile limits, or permitting email-less accounts requires a contract change.

## 1. Authority and precedence

This contract defines product behavior, not one implementation. A conflicting runtime, legacy document, generated client, or platform convention is a defect unless source code contains a deliberate exception governed by `../governance.md`. Broad platform forks are forbidden.

The authority split is absolute:

| Principal or fact | Authority | Notes |
| --- | --- | --- |
| Portico Account, contact email, SSO links, MFA, account sessions, Hosted Devices | Hosted Services | Global across servers. |
| Local Auth account, password, profiles, PINs, server sessions, local Devices | The selected Portico Server | Never promoted into Hosted identity. |
| Hosted server membership, invitation, ownership association | Hosted Services | The server receives a signed projection and remains final media authority. |
| Server-local permissions, media authorization, viewer state | The selected Portico Server | Enforced for every request, including Hosted users. |
| Profile viewing identity | Hosted Services for Hosted profiles; Portico Server for Local profiles | Profile identifiers from different authorities are unrelated even if their strings match. |
| Client installation identifier | Client metadata only | Never a credential, proof of possession, or required authentication factor. |

No identifier may change meaning across boundaries. Every viewer scope follows current `PC-FOUNDATION` ownership and fencing rules; the original [`ViewerScope`](../../../References/Contract%20Baseline%202026-09-05/foundation/viewer-scope.schema.json) is a reference shape to port, not the rebuilt wire authority. `account-only` is Hosted and omits `serverId` before server selection; `server-bound` includes it. `authorizationRevision` fences requests, grants and cached projections. Durable viewer data is owned by the stable authority/account/profile/server key defined in PC-FOUNDATION; a revision change does not create a different owner or erase history.

## 2. Product identities

### 2.1 Portico Account

A Portico Account is a global identity. It can exist with zero servers, own or join multiple servers, remain signed in while every server is offline, and switch servers without re-entering account credentials. Account authentication never implies access to any server.

Every account has:

- one immutable opaque account ID;
- one required unique username;
- one verified contact email;
- one or more authentication identities (password, Google, Apple, or future approved provider);
- zero or more Hosted Devices and session families;
- one primary profile counted within a maximum of eight active profiles;
- zero or more server memberships.

### 2.2 Local Auth account

A Local Auth account exists on exactly one Portico Server and is useful when the user chooses **Sign in directly to a server**. It is not a degraded Portico Account, cannot receive Hosted invitations, and does not appear in global server lists. Its credentials and profiles remain on that server. The same maximum of eight active profiles, including the primary profile, applies to a Direct Sign-In account.

The product must label the authority clearly. Approved user-facing phrases are:

- `Portico Account`
- `Sign in with a Portico Account`
- `Sign in directly to a server`
- `This Server` when a compact origin label is required

`Server-only authentication`, `local account` when it could mean an operating-system account, and any suggestion that direct sign-in is less secure are prohibited.

### 2.3 Profile

A profile is a viewing identity beneath one account, never a login account or server role. Profile-owned data includes progress, history, ratings, Saved resources, search history, queues, downloads, notifications, Live TV viewer state, and viewing preferences. Account-owned data includes credentials, Devices, memberships, ownership, security settings, and aggregate limits.

The primary profile administers subordinate profiles inside its account. It does not gain server administration merely by being primary. Server ownership and profile administration are separate authorities.

When an account has one profile, clients skip the chooser. With multiple profiles, mobile/web may use a previously and explicitly trusted exact-scope selection; television asks at launch by default. A remembered choice is keyed by the complete viewer scope and installation, and never crosses accounts, servers, or Hosted/Local authority.

Switching profiles is an identity transaction. Before the replacement profile is shown, the client must fence the old viewer, stop or detach this installation's affected playback under PC-PLAYBACK and PC-SHARING-SOCIAL, close its old event subscriptions, cancel/fence mutations, clear cached private presentation, durably persist the candidate credential, verify its identity, and publish it. This does not stop independent playback by that profile on other devices or delete its durable server data.

### 2.4 Hosted outage and offline profile selection

An established server-native session remains usable during a Hosted outage according to `PC-NETWORKING`; a television MUST NOT strand that session merely because it normally shows a profile chooser at launch.

- Reopening the exact last verified Hosted `ViewerScope` on an established installation may continue without Hosted while its server-native session and signed membership policy remain valid.
- Switching to another Hosted profile offline is allowed only on an explicitly trusted installation that already holds a server-scoped offline profile proof minted after that profile's most recent successful online selection/PIN challenge. The proof is bound to installation, server, profile, PIN revision, authorization revision, and a maximum 30-day expiry.
- A PIN-protected offline proof is encrypted under platform-protected material and a rate-limited PIN unlock; it never stores the PIN or a reusable Hosted credential. PIN revision, profile disablement, membership revision, trust removal, or expiry invalidates it.
- A profile not previously unlocked on that trusted installation is unavailable until Hosted returns. The chooser explains this without revealing private profile content.
- A new installation cannot bootstrap a Hosted profile, server membership, or offline proof while Hosted is unavailable.
- Downloads remain visible only after their own `ViewerScope` and offline-authorization proof validate under `PC-SAVED-DOWNLOADS`; a cached profile name is not download authority.

This bounded path intentionally avoids a last-user-only outage trap while refusing to turn every cached profile into an offline credential.

## 3. Username and email policy

### 3.1 Username

A final Portico Account never receives a generated username. Username selection is mandatory for password registration and every new SSO identity.

- Length: 3–32 characters.
- Grammar: `^[A-Za-z0-9][A-Za-z0-9._-]{1,30}[A-Za-z0-9]$`.
- Comparison and uniqueness: trimmed and case-insensitive using the canonical `en-US` lowercase form.
- Leading/trailing punctuation, spaces, consecutive normalization surprises, email addresses, and provider display names are not silently accepted as substitutes.
- A collision returns to username selection with the provider proof preserved through a newly issued one-time onboarding token. It must not restart provider authentication.

Username is the ordinary social and membership label. Contact email is not shown in server member, profile, activity, playback, or invitation-acceptance directories after membership is established.

### 3.2 Contact email

A verified contact email is required for recovery and security notices. It is private account data, not a display identity.

- Password registration completes email verification before the address is treated as verified.
- A provider-verified email may establish the initial contact address.
- An Apple private-relay address is valid and remains marked `privateEmail=true`; Portico must not attempt to discover or request the hidden address.
- If the provider supplies no verified email, onboarding requires a separate email-ownership challenge. Merely submitting an address alongside an SSO assertion never verifies it.
- A server owner who sends an invitation may see the address they entered while the invitation is pending. After acceptance, membership surfaces use username and profile identity, not the invite or contact address.
- Logs, telemetry, route documents, QR payloads, device codes, and server-signed policy must not contain raw contact email unless the particular audited operation requires it.

Changing contact email is a high-risk account operation and requires fresh account authentication, MFA when configured, verification of the new address, notification of the old address, and revocation policy appropriate to the risk.

## 4. Authentication methods

### 4.1 Password

Hosted and Local password creation use the shared password contract: at least eight Unicode characters, at least one uppercase, one lowercase, and one number or non-letter special character, with a maximum of 72 UTF-8 bytes at the bcrypt boundary. Password managers, paste, autofill, and explicit show/hide controls remain enabled. Policy applies at create/change/reset; valid existing hashes remain valid.

Unknown-account and bad-password responses are indistinguishable. Authentication may upgrade an allowed hash cost. Password reset never reveals whether an account exists and never returns production reset material in the response.

### 4.2 Google

- Browser: authorization-code flow with PKCE, state, and nonce in the system browser.
- iOS/iPadOS: Google-supported system authentication session or native provider SDK; never an in-app WebView or a React Native imitation page.
- Android: Google-supported native/system flow using the exact registered Android application ID and signing-certificate identity.
- TV: device authorization through `web.getportico.tv/device`; no embedded Google credential collection.

Hosted Services verifies issuer, signature, exact allowlisted audience, expiry, nonce, state/PKCE where applicable, and one-time transaction use. Client-provided provider claims are never authoritative.

### 4.3 Apple

- Apple platforms: native Sign in with Apple authorization flow and nonce-bound token exchange.
- Web, Android, and non-Apple platforms: Apple's browser authorization flow through the canonical Portico web origin.
- TV: device authorization through `web.getportico.tv/device`.

Apple private relay is supported as described in §3.2. Apple subject is the stable external identity; email is never used as the provider identity key.

### 4.4 Direct server sign-in

Direct sign-in accepts a server address plus credentials owned by that server. It must work without Hosted Services, a Portico Account, public remote access, or a claimed server. A local server page never collects Portico Account credentials; it opens or hands off to the canonical Hosted web flow when Portico Account authentication is requested.

### 4.5 Limited-input devices

TVs and other limited-input devices use the device authorization contract in `../onboarding/contract.md`. Password entry on television remains an available direct method where the platform supports it, but SSO credentials are never collected on the TV.

### 4.6 Linking and unlinking sign-in methods

Provider identities are linked only from an authenticated account-security flow with a fresh provider authorization. Matching email addresses never silently merge accounts or link identities. One provider subject can belong to only one Portico Account.

Linking, unlinking, and adding a password require recent account authentication and MFA when configured. The product must not remove the last currently usable sign-in/recovery method. If a provider identity is already linked elsewhere, return a privacy-preserving conflict and direct the user to account recovery; do not reveal the other account.

Portico does not automatically merge accounts. An explicit future merge feature would require proof of both accounts, conflict rules for usernames/profiles/memberships/devices, a revocation transaction, and a separate contract change.

### 4.7 MFA and recovery

Hosted accounts support TOTP MFA, single-use recovery codes, verified email recovery, session/device review, and account deletion. Accepted TOTP steps cannot replay. Recovery and MFA challenges are purpose-bound, expire, are rate-limited independently of password login, and are audited without secret material.

An SSO-only account may add a password after fresh provider/MFA proof. Password reset operates only for accounts with a password credential and never discloses account existence. Provider-account recovery remains with the provider; Portico must not present its email reset as a way to recover Google or Apple credentials.

## 5. Session and Device model

Hosted account credentials and selected-server credentials are distinct families with different audiences and revocation boundaries.

1. Hosted Services authenticates the account and issues the Hosted account family.
2. The user selects a server and profile.
3. Hosted Services issues only a short-lived, one-time, exact-server bootstrap/selection envelope.
4. The verified server exchanges it for its own durable access/refresh family.
5. Navigation, playback, events, and refresh continue directly with that server.

Hosted Services must never issue a durable refreshable server credential. A client must never store a Hosted refresh token inside a selected-server session record.

The completed browser remember-session flow uses Secure, HttpOnly, host-scoped renewal cookies and origin/CSRF protections. A deliberately incomplete memory-only bearer flow may remain during local integration, as recorded in accepted journey 08; it must not persist bearer credentials in localStorage or be described as completed browser persistence. Cookie and bearer adapters share the same domain authorization and revocation semantics. Native credentials are bearer-only, excluded from URLs/logs, and stored in Keychain, Keystore, or an equivalent platform secret store. Refresh tokens rotate with reuse detection.

Transient transport, DNS, TLS, timeout, `408`, `429`, and `5xx` failures preserve credentials and signed-in state. Explicit sign-out, verified revocation, invalid/reused refresh, deleted account/profile, or another documented terminal authorization response may clear them. Ambiguous failure is never terminal.

A Device is authority-issued and revocable. Hosted and server Device IDs are separate namespaces. An optional installation ID may support naming, push, preference partitioning, and recovery hints, but authentication must succeed when it is absent or rotated.

## 6. Multiple-server and zero-server behavior

Account sign-in completes independently of server count.

- **Zero servers:** show the signed-in account frame and a useful empty state with invitations, account settings, sign-out, and instructions for obtaining access. Never report login failure.
- **One eligible server:** connect automatically when a trusted route and profile choice are available; otherwise show the required profile or reachability step.
- **Multiple servers:** present a server chooser on first use and expose **Switch server** in Settings and the account/server selector. Remember the last successful server only as a convenience hint.
- **Selected server unavailable:** preserve the Portico Account session and show retrying status, **Choose another server**, account settings, and sign-out. Unrelated navigation must not become dead UI.
- **Membership removed:** clear only that server's credential and choice. Do not sign out of the Portico Account or unrelated servers.

A server/profile selection becomes visible only after route verification, server-native credential durability, final server identity assertion, and runtime publication succeed as one fenced transaction. Failure restores the previous known-good selection; uncertain rollback fails closed and quarantines both candidates.

## 7. State machines

### 7.1 Account authentication

`signed_out → authenticating → onboarding_required | account_authenticated | failed_retryable | failed_terminal`

- `onboarding_required → choosing_username → verifying_contact_email? → account_authenticated`
- Cancel/abandon before credential issuance returns to `signed_out` without a partial account.
- A consumed or expired onboarding transaction produces a resumable restart message, not a blank screen.
- `failed_retryable` preserves prior authenticated state and retries only the attempted candidate.

### 7.2 Viewer publication

`account_authenticated → server_directory → server_selected → profile_selected → route_verified → server_session_persisted → viewer_verified → active`

No product screen may render fake account/server/profile defaults to bridge missing data. Each intermediate state has an explicit loading, empty, chooser, or actionable failure surface.

### 7.3 Sign-out

Sign-out immediately fences product UI, stops active media/network work, revokes the scoped remote family where reachable, and read-verifies local deletion. `Sign out of this server`, `Sign out of Portico`, and `Sign out all devices` are distinct operations. Failure to remove durable credentials remains unauthenticated and reports the storage cleanup warning; it does not silently restore the session.

## 8. Error and privacy language

Authentication errors state the affected authority and next action without exposing account existence, provider subjects, raw provider messages, tokens, addresses, fingerprints, or internal hostnames.

Examples:

- `That username or password isn't correct.`
- `Portico couldn't reach this server. Check the address and your connection, then try again.`
- `Your Portico Account is signed in, but this server is currently unavailable.`
- `Your sign-in is still safe. Portico will keep trying to reach account services.`

`Something interrupted the request`, `identity_login_failed`, raw JSON problem bodies, and user-visible `api.getportico.tv` links are prohibited. All user-facing Hosted links use `web.getportico.tv`.

## 9. Security invariants

- Provider, password, PIN, refresh, setup, and bootstrap secrets never appear in URLs, analytics, logs, crash reports, QR payload telemetry, or diagnostics.
- Every one-time token is audience-, purpose-, identity-, and expiry-bound and consumed atomically.
- CSRF protections apply to cookie-authenticated mutations; native bearer requests do not pretend to be browser-cookie flows.
- Rate limits are independent by account, IP, Device/session, and secret/code class where appropriate. Error bodies remain enumeration-resistant.
- Profile PINs are convenience locks, not account authentication. Four numeric digits, bcrypt cost 10, monotonic PIN revision, attempt limits, delay, and lockout are required.
- Hosted account/profile deletion is an immediate authority tombstone plus eventual per-server erasure, not a fictional cross-server transaction. Hosted sign-in, new bootstrap, invitation, membership, and profile assertions stop atomically at acceptance. Every reachable server receives a revisioned idempotent erasure job and acknowledgement requirement; offline servers remain visibly `pending-offline-servers` until they reconcile.
- An account cannot enter deletion while it owns a server. Each owned server must first be transferred, unclaimed into its Local recovery-owner authority, or explicitly deleted. Pending invitations are cancelled, active server-hosted groups/receivers end, and shared-resource ownership is transferred or deleted by an explicit previewed choice.
- Server erasure removes or anonymizes viewer state, rules, group membership, pending work, and downloadable authorization according to the owning contract. Media recordings/files are not silently deleted merely because personal identity is erased; ownership/retention resolution is explicit.
- Completion means Hosted erasure is complete and every known server has acknowledged the current tombstone. Until then the account is inaccessible and the user-visible deletion receipt shows pending servers. Minimum non-identifying security/audit evidence and tombstone IDs may remain for abuse prevention, replay defense, and proof of completion under a documented retention period.
- No fake fixture identity, `Portico` fallback account, or stale cached directory may be published as authenticated state.

## 10. Conformance

Every client and service must execute the cases in `conformance.json`. Machine-readable identity records must validate against `identity-model.schema.json`. Conformance includes provider onboarding, private relay, zero/multiple servers, server/profile switching, transient failure preservation, terminal revocation, durable publication, concurrent candidate replacement, and rollback uncertainty.

Minimum applicability matrix:

| Rule family | Server | Hosted | Web | Client Core | Apple | Android | Roku | Cast |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Authority/scopes | required | required | required | required | required | required | required | required |
| Password/SSO | Local required | required | required | required | required | required | device auth | device auth |
| Profiles/switching | required | Hosted required | required | required | required | required | required | receiver scope |
| Durable sessions | required | required | required | required | required | required | required | required |
| Zero/multiple servers | not applicable | required | required | required | required | required | required | not applicable |
| Email privacy | projection only | required | required | required | required | required | no email display | no email display |

Platform adaptations may use native authorization sessions, secret stores, profile controls, and navigation, but may not remove an authentication choice, redefine authority, use email as provider identity, require installation metadata, or publish product UI before durable final scope verification.

Approved exceptions use the full `PORTICO-CONTRACT-EXCEPTION` record in `../governance.md`, with `Contract: PC-IDENTITY#<section>`. An exception without approval and conformance evidence is a defect.

The comparative evidence behind several choices is recorded in `../networking/plex-jellyfin-precedent-review.md`. That note is informative and cannot override this contract.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.
