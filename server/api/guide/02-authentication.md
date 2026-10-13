# 2. Authentication

There are two ways to authenticate, for two kinds of caller:

- **An app a person uses** (a player, a remote, a companion app) signs that
  person in and holds a **session**. This section. No app registration is
  needed: any app may sign a member in, directly or with a code.
- **A script, an integration or an assistant** carries an **API key** or a
  **personal access token** that somebody made for it. See
  `07-keys-tokens-and-scopes.md`.

## Signing in from your own app

Send these headers on every request, so the person can recognise and manage the
device in their device list:

| Header | Value |
|---|---|
| `X-Portico-Installation-Id` | A random id your app generates once and keeps (32–128 characters of letters, digits, `-` and `_`). It ties refresh to this install. |
| `X-Portico-Device-Name` | What the person would call the device ("Sam's laptop"). |
| `X-Portico-Device-Platform` | For example `linux`, `android`, `web`. |
| `X-Portico-App`, `X-Portico-App-Version` | Your app's name and version. |

Then either:

1. **Server address, user name and password.** `POST /v1/direct/sign-in
   (openapi.yaml, operationId directSignIn)` with the credentials; pick a
   profile with `POST /v1/direct/profiles/{id}/select (openapi.yaml,
   operationId selectDirectProfile)`. For an account with one profile and no
   second factor, `POST /v1/sessions (openapi.yaml, operationId createSession)`
   does both in one call. This works on any server, whether or not it uses
   Portico Accounts, for members who have a password on that server.
2. **A code the person enters elsewhere** ("TV code", below). Your app shows a
   short code; the person approves it in any Portico app already signed in to
   that server, or on the server's own `/device` page. Use this when typing a
   password is awkward, or for a member who signs in with a Portico Account.

Both give an ordinary member session: an access token that lasts 15 minutes and
a refresh token (see Refresh). The session is that member's, bounded by their
role and restrictions, and appears in their device list, where they or the owner
can end it. If the owner approves new devices by hand, a new device answers
`device_approval_pending` until it is approved.

Signing in through the Portico Account service ("Sign in with Portico") from a
third-party app is not offered yet.

Every authenticated call carries `Authorization: Bearer <accessToken>`. There
are two session kinds: a **viewer session** (what catalogue, playback, and
personal-state routes take) and a **direct account session** (`accountToken`,
the bearer for the `/v1/direct/*` account-management routes). Which one a
sign-in returns depends on the entry point.

## Direct sign-in

`POST /v1/sessions (openapi.yaml, operationId createSession)` is the one-call
shortcut: username and password straight to a viewer session.

<!-- example: auth-credentials-request file=openapi.yaml operationId=createSession direction=request schema=@request /v1/sessions post -->
```json
{
  "username": "recovery-owner",
  "password": "correct-horse-battery-staple"
}
```

<!-- example: auth-session-response file=openapi.yaml operationId=createSession direction=response status=200 schema=#/components/schemas/AuthSession -->
```json
{
  "accessToken": "tok_01J6X7QZ9MAB0CDEF123456789",
  "expiresAt": "2026-09-24T13:00:00.000Z",
  "viewer": {
    "accountId": "acct_1",
    "profileId": "prof_1",
    "serverId": "srv_1",
    "authority": "local",
    "role": "member"
  },
  "sessionFamilyId": "fam_1",
  "tokenGeneration": "3",
  "authorizationHorizon": "2026-09-24T13:00:00.000Z"
}
```

`POST /v1/direct/sign-in (openapi.yaml, operationId directSignIn)` instead
returns an account session (`accountToken`) for the `/v1/direct/*` routes —
profiles, members, password, ownership. It accepts either `Credentials` or a
Portico identity assertion (next section). To browse, select a profile with
`POST /v1/direct/profiles/{id}/select (openapi.yaml, operationId
selectDirectProfile)`, which returns the viewer session. `GET /v1/me
(openapi.yaml, operationId getCurrentViewer)` reads the current viewer.

Personal access tokens, API keys and scopes are described in
`07-keys-tokens-and-scopes.md`. OAuth 2.1 (PKCE), `client_id`, consent screens
and open bearer CORS are **not specified**.

## Portico Account sign-in (challenge to assertion)

A Portico Account member signs in to the server itself in two steps. First ask
for a single-use challenge with `POST /v1/direct/portico-challenge
(openapi.yaml, operationId porticoChallenge)`:

<!-- example: auth-challenge-request file=openapi.yaml operationId=porticoChallenge direction=request schema=@request /v1/direct/portico-challenge post -->
```json
{
  "nonce": "n_01J6X7QZ9MAB0CDEF123456789"
}
```

<!-- example: auth-challenge-response file=openapi.yaml operationId=porticoChallenge direction=response status=200 schema=@response /v1/direct/portico-challenge post 200 -->
```json
{
  "challenge": "ch_01J6X7QZ9MAB0CDEF123456789",
  "expiresAt": "2026-09-24T12:05:00.000Z",
  "proof": {
    "payload": "cGF5bG9hZA",
    "signature": "c2lnbmF0dXJl"
  }
}
```

Hosted signs an identity assertion answering that challenge; the client sends
it as `porticoIdentity` to `POST /v1/direct/sign-in (openapi.yaml, operationId
directSignIn)`. The assertion envelope is `SignedEnvelope` (payload,
signature, keyId, verified offline against the pinned Hosted root):

<!-- example: auth-signed-envelope file=openapi.yaml operationId=directSignIn direction=request schema=#/components/schemas/SignedEnvelope -->
```json
{
  "payload": "cGF5bG9hZA",
  "signature": "c2lnbmF0dXJl",
  "keyId": "hosted-root-2026"
}
```

Membership is this server's own: an account that is not a member here gets
`403 access_refused`. Hosted ticket attach, Hosted restrictions, and offline
profile proofs are gone and answer `410`. The Hosted-side issuance route is
**not specified** in the server OpenAPI files.

## Device sessions

A session belongs to a family: `sessionFamilyId` identifies it and
`tokenGeneration` numbers the current token within it. Rotating a token is a
new generation of the same family — the same device, not a new one.
`GET /v1/direct/sessions (openapi.yaml, operationId listDirectSessions)`
lists the account's live sessions with a `current` flag on the caller:

<!-- example: auth-direct-sessions-response file=openapi.yaml operationId=listDirectSessions direction=response status=200 schema=@response /v1/direct/sessions get 200 -->
```json
{
  "items": [
    {
      "id": "sess_1",
      "profileId": "prof_1",
      "profileName": "Living Room",
      "expiresAt": "2026-09-24T13:00:00.000Z",
      "current": true
    }
  ]
}
```

`DELETE /v1/direct/sessions/{id} (openapi.yaml, operationId
revokeDirectSession)` revokes one session; `DELETE /v1/sessions/current
(openapi.yaml, operationId logoutSessionFamily)` signs out the whole family
and accepts any authentic token of the family, including an expired or retired
generation, so a stale client can still sign itself out. Revocation is
immediate: the fence is re-evaluated inside each request's transaction and
in-flight media reads are re-checked mid-transfer. Short-lived (15-minute or
1-hour) access tokens, rotating refresh semantics on the server side, and a
"Connected apps" listing are **not specified**.

## Refresh

First-party refresh is `POST /v1/auth/refresh (identity.openapi.yaml,
operationId refreshFirstPartySession)` with the installation-bound refresh
token:

<!-- example: auth-refresh-request file=identity.openapi.yaml operationId=refreshFirstPartySession direction=request schema=@request /v1/auth/refresh post -->
```json
{
  "refreshToken": "hrt_01J6X7QZ9MAB0CDEF123456789",
  "installationId": "my-player-1",
  "requestId": "refresh-1"
}
```

Success returns an `AuthSession` (same shape as the direct sign-in example).
During playback, a client renewing without interrupting uses `POST
/v2/playback/authorization-renewal (openapi.yaml, operationId
renewPlaybackAuthorization)` (bearer plus controller token); a retired
generation answers `receipt_expired` with a reauthorize action. Refresh working
identically over HTTP and HTTPS, and Secure-connections-required servers, are
**not specified** in the OpenAPI.

## TV code (device authorization)

Quick Connect is the device-code flow for a TV, console, or headless client.
The waiting device calls `POST /v1/quick-connect (openapi.yaml, operationId
startQuickConnect)` and displays the `userCode`:

<!-- example: auth-quickstart-request file=openapi.yaml operationId=startQuickConnect direction=request schema=@request /v1/quick-connect post -->
```json
{
  "requestId": "req-tv-1",
  "deviceName": "Living Room TV"
}
```

Any member signed in to this server looks the code up with `POST
/v1/quick-connect/review (openapi.yaml, operationId reviewQuickConnect)`
(which returns the device's claimed name, platform, and app version) and then
records the decision with `POST /v1/quick-connect/decision (openapi.yaml,
operationId decideQuickConnect)`:

<!-- example: auth-quickdecision-request file=openapi.yaml operationId=decideQuickConnect direction=request schema=@request /v1/quick-connect/decision post -->
```json
{
  "requestId": "req-tv-1",
  "userCode": "ABCD-1234",
  "decision": "approve"
}
```

The device polls `POST /v1/quick-connect/token (openapi.yaml, operationId
pollQuickConnectToken)` with its `deviceCode`, receiving
`authorization_pending` (or `slow_down` with `Retry-After`) until the decision
lands, then its own session. `POST /v1/quick-connect/cancel (openapi.yaml,
operationId cancelQuickConnect)` abandons the attempt. The new device is signed
in as the member who approved the code, with that member's role, so approve only
a code you are looking at on your own device. The answer to starting a code
carries `verificationUri` (the server's own `/device` page) when the server knows
an address to offer. RFC 8628 discovery metadata and expiry configuration beyond
the issued `expiresAt`/`expiresIn`/`interval` are **not specified**.
