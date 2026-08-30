import type { PlaybackPreparedResponse, PlaybackResponse } from '@porticomediaserver/client-core';
import { describe, expect, it } from 'vitest';
import { createPlaybackSessionAdapter } from './playbackAdapter';

function playback(nextEventSequence: number, sessionId = 'session-1'): PlaybackResponse {
  return {
    sessionId,
    nextEventSequence,
    media: { id: 'movie-1', type: 'movie' },
    generation: 1,
    queueRevision: 1,
    repeatMode: 'off',
    timeline: { type: 'vod', canPause: true, canSeek: true },
    qualities: [],
    audioStreams: [],
    subtitleStreams: [],
    chapters: [],
    queue: [],
    resources: [],
  } as unknown as PlaybackResponse;
}

describe('playback session adapter', () => {
  it('keeps normalization, session seeding, and progress ordering in one boundary', () => {
    const adapter = createPlaybackSessionAdapter();

    const accepted = adapter.acceptPlayback(playback(4));
    expect(accepted.nextEventSequence).toBe(4);
    expect(adapter.orderProgress('session-1', { state: 'playing', progressSeconds: 9 }).eventSequence).toBe(4);
    expect(adapter.orderProgress('session-1', { state: 'playing', progressSeconds: 10 }).eventSequence).toBe(5);

    const prepared = adapter.acceptPreparedPlayback({ playback: playback(9) } as PlaybackPreparedResponse);
    expect(prepared.playback.nextEventSequence).toBe(9);
    expect(adapter.orderProgress('session-1', { state: 'playing', progressSeconds: 11 }).eventSequence).toBe(9);
  });

  it('normalizes renegotiation without reseeding progress, then releases a session cleanly', () => {
    const adapter = createPlaybackSessionAdapter();
    adapter.acceptPlayback(playback(4));

    expect(adapter.normalizePlayback(playback(99)).nextEventSequence).toBe(99);
    expect(adapter.orderProgress('session-1', { state: 'paused', progressSeconds: 12 }).eventSequence).toBe(4);

    adapter.releaseSession('session-1');
    expect(adapter.orderProgress('session-1', { state: 'paused', progressSeconds: 12 }).eventSequence).toBe(1);
  });
});
