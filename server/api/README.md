# Portico media server HTTP API

This directory is the published contract for everything the Portico media server serves over HTTP.

| File | What it is |
| --- | --- |
| `openapi.yaml` | **The complete API.** Every route registered by `server/internal/httpapi`, plus the `internal/networking` claim/remote/certificate routes composed into the same mux. |
| `playback-v2.openapi.json` | The renderer-independent playback protocol in full, including operations this server build does not yet register. `openapi.yaml` references its schemas by external `$ref` rather than copying them. |
| `playback-v2-protocol.md` | Normative semantics for that protocol — canonical encoding, receipt dispositions, provenance, lane ordering. |
| `playback-v2-canonical-fixtures.json`, `playback-v2-source-reference-fixtures.json` | Wire fixtures the protocol tests run against. |
| `social.openapi.yaml` | Watch Together groups, Portico receiver handoff and Google Cast pairing, including the group SSE stream and its event types. |
| `social.md` | Normative semantics for that group — the group timeline and correction bands, the host-disconnect timeline, the two-phase handoff state machine, and why DLNA is omitted and AirPlay is client-only. |
| `playback-admission.md` | Active typed timeline reporting cadence, authenticated device limits, response deadlines and overload recovery. |

`server/internal/httpapi/openapi_coverage_test.go` fails the build if a route is
registered without an entry in `openapi.yaml`, so the document cannot silently drift. This is
standing principle 2a.1 of the Feature Depth Program: everything a client can do is reachable
through a sensible, documented endpoint, because third parties build players and
library-management automations on this API. A client-only feature is not acceptable.

---

## Versioning

**`/v1` is the product surface.** Identity and accounts, the catalogue, browsing and search,
personal state, playlists and saved views, metadata administration, storage, live TV and DVR, the
operations console — and the original single-session playback model (`POST /v1/playback/sessions`
and its progress and chapter routes). A `/v1` route is a resource you read or write.

**`/v2` is the playback control plane.** It exists because the `/v1` session model could not
express the things a real player needs: a phone controlling a TV, a renderer that reconnects after
its process died, two clients that both think they are in charge, a command whose response was lost
on the way back. `/v2` replaces "the session" with *controllers*, *command lanes*, *occurrences*
and *queues*, each addressed by an explicit identity and fenced by a revision. It is a different
model, not a newer spelling of the same one, which is why it is a separate namespace rather than
`/v1` with more fields.

**There is no `/v3` or `/v4`.** `api4.thetvdb.com/v4` appears in the server source, but that is an
outbound URL for TheTVDB's own API, not a Portico route namespace.

**Media URLs are not resources.** `/v1/media/{grant}/...` and `/v2/media/linear/...` are
capability URLs: unguessable, grant-bound byte streams whose authority is the grant in the path
rather than a bearer token. They are re-authorised *during* the transfer — every 100ms for HLS
segments, around every chunk for subtitles and audio renders — so a revoked session stops
mid-stream instead of finishing the file.

This is pre-release software. Per rule 2.1 of the Feature Depth Program, no backward compatibility
with prior databases, endpoints or clients is required: routes are reshaped and deleted rather than
deprecated. Treat this document, not any earlier client, as the contract.

---

## Authentication

Every authenticated call carries `Authorization: Bearer <accessToken>`. There are four ways to get
one, and the choice depends on where the account lives and what kind of device is asking.

### Direct sign-in (a server-local account)

`POST /v1/direct/sign-in` with a username and password returns an **account session**
(`accountToken`), not a viewer session. The account session is the bearer for the rest of
`/v1/direct/*`: list profiles, create and edit them, reorder them, manage members, change the
password, transfer ownership. To actually browse anything you then call
`POST /v1/direct/profiles/{id}/select` with the profile PIN (and optionally the caller's
`installationId` plus `trust: true`, which returns a trust token that lets that installation skip
the PIN next time). That call returns the **viewer session** — `session.accessToken` — used
everywhere else.

`POST /v1/sessions` is the one-call shortcut: username and password straight to a viewer session.
Both entry points are rate limited to 20 attempts per minute per peer address, alongside
`POST /v1/setup` and every `POST /v1/direct/*`.

### Portico Account sign-in

A Portico Account member signs in to the server itself: `POST /v1/direct/portico-challenge`
returns a single-use challenge, Hosted signs an identity assertion answering it
(`POST /v1/servers/{id}/identity` on Hosted), and `POST /v1/direct/sign-in {porticoIdentity}`
verifies that assertion offline against the pinned Hosted root. Membership is this server's own:
an account that is not a member here gets `403 access_refused`. The old Hosted ticket attach,
Hosted restrictions and offline profile proofs are gone and answer `410 moved`.

### Quick Connect (a device with no keyboard)

The TV, console or headless automation calls `POST /v1/quick-connect` and displays the `userCode`
it gets back. A signed-in local owner reads that code into `POST /v1/quick-connect/review` — which
returns the device's claimed name, platform and app version so the approval is made against a
described device — and then approves or denies it with `POST /v1/quick-connect/decision`. The
waiting device polls `POST /v1/quick-connect/token` with its `deviceCode`, receiving
`authorization_pending` (or `slow_down` with a `Retry-After`) until the decision lands, then its
own viewer session. `POST /v1/quick-connect/cancel` abandons the attempt.

### Session refresh and sign-out

A session belongs to a **family**: `sessionFamilyId` identifies it and `tokenGeneration` numbers
the current token within it. Rotating a token produces a new generation of the same family — it is
the same device, not a new one — which is why asynchronous work started under an older generation
keeps working. A client whose token is about to expire while playing calls
`POST /v2/playback/authorization-renewal` (bearer plus controller token) to obtain the next
generation without interrupting playback; a retired generation answers `receipt_expired` with
action `reauthorize`.

`DELETE /v1/sessions/current` signs out. It accepts *any* authentic token of the family, including
an expired or retired generation — deliberately, so a client holding a stale token can still sign
itself out — and it revokes the whole family, not just the presented token. It takes no query
string and no request body of any kind. `GET /v1/direct/sessions` lists a direct account's live
sessions and `DELETE /v1/direct/sessions/{id}` (or `.../all`) revokes them. Revocation is
immediate: the permission fence is re-evaluated inside the transaction that answers each request,
and in-flight media reads are re-checked mid-transfer.

---

## The controller and lane model

**A controller is an installation, not a session.** Before a client can do anything in `/v2` it
calls `POST /v2/playback/controllers` with an `installationId` it persists and an
`installationSecret` of at least 256 random bits that it generates and stores securely. The server
returns a `controllerId` and a `controllerEpoch`. From then on, every `/v2` call carries two
credentials: the bearer token, which says *who* is asking, and the `X-Playback-Controller-Token`
header carrying that secret, which says *which client* is asking. The secret is always bound to
this account, profile, controller and epoch; it is never accepted as authority on its own, and it
is never logged or returned in diagnostics. If a client loses its stored secret it registers a new
installation and explicitly retires its own previous controller via `retirePrevious` — it cannot
recover the old one, and a retired epoch never implicitly reactivates.

**A command lane is one ordered channel of commands.** `POST /v2/playback/lanes` establishes a
lane on a controller epoch. Every command a client sends carries that lane's identity plus a
`sequence` and a client-chosen `requestId`. The sequence is what lets the server reject a replayed
or out-of-order command instead of applying it twice; the request id is what lets the client ask,
later, what happened to a command whose response it never saw. Lane creation is idempotent, and
there is no arbitrary cap on how many lanes a controller may have — a client with a foreground
player and a background pre-fetcher can give each its own lane so their orderings do not interfere.
A queue is bound to one lane: `GET /v2/queues` with no queue id returns whatever queue that lane
currently owns.

**An occurrence is one act of playing something, independent of any renderer.** `POST
/v2/playback/occurrences` (or `POST /v2/playback/channels` for live and library channels) creates
one. The response is not a finished state — it is a **receipt**, with a `Location` header pointing
at its status URL and a `disposition` saying whether the command was accepted, deduplicated or
rejected. The actual state is read from `GET /v2/playback/occurrences/{playbackId}`, which returns
the desired state, the plan, the media access and the revisions. This split is the whole point: the
command channel is ordered and idempotent, and the state is a separate, revision-fenced read that
any authorised controller can perform.

**Intent and observation flow in opposite directions.** A controlling client posts *intent* —
`POST /v2/playback/occurrences/{playbackId}/intent` with play, pause, seek, rate or track
selection. It does not command a renderer, because the renderer may be a different device
entirely. The renderer posts *observations* —
`POST /v2/playback/occurrences/{playbackId}/observations` with position, buffering, errors and
acquisition facts. Observations are evidence, and they are what advance watched state, resume
positions and history; the response to an observation is the occurrence's state after it was
applied, so a player needs no second read. A marker skip
(`POST /v1/playback/occurrences/{playbackId}/marker-skip`) is deliberately *not* a lane command: it
is recorded evidence about one occurrence, with no sequence, receipt or replay window. Channel
occurrences use their own `channel-intents` and `channel-observations` routes and never enter
personal state or finite queue completion.

**Queues sit on top and are where most playback actually starts.** A client creates a queue with
`PUT /v2/queues/{id}` (or fills it from a playlist), then calls `POST /v2/queues/{id}/playback`
naming an `entryId`, the queue's `expectedRevision`, a lane `sequence` and a `requestId`. The
server creates the occurrence, prepares delivery and answers **200 when it is ready or 202 when
`state` is `preparing`** — in the 202 case the client polls
`GET /v2/queues/{id}/playback/requests/{request}`. `GET /v2/queues/{id}/playback` returns the
combined view: the queue, the current occurrence, the next entry with its availability reason, the
media rows, and a `postPlay` block carrying the viewer's autoplay preference, up-next countdown and
pass-out accounting. `GET /v2/queues/{id}/events` long-polls the same view for up to two seconds
against an opaque cursor, so an idle client costs one request every two seconds rather than a tight
loop. Every queue read is bounded — 8 concurrent command slots and a separate 24 for event polls,
over which the server answers 429 `queue_busy` — and entries the viewer may not see are marked
`hidden` rather than removed, so positions stay stable while their media rows disappear.

---

## Paging

Paged reads take `cursor` and `limit` and return `nextCursor`. A `nextCursor` of `""` means the
listing is exhausted; anything else is opaque and must be sent back verbatim. **Never construct or
mutate a cursor** — an unrecognised one answers `400 invalid_cursor`. Default and maximum page
sizes vary by surface: most catalogue reads default to 40 and cap at 100, console and admin reads
cap at 40, search caps at 40, DVR defaults to 50, and chapter reads default to 100. Browse,
search and home clamp to their maxima; other catalogue lists reset an out-of-range `limit`
to the 40 default; console, admin, chapter, queue, playlist and people reads reject one
with `400`.

A page may contain fewer rows than `limit` and still return a `nextCursor`: rows the viewer may not
see are removed after the page is read. Do not infer "end of list" from a short page — only from an
empty `nextCursor`.

Composed surfaces re-check the viewer's permission fence *after* they finish composing. If the
fence changed mid-read — a library was shared, revoked, or a hosted restriction landed — the server
answers `409 stale_continuation` with `retryable: true` rather than returning a page built under
two different permission states. The correct response is to discard the cursor, re-read from the
start, and re-render. The same code appears when a paged continuation crosses a configuration
change on an admin surface.

Live TV uses a windowed guide instead: `GET /v1/guide` takes `start`, `end` and `limit`, and
returns `nextCursor` for channels within that window. A window larger than the server will build
answers `422 guide_window_too_large`; a cursor outstanding across a guide refresh answers
`409 guide_refresh_required`.

Servers advertising `features.guide_windowed_directory=enabled` offer two small navigation reads:
`GET /v1/guide/sources` provides all authorized source summaries, group/favorite counts and current
availability without programmes. `GET /v1/guide/channels?kind=all&sort=name&offset=0&limit=50`
provides globally sorted channel rows and an exact authorized total. Pass its `revision` on later
pages; if it changes, discard loaded positions and restart. Use `GET /v1/guide` with `channels`
and a time window only for programmes of visible channels, for live and Library Channels alike.
For channel up/down, use the same channel directory request with `direction=next` or `previous`
and `anchorChannelId`, `anchorSourceId`, `anchorProvenance` instead of an offset. The server
returns at most one watchable neighbor, wraps at the ends and preserves the selected view's
sorting and filters; never download the complete lineup just to change channels.

Older servers retain the original endpoint. Only an absent endpoint (404/405) justifies falling
back; permission, malformed response, overload and transient errors must remain visible.


---

## Errors

Every JSON error is the same envelope:

```json
{"error": {"code": "stale_continuation", "message": "…", "retryable": true}}
```

`code` is stable and machine-readable, `message` is already safe to display, and `retryable` says
whether repeating the *identical* request could succeed. Some handlers add sibling fields beside
`error`: `current` (the server's authoritative state, on a conflict), `serverId`, `viewerFence`,
`itemId`, `currentRevision`, `fields` (the rejected field paths on a validation failure) and
`serverTime`. Each operation in `openapi.yaml` lists the codes its handler can produce under
`x-error-codes`.

The `/v2` playback control routes wrap the envelope: `{"protocolVersion": "2.0", "error": {…}}`,
where the error additionally carries `stage` (where it failed) and `action` — `none`,
`retry_same_request`, `reconcile` or `reauthorize`. Act on `action`: `reconcile` means re-read the
controller or occurrence before trying again; `reauthorize` means get a fresh authorization first.

Two families of codes deserve specific handling:

- **Optimistic concurrency.** Anything that changes state takes an `expectedRevision` (or
  `expectedGeneration`, or `revision`) naming the revision the change was planned against. A
  mismatch answers 409 with a `*_conflict` code — `personal_state_conflict`, `playlist_conflict`,
  `queue_conflict`, `library_configuration_conflict`, `console_conflict`, `metadata_conflict`,
  `dvr_conflict`, and so on. Re-read, re-plan, retry. Several of these return the current state
  alongside the error so no second read is needed.
- **Idempotency.** Mutations also take an `operationId`, `requestId` or `idempotencyKey`. Replaying
  the same key returns the original receipt instead of applying the change twice; personal-state
  and saved-resource receipts are replayable for 30 days (`receiptLifetimeSeconds: 2592000`).
  Reusing a key for a *different* payload answers `operation_conflict` (or
  `queue_operation_conflict`), and a key older than its window answers `operation_expired`.

Back-pressure is explicit rather than silent. `429` with a `Retry-After` header means a rate limit
or an admission cap: `rate_limited` (120 personal writes per minute per profile, 20 authentication
attempts per minute per peer), `queue_busy`, `console_capacity`, `operations_busy`, `support_busy`,
`channel_busy`, `lyrics_capacity`, `conversion_capacity`. `503` with `retryable: true` means a
dependency is unavailable and the request is worth retrying: `segment_preparing`,
`stream_not_ready`, `playback_source_unavailable`, `mount_unavailable`, `remote_busy`,
`search_unavailable`, `session_logout_unavailable`. `500 persistence_error` never carries the
underlying cause; server-side failures keep their detail in the server log only.

---

## Recipes

The examples assume a development server at `http://localhost:32500` and use `jq`.

### 1. Sign in and list the libraries

```bash
BASE=http://localhost:32500

TOKEN=$(curl -sS -X POST "$BASE/v1/sessions" \
  -H 'Content-Type: application/json' \
  -d '{"username":"recovery-owner","password":"correct-horse-battery-staple"}' \
  | jq -r .accessToken)

curl -sS "$BASE/v1/libraries" -H "Authorization: Bearer $TOKEN" | jq .
```

```json
{
  "items": [
    { "id": "lib_movies", "name": "Movies", "kind": "movie", "defaultView": "grid" },
    { "id": "lib_shows",  "name": "Shows",  "kind": "tv",  "defaultView": "grid" }
  ]
}
```

The list is already filtered to what this viewer may see — a client must never filter libraries
itself. `defaultView` is the server's choice of first screen for that library; render it rather
than picking one.

### 2. Browse a library page by page

```bash
LIB=lib_movies
CURSOR=""

while :; do
  BODY=$(jq -nc --arg cursor "$CURSOR" '{limit:40,cursor:$cursor}')
  PAGE=$(curl -sS "$BASE/v1/libraries/$LIB/browse" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    --data "$BODY")

  echo "$PAGE" | jq -r '.items[] | "\(.id)\t\(.title)"'

  CURSOR=$(echo "$PAGE" | jq -r '.nextCursor // ""')
  [ -n "$CURSOR" ] || break
done
```

Stop on an empty `nextCursor`, never on a short page. If any request answers
`409 stale_continuation`, the viewer's permissions changed mid-walk: throw the cursor away and
start again from the beginning. For a screen a person will actually look at, prefer
`GET /v1/libraries/{id}/content`, which returns the heading, tabs, sorts, filters and composed rows
in one request.

### 3. Start playback of an item through a queue, and poll its state

```bash
ITEM=item_0001
SECRET=$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=')

# a) Register this installation as a controller.
CTRL=$(curl -sS -X POST "$BASE/v2/playback/controllers" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"protocolVersion\":\"2.0\",
       \"registrationRequestId\":\"reg-1\",
       \"installationId\":\"my-player-1\",
       \"installationSecret\":\"$SECRET\",
       \"retirePrevious\":null}")

CID=$(echo "$CTRL" | jq -r .controllerId)
EPOCH=$(echo "$CTRL" | jq -r .controllerEpoch)
AUTH=(-H "Authorization: Bearer $TOKEN" -H "X-Playback-Controller-Token: $SECRET")

# b) Establish a command lane on that epoch.
LANE=lane-1
curl -sS -X POST "$BASE/v2/playback/lanes" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d "{\"protocolVersion\":\"2.0\",
       \"controller\":{\"controllerId\":\"$CID\",\"controllerEpoch\":\"$EPOCH\"},
       \"commandLaneId\":\"$LANE\"}" > /dev/null

# c) Create a queue holding the item.
QUEUE=queue-1
SNAP=$(curl -sS -X PUT "$BASE/v2/queues/$QUEUE" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d "{\"controller\":{\"controllerId\":\"$CID\",\"controllerEpoch\":\"$EPOCH\",\"commandLaneId\":\"$LANE\"},
       \"entries\":[{\"id\":\"e1\",\"itemId\":\"$ITEM\",\"editionId\":null,\"partId\":null,
                     \"sourceContext\":{\"kind\":\"library\",\"id\":\"lib_movies\",
                                        \"revision\":null,\"entryId\":null},
                     \"hidden\":false,\"removed\":false}]}")

REV=$(echo "$SNAP" | jq -r .revision)

# d) Start the first entry. 200 = playing, 202 = still preparing.
START=$(curl -sS -w '\n%{http_code}' -X POST "$BASE/v2/queues/$QUEUE/playback" "${AUTH[@]}" \
  -H 'Content-Type: application/json' \
  -d "{\"controller\":{\"controllerId\":\"$CID\",\"controllerEpoch\":\"$EPOCH\",\"commandLaneId\":\"$LANE\"},
       \"command\":{\"requestId\":\"play-1\",\"sequence\":\"1\",\"expectedRevision\":\"$REV\",
                    \"entryId\":\"e1\",\"currentPlaybackId\":null,\"reason\":\"user\",
                    \"startSeconds\":null}}")

echo "$START" | head -n -1 | jq '{state, transitionId, stream: .session.streamUrl}'

# e) Poll the queue's playback view until the current occurrence is playing.
for _ in $(seq 1 30); do
  VIEW=$(curl -sS -G "$BASE/v2/queues/$QUEUE/playback" "${AUTH[@]}" \
    --data-urlencode "controllerId=$CID" \
    --data-urlencode "controllerEpoch=$EPOCH" \
    --data-urlencode "commandLaneId=$LANE")
  echo "$VIEW" | jq -c '{state: .current.state, next: .next.entryId, autoplay: .postPlay.autoplay}'
  [ "$(echo "$VIEW" | jq -r '.current.state // ""')" = "playing" ] && break
  sleep 1
done
```

Notes on the last step. The three controller parameters are required on the `GET` and **no other
query parameter is accepted**. To follow changes efficiently instead of polling on a timer, use
`GET /v2/queues/{id}/events` with `after` set to the previous response's `cursor`; it blocks for up
to two seconds and returns as soon as anything moves. Once playback is running, the renderer should
post observations to `POST /v2/playback/occurrences/{playbackId}/observations` — that is what
records progress, resume position and history — and read `session.streamUrl` for the actual media,
re-reading it after any renewal because the grant rotates.
