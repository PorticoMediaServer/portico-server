# Offline downloads

How a third-party client takes Portico media offline and plays it with no server in reach.
`downloads.openapi.yaml` is the normative contract; this page explains the model and the order
things happen in.

Downloads produce no media of their own. A download is a *claim* on bytes that already exist — the
library's own file, or a published optimization — plus a verification hash, a short-lived transfer
handle, and a signed statement that this viewer may play those bytes offline.

---

## The four steps

```
GET  /v1/items/{id}/download-options        what can I take, how big, may I?
POST /v1/downloads/preparations             claim it
POST /v1/downloads/preparations/{id}/grant  get a transfer handle
GET  /v1/downloads/artifacts/{grant}        move the bytes  (Range, ETag, 206)
POST /v1/downloads/receipts                 take the offline receipt
```

Then, whenever the client is back in contact:

```
POST /v1/downloads/receipts/revalidate      renew before the 30 days run out
GET  /v1/downloads/receipts/revocations     has anything been withdrawn?
POST /v1/downloads/progress                 hand back what was watched offline
```

---

## Qualities

`quality` is either `original` or a **quality ladder rung id** — `2160p`, `1440p`, `1080p`, `720p`,
`480p`, `360p` — the same vocabulary `/v1` playback publishes. `download-options` lists exactly the
rungs this source supports; a rung taller than the source is omitted rather than offered as an
upscale, and an audio item publishes one optimized rung rather than repeating the same AAC recipe
six times.

`original` is the untouched catalogue file. A rung resolves to a published prepared version from
the optimization producer, mapped as:

| Source | Rung | Recipe |
| --- | --- | --- |
| song, audiobook file | any | `audio-aac-v3` |
| video | each published rung | `portable-{height}-v3` (360, 480, 720, 1080, 1440, 2160) |

Size estimates use the selected recipe’s video and audio bitrates. Offline progress submitted while `privacy.pauseWatchHistory` is enabled returns `history_paused` and does not change progress, history, or watched state.

An audio item publishes exactly one optimized option — the `1080p` rung, labelled `AAC 192 kbps`,
whose audio budget is the recipe's own. Naming another rung for an audio item resolves to the same
recipe and the same size estimate; the published option is the one to show.

**Conversions stay owner-authorized.** An owner asking for a rung that has not been prepared yet
queues the conversion, and the preparation reports `running` with the conversion's own byte
progress. Any other viewer asking for an unprepared rung gets `unavailable` with
`optimized_version_unavailable` — downloads never widens who may order encoding work.
`download-options` says which case you are in: `requiresPreparation: true` means asking will queue
work, `available: false` with that reason means it will not.

`estimated` is the honest flag on every size. It is `false` for the original (a stat of the file)
and for a rung that is already prepared (the artifact's real size); it is `true` where the number
is bitrate × duration + 3% container overhead.

---

## Preparation lifecycle

```
queued ──► running ──► ready ──► expired
  │           │          │
  │           ▼          ▼
  └────────► paused   (removed / cancelled)
              │
              ▼
  failed / unavailable ──retry──► queued
```

| State | Meaning |
| --- | --- |
| `queued` | Admitted. The worker has not picked it up yet. |
| `running` | Bytes are being hashed (for `original`) or converted (for a rung). `progress` is real. |
| `ready` | `artifact.sha256` is published. The claim can be granted and receipted. |
| `paused` | The viewer stopped it. Work already done is kept. |
| `failed` | Something went wrong; `reason` says what. `retry` is offered. |
| `unavailable` | Nothing to do: the source, the policy or the optimization is missing. |
| `cancelled` | The viewer gave it up. Terminal. |
| `expired` | Retention took it. Terminal. |

Each preparation publishes its own `actions` array — exactly the actions its state accepts. Render
those; the server refuses anything else with `409 download_conflict`. Every action carries
`expectedRevision`, and a conflict answers with the current preparation beside the error so no
second read is needed.

`pause` keeps the work. For `original`, the SHA-256 is computed in bounded windows and its state is
checkpointed between them, so `resume` continues from where it stopped rather than rehashing a
60 GB file from the start. The same checkpointing is why a server restart costs one window.

`cancel` and `remove` also **revoke every offline receipt** issued for that claim. A receipt that
outlived its preparation would authorize bytes the server no longer accounts for.

---

## Batches

One request, one quality, and exactly one of four target forms:

```json
{"operationId": "op-4f22", "containerId": "season_0003", "quality": "720p"}
```

`containerId` accepts a season, an album, a book or a playlist, and expands in playback order up to
100 items. `nextAfterMediaId` resolves to exactly one item — the next episode in the season, or the
first episode of the next season — so an auto-next download stays one episode ahead instead of
silently pulling a whole run.

Targets are admitted **independently**. One unavailable episode appears in `rejected` with a reason
code; the rest are accepted. A target the viewer already holds a live claim on is returned in
`items` unchanged rather than duplicated, so a client that retried a batch does not accumulate
copies.

`operationId` is a real idempotency key. Replaying it returns the original batch and answers `200`
with `duplicate: true`. Reusing it for a *different* payload answers `operation_conflict`.

---

## Transfers

`POST /v1/downloads/preparations/{id}/grant` returns a capability URL. The token in the path is the
whole authority — no bearer token is read on the transfer route — exactly as for playback media
URLs. The grant names the viewer it was issued to, and that viewer's library permission is
re-checked before a byte is served.

- TTL: **10 minutes** from issue.
- Replay window: **10 minutes** from first use. Inside it the same grant serves as many ranged
  requests as a resumable transfer needs. Outside it, mint a new grant.

The response carries `Accept-Ranges: bytes`, `Content-Length`, `ETag: "sha256-<hex>"`,
`X-Portico-Artifact-Sha256` and `Content-Disposition`. A `Range` request answers `206` with
`Content-Range`. Use `If-Range` against the ETag so a transfer resumed across a server restart
either continues or restarts cleanly rather than splicing two different files.

**Verify the completed file against `artifact.sha256` before trusting it offline.** The receipt
vouches for that digest and nothing else.

---

## Offline receipts

A receipt is an Ed25519-signed statement, good for **30 days**, that names the viewer, the item,
the quality, the artifact's SHA-256 and size, and the key that signed it.

```json
{
  "receiptId": "…",
  "algorithm": "ed25519",
  "keyId": "…",
  "payload": "eyJraW5kIjoicG9ydGljby5kb3dubG9hZC1yZWNlaXB0Iiw…",
  "signature": "…",
  "claims": {
    "kind": "portico.download-receipt",
    "version": "1",
    "receiptId": "…",
    "keyId": "…",
    "viewer": {"authority": "local", "accountId": "…", "profileId": "…", "serverId": "…"},
    "itemId": "item_0001",
    "preparationId": "prep_a",
    "quality": "1080p",
    "qualityLabel": "1080p",
    "artifact": {"sha256": "9f2c…", "bytes": 4183920640},
    "issuedAt": "2026-09-16T10:00:00Z",
    "expiresAt": "2026-10-16T10:00:00Z"
  },
  "revision": 1
}
```

`payload` is the exact base64url-encoded bytes that were signed, so a verifier never has to
reproduce this server's JSON formatting. To verify offline:

1. Fetch and pin `GET /v1/downloads/receipt-keys` while online.
2. base64url-decode `payload` and `signature`.
3. Check `claims.kind` and `claims.version`, and that `claims.keyId` equals the envelope's `keyId`.
4. `ed25519.Verify(publicKey, payload, signature)`.
5. Check `claims.expiresAt` is in the future and `claims.viewer` is the signed-in viewer.
6. Check the file's SHA-256 against `claims.artifact.sha256`.

A receipt is **not transferable**: it names one viewer on one server.

### Revalidation

Send the receipt ids you hold. The server answers from its own rows, not from the envelope you
sent — a stale envelope is exactly what a client returning from a month offline would be holding.

`renewed` extends the receipt another 30 days. `refused` means stop and delete the file; the `code`
says why:

| Code | What happened |
| --- | --- |
| `downloads_not_allowed` | The profile lost download permission. |
| `account_disabled` | The account was switched off. |
| `item_deleted` | The item is gone from the library. |
| `retention_expired` | The preparation is no longer ready. |
| `revoked` | Someone withdrew this receipt. |
| `unknown_receipt` | Not a receipt this server issued to this viewer. |

### Revocation list

`GET /v1/downloads/receipts/revocations` pages on a monotonic `sequence`. Store the last sequence
you saw and pass it as `cursor`; you get only what you missed. Poll it on reconnect and act on
every entry, even for receipts you have not revalidated yet. An owner may pass `scope=server`.

---

## Deferred progress

Hand back what was watched offline:

```json
{
  "operationId": "op-8d01",
  "entries": [
    {"itemId": "item_0001", "positionSeconds": 1723.5, "observedAt": "2026-09-14T09:12:00Z"},
    {"itemId": "item_0002", "positionSeconds": 0, "watched": true, "observedAt": "2026-09-14T10:41:00Z"}
  ]
}
```

Applied entries land in ordinary personal state — the same resume position, activity row and
watched flag an online client writes. There is no second kind of progress.

Resolution is last-write-wins by `observedAt`, compared against **two** things: the last offline
observation this server accepted for that viewer and item, and the last online activity it
recorded. Losing does not mean silently dropped — every entry gets an outcome and the server's
current value:

| `outcome` | Meaning |
| --- | --- |
| `applied` | Written. |
| `stale_observation` | An offline observation you already sent was newer. |
| `superseded_online` | The viewer was demonstrably at a server after this was recorded. |
| `item_deleted` | No such item. |
| `invalid_entry` | Unreadable `observedAt`, negative or non-finite position. |
| `observation_in_future` | More than 24 hours ahead of the server's clock; refused rather than allowed to pin the item. |

---

## Storage and retention

`downloads.maxPreparedBytes` (default `0` = unlimited) is the ceiling the owner sets. A claim counts
against it from the moment it is admitted, not when its bytes land — admitting work the store cannot
hold and discovering it later would leave a viewer watching a progress bar that can only end in
failure. Over the ceiling, new preparations answer `409 storage_full`, and a partly-fitting batch
reports `storage_full` per rejected target.

`GET /v1/downloads/usage` reports `profileBytes` and `serverBytes` as *claims*: two viewers holding
the same prepared version each account for its bytes, because either one releasing the claim frees
nothing while the other holds it. `distinctArtifactBytes` is what the disk actually holds.

`downloads.retentionDays` (default 30) expires an unused ready claim — measured from the last
transfer if there was one, and from the moment it became ready if there was not. Expiry revokes the
claim's receipts, so a client learns about it through the revocation list or its next revalidation.

A ready claim whose item leaves the library is retired the same way: it becomes `unavailable` with
`item_deleted`, stops counting against the ceiling, and its receipts are revoked. A client does not
have to notice this itself — it arrives in the revocation list.

---

## Policy

`allowDownloads` is published per profile in `download-options.policy`. Today it answers `true`
unless a hosted restriction has revoked the profile; the dedicated per-profile column
(`profiles.allow_downloads`) is owned by the profile-restrictions workstream, and the server reads
it the moment it exists with no client change. A profile that loses the permission cannot prepare,
cannot take grants, and fails revalidation with `downloads_not_allowed`.

---

## Polling

While a preparation is `queued` or `running`, poll `GET /v1/downloads/preparations/{id}` at most
once every 3 seconds. The worker advances in bounded passes; a tighter loop buys nothing. Prefer
`GET /v1/downloads/preparations` with `state=running` over polling each claim separately.


## Whole-container requests

Use `POST /v1/downloads/requests` for a show, season, album, book or playlist. The full contract is in `download-requests.openapi.yaml`. Supply one stable operationId and the authenticated deviceId. Capture and admission run in durable 20-member pages; the initial response publishes totalKnown=false. GET the request for live visible-member counts and paged preparations, then use the ordinary grants and receipts for each ready artifact.

The 200 limit bounds running preparation work. Queued and ready claims no longer consume an active slot. Storage reservations and retention still apply. The legacy preparations endpoint rejects containers over 100 instead of truncating them. New container requests have no 100-item ceiling.
