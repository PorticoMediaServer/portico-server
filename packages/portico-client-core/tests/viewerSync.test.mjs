import assert from "node:assert/strict";
import test from "node:test";
import {
  ViewerSyncCircuitOpenError,
  ViewerSyncCoordinator,
  publishViewerSyncEvent
} from "../dist/viewerSync.js";

const turn = () => new Promise(resolve => setTimeout(resolve, 0));
const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));

function coordinator(overrides = {}) {
  const state = { generation: 1 };
  const lifecycle = [];
  const instance = new ViewerSyncCoordinator({
    generationFence: { generation: 1, currentGeneration: () => state.generation },
    onLifecycleEvent: event => lifecycle.push(event),
    coalesceWindowMs: 0,
    subscriptionLingerMs: 10,
    ...overrides
  });
  return { instance, state, lifecycle };
}

test("resource invalidations are tag-selective and identical events coalesce", async () => {
  const timers = [];
  const { instance } = coordinator({
    coalesceWindowMs: 25,
    runtime: {
      setTimer(callback) { timers.push(callback); return callback; },
      clearTimer(handle) { const index = timers.indexOf(handle); if (index >= 0) timers.splice(index, 1); }
    }
  });
  const home = [];
  const notifications = [];
  const all = [];
  instance.registerResource({ key: "home", tags: ["home", "viewer-state"], refresh: batch => home.push([...batch.tags]) });
  instance.registerResource({ key: "notifications", tags: ["notifications"], refresh: batch => notifications.push([...batch.tags]) });
  instance.registerResource({ key: "platform-cache-adapter", tags: ["*"], refresh: batch => all.push([...batch.tags]) });
  instance.invalidate(["home"]);
  instance.invalidate(["home", "viewer-state"]);
  assert.equal(timers.length, 1);
  timers.shift()();
  await turn();
  assert.deepEqual(home, [["home", "viewer-state"]]);
  assert.deepEqual(notifications, []);
  assert.deepEqual(all, [["home", "viewer-state"]]);
  assert.equal(instance.metrics().invalidationBatches, 1);
  assert.equal(instance.metrics().authoritativeRefreshes, 2);
});

test("resource refreshes are latest-wins rather than parallel", async () => {
  const { instance } = coordinator();
  const sequences = [];
  let release;
  instance.registerResource({
    key: "home",
    tags: ["home"],
    async refresh(batch) {
      sequences.push(batch.sequence);
      if (sequences.length === 1) await new Promise(resolve => { release = resolve; });
    }
  });
  instance.invalidate(["home"], "immediate");
  await turn();
  instance.invalidate(["home"], "immediate");
  instance.invalidate(["home"], "immediate");
  release();
  await turn();
  assert.deepEqual(sequences, [1, 3]);
});

test("logical subscription leases survive remounts without duplicate physical connections", async () => {
  const { instance } = coordinator({ subscriptionLingerMs: 30 });
  let starts = 0;
  let active = 0;
  let maximumActive = 0;
  const start = signal => {
    starts += 1;
    active += 1;
    maximumActive = Math.max(maximumActive, active);
    return new Promise(resolve => signal.addEventListener("abort", () => { active -= 1; resolve(); }, { once: true }));
  };
  const first = instance.leaseSubscription({ key: "application", start });
  await turn();
  first.release();
  const replacement = instance.leaseSubscription({ key: "application", start });
  await turn();
  assert.equal(starts, 1);
  assert.equal(maximumActive, 1);
  replacement.release();
  await delay(40);
  assert.equal(active, 0);
  assert.equal(instance.metrics().activeLogicalSubscriptions, 0);
});

test("subscription retry state belongs to the coordinator and terminal auth never loops", async () => {
  const terminal = coordinator({
    subscriptionRetryPolicy: { initialDelayMs: 0, maximumDelayMs: 0, jitterRatio: 0 }
  });
  let attempts = 0;
  terminal.instance.leaseSubscription({
    key: "notifications",
    async start() {
      attempts += 1;
      throw Object.assign(new Error("expired"), { status: 401 });
    }
  });
  await turn();
  await turn();
  assert.equal(attempts, 1);
  assert.equal(terminal.lifecycle.length, 1);
  assert.equal(terminal.lifecycle[0].reason, "unauthenticated");
  assert.equal(terminal.instance.metrics().subscriptionRetries, 0);

  const retrying = coordinator({
    subscriptionRetryPolicy: { initialDelayMs: 1, maximumDelayMs: 2, jitterRatio: 0 }
  });
  let retryAttempts = 0;
  const lease = retrying.instance.leaseSubscription({
    key: "application",
    async start(signal) {
      retryAttempts += 1;
      if (retryAttempts < 3) throw new TypeError("offline");
      await new Promise(resolve => signal.addEventListener("abort", resolve, { once: true }));
    }
  });
  await delay(15);
  assert.equal(retryAttempts, 3);
  assert.equal(retrying.instance.metrics().subscriptionRetries, 2);
  lease.release();
  retrying.instance.close();
});

test("endpoint-scoped permanent subscription failures halt without a hot loop", async () => {
  const { instance, lifecycle } = coordinator({
    subscriptionRetryPolicy: { initialDelayMs: 0, maximumDelayMs: 0, jitterRatio: 0 }
  });
  let attempts = 0;
  const first = instance.leaseSubscription({
    key: "application",
    async start() {
      attempts += 1;
      throw Object.assign(new Error("feature forbidden"), { status: 403, code: "feature_not_allowed" });
    }
  });
  await turn();
  await turn();
  assert.equal(attempts, 1);
  assert.equal(lifecycle.length, 0);
  assert.equal(instance.metrics().subscriptionRetries, 0);

  first.release();
  await delay(15);
  const replacement = instance.leaseSubscription({
    key: "application",
    async start(signal) {
      attempts += 1;
      await new Promise(resolve => signal.addEventListener("abort", resolve, { once: true }));
    }
  });
  await turn();
  assert.equal(attempts, 2);
  replacement.release();
  instance.close();
});

test("query coordinator provides bounded cache and single-flight", async () => {
  let now = 10;
  const { instance } = coordinator({
    maximumCacheEntries: 2,
    defaultCacheMaxAgeMs: 100,
    runtime: { now: () => now }
  });
  let resolve;
  let loads = 0;
  const request = {
    key: "home",
    load: async () => { loads += 1; return new Promise(done => { resolve = done; }); }
  };
  const first = instance.query(request);
  const second = instance.query(request);
  assert.equal(loads, 1);
  resolve({ rows: 2 });
  assert.deepEqual(await first, { rows: 2 });
  assert.deepEqual(await second, { rows: 2 });
  assert.deepEqual(await instance.query({ ...request, load: async () => { throw new Error("must not load"); } }), { rows: 2 });
  await instance.query({ key: "two", load: async () => 2 });
  now += 1;
  await instance.query({ key: "three", load: async () => 3 });
  assert.equal(instance.metrics().cacheEntries, 2);
  assert.equal(instance.metrics().querySingleflightJoins, 1);
  assert.equal(instance.metrics().queryCacheHits, 1);
  assert.equal(instance.metrics().cacheEvictions, 1);
});

test("stale-if-error remains visible while terminal failures signal lifecycle", async () => {
  let now = 0;
  const { instance, lifecycle } = coordinator({
    defaultCacheMaxAgeMs: 5,
    defaultStaleIfErrorMs: 100,
    runtime: { now: () => now }
  });
  await instance.query({ key: "preferences", load: async () => "saved" });
  now = 10;
  assert.equal(await instance.query({ key: "preferences", load: async () => { throw new TypeError("network"); } }), "saved");
  instance.invalidateQuery("preferences");
  await assert.rejects(instance.query({
    key: "preferences",
    load: async () => { throw Object.assign(new Error("forbidden"), { status: 403, code: "profile_access_revoked" }); }
  }), /forbidden/);
  assert.equal(lifecycle.length, 1);
  assert.equal(lifecycle[0].reason, "forbidden");
});

test("generation fencing rejects a late failed query instead of returning old stale data", async () => {
  const { instance, state } = coordinator();
  await instance.query({ key: "preferences", load: async () => "old-profile" });
  instance.invalidateQuery("preferences");
  let rejectLoad;
  const pending = instance.query({
    key: "preferences",
    load: () => new Promise((_resolve, reject) => { rejectLoad = reject; })
  });
  state.generation = 2;
  rejectLoad(new TypeError("old profile request failed"));
  await assert.rejects(pending, error => error.name === "AbortError");
  assert.equal(instance.metrics().queryStaleFallbacks, 0);
});

test("closing the coordinator cancels a query wait even when its loader ignores abort", async () => {
  const { instance } = coordinator();
  let finish;
  const pending = instance.query({
    key: "slow",
    load: () => new Promise(resolve => { finish = resolve; })
  });
  instance.close();
  await assert.rejects(pending, error => error.name === "AbortError");
  finish("late");
  await turn();
});

test("terminal lifecycle aborts active resource refreshes and blocks later invalidations", async () => {
  const { instance, lifecycle } = coordinator();
  let aborted = false;
  let refreshes = 0;
  instance.registerResource({
    key: "home",
    tags: ["home"],
    refresh: (_batch, signal) => new Promise(resolve => {
      refreshes += 1;
      signal.addEventListener("abort", () => {
        aborted = true;
        resolve();
      }, { once: true });
    })
  });
  instance.invalidate(["home"], "immediate");
  await turn();
  assert.equal(refreshes, 1);
  await assert.rejects(instance.query({
    key: "auth",
    load: async () => { throw Object.assign(new Error("profile revoked"), { status: 403, code: "profile_access_revoked" }); }
  }), /profile revoked/);
  assert.equal(aborted, true);
  assert.equal(lifecycle.length, 1);
  instance.invalidate(["home"], "immediate");
  await turn();
  assert.equal(refreshes, 1);
});

test("endpoint-level forbidden failures do not terminalize the viewer", async () => {
  const { instance, lifecycle } = coordinator();
  await assert.rejects(instance.query({
    key: "feedback",
    load: async () => { throw Object.assign(new Error("disabled"), { status: 403, code: "feedback_not_allowed" }); }
  }), /disabled/);
  assert.equal(lifecycle.length, 0);
  assert.equal(await instance.query({ key: "home", load: async () => "available" }), "available");
});

test("query invalidation epochs prevent an older in-flight load from resurrecting cache", async () => {
  const { instance } = coordinator();
  let complete;
  const first = instance.query({ key: "home", load: () => new Promise(resolve => { complete = resolve; }) });
  instance.invalidateQuery("home");
  complete("obsolete");
  assert.equal(await first, "obsolete");
  let reloads = 0;
  assert.equal(await instance.query({ key: "home", load: async () => { reloads += 1; return "fresh"; } }), "fresh");
  assert.equal(reloads, 1);
});

test("prefix and global invalidation fence matching in-flight query writes", async () => {
  const { instance } = coordinator();
  let finishHome;
  let finishSettings;
  const home = instance.query({ key: "home:rows", load: () => new Promise(resolve => { finishHome = resolve; }) });
  const settings = instance.query({ key: "settings", load: () => new Promise(resolve => { finishSettings = resolve; }) });
  instance.invalidateQueries("home:");
  instance.invalidateQueries();
  finishHome("old-home");
  finishSettings("old-settings");
  await Promise.all([home, settings]);
  assert.equal(instance.metrics().cacheEntries, 0);
});

test("request multiplication opens a recoverable per-resource circuit", async () => {
  let now = 0;
  const { instance } = coordinator({
    maximumRequestsPerResourceWindow: 2,
    requestRateWindowMs: 1_000,
    circuitOpenMs: 500,
    defaultCacheMaxAgeMs: 0,
    defaultStaleIfErrorMs: 0,
    runtime: { now: () => now }
  });
  await instance.query({ key: "search", load: async () => 1 });
  instance.invalidateQuery("search");
  await instance.query({ key: "search", load: async () => 2 });
  instance.invalidateQuery("search");
  await assert.rejects(instance.query({ key: "search", load: async () => 3 }), ViewerSyncCircuitOpenError);
  now = 501;
  assert.equal(await instance.query({ key: "search", load: async () => 4 }), 4);
  assert.equal(instance.metrics().circuitRejections, 1);
});

test("viewer generation fences late queries and semantic adapters share invalidation vocabulary", async () => {
  const { instance, state } = coordinator();
  const refreshed = [];
  instance.registerResource({ key: "home", tags: ["home"], refresh: batch => refreshed.push(batch.sequence) });
  publishViewerSyncEvent(instance, {
    subscriptionKey: "application",
    tagsForEvent: event => event.tags,
    urgencyForEvent: () => "immediate"
  }, { tags: ["home"] });
  await turn();
  assert.deepEqual(refreshed, [1]);
  let complete;
  const pending = instance.query({ key: "late", load: async () => new Promise(resolve => { complete = resolve; }) });
  state.generation = 2;
  complete("old-profile-data");
  await assert.rejects(pending, error => error.name === "AbortError");
  assert.equal(instance.metrics().cacheEntries, 0);
});

test("foreground and online state suspend streams and reconcile resources once restored", async () => {
  const { instance } = coordinator({ subscriptionLingerMs: 0 });
  let starts = 0;
  let refreshes = 0;
  instance.registerResource({ key: "home", tags: ["home"], refresh: () => { refreshes += 1; } });
  instance.setRuntimeState({ foreground: false });
  instance.leaseSubscription({
    key: "application",
    start: signal => new Promise(resolve => { starts += 1; signal.addEventListener("abort", resolve, { once: true }); })
  });
  await turn();
  assert.equal(starts, 0);
  instance.setRuntimeState({ foreground: true, online: true });
  await turn();
  assert.equal(starts, 1);
  assert.equal(refreshes, 1);
  instance.close();
});

test("playback continuity retains reserved capacity and interactive refresh outranks background work", async () => {
  const { instance } = coordinator({
    maximumInflightQueries: 2,
    reservedPlaybackQuerySlots: 1,
    maximumBackgroundInflightQueries: 1
  });
  const order = [];
  let finishInteractive;
  instance.registerResource({
    key: "visible-home",
    tags: ["home"],
    priority: "interactive",
    async refresh() {
      order.push("interactive:start");
      await new Promise(resolve => { finishInteractive = resolve; });
      order.push("interactive:end");
    }
  });
  instance.registerResource({
    key: "hidden-settings",
    tags: ["home"],
    priority: "background",
    refresh() { order.push("background"); }
  });
  instance.invalidate(["home"], "immediate");
  await turn();
  assert.deepEqual(order, ["interactive:start"]);
  finishInteractive();
  await turn();
  assert.deepEqual(order, ["interactive:start", "interactive:end", "background"]);

  let finishInteractiveQuery;
  const interactive = instance.query({
    key: "active-route",
    priority: "interactive",
    load: () => new Promise(resolve => { finishInteractiveQuery = resolve; })
  });
  await assert.rejects(instance.query({
    key: "hidden-route",
    priority: "background",
    load: async () => "hidden"
  }), error => error.name === "ViewerSyncOverloadedError");
  let finishPlayback;
  const playback = instance.query({
    key: "playback-continuity",
    priority: "playback-continuity",
    load: () => new Promise(resolve => { finishPlayback = resolve; })
  });
  finishPlayback("segment-ready");
  finishInteractiveQuery("route-ready");
  assert.equal(await playback, "segment-ready");
  assert.equal(await interactive, "route-ready");

  let backgroundStarts = 0;
  let playbackStarts = 0;
  instance.setPlaybackContinuityActive(true);
  instance.leaseSubscription({
    key: "background-notifications",
    priority: "background",
    start: signal => new Promise(resolve => { backgroundStarts += 1; signal.addEventListener("abort", resolve, { once: true }); })
  });
  instance.leaseSubscription({
    key: "playback-command",
    priority: "playback-continuity",
    start: signal => new Promise(resolve => { playbackStarts += 1; signal.addEventListener("abort", resolve, { once: true }); })
  });
  await turn();
  assert.equal(backgroundStarts, 0);
  assert.equal(playbackStarts, 1);
  instance.setPlaybackContinuityActive(false);
  await turn();
  assert.equal(backgroundStarts, 1);
  instance.close();
});
