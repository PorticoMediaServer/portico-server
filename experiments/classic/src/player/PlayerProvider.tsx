import React, {createContext, useCallback, useContext, useEffect, useMemo, useRef, useState} from 'react';
import {useClient} from '../data/DataProvider';
import type {Entry} from '../data/types';
import {durationOf} from '../data/fixtures/catalog';

export type PlayerMode = 'none' | 'video' | 'music' | 'book' | 'live';
export type CastTarget = {kind: 'airplay' | 'cast' | 'portico'; name: string} | undefined;

export type PlayerState = {
  mode: PlayerMode;
  expanded: boolean;
  queue: Entry[];
  index: number;
  position: number;
  duration: number;
  playing: boolean;
  buffering: boolean;
  audio: string;
  subtitles: string;
  quality: string;
  speed: number;
  sleepEndsAt?: number;
  sleepLabel?: string;
  shuffle: boolean;
  repeat: 'off' | 'all' | 'one';
  cast: CastTarget;
  captions: boolean;
  liveChannelId?: string;
  ended: boolean;
  subtitleOffset: number;
};

export type PlayerApi = {
  play: (entries: Entry[], index?: number, opts?: {fromStart?: boolean; mode?: PlayerMode; position?: number}) => void;
  playLive: (channelId: string, entry: Entry) => void;
  toggle: () => void;
  setPlaying: (p: boolean) => void;
  seek: (seconds: number) => void;
  skip: (delta: number) => void;
  next: () => void;
  previous: () => void;
  jump: (index: number) => void;
  close: () => void;
  expand: () => void;
  minimize: () => void;
  playNext: (e: Entry) => void;
  enqueue: (e: Entry) => void;
  remove: (index: number) => void;
  move: (index: number, delta: number) => void;
  set: (patch: Partial<PlayerState>) => void;
  sleep: (minutes: number | 'end' | undefined) => void;
};

const initial: PlayerState = {mode: 'none', expanded: false, queue: [], index: 0, position: 0, duration: 0, playing: false, buffering: false, audio: 'a1', subtitles: 'off', quality: 'auto', speed: 1, shuffle: false, repeat: 'off', cast: undefined, captions: false, ended: false, subtitleOffset: 0};
const StateCtx = createContext<PlayerState>(initial);
const ApiCtx = createContext<PlayerApi | null>(null);

function modeFor(e: Entry): PlayerMode {
  if (e.kind === 'track' || e.kind === 'album' || e.kind === 'artist' || e.kind === 'playlist') return 'music';
  if (e.kind === 'audiobook') return 'book';
  if (e.kind === 'channel' || e.kind === 'program') return 'live';
  return 'video';
}
const lengthOf = (e: Entry) => e.duration || durationOf(e.id) || 600;

/**
 * Simulated playback: the player chrome, timeline, queue and options are
 * real; the clock advances once a second while playing. Progress is written
 * back to the server (Continue Watching, watched state).
 */
export function PlayerProvider({children}: {children: React.ReactNode}) {
  const client = useClient();
  const [s, setS] = useState<PlayerState>(initial);
  const ref = useRef(s);
  ref.current = s;
  const saveProgress = useCallback((st: PlayerState) => {
    const e = st.queue[st.index];
    if (!e || st.mode === 'live' || st.mode === 'none' || e.id.includes('-trailer') || e.id.includes('-bts') || e.id.includes('-scene')) return;
    client.personal.setProgress(e.id, Math.floor(st.position));
  }, [client]);

  useEffect(() => {
    if (!s.playing || s.mode === 'none') return;
    const t = setInterval(() => {
      setS(prev => {
        if (!prev.playing) return prev;
        if (prev.sleepEndsAt && Date.now() >= prev.sleepEndsAt) return {...prev, playing: false, sleepEndsAt: undefined, sleepLabel: undefined};
        if (prev.mode === 'live') return {...prev, position: prev.position + 1};
        const pos = prev.position + prev.speed;
        if (pos < prev.duration) return {...prev, position: pos};
        // Reached the end.
        if (prev.repeat === 'one') return {...prev, position: 0};
        const last = prev.index >= prev.queue.length - 1;
        if (prev.mode === 'video') return {...prev, position: prev.duration, playing: false, ended: true};
        if (last && prev.repeat !== 'all') return {...prev, position: prev.duration, playing: false, ended: true};
        const index = last ? 0 : prev.index + 1;
        const e = prev.queue[index]!;
        return {...prev, index, position: 0, duration: lengthOf(e)};
      });
    }, 1000);
    return () => clearInterval(t);
  }, [s.playing, s.mode]);

  // Save progress every 15 s while playing.
  useEffect(() => {
    if (!s.playing) return;
    const t = setInterval(() => saveProgress(ref.current), 15000);
    return () => clearInterval(t);
  }, [s.playing, saveProgress]);

  const api = useMemo<PlayerApi>(() => ({
    play(entries, index = 0, opts) {
      const e = entries[index];
      if (!e) return;
      const mode = opts?.mode ?? modeFor(e);
      const p = client.personal.get(e.id);
      const position = opts?.position ?? (opts?.fromStart || p.watched ? 0 : p.progress || 0);
      saveProgress(ref.current);
      setS(prev => ({...initial, audio: prev.audio, quality: prev.quality, speed: mode === 'book' ? prev.speed : 1, cast: prev.cast, mode, expanded: true, queue: entries, index, position, duration: lengthOf(e), playing: true, buffering: true, shuffle: mode === 'music' && client.prefs.get('music.shuffleDefault') === true, subtitles: client.prefs.get('playback.subtitlesEnabled') ? 's1' : 'off'}));
      setTimeout(() => setS(prev => ({...prev, buffering: false})), 900);
    },
    playLive(channelId, entry) {
      saveProgress(ref.current);
      setS({...initial, mode: 'live', expanded: true, queue: [entry], index: 0, position: 0, duration: 0, playing: true, buffering: true, liveChannelId: channelId});
      setTimeout(() => setS(prev => ({...prev, buffering: false})), 1200);
    },
    toggle() { setS(prev => { const next = {...prev, playing: !prev.playing, ended: false}; if (!next.playing) saveProgress(next); return next; }); },
    setPlaying(playing) { setS(prev => ({...prev, playing})); },
    seek(seconds) { setS(prev => ({...prev, position: Math.max(0, Math.min(prev.duration || seconds, seconds)), ended: false})); },
    skip(delta) { setS(prev => ({...prev, position: Math.max(0, Math.min(prev.duration, prev.position + delta)), ended: false})); },
    next() {
      setS(prev => {
        saveProgress(prev);
        if (prev.index >= prev.queue.length - 1) return prev.repeat === 'all' ? {...prev, index: 0, position: 0, duration: lengthOf(prev.queue[0]!), ended: false, playing: true} : prev;
        const e = prev.queue[prev.index + 1]!;
        return {...prev, index: prev.index + 1, position: 0, duration: lengthOf(e), ended: false, playing: true};
      });
    },
    previous() {
      setS(prev => {
        if (prev.position > 5 || prev.index === 0) return {...prev, position: 0};
        const e = prev.queue[prev.index - 1]!;
        return {...prev, index: prev.index - 1, position: 0, duration: lengthOf(e), ended: false};
      });
    },
    jump(index) { setS(prev => ({...prev, index, position: 0, duration: lengthOf(prev.queue[index]!), ended: false, playing: true})); },
    close() { saveProgress(ref.current); setS(prev => ({...initial, audio: prev.audio, quality: prev.quality, cast: prev.cast})); },
    expand() { setS(prev => ({...prev, expanded: true})); },
    minimize() { saveProgress(ref.current); setS(prev => (prev.mode === 'video' || prev.mode === 'live' ? {...initial} : {...prev, expanded: false})); },
    playNext(e) { setS(prev => (prev.mode === 'none' ? prev : {...prev, queue: [...prev.queue.slice(0, prev.index + 1), e, ...prev.queue.slice(prev.index + 1)]})); },
    enqueue(e) { setS(prev => (prev.mode === 'none' ? prev : {...prev, queue: [...prev.queue, e]})); },
    remove(i) { setS(prev => (i === prev.index ? prev : {...prev, queue: prev.queue.filter((_, j) => j !== i), index: i < prev.index ? prev.index - 1 : prev.index})); },
    move(i, delta) {
      setS(prev => {
        const j = i + delta;
        if (j <= prev.index || j >= prev.queue.length || i <= prev.index) return prev;
        const q = [...prev.queue];
        [q[i], q[j]] = [q[j]!, q[i]!];
        return {...prev, queue: q};
      });
    },
    set(patch) { setS(prev => ({...prev, ...patch})); },
    sleep(minutes) {
      setS(prev => {
        if (minutes === undefined) return {...prev, sleepEndsAt: undefined, sleepLabel: undefined};
        const secs = minutes === 'end' ? Math.max(1, prev.duration - prev.position) : minutes * 60;
        return {...prev, sleepEndsAt: Date.now() + secs * 1000, sleepLabel: minutes === 'end' ? 'End of chapter' : `${minutes} min`};
      });
    },
  }), [client, saveProgress]);

  return (
    <ApiCtx.Provider value={api}>
      <StateCtx.Provider value={s}>{children}</StateCtx.Provider>
    </ApiCtx.Provider>
  );
}

export const usePlayer = () => useContext(ApiCtx)!;
export const usePlayerState = () => useContext(StateCtx);

export function formatTime(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const r = s % 60;
  return h ? `${h}:${String(m).padStart(2, '0')}:${String(r).padStart(2, '0')}` : `${m}:${String(r).padStart(2, '0')}`;
}

export function formatDuration(sec?: number): string {
  if (!sec) return '';
  const m = Math.round(sec / 60);
  if (m < 60) return `${m}m`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}
