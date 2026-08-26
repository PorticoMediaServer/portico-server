# Viewer Sync Coordinator

`ViewerSyncCoordinator` is the application-owned synchronization and query
boundary for one authenticated viewer generation. It prevents component
mounting, navigation, visibility changes, and reconnects from creating
additional physical subscriptions or parallel copies of the same query.

## Ownership and lifetime

Create exactly one coordinator after the final server-authenticated viewer
scope is known. Its generation fence must advance synchronously before profile
switch, sign-out, revocation, or authorization-policy replacement. Close the
old coordinator as part of `ViewerRuntimeAdapter.closeRealtime` and
`clearQueryCaches`; never reuse it for another profile.

Views call `registerResource()` and `leaseSubscription()`. A lease release is
not an instruction to reconnect: the coordinator retains a zero-consumer
logical stream briefly so a route or React component can remount without
resetting retry state. Inline callback identities may change across remounts;
the current physical connection remains authoritative and the newest driver is
used after its next reconnect.

Platform cache adapters may register the reserved `*` tag to receive every
batch once, then translate the batch into TanStack Query keys or an external
store. Product resources should prefer exact tags so unrelated views do not
refresh.

## Three workload priorities

Every adapter must preserve this order:

1. `playback-continuity`: established playback, player command/session events,
   and work required to keep buffered playback progressing;
2. `interactive`: the visible route and direct user actions;
   3. `background`: hidden screens, notification-inbox reconciliation, prefetch, and broad
   foreground reconciliation.

Ordinary app synchronization must never consume the reserved playback query
slots. `setPlaybackContinuityActive(true)` pauses background subscriptions and
aborts/defer background resource refresh while playback and interactive work
remain available. Resource adapters must honor the supplied abort signal. It
does not stop playback-priority subscriptions. Calling it with
`false` resumes one latest-wins authoritative refresh rather than replaying
every deferred event.

The coordinator is not a media transport. Segment, manifest, and byte-range
delivery should continue through the platform player and its dedicated server
workload lane. Do not put media reads into a background query merely to reuse
the cache.

## Platform integration

```ts
const sync = new ViewerSyncCoordinator({
  generationFence: {
    generation: viewerGeneration,
    currentGeneration: () => runtime.viewerGeneration
  },
  onLifecycleEvent(event) {
    // 401/403 is a terminal viewer transition, never a fetch retry.
    runtime.handleViewerLifecycle(event);
  }
});

const home = sync.registerResource({
  key: "home",
  tags: ["home", "viewer-state"],
  priority: "interactive",
  refresh: () => queryClient.invalidateQueries({ queryKey: ["home"] })
});

const applicationEvents = sync.leaseSubscription({
  key: "application",
  priority: "background",
  start: signal => client.subscribeAppEvents({
    transport: selectedTransport,
    signal,
    publicationFence,
    onEvent: event => publishViewerSyncEvent(sync, appEventAdapter, event),
    onResetRequired: () => refreshMountedAuthoritativeResources()
  })
});
```

- Web maps `visibilitychange` and network state to `setRuntimeState` and uses
  query-library invalidation inside registered resource refresh callbacks.
- React Native maps `AppState` and network reachability to `setRuntimeState`.
- Roku and other constrained clients keep their platform event/poll transport,
  but pass parsed events through the same semantic adapter and use logical
  leases rather than screen-owned retry loops.
- Player adapters call `setPlaybackContinuityActive` on established playback
  transitions and mark player command/session subscriptions as
  `playback-continuity`.

## Query behavior

`query()` provides viewer-generation fencing, per-key single-flight, bounded
weighted LRU storage, stale-if-error fallback for transient failures, global
in-flight limits, reserved playback capacity, and per-resource request-rate
circuit breaking. Abort by one consumer stops only that consumer's wait; the
shared load continues for other consumers. Closing the coordinator aborts the
underlying loads.

`401` and `403` never return stale data and never enter retry. They publish one
terminal lifecycle event and stop all synchronization for that viewer
generation. Network and temporary service failures may use a recent stale
value while the next event, foreground transition, or explicit user retry
performs a new authoritative read.

## Coverage matrix

| Concern | Shared guarantee | Platform responsibility |
| --- | --- | --- |
| Component remount | Logical lease and retry lifetime survive remount | Acquire/release leases |
| Invalidation storm | Tag dedupe, bounded batch, latest-wins refresh | Map wire events to canonical tags |
| Query fanout | Per-key single-flight and bounded in-flight work | Use stable viewer-scoped keys |
| Cache growth | Entry and weight bounds with LRU eviction | Provide realistic response weight |
| Reconnect | Exponential backoff retained by coordinator | Supply transport driver |
| Lost continuity | One authoritative reconciliation | Refresh mounted resources |
| Authorization | 401/403 terminates viewer generation | Transition to sign-in/profile recovery |
| Profile switch | Synchronous generation fence rejects late results | Advance generation before teardown |
| Background state | Streams pause; one reconcile on resume | Report foreground/network state |
| Playback load | Reserved capacity; background work defers | Mark playback active and keep media transport separate |
| Observability | Aggregate and bounded per-resource counters | Export/scrape without high-cardinality user data |

## Metrics and overload

`metrics()` reports aggregate connection, retry, invalidation, refresh, cache,
query, circuit, and eviction counts. `resourceMetrics()` provides a bounded
diagnostic breakdown by caller-defined logical key. Keys must describe product
resources, not user-entered search text, credentials, URLs, or media titles.

The circuit breaker is a client stability boundary, not user-facing rate-limit
copy. A recent cached value may remain visible while the circuit is open. If no
safe stale value exists, platforms may present their standard retryable failure
state; they must not start a second polling loop around the coordinator.
