import {useRef, useSyncExternalStore} from 'react';

/**
 * Playback engine (simulated). Keeps one play queue with a ticking position so the video player,
 * the now-playing screen and the mini player all show the same session. A real build swaps the
 * ticker for the native player's events; the store's shape stays.
 */
export type PlaybackKind = 'video' | 'audio' | 'book' | 'live';
export type SleepTimer = {mode: 'off' | 'minutes' | 'endOfItem' | 'endOfChapter'; until: number | null; label: string};

export type PlayerState = {
  active: boolean;
  kind: PlaybackKind;
  queue: string[];
  index: number;
  itemId: string | null;
  channelId: string | null;
  position: number;
  duration: number;
  playing: boolean;
  buffering: boolean;
  rate: number;
  audioTrack: string | null;
  subtitle: string | null;
  subtitleOffset: number;
  quality: 'auto' | 'original' | '1080p' | '720p' | '480p';
  versionId: string | null;
  shuffle: boolean;
  repeat: 'off' | 'all' | 'one';
  sleep: SleepTimer;
  minimized: boolean;
  playOn: {device: string; kind: 'airplay' | 'cast' | 'portico'; state: 'connecting' | 'waiting' | 'playing' | 'failed'} | null;
  /** A problem shown over the player (reconnecting, unplayable, Dolby Vision on a display without it). */
  problem: 'reconnecting' | 'unplayable' | 'dolbyVision' | 'channel' | null;
  /** Episodes played back to back without input (drives "Still watching?"). */
  streak: number;
  pip: boolean;
  liveBehind: number;
  captions: boolean;
  ownerStopped: string | null;
};

const initial: PlayerState = {
  active: false, kind: 'video', queue: [], index: 0, itemId: null, channelId: null, position: 0, duration: 0, playing: false, buffering: false, rate: 1,
  audioTrack: null, subtitle: null, subtitleOffset: 0, quality: 'auto', versionId: null, shuffle: false, repeat: 'off',
  sleep: {mode: 'off', until: null, label: 'Off'}, minimized: false, playOn: null, liveBehind: 0, captions: false, ownerStopped: null, problem: null, streak: 0, pip: false,
};

let state: PlayerState = initial;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach(l => l());
const set = (patch: Partial<PlayerState>) => {
  state = {...state, ...patch};
  emit();
};

/** Durations come from the catalogue through this resolver (set by the app at startup). */
let durationOf: (id: string) => number = () => 1800;
export function setDurationResolver(fn: (id: string) => number) {
  durationOf = fn;
}

let ticker: ReturnType<typeof setInterval> | null = null;
function ensureTicker() {
  if (ticker) return;
  ticker = setInterval(() => {
    if (!state.active || !state.playing || state.buffering) return;
    if (state.sleep.until && Date.now() >= state.sleep.until) {
      set({playing: false, sleep: {mode: 'off', until: null, label: 'Off'}});
      return;
    }
    if (state.kind === 'live') {
      set({position: state.position + state.rate});
      return;
    }
    const next = state.position + state.rate;
    if (next >= state.duration) {
      if (state.sleep.mode === 'endOfItem') {
        set({playing: false, position: state.duration, sleep: {mode: 'off', until: null, label: 'Off'}});
        return;
      }
      if (state.repeat === 'one') return set({position: 0});
      if (state.kind !== 'video') return player.next(true);
      set({position: state.duration, playing: false});
      return;
    }
    set({position: next});
  }, 1000);
}

export const player = {
  get: () => state,
  subscribe(l: () => void) {
    listeners.add(l);
    return () => listeners.delete(l);
  },
  play(itemIds: string[] | string, opts: {kind?: PlaybackKind; index?: number; from?: number; shuffle?: boolean} = {}) {
    const queue = Array.isArray(itemIds) ? itemIds : [itemIds];
    const index = opts.index ?? 0;
    const id = queue[index];
    const kind = opts.kind ?? 'video';
    set({...initial, active: true, kind, queue, index, itemId: id, position: opts.from ?? 0, duration: durationOf(id), playing: true, buffering: true, shuffle: !!opts.shuffle, minimized: kind === 'audio' || kind === 'book', sleep: state.sleep, rate: kind === 'book' ? state.rate : 1, subtitle: state.subtitle, audioTrack: state.audioTrack, versionId: state.versionId, streak: kind === 'video' && state.active && state.kind === 'video' ? state.streak : 0});
    setTimeout(() => set({buffering: false}), 700);
    ensureTicker();
  },
  playLive(channelId: string) {
    set({...initial, active: true, kind: 'live', channelId, itemId: null, playing: true, buffering: true, position: 0, duration: 0});
    setTimeout(() => set({buffering: false}), 900);
    ensureTicker();
  },
  toggle() {
    set({playing: !state.playing});
  },
  pause() {
    set({playing: false});
  },
  resume() {
    set({playing: true});
  },
  seek(seconds: number) {
    if (state.kind === 'live') {
      set({liveBehind: Math.max(0, Math.min(3600, state.liveBehind - seconds))});
      return;
    }
    set({position: Math.max(0, Math.min(state.duration - 1, seconds))});
  },
  skip(delta: number) {
    if (state.kind === 'live') return set({liveBehind: Math.max(0, Math.min(3600, state.liveBehind - delta))});
    player.seek(state.position + delta);
  },
  jumpToLive() {
    set({liveBehind: 0});
  },
  next(auto = false) {
    if (state.kind === 'live') return;
    let i = state.index + 1;
    if (state.shuffle && state.queue.length > 1) i = (state.index + 1 + Math.floor(Math.random() * (state.queue.length - 1))) % state.queue.length;
    if (i >= state.queue.length) {
      if (state.repeat === 'all') i = 0;
      else return set({playing: false, position: auto ? state.duration : state.position});
    }
    const id = state.queue[i];
    set({index: i, itemId: id, position: 0, duration: durationOf(id), playing: true, streak: auto ? state.streak + 1 : 0});
  },
  previous() {
    if (state.position > 5 || state.index === 0) return set({position: 0});
    const i = state.index - 1;
    const id = state.queue[i];
    set({index: i, itemId: id, position: 0, duration: durationOf(id)});
  },
  jumpTo(index: number) {
    const id = state.queue[index];
    if (!id) return;
    set({index, itemId: id, position: 0, duration: durationOf(id), playing: true});
  },
  enqueue(ids: string[], next = false) {
    if (!state.active) return player.play(ids, {kind: 'audio'});
    const q = state.queue.slice();
    q.splice(next ? state.index + 1 : q.length, 0, ...ids);
    set({queue: q});
  },
  removeAt(index: number) {
    if (index === state.index) return;
    const q = state.queue.slice();
    q.splice(index, 1);
    set({queue: q, index: index < state.index ? state.index - 1 : state.index});
  },
  move(from: number, to: number) {
    if (to < 0 || to >= state.queue.length) return;
    const q = state.queue.slice();
    const [x] = q.splice(from, 1);
    q.splice(to, 0, x);
    let index = state.index;
    if (from === state.index) index = to;
    else if (from < state.index && to >= state.index) index--;
    else if (from > state.index && to <= state.index) index++;
    set({queue: q, index});
  },
  set(patch: Partial<PlayerState>) {
    set(patch);
  },
  setSleep(mode: SleepTimer['mode'], minutes?: number) {
    if (mode === 'off') return set({sleep: {mode, until: null, label: 'Off'}});
    if (mode === 'minutes' && minutes) return set({sleep: {mode, until: Date.now() + minutes * 60_000, label: `${minutes} min`}});
    set({sleep: {mode, until: null, label: mode === 'endOfChapter' ? 'End of chapter' : 'End of this item'}});
  },
  minimize() {
    set({minimized: true});
  },
  stop() {
    set({...initial, sleep: state.sleep});
  },
};

const shallowEqual = (a: unknown, b: unknown) => {
  if (Object.is(a, b)) return true;
  if (typeof a !== 'object' || typeof b !== 'object' || !a || !b) return false;
  const ka = Object.keys(a);
  if (ka.length !== Object.keys(b).length) return false;
  return ka.every(k => Object.is((a as Record<string, unknown>)[k], (b as Record<string, unknown>)[k]));
};

/** Subscribes to the playback store. With `shallow`, a selector may return a fresh object each time. */
export function usePlayer<T = PlayerState>(select?: (s: PlayerState) => T, shallow = false): T {
  const last = useRef<{v: T} | null>(null);
  return useSyncExternalStore(player.subscribe, () => {
    const v = select ? select(state) : (state as unknown as T);
    if (shallow && last.current && shallowEqual(last.current.v, v)) return last.current.v;
    last.current = {v};
    return v;
  });
}
