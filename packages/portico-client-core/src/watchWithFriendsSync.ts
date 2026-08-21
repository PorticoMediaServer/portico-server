import type { WatchWithFriendsGroup } from "./types.js";

export type WatchWithFriendsLocalPlayback = {
  mediaId?: string;
  positionSeconds: number;
  paused: boolean;
  buffering?: boolean;
  playbackRate?: number;
};

export type WatchWithFriendsSyncAction =
  | { type: "ignore"; reason: "stale" | "duplicate" | "buffering" }
  | { type: "load"; mediaId: string; positionSeconds: number; paused: boolean }
  | { type: "seek"; positionSeconds: number; paused: boolean; driftSeconds: number }
  | { type: "rate"; playbackRate: number; durationMs: number; driftSeconds: number }
  | { type: "play" }
  | { type: "pause" }
  | { type: "none"; driftSeconds: number };

export type WatchWithFriendsSyncState = {
  revision: number;
  playbackRevision: number;
  reconnectGeneration: number;
};

export type WatchWithFriendsSyncResult = {
  state: WatchWithFriendsSyncState;
  action: WatchWithFriendsSyncAction;
  targetPositionSeconds: number;
};

const parseTime = (value?: string): number | undefined => {
  if (!value) return undefined;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : undefined;
};

/**
 * Deterministic participant reducer. Reordered snapshots are ignored, position
 * extrapolation uses the playback anchor rather than generic metadata time,
 * and small drift is corrected temporarily without creating command feedback.
 */
export function reduceWatchWithFriendsSnapshot(
  previous: WatchWithFriendsSyncState | undefined,
  group: WatchWithFriendsGroup,
  local: WatchWithFriendsLocalPlayback,
  receivedAtMs = Date.now()
): WatchWithFriendsSyncResult {
  const nextState = {
    revision: group.revision,
    playbackRevision: group.playbackRevision,
    reconnectGeneration: group.reconnectGeneration
  };
  const anchorMs = parseTime(group.positionUpdatedAt) ?? receivedAtMs;
  const serverMs = parseTime(group.serverTime) ?? receivedAtMs;
  const transitMs = Math.max(0, receivedAtMs - serverMs);
  const runningMs = group.state === "playing" ? Math.max(0, serverMs - anchorMs) + transitMs : 0;
  const target = Math.max(0, group.positionSeconds + runningMs / 1000 * (group.playbackRate || 1));
  if (previous && group.reconnectGeneration < previous.reconnectGeneration) {
    return { state: previous, action: { type: "ignore", reason: "stale" }, targetPositionSeconds: target };
  }
  if (
    previous &&
    group.reconnectGeneration === previous.reconnectGeneration &&
    (group.revision < previous.revision ||
      group.playbackRevision < previous.playbackRevision)
  ) {
    return { state: previous, action: { type: "ignore", reason: "stale" }, targetPositionSeconds: target };
  }
  if (previous && group.reconnectGeneration === previous.reconnectGeneration && group.revision === previous.revision && group.playbackRevision === previous.playbackRevision) {
    return { state: previous, action: { type: "ignore", reason: "duplicate" }, targetPositionSeconds: target };
  }
  if (local.mediaId !== group.mediaId) {
    return { state: nextState, action: { type: "load", mediaId: group.mediaId, positionSeconds: target, paused: group.state !== "playing" }, targetPositionSeconds: target };
  }
  if (local.buffering) {
    return { state: nextState, action: { type: "ignore", reason: "buffering" }, targetPositionSeconds: target };
  }
  const drift = target - Math.max(0, local.positionSeconds);
  const absoluteDrift = Math.abs(drift);
  // A state transition and a large timeline correction must be atomic from the
  // adapter's perspective. Seeking first with the authoritative paused flag
  // avoids briefly playing from a stale position after reconnect/load.
  if (absoluteDrift >= 3) {
    return { state: nextState, action: { type: "seek", positionSeconds: target, paused: group.state !== "playing", driftSeconds: drift }, targetPositionSeconds: target };
  }
  if (group.state === "paused" && !local.paused) {
    return { state: nextState, action: { type: "pause" }, targetPositionSeconds: target };
  }
  if (group.state === "playing" && local.paused) {
    return { state: nextState, action: { type: "play" }, targetPositionSeconds: target };
  }
  if (group.state === "playing" && absoluteDrift >= 0.75) {
    const baseRate = group.playbackRate || 1;
    const rate = Math.max(0.9, Math.min(1.1, baseRate + drift * 0.025));
    return { state: nextState, action: { type: "rate", playbackRate: rate, durationMs: 4000, driftSeconds: drift }, targetPositionSeconds: target };
  }
  return { state: nextState, action: { type: "none", driftSeconds: drift }, targetPositionSeconds: target };
}
