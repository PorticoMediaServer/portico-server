# Portico link contract (normative)

October boundary: plain HTTP is permitted for direct local-server addresses from any network. Hosted links and advertised routes remain TLS. No email or consumer push is sent. The outside-app consent flow is a deliberate exception to the general fragment rule: reviewed callback code/state are query values, bound to exact return address and PKCE. [Hosted review fixes](../../../_program/reports/review-hosted.md) define that behaviour; actual universal/app-scheme handoff remains unverified.

One table for every public link Portico emits: QR codes, share sheets, Handoff and universal links. Hosted keeps these paths in one Go constant set, web implements the routes, and Apple maps them to `portico://`. Secrets (tokens, invite codes) always go in the **fragment**, never the query, and the page scrubs them from history after reading them (`history.replaceState`).

| Purpose | Web URL (`https://web.getportico.tv`) | App link | Universal link |
|---|---|---|---|
| Password reset / email verification | Historical Hosted paths require no-email reconciliation; no mail is sent | — | no |

| Server or account invitation | `/join#invite=<t>` | `portico://invite#invite=<t>` | yes (`/join`) |
| Watch Together invite | `/together/join#code=<c>` | `portico://together/join/<c>` | yes (`/together/join`) |
| Approve a TV sign-in | `/device` (optional `#code=<c>`) | `portico://device` | yes |
| Approve a server claim | `/claim#code=<c>&server=<name>&return=<origin>` | — (web) | no |
| Account | `/account` (`?tab=profiles|sessions|security`; Hosted invitations were removed, Hosted at Scale §11) | `portico://account` (same tab values) | yes |
| Security notice | `/account?tab=security&event=<id>` | `portico://account?tab=security` | yes |
| Title (Handoff to Mac/web) | `/media/<itemId>` | `portico://item/<itemId>` | no (Handoff only) |

## `apple-app-site-association`
Served by Hosted web at `https://web.getportico.tv/.well-known/apple-app-site-association` (and the same bytes at `https://getportico.tv/.well-known/…` once the public site is in scope): `application/json`, 200, no redirect, no auth. `TEAMID` is filled in from build configuration (not committed).

```json
{
  "applinks": {"details": [{"appIDs": ["TEAMID.tv.getportico.portico.ios"], "components": [
    {"/": "/account"}, {"/": "/device"}, {"/": "/join"}, {"/": "/together/join"}
  ]}]},
  "activitycontinuation": {"apps": ["TEAMID.tv.getportico.portico.ios"]},
  "webcredentials": {"apps": ["TEAMID.tv.getportico.portico.ios"]}
}
```

Fragments aren't part of path matching, so `/join` and `/together/join` match regardless of the secret; the app receives the full URL and reads the fragment.
