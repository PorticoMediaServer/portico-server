import {
  defaultPlaybackQuality,
  effectivePlaybackVolume,
  playbackResourceUrl,
  playbackSourceFor,
  type MediaItem,
  type PlaybackHandoffRequest,
  type PlaybackPreparedResponse,
  type PlaybackResponse,
  type PlaybackSessionQueueResponse,
} from '@portico/client-core';
import { type RefObject, useEffect, useRef } from 'react';
import { usePorticoDataSource } from '../../data/DataProvider';
import type { MusicPlaybackPreferences } from '../../data/models';
import { useOptionalWebDisplayPreferences } from '../../preferences/WebDisplayPreferencesProvider';
import { defaultWebDisplayPreferences, webPlaybackIntent } from '../../preferences/webDisplayPreferences';

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
}: MusicTransitionBridgeProps) {
  const source = usePorticoDataSource();
  const webPreferences = useOptionalWebDisplayPreferences()?.preferences ?? defaultWebDisplayPreferences;
  const preloadRef = useRef<HTMLAudioElement>(null);
  const preparedRef = useRef<PreparedTrack | undefined>(undefined);
  const preparingRef = useRef(false);
  const transitioningRef = useRef(false);
  const continuationRef = useRef<ActiveContinuation | undefined>(undefined);

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
    const candidate = nextCandidate(playback, queue);
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
        volume,
        muted,
        normalization: playback.media.audioNormalization,
        normalizationMode: preferences.normalizationMode,
        onSettled: () => {
          if (continuationRef.current === activeContinuation) continuationRef.current = undefined;
          transitioningRef.current = false;
          onTransitioning(false);
        },
      });
    }
    preparedRef.current = undefined;
    preparingRef.current = false;
    transitioningRef.current = Boolean(continuationRef.current);
    onTransitioning(Boolean(continuationRef.current));

    const prepare = async () => {
      if (preparingRef.current || preparedRef.current || continuationRef.current || disposed) return;
      preparingRef.current = true;
      try {
        const queueItems = queue?.items ?? playback.queue;
        const index = queueItems.findIndex((item) => item.id === candidate.id);
        const remainingIDs = (index >= 0 ? queueItems.slice(index + 1) : queueItems.filter((item) => item.id !== candidate.id)).map((item) => item.id);
        const response = await source.prepareNextPlayback(playback.sessionId, controller.signal, {
          mediaId: candidate.id,
          queueMediaIds: remainingIDs,
          crossfadeSeconds: preferences.crossfadeSeconds,
          preferredHandoff: preferences.crossfadeSeconds > 0 ? 'crossfade' : 'gapless',
          sourceContext: queue?.sourceContext ?? playback.sourceContext,
          commitPreviousEnd: true,
          intent: webPlaybackIntent(webPreferences),
        });
        if (disposed || controller.signal.aborted) return;
        const resolve = (path: string) => playbackResourceUrl(response.playback, path, (value) => source.playbackResourceUrl(value), window.location.href);
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
      onTransitioning(true);
      const fadeSeconds = prepared.canOverlap ? preferences.crossfadeSeconds : 0;
      let preloadStarted = false;
      if (prepared.canOverlap && preload.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
        preload.muted = muted;
        preload.volume = fadeSeconds > 0 ? 0 : effectivePlaybackVolume(volume, prepared.response.playback.media.audioNormalization, preferences.normalizationMode);
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
          media.volume = effectivePlaybackVolume(volume * (1 - ratio), playback.media.audioNormalization, preferences.normalizationMode);
          preload.volume = effectivePlaybackVolume(volume * ratio, prepared.response.playback.media.audioNormalization, preferences.normalizationMode);
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
        next = await handoff({
          preparedSessionId: prepared.response.preparedSessionId,
          progressSeconds: Number.isFinite(media.currentTime) ? media.currentTime : undefined,
        });
      } catch {
        next = undefined;
      }
      if (!next) {
        if (continuationRef.current?.audio === preload) {
          stopPreloadedAudio(preload);
          continuationRef.current = undefined;
        }
        transitioningRef.current = false;
        onTransitioning(false);
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
      const transitionLead = preferences.crossfadeSeconds > 0 ? preferences.crossfadeSeconds : 0.18;
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
      if (!continuationOwnsPreload) onTransitioning(false);
    };
  }, [enabled, handoff, mediaRef, muted, onTransitioning, playback, preferences, queue, source, volume, webPreferences]);

  return <audio ref={preloadRef} className="player-preload-audio" aria-hidden="true" />;
}

export function nextCandidate(playback: PlaybackResponse, queue: PlaybackSessionQueueResponse | undefined): MediaItem | undefined {
  return (queue?.items ?? playback.queue).find((item) => item.id !== playback.media.id);
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
