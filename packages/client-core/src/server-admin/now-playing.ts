/**
 * Server › Overview Now Playing on Playback Protocol v1 (spec §14, be/playback c85d526):
 * `GET /v1/admin/sessions` and `POST /v1/admin/sessions/{id}:terminate`. A server that
 * announces v1 is read through client-core's `NowPlayingStore`; one without it (or one whose
 * v1 list answers 404) keeps the /v2 owner streams list.
 *
 * Refreshing never storms: while the device's v1 event feed is connected the list re-reads on
 * `admin.sessions` / `session.updated` (and a resync), coalesced by the store; otherwise it
 * re-reads every 10 s while the tab is visible, and once when the tab is shown again.
 */
import type {AdminSession} from '../playback-v1/types.ts';
import type {MessageId, MessageValues} from '../../../i18n/src/index.ts';
import {when} from './console-words.ts';

export const NOW_PLAYING_POLL_MS = 10_000;
/** The server's limit for the message shown to the viewer (`AdminTerminateMessageMax`). */
export const STOP_MESSAGE_MAX = 500;
/** Row pitch of the windowed list, px; each row's copy is clamped so the pitch is fixed. */
export const NOW_PLAYING_ROW_PX = 68;

type EventsLike = {
  on(type: string, fn: () => void): () => void;
  onResync?(fn: () => void): () => void;
  getStatus(): {connected: boolean};
  subscribeStatus(fn: () => void): () => void;
};
type Env = {setTimer: (fn: () => void, ms: number) => unknown; clearTimer: (t: unknown) => void; hidden: () => boolean; onVisibility: (fn: () => void) => () => void};
const browserEnv: Env = {
  setTimer: (fn, ms) => setTimeout(fn, ms),
  clearTimer: t => clearTimeout(t as ReturnType<typeof setTimeout>),
  hidden: () => typeof document !== 'undefined' && document.visibilityState === 'hidden',
  // A client without a document (a phone app) passes its own environment, or simply polls.
  onVisibility: fn => { if (typeof document === 'undefined') return () => {}; document.addEventListener('visibilitychange', fn); return () => document.removeEventListener('visibilitychange', fn); },
};

/** Reads now, then keeps the list current (see the module note). Returns a stop function. */
export function driveNowPlaying(refresh: () => void, events: EventsLike | undefined, env: Env = browserEnv): () => void {
  let timer: unknown, stopped = false;
  const live = () => !!events?.getStatus().connected;
  const arrange = () => {
    env.clearTimer(timer); timer = undefined;
    if (stopped || live() || env.hidden()) return;
    timer = env.setTimer(() => { timer = undefined; if (stopped || live() || env.hidden()) return; refresh(); arrange(); }, NOW_PLAYING_POLL_MS);
  };
  const offs: (() => void)[] = [];
  if (events) {
    offs.push(events.on('admin.sessions', refresh), events.on('session.updated', refresh));
    if (events.onResync) offs.push(events.onResync(refresh));
    let was = live();
    // The status also moves with every event's cursor; only a change of connection matters here.
    offs.push(events.subscribeStatus(() => { const now = live(); if (now !== was) { was = now; arrange(); } }));
  }
  offs.push(env.onVisibility(() => { if (!env.hidden() && !live()) refresh(); arrange(); }));
  refresh();
  arrange();
  return () => { stopped = true; env.clearTimer(timer); for (const off of offs) off(); };
}

/** The rows in view (plus `overscan` either side) of a fixed-pitch list. */
export function windowRange(scrollTop: number, viewport: number, count: number, rowPx = NOW_PLAYING_ROW_PX, overscan = 3): {first: number; last: number} {
  if (count <= 0) return {first: 0, last: -1};
  const first = Math.max(0, Math.floor(Math.max(0, scrollTop) / rowPx) - overscan);
  const last = Math.min(count - 1, Math.ceil((Math.max(0, scrollTop) + Math.max(viewport, rowPx)) / rowPx) + overscan);
  return {first, last};
}

/** How a session is delivered, as a catalogue id: converting beats repackaging beats direct. */
export function deliveryId(decision: AdminSession['decision']): string | undefined {
  const video = decision.video?.action, audio = decision.audio?.action, subtitles = decision.subtitles?.action;
  if (video === 'transcode' || subtitles === 'burn' || video === 'burn') return 'web.nowPlaying.delivery.convertVideo';
  if (audio === 'transcode') return 'web.nowPlaying.delivery.convertAudio';
  if (video === 'copy' || audio === 'copy') return 'web.nowPlaying.delivery.directStream';
  if (video === 'direct' || audio === 'direct') return 'web.nowPlaying.delivery.directPlay';
  return undefined;
}

export function stateId(state: string): string | undefined {
  return state === 'playing' ? 'web.nowPlaying.state.playing' : state === 'paused' ? 'web.nowPlaying.state.paused' : state === 'buffering' || state === 'preparing' ? 'web.nowPlaying.state.starting' : undefined;
}

export function locationId(location: AdminSession['location']): string | undefined {
  return location === 'local' ? 'web.nowPlaying.location.local' : location === 'remote' ? 'web.nowPlaying.location.remote' : undefined;
}

/** Megabits per second, one decimal below 10 (the delivered rate, else the chosen bitrate). */
export function rateMbps(session: Pick<AdminSession, 'bandwidthKbps' | 'bitrateKbps'>): string | undefined {
  const kbps = session.bandwidthKbps || session.bitrateKbps;
  if (!kbps || kbps <= 0) return undefined;
  const mbps = kbps / 1000;
  return mbps >= 10 ? String(Math.round(mbps)) : mbps.toFixed(1);
}

/** The optional message for the viewer: trimmed, at most 500 characters, empty means none. */
export function stopMessage(text: string): string | undefined {
  const t = text.trim();
  return t ? [...t].slice(0, STOP_MESSAGE_MAX).join('') : undefined;
}

type Translate = (id: MessageId, values?: MessageValues) => string;

/** Who is watching: the profile, with its account when they differ. */
export function sessionViewer(x: AdminSession, t: Translate): string {
  return x.profile && x.user && x.profile !== x.user ? t('web.nowPlaying.viewerWithAccount', {profile: x.profile, account: x.user}) : x.profile || x.user || t('web.nowPlaying.viewer');
}

/** One session's line under its title: who, on what, how it is delivered, how fast, since when. */
export function sessionFacts(x: AdminSession, t: Translate): string {
  const parts = [sessionViewer(x, t)];
  if (x.device) parts.push(t('web.nowPlaying.onDevice', {device: x.device}));
  for (const id of [stateId(x.state), deliveryId(x.decision), locationId(x.location)]) if (id) parts.push(t(id as MessageId));
  const rate = rateMbps(x);
  if (rate) parts.push(t('web.nowPlaying.rate', {rate}));
  const started = x.startedAt ? Date.parse(x.startedAt) : NaN;
  if (Number.isFinite(started)) parts.push(t('web.nowPlaying.started', {time: when(started)}));
  return parts.join(' · ');
}
