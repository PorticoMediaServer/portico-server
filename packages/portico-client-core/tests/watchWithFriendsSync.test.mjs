import assert from "node:assert/strict";
import test from "node:test";
import { reduceWatchWithFriendsSnapshot } from "../dist/index.js";

const group = (overrides = {}) => ({
  id: "group-1", name: "Friday screening", ownerProfileId: "host", ownerName: "Host",
  mediaId: "movie-1", mediaTitle: "Movie", state: "playing", positionSeconds: 30,
  positionUpdatedAt: "2026-07-16T12:00:00.000Z", serverTime: "2026-07-16T12:00:01.000Z",
  playbackRate: 1, revision: 4, playbackRevision: 3, reconnectGeneration: 0,
  permissions: { isHost: false, canControl: false, canManageQueue: false }, shuffleEnabled: false,
  repeatMode: "none", command: {}, members: [], queue: [], createdAt: "2026-07-16T11:00:00.000Z",
  updatedAt: "2026-07-16T12:00:01.000Z", ...overrides
});

test("Watch With Friends ignores reordered snapshots", () => {
  const result = reduceWatchWithFriendsSnapshot({ revision: 9, playbackRevision: 6, reconnectGeneration: 1 }, group({ revision: 8, reconnectGeneration: 1 }), { mediaId: "movie-1", positionSeconds: 31, paused: false }, Date.parse("2026-07-16T12:00:01.000Z"));
  assert.deepEqual(result.action, { type: "ignore", reason: "stale" });
  assert.equal(result.state.revision, 9);
});

test("Watch With Friends ignores a snapshot that regresses playback revision", () => {
  const result = reduceWatchWithFriendsSnapshot(
    { revision: 5, playbackRevision: 4, reconnectGeneration: 1 },
    group({ revision: 6, playbackRevision: 3, reconnectGeneration: 1 }),
    { mediaId: "movie-1", positionSeconds: 30, paused: false },
    Date.parse("2026-07-16T12:00:01.000Z")
  );
  assert.deepEqual(result.action, { type: "ignore", reason: "stale" });
  assert.equal(result.state.playbackRevision, 4);
});

test("Watch With Friends hard-seeks large drift and soft-corrects small drift", () => {
  const receivedAt = Date.parse("2026-07-16T12:00:01.000Z");
  const hard = reduceWatchWithFriendsSnapshot(undefined, group(), { mediaId: "movie-1", positionSeconds: 20, paused: false }, receivedAt);
  assert.equal(hard.action.type, "seek");
  const soft = reduceWatchWithFriendsSnapshot(undefined, group(), { mediaId: "movie-1", positionSeconds: 30.1, paused: false }, receivedAt);
  assert.equal(soft.action.type, "rate");
  assert.ok(soft.action.playbackRate > 1);
});

test("Watch With Friends hard drift wins over a play-state mismatch", () => {
  const receivedAt = Date.parse("2026-07-16T12:00:01.000Z");
  const result = reduceWatchWithFriendsSnapshot(
    undefined,
    group(),
    { mediaId: "movie-1", positionSeconds: 5, paused: true },
    receivedAt
  );
  assert.deepEqual(result.action, {
    type: "seek",
    positionSeconds: 31,
    paused: false,
    driftSeconds: 26
  });
});

test("Watch With Friends loads media changes and defers correction while buffering", () => {
  const receivedAt = Date.parse("2026-07-16T12:00:01.000Z");
  assert.equal(reduceWatchWithFriendsSnapshot(undefined, group(), { mediaId: "movie-2", positionSeconds: 0, paused: true }, receivedAt).action.type, "load");
  assert.deepEqual(reduceWatchWithFriendsSnapshot(undefined, group(), { mediaId: "movie-1", positionSeconds: 0, paused: true, buffering: true }, receivedAt).action, { type: "ignore", reason: "buffering" });
});
