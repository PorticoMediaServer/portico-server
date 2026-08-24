import type { PlaybackResponse } from '@porticomediaserver/client-core';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { HttpPorticoDataSource } from '../../data/httpSource';

function jsonResponse(payload: unknown) {
  const encoded = JSON.stringify(payload);
  return { ok: true, status: 200, statusText: 'OK', headers: new Headers({ 'Content-Length': String(new TextEncoder().encode(encoded).byteLength) }), json: async () => payload, text: async () => encoded } as Response;
}

function response(): PlaybackResponse {
  return {
    sessionId: 'session-1', nextEventSequence: 4,
    mediaGrant: { token: 'grant', expiresAt: '2099-01-01T00:00:00Z' },
    continuationCredential: { token: 'continuation', expiresAt: '2099-01-01T00:00:00Z', generation: 1, origin: 'http://localhost:32500' },
    media: { id: 'movie-1', type: 'movie', title: 'Arrival', sortTitle: 'Arrival', addedAt: '2026-01-01T00:00:00Z', genres: [], tags: [], labels: [], images: { poster: '', backdrop: '', thumb: '' }, state: { watchlisted: false, favorite: false, watched: false, progressSeconds: 0, rating: 0 }, actions: ['play'] },
    sourceUrl: '/api/media/movie-1/stream.mp4', directPlay: true, streamFormat: 'direct',
    resources: [{ id: 'movie-1-direct', sourceUrl: '/api/media/movie-1/stream.mp4', streamFormat: 'direct', default: true }],
    decision: { mode: 'direct_play', reason: 'compatible', reasonCodes: ['exact_tuple'], requiresTranscode: false, isProxied: true, isServerCached: false },
    policy: { networkClass: 'local', qualityProfile: 'original', directPlayPolicy: 'prefer', directStreamPolicy: 'allow', transcodePolicy: 'allow', allowHdr: true, deliveryProfile: 'video-original', serverClamps: [] },
    qualities: [], audioStreams: [], subtitleStreams: [], chapters: [], queue: [], repeatMode: 'off', queueRevision: 0, playbackRevision: 1, timeline: { type: 'vod', durationSeconds: 60 }, generation: 1,
  } as unknown as PlaybackResponse;
}

afterEach(() => vi.unstubAllGlobals());

describe('playback HTTP adapter', () => {
  it('starts playback with the browser profile and sends ordered session progress', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(response())).mockResolvedValueOnce(jsonResponse({ accepted: true, duplicate: false, stale: false, highestEventSequence: 4, sessionState: 'playing' }));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();

    await source.startPlayback('movie-1', { startSeconds: 8, repeatMode: 'all' }, new AbortController().signal);
    await source.touchPlayback('session-1', { state: 'playing', progressSeconds: 9 });

    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/playback-sessions');
    const startBody = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(startBody).toEqual(expect.objectContaining({ mediaId: 'movie-1', startSeconds: 8, repeatMode: 'all', clientInstanceId: expect.stringMatching(/^web-/), clientProfile: expect.any(Object) }));
    const progressBody = JSON.parse(fetchMock.mock.calls[1][1].body as string);
    expect(progressBody).toEqual(expect.objectContaining({ state: 'playing', progressSeconds: 9, eventSequence: 4, recordedAt: expect.any(String) }));
  });
});
