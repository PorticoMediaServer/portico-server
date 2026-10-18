# 01 — Users, identity, sign-in, sessions, devices, permissions and parental controls

Read-only static review, 11 October 2026. Nothing was built, run or tested. Every statement about behaviour is a **source finding** (what the code says it does), not something I watched happen.

Path shorthand used throughout (all relative to `/home/justin/projects/Portico`):

| Short | Means |
|---|---|
| `S/` | `apps/portico-server/server/internal/` |
| `C/` | `apps/portico-server/packages/client-core/src/` |
| `W/` | `apps/portico-server/web/src/` |
| `N/` | `apps/portico-react-native/apps/native/src/` |
| `PC/` | `portico-internal/Product Contracts/` |

Comparison paths are written `jellyfin/…`, `plezy/…`, `swiftfin/…` and so on.

## Scope and sources

**Compared:** who a "person" is in each product (accounts, users, profiles, roles), how they sign in, what a session and a device are, what an owner can restrict per person, parental controls, API keys, TV sign-in codes, password storage and reset, first-run setup, what happens when a person is removed, multi-server identity, and how Portico's dependence on its Hosted service changes failure behaviour compared with Jellyfin (all local) and Plex (cloud).

**Portico, read in full or nearly so**

- Server: all of `S/identity/` non-test files (about 9,000 lines), all of `S/access/`, `S/contentaccess/item.go`, `S/authoritygate/gate.go`, `S/hostedtrust/`, `S/testauth/`, `S/hosted/identity_assertion.go`, parts of `S/hosted/` (service, claim cleanup, membership push, legacy policy), `S/catalog/restrictions.go`, and the identity routes in `S/httpapi/` (`onboarding.go`, `direct_identity.go`, `identity_routes.go`, `access_admission.go`, `access_area.go`, `owner_authority.go`, `request_locality.go`, `principal_cache.go`, `restriction_admission.go`, `session_family_logout.go`, `device_contract.go`, the sign-in and middleware parts of `router.go`).
- Schema: the identity, access, device, quick-connect and Hosted-link tables, views and triggers in `S/persistence/migrations/0001_baseline.sql`.
- Docs: `01 Overview`, `02 Server`, `07 Hosted Services`, `Backlog.md`, `PC/identity/contract.md`, the setup-code and recovery-owner parts of `PC/onboarding/contract.md`, `apps/portico-server/server/api/identity.md` and `admin-access.md`.
- Clients: `C/session-refresh.ts`, `C/local-session.ts`, `C/identity-client.ts`, `C/installation.ts`, `C/known-servers.ts`, the refresh part of `C/server-connections.ts`, the header of `C/credentials.ts`; `W/screens/auth/SignIn.tsx`, `W/bridge/server-connections.ts`, `W/bridge/profile-trust.ts`; the session model in `N/app/session.tsx`.
- Hosted: `portico-internal/hosted-services/hosted/ACCOUNT_SECURITY.md` and the package layout only.

**Comparison projects, read for this domain**

- Jellyfin server: `Jellyfin.Server.Implementations/Users/UserManager.cs`, `DefaultAuthenticationProvider.cs`, `DefaultPasswordResetProvider.cs`, `Jellyfin.Api/Auth/*` handlers, `Emby.Server.Implementations/QuickConnect/QuickConnectManager.cs`, `Session/SessionManager.cs` (authentication, tokens, logout), `Devices/DeviceManager.cs`, `MediaBrowser.Model/Users/UserPolicy.cs`, `MediaBrowser.Controller/Entities/BaseItem.cs` (parental checks), `Localization/LocalizationManager.cs` and `Localization/Ratings/*.json`, `Security/AuthorizationContext.cs`, the user, API-key and startup controllers, EF user configuration.
- Clients: jellyfin-web `connectionManager.js`; jellyfin-androidtv `auth/` package; Swiftfin user security files; Streamyfin `JellyfinProvider.tsx` and `secureCredentials`; Plezy `lib/profiles/*`, `credential_vault.dart`, auth services.
- Navidrome configuration and model names; Audiobookshelf `server/auth/*` and `models/User.js` (checked for specific facts only).

**Not reached / lighter than I would like**

- Hosted's own Go code (accounts, sessions, OIDC, device authorization, directory) was not read line by line; statements about Hosted come from its documents and from the server side of each exchange.
- `S/identity/avatars.go`, `topshelf.go`, `migrate_sealed.go`, `two_factor.go` were skimmed, not traced.
- Native Keychain / Keystore modules were located but not read.
- Plex and Emby are closed source: statements about them are from public documentation and general knowledge and are marked as such.
- The Roku, Vega and Cast receivers were not examined.

## How Portico does it today

### People: accounts, profiles and three tiers

A Portico server keeps its own list of **accounts** (`accounts` table) and each account has up to eight **profiles** (`direct_profiles`; the limit is a database trigger, `0001_baseline.sql:4221`). An account is what signs in; a profile is who is watching. Watch state, history, playlists and restrictions hang off the profile.

Each account has exactly one **membership row** (`direct_memberships`) with a tier: `owner`, `admin` or `member` (`S/identity/access_tier.go`). There is exactly one owner (a unique index). The first account ever created becomes the owner through a trigger (`0001_baseline.sql:4249`). An account's first profile is its **primary** profile; only the primary profile carries the account's tier, every other profile acts as a plain member (`S/identity/direct_profiles.go:372-375`).

An account is one of two kinds:

- a **direct account** with a username and bcrypt password stored on the server, or
- a **Portico Account member**: an ordinary local account with an empty password that is linked to a Hosted account id (`account_portico_links`). Hosted only proves *who* someone is; the server decides whether that person is a member, their tier, libraries, profiles, PINs and devices (`S/identity/portico_accounts.go:12-16`).

### What an owner can restrict

There are two separate restriction systems, at two different levels.

1. **Per profile** (`profile_restrictions`, `S/identity/restrictions.go`): a maximum age rating picked from one named rating system, whether unrated titles are allowed, a list of blocked labels, and four feature switches (downloads, Live TV, DVR, Watch Together). A profile can also be narrowed to a subset of its account's libraries. Managed by the account's primary profile.
2. **Per account** ("member limits", `access_limits`, `S/access/limits.go`): maximum simultaneous streams, remote bitrate cap, a maximum content rating on a fixed US ladder, allow-unrated, an access schedule (weekday/time windows in a chosen time zone), a Live TV channel allow/deny list, and denied labels. Managed by the owner or an admin.

Library access is an allow list on the account (`direct_memberships.allowed_libraries`; owner and admins see everything), intersected with the profile's optional narrower list (`S/identity/direct_policy.go:15-67`). A new member starts with **no** libraries.

Rating and label restrictions are applied inside the catalogue's SQL (`S/catalog/restrictions.go`), so lists, counts, search, a title opened by id, and playback all agree. Ratings from different countries are compared by "minimum admission age" using a built-in table of seven systems (`S/identity/ratings.go`). Episodes inherit their show's rating (`S/metadata/repair_fields.go:521-531`).

### Signing in

- **Username and password** (`POST /v1/direct/sign-in`). Passwords are bcrypt cost 10, at least 8 characters, at most 72 bytes (`S/identity/hash_cost.go`, `direct_accounts.go:85`). An unknown username costs the same bcrypt work as a real one. Repeated failures cause a short, growing **cool-off** (1 s doubling to a 60 s cap, starting at the fifth failure, halving every 15 minutes), counted per source network and per known device, plus an account-wide budget; nobody is ever locked out permanently (`S/identity/credential_security.go`). A per-address and per-network request limiter sits in front (`S/httpapi/rate.go`).
- **Optional TOTP second factor** with ten recovery codes for direct accounts (`S/identity/two_factor.go`). When on, the password step returns a five-minute challenge instead of a session.
- **Portico Account**: the app asks the server for a challenge, has Hosted sign an identity assertion for that server and challenge, and presents it to the same sign-in route. The server verifies it offline against Hosted's root key compiled into the build (`S/hosted/identity_assertion.go`). Apple and Google sign-in exist only at Hosted.
- **TV code** ("Quick Connect", `S/identity/quick_connect.go`): the TV shows an eight-character code valid for ten minutes; someone already signed in approves it; the TV is then signed in **as the approving profile**. The request lives in the database, so it survives a restart. The approval page is served by the server itself (`S/httpapi/onboarding.go:250-255`).
- **Invitations** (`S/access/invitations.go`): the owner creates a code (7 days by default) carrying a tier and library list; the invitee picks a username and password, or joins with a Portico Account.
- **Self-registration** has an owner policy of `off` (default), `invite-only` or `open` on the server, but no first-party screen sets it.

After sign-in the client holds an **account session** that can manage the account but cannot browse or play. Choosing a profile (entering its four-digit PIN if it has one) issues a separate **viewing session**; the apps then sign the account session out (`N/app/session.tsx:63-68`). An account with one PIN-less profile skips the chooser (`S/identity/direct_accounts.go:295-298`).

### Sessions and tokens

A sign-in creates a **session family** bound to a **device record**. The client gets a 15-minute access token and a refresh token; both are random 256-bit strings stored on the server only as SHA-256 digests (`S/identity/families.go:146-161`). Refreshing rotates both and slides the sign-in forward 90 days (`LocalSignInWindow`). Presenting an old refresh token revokes the whole family ("refresh reuse"). A refresh carries a client-chosen request id; if the answer is lost, repeating the same request returns the same stored answer (`S/identity/refresh.go`).

Every authenticated request re-checks, in the database, that the token is the family's current one, the family is not revoked, the account is not disabled, its "epoch" has not moved, the profile is not deleted and the device is still approved (`S/identity/families.go:259-272`, view `authorization_access`). Requests from outside the LAN may use a 15-second in-memory cache of that answer, invalidated by any authority-changing write (`S/httpapi/principal_cache.go`, `router.go:618-626`).

Changing a password, a profile's PIN, restrictions or library list, an account's tier or libraries, or disabling an account **revokes the affected sessions** rather than only changing what they may do.

### Devices

`identity_devices` holds one row per (account, installation id): name, platform, app version, last IP, first and last seen, an approval state and a "remember this account" flag. People can rename a device, sign it out, or forget it (`S/identity/devices.go`). A server policy can hold new devices as `pending` until approved, but no first-party screen sets that policy.

A second, older device list (`access_devices`, `S/access/devices.go`) is fed from an `X-Portico-Device-Id` request header at playback and backs the admin console's "Devices" list and the `deviceApprovalRequired` setting.

### Setup, recovery and ownership

First-run setup is open to any browser that can reach the server; the first account created wins (`S/httpapi/onboarding.go:141-145`, recorded as Justin's decision of 23 Sep). A server set up with a Portico Account also gets a local **recovery owner** password, usable only from a private network unless the owner opts in to remote recovery (`S/identity/onboarding.go:198-214`). An owner who has lost everything runs `portico-server recover-owner` on the host to get a one-time temporary password (`S/identity/recover_owner.go`). Ownership can be transferred to another member with the owner's password or a fresh Portico identity assertion.

### What Hosted does and does not do

Hosted holds Portico Accounts, their password/SSO/2FA, and an index of which servers an account belongs to. The server pushes membership changes to Hosted; Hosted tells the server when a member leaves or deletes their account. Hosted is needed at the moment a Portico Account signs in on a new device and for discovery; an established session is renewed by the server alone.

## How the others do it

### Jellyfin

**Users.** One flat list of users on each server. Every user is a login with its own (possibly empty) password. There are no profiles under a user and no "owner": any number of users can be administrators, and the server refuses to delete the last user or the last administrator (`jellyfin/Jellyfin.Server.Implementations/Users/UserManager.cs:388-408`).

**Permissions.** Each user has a `UserPolicy` (`jellyfin/MediaBrowser.Model/Users/UserPolicy.cs`) with about forty fields: administrator, hidden from the login screen, disabled, remote access allowed, media playback allowed, audio and video transcoding allowed, remuxing allowed, force-transcode remote sources, content deletion (globally or per folder), downloading, collection, subtitle and lyric management, Live TV access and management, remote control of other users, SyncPlay access level, all-or-listed libraries, all-or-listed channels, all-or-listed devices, maximum sessions, remote bitrate limit, login attempts before lockout, access schedules, maximum parental rating, blocked and allowed tags, and which kinds of unrated item to block.

**Parental controls.** A user has a maximum rating *score*. Ratings are mapped to scores through one JSON table per country (44 files in `jellyfin/Emby.Server.Implementations/Localization/Ratings/`), resolved against the library's metadata country, with handling for `CC:rating` and `CC-rating` prefixes and for lists like `SE:15 / SE:15+` (`LocalizationManager.cs:374-507`). An item's owner-set **custom rating** overrides its official rating (`BaseItem.cs:1876-1881`). Unrated handling is chosen per item type. Blocked tags and allowed tags are inherited from parent folders and series. Access schedules (day and hour ranges) are checked on **every request**; administrators are exempt (`Jellyfin.Api/Auth/DefaultAuthorizationPolicy/DefaultAuthorizationHandler.cs`).

**Sign-in and tokens.** Username + password returns one opaque access token per (user, device id). It never expires and is stored in plain text in the `Devices` table; signing in again on the same device deletes the old token (`SessionManager.cs:1784-1811`). A password change revokes the user's other tokens. There is no refresh token and no token/device binding beyond the client-supplied device id. Passwords are PBKDF2-SHA512 with 210,000 iterations (`MediaBrowser.Model/Cryptography/Constants.cs:21`).

**Brute force.** After N failed logins (default 3 for users, configurable; -1 disables) the account is **disabled** until an administrator re-enables it (`UserManager.cs:680-693`). There is no request rate limit in the server.

**Quick Connect.** A six-digit code held in memory for ten minutes; any signed-in user types it in their own client; the new device gets that user's token (`QuickConnectManager.cs`). A server setting turns it off.

**Password reset.** "Forgot password" from inside the LAN writes a PIN file into the server's config folder; whoever can read that folder enters the PIN and the password becomes the PIN (`DefaultPasswordResetProvider.cs`). An administrator can also set or blank any user's password.

**API keys.** Administrators create keys; a key is an unrestricted administrator credential with no user, stored in plain text.

**Extensibility.** Authentication and password-reset providers are plugin interfaces; an official LDAP plugin and a community SSO (OIDC/SAML) plugin use them.

**First run.** The startup wizard endpoints are open to anyone until the wizard is completed (`FirstTimeSetupHandler.cs`).

**Clients.** jellyfin-web keeps server and token in `localStorage`. The Android TV app keeps a JSON file of servers and users with tokens in app-private storage, shows a user grid, and offers "auto-login as last/specific user" and "always authenticate" options (`jellyfin-androidtv/.../auth/store/AuthenticationPreferences.kt`). Swiftfin keeps tokens in the Keychain and lets each saved user be locked on that device with Face ID/passcode or a local PIN (`swiftfin/Shared/Objects/LocalUserAccessPolicy.swift`). Streamyfin keeps several saved accounts per server, each optionally protected by a local PIN or "re-enter password".

### Plezy

Plezy is a client, so it has no user database. It layers its own **profiles** on top of servers: a *local profile* (Plezy-only, optional four-digit PIN stored as a salted SHA-256 on the device) owns one or more server connections each with its own token; a *Plex Home profile* is discovered from the Plex account and switched through plex.tv, which enforces the Home PIN (`plezy/lib/profiles/profile.dart`). Tokens are AES-GCM encrypted in its database with a key kept in shared preferences, which its own comment describes as obfuscation rather than OS-backed protection (`plezy/lib/services/credential_vault.dart:15-18`). A rejected token (401 or 403) wipes that cached token and raises a "sign in again" banner while keeping the connection.

### Others (lighter)

- **Plex** (public documentation): a cloud account at plex.tv is the identity. A server is "claimed" by an account, which requires local access to the server. Friends are invited by email or username and given libraries; a Plex Home has managed users with optional PINs and restriction profiles (younger kid, older kid, teen) plus label allow/deny, and per-user switches for downloads and Live TV. Tokens are long-lived per device and are revoked centrally from the account's device list.
- **Emby** (public documentation): Jellyfin's ancestor, same per-user policy model, plus optional Emby Connect cloud linking.
- **Navidrome**: admin and regular users; a signed session token with a sliding timeout; a sign-in rate limit; optional authentication by a trusted reverse-proxy header; share links.
- **Audiobookshelf**: user types root/admin/user/guest; per-user library list, tag allow or deny list, explicit-content switch and permission flags; built-in OpenID Connect; short-lived access tokens (1 hour) with 30-day refresh tokens; API keys with expiry that act as a user.

## Architectural differences

Each entry ends with an assessment: **Portico ahead**, **Portico behind**, **Trade-off (subjective)** or **Unclear**. "Deliberate" means I found the choice written down. Sizes (small / medium / large) are rough.

### People and roles

### A-01.1 Accounts with profiles underneath, versus a flat list of users
- **Portico:** two levels. An account signs in; up to eight profiles under it are who is watching (`S/identity/direct_profiles.go`, trigger at `0001_baseline.sql:4221`). Profiles have no password, only an optional four-digit PIN.
- **Jellyfin:** one level. Every person is a user with their own password; a family of five is five logins. No profiles.
- **Plex / Plezy:** Plex is account + Home users (managed users have no password, optional PIN); Plezy adds its own client-side profiles on top of any server.
- **Why it matters:** the two-level model is what makes "Who's watching?" on a TV cheap (one sign-in, then a picker), and it is what Netflix-trained households expect. The cost is two kinds of thing to explain, two places restrictions live (see A-01.5), and the fact that the wall between profiles is only a PIN (A-01.26).
- **Assessment:** **Portico ahead** for household use; deliberate (`PC/identity/contract.md` §2.3). Jellyfin's model is simpler to reason about for "separate adults who each want a real login", which Portico covers with separate member accounts.

### A-01.2 Three fixed tiers with exactly one owner, versus an administrator flag
- **Portico:** `owner` > `admin` > `member`, one owner enforced by a unique index (`0001_baseline.sql:3814`); the tier predicate is `identity.Grants` (`S/identity/access_tier.go`). Ownership moves only by an explicit transfer (`direct_accounts.go:754-836`).
- **Jellyfin:** `IsAdministrator` is a per-user boolean; any number of admins; no owner. The only rule is "at least one user and one admin must remain".
- **Why it matters:** a single owner gives Portico one unambiguous answer to "who holds backups, security settings, API keys and the Hosted claim", and stops an admin promoting themselves. It also creates a single point of loss, which is why the recovery command exists.
- **Assessment:** **Trade-off (subjective)**, leaning Portico. Deliberate (`api/admin-access.md` "Access tiers").

### A-01.3 A few coarse capabilities, versus forty per-user permission switches
- **Portico:** what a person may do follows from tier (owner/admin/member) plus four per-profile feature switches (downloads, Live TV, DVR, Watch Together) and the per-account limits. There is no per-person switch for most individual capabilities.
- **Jellyfin:** `UserPolicy` (`jellyfin/MediaBrowser.Model/Users/UserPolicy.cs`). The table shows what each product has.

| Per-person control | Jellyfin | Portico |
|---|---|---|
| Disable a person | `IsDisabled` | `direct_memberships.disabled` (yes) |
| Hide from login screen | `IsHidden` | not applicable (no public user list, A-01.22) |
| Library allow list | `EnableAllFolders` / `EnabledFolders` | account list ∩ profile list (yes) |
| Remote access allowed | `EnableRemoteAccess` (per user, every request) | server-wide only (A-01.44) |
| Device allow list | `EnableAllDevices` / `EnabledDevices` | approve/deny per device; no per-person list (A-01.31) |
| Maximum sessions | `MaxActiveSessions` | none; counts streams instead (A-01.46) |
| Remote bitrate cap | `RemoteClientBitrateLimit` | `remoteBitrateKbps` (yes) |
| Playback allowed at all | `EnableMediaPlayback` | none |
| Audio / video transcoding allowed | two switches | server-wide `transcodingEnabled` only |
| Remux allowed, force transcode of remote sources | two switches | none |
| Download allowed | `EnableContentDownloading` | per profile `allowDownloads` (yes) |
| Delete media | `EnableContentDeletion` (+ per folder) | owner/admin tier only |
| Collection, subtitle, lyric management | three switches | tier; shared subtitles need the owner |
| Live TV watch / manage | two switches | `allowLiveTv`, `allowDvr`, channel allow/deny (yes, finer for channels) |
| Control other people's sessions | two switches | social domain (not examined here) |
| Group watch | `SyncPlayAccess` (none / join / create) | `allowWatchTogether` on/off |
| Login lockout threshold | `LoginAttemptsBeforeLockout` | none by design (A-01.21) |
| Access schedule | `AccessSchedules` | per account (A-01.10) |
| Maximum rating, tags, unrated | yes | yes (A-01.5 to A-01.8) |
| Sign-in method per user | `AuthenticationProviderId` | direct or Portico Account per account |

- **Why it matters:** each Jellyfin switch exists because some owner needed it: "my nephew may watch but not transcode 4K", "the kids' account must not delete", "this account is LAN-only". Portico's tiers cover the common cases with far less surface to test and explain.
- **Assessment:** **Trade-off (subjective)**. Portico's coarse model fits "low ceremony". The three I would expect owners to ask for first are per-person remote access, per-person transcoding (protecting a weak box from one heavy user), and a browse-only/no-playback state. Adding switches is small each (a column in the limits envelope and one admission check), but every switch is a permanent test and UI cost.

### A-01.4 New members start with no libraries; Jellyfin users start with all
- **Portico:** `allowed_libraries` defaults to `[]` and an empty list means nothing is visible (`S/identity/direct_policy.go:12-14`; invitations in the web app always send an empty list, `W/screens/server/Access.tsx` `create`).
- **Jellyfin:** `EnableAllFolders = true` by default in a new policy.
- **Why it matters:** Portico is safe by default but adds a step: invite, then go back and grant libraries. A new member who signs in before that sees an empty server.
- **Assessment:** **Trade-off (subjective)**; deliberate (`api/identity.md` §10: "Reaching the server is not evidence of what its owner wants shared"). A library picker in the invitation dialog would keep the safety and remove the extra step (small, web only; the server already accepts the list).

### Restrictions and parental controls

### A-01.5 Two restriction systems in Portico (per profile and per account), one in Jellyfin
- **Portico:** profile restrictions (`profile_restrictions`) and member limits (`access_limits`) are separate documents, edited by different people, stored differently, and both are compiled into the same catalogue predicate (`S/catalog/restrictions.go:88-143`, `S/httpapi/identity_restrictions.go:60`). They even use **different rating scales**: profiles use minimum age across seven systems; member limits use a fixed list of eleven US codes matched by exact text (`S/access/limits.go:18`).
- **Jellyfin:** one policy per user.
- **Why it matters:** an owner has two "highest rating" pickers that behave differently for the same title (see B-01.10). `api/admin-access.md` itself calls part of this area a pending merge.
- **Assessment:** **Portico behind** on coherence. The split of authority is sensible (owner limits an account; the account holder limits its own profiles), but both should use one rating engine and one label rule. Medium: retire the US ladder in `access`, store the member ceiling as an age like the profile ceiling, and keep the envelope shape.

### A-01.6 Ratings compared by "minimum age" from a small built-in table, versus per-country score tables
- **Portico:** seven systems (MPA, US TV, BBFC, FSK, CNC, ACB, plain age) plus about twenty extra spellings; any bare number up to 21 is read as an age; everything else is "unrated" (`S/identity/ratings.go`). Region qualifiers (`US:PG-13`, `de/FSK 16`) are stripped as a fallback. The country a rating came from is not stored, so the same letters always mean the same age.
- **Jellyfin:** 44 country files, each mapping every spelling used in that country to a score and optional sub-score; lookups try the item's metadata country, then a `CC:`/`CC-` prefix, then the US table (`jellyfin/Emby.Server.Implementations/Localization/LocalizationManager.cs:441-507`).
- **Why it matters:** letters collide across countries. `A` is "all ages" in Spain and "adults" in India and Canada; `M` is 15 in Australia and 16 in New Zealand; `R` is 17 in the US and 18 in Canada. Portico resolves collisions conservatively (the higher age wins, `ratings.go:126-133`), which is safe but hides suitable titles. Ratings Portico does not know (`14A`, `R13`, `R15+`, `AL`, `L`, `T`, `UA`, `Btl`, `TV-Y7-FV`) become unrated, so a kids profile with "allow unrated" off loses them and one with it on sees them regardless of age.
- **Assessment:** **Portico behind** on coverage, **ahead** on the idea: one ordered age scale is easier to explain than Jellyfin's scores. Adopting Jellyfin's data (the JSON files are GPL-2.0, compatible with Portico's GPL-3.0-or-later as far as I know, but check) or writing an equivalent table keyed by country would be small-to-medium, since `content_rating_ages` already caches the mapping per spelling. Storing the certification's country with the rating is the larger part (metadata domain).

### A-01.7 One "allow unrated" switch, versus per-type unrated blocking
- **Portico:** `allowUnrated` is a single boolean per profile (and another per account).
- **Jellyfin:** `BlockUnratedItems` lists item kinds: movies, trailers, series, music, books, Live TV channels and programmes, channel content, other.
- **Why it matters:** home videos, music and audiobooks almost never carry a rating. With one switch, a kids profile that blocks unrated films also loses the family's home videos and all music.
- **Assessment:** **Portico behind**. Small-to-medium: a per-library "treat unrated as suitable" flag would fit Portico better than per-type lists, and libraries already have kinds.

### A-01.8 Blocked labels only, versus blocked tags plus an allow list, inherited from parents
- **Portico:** a profile blocks **labels** (attribute field 2 only, `S/catalog/restrictions.go:98-105`). Member limits block labels **and tags** (fields 2 and 3, `restrictions.go:130-133`). There is no allow list. Episodes inherit a show's tags only when they have none of their own (`S/metadata/repair_fields.go:539-541`).
- **Jellyfin:** `BlockedTags` and `AllowedTags` (show only items carrying one of these), both matched against the item's tags plus all its parents' tags (`BaseItem.cs:1933-1942`).
- **Audiobookshelf:** a tag list per user that is either an allow list or a deny list.
- **Why it matters:** the allow list is the simplest strong kids mode there is: tag forty titles "kids" and nothing else exists for that profile, whatever the ratings say. Blocking by label needs the owner to label everything unsuitable.
- **Assessment:** **Portico behind**; and the label/tag asymmetry between the two systems looks accidental. Small-to-medium: one more `EXISTS` clause in the single predicate.

### A-01.9 No owner override of a title's rating
- **Portico:** the rating is whatever metadata says (an owner can edit `contentRating` in the metadata editor, which changes what is displayed too).
- **Jellyfin:** `CustomRating` is a separate field used only for parental comparison, leaving the official rating displayed (`BaseItem.cs:1876-1881`).
- **Why it matters:** "this PG-13 film is fine for my kids" or "this unrated documentary is not" without falsifying the displayed certificate.
- **Assessment:** **Portico behind** (minor). Small.

### A-01.10 Restrictions compiled into catalogue SQL, with one predicate for every surface
- **Portico:** one SQL fragment decides visibility for lists, counts, facets, home rows, search, detail and playback (`S/catalog/restrictions.go:15-20`). Containers (shows, albums) are visible when they hold at least one visible item. A restricted title opened by id answers "restricted", and counts are the filtered counts.
- **Jellyfin:** restrictions are fields on the item query and are translated into SQL too, but there are also in-memory checks (`IsVisible`, `IsParentalAllowed`) on other paths, and a history of surfaces that forgot one (search hints, "next up", images by id).
- **Why it matters:** a single enforcement point is the difference between "restricted" and "mostly restricted".
- **Assessment:** **Portico ahead**. Worth advertising. The weak spot is feature gating by URL prefix (A-01.12).

### A-01.11 Access schedules: per account and enforced at the door, versus per user and enforced on every request
- **Portico:** the schedule is part of the per-account limits, evaluated in a time zone stored with it (empty means UTC, `S/access/enforcement.go:299-327`). It is checked at sign-in, at token refresh and at the start of playback or a channel. A refusal at sign-in or refresh **revokes the session** (`S/httpapi/access_admission.go:61-83`). Browsing with a still-valid 15-minute token is not checked. No first-party screen edits schedules yet; the web member editor only preserves them (`W/screens/server/Access.tsx:31`).
- **Jellyfin:** schedules are per user, in the server's local time, checked by the authorization handler on essentially every request; administrators are exempt; a refusal is a plain 403 and the token stays valid for when the window reopens.
- **Why it matters:** Portico cannot schedule one child's profile without giving that child a separate account. Revoking on refusal turns "it's bedtime" into "re-enter the password tomorrow" on every device (B-01.1).
- **Assessment:** **Portico behind**. Medium: move the schedule to the profile (or allow it at both levels), refuse without revoking, and return a distinct code the apps show as "outside allowed hours".

### A-01.12 Feature switches enforced by URL prefix
- **Portico:** downloads, Live TV, DVR and Watch Together are refused by matching the request path (`/v1/downloads…`, `/v1/channels…`, `/v1/dvr…`, `/v1/groups…`) in `S/httpapi/restriction_admission.go:39-52`, plus one explicit check for channel playback.
- **Jellyfin:** each controller action names the permission it needs.
- **Why it matters:** a new route outside those prefixes that does the same thing is unguarded by default, and nothing fails when that happens.
- **Assessment:** **Portico behind** (design risk, not a found hole). Small: declare the feature on the route in the apikit registry so a guard test can assert coverage.

### A-01.13 Changing someone's permissions signs them out; Jellyfin applies changes live
- **Portico:** editing a member's libraries or disabling them revokes all their sessions and API keys and bumps the account epoch (`S/identity/direct_accounts.go:720-735`); changing a profile's restrictions, PIN or library list revokes that profile's sessions and remembered-device trust (`restrictions.go:301-309`, `direct_profiles.go:208-215`); a tier change does the same (`S/access/members.go:138-146`). Member **limits**, by contrast, apply live without revoking.
- **Jellyfin:** a policy edit takes effect on the next request; nobody is signed out (except on password change).
- **Why it matters:** granting a member one more library signs them out of every phone and TV. Tightening a child's rating limit signs the child's Apple TV out, and because the apps discard the account session after profile choice, getting back in needs the password or a new TV code. Every request already re-reads the live rules and the restriction "fence" already invalidates open cursors, so revocation is not needed for correctness.
- **Assessment:** **Portico behind** on convenience; deliberate (`api/identity.md` §3 "Changing restrictions"). I would keep revocation for disable, tier demotion and password change, and drop it for library and restriction edits. Small.

### Sessions, tokens and devices

### A-01.14 Short access token plus rotating refresh token, bound to a device, versus one permanent token
- **Portico:** 15-minute access tokens; refresh rotates both tokens; a sliding 90-day horizon; reuse of an old refresh token revokes the family; refresh also requires the installation id the family was bound to (`S/identity/refresh.go`, `families.go:164-169`).
- **Jellyfin:** one opaque token per user and device, valid until deleted. No expiry, no rotation.
- **Audiobookshelf:** recently moved to 1-hour access and 30-day refresh tokens, for the same reasons.
- **Why it matters:** a leaked Jellyfin token (logs, a shared screenshot of a stream URL, a backup) works forever. A leaked Portico access token dies in 15 minutes, and a stolen refresh token is detected the first time both parties use it. The cost is real complexity: clients must refresh correctly across tabs, app suspends and lost responses, and a mistake signs people out. The Backlog already records one unexplained sign-out.
- **Assessment:** **Portico ahead** on security, at a complexity cost Jellyfin never pays. Deliberate (`PC/identity/contract.md` §5).

### A-01.15 Tokens stored as digests; Jellyfin stores them in plain text
- **Portico:** only SHA-256 digests of access and refresh tokens are stored (`families.go:155-158`), API key secrets and invitation codes likewise. Exceptions are deliberate under "open data": refresh replay receipts and TV-code grants hold a whole session envelope as plain JSON for a short time (`quick_connect.go:106-110`), and TOTP secrets are plain columns.
- **Jellyfin:** `Devices.AccessToken` and API keys are plain text; a copy of the database is a set of working logins.
- **Assessment:** **Portico ahead**. One loose end: the plain receipts are meant to be cleared after five minutes, and that clean-up can stop running (B-01.5).

### A-01.16 Three kinds of credential with different reach
- **Portico:** an account session can manage but not play; a viewing session plays; a Top Shelf token reads one feed (`api/identity.md` §1).
- **Jellyfin:** one token does everything the user may do.
- **Why it matters:** a settings page left open cannot be used to browse as someone else, and the tvOS extension never holds a real session.
- **Assessment:** **Portico ahead**, with the caveat that in practice the apps throw the account session away after a profile is chosen, so account management happens through the primary profile's viewing session (`direct_accounts.go:328-332`).

### A-01.17 Every request re-checks the account, profile and device in the database
- **Portico:** disable, delete, epoch change, device denial and family revocation all take effect on the next request. A 15-second cache applies only to requests from outside the LAN and is invalidated by any authority write (`S/httpapi/router.go:618-626`, `principal_cache.go`).
- **Jellyfin:** also looks the token up per request and loads the user; similar immediacy.
- **Why it matters:** immediate revocation is good. The asymmetry is odd for performance: LAN requests (the majority, including every poster) never use the cache and pay four or five statements each, while remote ones do.
- **Assessment:** **Trade-off (subjective)**. The LAN exclusion appears to exist because of the recovery-owner private-network rule; worth revisiting in the performance pass (outside this domain).

### A-01.18 Idempotent refresh with a stored answer
- **Portico:** the client stores a request id before sending a refresh; if the reply is lost, the same request returns the same new tokens until the new ones are first used (`refresh.go:118-131`).
- **Others:** nothing comparable; Audiobookshelf rotates without replay, so a lost reply strands the client.
- **Assessment:** **Portico ahead**. This is the piece that makes rotation survivable on flaky mobile networks.

### A-01.19 What a client does when its token is refused
- **Portico:** a 401 on a normal request triggers one refresh and one retry. A **401 or 403** on the refresh itself is terminal: the stored sign-in is deleted and the app shows the sign-in screen (`C/server-connections.ts:253-258`). Network errors, timeouts and 5xx keep the credential and retry with backoff (`C/session-refresh.ts:156-169`).
- **Jellyfin clients:** a 401 returns to the login or user-select screen; the server entry and (on Android TV, Swiftfin, Streamyfin) the saved user remain, so signing back in is one password or Quick Connect.
- **Plezy:** 401/403 wipes that token, keeps the connection and shows a banner.
- **Why it matters:** treating a bare 403 as "your sign-in is over" is wider than Portico's own contract ("ambiguous failure is never terminal", `PC/identity/contract.md` §5). See B-01.1 and B-01.9.
- **Assessment:** **Portico behind** on this one edge; otherwise the transient/terminal split is more careful than any compared client.

### A-01.20 Signing out a family from any of its old tokens
- **Portico:** sign-out accepts an expired or already-rotated token of the same family (`families.go:405-439`), so an app that was offline for a week can still end its session.
- **Jellyfin:** sign-out needs the valid token (which never expires, so the case does not arise).
- **Assessment:** **Portico ahead** (a correct consequence of rotation).

### Passwords, brute force and second factors

### A-01.21 Cool-off instead of lockout
- **Portico:** after five failures a source waits 1, 2, 4 … up to 60 seconds; counts halve every 15 minutes; a device that has signed in before has its own bucket so strangers on the same network cannot delay it; an account-wide budget slows distributed guessing; the account is never disabled (`S/identity/credential_security.go:75-121, 281-346`). The owner gets an in-app security notice at the fifth failure.
- **Jellyfin:** after three failures (default) the account is disabled until an admin intervenes; no rate limit. Anyone who knows a username can lock that person out, including from the internet.
- **Why it matters:** Jellyfin's lockout is a denial-of-service lever and a support burden; Portico's design removes it. The price is that online guessing is bounded only by rate (roughly one guess a minute per network once warmed up), so weak passwords are the owner's risk.
- **Assessment:** **Portico ahead**; deliberate ("a correct password always signs you in", `02 Server.md`).

### A-01.22 No list of users before sign-in
- **Portico:** nothing reveals whether a username exists: identical errors, identical bcrypt work for unknown names and for password-less Portico members (`direct_accounts.go:113-133`), and the pre-login capabilities route takes no username. A browser can show "remembered accounts", but only for an installation id it already holds.
- **Jellyfin:** `GET /Users/Public` lists every non-hidden user with picture for the login screen; a disabled account's error names the account.
- **Why it matters:** Jellyfin's tile login is friendlier on a shared TV; Portico's is safer on the internet.
- **Assessment:** **Trade-off (subjective)**; Portico's choice suits a server that is port-forwarded by design. Deliberate (`api/identity.md` §2).

### A-01.23 bcrypt cost 10 with an 8-character minimum, versus PBKDF2 with 210,000 iterations and empty passwords allowed
- **Portico:** bcrypt cost 10 for passwords, 8 for PINs (`S/identity/hash_cost.go`, recorded as Justin's decision of 23 Sep), 72-byte cap, minimum 8 characters, no composition rules ("a weak password is the user's choice", `direct_accounts.go:81-87`). Old costs are upgraded on sign-in. Hashing is limited to two at a time; a third concurrent sign-in is told "busy" (`S/identity/service.go:125`, `direct_accounts.go:121-125`).
- **Jellyfin:** PBKDF2-SHA512, 210,000 iterations, re-hashed on login when parameters change; blank passwords are permitted and common for LAN-only users.
- **Why it matters:** both are acceptable. bcrypt cost 10 is on the light side by current guidance (OWASP suggests 10 as a floor) but reasonable for weak hardware. The two-slot limit protects a small box and is also a way for a flood of bogus sign-ins to make real ones answer "busy" (B-01.15).
- **Assessment:** **Trade-off (subjective)**. Requiring a password is the right default for a server meant to be reachable remotely.

### A-01.24 Built-in TOTP second factor on the server itself
- **Portico:** direct accounts can enrol TOTP with recovery codes; codes cannot be replayed within their window; five wrong tries retire a challenge (`S/identity/two_factor.go`, `api/identity.md` §9). Hosted accounts have their own.
- **Jellyfin:** none in the server; third-party plugins or an SSO plugin in front.
- **Assessment:** **Portico ahead**.

### A-01.25 Password reset: owner resets with forced change and a host-side recovery command, versus a PIN file
- **Portico:** an owner or admin sets a member's new password; the member must change it at next sign-in (`must_change`, `direct_accounts.go:736-741`). The owner recovers with `portico-server recover-owner [--reset-mfa]` on the host. Members have no self-service reset (the server sends no email). Portico Account passwords are reset at Hosted by email.
- **Jellyfin:** any user on the LAN can ask for a reset; the server writes a PIN to a file in its config folder; whoever can read the folder completes it. Admins can set or blank passwords; no forced change.
- **Assessment:** **Trade-off (subjective)**. Portico's is tidier and has the forced change Jellyfin lacks. Jellyfin's lets a household member recover without the admin retyping a password for them.

### A-01.26 Profile PINs are the only wall between profiles
- **Portico:** any session on an account, including a child profile's, may select another profile of that account; the only check is the target's PIN, if it has one (`direct_profiles.go:306-371`). PIN attempts back off per profile and device (2 s, then 30 s, 5 min, 15 min, 1 hour; reset after a day). A device can be "trusted" for a profile for 30 days and skip the PIN.
- **Jellyfin:** leaving a restricted user means signing in as another user with that user's password.
- **Plex:** same model as Portico (Home PIN), enforced at plex.tv.
- **Why it matters:** a kids profile is only contained if every adult profile on that account has a PIN. Nothing in the server requires or warns about that. Four digits with per-device backoff is fine against a child, not against someone determined.
- **Assessment:** **Trade-off (subjective)**; deliberate ("profile PINs are convenience locks", `PC/identity/contract.md` §9). A prompt when creating a restricted profile ("add a PIN to your own profile") would close the common gap (small, clients only).

### A-01.27 Switching profile creates a new session; the old profile's playback is ended on the server only if the client asks
- **Portico:** selecting a profile mints a new session family for that profile on the same device (`direct_profiles.go:379`). The server offers `POST /v1/direct/profile-switch` to end the outgoing profile's playback (`devices.go:581-636`), but no first-party client calls it.
- **Jellyfin:** switching user is logout then login; the old session ends with its token.
- **Assessment:** **Unclear**. Either the route is unnecessary because clients stop playback themselves, or the fence that `api/identity.md` §11 describes is not being applied. See the doc/code section.

### TV codes, setup and recovery

### A-01.28 TV code: durable, eight characters, approval on the server's own page
- **Portico:** follows the OAuth device-flow shape (device secret, user code, interval, `slow_down`), stores requests in the database, uses a 31-letter alphabet without look-alikes, and tells the TV to send people to the server's own `/device` page (`quick_connect.go`, `onboarding.go:250-255`). Approval raises an in-app "new device" notice. The TV gets the approving **profile**, with that profile's role.
- **Jellyfin:** six digits, kept in memory (lost on restart), entered inside any signed-in client; can be switched off server-wide.
- **Why it matters:** Portico's code space is about 2 × 10¹¹ against Jellyfin's 10⁶, and it survives restarts. Approving as a profile means a child can link a TV as themselves, and a parent's approval makes the TV the parent, which is the documented intent (`02 Server.md`).
- **Assessment:** **Portico ahead**. Two edge cases break it (B-01.5, B-01.6). There is no owner switch to turn TV codes off (F-01.15).

### A-01.29 First-run setup is open to whoever reaches the server first
- **Portico:** any browser may fetch the setup token and create the owner, from any network (`S/httpapi/onboarding.go:132-164`; comment cites Justin, 23 Sep). The origin check only stops another website driving a visitor's browser. Unused helpers `sameMachineSetup` and `localSetupHost` and a stale comment at `onboarding.go:59-60` survive from the earlier loopback-only design.
- **Jellyfin:** the same exposure: wizard endpoints are open until the wizard completes.
- **Plex (public docs):** an unclaimed server can only be claimed from the same machine or subnet, or with a claim token passed at install.
- **Why it matters:** on a home LAN this is harmless. On a VPS or a NAS with the port already forwarded, whoever finds the port first owns the server until someone notices. Jellyfin has the same weakness and it is a recurring complaint there.
- **Assessment:** **Trade-off (subjective)**, deliberate. A middle path that keeps "no setup code" for the normal case: allow open setup from private addresses, and from public addresses require either a token printed in the server log or a `PORTICO_SETUP_TOKEN` environment value. Small.

### A-01.30 A local recovery owner whose password only works from a private network
- **Portico:** for servers set up with a Portico Account, the recovery owner's password is refused from public addresses unless remote recovery was explicitly enabled; sessions that came through a Portico identity assertion are exempt (`S/identity/onboarding.go:198-230`).
- **Others:** nothing equivalent; Plex has no local fallback account at all, which is why a plex.tv outage locks owners out of their own settings.
- **Why it matters:** this is Portico's answer to "what if Hosted is down or gone". It is the main structural advantage over Plex's cloud dependence.
- **Assessment:** **Portico ahead**. The "private network" test here is a different function from the one used for remote-playback decisions (B-01.14).

### A-01.31 Device records keyed by an installation id; two device lists
- **Portico:** `identity_devices` is keyed by a random installation id the app generates once; it has approval states and "remember account" (`S/identity/devices.go`). A second table, `access_devices`, is keyed by an `X-Portico-Device-Id` header no first-party client sends (B-01.4).
- **Jellyfin:** `Devices` rows are (user, client device id, token); the device list is the session list; deleting a device revokes its token; a custom name can be set; `EnabledDevices` restricts a user to listed devices, with administrators always exempt (`DeviceManager.cs:237-249`).
- **Assessment:** **Trade-off (subjective)**. Portico's identity device record is richer and correctly separates "device" from "session". Having two inventories and two approval switches is a liability until they are merged, as `api/admin-access.md` already says.

### A-01.32 Session history keeps the reason a session ended
- **Portico:** revoked families keep `revoked_reason` and `revoked_at`, and the sessions list shows text such as "Signed out: sign-in was used twice" (`direct_accounts.go:449-466`).
- **Jellyfin:** an activity log records sign-ins and failures, not why a token died.
- **Assessment:** **Portico ahead** for diagnosing "why was I signed out". Revoked rows are never pruned (B-01.18).

### Integrations

### A-01.33 API keys: scoped and hashed in Portico but not usable; unrestricted and plain in Jellyfin
- **Portico:** the owner can create up to 100 keys with one of four scopes; secrets are stored hashed with a last-used time (`S/access/apikeys.go`). The request path does not accept them (B-01.3).
- **Jellyfin:** a key is a full administrator with no user, plain text, no scope, no expiry. It works, and a large ecosystem depends on it.
- **Audiobookshelf:** keys with expiry that act as a chosen user.
- **Assessment:** **Unclear** today (Portico's design is better; Jellyfin's exists). Finishing it is medium: resolve `pk_` bearers in `resolvePrincipal`, map routes to scope classes in the registry, decide whether keys bypass device binding.

### A-01.34 No path for third-party clients
- **Portico:** sign-in requires an installation id; tokens rotate; API keys do not authenticate; external contributions are not accepted (brief).
- **Jellyfin:** a documented header scheme and permanent tokens make third-party clients trivial, which is why Swiftfin, Streamyfin, Finamp, Wholphin and Plezy exist.
- **Assessment:** **Trade-off (subjective)**; presumably deliberate. Worth deciding explicitly whether automation tools (the *arr family, dashboards, home automation) are in scope; if so, working API keys are the minimum.

### A-01.35 Single sign-on: Apple and Google through Hosted only
- **Portico:** SSO exists only for Portico Accounts, at Hosted. A direct server has passwords and TOTP only.
- **Jellyfin:** pluggable authentication providers; LDAP is an official plugin; OIDC/SAML is a community plugin.
- **Audiobookshelf:** OpenID Connect built in. **Navidrome:** trusts a username header from a configured reverse proxy.
- **Why it matters:** power users who already run Authentik, Authelia or Keycloak expect to plug a media server into it.
- **Assessment:** **Portico behind** for that audience; conflicts mildly with "low ceremony" and with keeping one identity model. A generic OIDC option on the direct server would be medium-to-large. See F-01.9.

### Hosted, multiple servers and failure behaviour

### A-01.36 A global account that proves identity, while each server owns membership
- **Portico:** the Portico Account lives at Hosted. A server never asks Hosted "may this person in?"; it verifies a signed, five-minute, challenge-bound assertion of who the person is and then consults its own membership table (`S/hosted/identity_assertion.go`). Sessions issued afterwards are ordinary local sessions.
- **Jellyfin:** no global identity; every server is an island.
- **Plex (public docs):** plex.tv is the authority for both identity and sharing; servers validate tokens against it and cache.
- **Why it matters:** Portico gets Plex's "one login, my servers appear" without Plex's "the cloud decides who may watch my files". A Hosted compromise could impersonate people at sign-in but could not change who is a member or what they may see.
- **Assessment:** **Portico ahead**. This is the strongest structural idea in the domain.

### A-01.37 What breaks when things are down

| Situation | Portico | Jellyfin | Plex (public docs) |
|---|---|---|---|
| Hosted/cloud down, device already signed in | Works; refresh is local, 90-day sliding | Works | Usually works from cached auth |
| Hosted/cloud down, **new** device, global account | Cannot sign in (needs an assertion). A TV code approved from an already signed-in device works. | not applicable | Cannot sign in |
| Hosted/cloud down, direct/local account | Works | Works | not applicable (optional LAN no-auth list) |
| Internet down, on the LAN | Direct accounts and existing sessions work; Portico Account members cannot add a device | Works | Works only with cached auth or the no-auth list |
| Owner locked out and cloud down | Recovery owner on LAN, or `recover-owner` on the host | Reset PIN file on the host | No local fallback |
| Cloud account compromised, owner reacts in the cloud | Server sessions keep running until ended on each server (A-01.38) | not applicable | Central device sign-out |

- **Assessment:** **Portico ahead** of Plex and close to Jellyfin for anything already signed in. The gap is a Portico Account **member** (not owner) wanting to add a device while Hosted is unreachable: they have no password on the server, so only a TV code from another device helps. Deliberate (`PC/identity/contract.md` §2.4), though that section still describes an older offline-proof mechanism (see doc/code).

### A-01.38 Security actions at Hosted do not reach sessions on servers
- **Portico:** from the server side, the only identity signals Hosted sends are "this account left" and account-deletion erasure jobs (`S/hosted/membership_push.go` departures, `S/hosted/claim_cleanup.go`). I found no path by which a Portico Account password change, "sign out all devices" or 2FA reset at Hosted ends that account's sessions on its servers; those are local families that slide forward for 90 days.
- **Plex (public docs):** removing a device or changing the password with "sign out connected devices" invalidates tokens everywhere.
- **Why it matters:** if a Portico Account is taken over and the attacker opens a server, the victim's clean-up at Hosted leaves the attacker's server session alive until someone signs that device out on the server's own Devices page.
- **Assessment:** **Portico behind**, as a consequence of a deliberate boundary (servers own sessions). A cheap fix that respects "no chatter": Hosted already pushes departures on the server's next check-in; a per-account "security epoch" in the same answer would let the server revoke that account's Portico-admitted families. Medium (both sides). Confidence that the gap exists: probable, not certain, since I did not read Hosted's code.

### A-01.39 One signed-in server at a time on a device
- **Portico:** the protected store holds one server sign-in; the list of known direct servers holds addresses and names only, and "switching to a listed server signs in again" (`C/known-servers.ts:1-9`). With a Portico Account the switch is silent because a new assertion is minted.
- **Jellyfin clients:** keep a token per server and user; switching is instant (Android TV user grid, Swiftfin, Streamyfin saved accounts).
- **Assessment:** **Portico behind** for people with two direct servers or two direct accounts on one device; fine for Portico Account users. Medium (store shape and switcher).

### A-01.40 Invitations by code
- **Portico:** hashed one-time code, 7-day default (1 hour to 30 days), carries tier and libraries, optional email used only as a label and to prevent duplicates; a preview route shows a masked email and library count; a Portico Account can accept and is linked (`S/access/invitations.go`). The server sends no email.
- **Jellyfin:** no invitations; an admin creates the user and tells them the password (third-party tools like jfa-go fill the gap).
- **Plex:** invitation by email or username, delivered by Plex.
- **Assessment:** **Portico ahead** of Jellyfin.

### A-01.41 Remote sign-in is a server-wide switch
- **Portico:** `remoteSignInPolicy` is `allow`, `owner-only` or `off`, applied when a session is issued, not afterwards (`S/httpapi/access_admission.go:89-116`); existing remote sessions continue by design.
- **Jellyfin:** `EnableRemoteAccess` per user, checked on every request, plus a server-wide switch and an IP allow/deny list.
- **Assessment:** **Portico behind**: no per-person control, no IP filter, and the switch is not applied on the sign-in routes the apps use (B-01.2).

### A-01.42 "Remote" is decided in two different ways
- **Portico:** playback and remote-policy decisions use `requestIsRemote`, which honours the owner's `lanNetworks`, trusted proxies and "treat WAN as LAN" (`S/httpapi/request_locality.go`). Recovery-owner and setup decisions use `privateSetupPeer`, which only looks at whether the address is private, loopback or link-local and at proxies from the environment variable (`onboarding.go:18-45`).
- **Jellyfin:** one notion (`IsInLocalNetwork`) from one settings page.
- **Assessment:** **Portico behind** (B-01.14).

### A-01.43 Stream limits count streams, not signed-in devices
- **Portico:** `maxStreams` per account counts live playback sessions (`S/access/enforcement.go:105-122`); there is deliberately no cap on how many devices are signed in.
- **Jellyfin:** `MaxActiveSessions` counts sessions (signed-in clients), which blocks sign-in rather than the fourth stream.
- **Assessment:** **Portico ahead**; counting streams is what owners mean.

### Lifecycle, storage and client behaviour

### A-01.44 A member cannot be deleted, only disabled
- **Portico:** no route deletes a member account. The only code that does is erasure after a Portico Account is deleted at Hosted (`S/hosted/claim_cleanup.go:169-211`). A profile can be deleted: its watch state, history, playlists, saved items, search history, download requests and DVR rules are erased, recordings are kept, and the row becomes a "Deleted profile" tombstone (`direct_profiles.go:451-519`, `profile_erasure.go`).
- **Jellyfin:** deleting a user removes the row and cascades to preferences, permissions, schedules and watch data.
- **Why it matters:** an owner who removes a friend expects them gone from the People list and their data cleared. Disabled accounts accumulate, keep their usernames reserved and keep their history.
- **Assessment:** **Portico behind**. Small-to-medium: the erasure function exists; it needs an owner route and a confirmation that says what is erased.

### A-01.45 The owner's play history outlives the person
- **Portico:** `play_history` stores account id, profile id, `user_name` and `profile_name` (`migrations/0007_play_history.sql:11-18`) and is not touched by profile deletion or account erasure. Recorded decision: "the owner always sees every play" (`02 Server.md`).
- **Jellyfin:** watch data goes with the user; the activity log keeps names in text.
- **Assessment:** **Trade-off (subjective)**, deliberate, but in tension with the identity contract's erasure promise for Portico Accounts (B-01.12).

### A-01.46 Credential storage on clients
- **Portico:** web keeps the session in IndexedDB encrypted with a non-extractable AES-GCM key (plain JSON when the page is plain HTTP and has no WebCrypto, `W/bridge/server-connections.ts:6-11`); native apps use Keychain/Keystore; profile-trust tokens get the same treatment. Hosted account refresh secrets in the browser are HttpOnly cookies.
- **Jellyfin:** jellyfin-web keeps the token in `localStorage`; Android TV keeps a plain JSON file in app-private storage; Swiftfin uses the Keychain.
- **Plezy:** AES-GCM with the key in shared preferences (self-described obfuscation).
- **Assessment:** **Portico ahead**.

### A-01.47 Clients pin the server's identity key
- **Portico:** a session answer carries the server's public key and fingerprint; clients pin it and reject a refresh answer from a different key (`S/identity/service.go:72-77`, `C/server-connections.ts:267`).
- **Jellyfin:** nothing; a client trusts whatever answers at the address (TLS aside).
- **Assessment:** **Portico ahead**, especially because plain HTTP is first-class.

### A-01.48 Shared-TV behaviour
- **Portico:** TVs show the profile chooser at launch by default; PINs and 30-day trusted-device tokens decide friction (`PC/identity/contract.md` §2.3).
- **Jellyfin Android TV:** a setting picks "last user", "a specific user" or "always ask", plus "always authenticate" which never stores a token. **Swiftfin:** each saved user can demand Face ID/passcode or a local PIN on that device. **Streamyfin:** saved accounts with optional local PIN.
- **Assessment:** **Trade-off (subjective)**. Portico's server-side PIN is stronger than a client-side lock. The Jellyfin clients' explicit "which user does this TV open as" setting is a nice touch Portico does not appear to expose (not verified in the TV app).

### A-01.49 Remembered accounts for the browser are kept on the server
- **Portico:** the server stores, per installation id, up to ten account descriptors so a signed-out browser can show an account switcher (`S/identity/browser_accounts.go`). The list is read without authentication, with the installation id in the query string.
- **Jellyfin:** the browser keeps its own list; the server offers the public user list instead.
- **Assessment:** **Unclear** why this needs the server; the browser could keep names itself. It adds an unauthenticated read whose only protection is an id that also travels in URLs and is shown to every profile in the device list (B-01.17).

### A-01.50 Security notices inside the app, no email
- **Portico:** password change, sign-in cool-off, TV linked, owner recovery and 2FA reset raise in-app notifications (`credential_security.go:359-370`, `quick_connect.go:286-296`).
- **Jellyfin:** an activity log in the dashboard; plugins for webhooks/email.
- **Assessment:** **Trade-off (subjective)**; deliberate (no server email). There is no owner-facing list of sign-in events (F-01.12).

### A-01.51 Administration writes carry a revision and an operation id
- **Portico:** every access mutation states the revision it read and an idempotency key; stale writes get a conflict with the current revision (`api/admin-access.md` "Concurrency and retries").
- **Jellyfin:** policy updates are last-write-wins.
- **Assessment:** **Portico ahead**.

### A-01.52 Usernames
- **Portico:** lower-cased, 3 to 64 characters on most paths with a short list of forbidden characters; the rules differ between setup, owner-created members, invitations and self-registration (`onboarding.go:62`, `direct_accounts.go:619`, `S/access/invitations.go:350`, `self_registration.go:137`). No Unicode normalisation.
- **Jellyfin:** a single regex (letters, digits, a few punctuation marks) and case-insensitive uniqueness via a normalised column.
- **Assessment:** **Portico behind** on consistency (see doc/code); trivial to unify.

### A-01.53 Avatars
- **Portico:** uploads are sniffed, cropped, re-encoded to three PNG sizes, EXIF dropped, and reading one needs a session on the same account (`api/identity.md` §6).
- **Jellyfin:** user images are served by user id without authentication (needed for the login tiles).
- **Assessment:** **Portico ahead** on privacy.

## Missing features and capabilities to consider

Sizes are rough. "Conflict" names any Portico principle the feature rubs against.

### F-01.1 Per-person capability switches
- **What:** individual on/off permissions beyond tier: transcoding allowed, playback allowed at all (browse-only), delete media, manage subtitles, manage collections.
- **Who has it:** Jellyfin (`UserPolicy`, enforced per controller action); Emby; Audiobookshelf (download, update, delete, upload flags).
- **Value:** protects a weak server from one heavy remote user; lets a trusted member tidy subtitles or collections without becoming an admin.
- **Size / conflict:** small per switch (a field in the limits envelope and one admission check); cumulative UI and test cost; mild tension with "low ceremony".
- **Opinion:** add only "video transcoding allowed" and "may manage collections/subtitles" if asked; skip the rest.

### F-01.2 Remote access allowed per person
- **What:** this member may only sign in and play on the home network.
- **Who has it:** Jellyfin (`EnableRemoteAccess`, checked on every request).
- **Value:** children's and guests' accounts that must not work from outside; a cheap containment for a shared password.
- **Size:** small; `requestIsRemote` already exists. Needs checking on requests, not only at sign-in.
- **Opinion:** worth having; pairs naturally with fixing B-01.2.

### F-01.3 Time limits on a child's profile
- **What:** allowed hours per **profile** (Portico's schedule is per account, and has no editor yet), refused without signing the device out.
- **Who has it:** Jellyfin (per user, every request). Plex has none; this is a frequent request there.
- **Size:** medium (move or duplicate the schedule at profile level, editor on web and Apple, a distinct refusal the apps can explain).
- **Opinion:** high value for the family audience; see A-01.11 and B-01.1.

### F-01.4 "Only show titles tagged …" allow list
- **What:** a profile sees only titles carrying one of a few labels or tags, inherited from the show or album.
- **Who has it:** Jellyfin (`AllowedTags`), Audiobookshelf (tag allow list), Plex (label allow).
- **Size:** small-to-medium (one clause in the single predicate; container inheritance).
- **Opinion:** the most reliable kids mode; recommended.

### F-01.5 Owner's parental rating override per title, and unrated handling per library
- **What:** a "treat as" rating used only for restriction, and a way to say "unrated titles in Home Videos and Music are fine".
- **Who has it:** Jellyfin (`CustomRating`; `BlockUnratedItems` per kind).
- **Size:** small each.
- **Opinion:** the per-library unrated flag matters more; without it "block unrated" is unusable for mixed libraries.

### F-01.6 Rating tables for many more countries
- **What:** recognise local certificates (Canada's 14A/18A, New Zealand's R13/R16, Japan's R15+, Brazil's L/10/12/14/16/18, Netherlands' AL/6/9/12/16, and so on) and resolve ambiguous letters by country.
- **Who has it:** Jellyfin (44 country files).
- **Size:** small for spellings that are unambiguous; medium to carry the country with the rating.
- **Opinion:** needed before non-US households rely on restrictions.

### F-01.7 Delete a member
- **What:** remove an account and choose what happens to its data, instead of leaving it disabled forever.
- **Who has it:** everyone compared.
- **Size:** small-to-medium (erasure code exists).
- **Opinion:** expected basic function.

### F-01.8 Working API keys, and personal tokens
- **What:** a credential for scripts and other tools. Owner keys with scopes (already designed), and optionally per-member tokens limited to that member's rights.
- **Who has it:** Jellyfin (admin keys), Audiobookshelf (keys acting as a user, with expiry), Plex (the account token, unofficially).
- **Size:** medium (see A-01.33).
- **Opinion:** finish or remove; a screen that issues keys that do nothing is worse than no screen.

### F-01.9 Sign-in through the owner's own identity provider (OIDC), or LDAP
- **Who has it:** Audiobookshelf (OIDC built in, with auto-registration and group mapping), Jellyfin (LDAP plugin, community SSO plugin), Navidrome (reverse-proxy header).
- **Value:** homelab owners with Authentik/Authelia/Keycloak; one password and their own 2FA.
- **Size / conflict:** medium-to-large; adds a third identity path beside direct and Portico Account.
- **Opinion:** popular with exactly the "power user" audience named in the overview; worth an explicit yes/no.

### F-01.10 Trusting a reverse proxy's authenticated-user header
- **Who has it:** Navidrome (`ExtAuth.UserHeader` with a trusted-source list).
- **Size:** small given trusted proxies already exist; security-sensitive.
- **Opinion:** a cheap way to satisfy most of F-01.9's audience for web use; does nothing for native apps.

### F-01.11 Member self-service password reset on the home network
- **Who has it:** Jellyfin (PIN file read from the server's folder).
- **Size:** small.
- **Opinion:** low priority; the owner-reset-with-forced-change flow is cleaner. An owner-generated one-time reset link would be the Portico-shaped version.

### F-01.12 A sign-in and security activity list for the owner
- **What:** who signed in, when, from which address and device; failed attempts; cool-offs; devices linked; sessions revoked and why.
- **Who has it:** Jellyfin (activity log in the dashboard), Plex (device list with last-seen at plex.tv).
- **Size:** small-to-medium; the facts already exist across `identity_devices`, family revocation reasons and security notices.
- **Opinion:** valuable for a server that is exposed to the internet by design.

### F-01.13 Owner view of every member's devices and sessions
- **What:** today a device list shows only the caller's own account (`S/identity/devices.go:282-293`); the admin Devices page reads a table nothing fills (B-01.4). The owner cannot see or sign out a member's TV.
- **Who has it:** Jellyfin (Devices page lists all, delete to revoke), Plex.
- **Size:** small once the two device tables are merged.
- **Opinion:** needed; also the missing half of device approval (B-01.7).

### F-01.14 Limit on signed-in devices per member
- **Who has it:** Jellyfin (`MaxActiveSessions`).
- **Opinion:** low value; Portico's stream limit is the better control. Recorded for completeness; deliberate omission (`S/access/enforcement.go:72-73`).

### F-01.15 Owner switches for sign-in methods
- **What:** turn TV codes off, require a second factor for admins, turn password sign-in off for Portico-linked servers.
- **Who has it:** Jellyfin (Quick Connect toggle), Audiobookshelf (which auth methods are enabled).
- **Size:** small.
- **Opinion:** the TV-code toggle is cheap and some owners will want it.

### F-01.16 Per-person device allow list
- **Who has it:** Jellyfin (`EnabledDevices`), with administrators exempt.
- **Opinion:** low; the approve-new-devices policy covers the need once it works end to end.

### F-01.17 A nudge to protect adult profiles when a restricted profile exists
- **What:** when a profile gets restrictions, prompt for a PIN on the account's unrestricted profiles; optionally require the PIN to leave a kids profile.
- **Who has it:** Netflix-style products; Plex shows a warning for unprotected admins in a Home.
- **Size:** small, clients only.
- **Opinion:** closes the most likely real-world bypass (A-01.26).

### F-01.18 Per-profile app lock on a device
- **What:** require Face ID/passcode or a local PIN to open a saved profile on this phone.
- **Who has it:** Swiftfin (`LocalUserAccessPolicy`), Streamyfin.
- **Opinion:** nice-to-have; Portico's server PIN plus "don't trust this device" already covers it.

### F-01.19 Choose what a TV opens as
- **What:** "always open as this profile", "always ask", "never remember".
- **Who has it:** Jellyfin Android TV (`AuthenticationPreferences`: auto-login behaviour and "always authenticate").
- **Size:** small.
- **Opinion:** worth checking against the tvOS app; I did not verify what it offers.

### F-01.20 "Sign out of all my servers" from the Portico Account
- **What:** a Hosted-side action or security event that ends the account's sessions on every server at its next check-in.
- **Who has it:** Plex (central device revocation).
- **Size:** medium.
- **Opinion:** recommended; see A-01.38.

### F-01.21 Public or guest sharing
- **What:** a link that plays one title or playlist without an account; or a guest tier.
- **Who has it:** Navidrome and Audiobookshelf (share links with expiry), Audiobookshelf (guest user type). Jellyfin has only a vestigial flag.
- **Conflict:** the identity contract lists anonymous guests as a non-goal.
- **Opinion:** no action; recorded because it is a common request for music and audiobooks.

### F-01.22 Remote IP allow/deny list
- **Who has it:** Jellyfin (remote IP filter, allow or block mode).
- **Size:** small.
- **Opinion:** low; firewalls do this better.

### F-01.23 Keep several servers signed in on one device
- **Who has it:** every Jellyfin client; Plezy.
- **Size:** medium.
- **Opinion:** matters for direct-account users with more than one server (A-01.39).

## Possible bugs and risks in Portico

None of these were reproduced; all are from reading. "Latent" means the code path exists but no first-party screen I found reaches it today. None of them is listed in `Backlog.md` unless stated.

### B-01.1 An access schedule signs people out instead of pausing them
- **What happens:** a refresh outside the member's allowed hours rotates the tokens, then `admitSession` finds the schedule closed and **revokes the family** before answering 403 `access_limit`. The client treats any 401 or 403 on refresh as final and deletes its stored sign-in. So every device that wakes up outside the window (a phone opened at night, a TV left on) is signed out and needs the password or a new TV code the next day.
- **Evidence:** `S/httpapi/identity_routes.go:230`; `S/httpapi/access_admission.go:61-83`; `S/httpapi/access_area.go:148-150`; `C/server-connections.ts:253-258`. The schedule's empty time zone is evaluated as UTC (`S/access/enforcement.go:303-310`) while the web label calls it "server time" (`W/screens/server/Access.tsx:31`).
- **How the comparison surfaced it:** Jellyfin refuses each request during closed hours and leaves the token alone.
- **Confidence:** Confirmed by reading. **Severity:** Medium. Latent on the web (no schedule editor; the API accepts schedules).

### B-01.2 "Remote sign-in: owner only / off" is not applied on the sign-in routes the apps use
- **What happens:** `admitRemoteSignIn` is called from exactly one place, the legacy `POST /v1/sessions`. `POST /v1/direct/sign-in` (password and Portico Account), the two-factor completion, self-registration, the TV-code token route and profile selection never call it. An owner who sets the policy to `off` still accepts remote sign-ins from every first-party app, and anyone else can simply use the unguarded route. The TV-code token route and profile selection also skip the access-schedule check.
- **Evidence:** only caller at `S/httpapi/router.go:856`; `S/httpapi/direct_identity.go:105-110`; `S/httpapi/identity_routes.go:230, 250, 275`; `S/httpapi/onboarding.go:334-343`; clients sign in through `/v1/direct/sign-in` (`C/profile-management.ts:78`, `C/portico-servers.ts:87`).
- **How the comparison surfaced it:** Jellyfin enforces its per-user remote flag in the handler every request passes through.
- **Confidence:** Confirmed by reading. **Severity:** High (a security setting that silently does not hold).

### B-01.3 API keys can be created but never authenticate
- **What happens:** the web app issues keys (`W/screens/server/Access.tsx:115`), the server stores them, but no request path resolves a `pk_…` bearer. `access.Store.AuthenticateKey` and `ScopeAllows` are called only from tests; the HTTP layer says so itself: "accepts only a device-bound session family while legacy API keys are disabled".
- **Evidence:** `S/httpapi/access_area.go:85-99`; `S/access/apikeys.go:225` (no non-test caller); `S/httpapi/router.go:608-654` authenticates only session tokens. `api/admin-access.md` "API keys" describes them as working.
- **Confidence:** Confirmed by reading. **Severity:** Medium (advertised function does nothing; owners will paste keys into tools and get 401).

### B-01.4 "Require approval before a device can play" does nothing
- **What happens:** the `deviceApprovalRequired` setting is enforced only when a request carries an `X-Portico-Device-Id` header (`if q.DeviceID != "" && e.ApprovalRequired`). No first-party client sends that header (it appears nowhere in client-core, the web app or the native app), so the check never runs, and the admin Devices list it feeds stays empty. Even if clients sent it, a caller could omit it, or send another device's id: the record is re-assigned to whoever presents the id (`ON CONFLICT(id) DO UPDATE SET account_id=excluded.account_id`).
- **Evidence:** `S/httpapi/access_admission.go:146-147, 160`; `S/access/enforcement.go:96-104`; `S/access/devices.go:163-181`; setting at `S/operations/settings.go:110`.
- **How the comparison surfaced it:** Jellyfin ties device access to the device id recorded with the token at sign-in, not to a header on each request.
- **Confidence:** Confirmed by reading. **Severity:** Medium (false sense of security). The identity-side approval (`identity_devices.approval_state`) is the real mechanism and is enforced on every request; the two should be one.

### B-01.5 One approved-but-never-collected TV code stops identity clean-up for good
- **What happens:** `PruneSetup` selects `token_hash` from TV-code rows with `status='approved'` that have expired and scans it into a Go `string`. Since sessions are now issued when the TV collects (not when the phone approves), an approved row always has a NULL `token_hash`, and scanning NULL into a string is an error. The function returns at that point, every time, because the row is never changed; the caller discards the error. Everything after it in the function stops running: releasing old codes, clearing stored session envelopes (`grant_cipher`) after their five-minute recovery window, clearing the setup receipt, deleting old TV-code rows and old rate-limit rows.
- **When:** someone approves a code and the TV never polls again (app closed, network lost, or B-01.6).
- **Evidence:** `S/identity/quick_connect.go:438-449` (scan), `:299` (approval sets no token), `:586` (token set only together with `status='consumed'`), `:498` (error ignored). No test calls `PruneSetup`.
- **Consequence:** plain-JSON session envelopes (including a first-generation refresh token) stay in the database indefinitely instead of five minutes; two small tables grow without bound.
- **Confidence:** Confirmed by reading (driver behaviour for NULL into `string` is standard `database/sql`). **Severity:** Low to Medium.

### B-01.6 A TV code cannot complete when new devices need approval (latent)
- **What happens:** with the `owner-approves-new-devices` policy, collecting an approved TV code tries to register the TV, finds it `pending`, returns an error, and the whole transaction rolls back, including the pending device record. The owner therefore never sees a device to approve, the TV polls forever, and when the code expires the row is the exact case that triggers B-01.5. Password sign-in handles the same situation correctly by committing the pending record.
- **Evidence:** `S/identity/quick_connect.go:360-365` and `:568-571`; `S/identity/issuing_device.go:56-59`; `S/identity/devices.go:193-211`; contrast `S/identity/direct_accounts.go:281-284`.
- **Confidence:** Confirmed by reading. **Severity:** Medium, latent (no first-party screen sets the policy; `PUT /v1/direct/registration-policy` does).

### B-01.7 The approve-new-devices policy can lock the owner out, and members' devices cannot be approved (latent)
- **What happens:** (a) The policy applies to every account including the owner; there is no exemption. An owner with the policy on and no remaining approved, signed-in device cannot approve their own new device, and `recover-owner` resets the password but not the policy or the device, so it does not help. (b) The device list route returns only the caller's own account's devices, and the admin list reads the other table, so an owner has no way to discover a member's pending device id in order to approve it.
- **Evidence:** `S/identity/devices.go:193-196` (no role check), `:282-293` (own account only), `:401-455` (approval needs the id); `S/identity/recover_owner.go:28-86`.
- **How the comparison surfaced it:** Jellyfin always exempts administrators from device restrictions (`jellyfin/Jellyfin.Server.Implementations/Devices/DeviceManager.cs:242`).
- **Confidence:** Confirmed by reading. **Severity:** Medium, latent (same policy as B-01.6).

### B-01.8 `hostedAttached` is always false in release builds
- **What happens:** `/v1/system.hostedAttached` and `/v1/auth/capabilities.hostedAttach` come from `identity.Service.Hosted()`, which tests whether the `policy` table has a row. Since membership moved to the server ("migration 0120"), the only code that writes that table is a file excluded from release builds. A server's own web page therefore always concludes "this server signs people in directly" and opens on the direct form; the invitation join page concludes the server is not a Portico server.
- **Evidence:** `S/identity/pin_recovery.go:57-61`; only writer `S/hosted/legacy_policy_development.go:1` (`//go:build !release`) and `:55`; `S/httpapi/router.go:812-817`; `W/screens/auth/SignIn.tsx:46`; `W/screens/auth/DirectJoin.tsx:70`.
- **Confidence:** Probable (I did not confirm what the web page shows for a claimed server; the hand-off stays reachable through the form's "use a Portico Account" link). **Severity:** Medium for servers set up with a Portico Account; invisible on the demo, which is direct.

### B-01.9 Any 403 on refresh deletes the stored sign-in
- **What happens:** the client decides a refresh was refused from the HTTP status alone. A 403 that did not come from Portico (a reverse proxy, a web application firewall, a filtering gateway on a guest network) would delete a perfectly good sign-in.
- **Evidence:** `C/server-connections.ts:253-258`. The identity contract says ambiguous failures are never terminal (`PC/identity/contract.md` §5).
- **Confidence:** Confirmed code path; whether a real deployment produces such a 403 is Speculative. **Severity:** Low to Medium. A fix is to require Portico's error envelope and a known code before discarding credentials.

### B-01.10 The two "highest rating" controls disagree
- **What happens:** the per-account limit compares the title's rating text, upper-cased, against eleven US codes in one mixed order: TV-Y, TV-Y7, G, TV-G, PG, TV-PG, PG-13, TV-14, R, TV-MA, NC-17. Consequences: a limit of **PG hides TV-PG** shows and **PG-13 hides TV-14**; any non-US or qualified rating (`12`, `15`, `FSK 12`, `US:PG-13`) counts as "unrated" and is shown unless unrated is blocked; and it looks only at the first stored rating while the profile rule looks at all of them. The per-profile control uses ages across systems and would treat the same titles differently.
- **Evidence:** `S/access/limits.go:18`; `S/access/enforcement.go:212-229, 231-270`; `S/catalog/restrictions.go:106-129`; profile rule at `S/catalog/restrictions.go:91-97`.
- **Confidence:** Confirmed by reading. **Severity:** Medium (the per-account control is the one the web app's People page exposes).

### B-01.11 Some common rating spellings are read as "unrated"
- **What happens:** `ratingKey` keeps hyphens, so `TV-Y7-FV` becomes `TV-Y7-FV`, which is not in the table (the table has `TVY7FV`, which only matches a hyphen-less spelling). The entry `M/PG` can never match because slashes are stripped before lookup (it still resolves, by falling back to `PG`). Certificates outside the seven systems are unrated (A-01.6). `A` is always 18, which hides Spanish all-ages titles.
- **Evidence:** `S/identity/ratings.go:101-109, 139-141`.
- **Confidence:** Confirmed by reading. **Severity:** Low (errs towards hiding when unrated is blocked, towards showing when it is allowed).

### B-01.12 Erasing a person leaves their name in the owner's play history
- **What happens:** profile deletion and Portico Account erasure clear personal state but not `play_history`, which stores the account id, profile id, user name and profile name with every play. The identity contract promises that server erasure "removes or anonymizes viewer state" for a deleted Portico Account.
- **Evidence:** `S/identity/direct_profiles.go:497-518`; `S/hosted/claim_cleanup.go:169-211`; `S/persistence/migrations/0007_play_history.sql:11-18`; `PC/identity/contract.md` §9; decision "the owner always sees every play" in `02 Server.md`.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (two written rules conflict; a privacy promise to account holders is involved). Anonymising the name columns on erasure would satisfy both.

### B-01.13 Invite-only self-registration can never succeed (latent)
- **What happens:** `invite-only` mode calls an invitation redeemer that nothing installs, so every attempt is refused, while the capabilities document still lists `self-registration` as a method. The redeemer's signature also has no transaction, so if one were installed it would consume the invitation before the username check and lose it on a name clash.
- **Evidence:** `S/identity/self_registration.go:145-148`; field declared at `S/identity/service.go:101-103` and never assigned outside tests; `S/identity/pin_recovery.go:149-151`. Invitations work through `POST /v1/access/invitations/accept` instead.
- **Confidence:** Confirmed by reading. **Severity:** Low, latent.

### B-01.14 Two different tests for "is this request local?"
- **What happens:** the recovery-owner rule, setup and the choice to bypass the sign-in cache use `privateSetupPeer`: any private, loopback or link-local address is local, and only proxies named in the environment variable are believed. Remote-playback decisions use `requestIsRemote`, which honours the owner's `lanNetworks` and `trustedProxies` settings. An owner who marks a guest network as not-LAN, or adds a trusted proxy in settings, changes one and not the other. In container setups where all traffic arrives from a private gateway address (rootless Docker, some NAS bridges), every remote visitor looks private to the first test, which defeats "recovery owner only from the private network".
- **Evidence:** `S/httpapi/onboarding.go:18-45`; `S/httpapi/request_locality.go:64-101`; uses at `S/httpapi/router.go:614`, `direct_identity.go:95`, `owner_authority.go:52`.
- **Confidence:** the inconsistency is Confirmed by reading; the container exposure is Speculative. **Severity:** Medium for servers set up with a Portico Account behind such networking.

### B-01.15 Two hashing slots, no queue
- **What happens:** at most two bcrypt operations run at once; a third sign-in, PIN check or password change is refused immediately with "busy". Unknown usernames cost a full hash too. On slow hardware a steady trickle of bogus sign-ins from one network (the per-network limit is 1,280 a minute) can keep both slots occupied so real people see "Sign-in is busy".
- **Evidence:** `S/identity/service.go:125`; `S/identity/direct_accounts.go:121-125`; `S/httpapi/rate.go` constants.
- **Confidence:** Speculative (depends on hardware and timing). **Severity:** Low to Medium. A short wait for a slot instead of an instant refusal would blunt it.

### B-01.16 Profile PIN is checked while holding the database's single writer
- **What happens:** `SelectDirectProfileFrom` opens a write transaction and then runs bcrypt on the PIN inside it. The code elsewhere states that bcrypt "must never hold SQLite's one writer" and proves passwords outside the transaction.
- **Evidence:** `S/identity/direct_profiles.go:300-345`; rule stated at `S/identity/credential_security.go:224-226`.
- **Confidence:** Confirmed by reading. **Severity:** Low (cost 8 is tens of milliseconds, longer on a weak box; every other write waits).

### B-01.17 The installation id is treated as a secret but is shown and sent in URLs
- **What happens:** the server calls the installation id "the only thing standing between a stranger and this installation's remembered-account list" and requires it on refresh. Yet the device list returns every device's installation id to any profile on the account, including a restricted one, and the remembered-accounts routes carry it in the query string, where proxies and access logs record it. The web app's own comment calls it non-secret.
- **Evidence:** `S/identity/devices.go:37-42, 83-86, 282-308`; `S/httpapi/identity_routes.go:170-180, 198-200`; `W/bridge/profile-trust.ts:4`.
- **Confidence:** Confirmed by reading. **Severity:** Low (what leaks is a list of names; refresh still needs the refresh token).

### B-01.18 Ended session records are never deleted
- **What happens:** revoked and expired session families stay in `authorization_session_families` forever (their tokens are pruned). Every sign-in, profile choice and TV link adds one. The per-account failed-attempt ledger is also unpruned.
- **Evidence:** no `DELETE FROM authorization_session_families` anywhere; `S/identity/prune_sessions.go`; indexes `authorization_families_retention` and `…_owner_history` suggest a retention rule was intended.
- **Confidence:** Confirmed by reading. **Severity:** Low (slow growth; the sessions list also returns every revoked family ever).

### B-01.19 Some revocations skip the playback clean-up hook
- **What happens:** the service routes most revocations through `OnFamilyRevokedTx` so queues, controllers and readers retire. Four paths revoke without it: refresh-token reuse, an access-schedule or remote-policy refusal, a tier change, and Portico Account erasure. A database trigger still stops legacy playback rows, and the next request from the revoked token is refused, so the likely leftover is v1 playback sessions that count against a stream limit until their lease expires.
- **Evidence:** `S/identity/refresh.go:45`; `S/httpapi/access_admission.go:76, 110`; `S/access/members.go:141`; `S/hosted/claim_cleanup.go:182`; hook description at `S/identity/direct_accounts.go:382-383`.
- **Confidence:** Probable. **Severity:** Low.

### B-01.20 Refusals that answer 401 where the project's own rule says 403
- **What happens:** the service documents that a valid session being refused must be 403, because clients treat 401 as "your sign-in ended". Several administrative refusals still return the 401 error: a non-admin editing a member, an admin editing a peer, approving a device without rights, a bad ownership-transfer target, changing one's own tier, inviting above one's tier.
- **Evidence:** rule at `S/identity/service.go:20-31`; `S/identity/direct_accounts.go:697, 705, 814, 821`; `S/identity/devices.go:416, 424, 431`; `S/access/members.go:116, 133`; `S/access/invitations.go:167`.
- **Confidence:** Confirmed by reading; client reaction not traced. **Severity:** Low.

### B-01.21 A restricted profile can do account-level things
- **What happens:** routes that do not require the primary profile accept any profile's session: listing devices with their IP addresses, renaming devices, turning "remember this account" off, forgetting remembered accounts, and approving a TV code (the TV then signs in as that profile). A restricted profile can also switch to any profile without a PIN (A-01.26).
- **Evidence:** `S/identity/devices.go:289, 332`; `S/identity/browser_accounts.go:138`; `S/identity/quick_connect.go:229`.
- **Confidence:** Confirmed by reading. **Severity:** Low (nuisance, mostly by design).

### B-01.22 Left-over code from earlier identity designs
- **What it is:** a whole "hosted" session authority that can no longer be issued but is still branched on in about twenty places (where it means "no restrictions at all", `S/identity/restrictions.go:180-181, 196-198`); `viewer_only` grants that are only ever inherited and never created (`S/identity/quick_connect.go:278, 573`); unused setup helpers (`S/httpapi/onboarding.go:61, 79`); legacy `pin_attempts` columns; three retired Hosted routes that answer 410.
- **Why it is a risk:** dead branches that fail open are the kind of thing a later change accidentally re-enables.
- **Confidence:** Confirmed by reading. **Severity:** Low. Worth a clean-up pass before the architecture is frozen.

### B-01.23 Related Backlog item: two Apple sign-ins ended with `explicit_signout`
- `Backlog.md` (Server) records two simulator sign-ins ending after a demo restart with `reason=explicit_signout` and no `refresh_reuse`. I could not find the cause, but can narrow where to look: that reason is written only by the sign-out route, device sign-out/forget, "sign out everywhere", the unused profile-switch fence, Cast session replacement, a cancelled or expired TV code, and the clean-up of the temporary account session after a profile is chosen (`N/app/session.tsx:597, 610`). The last one runs on every sign-in and is the only one the apps call without the person asking, so I would check first whether it can be handed the viewing session's token instead of the account session's (for accounts where sign-in returns a viewing session directly).
- **Confidence:** Speculative. **Severity:** as Backlog.

## Where Portico is stronger

- **Membership stays on the owner's server.** Hosted proves identity with a short-lived signed assertion; it cannot grant or widen access (A-01.36). Plex cannot say this.
- **It keeps working without the cloud** for everyone already signed in, and the owner has two local ways back in (A-01.30, A-01.37).
- **Tokens are short-lived, rotated, device-bound and stored as digests**, with reuse detection and a replay-safe refresh (A-01.14, A-01.15, A-01.18). Jellyfin's permanent plain-text tokens are its weakest point.
- **No lockout, no user enumeration, no public user list** (A-01.21, A-01.22). Jellyfin's lockout lets anyone disable anyone.
- **Restrictions are enforced in one SQL predicate** shared by lists, counts, search, detail and playback (A-01.10).
- **Built-in TOTP with recovery codes** on the server itself (A-01.24), and forced password change after an owner reset (A-01.25).
- **TV codes that survive restarts**, with a far larger code space and an approval page on the server's own origin (A-01.28).
- **Invitations** with tier and library grants (A-01.40).
- **Profiles under an account**, with per-profile restrictions, PIN backoff per device and time-limited device trust (A-01.1, A-01.26).
- **Stream limits that count streams**, not sign-ins (A-01.43).
- **Clients pin the server's key and keep credentials in protected storage** (A-01.46, A-01.47).
- **Every admin change is revision-checked and idempotent** (A-01.51).
- **Sessions remember why they ended**, which makes "why was I signed out" answerable (A-01.32).

## Doc/code disagreements

| # | Document says | Code does |
|---|---|---|
| D1 | `PC/identity/contract.md` §4.1: passwords need upper case, lower case and a digit or symbol | at least 8 characters, nothing else (`S/identity/direct_accounts.go:81-87`; the comment says a weak password is the user's choice) |
| D2 | contract §3.1: usernames 3–32 characters with a strict pattern | 3–64 with a short forbidden-character list; self-registration accepts 1 character; rules differ on four paths (A-01.52) |
| D3 | contract §9: profile PIN bcrypt cost 10 | cost 8 (`S/identity/hash_cost.go`, citing Justin, 23 Sep) |
| D4 | contract §2.4: offline profile proofs bound to PIN revision, 30-day expiry, encrypted under platform material | removed; `POST /v1/hosted/profiles/offline-select` answers 410 (`S/httpapi/router.go:890-894`); profiles are local to the server |
| D5 | contract §1 and §5: Hosted is the authority for Hosted profiles and issues a one-time bootstrap envelope the server exchanges | Hosted issues an identity assertion only; "Hosted keeps no profiles for a server" (`S/access/invitations.go:460-466`); membership and profiles are the server's |
| D6 | contract §2.1: a Portico Account has a primary profile counted within eight profiles | profiles exist per server account; the eight-profile limit is per server account |
| D7 | `api/identity.md` §7: "Sign out everywhere requires the account password" | no password; the session is the authority (`S/identity/devices.go:558-579`; `C/identity-client.ts` comment "C45: no password") |
| D8 | `api/identity.md` §7: device routes under `/v1/direct/devices` | routes are `/v1/devices…` (`S/httpapi/device_contract.go`) |
| D9 | `api/identity.md` §1 says the Top Shelf token "lives thirty days"; §12 says "a 24-hour feed token" | maximum 30 days (`S/identity/topshelf.go:33`) |
| D10 | `api/identity.md` §11: clients call `POST /v1/direct/profile-switch` before selecting a new profile | no first-party client calls it |
| D11 | `api/identity.md` §10: invite-only self-registration works once a redeemer is installed | none is installed (B-01.13) |
| D12 | `api/identity.md` §7: "Generate 256 bits of randomness" for the installation id, which is a secret | the web app uses a UUID (122 random bits) in `localStorage` and calls it non-secret (`W/bridge/profile-trust.ts:4-7`) |
| D13 | `api/admin-access.md` "API keys": a key authenticates as a bearer token | keys are not accepted (B-01.3) |
| D14 | `api/admin-access.md` "Connectivity": `remoteSignInPolicy` is applied after a sign-in succeeds | only on `POST /v1/sessions` (B-01.2) |
| D15 | `api/admin-access.md` "Devices": recorded from `X-Portico-Device-Id`; a "merge note" says the identity device table is still to come | the identity table exists and is authoritative; no client sends the header (B-01.4) |
| D16 | `api/admin-access.md` "Access tiers": nothing else in the server compares role strings | direct comparisons remain, for example `S/identity/direct_accounts.go:594, 642, 810`, `S/identity/devices.go:384`, `S/httpapi/owner_authority.go:19` |
| D17 | `api/admin-access.md`: an admin may manage "any account below the admin tier" | creating a member and listing members are owner-only (`direct_accounts.go:594, 642`); editing is admin-or-owner |
| D18 | `02 Server.md`: "A session that is itself viewer-only links a viewer-only device" | viewer-only grants can only be inherited from rows that nothing creates any more (B-01.22) |
| D19 | code comment `S/httpapi/onboarding.go:59-60`: the setup token convenience is only for a loopback browser | any browser on any network gets it (`onboarding.go:141-164`) |
| D20 | code comment `S/httpapi/router.go:812-815`: `hostedAttached` means the server holds Hosted policy | no release code writes that policy (B-01.8) |
| D21 | principle "no time-based caches of derived results" | the sign-in answer is cached for up to 15 seconds for non-LAN requests, though also keyed by an authority generation (`S/httpapi/principal_cache.go`) |
| D22 | `W/screens/server/Access.tsx:31` labels an empty schedule time zone "server time" | the server evaluates an empty time zone as UTC (`S/access/enforcement.go:303-310`) |
| D23 | `C/server-connections.ts` comment: a lost refresh answer can be replayed for "five minutes" | the server keeps the answer until the new tokens are first used, up to the family's horizon (`S/identity/refresh.go:118-131`) |

## Open questions for Justin

1. **Remote sign-in policy (B-01.2):** should `owner-only` / `off` apply to every way of getting a session, and should it also be checked on requests (so an already signed-in remote device is stopped), or only at sign-in as the setting text says?
2. **API keys (B-01.3, A-01.34):** are scripts and third-party tools in scope for release? If yes, finish the keys; if no, remove the screen.
3. **Device approval (B-01.4, B-01.6, B-01.7):** keep one mechanism (the identity one), exempt the owner, and give the owner a list of all pending devices? Or drop approval entirely for the first release?
4. **Access schedules (A-01.11, B-01.1):** per profile, per account, or both? And should a closed window merely refuse rather than sign out?
5. **Sign-out on permission change (A-01.13):** is it intended that granting a library or changing a child's rating signs that person out everywhere?
6. **Rating engine (A-01.5, A-01.6, B-01.10):** one age-based engine for both levels? Adopt country tables? Store the certificate's country?
7. **Unrated titles (A-01.7):** should "block unrated" be per library so home videos and music are not swept up?
8. **Allow-list kids mode (A-01.8 / F-01.4):** wanted?
9. **Deleting a member (A-01.44):** add it, and what should happen to their playlists shared with others and their recordings?
10. **Play history and erasure (B-01.12):** when a person or Portico Account is erased, keep their plays with the name, or keep the plays and blank the name?
11. **First-run setup from the internet (A-01.29):** keep fully open, or require a log-printed token when the first request comes from a public address?
12. **Hosted security events (A-01.38):** should a Portico Account password change or "sign out everywhere" end sessions on the account's servers at their next check-in?
13. **Members while Hosted is down (A-01.37):** is "no new devices for Portico Account members until Hosted returns, except by TV code" acceptable, or should a member be able to set an optional server password?
14. **Profile containment (A-01.26):** is a PIN on the adult profiles enough, and should the apps prompt for one when a restricted profile is created?
15. **Own identity provider (F-01.9, F-01.10):** is OIDC or proxy-header sign-in for direct servers in or out?
16. **Per-person switches (F-01.1, F-01.2):** which, if any: remote access, transcoding, browse-only?
17. **Several servers signed in at once (A-01.39):** needed for direct accounts?
18. **Identity contract:** it still describes the pre-"0120" Hosted model in several sections (D4–D6). Rewrite it to the current design before freezing?
19. **TV codes:** should there be an owner switch to turn them off, and should a restricted profile be allowed to approve one?
20. **Password rules (D1):** confirm "8 characters, no composition rules" is the decision, and update the contract.

## Evidence boundary

- **Nothing was executed.** No server, client, test or build was run. Every bug is a reading of source; none was reproduced.
- **Runtime behaviour not established:** what the apps actually show for 401/403 in each state; whether the web sign-in page misroutes on a claimed server (B-01.8); real bcrypt timings on weak hardware (B-01.15, B-01.16); whether any deployment produces a non-Portico 403 (B-01.9); container address behaviour (B-01.14).
- **Hosted's Go code was not read.** Statements about what Hosted sends or does come from its documents and from the server-side handlers. A-01.38 in particular needs confirming against Hosted.
- **Native apps:** the session model in `N/app/session.tsx` was read; Keychain/Keystore modules, the tvOS profile chooser and the Android, Fire TV and Vega ports were not examined. "No first-party screen sets X" was established by searching the web app, client-core and the React Native `src` tree for the route names; a screen built another way would have been missed.
- **Jellyfin** was read at the repository's current master; released versions may differ (for example the default lockout count and the legacy-authorization switch).
- **Plex and Emby** claims are from public documentation and general knowledge, not from code, and were not re-verified on the web during this review.
- **Licensing** of Jellyfin's rating tables for reuse was not checked.
- **Performance** of per-request authorization was not measured.

## Outside my domain

- **Certification selection (metadata):** a title gets one certificate, from the library's region or else the first any country supplies, stored without its country (`S/metadata/publication_fields.go:55-80`). That limits what parental controls can do (A-01.6).
- **Per-request authorization cost on the LAN (performance):** the sign-in cache is skipped for private addresses (`S/httpapi/router.go:620`), so every LAN request, including artwork, pays the full database check.
- **Playback admission takes the write gate** on every start to record the device sighting (`S/httpapi/access_admission.go:140-165`), even though nothing sends the device id.
