import {
  defaultPlaybackQuality,
  effectivePlaybackVolume,
  playbackResourceUrl,
  playbackSourceFor,
  type MediaItem,
  type PlaybackHandoffRequest,
  type PlaybackPrepareNextRequest,
  type PlaybackPreparedResponse,
  type PlaybackResponse,
  type PlaybackSessionQueueResponse,
} from '@porticomediaserver/client-core';
import { type RefObject, useEffect, useRef } from 'react';
import { usePorticoDataSource } from '../../data/DataProvider';
import type { MusicPlaybackPreferences } from '../../data/models';
import { useOptionalWebDisplayPreferences } from '../../preferences/WebDisplayPreferencesProvider';
import { defaultWebDisplayPreferences, webPlaybackIntent, type WebDisplayPreferences } from '../../preferences/webDisplayPreferences';

type MusicTransitionBridgeProps = {
  playback: PlaybackResponse;
  queue: PlaybackSessionQueueResponse | undefined;
  mediaRef: RefObject<HTMLVideoElement | null>;
  preferences: MusicPlaybackPreferences;
  volume: number;
  muted: boolean;
  enabled: boolean;
  onTransitioning: (transitioning: boolean) => void;
  handoff: (request: PlaybackHandoffRequest) => Promise<PlaybackResponse | undefined>;
  prepareNext: (candidate: MediaItem, request: PlaybackPrepareNextRequest, force?: boolean) => Promise<PlaybackPreparedResponse>;
};

type PreparedTrack = {
  response: PlaybackPreparedResponse;
  candidate: MediaItem;
  sourceURL: string;
  canOverlap: boolean;
};

type ActiveContinuation = {
  targetMediaId: string;
  audio: HTMLAudioElement;
  stop?: () => void;
};

export function MusicTransitionBridge({
  playback,
  queue,
  mediaRef,
  preferences,
  volume,
  muted,
  enabled,
  onTransitioning,
  handoff,
  prepareNext,
}: MusicTransitionBridgeProps) {
  const source = usePorticoDataSource();
  const webPreferences = useOptionalWebDisplayPreferences()?.preferences ?? defaultWebDisplayPreferences;
  const preloadRef = useRef<HTMLAudioElement>(null);
  const preparedRef = useRef<PreparedTrack | undefined>(undefined);
  const preparingRef = useRef(false);
  const transitioningRef = useRef(false);
  const continuationRef = useRef<ActiveContinuation | undefined>(undefined);
  const sourceRef = useRef(source);
  const volumeRef = useRef(volume);
  const mutedRef = useRef(muted);
  const onTransitioningRef = useRef(onTransitioning);
  const handoffRef = useRef(handoff);
  const prepareNextRef = useRef(prepareNext);
  sourceRef.current = source;
  volumeRef.current = volume;
  mutedRef.current = muted;
  onTransitioningRef.current = onTransitioning;
  handoffRef.current = handoff;
  prepareNextRef.current = prepareNext;
  const candidate = nextCandidate(playback, queue);
  const transitionOwnerKey = musicTransitionOwnerKey(playback, queue, preferences, webPreferences);

  useEffect(() => () => {
    const continuation = continuationRef.current;
    if (!continuation) return;
    continuation.stop?.();
    if (!continuation.stop) stopPreloadedAudio(continuation.audio);
    continuationRef.current = undefined;
  }, []);

  useEffect(() => {
    const media = mediaRef.current;
    const preload = preloadRef.current;
    if (!enabled || !media || !preload) return;
    if (!candidate || (!preferences.gapless && preferences.crossfadeSeconds <= 0)) return;
    let disposed = false;
    const controller = new AbortController();
    const activeContinuation = continuationRef.current;
    if (activeContinuation && activeContinuation.targetMediaId !== playback.media.id) {
      activeContinuation.stop?.();
      if (!activeContinuation.stop) stopPreloadedAudio(activeContinuation.audio);
      continuationRef.current = undefined;
    } else if (activeContinuation && !activeContinuation.stop) {
      activeContinuation.stop = continuePreloadedAudioUntilPrimaryPlays({
        primary: media,
        preload: activeContinuation.audio,
        volume: volumeRef.current,
        muted: mutedRef.current,
        normalization: playback.media.audioNormalization,
        normalizationMode: preferences.normalizationMode,
        onSettled: () => {
          if (continuationRef.current === activeContinuation) continuationRef.current = undefined;
          transitioningRef.current = false;
          onTransitioningRef.current(false);
        },
      });
    }
    preparedRef.current = undefined;
    preparingRef.current = false;
    transitioningRef.current = Boolean(continuationRef.current);
    onTransitioningRef.current(Boolean(continuationRef.current));

    const prepare = async () => {
      if (preparingRef.current || preparedRef.current || continuationRef.current || disposed) return;
      preparingRef.current = true;
      try {
        const response = await prepareNextRef.current(candidate, musicTransitionRequest(playback, queue, preferences, webPreferences));
        if (disposed || controller.signal.aborted) return;
        const resolve = (path: string) => playbackResourceUrl(response.playback, path, (value) => sourceRef.current.playbackResourceUrl(value), window.location.href);
        const sourceURL = playbackSourceFor(response.playback, resolve, {
          quality: defaultPlaybackQuality(response.playback),
          baseHref: window.location.href,
        });
        const isHLS = response.playback.streamFormat === 'hls' || sourceURL.includes('.m3u8');
        const canOverlap = !isHLS || Boolean(preload.canPlayType('application/vnd.apple.mpegurl') || preload.canPlayType('application/x-mpegURL'));
        preparedRef.current = { response, candidate, sourceURL, canOverlap };
        if (canOverlap) {
          preload.src = sourceURL;
          preload.preload = 'auto';
          preload.load();
        }
      } catch {
        // The ordinary ended -> next flow remains the reliability fallback.
      } finally {
        preparingRef.current = false;
      }
    };

    const transition = async () => {
      const prepared = preparedRef.current;
      if (!prepared || transitioningRef.current || disposed) return;
      transitioningRef.current = true;
      onTransitioningRef.current(true);
      const fadeSeconds = prepared.canOverlap ? preferences.crossfadeSeconds : 0;
      let preloadStarted = false;
      // Zero-crossfade owns a direct prepared handoff. Waiting for a secondary
      // element's play promise can outlive the primary ended event and suppress
      // the sole terminal consumer without ever emitting the handoff request.
      if (fadeSeconds > 0 && prepared.canOverlap && preload.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
        preload.muted = mutedRef.current;
        preload.volume = fadeSeconds > 0 ? 0 : effectivePlaybackVolume(volumeRef.current, prepared.response.playback.media.audioNormalization, preferences.normalizationMode);
        try {
          await preload.play();
          preloadStarted = true;
        } catch {
          preloadStarted = false;
        }
      }
      if (preloadStarted && fadeSeconds > 0) {
        const startedAt = performance.now();
        while (!disposed) {
          const ratio = Math.min(1, (performance.now() - startedAt) / (fadeSeconds * 1_000));
          media.volume = effectivePlaybackVolume(volumeRef.current * (1 - ratio), playback.media.audioNormalization, preferences.normalizationMode);
          preload.volume = effectivePlaybackVolume(volumeRef.current * ratio, prepared.response.playback.media.audioNormalization, preferences.normalizationMode);
          if (ratio >= 1) break;
          await delay(50, controller.signal).catch(() => undefined);
        }
      }
      if (disposed) return;
      if (preloadStarted) {
        media.pause();
      }
      if (preloadStarted) {
        continuationRef.current = {
          targetMediaId: prepared.candidate.id,
          audio: preload,
        };
      }
      let next: PlaybackResponse | undefined;
      try {
        console.info('[portico-music-transition]', JSON.stringify({ phase: 'handoff', sourceSessionId: playback.sessionId, preparedSessionId: prepared.response.preparedSessionId }));
        next = await handoffRef.current({
          preparedSessionId: prepared.response.preparedSessionId,
          progressSeconds: Number.isFinite(media.currentTime) ? media.currentTime : undefined,
        });
      } catch (reason) {
        console.warn('[portico-music-transition]', JSON.stringify({ phase: 'handoff-rejected', sourceSessionId: playback.sessionId, preparedSessionId: prepared.response.preparedSessionId, failure: reason instanceof Error ? reason.name : 'unknown' }));
        next = undefined;
      }
      if (!next) {
        if (continuationRef.current?.audio === preload) {
          stopPreloadedAudio(preload);
          continuationRef.current = undefined;
        }
        transitioningRef.current = false;
        onTransitioningRef.current(false);
        return;
      }
    };

    const tick = () => {
      // A deliberate seek near the end must not be mistaken for the natural
      // end-of-track handoff while the browser is still resolving the seek.
      if (disposed || media.paused || media.seeking || !Number.isFinite(media.duration) || media.duration <= 0) return;
      const remaining = Math.max(0, media.duration - media.currentTime);
      const prepareLead = Math.max(8, preferences.crossfadeSeconds + 3);
      if (remaining <= prepareLead) void prepare();
      // Browsers can advance from their last sparse timeupdate directly to a
      // paused ended state. Give a zero-crossfade handoff a full second to
      // commit while the source is still playing.
      const transitionLead = preferences.crossfadeSeconds > 0 ? preferences.crossfadeSeconds : 1;
      if (remaining <= transitionLead) void transition();
    };
    const timer = window.setInterval(tick, 80);
    media.addEventListener('timeupdate', tick);
    tick();
    return () => {
      disposed = true;
      controller.abort();
      window.clearInterval(timer);
      media.removeEventListener('timeupdate', tick);
      const continuationOwnsPreload = continuationRef.current?.audio === preload;
      if (!continuationOwnsPreload) stopPreloadedAudio(preload);
      preparedRef.current = undefined;
      preparingRef.current = false;
      transitioningRef.current = continuationOwnsPreload;
      if (!continuationOwnsPreload) onTransitioningRef.current(false);
    };
    // Playback and queue responses may be re-projected while progress is being
    // recorded.  Those equivalent objects must not tear down the sole prepare
    // owner and issue another POST. The owner key contains every input that can
    // change the prepared continuation; the captured values are immutable for
    // that identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, mediaRef, transitionOwnerKey]);

  return <audio ref={preloadRef} className="player-preload-audio" aria-hidden="true" />;
}

export function nextCandidate(playback: PlaybackResponse, queue: PlaybackSessionQueueResponse | undefined): MediaItem | undefined {
  return (queue?.items ?? playback.queue).find((item) => item.id !== playback.media.id);
}

export function musicTransitionOwnerKey(
  playback: PlaybackResponse,
  queue: PlaybackSessionQueueResponse | undefined,
  preferences: MusicPlaybackPreferences,
  webPreferences: WebDisplayPreferences,
) {
  const queueItems = queue?.items ?? playback.queue;
  const candidate = nextCandidate(playback, queue);
  const candidateIndex = candidate ? queueItems.findIndex((item) => item.id === candidate.id) : -1;
  const continuationMediaIds = candidateIndex >= 0
    ? queueItems.slice(candidateIndex + 1).map((item) => item.id)
    : queueItems.filter((item) => item.id !== playback.media.id && item.id !== candidate?.id).map((item) => item.id);
  const intent = webPlaybackIntent(webPreferences);
  return JSON.stringify({
    sessionId: playback.sessionId,
    candidateId: candidate?.id ?? '',
    continuationMediaIds,
    sourceContext: queue?.sourceContext ?? playback.sourceContext,
    gapless: preferences.gapless,
    crossfadeSeconds: preferences.crossfadeSeconds,
    normalizationMode: preferences.normalizationMode,
    intent,
  });
}

export function musicTransitionRequest(
  playback: PlaybackResponse,
  queue: PlaybackSessionQueueResponse | undefined,
  preferences: MusicPlaybackPreferences,
  webPreferences: WebDisplayPreferences,
): PlaybackPrepareNextRequest {
  const candidate = nextCandidate(playback, queue);
  const queueItems = queue?.items ?? playback.queue;
  const candidateIndex = candidate ? queueItems.findIndex((item) => item.id === candidate.id) : -1;
  return {
    mediaId: candidate?.id,
    queueMediaIds: (candidateIndex >= 0 ? queueItems.slice(candidateIndex + 1) : queueItems.filter((item) => item.id !== playback.media.id && item.id !== candidate?.id)).map((item) => item.id),
    crossfadeSeconds: preferences.crossfadeSeconds,
    preferredHandoff: preferences.crossfadeSeconds > 0 ? 'crossfade' : 'gapless',
    sourceContext: queue?.sourceContext ?? playback.sourceContext,
    commitPreviousEnd: true,
    intent: webPlaybackIntent(webPreferences),
  };
}

export function continuePreloadedAudioUntilPrimaryPlays({
  primary,
  preload,
  volume,
  muted,
  normalization,
  normalizationMode,
  onSettled,
}: {
  primary: HTMLMediaElement;
  preload: HTMLAudioElement;
  volume: number;
  muted: boolean;
  normalization: PlaybackResponse['media']['audioNormalization'];
  normalizationMode: MusicPlaybackPreferences['normalizationMode'];
  onSettled: () => void;
}) {
  let started = false;
  let settled = false;
  const startPrimary = () => {
    if (started || settled) return;
    started = true;
    const bridgedSeconds = Number.isFinite(preload.currentTime) ? preload.currentTime : 0;
    primary.currentTime = Math.min(bridgedSeconds, Number.isFinite(primary.duration) ? primary.duration : bridgedSeconds);
    primary.muted = muted;
    primary.volume = effectivePlaybackVolume(volume, normalization, normalizationMode);
    void primary.play().catch(() => undefined);
  };
  const settle = () => {
    if (settled) return;
    settled = true;
    window.clearTimeout(timeout);
    primary.removeEventListener('loadedmetadata', startPrimary);
    primary.removeEventListener('playing', settle);
    primary.removeEventListener('error', settle);
    stopPreloadedAudio(preload);
    onSettled();
  };
  const timeout = window.setTimeout(settle, 10_000);
  primary.addEventListener('playing', settle, { once: true });
  primary.addEventListener('error', settle, { once: true });
  if (primary.readyState >= HTMLMediaElement.HAVE_METADATA) startPrimary();
  else primary.addEventListener('loadedmetadata', startPrimary, { once: true });
  return settle;
}

function stopPreloadedAudio(preload: HTMLAudioElement) {
  preload.pause();
  preload.removeAttribute('src');
  preload.load();
}

function delay(milliseconds: number, signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('Aborted', 'AbortError'));
      return;
    }
    const onAbort = () => {
      window.clearTimeout(timer);
      reject(new DOMException('Aborted', 'AbortError'));
    };
    const timer = window.setTimeout(() => {
      signal.removeEventListener('abort', onAbort);
      resolve();
    }, milliseconds);
    signal.addEventListener('abort', onAbort, { once: true });
  });
}
