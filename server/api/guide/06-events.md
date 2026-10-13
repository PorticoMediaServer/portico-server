# 6. Events

One feed, two transports. `GET /v1/events (openapi.yaml, operationId getEvents)`
is the viewer's unified invalidation feed; the notifications family carries the
same inbox content over SSE and long-poll for clients that cannot stream. Events
say *what* changed and its new revision; the client refetches the resource.

## The `/v1/events` feed

Bearer-authenticated. Without `waitSeconds` it is `text/event-stream`; with
`after=<eventId>&waitSeconds=<n>` (n ≤ 25) it is a long-poll returning
`{events, nextAfter}` immediately when anything is pending, else parked up to
`n` seconds. Resume with `Last-Event-ID` (stream) or `after` (long-poll); ids
are per-server monotonic sequences. Older than the ring yields a `stream.resync`
event — refetch what is visible.

<!-- example: events-longpoll-response file=openapi.yaml operationId=getEvents direction=response status=200 contentType=application/json schema=@response /v1/events get 200 -->
```json
{
  "events": [
    {
      "id": "1042",
      "type": "notification.changed",
      "at": "2026-09-24T12:00:00.000Z",
      "resource": { "kind": "inbox", "id": "profile" },
      "revision": "881"
    }
  ],
  "nextAfter": "1042"
}
```

The registered event types are `notification.changed`, `operation.updated`,
`library.scan.updated` and `stream.resync` with resources
`inbox|operation|library|stream`. Every signed-in viewer whose profile can see
a library receives that library's `library.scan.updated` events. Heartbeat
interval, maximum stream lifetime, per-installation exclusivity (`stream_exists`),
coalescing windows, per-client queue depth, explicit topic query (`queue:{id}`,
`group:{id}`, `session:{id}`), and `ETag`/`304` refetch are **not specified**
in the `getEvents` operation.

## Notifications

The inbox is `GET /v1/notifications/inbox (notifications.openapi.yaml,
operationId getNotificationInbox)` (revision-fenced, cursor-paged) with the
count at `GET /v1/notifications/unread-count (notifications.openapi.yaml,
operationId getNotificationUnreadCount)`:

<!-- example: events-inbox-response file=notifications.openapi.yaml operationId=getNotificationInbox direction=response status=200 schema=#/components/schemas/NotificationInbox -->
```json
{
  "revision": 881,
  "audience": "profile",
  "audiences": ["profile"],
  "state": "active",
  "counts": { "unread": 1, "read": 4, "archived": 0, "total": 5 },
  "items": [
    {
      "id": "notif_1",
      "audience": "profile",
      "severity": "info",
      "source": "broadcast",
      "category": "broadcast.info",
      "title": "Server updated",
      "body": "The server installed an update.",
      "arguments": {},
      "actions": [],
      "dedupeKey": "broadcast-2026-09-24",
      "revision": 881,
      "createdAt": 1758715200000,
      "updatedAt": 1758715200000,
      "expiresAt": 1759320000000,
      "readAt": null,
      "archivedAt": null,
      "read": false,
      "archived": false,
      "cursor": "cur_1"
    }
  ],
  "nextCursor": "",
  "observedAt": 1758715200000,
  "retentionDays": 30
}
```

Mutations go through `POST /v1/notifications/inbox/actions
(notifications.openapi.yaml, operationId applyNotificationActions)`
(`read|unread|archive|unarchive|read-all`, at most 8 operations, `ids` at most
200 — required for every action except `read-all`):

<!-- example: events-actions-request file=notifications.openapi.yaml operationId=applyNotificationActions direction=request schema=#/components/schemas/NotificationBatch -->
```json
{
  "operationId": "notif-op-0001",
  "operations": [
    { "action": "read", "ids": ["notif_1"] }
  ]
}
```

Live delivery shares `GET /v1/events` with library, progress, settings and
receiver hints. SSE ids are feed positions, separate from inbox revisions;
`Last-Event-ID` resumes the feed. A `notification.changed` event carries the
inbox revision and asks the client to quietly reread its inbox and counts.
`stream.resync` asks for a quiet reread after retained history is unavailable.
The long-poll fallback is the same endpoint with `after=<event-id>` and
`waitSeconds=0..25`; consume `{events,nextAfter,more}` and continue immediately
while `more` is true. One installation shares one feed across features.

Administration is `POST /v1/admin/notifications/broadcast
(notifications.openapi.yaml, operationId broadcastNotification)` with settings
at `GET /v1/admin/notifications/settings (notifications.openapi.yaml,
operationId getNotificationSettings)`.

Group streams and the separate admin log tail have their own contracts
(group SSE lives under social; the admin log tail is
`GET /v1/admin/logs/events (admin-access.openapi.yaml, operationId
tailMessageLog)`).
