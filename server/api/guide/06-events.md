# 6. Changes while a screen is open

`GET /v1/events` is a bearer-authenticated invalidation feed. It tells a client
which resources changed; the client rereads them while retaining its current
screen. It is local server traffic, not push notification or email delivery.

## Streaming and polling

Without `waitSeconds`, the response is SSE (`text/event-stream`). Send the last
received SSE `id` as `Last-Event-ID` on reconnect. Do not put `after` in a
streaming query. For example:

```bash
curl -N -fsS "$BASE/v1/events" -H "Authorization: Bearer $TOKEN" \
  -H 'Accept: text/event-stream' -H 'Last-Event-ID: 1042'
```

The first idle connection emits `stream.opened` with its position. A
`stream.position` frame advances the resume point through events filtered out
for this viewer. Keep those IDs as well as the IDs of resource events.
Heartbeats arrive about every 25 seconds; a stream ends by 30 minutes or its
token deadline. Refresh the session and reconnect. One installation shares
one stream; a newcomer replaces its earlier stream.

A polling client uses
`GET /v1/events?after=1042&waitSeconds=25`. Wait is 0–25 seconds. The JSON answer
has `events`, `nextAfter` and `more`. Save `nextAfter` even when no visible event
was returned. Continue immediately while `more` is true, otherwise park again.
IDs are non-negative decimal positions, not inbox or resource revisions.

A cursor outside retained history produces `stream.resync`: quietly reread
visible resources and continue from the new feed position. A process restart,
permission change or lost stream must not make a client blank its cached screen.
Retry transient errors with backoff and the response's `Retry-After`.

## Resource hints

An event has `id`, `type`, `at`, a `resource` (`kind`, `id`), and where applicable
`revision` and `data`. Events are filtered by profile, device, library access
and owner/admin visibility. Known hints include:

- `notification.changed`: reread inbox and unread count.
- `library.scan.updated`, `library.changed`: reread relevant library content.
- `personal.changed`, `progress.changed`: reread viewer state and affected rows.
- `session.updated`, `queue.updated`, `admin.sessions`: reread playback resources.
- `settings.changed`: reread the affected settings document.
- `receiver.inbox_changed`: reread a receiver's inbox.
- `operation.updated`: reread a background operation.

Treat hints as coalescible invalidations, not a complete change log or a payload
for mutating local records. Ignore unknown event types safely. Source:
`internal/httpapi/events_routes.go`, `internal/eventfeed/feed.go`,
`internal/apievents/` and producers in the relevant services.

## Notifications and other streams

`GET /v1/notifications/inbox` is a paged inbox with its own revision; unread
count is `GET /v1/notifications/unread-count`. Apply read/archive operations
with `POST /v1/notifications/inbox/actions`. Its revision is separate from the
feed position. [Notifications](../notifications.md) documents its envelopes.

Watch Together groups have their own `/v1/groups/{id}/events` protocol, and
administrators can tail `/v1/admin/logs/events`. These are separate streams with
separate cursor and authority rules. Outbound [webhooks](09-webhooks.md) are
for integrations, not a replacement for the viewer's invalidation feed.
