# Hosted account security

This source implements verified registration, password/provider sign-in, recovery,
contact changes, TOTP/recovery codes, provider management, deletion previews and
account mail. It uses the existing account/session-family authority. It does not
implement invitation administration, server claims, DNS, certificates or media.

Almost everything described here is covered by tests that need a PostgreSQL
database and that skip silently without one. `TESTING.md` explains how to point
them at a throwaway database; `dev/test-db.sh` does it for you. Run them before
trusting a change to sign-in, sessions, claims, membership, deletion or departure.

## Configuration and custody

Use the declared Go 1.26.2 toolchain. Existing startup and HTTPS edge requirements
remain in force. Add `--smtp-config-file` and `--oidc-config-file` to the normal
Hosted invocation. Examples are in `config/account-*.example.json`; copy them to
operator-owned private files and replace the example values. They are not usable
credentials and must not be installed unchanged.

SMTP configuration and its password file must be regular files with no group or
other permission bits (for example, mode 0600). Choose `starttls` for a mandatory
verified upgrade, or `implicit` for TLS from connection establishment. There is
no plaintext or insecure-certificate option. Authentication inputs are paired;
when using a trusted authenticated-by-network submission service, omit both
`username` and `passwordFile`, not just one. The configured sender must be a bare
mail address. TLS verifies the SMTP host with the system trust store and requires
TLS 1.2 or newer. Each delivery has a 5-second dial and 20-second total deadline.

OIDC configuration and the Apple signing key must be private. The Apple key is a
PKCS#8 P-256 private key. The Google secret stays server-side; Apple client secrets
are generated server-side from the configured Team ID, Key ID and private key.
Remove all fields for an unused provider rather than leaving fake credentials.
Native Apple requires the web Apple configuration plus the exact bundle IDs in
`appleNativeClientIds`. The older singular `appleNativeClientId` is also accepted
and normalized into that list. Changing the provider descriptor invalidates
in-flight attempts; it does not reinterpret an old assertion under a new audience.

Persist and protect the existing `--key-file`. Its sealing key protects pending
mail payloads, challenge receipts, pending TOTP secrets and native/OIDC
continuations. Losing or replacing it makes those encrypted records unreadable;
this change does not add an automatic key-rotation service. Keep database/key
backups consistent and coordinate rotation with the existing security owner.
Never log SMTP passwords, raw ID tokens, refresh credentials, email/recovery
codes, provider callback bodies or private continuation headers.

Missing SMTP configuration disables only operations requiring outbound account
verification/recovery mail. Already usable password/provider accounts still sign
in. Security notices are durable and can await SMTP configuration until their
expiry. Invalid supplied configuration is a startup error, not a pretend-success
mail transport. `/v1/identity/options` reports actual configured capability.

## Public origins and callbacks

The canonical account web origin is `https://web.getportico.tv`. Configure exactly:

* Google redirect: `https://web.getportico.tv/auth/sso/google/callback` (GET).
* Apple web return: `https://web.getportico.tv/auth/sso/apple/callback` (form POST).
* Device approval: `https://web.getportico.tv/device`.

Route both callback paths through the existing HTTPS edge to Hosted, without
logging query strings or Apple POST bodies. The final web redirect contains only
an opaque transaction ID. Native browser return is the claimed HTTPS app link
`https://web.getportico.tv/app/account-return#tx=<opaque-id>&code=<completion-code>`;
it contains no assertion, authorization code, refresh token or account bearer
token. The app must present both the completion code (which only the browser
where the person signed in receives) and its independently generated private
continuation binding. Every native sign-in through the browser requires the code.

Include the canonical web origin and each approved media-web origin in the
existing explicit `--origins` allowlist. Do not replace that list with a wildcard.
Use the existing trusted-proxy configuration only for sanitized trusted peers.
Set `VITE_HOSTED_URL` to the actual HTTPS Hosted API origin. Canonical web is
recognized by its hostname; `VITE_ACCOUNT_WORKSPACE=true` is the explicit local
workspace flag. Development HTTP exceptions still require the existing explicit
loopback setting and IP-loopback browser origin. Production provider callbacks
remain canonical HTTPS, not local development callback URLs.

Browser refresh secrets never enter JSON or localStorage. Canonical refresh
cookies remain host-only, HttpOnly, Secure and SameSite=Lax. Each approved media
origin gets its own origin-hashed cookie name with SameSite=None and Secure.
Refresh/revoke requests require that origin's expected family ID; unrelated
origin cookies are not selected. This avoids a media tab rotating or overwriting
the account website's session. Browser cross-site-cookie restrictions can still
prevent media renewal; that is a runtime browser policy, not permission to expose
refresh secrets to JavaScript. Test the deployed origin arrangement and surface
renewal failures rather than claiming the account remains usable indefinitely.

Media web, including local-server enrollment settings, uses the existing browser
approval protocol and does not collect Hosted passwords. Direct Sign-In remains
separate. Both native clients keep session credentials with the existing
CredentialSessionService/Keychain owner. The new Keychain entry stores only the
private continuation binding, opaque transaction ID and pending MFA continuation.

## Apple integration

Both Xcode targets include `Native/PorticoAccountAuth.m` and AuthenticationServices.
`mobile/` is iOS and `ios/` is tvOS in this repository. Both declare the `portico`
return scheme and Sign in with Apple entitlement. The local team must use
provisioning profiles matching that entitlement and the configured bundle IDs.
Run normal CocoaPods/native build steps; no generated Pods or binaries are shipped.

On iOS, Google/Apple browser flows use ASWebAuthenticationSession; native Apple is
used when the running bundle ID is configured. tvOS supports native Apple and the
existing phone/computer activation path for Google. The cold/warm Linking and
AppDelegate paths accept only the expected opaque account-return transaction.
Security status is available without a selected server. iOS opens canonical
account security; tvOS shows its public QR/URL. Those handoffs contain no account
credential and require explicit browser authentication. Foreground refresh learns
revocation after a security change.

## Transactions and recovery behavior

Registration is pending for 30 minutes. Its six-digit verification code lasts at
most 10 minutes, has five attempts and can be replaced up to five times. The
verified contact, account and first session are committed
atomically. A username collision retains verified contact and asks for another
username. The same request ID retries the same operation rather than minting a
new account or password. Completed session receipts are sealed, bounded to five
minutes and remain subject to current-family validity.

Password reset uses a 15-minute high-entropy, one-use emailed token. Public
requests have the same outcome for unknown, mismatched and provider-only
accounts. Provider-only users recover through their provider. Reset requires an
MFA factor when enabled, revokes account families and does not silently sign in.
An identical successful retry is allowed; changed input cannot reuse the token.
Email change keeps the old verified primary address until the new code is
consumed under the initiating family/epoch/access authority, then revokes old
sessions and queues security notices.

Only credential changes need recent authentication: password, email, and the
second factor (enroll, disable, recovery codes). Account management (removing or
transferring a server, linking or unlinking a provider, the deletion preview and
export) needs the signed-in session only. The final account deletion is confirmed
by typing the account's email (its username when it has none), not a password.
Recent authentication is scoped to account, family, epoch, generation, captured
access, purpose, target and caller operation ID for five minutes. Password proof
also requires MFA when enabled. Provider proof must authenticate the account's
existing issuer/subject identity; email similarity never links an account.
Provider linking still runs a provider sign-in for the new identity and signs
nobody out. Unlinking cannot remove the final usable sign-in method; it signs
out every other device (families don't record how they signed in) and keeps the
caller's session. Providers can add a password after fresh provider
authentication. Changes that invalidate account credentials revoke old families;
clients retain scoped result receipts without adopting a different account.

TOTP uses six digits, 30-second periods and one-step drift; accepted steps cannot
be replayed for another operation. Enrollment is not active until confirmation.
Ten random single-use recovery codes are shown after enrollment/regeneration;
only keyed hashes remain in the code table. A short encrypted result receipt
allows recovery of a lost response. Users must save those codes before dismissing
the result. There is no unauthenticated MFA bypass: losing every provider/password
recovery factor and every recovery code does not authorize account takeover.

Second-factor failures are also remembered per account until a day passes
without one, across fresh challenges. After ten, each further code waits a
cool-off doubling from 30 seconds to a 30-minute cap (HTTP 429 with
Retry-After). A refused code is neither checked nor counted and does not spend a
challenge attempt, but the challenge still expires after five minutes, so after a
long cool-off the caller signs in with the password again. A correct code clears
the count. Recovery codes are still checked during a cool-off, and password reset
keeps its own count. A successful reset (emailed link plus a factor) also clears
the sign-in count, so someone holding only the old password can slow the owner
down but never lock them out. Sustained guessing is cut from about 1,900 codes a
day to a few dozen; it is slowed, not made impossible.

Password cool-off answers alike for existing and unknown usernames. A
well-formed unknown name runs the same per-network (four free failures in 15
minutes, then 30 seconds doubling to 15 minutes) and account-wide (over 100
failures an hour) schedule as an account, in durable rows shared by every
replica and kept across restarts. The rows are fixed buckets chosen by the seal
key's HMAC of the name (and network): at most 3 × 65,536 rows, never the name
itself, cleaned up by the usual security-limit retention. A malformed name
cannot be an account and gets no state. Limits: names that share a bucket share
its count, so an unknown name can cool off early (never an account), which a
large distributed flood could occasionally turn into a probabilistic hint; the
dummy bcrypt equalizes work, not every timing detail; and registration still
says when a username is taken, after the emailed code is verified, which is a
costlier, intentional answer outside this cool-off.

## Durable outbox and invitation boundary

`postgres.EnqueueAccountMailTx(ctx, tx, keys, mailoutbox.Intent{...})` is the typed
boundary. Call it in the same transaction as the authoritative domain change.
Supply a stable intent ID, resource ID/revision, intended account/epoch, expiry and
payload. Reusing an ID with a different payload is a conflict. Supported kinds:
registration, OIDC contact, password reset, email change and security notice.
Invitations, profiles and their data belong only to each Server; Hosted neither
stores invitations nor sends invitation email.

Two worker lanes claim durable 45-second leases and recheck current eligibility
immediately before SMTP, with no database lock held during network I/O. There are
at most eight attempts with bounded exponential backoff/jitter. Permanent SMTP
5xx responses are terminal; transient or ambiguous failures retry the same
message ID and code. Message IDs use the immutable intent ID at web.getportico.tv,
independent of later sender configuration changes. A lost DATA acknowledgment may
produce duplicate email; this is deliberately at-least-once delivery, not an
exactly-once claim. Cancellation after the eligibility check cannot unsend an
in-flight message, but expired/superseded/consumed proofs remain unusable.

Text and escaped HTML contain canonical links without embedded proofs; users
paste the code into their originating flow. Sent/terminal payload ciphertext is
cleared. Bounded maintenance prunes expired proofs, challenges, receipts, limits
and old mail metadata. No tests in this delivery contacted an SMTP service.

## Schema and deletion integration

`InitializeCurrentHostedSchemaTx` installs the complete current baseline in one
transaction. The `hosted_schema_contract` marker is version 24. Historical or
unrecognized schemas are refused and must be provisioned explicitly; no legacy
profile, invitation, policy or remote-erasure tables are adopted. Account,
session-family, claim, return and generation authority remains transactional.

Deletion requires an explicit fresh, current-family-bound preview and confirmation.
An account that owns any server cannot be deleted; its ownership must first be
resolved on that server. Accepted member deletion revokes central identity and
sessions and records only a durable departure notice for its servers. It never
queues deletion of Server profiles or media activity.

`POST /v1/servers/leave` accepts an immutable original-key signed notice bound to
the exact operation and installed claim/credential generations. It returns an
empty 204, including exact replay. A Server persists the notice before moving to
local sign-in; delivery can follow later without enabling other Local traffic.
The transaction removes central registration, account links, addresses and
certificate work. It retains only a hashed replay receipt and normalized public
name/sequence withdrawal records so offline DNS readers can remove stale names.
Old notices and stale positive authority cannot affect a successor registration.
