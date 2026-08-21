import assert from "node:assert/strict";
import test from "node:test";
import {
  PORTICO_EVENT_ENVELOPE_VERSION,
  PorticoEventProtocolError,
  PorticoEventSubscriptionCoordinator,
  parsePorticoLongPollEnvelope,
  porticoEventFailureIsRetryable,
  porticoEventRetryDelay,
  runPorticoEventSubscription
} from "../dist/eventSubscriptions.js";
import { createMemorySessionStore, createPorticoClient } from "../dist/client.js";

function envelope(cursor, events = [], overrides = {}) {
  return {
    version: PORTICO_EVENT_ENVELOPE_VERSION,
    cursor,
    serverTime: "2026-07-17T18:42:31.123Z",
    resetRequired: false,
    hasMore: false,
    events,
    ...overrides
  };
}

function generationFence(state = { current: 1 }) {
  return { generation: state.current, currentGeneration: () => state.current };
}

function abortFailure() {
  const error = new Error("aborted");
  error.name = "AbortError";
  return error;
}

function jsonResponse(value, init = {}) {
  return new Response(JSON.stringify(value), {
    status: 200,
    headers: { "Content-Type": "application/json", ...(init.headers ?? {}) },
    ...init
  });
}

test("long-poll envelopes are versioned, bounded, and timestamped", () => {
  assert.deepEqual(parsePorticoLongPollEnvelope(envelope("cursor-1", [{ id: 1 }])), envelope("cursor-1", [{ id: 1 }]));
  assert.throws(() => parsePorticoLongPollEnvelope({ ...envelope("cursor-1"), version: "v2" }), /version/);
  assert.throws(() => parsePorticoLongPollEnvelope({ ...envelope("cursor-1"), cursor: "" }), /cursor/);
  assert.throws(() => parsePorticoLongPollEnvelope({ ...envelope("cursor-1"), serverTime: "later" }), /server time/);
  assert.throws(() => parsePorticoLongPollEnvelope({ ...envelope("cursor-1"), events: Array.from({ length: 501 }) }), /events/);
});

test("transport selection is deliberate and authorization failures never downgrade", async () => {
  let streams = 0;
  let polls = 0;
  const error = Object.assign(new Error("forbidden"), { status: 403 });
  await assert.rejects(runPorticoEventSubscription({
    transport: "sse",
    signal: new AbortController().signal,
    publicationFence: generationFence(),
    onEvent() {},
    onResetRequired() {}
  }, {
    async stream() { streams += 1; throw error; },
    async poll() { polls += 1; return envelope("unused"); },
    parseEvent: value => value
  }), error);
  assert.equal(streams, 1);
  assert.equal(polls, 0);
  assert.equal(porticoEventFailureIsRetryable({ status: 401 }), false);
  assert.equal(porticoEventFailureIsRetryable({ status: 429 }), true);
  assert.equal(porticoEventFailureIsRetryable({ status: 503 }), true);
});

test("SSE reconnect repairs authoritative state after the new stream is established and before frames publish", async () => {
  const controller = new AbortController();
  const order = [];
  let attempts = 0;
  await runPorticoEventSubscription({
    transport: "sse",
    signal: controller.signal,
    publicationFence: generationFence(),
    retryPolicy: { initialDelayMs: 0, maximumDelayMs: 0, jitterRatio: 0 },
    onEvent(event) {
      order.push(`event:${event.id}`);
      if (attempts === 2) controller.abort();
    },
    async onResetRequired(context) {
      assert.equal(context.transport, "sse");
      assert.equal(context.cursor, "");
      assert.equal(context.isCurrent(), true);
      order.push("reset");
    }
  }, {
    async stream(_signal, onEvent, onConnected) {
      attempts += 1;
      order.push(`connected:${attempts}`);
      await onConnected?.();
      onEvent({ id: "repeat" });
      if (attempts === 1) throw new TypeError("connection dropped");
    },
    async poll() { throw new Error("long-poll was not selected"); },
    parseEvent: value => value
  }, {
    random: () => 0.5,
    sleep: async () => undefined
  });
  assert.deepEqual(order, ["connected:1", "event:repeat", "connected:2", "reset", "event:repeat"]);
});

test("a failed long-poll reset does not advance the cursor or suppress the retried repair", async () => {
  const controller = new AbortController();
  const requests = [];
  const resetCursors = [];
  const sleeps = [];
  let resets = 0;
  await runPorticoEventSubscription({
    transport: "long-poll",
    signal: controller.signal,
    publicationFence: generationFence(),
    retryPolicy: { initialDelayMs: 10, maximumDelayMs: 100, jitterRatio: 0 },
    onEvent() { throw new Error("events must not publish before repair"); },
    async onResetRequired(context) {
      resets += 1;
      resetCursors.push(context.cursor);
      if (resets === 1) throw new TypeError("authoritative refetch failed");
      controller.abort();
    }
  }, {
    async stream() {},
    async poll(request) {
      requests.push(request);
      return envelope("replacement-cursor", [{ id: "discarded" }], { resetRequired: true });
    },
    parseEvent: value => value
  }, {
    sleep: async delay => { sleeps.push(delay); }
  });
  assert.equal(resets, 2);
  assert.deepEqual(requests.map(request => request.cursor), [undefined, undefined]);
  assert.deepEqual(resetCursors, ["replacement-cursor", "replacement-cursor"]);
  assert.deepEqual(sleeps, [10]);
});

test("an integrity-valid cursor from a prior server boot repairs authoritatively instead of terminating", async () => {
  const controller = new AbortController();
  const requests = [];
  const order = [];
  let polls = 0;
  await runPorticoEventSubscription({
    transport: "long-poll",
    signal: controller.signal,
    publicationFence: generationFence(),
    onEvent(event) {
      order.push(`event:${event.id}`);
      controller.abort();
    },
    async onResetRequired(context) {
      assert.equal(context.transport, "long-poll");
      assert.equal(context.cursor, "");
      order.push("reset");
    }
  }, {
    async stream() {},
    async poll(request) {
      requests.push(request);
      polls += 1;
      if (polls === 1) return envelope("old-boot-cursor");
      if (polls === 2) throw Object.assign(new Error("cursor no longer verifies"), { status: 400, code: "invalid_poll_cursor" });
      return envelope("new-boot-cursor", [{ id: "fresh" }]);
    },
    parseEvent: value => value
  });
  assert.deepEqual(requests.map(request => request.cursor), [undefined, "old-boot-cursor", undefined]);
  assert.deepEqual(order, ["reset", "event:fresh"]);
});

test("malformed transport payloads fail closed instead of entering a retry loop", async () => {
  let polls = 0;
  const sleeps = [];
  await assert.rejects(runPorticoEventSubscription({
    transport: "long-poll",
    signal: new AbortController().signal,
    publicationFence: generationFence(),
    onEvent() {},
    onResetRequired() {}
  }, {
    async stream() {},
    async poll() { polls += 1; return { ...envelope("cursor-1"), version: "unexpected" }; },
    parseEvent: value => value
  }, {
    sleep: async delay => { sleeps.push(delay); }
  }), PorticoEventProtocolError);
  assert.equal(polls, 1);
  assert.deepEqual(sleeps, []);
});

test("successful long-poll timeouts reissue immediately without backoff", async () => {
  const controller = new AbortController();
  const requests = [];
  const sleeps = [];
  const published = [];
  await runPorticoEventSubscription({
    transport: "long-poll",
    signal: controller.signal,
    publicationFence: generationFence(),
    onEvent(event) { published.push(event); controller.abort(); },
    onResetRequired() {}
  }, {
    async stream() { throw new Error("SSE was not selected"); },
    async poll(request) {
      requests.push(request);
      return requests.length < 3 ? envelope(`cursor-${requests.length}`) : envelope("cursor-3", [{ id: "ready" }]);
    },
    parseEvent: value => value
  }, {
    random: () => 0.5,
    sleep: async delay => { sleeps.push(delay); }
  });
  assert.deepEqual(requests.map(request => request.waitSeconds), [20, 20, 20]);
  assert.deepEqual(requests.map(request => request.cursor), [undefined, "cursor-1", "cursor-2"]);
  assert.deepEqual(published, [{ id: "ready" }]);
  assert.deepEqual(sleeps, []);
});

test("hasMore drains with zero wait and resetRequired refetches before continuing", async () => {
  const controller = new AbortController();
  const requests = [];
  const order = [];
  await runPorticoEventSubscription({
    transport: "long-poll",
    signal: controller.signal,
    publicationFence: generationFence(),
    onEvent(event) { order.push(`event:${event.id}`); controller.abort(); },
    async onResetRequired(context) {
      assert.equal(context.isCurrent(), true);
      order.push(`reset:${context.cursor}`);
    }
  }, {
    async stream() {},
    async poll(request) {
      requests.push(request);
      if (requests.length === 1) return envelope("reset-cursor", [{ id: "discarded" }], { resetRequired: true, hasMore: true });
      return envelope("event-cursor", [{ id: "authoritative" }]);
    },
    parseEvent: value => value
  });
  assert.deepEqual(requests, [
    { cursor: undefined, waitSeconds: 20 },
    { cursor: "reset-cursor", waitSeconds: 0 }
  ]);
  assert.deepEqual(order, ["reset:reset-cursor", "event:authoritative"]);
});

test("retry uses bounded jitter and honors Retry-After for temporary responses", async () => {
  const controller = new AbortController();
  const sleeps = [];
  let attempts = 0;
  await runPorticoEventSubscription({
    transport: "long-poll",
    signal: controller.signal,
    publicationFence: generationFence(),
    retryPolicy: { initialDelayMs: 100, maximumDelayMs: 1_000, jitterRatio: 0.2 },
    onEvent() { controller.abort(); },
    onResetRequired() {}
  }, {
    async stream() {},
    async poll() {
      attempts += 1;
      if (attempts === 1) throw { status: 503, retryAfterMs: 2_500 };
      if (attempts === 2) throw new TypeError("network unavailable");
      return envelope("ready", [{ id: 1 }]);
    },
    parseEvent: value => value
  }, {
    random: () => 1,
    sleep: async delay => { sleeps.push(delay); }
  });
  assert.deepEqual(sleeps, [2_500, 240]);
  assert.equal(porticoEventRetryDelay({ status: 429, retryAfterMs: 500_000 }, 0), 120_000);
});

test("abort cancels the outstanding request and prevents future reissue", async () => {
  const controller = new AbortController();
  let requests = 0;
  let requestSignal;
  const subscription = runPorticoEventSubscription({
    transport: "long-poll",
    signal: controller.signal,
    publicationFence: generationFence(),
    onEvent() {},
    onResetRequired() {}
  }, {
    async stream() {},
    poll(_request, signal) {
      requests += 1;
      requestSignal = signal;
      return new Promise((_resolve, reject) => signal.addEventListener("abort", () => reject(abortFailure()), { once: true }));
    },
    parseEvent: value => value
  });
  await new Promise(resolve => setTimeout(resolve, 0));
  controller.abort();
  await subscription;
  assert.equal(requests, 1);
  assert.equal(requestSignal.aborted, true);
});

test("abort settles a pending authoritative reset even when the reset handler ignores the signal", async () => {
  const controller = new AbortController();
  let resolveReset;
  let resetCalls = 0;
  let polls = 0;
  const resetPromise = new Promise(resolve => { resolveReset = resolve; });
  const subscription = runPorticoEventSubscription({
    transport: "long-poll",
    signal: controller.signal,
    publicationFence: generationFence(),
    onEvent() {},
    onResetRequired() {
      resetCalls += 1;
      return resetPromise;
    }
  }, {
    async stream() {},
    async poll() {
      polls += 1;
      return envelope("reset-cursor", [], { resetRequired: true });
    },
    parseEvent: value => value
  });
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(resetCalls, 1);
  controller.abort();
  await subscription;
  assert.equal(polls, 1);
  resolveReset();
});

test("abort settles a pending retry delay even when custom runtime sleep ignores the signal", async () => {
  const controller = new AbortController();
  let resolveSleep;
  let polls = 0;
  const subscription = runPorticoEventSubscription({
    transport: "long-poll",
    signal: controller.signal,
    publicationFence: generationFence(),
    retryPolicy: { initialDelayMs: 10, maximumDelayMs: 10, jitterRatio: 0 },
    onEvent() {},
    onResetRequired() {}
  }, {
    async stream() {},
    async poll() {
      polls += 1;
      throw new TypeError("offline");
    },
    parseEvent: value => value
  }, {
    sleep: async () => new Promise(resolve => { resolveSleep = resolve; })
  });
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(polls, 1);
  controller.abort();
  await subscription;
  resolveSleep();
});

test("viewer-generation fencing rejects a late poll publication", async () => {
  const state = { current: 7 };
  let resolvePoll;
  let publications = 0;
  let polls = 0;
  const subscription = runPorticoEventSubscription({
    transport: "long-poll",
    signal: new AbortController().signal,
    publicationFence: generationFence(state),
    onEvent() { publications += 1; },
    onResetRequired() { publications += 1; }
  }, {
    async stream() {},
    poll() {
      polls += 1;
      return new Promise(resolve => { resolvePoll = resolve; });
    },
    parseEvent: value => value
  });
  await new Promise(resolve => setTimeout(resolve, 0));
  state.current = 8;
  resolvePoll(envelope("late", [{ id: "late" }], { resetRequired: true }));
  await subscription;
  assert.equal(polls, 1);
  assert.equal(publications, 0);
});

test("idempotent identities suppress duplicate command publication after retry", async () => {
  const controller = new AbortController();
  const events = [];
  let polls = 0;
  await runPorticoEventSubscription({
    transport: "long-poll",
    signal: controller.signal,
    publicationFence: generationFence(),
    onEvent(event) { events.push(event.id); if (event.id === "command-2") controller.abort(); },
    onResetRequired() {}
  }, {
    async stream() {},
    async poll() {
      polls += 1;
      return polls === 1
        ? envelope("one", [{ id: "command-1" }, { id: "command-1" }])
        : envelope("two", [{ id: "command-1" }, { id: "command-2" }]);
    },
    parseEvent: value => value,
    eventIdentity: event => event.id
  });
  assert.deepEqual(events, ["command-1", "command-2"]);
});

test("subscription coordinator waits for an aborted predecessor before replacement", async () => {
  const coordinator = new PorticoEventSubscriptionCoordinator();
  const firstController = new AbortController();
  const secondController = new AbortController();
  let active = 0;
  let maximumActive = 0;
  const start = async signal => {
    active += 1;
    maximumActive = Math.max(maximumActive, active);
    try {
      await new Promise(resolve => signal.addEventListener("abort", resolve, { once: true }));
    } finally {
      active -= 1;
    }
  };
  const first = coordinator.run("same-stream", firstController.signal, start);
  await new Promise(resolve => setTimeout(resolve, 0));
  const second = coordinator.run("same-stream", secondController.signal, start);
  await new Promise(resolve => setTimeout(resolve, 0));
  secondController.abort();
  await Promise.all([first, second]);
  assert.equal(maximumActive, 1);
  assert.equal(coordinator.activeCount, 0);
});

test("client long-poll subscriptions use explicit sibling routes and JSON accept headers", async () => {
  const calls = [];
  const controller = new AbortController();
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (input, init) => {
      calls.push({ input: String(input), init });
      return jsonResponse(envelope("app-cursor", [{ id: 1, type: "data.changed", tags: ["home"], createdAt: "2026-07-17T18:42:31Z" }]));
    } }
  });
  const events = [];
  await client.subscribeAppEvents({
    transport: "long-poll",
    signal: controller.signal,
    publicationFence: generationFence(),
    onEvent(event) { events.push(event); controller.abort(); },
    onResetRequired() {}
  });
  assert.equal(calls[0].input, "https://server.example/api/events/poll?waitSeconds=20");
  assert.equal(calls[0].init.headers.Accept, "application/json");
  assert.deepEqual(events.map(event => event.tags), [["home"]]);
});

test("client resource subscriptions expose all constrained-client event surfaces", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (input) => {
      const url = String(input);
      calls.push(url);
      if (url.includes("/notifications/")) return jsonResponse(envelope("notifications", [{ version: "v1", kind: "notifications.invalidated", occurredAt: "2026-07-17T18:42:31Z" }]));
      if (url.includes("/command/events/")) return jsonResponse(envelope("command", [{ id: "command-1", action: "pause", issuedAt: "2026-07-17T18:42:31Z" }]));
      if (url.includes("/receivers/")) return jsonResponse(envelope("receiver", [{
        id: "receiver-1", name: "Living Room", code: "ABC12345", app: "roku", platform: "roku",
        supportedCommands: ["load"], command: { id: "", action: "" },
        createdAt: "2026-07-17T18:42:31Z", lastSeenAt: "2026-07-17T18:42:31Z"
      }]));
      return jsonResponse(envelope("group", [{
        id: "group-1", name: "Movie Night", mediaId: "movie-1", mediaTitle: "Movie", ownerName: "Host", ownerProfileId: "profile-1",
        members: [], queue: [], permissions: { canControl: true, canManageQueue: true, isHost: true }, command: {}, state: "paused",
        positionSeconds: 0, playbackRate: 1, revision: 1, playbackRevision: 1, reconnectGeneration: 1, repeatMode: "none",
        shuffleEnabled: false, createdAt: "2026-07-17T18:42:31Z", updatedAt: "2026-07-17T18:42:31Z",
        positionUpdatedAt: "2026-07-17T18:42:31Z", serverTime: "2026-07-17T18:42:31Z"
      }]));
    } }
  });
  const subscribe = async (start) => {
    const controller = new AbortController();
    await start({
      transport: "long-poll",
      signal: controller.signal,
      publicationFence: generationFence(),
      onEvent() { controller.abort(); },
      onResetRequired() {}
    });
  };
  await subscribe(options => client.subscribeViewerNotificationInvalidations({ audience: "profile" }, options));
  await subscribe(options => client.subscribePlaybackCommandEvents("session one", options));
  await subscribe(options => client.subscribePlaybackReceiverEvents("receiver one", options));
  await subscribe(options => client.subscribeWatchWithFriendsGroupEvents("group one", options));
  assert.deepEqual(calls, [
    "https://server.example/api/notifications/events/poll?audience=profile&waitSeconds=20",
    "https://server.example/api/playback-sessions/session%20one/command/events/poll?waitSeconds=20",
    "https://server.example/api/playback/receivers/receiver%20one/events/poll?waitSeconds=20",
    "https://server.example/api/watch-with-friends/groups/group%20one/events/poll?waitSeconds=20"
  ]);
});

test("client explicit SSE subscription uses the canonical stream route and never polls", async () => {
  const calls = [];
  const controller = new AbortController();
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (input, init) => {
      calls.push({ input: String(input), init });
      return new Response(null, { status: 200, headers: { "Content-Type": "text/event-stream" } });
    } },
    eventStream: {
      async *read() {
        yield "data: {\"id\":1,\"type\":\"data.changed\",\"createdAt\":\"2026-07-17T18:42:31Z\",\"tags\":[\"home\"]}\n\n";
      }
    }
  });
  await client.subscribeAppEvents({
    transport: "sse",
    signal: controller.signal,
    publicationFence: generationFence(),
    onEvent() { controller.abort(); },
    onResetRequired() {}
  });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].input, "https://server.example/api/events");
  assert.equal(calls[0].init.headers.Accept, "text/event-stream");
});

test("direct-route event streams carry the selected viewer bearer without putting it in the URL", async () => {
  const calls = [];
  const controller = new AbortController();
  const client = createPorticoClient({
    sessionStore: createMemorySessionStore({
      apiBaseUrl: "https://viewer.direct.getportico.tv",
      accessToken: "direct-access",
      refreshToken: "direct-refresh"
    }),
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return new Response(null, { status: 200, headers: { "Content-Type": "text/event-stream" } });
      }
    },
    eventStream: {
      async *read() {
        controller.abort();
      }
    }
  });
  await client.subscribeAppEvents({
    transport: "sse",
    signal: controller.signal,
    publicationFence: generationFence(),
    onEvent() {},
    onResetRequired() {}
  });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].input, "https://viewer.direct.getportico.tv/api/events");
  assert.equal(calls[0].init.headers.Authorization, "Bearer direct-access");
  assert.equal(new URL(calls[0].input).searchParams.has("accessToken"), false);
});
