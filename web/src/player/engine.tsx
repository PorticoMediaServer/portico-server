import React, {createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {HttpLocalApi, PlaybackService, type MediaItem, type PlaybackSnapshot} from '@core/index.ts';
import type {ContentEntry} from '@core/library-content.ts';
import type {GuideChannel} from '@core/channel-guide.ts';
import {queueItemInput, type QueueController} from '@core/queue-controller.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import {V1ChannelControl} from '@core/playback-v1/channel-control.ts';
import {V1PlaybackApi} from '@core/playback-v1/legacy-bridge.ts';
import {V1QueuePlayer} from '@core/playback-v1/v1-queue-player.ts';
import {EventsClient} from '@core/playback-v1/events.ts';
import type {PreparedChoice} from '@core/prepared-media.ts';
import {useSession} from '../app/session';
import {documentVisibility} from './page-visibility';
import {getDeviceEvents, setDeviceEvents} from '../app/device-events';
import type {MenuAnchor} from '../ui/Menu';
import {ConfirmDialog} from '../ui';
import {errorText} from '../app/errors';
import {currentI18n} from '../app/i18n';
import {ownerStopOf} from './owner-stop';
import {dolbyVisionWarningNeeded, mayCarryVideo} from './dolby-vision';
import {sourceFactsOf, type SourceFacts} from '@core/presentation/index.ts';
import {musicAudioEffects, musicQueueDefaults} from '@core/music-preferences.ts';
import {noteTunedChannel} from './live';
import {useServerPreferences} from '../app/server-preferences';
import type {SequenceContainer} from '../app/container-play';
export type {SequenceContainer};

/**
 * The player engine. One PlaybackService per signed-in viewer, one device
 * queue lane, and a small command surface for the chrome. Presentation
 * state (expanded / collapsed) lives here so the shell, mini bar and full
 * player agree. Screens express intent through `play`, `tune` and `more`.
 */
export type NowPlaying = {itemId?: string; title: string; subtitle?: string; posterUrl?: string; backdropUrl?: string; kind?: string; libraryId?: string; artist?: string; album?: string; channel?: boolean; item?: MediaItem;
  /** PERF-24: the entry the play action came from already says what the player shows (title, kind,
   * library), so the item isn't read again. */
  seeded?: boolean};

export type PlayerEngine = {
  service: PlaybackService;
  state: PlaybackSnapshot;
  /** The device's v1 queue (`V1QueuePlayer`). */
  queue?: QueueController;
  /** "Getting your queue ready…" while a v1 queue's first entry is being chosen. */
  queueNotice?: string;
  identity: NowPlaying;
  active: boolean;
  isAudio: boolean;
  expanded: boolean;
  volume: number;
  muted: boolean;
  moreEntry?: ContentEntry;
  moreAnchor?: MenuAnchor;
  moreOrigin?: EntryOrigin;
  /** 4K/HDR source facts parsed from the pre-play playback-options read (M27's request, reused: no new request per play). */
  sourceFacts: (itemId: string | undefined) => SourceFacts | undefined;
  play: (itemId: string, startSeconds: number, entry?: ContentEntry, prepared?: PreparedChoice, quality?: string) => void;
  /** Play `entries[index]` and queue everything after it, so Up Next has somewhere to go. */
  playSequence: (entries: readonly ContentEntry[], index: number, startSeconds: number, container?: SequenceContainer) => void;
  tune: (channel: GuideChannel) => void;
  more: (entry: ContentEntry, anchor?: MenuAnchor, origin?: EntryOrigin) => void;
  clearMore: () => void;
  toggle: () => void;
  retry: () => void;
  seekBy: (delta: number) => void;
  seekTo: (seconds: number) => void;
  leave: () => void;
  collapse: () => void;
  expand: () => void;
  setVolume: (v: number) => void;
  setMuted: (m: boolean) => void;
  next: () => void;
  previous: () => void;
  canNext: boolean;
  canPrevious: boolean;
  /** Advance past a held completion through the server's post-play policy. */
  advance: (mode: 'automatic' | 'manual' | 'still-watching') => Promise<void>;
  /** While watching with a group, play/pause and seeking belong to the group. A handler
   * returns true when it took the intent, so the local player is left to follow. */
  setGroupControl: (control: GroupControl | null) => void;
  /** Names what is playing when it was started by id alone (a group's pick). */
  describe: (identity: NowPlaying) => void;
};
export type GroupControl = {toggle(playing: boolean): boolean; seek(seconds: number): boolean};
/** Where a menu is opened from, when that changes its actions: a recommendation offers Not interested; a title's own page already has Watchlist, Favorite and Watched on its action row, so its ⋯ does not repeat them. */
export type EntryOrigin = 'recommendation' | 'page';
// `SequenceContainer` (the container a sequence comes from: on v1 one server-snapshotted request
// instead of an item list) lives in `app/container-play` with the kinds it names.

const Ctx = createContext<PlayerEngine | null>(null);
/** Intents only. Stable across playback ticks, so screens and cards that just need `play`/`more` never re-render with the timeline. */
export type PlayerIntents = Pick<PlayerEngine, 'play' | 'playSequence' | 'tune' | 'more'>;
export const ActionsCtx = createContext<PlayerIntents | null>(null);
/** PERF-28: the card menu's hand-off (entry and anchor) on its own, so the sheet doesn't re-render with playback. */
type MoreHandOff = {moreEntry?: ContentEntry; moreAnchor?: MenuAnchor; moreOrigin?: EntryOrigin; clearMore: () => void};
const MoreCtx = createContext<MoreHandOff>({clearMore: () => {}});
export const useMoreHandOff = () => useContext(MoreCtx);
/** This device's Playback v1 event feed, when the server announces v1 (one per device; the
 * admin Now Playing list listens on it rather than opening a second one). */
const V1EventsCtx = createContext<EventsClient | undefined>(undefined);
export const useV1Events = () => useContext(V1EventsCtx);
/** The id of the item that is playing, on its own, so a track row can mark itself without re-rendering with the clock. */
const PlayingCtx = createContext<string | undefined>(undefined);
export const usePlayingItemId = () => useContext(PlayingCtx);
const volumeKey = 'portico.volume.v1';

export function PlayerProvider({children}: {children: React.ReactNode}) {
  const {api, session, serverUrl} = useSession();
  const viewer = session!.viewer;
  const viewerKey = JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId, viewer.serverId]);
  // One Playback v1 stack per signed-in viewer: sessions, the device's v1 queue, channels as v1
  // sessions (spec §18.6), and this device's event feed.
  const musicDefaultsSource = useServerPreferences().snapshot;
  const musicDefaultsRef = useRef(musicDefaultsSource);
  musicDefaultsRef.current = musicDefaultsSource;
  const v1 = useMemo(() => {
    const http = localV1Http(api);
    const v1api = new V1PlaybackApi(http);
    const playback = new PlaybackService(v1api, undefined, {queueRequired: true, diagnostics: import.meta.env.DEV, channels: new V1ChannelControl({http}), pageVisibility: documentVisibility});
    const events = new EventsClient({http});
    const player = new V1QueuePlayer({musicDefaults: () => (musicDefaultsRef.current ? musicQueueDefaults(musicDefaultsRef.current) : undefined), http, api: v1api, playback, events, viewer: {serverId: viewer.serverId, authority: viewer.authority === 'hosted' ? 'hosted' : 'local', accountId: viewer.accountId, profileId: viewer.profileId}});
    player.setCompletionPolicy(() => 'hold');
    return {api: v1api, playback, player, events};
  }, [api, viewerKey]); // eslint-disable-line react-hooks/exhaustive-deps
  const service = v1.playback;
  useEffect(() => {
    // Development aid: inspect the live service from the console.
    if (import.meta.env.DEV) (window as unknown as {__porticoPlayback?: PlaybackService}).__porticoPlayback = service;
    v1.events.start();
    setDeviceEvents(v1.events);
    const stop = v1.player.connect();
    // WEB-06: this provider owns the service, so it ends it (and the queue, and the feed).
    return () => { service.leave(); stop(); v1.player.dispose(); v1.events.stop(); if (getDeviceEvents() === v1.events) setDeviceEvents(null); };
  }, [v1, service]);
  /* The server ending this device's session (an owner's stop, a transfer) reaches the service at
     once: `session.updated` goes to the bridge, which stops with the reason (`snapshot.ended`). */
  useEffect(() => v1.events.on('session.updated', event => { if (event.resource?.kind === 'session' && event.resource.id) v1.api.sessionUpdated(event.resource.id, event.data); }), [v1]);
  /* X-02: the viewer's music preferences (gapless, crossfade, normalization) are what the audio
     engine renders with, on sign-in, profile selection and every change. Until the server answers
     (or offline) the device's cached copy applies. */
  const musicPreferences = useServerPreferences().snapshot;
  useEffect(() => {
    if (musicPreferences) service.adoptAudioEffectsPreference(musicAudioEffects(musicPreferences));
  }, [musicPreferences, service]);
  // The position advances several times a second and nothing here but the timeline needs that
  // (it listens to the service itself). Everything else sees the position in whole seconds, so
  // playback re-renders the player once a second rather than on every tick.
  const settled = useMemo(() => {
    let held: PlaybackSnapshot | undefined;
    return () => {
      const next = service.getSnapshot();
      if (held && samePace(held, next)) return held;
      held = next;
      return next;
    };
  }, [service]);
  const state = useSyncExternalStore(service.subscribe, settled);
  const activeService = service;
  const activeQueue: QueueController = v1.player;
  const queueView = useSyncExternalStore(activeQueue?.subscribe ?? (() => () => {}), () => activeQueue?.getSnapshot().view ?? null);
  const queuePhase = useSyncExternalStore(activeQueue?.subscribe ?? (() => () => {}), () => activeQueue?.getSnapshot().phase ?? 'ready');
  // A v1 start whose first entry the server is still choosing (a very large shuffle).
  const queueNotice = queuePhase === 'preparing' && !queueView?.queue.currentEntryId ? currentI18n().t('web.queue.preparing') : undefined;

  /* Identity of what is playing. The intent carries a fast first paint; the item record completes it. */
  const [identity, setIdentity] = useState<NowPlaying>({title: ''});
  const [expanded, setExpanded] = useState(false);
  const [moreEntry, setMoreEntry] = useState<ContentEntry>();
  const [moreAnchor, setMoreAnchor] = useState<MenuAnchor>();
  const [moreOrigin, setMoreOrigin] = useState<EntryOrigin>();
  const more = useCallback((entry: ContentEntry, anchor?: MenuAnchor, origin?: EntryOrigin) => { setMoreAnchor(anchor); setMoreOrigin(origin); setMoreEntry(entry); }, []);
  /* M27 Dolby Vision warning: pending play intent awaiting confirmation, and item ids already
   * acknowledged for this session (so it isn't asked twice per item). */
  type DolbyPendingSingle = {kind: 'single'; itemId: string; startSeconds: number; entry?: ContentEntry; prepared?: PreparedChoice; quality?: string};
  type DolbyPendingSequence = {kind: 'sequence'; entries: readonly ContentEntry[]; index: number; startSeconds: number; container?: SequenceContainer};
  const [dolbyPending, setDolbyPending] = useState<DolbyPendingSingle | DolbyPendingSequence | null>(null);
  const dolbyAcknowledged = useRef<Set<string>>(new Set());
  /* MU4 COMPAT-01: 4K/HDR facts per item, parsed from the same pre-play read (never a new request). */
  const sourceFactsRef = useRef(new Map<string, SourceFacts>());
  const sourceFacts = useCallback((itemId: string | undefined) => (itemId ? sourceFactsRef.current.get(itemId) : undefined), []);
  const [volume, setVolumeState] = useState(() => {
    try {
      const raw = JSON.parse(localStorage.getItem(volumeKey) ?? 'null');
      return raw && typeof raw.volume === 'number' ? Math.max(0, Math.min(1, raw.volume)) : 1;
    } catch {
      return 1;
    }
  });
  const [muted, setMutedState] = useState(() => {
    try {
      return JSON.parse(localStorage.getItem(volumeKey) ?? 'null')?.muted === true;
    } catch {
      return false;
    }
  });
  useEffect(() => {
    try {
      localStorage.setItem(volumeKey, JSON.stringify({volume, muted}));
    } catch {}
  }, [volume, muted]);
  const playingItemId = state.channel ? undefined : state.itemId;
  useEffect(() => {
    if (!playingItemId || identity.item?.id === playingItemId || (identity.seeded && identity.itemId === playingItemId)) return;
    const abort = new AbortController();
    const intent = state.intentId;
    const brief = queueView?.items.find(x => x.itemId === playingItemId);
    if (identity.itemId !== playingItemId) setIdentity(prev => ({...prev, itemId: playingItemId, title: brief?.title ?? prev.title ?? 'Now playing', kind: brief?.kind ?? prev.kind, libraryId: brief?.libraryId ?? prev.libraryId}));
    void api.request<MediaItem & {song?: {artist: string; albumTitle: string}; bookFile?: {bookTitle: string}; episode?: {showTitle: string; seasonNumber?: number; number: number}}>('/v1/items/' + encodeURIComponent(playingItemId), 'GET', undefined, abort.signal)
      .then(item => {
        if (abort.signal.aborted || activeService.getSnapshot().intentId !== intent || item.id !== playingItemId) return;
        setIdentity({itemId: item.id, title: item.title, kind: item.kind, libraryId: item.libraryId, posterUrl: item.posterUrl, backdropUrl: item.backdropUrl, artist: item.song?.artist, album: item.song?.albumTitle, subtitle: item.song ? `${item.song.artist} · ${item.song.albumTitle}` : item.bookFile ? item.bookFile.bookTitle : item.episode ? `${item.episode.showTitle} · S${item.episode.seasonNumber ?? '?'} E${item.episode.number}` : item.year ? String(item.year) : undefined, item});
      })
      .catch(() => {});
    return () => abort.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [playingItemId, api, activeService]);

  const isAudio = identity.kind === 'song' || identity.kind === 'audiobook_file';
  const active = state.phase !== 'idle';
  const startSingle = useCallback((itemId: string, startSeconds: number, entry?: ContentEntry, prepared?: PreparedChoice, quality?: string) => {
    setIdentity({itemId, title: entry?.title ?? 'Now playing', subtitle: entry?.subtitle, posterUrl: entry?.posterUrl, backdropUrl: entry?.backdropUrl, kind: entry?.kind, libraryId: entry?.libraryId, seeded: !!(entry?.title && entry.kind && entry.libraryId && entry.id === itemId)});
    // A song starts in the mini player and leaves the listener on the page; Now Playing opens on click.
    setExpanded(entry?.kind !== 'song');
    v1.player.listening.interact();
    void v1.player.playItem(itemId, startSeconds, prepared, quality).catch(() => {});
  }, [v1]);
  /** Behind presence: older servers have no Dolby reason and read as no warning (fail open). */
  const checkDolbyFor = useCallback(async (itemId: string): Promise<boolean> => {
    try {
      const raw = await api.request<unknown>('/v1/items/' + encodeURIComponent(itemId) + '/playback-options');
      // MU4 COMPAT-01: keep the source facts from this same answer for the subtitle warning.
      sourceFactsRef.current.set(itemId, sourceFactsOf(raw));
      if (sourceFactsRef.current.size > 50) sourceFactsRef.current.delete(sourceFactsRef.current.keys().next().value!);
      return dolbyVisionWarningNeeded(raw);
    } catch {
      return false;
    }
  }, [api]);
  const play = useCallback((itemId: string, startSeconds: number, entry?: ContentEntry, prepared?: PreparedChoice, quality?: string) => {
    if (!mayCarryVideo(entry?.kind) || dolbyAcknowledged.current.has(itemId)) { startSingle(itemId, startSeconds, entry, prepared, quality); return; }
    void checkDolbyFor(itemId).then(needs => {
      if (!needs) startSingle(itemId, startSeconds, entry, prepared, quality);
      else setDolbyPending({kind: 'single', itemId, startSeconds, entry, prepared, quality});
    });
  }, [startSingle, checkDolbyFor]);
  const startSequenceInner = useCallback((entries: readonly ContentEntry[], index: number, startSeconds: number, container?: SequenceContainer) => {
    const playable = entries.slice(index).filter(e => e.playback);
    const first = playable[0];
    if (!first?.playback) return;
    if (!container && playable.length === 1) {
      startSingle(first.playback.itemId, startSeconds, first);
      return;
    }
    setIdentity({itemId: first.playback.itemId, title: first.title, subtitle: first.subtitle, posterUrl: first.posterUrl, backdropUrl: first.backdropUrl, kind: first.kind, libraryId: first.libraryId});
    setExpanded(first.kind !== 'song');
    v1.player.listening.interact();
    // A container plays as one server-snapshotted request, however large.
    if (container) {
      void v1.player.playSelector({container: {kind: container.kind, id: container.id}}, {shuffle: container.shuffle, ...(container.shuffle ? {} : {anchor: {itemId: first.playback.itemId}}), startSeconds}).catch(() => startSingle(first.playback!.itemId, startSeconds, first));
      return;
    }
    void v1.player.playEntries(playable.map(e => queueItemInput(e.playback!.itemId)), {startSeconds}).catch(() => startSingle(first.playback!.itemId, startSeconds, first));
  }, [v1, startSingle]);
  const playSequence = useCallback((entries: readonly ContentEntry[], index: number, startSeconds: number, container?: SequenceContainer) => {
    const first = entries.slice(index).filter(e => e.playback)[0];
    const firstId = first?.playback?.itemId;
    if (!firstId) { startSequenceInner(entries, index, startSeconds, container); return; }
    if (!mayCarryVideo(first?.kind) || dolbyAcknowledged.current.has(firstId)) { startSequenceInner(entries, index, startSeconds, container); return; }
    void checkDolbyFor(firstId).then(needs => {
      if (!needs) startSequenceInner(entries, index, startSeconds, container);
      else setDolbyPending({kind: 'sequence', entries, index, startSeconds, container});
    });
  }, [startSequenceInner, checkDolbyFor]);
  const confirmDolbyPending = useCallback(() => {
    const pending = dolbyPending;
    if (!pending) return;
    if (pending.kind === 'single') {
      dolbyAcknowledged.current.add(pending.itemId);
      setDolbyPending(null);
      startSingle(pending.itemId, pending.startSeconds, pending.entry, pending.prepared, pending.quality);
    } else {
      const first = pending.entries.slice(pending.index).filter(e => e.playback)[0];
      if (first?.playback) dolbyAcknowledged.current.add(first.playback.itemId);
      setDolbyPending(null);
      startSequenceInner(pending.entries, pending.index, pending.startSeconds, pending.container);
    }
  }, [dolbyPending, startSingle, startSequenceInner]);
  const tune = useCallback((channel: GuideChannel) => {
    // MU4 FEAT-07: the mini guide works on the channels this player tuned.
    noteTunedChannel(channel);
    setIdentity({title: channel.name, kind: 'channel', channel: true});
    setExpanded(true);
    void service.playChannel({kind: channel.provenance, sourceId: channel.sourceId, channelId: channel.id, generation: channel.generation}).catch(() => {});
  }, [service]);
  const retry = useCallback(() => {
    if (ownerStopOf(activeService.getSnapshot().ended)) return; // An owner's stop is never retried.
    void activeService.retry().catch(() => {});
  }, [activeService]);
  const groupControl = useRef<GroupControl | null>(null);
  const setGroupControl = useCallback((control: GroupControl | null) => { groupControl.current = control; }, []);
  const toggle = useCallback(() => {
    const s = activeService.getSnapshot();
    if (ownerStopOf(s.ended)) return; // Stopped by the server owner: play starts again only from a title.
    activeQueue?.listening.interact();
    if (s.phase === 'error') retry();
    else if (groupControl.current?.toggle(s.intent === 'playing')) return;
    else if (s.intent === 'playing') activeService.pause();
    else activeService.resume();
  }, [activeService, activeQueue, retry]);
  const seekBy = useCallback((delta: number) => {
    const s = activeService.getSnapshot();
    const target = Math.max(0, Math.min(s.duration || Infinity, (s.pendingSeek?.positionSeconds ?? s.positionSeconds) + delta));
    if (groupControl.current?.seek(target)) return;
    activeService.seek(target);
  }, [activeService]);
  const seekTo = useCallback((seconds: number) => { if (!groupControl.current?.seek(Math.max(0, seconds))) activeService.seek(Math.max(0, seconds)); }, [activeService]);
  const leave = useCallback(() => {
    activeQueue?.listening.clearTimer();
    activeService.leave();
    setExpanded(false);
  }, [activeService, activeQueue]);
  const next = useCallback(() => {
    const view = activeQueue?.getSnapshot().view;
    if (activeQueue && view?.next.available) void activeQueue.next(view.queue.id, view.queue.revision).catch(() => {});
  }, [activeQueue]);
  const previous = useCallback(() => {
    void activeQueue?.previous().catch(() => {});
  }, [activeQueue]);
  const advance = useCallback(async (mode: 'automatic' | 'manual' | 'still-watching') => {
    await activeQueue?.continueCompletion(mode);
  }, [activeQueue]);
  const value = useMemo<PlayerEngine>(() => ({
    service: activeService, state, queue: activeQueue, queueNotice, identity, active, isAudio, expanded, volume, muted, moreEntry, moreAnchor, moreOrigin, sourceFacts,
    play, playSequence, tune, more, clearMore: () => setMoreEntry(undefined), toggle, retry, seekBy, seekTo, leave,
    collapse: () => setExpanded(false), expand: () => setExpanded(true),
    setVolume: setVolumeState, setMuted: setMutedState,
    next, previous, advance, setGroupControl, describe: setIdentity,
    canNext: !!queueView?.next.available && !state.channel,
    canPrevious: !state.channel && !!queueView && queueView.items.length > 1,
  }), [activeService, state, activeQueue, queueNotice, identity, active, isAudio, expanded, volume, muted, moreEntry, moreAnchor, moreOrigin, sourceFacts, more, play, playSequence, tune, toggle, retry, seekBy, seekTo, leave, next, previous, advance, queueView, setGroupControl]);
  const actions = useMemo<PlayerIntents>(() => ({play, playSequence, tune, more}), [play, playSequence, tune, more]);
  const handOff = useMemo<MoreHandOff>(() => ({moreEntry, moreAnchor, moreOrigin, clearMore: () => setMoreEntry(undefined)}), [moreEntry, moreAnchor, moreOrigin]);
  const t = currentI18n().t;
  return <ActionsCtx.Provider value={actions}><MoreCtx.Provider value={handOff}><V1EventsCtx.Provider value={v1.events}><PlayingCtx.Provider value={active ? identity.itemId : undefined}><Ctx.Provider value={value}>{children}<ConfirmDialog open={!!dolbyPending} onOpenChange={o => { if (!o) setDolbyPending(null); }} title={t('player.dolbyVisionWarning')} confirmLabel={t('player.playAnyway')} cancelLabel={t('action.cancel')} destructive={false} onConfirm={confirmDolbyPending} /></Ctx.Provider></PlayingCtx.Provider></V1EventsCtx.Provider></MoreCtx.Provider></ActionsCtx.Provider>;
}

/** The engine when there is one. Pages that only offer a shortcut into the player use this. */
/** True when only the sub-second position (and the revision it bumps) differs. */
function samePace(a: PlaybackSnapshot, b: PlaybackSnapshot): boolean {
  if (Math.floor(a.positionSeconds) !== Math.floor(b.positionSeconds)) return false;
  const keys = new Set([...Object.keys(a), ...Object.keys(b)]) as Set<keyof PlaybackSnapshot>;
  for (const key of keys) {
    if (key === 'positionSeconds' || key === 'revision') continue;
    if (a[key] !== b[key]) return false;
  }
  return true;
}

export function usePlayerOptional(): PlayerEngine | null {
  return useContext(Ctx);
}
export function usePlayer(): PlayerEngine {
  const v = useContext(Ctx);
  if (!v) throw new Error('usePlayer must be used inside PlayerProvider.');
  return v;
}
