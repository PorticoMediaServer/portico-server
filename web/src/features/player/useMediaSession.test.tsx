/** @vitest-environment jsdom */
import type { MediaItem } from '@porticomediaserver/client-core';
import { render } from '@testing-library/react';
import { createRef } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mediaSessionMetadata, useMediaSession } from './useMediaSession';

const item = {
  id: 'track-1',
  title: 'Happiest I’ve Ever Been',
  type: 'track',
  images: { poster: '/album.jpg' },
  typedMetadata: { artist: 'Big Girl', albumTitle: 'A New Set of Songs' },
} as unknown as MediaItem;

function Harness({ media }: { media: HTMLVideoElement }) {
  const ref = createRef<HTMLVideoElement>();
  ref.current = media;
  useMediaSession(item, ref, {
    play: vi.fn(), pause: vi.fn(), stop: vi.fn(), seekTo: vi.fn(), seekBy: vi.fn(), previous: vi.fn(), next: vi.fn(),
    skipBackSeconds: 10, skipForwardSeconds: 30,
  });
  return null;
}

describe('Media Session integration', () => {
  const handlers = new Map<string, MediaSessionActionHandler | null>();
  const mediaSession = {
    metadata: null,
    playbackState: 'none',
    setActionHandler: vi.fn((action: string, handler: MediaSessionActionHandler | null) => handlers.set(action, handler)),
    setPositionState: vi.fn(),
  };

  beforeEach(() => {
    handlers.clear();
    mediaSession.metadata = null;
    mediaSession.setActionHandler.mockClear();
    mediaSession.setPositionState.mockClear();
    Object.defineProperty(navigator, 'mediaSession', { configurable: true, value: mediaSession });
    Object.defineProperty(globalThis, 'MediaMetadata', { configurable: true, value: class MediaMetadataMock { constructor(value: unknown) { Object.assign(this, value); } } });
  });

  afterEach(() => vi.restoreAllMocks());

  it('builds useful track metadata', () => {
    expect(mediaSessionMetadata(item)).toMatchObject({ title: item.title, artist: 'Big Girl', album: 'A New Set of Songs' });
    expect(mediaSessionMetadata(item).artwork[0]?.src).toContain('/album.jpg');
  });

  it('resolves server-relative artwork against the selected server and preserves absolute artwork', () => {
    const resolve = vi.fn((path: string) => `https://server.direct.getportico.tv${path}`);
    expect(mediaSessionMetadata(item, resolve).artwork[0]?.src).toBe('https://server.direct.getportico.tv/album.jpg');
    expect(resolve).toHaveBeenCalledWith('/album.jpg');

    const absolute = { ...item, images: { poster: 'https://images.example/album.jpg' } } as MediaItem;
    expect(mediaSessionMetadata(absolute, resolve).artwork[0]?.src).toBe('https://images.example/album.jpg');
    expect(resolve).toHaveBeenCalledOnce();
  });

  it('registers and cleans up supported hardware actions', () => {
    const media = document.createElement('video');
    const view = render(<Harness media={media} />);
    expect(handlers.get('play')).toEqual(expect.any(Function));
    expect(handlers.get('seekto')).toEqual(expect.any(Function));
    expect(handlers.get('nexttrack')).toEqual(expect.any(Function));
    view.unmount();
    expect(handlers.get('play')).toBeNull();
    expect(handlers.get('nexttrack')).toBeNull();
  });
});
