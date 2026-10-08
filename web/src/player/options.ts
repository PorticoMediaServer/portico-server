import {audioTrackLabel} from '@core/index.ts';
import {parseTrickplay, preferredTrickplaySet, type TrickplayView} from '@core/trickplay.ts';
import {useEffect, useMemo, useState, useSyncExternalStore, type RefObject} from 'react';
import type {HttpLocalApi, PlaybackService} from '@core/index.ts';
import {PlayerOffersService, type PlayerOffersSnapshot} from '@core/player-offers.ts';
import {SubtitleService, fetchSubtitleDocument, type SubtitleDocument, type SubtitleSnapshot} from '@core/subtitles.ts';
import {PlayerChaptersService, type PlayerChaptersSnapshot} from '@core/player-chapters.ts';
import type {ContentScope} from '@core/library-content.ts';

/*
 * Option services for the active playback session: audio plan installation,
 * quality/version offers, subtitle catalogue and rendering, chapters. Each
 * binds a core service to the current session and fences stale results.
 */

const empty: SubtitleSnapshot = {catalog: null, plan: null, busy: false, loading: false, suppressed: true, error: null, canRetry: false, candidates: []};
const noop = () => () => {};

/** Installs the session's audio plan into the playback service so track selection works. */
function sameDocument(a: {cues: readonly unknown[]}, b: {cues: readonly unknown[]}): boolean {
  if (a.cues.length !== b.cues.length) return false;
  return JSON.stringify(a.cues) === JSON.stringify(b.cues);
}

/**
 * Quality and version offers for the current session (Automatic, original, prepared copies), read
 * once per session generation (PERF-24: one `/playback-offers` request per play, none before a
 * session exists). The audio plan below reads the same answer.
 */
export function usePlayerOffers(api: HttpLocalApi, scope: ContentScope, item: {id: string; libraryId: string} | undefined, sessionId?: string, generation?: number) {
  const service = useMemo(() => new PlayerOffersService({api, scope: {serverId: scope.serverId, viewerId: scope.viewerId}}), [api, scope.serverId, scope.viewerId, item?.id, item?.libraryId, sessionId, generation]); // eslint-disable-line react-hooks/exhaustive-deps
  const state = useSyncExternalStore<PlayerOffersSnapshot>(service.subscribe, service.getSnapshot);
  useEffect(() => {
    if (!item || !sessionId) return () => service.dispose();
    try {
      void service.select({libraryId: item.libraryId, itemId: item.id, sessionId}).catch(() => {});
    } catch {}
    return () => service.dispose();
  }, [service, item?.libraryId, item?.id, sessionId]); // eslint-disable-line react-hooks/exhaustive-deps
  const data = state.phase === 'ready' && item && state.data?.scope.itemId === item.id && state.data.scope.libraryId === item.libraryId ? state.data : null;
  return {service, state, data};
}

/** Installs the session's audio plan into the playback service so track selection works, from the
 * session's offers (`usePlayerOffers`). Returns a refresh that reads the offers again. */
export function usePlayerAudioPlan(offers: {service: PlayerOffersService; state: PlayerOffersSnapshot}, item: {id: string; libraryId: string} | undefined, playback: PlaybackService, sessionId?: string, generation?: number) {
  const result = offers.state;
  useEffect(() => {
    if (!item || !sessionId) return;
    const captured = playback.getSnapshot(), intent = captured.intentId;
    if (captured.session?.id !== sessionId || captured.session?.generation !== generation) return;
    if (captured.channel) {
      playback.audioUnavailable(intent, 'The channel uses its programme audio.');
      return;
    }
    if (result.query?.sessionId !== sessionId || (result.phase !== 'ready' && result.phase !== 'error' && result.phase !== 'refresh-required')) return;
    if (result.error && ['unauthorized', 'forbidden', 'permission_changed', 'stale_playback_offer', 'not_found'].includes(result.error.code)) {
      playback.invalidateAudioPlan(intent, 'Audio options are no longer available for this session.');
      return;
    }
    const source = result.data?.sources.find(s => s.id === result.data?.current?.sourceId);
    if (source) { const tracks = source.streams.filter(s => s.type === 'audio' && s.enabled); playback.installAudioTracks(intent, tracks.map((s, i) => ({streamIndex: s.index, label: audioTrackLabel(s, i + 1), language: s.language, channels: s.channels, codec: s.codec})), (tracks.find(s => s.default) ?? tracks[0])?.index ?? null); }
    playback.audioUnavailable(intent, result.error ? 'Audio options could not be loaded.' : 'Audio selection is not available for this stream.');
  }, [result, playback, sessionId, generation, item?.id, item?.libraryId]); // eslint-disable-line react-hooks/exhaustive-deps
  return () => { try { void offers.service.refresh().catch(() => {}); } catch { /* disposed */ } };
}

const tracks = new WeakMap<HTMLVideoElement, TextTrack>();

type SessionFacts = {itemId?: string; intentId: number; sessionId?: string; generation?: number; channel: boolean; phase: string};
/** PERF-20: the fields the option hooks read, as one string key, so the position ticks
 *  (≈ 4 Hz, 20 Hz with audio effects) don't re-render the player through these hooks. */
function useSessionFacts(playback: PlaybackService): SessionFacts {
  const key = useSyncExternalStore(playback.subscribe, () => {
    const s = playback.getSnapshot();
    return [s.itemId ?? '', s.intentId, s.session?.id ?? '', s.session?.generation ?? '', s.channel ? 1 : 0, s.phase].join('|');
  });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useMemo(() => { const s = playback.getSnapshot(); return {itemId: s.itemId ?? undefined, intentId: s.intentId, sessionId: s.session?.id, generation: s.session?.generation, channel: !!s.channel, phase: s.phase}; }, [playback, key]);
}

/** Subtitle catalogue for the session and a WebVTT text-track renderer bound to the video element. */
export function usePlayerSubtitles(api: HttpLocalApi, scope: ContentScope, playback: PlaybackService, video: RefObject<HTMLVideoElement | null>, resourceKey: string, audio: boolean) {
  const state = useSessionFacts(playback);
  const available = !audio && !!state.sessionId && state.generation !== undefined && !!state.itemId && !state.channel && !['error', 'ended', 'idle'].includes(state.phase);
  const service = useMemo(() => available ? new SubtitleService(api, {itemId: state.itemId!, sessionId: state.sessionId!, generation: state.generation!}, () => crypto.randomUUID(), {positionUs: () => String(Math.max(0, Math.round((playback.getSnapshot().pendingSeek?.positionSeconds ?? playback.getSnapshot().positionSeconds) * 1e6))), install: p => playback.getSnapshot().intentId === state.intentId && playback.installSubtitlePresentation(p)}) : null, [api, scope.serverId, scope.viewerId, available, state.itemId, state.intentId, state.sessionId, state.generation]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    service?.start();
    const focus = () => {
      if (document.visibilityState === 'visible') void service?.refresh();
    };
    document.addEventListener('visibilitychange', focus);
    return () => {
      document.removeEventListener('visibilitychange', focus);
      service?.stop();
    };
  }, [service]);
  const subtitles = useSyncExternalStore(service?.subscribe ?? noop, service?.getSnapshot ?? (() => empty));
  const [rendererError, setRendererError] = useState<string | null>(null);
  const [retry, setRetry] = useState(0);
  const plan = subtitles.plan;
  useEffect(() => {
    const el = video.current;
    if (!el || !service) return;
    if (typeof el.addTextTrack !== 'function' || typeof VTTCue !== 'function') {
      setRendererError(plan?.renderer === 'burn_in' || plan?.mode === 'off' ? null : 'This browser cannot render text subtitles.');
      return;
    }
    let active = true, doc: SubtitleDocument | null = null, authorizedUntil = 0, controller: AbortController | undefined, etag = '', etagUrl = '';
    let retry: ReturnType<typeof setTimeout> | undefined;
    let track = tracks.get(el);
    if (!track) {
      track = el.addTextTrack('subtitles', 'Portico subtitles', 'und');
      tracks.set(el, track);
    }
    const owned = track;
    const clear = () => {
      // A disabled track lists no cues (`cues` is null), so they are removed while it can still
      // name them; otherwise every re-render stacks another copy of each line.
      if (owned.mode === 'disabled') owned.mode = 'hidden';
      if (owned.cues) for (const cue of Array.from(owned.cues)) owned.removeCue(cue);
      owned.mode = 'disabled';
    };
    const exclusive = () => {
      for (const t of Array.from(el.textTracks)) if (t !== owned && (t.kind === 'subtitles' || t.kind === 'captions') && t.mode !== 'disabled') t.mode = 'disabled';
    };
    const render = () => {
      clear();
      exclusive();
      if (!active || !doc || Date.now() > authorizedUntil || subtitles.suppressed || plan?.mode !== 'track') return;
      const mapping = Number(el.dataset.porticoSubtitleOffset ?? '0');
      if (!Number.isFinite(mapping)) return;
      const offset = Number(plan.offsetUs) / 1e6 + mapping;
      for (const cue of doc.cues) {
        const start = Number(cue.startUs) / 1e6 + offset, end = Number(cue.endUs) / 1e6 + offset;
        if (end <= 0) continue;
        const words = cue.text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        const vtt = new VTTCue(Math.max(0, start), end, cue.italic ? '<i>' + words + '</i>' : words);
        vtt.align = 'center';
        // A cue authored at the top of the picture (a sign, a second speaker) stays there.
        if (cue.top) { vtt.snapToLines = true; vtt.line = 0; }
        owned.addCue(vtt);
      }
      owned.mode = 'showing';
    };
    const clock = () => {
      if (Date.now() > authorizedUntil && owned.mode !== 'disabled') {
        clear();
        setRendererError('Subtitle access expired. Refreshing…');
      }
    };
    async function load() {
      if (!active || !plan?.documentUrl || subtitles.suppressed || plan.mode !== 'track') {
        clear();
        return;
      }
      clearTimeout(retry);
      controller?.abort();
      const c = new AbortController();
      controller = c;
      const timeout = setTimeout(() => c.abort(), 15000);
      try {
        const url = api.mediaUrl(plan.documentUrl);
        if (new URL(url).origin !== new URL(api.baseUrl).origin) throw new Error('Unexpected subtitle origin');
        // The reload exists to stay authorised. The server checks in full every time and
        // answers 304 when the validator still names the document, so an unchanged track
        // costs a header rather than the whole document every 25 seconds.
        const read = await fetchSubtitleDocument(fetch, url, doc && etagUrl === url ? etag : '', c.signal);
        if (!active || c.signal.aborted) return;
        if (read.unchanged) {
          authorizedUntil = Date.now() + 75000;
          setRendererError(null);
          return;
        }
        const result = read.document;
        etag = read.etag;
        etagUrl = url;
        // Long enough to ride out two failed reloads (25 s apart, retried at 5 s) before cues lapse.
        authorizedUntil = Date.now() + 75000;
        setRendererError(null);
        // The periodic reload only extends authorisation; rebuilding every cue
        // for an unchanged document is a main-thread hitch every 25 seconds.
        if (doc && sameDocument(doc, result)) return;
        doc = result;
        render();
      } catch {
        if (active && !c.signal.aborted) {
          // A failed reload is not a reason to take subtitles off the screen: the cues already
          // loaded stay until the authorisation window really does lapse, and the next attempt
          // comes soon rather than a full interval later. Only a first load has nothing to show.
          if (doc && Date.now() <= authorizedUntil) retry = setTimeout(() => void load(), 5000);
          else {
            doc = null;
            clear();
            setRendererError('Subtitles could not be loaded.');
          }
        }
      } finally {
        clearTimeout(timeout);
      }
    }
    clear();
    exclusive();
    setRendererError(null);
    el.textTracks.addEventListener('change', exclusive);
    el.textTracks.addEventListener('addtrack', exclusive);
    el.addEventListener('portico-subtitle-timeline', render);
    el.addEventListener('timeupdate', clock);
    const focus = () => {
      if (document.visibilityState === 'visible') void load();
    };
    document.addEventListener('visibilitychange', focus);
    void load();
    const timer = setInterval(() => void load(), 25000);
    return () => {
      active = false;
      controller?.abort();
      clearInterval(timer);
      clearTimeout(retry);
      el.textTracks.removeEventListener('change', exclusive);
      el.textTracks.removeEventListener('addtrack', exclusive);
      el.removeEventListener('portico-subtitle-timeline', render);
      el.removeEventListener('timeupdate', clock);
      document.removeEventListener('visibilitychange', focus);
      clear();
    };
  }, [api, service, video, resourceKey, plan?.documentUrl, plan?.offsetUs, plan?.revision, subtitles.suppressed, retry]); // eslint-disable-line react-hooks/exhaustive-deps
  return {service, snapshot: subtitles, rendererError, retryRenderer: () => setRetry(v => v + 1)};
}

/** Chapter list for the active session; seeking stays with the playback service. */
export function usePlayerChapters(api: HttpLocalApi, scope: ContentScope, item: {id: string; libraryId: string} | undefined, playback: PlaybackService, enabled: boolean) {
  const player = useSessionFacts(playback);
  const sessionId = player.sessionId, sessionGeneration = player.generation;
  const active = enabled && !!item && player.itemId === item.id && !!sessionId && !!sessionGeneration && (player.phase === 'starting' || player.phase === 'ready');
  const service = useMemo(() => new PlayerChaptersService({api, scope: {serverId: scope.serverId, viewerId: scope.viewerId}}), [api, scope.serverId, scope.viewerId, item?.id, item?.libraryId, sessionId, sessionGeneration]); // eslint-disable-line react-hooks/exhaustive-deps
  const state = useSyncExternalStore<PlayerChaptersSnapshot>(service.subscribe, service.getSnapshot);
  useEffect(() => {
    if (active && sessionId && sessionGeneration && item) {
      try {
        void service.select({libraryId: item.libraryId, itemId: item.id, sessionId, sessionGeneration}).catch(() => {});
      } catch {}
    } else service.cancel();
    return () => service.cancel();
  }, [service, active, sessionId, sessionGeneration, item?.libraryId, item?.id]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => service.dispose(), [service]);
  const data = active && state.phase === 'ready' && state.data?.scope.sessionId === sessionId && state.data.scope.generation === sessionGeneration ? state.data : null;
  return {service, state, data, active};
}

/** Trickplay sets for the playing item; the finest current set wins. */
export function useTrickplay(api: HttpLocalApi, scope: ContentScope, item: {id: string; libraryId: string} | undefined, sourceId: string | undefined, enabled: boolean) {
  const [view, setView] = useState<TrickplayView | null>(null);
  useEffect(() => {
    setView(null);
    if (!item || !enabled) return;
    const abort = new AbortController();
    api.request<unknown>('/v1/items/' + encodeURIComponent(item.id) + '/trickplay', 'GET', undefined, abort.signal)
      .then(raw => { if (!abort.signal.aborted) setView(parseTrickplay(raw, scope.serverId, {libraryId: item.libraryId, itemId: item.id})); })
      .catch(() => {});
    return () => abort.abort();
  }, [api, scope.serverId, item?.id, item?.libraryId, enabled]); // eslint-disable-line react-hooks/exhaustive-deps
  return useMemo(() => (view ? preferredTrickplaySet(view, sourceId) : null), [view, sourceId]);
}
