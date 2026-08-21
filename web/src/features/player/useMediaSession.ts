import type { MediaItem } from '@portico/client-core';
import { type RefObject, useEffect, useRef } from 'react';
import { musicAlbum, musicArtist } from './musicPlayback';

type MediaSessionControls = {
  play: () => void;
  pause: () => void;
  stop: () => void;
  seekTo: (seconds: number) => void;
  seekBy: (seconds: number) => void;
  previous: () => void;
  next: () => void;
  skipBackSeconds: number;
  skipForwardSeconds: number;
};

export function mediaSessionMetadata(item: MediaItem, resolveResource?: (path: string) => string) {
  const image = item.images.poster || item.images.thumb || item.displayImages?.backdrop || item.images.backdrop;
  return {
    title: item.title,
    artist: musicArtist(item) || item.parentTitle || item.grandparentTitle || '',
    album: musicAlbum(item),
    artwork: image ? [{ src: mediaSessionArtworkURL(image, resolveResource) }] : [],
  };
}

export function useMediaSession(
  item: MediaItem | undefined,
  mediaRef: RefObject<HTMLVideoElement | null>,
  controls: MediaSessionControls,
  resolveResource?: (path: string) => string,
) {
  const controlsRef = useRef(controls);
  controlsRef.current = controls;

  useEffect(() => {
    if (!item || !('mediaSession' in navigator)) return;
    const session = navigator.mediaSession;
    if (typeof globalThis.MediaMetadata === 'function') {
      const metadata = new MediaMetadata(mediaSessionMetadata(item, resolveResource));
      session.metadata = metadata;
      return () => {
        if (session.metadata === metadata) session.metadata = null;
      };
    }
  }, [item, resolveResource]);

  useEffect(() => {
    if (!item || !('mediaSession' in navigator)) return;
    const session = navigator.mediaSession;
    const handlers: Partial<Record<MediaSessionAction, MediaSessionActionHandler>> = {
      play: () => controlsRef.current.play(),
      pause: () => controlsRef.current.pause(),
      stop: () => controlsRef.current.stop(),
      seekto: (details) => {
        if (typeof details.seekTime === 'number') controlsRef.current.seekTo(details.seekTime);
      },
      seekbackward: (details) => controlsRef.current.seekBy(-(details.seekOffset || controlsRef.current.skipBackSeconds)),
      seekforward: (details) => controlsRef.current.seekBy(details.seekOffset || controlsRef.current.skipForwardSeconds),
      previoustrack: () => controlsRef.current.previous(),
      nexttrack: () => controlsRef.current.next(),
    };
    for (const [action, handler] of Object.entries(handlers)) setActionHandler(session, action as MediaSessionAction, handler ?? null);
    return () => {
      for (const action of Object.keys(handlers)) setActionHandler(session, action as MediaSessionAction, null);
    };
  }, [item?.id]);

  useEffect(() => {
    const media = mediaRef.current;
    if (!item || !media || !('mediaSession' in navigator)) return;
    const session = navigator.mediaSession;
    let lastPositionUpdate = 0;
    const updatePlaybackState = () => {
      session.playbackState = media.paused ? 'paused' : 'playing';
    };
    const updatePosition = (force = false) => {
      if (typeof session.setPositionState !== 'function') return;
      const now = Date.now();
      if (!force && now - lastPositionUpdate < 750) return;
      const duration = media.duration;
      if (!Number.isFinite(duration) || duration <= 0) return;
      const position = Math.min(duration, Math.max(0, media.currentTime || 0));
      const playbackRate = Number.isFinite(media.playbackRate) && media.playbackRate > 0 ? media.playbackRate : 1;
      try {
        session.setPositionState({ duration, position, playbackRate });
        lastPositionUpdate = now;
      } catch {
        // Safari and older Chromium builds may expose a partial Media Session implementation.
      }
    };
    const onTimeUpdate = () => updatePosition();
    const onDurationChange = () => updatePosition(true);
    media.addEventListener('play', updatePlaybackState);
    media.addEventListener('pause', updatePlaybackState);
    media.addEventListener('playing', updatePlaybackState);
    media.addEventListener('timeupdate', onTimeUpdate);
    media.addEventListener('durationchange', onDurationChange);
    updatePlaybackState();
    updatePosition(true);
    return () => {
      media.removeEventListener('play', updatePlaybackState);
      media.removeEventListener('pause', updatePlaybackState);
      media.removeEventListener('playing', updatePlaybackState);
      media.removeEventListener('timeupdate', onTimeUpdate);
      media.removeEventListener('durationchange', onDurationChange);
    };
  }, [item?.id, mediaRef]);
}

function setActionHandler(session: MediaSession, action: MediaSessionAction, handler: MediaSessionActionHandler | null) {
  try {
    session.setActionHandler(action, handler);
  } catch {
    // A browser may expose Media Session while omitting individual actions.
  }
}

function mediaSessionArtworkURL(value: string, resolveResource?: (path: string) => string): string {
  if (/^(?:https?:|data:|blob:)/i.test(value)) return value;
  const resolved = resolveResource?.(value) ?? value;
  try {
    return new URL(resolved, globalThis.location?.href ?? 'http://portico.local').toString();
  } catch {
    return resolved;
  }
}
