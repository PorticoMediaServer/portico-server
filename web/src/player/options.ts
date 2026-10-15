import {audioTrackLabel} from '@core/index.ts';
import {parseTrickplay, preferredTrickplaySet, type TrickplayView} from '@core/trickplay.ts';
import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import type {HttpLocalApi, PlaybackService} from '@core/index.ts';
import {PlayerOffersService, type PlayerOffersSnapshot} from '@core/player-offers.ts';
import {SubtitleService, fetchSubtitleDocument, type SubtitleDocument, type SubtitleSnapshot} from '@core/subtitles.ts';
import {PlayerChaptersService, type PlayerChaptersSnapshot} from '@core/player-chapters.ts';
import type {ContentScope} from '@core/library-content.ts';
import {drawnRenderingFor, readSubtitleFile, readSubtitleFontList, selectedSubtitleOriginal, type DrawnSubtitleFormat} from '@core/subtitle-originals.ts';
import {subtitleSessionChange} from './subtitles/selection';
import type {SubtitleSurface} from './subtitle-surface';
import type {SubtitleOverlay} from './subtitles/overlay';
import {subtitleRenderingKinds} from './subtitles/declaration';
import type {DrawnSubtitleSource, ReadSubtitleFile} from './subtitles/types';
import {currentI18n} from '../app/i18n';

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
      playback.audioUnavailable(intent, currentI18n().t('player.opt.audioChannel'));
      return;
    }
    if (result.query?.sessionId !== sessionId || (result.phase !== 'ready' && result.phase !== 'error' && result.phase !== 'refresh-required')) return;
    if (result.error && ['unauthorized', 'forbidden', 'permission_changed', 'stale_playback_offer', 'not_found'].includes(result.error.code)) {
      playback.invalidateAudioPlan(intent, currentI18n().t('player.opt.audioGone'));
      return;
    }
    const source = result.data?.sources.find(s => s.id === result.data?.current?.sourceId);
    if (source) { const tracks = source.streams.filter(s => s.type === 'audio' && s.enabled); playback.installAudioTracks(intent, tracks.map((s, i) => ({streamIndex: s.index, label: audioTrackLabel(s, i + 1), language: s.language, channels: s.channels, codec: s.codec})), (tracks.find(s => s.default) ?? tracks[0])?.index ?? null); }
    playback.audioUnavailable(intent, currentI18n().t(result.error ? 'player.opt.audioLoadFailed' : 'player.opt.audioFixed'));
  }, [result, playback, sessionId, generation, item?.id, item?.libraryId]); // eslint-disable-line react-hooks/exhaustive-deps
  return () => { try { void offers.service.refresh().catch(() => {}); } catch { /* disposed */ } };
}

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

/**
 * Subtitle catalogue for the session, and the chosen track on the picture: a text track's cue
 * document on the player's subtitle surface, or a styled or image track drawn on the overlay
 * canvas from its original (`subtitles/overlay.ts`).
 *
 * A track this browser can draw is asked for as its original, so the server leaves the video
 * alone. If drawing it fails here, the same track is asked for the ordinary way (text, or burned
 * in) and is not tried again in this viewing.
 *
 * Choices are made on the playing session itself (`PATCH` of the session, through
 * `PlaybackService.changePlayback`): the session then knows the track and how it is delivered, so
 * a change of quality or audio, or a re-plan after the player failed, comes back with the same
 * subtitle drawn the same way, and the choice is remembered for the next episode. That is the only
 * way a choice, "off" or a timing correction is made: a session that refuses is the answer, and
 * the subtitles stay as they were. The subtitle routes are read here (the catalogue, the plan,
 * the cue documents and the originals) and used to manage files; they are not asked to select.
 */
/**
 * `elsewhere`: the picture is on a surface the page cannot draw over. A drawn track is handed to
 * the session the ordinary way while it lasts, and drawn on the page again when the picture comes
 * back. In the browser's own Picture in Picture window ('window') that is text for a styled track
 * the server has as text, otherwise the subtitle burned into the picture. In an iPhone's
 * full-screen player ('system') only text is asked for: a new picture would end the full-screen
 * player, so a track that would have to be burned in waits for the page.
 */
export function usePlayerSubtitles(api: HttpLocalApi, scope: ContentScope, playback: PlaybackService, surface: SubtitleSurface | null, resourceKey: string, audio: boolean, overlay: SubtitleOverlay | null = null, elsewhere: false | 'window' | 'system' = false) {
  const state = useSessionFacts(playback);
  const available = !audio && !!state.sessionId && state.generation !== undefined && !!state.itemId && !state.channel && !['error', 'ended', 'idle'].includes(state.phase);
  const service = useMemo(() => available ? new SubtitleService(api, {itemId: state.itemId!, sessionId: state.sessionId!, generation: state.generation!}, () => crypto.randomUUID(), {install: p => playback.getSnapshot().intentId === state.intentId && playback.installSubtitlePresentation(p)}) : null, [api, scope.serverId, scope.viewerId, available, state.itemId, state.intentId, state.sessionId, state.generation]); // eslint-disable-line react-hooks/exhaustive-deps
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
  const stored = useSyncExternalStore(service?.subscribe ?? noop, service?.getSnapshot ?? (() => empty));
  /* A change made on the session is not the subtitle service's own: while one is in flight the menu
     is busy, and "off" hides the subtitle at once instead of after the round trip. */
  const [changing, setChanging] = useState(0);
  const [hiding, setHiding] = useState(false);
  useEffect(() => { if (hiding && (!stored.plan || stored.plan.mode === 'off')) setHiding(false); }, [hiding, stored.plan]);
  useEffect(() => { setHiding(false); setChanging(0); }, [service]);
  const subtitles = useMemo<SubtitleSnapshot>(() => (changing || hiding ? {...stored, busy: stored.busy || changing > 0, suppressed: stored.suppressed || hiding} : stored), [stored, changing, hiding]);
  const [rendererError, setRendererError] = useState<string | null>(null);
  const [retry, setRetry] = useState(0);
  const plan = subtitles.plan;
  /* Timing: a nudge moves the lines at once on this device; it is saved to the profile once the
     viewer stops adjusting, so a fine correction is not a round trip per step. */
  const [nudged, setNudged] = useState<number | null>(null);
  const savedOffset = Number(plan?.offsetUs ?? 0) / 1e6;
  const offsetSeconds = nudged ?? savedOffset;
  const shown = useRef<{doc: SubtitleDocument; url: string} | null>(null);
  useEffect(() => { setNudged(null); }, [plan?.selected?.id, plan?.offsetUs]);
  useEffect(() => { if (surface && shown.current && nudged !== null) surface.show(shown.current.doc.cues, nudged); if (overlay && nudged !== null) overlay.setDelay(nudged); }, [surface, overlay, nudged]);
  const selectedId = plan?.selected?.id;
  /* Drawn subtitles. `draws` is what this browser renders itself; a track that failed to draw in
     this viewing is in `undrawable` and is asked for the ordinary way from then on. */
  const draws = useMemo(() => (overlay ? subtitleRenderingKinds() : ['text']), [overlay]);
  const undrawable = useRef(new Set<string>());
  const upgraded = useRef(new Set<string>());
  const away = useRef(elsewhere);
  away.current = elsewhere;
  const renderingFor = (resource: {id: string}) => (undrawable.current.has(resource.id) || away.current ? undefined : drawnRenderingFor(resource, draws));
  type Track = NonNullable<SubtitleSnapshot['plan']>['resources'][number];
  /**
   * Selects a track (or none) with a delivery: the original for this browser to draw, or the
   * ordinary way. One request, to the playing session. A session that has moved on since the
   * choice was made (another change landed first) is read again and asked once more; any other
   * refusal is the server's answer and rejects, with the subtitles left as they were.
   */
  const select = async (resource: Track | null, rendering: DrawnSubtitleFormat | undefined, offsetUs?: string): Promise<void> => {
    if (!service || !playback.changesInPlace()) throw new Error(currentI18n().t('player.subtitles.notChangeable'));
    const change = subtitleSessionChange(resource, rendering, draws, resource ? offsetUs ?? resource.offsetUs : '0');
    setChanging(count => count + 1);
    if (!resource) setHiding(true);
    try {
      try { await playback.changePlayback(change); }
      catch (error) {
        if ((error as {status?: unknown} | null)?.status !== 409) throw error;
        await service.refresh();
        await playback.changePlayback(change);
      }
      void service.refresh();
    } catch (error) {
      setHiding(false);
      throw error;
    } finally {
      setChanging(count => Math.max(0, count - 1));
    }
  };
  const original = service && overlay && !subtitles.suppressed ? selectedSubtitleOriginal(plan) : null;
  const originalKey = original ? [original.format, original.url, original.companionUrl ?? '', plan?.selected?.revision ?? ''].join('|') : '';
  const drawnKey = useRef('');
  const drawRun = useRef(0);
  const [drawing, setDrawing] = useState(false);
  const [drawFallbacks, setDrawFallbacks] = useState(0);
  useEffect(() => {
    const selected = plan?.selected;
    if (nudged === null || !service || !selected) return;
    // The timing is saved for the delivery in use: a drawn track stays drawn.
    const timer = setTimeout(() => { void select(selected, selectedSubtitleOriginal(plan)?.format, String(Math.round(nudged * 1e6))).catch(() => {}); }, 900);
    return () => clearTimeout(timer);
  }, [nudged, service, selectedId]); // eslint-disable-line react-hooks/exhaustive-deps
  /* A track selected for the viewer (by the server at the start, or carried from the last
     episode) as text or burned in, which this browser can draw: ask for its original, once. */
  useEffect(() => {
    const selected = plan?.selected;
    if (!service || !overlay || !plan || !selected || plan.mode !== 'track' || subtitles.suppressed || subtitles.busy || selectedSubtitleOriginal(plan)) return;
    const rendering = renderingFor(selected);
    // Once for a track in a session: a server that answers with text or a burn-in again is not asked twice.
    const once = plan.sessionId + ':' + selected.id;
    if (!rendering || upgraded.current.has(once)) return;
    upgraded.current.add(once);
    void select(selected, rendering, plan.offsetUs).catch(() => {});
  }, [service, overlay, selectedId, plan?.renderer, plan?.mode, subtitles.suppressed, subtitles.busy]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!overlay) return;
    if (!original || !service || !plan?.selected) { drawRun.current++; drawnKey.current = ''; overlay.clear(); setDrawing(false); setForcedChoice(false); return; }
    overlay.setDelay(Number(plan.offsetUs) / 1e6);
    if (drawnKey.current === originalKey && overlay.active) return;
    // The server handed over the original of a track that already failed to draw here: nothing is tried again.
    if (undrawable.current.has(plan.selected.id)) { overlay.clear(); setDrawing(false); setRendererError(currentI18n().t('player.subtitles.drawUnavailable')); return; }
    drawnKey.current = originalKey;
    const run = ++drawRun.current, current = () => drawRun.current === run, selected = plan.selected, offsetUs = plan.offsetUs;
    const absolute = (path: string) => { const url = api.mediaUrl(path); if (new URL(url).origin !== new URL(api.baseUrl).origin) throw new Error('Unexpected subtitle origin'); return url; };
    const read: ReadSubtitleFile = (url, maxBytes, signal) => readSubtitleFile(fetch, url, maxBytes, signal);
    const failed = () => {
      if (!current()) return;
      drawRun.current++;
      drawnKey.current = '';
      overlay.clear();
      setDrawing(false);
      // Shown another way: text for a styled track, burned in for an image track.
      undrawable.current.add(selected.id);
      setDrawFallbacks(count => count + 1);
      // Asked for the ordinary way. If the session refuses that too, nothing can show these subtitles here.
      void select(selected, undefined, offsetUs).catch(() => { if (current() || drawRun.current === run + 1) setRendererError(currentI18n().t('player.subtitles.drawUnavailable')); });
    };
    setDrawing(true);
    setRendererError(null);
    void (async () => {
      try {
        let source: DrawnSubtitleSource;
        // Use the original, companion and font paths supplied by this plan.
        // Their namespace belongs to the server, not the renderer.
        if (original.format === 'ass') {
          // The fonts attached to the file; a subtitle whose font list cannot be read is still drawn, in the fallback fonts.
          let fonts: string[] = [];
          if (original.fontsUrl) { try { fonts = (await readSubtitleFontList(fetch, absolute(original.fontsUrl), original.fontsUrl, new AbortController().signal)).map(font => absolute(font.url)); } catch { fonts = []; } }
          source = {kind: 'ass', script: absolute(original.url), fonts};
        } else if (original.format === 'vobsub') source = {kind: 'vobsub', index: absolute(original.url), data: absolute(original.companionUrl ?? '')};
        else source = {kind: original.format, data: absolute(original.url)};
        if (!current()) return;
        await overlay.present(source, read, failed);
        if (current()) { setDrawing(false); setForcedChoice(overlay.forcedChoice); }
      } catch { failed(); }
    })();
  }, [api, overlay, service, originalKey, plan?.offsetUs]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => { drawRun.current++; drawnKey.current = ''; overlay?.clear(); }, [overlay, service]);
  /* The picture went where the page cannot draw: the drawn track is asked for the ordinary way,
     and drawn again once the picture is back on the page (the same track, if it is still chosen). */
  const handedOver = useRef<string | null>(null);
  const [forcedChoice, setForcedChoice] = useState(false);
  useEffect(() => {
    const selected = plan?.selected;
    if (!service || !overlay || !selected || plan?.mode !== 'track' || subtitles.suppressed) { if (!elsewhere) handedOver.current = null; return; }
    if (elsewhere && selectedSubtitleOriginal(plan) && handedOver.current !== selected.id && (elsewhere === 'window' || selected.renderer !== 'burn_in')) {
      handedOver.current = selected.id;
      void select(selected, undefined, plan.offsetUs).catch(() => { handedOver.current = null; });
    } else if (!elsewhere && handedOver.current === selected.id) {
      handedOver.current = null;
      const rendering = renderingFor(selected);
      if (rendering && !selectedSubtitleOriginal(plan)) void select(selected, rendering, plan.offsetUs).catch(() => {});
    }
  }, [elsewhere, service, overlay, selectedId, plan?.mode, plan?.renderer, subtitles.suppressed]); // eslint-disable-line react-hooks/exhaustive-deps
  const choose = (resource: Track, offsetUs?: string): Promise<void> => select(resource, renderingFor(resource), offsetUs);
  const turnOff = (): Promise<void> => select(null, undefined);
  /** Turns on a track the server found beside the file but has not listed among the choices yet: it is selected like any other, on the session. */
  const add = (found: {id: string; sourceId: string; reason?: string}): Promise<void> => {
    const resource = plan?.resources.find(r => r.discoveryId === found.id && r.sourceId === found.sourceId);
    return resource ? choose(resource) : Promise.reject(new Error(currentI18n().t('player.subtitles.changeFailed')));
  };
  const nudge = (delta: number): number | undefined => {
    if (!plan?.selected || plan.mode !== 'track' || subtitles.suppressed) return undefined;
    const next = Math.max(-600, Math.min(600, Math.round((offsetSeconds + delta) * 10) / 10));
    setNudged(next);
    return next;
  };
  useEffect(() => {
    if (!service) { surface?.clear(); shown.current = null; return; }
    if (!surface) {
      setRendererError(plan?.renderer === 'burn_in' || plan?.mode === 'off' ? null : currentI18n().t('player.subtitles.textUnsupported'));
      return;
    }
    let active = true, doc: SubtitleDocument | null = null, controller: AbortController | undefined, etag = '', etagUrl = '';
    let again: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      if (!active || !surface) return;
      if (!plan?.documentUrl || subtitles.suppressed || plan.mode !== 'track') {
        surface.clear();
        shown.current = null;
        return;
      }
      clearTimeout(again);
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
        setRendererError(null);
        if (read.unchanged) return;
        const result = read.document;
        etag = read.etag;
        etagUrl = url;
        // The periodic reload only confirms access; rebuilding every cue for an unchanged
        // document is a main-thread hitch every 25 seconds.
        if (doc && sameDocument(doc, result)) return;
        doc = result;
        shown.current = {doc: result, url};
        surface.show(result.cues, Number(plan.offsetUs) / 1e6);
      } catch (error) {
        if (!active || c.signal.aborted) return;
        // The server saying no ends the subtitle at once. A server that cannot be reached does
        // not: the lines already loaded stay for as long as the picture itself plays on from its
        // buffer, and the next attempt comes soon rather than a full interval later.
        const status = (error as {status?: unknown} | null)?.status;
        const refused = typeof status === 'number' && [401, 403, 404, 410].includes(status);
        if (doc && !refused) again = setTimeout(() => void load(), 5000);
        else {
          doc = null;
          shown.current = null;
          surface.clear();
          setRendererError(currentI18n().t('player.subtitles.drawUnavailable'));
        }
      } finally {
        clearTimeout(timeout);
      }
    }
    // The same track with a new timing keeps its lines on screen while it is read again.
    const kept = shown.current;
    let same = false;
    try { same = !!kept && !!plan?.documentUrl && plan.mode === 'track' && !subtitles.suppressed && kept.url === api.mediaUrl(plan.documentUrl); } catch { same = false; }
    if (same && kept && plan) surface.show(kept.doc.cues, Number(plan.offsetUs) / 1e6);
    else { surface.clear(); shown.current = null; }
    surface.guard(true);
    setRendererError(null);
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
      clearTimeout(again);
      document.removeEventListener('visibilitychange', focus);
      surface.guard(false);
    };
  }, [api, service, surface, resourceKey, plan?.documentUrl, plan?.offsetUs, plan?.revision, plan?.mode, subtitles.suppressed, retry]); // eslint-disable-line react-hooks/exhaustive-deps
  return {
    service, snapshot: subtitles, rendererError, retryRenderer: () => setRetry(v => v + 1), offsetSeconds, nudge,
    /** Selects a track, as its original when this browser draws that kind. */
    choose,
    /** Turns subtitles off. */
    turnOff,
    /** Turns on a track found beside the file. */
    add,
    /** Whether this browser draws this track itself (so the video is not converted for it). */
    draws: (resource: {id: string}) => renderingFor(resource) !== undefined,
    /** The kind being drawn on the canvas now, and whether it is still loading. */
    drawnKind: original?.format ?? null, drawing,
    /** The image subtitle on screen marks some lines forced and not others: "Forced only" applies. */
    forcedChoice: forcedChoice && !!original,
    /** Counts the times a track could not be drawn and was shown another way. */
    drawFallbacks,
  };
}

/** Chapter list for the active session; seeking stays with the playback service. */
export function usePlayerChapters(api: HttpLocalApi, scope: ContentScope, item: {id: string; libraryId: string} | undefined, playback: PlaybackService, enabled: boolean, bookPosition?: number) {
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
  useEffect(() => {
    if (bookPosition === undefined || !data || data.status !== 'available') return;
    const first = data.chapters[0], last = data.chapters.at(-1);
    // A book's system metadata follows deep seeks through the same bounded, source-fenced pages.
    if (last && bookPosition >= last.endSeconds && data.nextCursor) void service.next().catch(() => {});
    else if (first && bookPosition < first.startSeconds && state.pagination.canPrevious) void service.previous().catch(() => {});
  }, [service, data, bookPosition, state.pagination.canPrevious]);
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
