import type { WatchConnectionState } from './watchWithFriendsSource';

export function watchClock(seconds: number) {
  const bounded = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(bounded / 3_600);
  const minutes = Math.floor((bounded % 3_600) / 60);
  const remainder = bounded % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}`
    : `${minutes}:${String(remainder).padStart(2, '0')}`;
}

export function watchStateLabel(value?: string) {
  switch (value) {
    case 'ready': return 'Ready';
    case 'buffering': return 'Buffering';
    case 'playing': return 'Playing';
    case 'paused': return 'Paused';
    case 'joined': return 'Joined';
    default: return 'Connected';
  }
}

export function connectionLabel(value: WatchConnectionState | 'idle') {
  switch (value) {
    case 'connecting': return 'Connecting to live updates';
    case 'live': return 'Live updates connected';
    case 'reconnecting': return 'Reconnecting live updates';
    case 'failed': return 'Live updates disconnected';
    default: return 'Join to receive live updates';
  }
}

export function initials(value: string) {
  const words = value.trim().split(/\s+/).filter(Boolean);
  return words.slice(0, 2).map((word) => word[0]?.toLocaleUpperCase()).join('') || 'P';
}

export function relativeSeen(value: string, now = Date.now()) {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return 'Last seen unavailable';
  const seconds = Math.max(0, Math.round((now - timestamp) / 1_000));
  if (seconds < 15) return 'Active now';
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ago`;
}
