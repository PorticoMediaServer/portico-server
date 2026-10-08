# Social and remote playback for third-party clients

Companion to `social.openapi.yaml`. That document is the schema; this one is the
behaviour you cannot read off a schema: how the group timeline works, what the
handoff state machine guarantees, and what this server deliberately does not do.

Everything here is normative for a third-party client. Nothing here requires a
first-party app: a group can be driven, a receiver can be paired and a handoff
can be completed with nothing but the documented endpoints.

## 1. How a group maps onto playback authority

A Watch Together group does **not** own a playback session, and members do not
share one.

A group binds to its host's **device**: the device the host's bearer token is
bound to, never a claim in a request body. The group's playback authority,
published as `group.authority`, is that `deviceId` and the Playback v1 session
the device is playing (`playbackId`, `state: bound`). With nothing playing the
device stays bound (`state: fresh`); once it is signed out the authority reports
`state: retired`.

Each member plays its own v1 session on its own device, with its own lease and
its own progress writer. This is not an implementation detail you may ignore:
exactly one writer per playback session is a hard invariant, and a shared session
would give a group of six one writer and five silent losers.

What the server owns is the **timeline**. What the client owns is **following
it**. Concretely:

* `POST /v1/groups/{id}/transport` moves the group timeline. It does not command
  anybody's player.
* Every member reads the timeline — from `GET /v1/groups/{id}` or from the event
  stream — and steers its own session onto it.
* A signed-out host device is the fence: the group then behaves exactly as it
  does for a disconnected host (pause, then end) unless the host returns on
  another device or transfers.
* Host transfer moves the role, and `reconnectGeneration` is bumped so members
  discard any in-flight correction computed against the old anchor. When the
  incoming host is the caller the group binds the caller's device at once;
  otherwise it follows no device until the new host's first heartbeat binds its
  own. A host's heartbeat always binds the device it comes from.

Group requests carry no playback authority, and like every v1 body they refuse
unknown fields. The seam is `social.Playback`, implemented over
`playback_v1_sessions` in `server/internal/playbackv1/social.go`.

## 2. The group timeline

The authoritative clock is four fields inside `group.timeline`:

```json
{
  "itemId": "item_7f2",
  "currentEntryId": "ent_9c1",
  "state": "playing",
  "anchorPositionUs": "184320000",
  "anchorAt": "2026-09-16T20:14:03.118Z",
  "rate": {"numerator": "1", "denominator": "1"},
  "queuePosition": 2
}
```

Your target position, while `state` is `playing`, is

```
target = anchorPositionUs + (serverTime - anchorAt) * rate
```

`serverTime` is on every response, so you can estimate your own offset from the
server's clock instead of trusting the device clock.

`group.sync` publishes the correction bands, so you do not hard-code them:

| drift from target | what you do |
| --- | --- |
| under `noCorrectionUnderMs` (750 ms) | nothing |
| `noCorrectionUnderMs` up to `seekAtOrOverMs` (3000 ms), while playing | correct by rate, bounded to `rateCorrectionMinimum`..`rateCorrectionMaximum` (0.90..1.10), for at most `rateCorrectionMaxMs` (4000 ms) |
| `seekAtOrOverMs` or more | authoritative seek, applied atomically with state |

When `state` is `paused` the anchor is the position: no extrapolation.

A late joiner loads the extrapolated timeline. It never restarts the group.

### Revisions and idempotency

Every authoritative command carries both:

* `expectedRevision` — the `group.revision` you last read. A stale value is
  refused with `409 revision_conflict` and the response names
  `error.currentRevision`. Re-read, re-decide, re-send. This is what stops a
  retried Play from undoing a Pause that landed in between.
* `idempotencyKey` — 1 to 120 characters, unique per member per group. A replay
  returns the stored receipt with `disposition: "duplicate"` and does not move
  the group again. The same key with a different body is `409
  idempotency_conflict` — never a silent overwrite. The window is the most recent
  128 keys per group.

Queue mutations fence on `queueRevision`, which is a separate counter from
`revision`, because a reorder and a seek are not the same kind of change.

### Host authority

`group.hostAuthority` is `host-only` or `anyone`.

`host-only` is the default and matches the sharing contract: the host alone
issues play, pause, seek, load, next, previous, stop, queue mutations and
shuffle/repeat. A participant's local Pause is a *request* to the host, not a
command — send it out of band, or ask the host to hand you authority.

`anyone` is this server's documented opt-in extension for small trusted groups.
It widens transport and queue commands to every joined member. It widens nothing
else: ending the group, transferring host authority, issuing invitations and
overriding visibility stay with the host.

`group.permissions` tells you what *you* may do; do not re-derive it.

### Readiness

Readiness is evidence about one member, never a command. `POST
/v1/groups/{id}/readiness` records `buffering`, `ready` or `lagging` plus your
position. The aggregate is the worst state present: `lagging` beats `buffering`
beats `ready`, and a report older than 30 seconds counts as *stale*, which holds
the aggregate away from `ready` just as `lagging` does.

A lobby promotes itself from `preparing` to `ready` only when every joined member
has reported `ready`. One member buffering never pauses the others — and the host
may start anyway. `ready` means "can start the exact media at the commanded
position with its current plan". An HTTP success or a metadata load is not
readiness; do not report it as one.

### The host-disconnect timeline

Measured from the host's last proof of life — a heartbeat, or any accepted host
command.

| elapsed | group state | what happens |
| --- | --- | --- |
| under 30 s | unchanged | `host.presence: connected` |
| 30 s to 120 s | `host-reconnecting-playing` / `host-reconnecting-paused` | playback continues; no new authoritative commands are expected |
| at 120 s | `host-reconnecting-paused` | the group pauses atomically and the clock is anchored at the extrapolated position |
| at 600 s | `ended` | `endedReason: host-unavailable` |

`host.pauseAt` and `host.endAt` publish both deadlines while the group is
reconnecting, so you can show a countdown instead of inventing one.

A host returning **before** the pause boundary restores the prior playing or
paused state. A host returning **after** it finds the group paused and must send
an explicit `play`. Both bump `reconnectGeneration`.

A member may claim host authority for itself — and only for itself — while the
group is in a `host-reconnecting-*` state. That is the recovery path when a host
does not come back.

The boundaries are wall-clock, not traffic-driven: they fire whether or not
anybody is polling.

### Shared queue visibility

A group queue can contain media not every member may see. The server never
resolves that by leaking and never by silently ejecting somebody.

An entry a member cannot see is reduced, for that member, to an opaque
placeholder:

```json
{"entryId": "ent_4b8", "position": 3, "itemId": null, "unavailable": true, "addedBy": ""}
```

`entryId` and `position` survive, so ordering and stable identity are preserved.
Nothing else does — no title, no artwork, no source, no owner, no provenance.

The **host** additionally receives `queue.eligibility`, naming which members are
blocked by which entries, and whether the current entry is one of them. A host is
expected to act on it: remove the member, or replace the entry, before that entry
becomes current.

The group refuses to start a blocked entry. `play`, `next`, `previous`, `load` and
`set-queue-position` all fail with `409 media_no_longer_accessible` and
`error.blockedMemberIds`. The host — never a member — may retry with
`allowUnavailable: true`. The override is recorded on the receipt as
`override: {"reason": "host-override", "blockedMemberIds": [...]}` and broadcast
to the whole group in the `group.transport` event, so it is never invisible.

A member may only add media it can see itself: adding something you cannot see is
`403 media_no_longer_accessible`.

### Invitations

The sharing contract prefers explicit invitations, and this server implements
them as short codes on top of that preference:

* Eight characters from `ABCDEFGHJKMNPQRSTUVWXYZ23456789` — no `I`, `L`, `O`, `0`
  or `1`, because a code gets read aloud.
* Default 15 minutes (60 s to 1 h), default 8 uses (1 to 32).
* Only the digest is stored. The plaintext is returned exactly once, from the
  issuing response.
* Issuance is host-only and rate limited to 12 per group per hour; redemption is
  rate limited to 20 attempts per viewer per hour.
* `recipientProfileId` binds a code to one profile. Use it. A bound code answers
  everybody else `403 invite_wrong_recipient`.
* The state-specific codes — `invite_expired`, `invite_revoked`,
  `invite_consumed` — are only returned once the recipient binding is satisfied.
  An unbound guesser learns only `invite_not_found`.

Group ceiling: 32 joined members. Queue ceiling: 256 entries.

### The event stream

`GET /v1/groups/{id}/events`, `text/event-stream`. Membership is checked before
one byte is written, so a `403` here is a real answer and not a truncated stream.

```
id: 6
event: group.snapshot
data: {"protocolVersion":"1.0","serverTime":"...","group":{...}}

id: 7
event: group.transport
data: {"command":"play","state":"playing","anchorPositionUs":"184320000","anchorAt":"...","revision":"12","override":null}

event: heartbeat
data: {"serverTime":"2026-09-16T20:14:18.004Z"}
```

Rules that matter:

* `id:` is the durable ledger ordinal. Send it back as `Last-Event-ID` to resume.
  `group.eventOrdinal` in any snapshot is also a valid resume point.
* The opening `group.snapshot` carries the ordinal it was taken at, so echoing
  back the last `id:` you saw is always a correct resume — you never have to read
  `group.eventOrdinal` out of the body, though it is the same number.
* `heartbeat` and `resume-gap` carry **no** `id:`, so neither moves your resume
  point. Heartbeats arrive every 15 seconds; treat a long silence as a dead
  connection.
* Retention is the most recent 512 events per group. A resume point older than
  that gets one `resume-gap` frame and then a fresh `group.snapshot`; discard
  what you had and adopt the snapshot.
* `group.queue` deliberately carries no entries, only `queueRevision`. The queue
  is projected per member, so a shared event body cannot be correct for
  everybody. Re-read `GET /v1/groups/{id}/queue`.
* `group.members` likewise carries only `revision`; re-read the group.
* `group.ended` is the last frame. The server closes the stream.

There is no long-poll fallback in this server: resume by ordinal covers the
reconnect case that a cursor-based poll was designed for, and a second transport
would be a second place for the retention rule to be wrong.

## 3. The handoff state machine

Portico-to-Portico handoff moves playback from a controller's own session to a
first-party receiver — a TV app — without either side guessing.

### Registration and grants

A receiver registers with a stable `deviceId`, a `displayName` and a
`keyFingerprint` (base64url SHA-256 of its public key). The fingerprint is the
fence: re-registering with a different one **revokes every open grant**, **fails
every handoff in flight**, and advances `authorizationRevision`. A credential
minted against an old key can never drive a rotated one.

A controller asks for a grant with `POST /v1/receivers/{id}/grants`. The
receiver's `grantPolicy` decides what happens:

* `open` — accepted immediately.
* `per-device` (default) — a controller that already holds a live accepted grant
  is renewed; a new device lands in `pending` for the receiver to decide. This is
  "accept once per device".
* `always-ask` — every request lands in `pending`, including a returning device.

`allowedCommands` is the intersection of what the controller asked for and what
the receiver declared, and must include `load`. Grants live 15 minutes and are
renewed on each accepted request.

### How a receiver finds out

A receiver waits on one read: `GET /v1/receivers/{id}/inbox`. It returns the grant
requests still `pending` for this receiver and the handoffs addressed to it that are
still open (`prepared` or `committing`, not yet expired), oldest first. Each handoff
carries the `itemId` and `requestedPositionUs` to load. It is a snapshot read that takes
no write gate, so polling it every few seconds while idle is the intended use; presence
is still proved by `POST /v1/receivers/{id}/heartbeat`. Another viewer asking for the
same receiver gets `404`.

A receiver that cannot hold the notifications event stream open waits on
`GET /v1/receivers/{id}/inbox/wait?waitSeconds=25` instead. It answers at once when the
inbox holds a pending grant or an open handoff, and otherwise holds the request until one
arrives or the wait (0 to 55 seconds, 25 by default) elapses, then answers with the inbox
as it is. The body is the plain read's; the receiver asks again as soon as it has an
answer. A viewer may hold four such reads at a time (`429 rate_limited` beyond that).

### The three phases

```
                 POST /v1/handoffs
   (source live) ─────────────────▶ state: prepared   outcome: waiting
                                        │
                POST …/{id}/readiness    │ receiver proves readiness == "playing"
                                        ▼
                                   state: prepared   outcome: pending
                                        │
                POST …/{id}/commit       │ controller commits
                                        ▼
                                   state: committed  outcome: accepted
                                   source retired at committedPositionUs

   any of: POST …/rollback, deadline passed, grant revoked, receiver key rotated
                                        ▼
              state: rolled_back | expired | failed     outcome: rejected
                                   source untouched, still playing
```

Three facts you can build on:

1. **Readiness is not acceptance.** A receiver reporting ready moves `outcome`
   from `waiting` to `pending` and changes nothing else. The source is still
   live, still authoritative, still the progress writer. Only `commit` ends
   it (end reason `transferred`). The only accepted readiness value is `playing`; a buffered first segment
   or an HTTP 200 is not readiness.
2. **Commit is fenced twice.** It carries the handoff `revision` the controller
   last read, and it re-reads the source session: a source that has since ended
   settles the handoff as `failed` rather than ending something twice.
   A retried commit returns the same commit.
3. **Failure returns to the retained source.** The source was never retired, so
   there is nothing to restore. Rollback, expiry (120 seconds), a revoked grant
   and a rotated receiver key all discard the prepared target and leave the
   source exactly where it was.

Only one open handoff per source session: two prepared proposals racing for
one terminal is exactly the ambiguity this design exists to prevent. A retried
`requestId` returns the same handoff; the same `requestId` with a different
proposal is `409 handoff_request_conflict`.

`committedPositionUs` is the position the *receiver* proved in phase two, not the
position the controller asked for in phase one. The receiver begins where it
actually is.

### Worked exchange

```json
POST /v1/handoffs
{"protocolVersion":"1.0","requestId":"handoff-7f2","receiverId":"rcv_a1",
 "grantId":"grt_b2","sourcePlaybackId":"pb_c3","startPositionUs":"184320000",
 "expectedPlaybackRevision":"4"}

201 {"protocolVersion":"1.0","serverTime":"...","handoff":{
  "id":"hdf_d4","state":"prepared","outcome":"waiting","revision":"1",
  "sourcePlaybackId":"pb_c3","requestedPositionUs":"184320000",
  "sourceRetired":false,"expiresAt":"2026-09-16T20:16:03Z"}}

POST /v1/handoffs/hdf_d4/readiness
{"protocolVersion":"1.0","readiness":"playing","receiverPlaybackId":"pb_e5",
 "positionUs":"184380000","expectedRevision":"1"}

200 {"handoff":{"state":"prepared","outcome":"pending","revision":"2",
  "readyPositionUs":"184380000","sourceRetired":false, ...}}

POST /v1/handoffs/hdf_d4/commit
{"protocolVersion":"1.0","expectedRevision":"2"}

200 {"handoff":{"state":"committed","outcome":"accepted","revision":"3",
  "committedPositionUs":"184380000","sourceRetired":true, ...}}
```

## 4. Google Cast

The receiver page has no Portico credential and cannot safely be handed one over
the Cast channel, so pairing is a short code the viewer reads off the TV.

1. The sender calls `POST /v1/cast/bootstrap` and displays `bootstrap.code` — six
   characters, five minutes, one use, one pending code per viewer, digest-only
   storage.
2. The receiver page at `GET /receiver/cast/` posts that code to
   `POST /v1/cast/redeem` — the only unauthenticated route here, because the
   receiver has nothing to authenticate with yet.
3. Redemption returns **two different credentials**, deliberately:
   * `deviceToken` — device-scoped, long-lived, accepted **only** by
     `POST /v1/cast/reconnect`. It is never a bearer, so a leaked device token
     cannot be replayed against the media API.
   * `session.accessToken` — an ordinary short-lived viewer bearer. The receiver
     plays with this, through `POST /v1/playback/sessions`, the returned
     session's `presentation.url`, and `POST /v1/playback/sessions/{id}/timeline`
     progress reports. Playback is v1-only; there are no v2 playback routes.
     The receiver has no private playback surface at all.
4. `POST /v1/cast/reconnect` renews after a restart and **rotates the device
   token every time**. A captured token is good for exactly one renewal, and the
   legitimate receiver discovers the theft at its next reconnect.
   `grantSemantics` is `initial` on redemption and `rotation` on reconnect.
   A reconnect whose answer never arrived leaves the receiver holding the token
   it replaced. That token, sent with the receiver's own `deviceId`, reconnects
   the same device for two minutes after the rotation or until the new token is
   used, whichever is first. Each device holds one live session: the one it was
   last issued. Earlier ones end when a new one is issued, and all of them end
   when the pairing is revoked.

The code is consumed, the device token stored and the session issued in one
transaction, so a code is never spent on a session that couldn't be issued. A
Portico Account viewer's session is issued under the server's current Hosted
policy like any other; a viewer that policy doesn't admit is refused at
`bootstrap`, before a code is shown.

Failed attempts (unknown, used, cancelled or expired codes and device tokens)
are budgeted at 30 per source network per hour and 1,000 per server per hour,
recorded in a transaction of their own so a refusal can't undo its own charge
and a restart doesn't reset it. Successful attempts aren't charged. Both routes
also sit behind the per-address sign-in limiter.

A paired Cast device also appears in the viewer's receiver directory with
`kind: "cast"`, so one directory answers "where can I send this?".

The Cast application id is the setting `cast.applicationId`, readable at
`GET /v1/cast/configuration` and writable by the server owner at
`PUT /v1/cast/configuration`. **It has no default.** An empty value means this
server has not been registered with the Google Cast console, and a sender must
not advertise Cast — a Cast button that connects to nothing is worse than no
button.

The receiver application is a static HTML and JavaScript bundle under
`server/receiver/cast/`, compiled into the server binary and served from
`/receiver/cast/`. There is no build step, so every platform target and the
Docker image serve identical bytes, and no deployment has to publish the page
somewhere else. Its only external resource is Google's Cast receiver framework
script; it fetches media, credentials and progress exclusively from the Portico
server that served it.

## 5. What this server does not do

**AirPlay is client-only. No server work exists for it, by design.** AirPlay uses
the originating-player model: the system route picker moves audio and video, and
the Portico session that started playback stays the playback and progress
authority. There is no Portico credential to hand an arbitrary AirPlay endpoint
and no server state to keep, so there is no AirPlay endpoint in this document.
Backgrounding is not Stop; route loss is a recoverable pause, not a terminal
state. If a future AirPlay target ever runs a first-party Portico receiver with
its own credentials, it follows §3 above rather than this model.

**DLNA is a deliberate omission.** It is not in the sharing and social playback
contract, and it is not implemented here. That is a choice, not an oversight, and
these are the reasons:

* DLNA/UPnP has no viewer identity. A renderer on the LAN is anonymous, so there
  is no profile to scope a library to, no progress owner and no per-viewer
  visibility — the three things every other surface in this document is built on.
  A DLNA path would need a parallel, weaker authorization model beside the one
  the rest of the server enforces.
* It has no readiness or commit step, so the two-phase guarantee in §3 cannot be
  offered. A DLNA `SetAVTransportURI` is a hope, not a handoff.
* It requires an unauthenticated, long-lived media URL on the LAN. Every other
  media path here is a short-lived grant bound to a session.
* It is progressive-download only in practice, which means no adaptive delivery,
  no subtitle selection and no quality ladder.

If DLNA is ever added it belongs behind an explicit server-owner opt-in, scoped
to named libraries, with its own documented authorization model — not as an
extension of the endpoints above.
