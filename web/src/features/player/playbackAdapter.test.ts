import type { PlaybackResponse, PorticoClient } from '@porticomediaserver/client-core';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { HttpPorticoDataSource } from '../../data/httpSource';

function jsonResponse(payload: unknown, ok = true, status = 200) {
  const encoded = JSON.stringify(payload);
  return { ok, status, statusText: ok ? 'OK' : 'Not Found', headers: new Headers({ 'Content-Length': String(new TextEncoder().encode(encoded).byteLength) }), json: async () => payload, text: async () => encoded } as Response;
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
    policy: { networkClass: 'local', directPlayPolicy: 'prefer', directStreamPolicy: 'allow', transcodePolicy: 'allow', allowHdr: true, serverClamps: [] },
    qualityOffers: { contractId: 'PC-PLAYBACK', schemaVersion: 'quality-offers.v1', mediaId: 'movie-1', versionId: 'qver-movie-1', sourceRevision: 'qsrc-movie-1', offerRevision: 'qrev-movie-1', offers: [{ selectionId: 'qsel-automatic', label: 'Automatic', kind: 'automatic' }, { selectionId: 'qsel-original', label: 'Original Quality', kind: 'original' }] },
    qualitySelection: { mode: 'automatic' }, audioStreams: [], subtitleStreams: [], chapters: [], queue: [], repeatMode: 'off', queueRevision: 0, playbackRevision: 1, timeline: { type: 'vod', durationSeconds: 60 }, generation: 1,
  } as unknown as PlaybackResponse;
}

afterEach(() => {
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe('playback HTTP adapter', () => {
  it('keeps committed replacement identity while startup recovery retries an ambiguous restore', async () => {
    const committed = { outcome: 'committed-restore-required' as const, sourceSessionId: 'session-source', replacementSessionId: 'session-committed' };
    const client = {
      pendingPlaybackTerminalMutations: vi.fn().mockResolvedValue([{ operation: 'replacement', sessionId: 'session-source', request: {}, target: {} }]),
      retryPendingPlaybackTerminalMutation: vi.fn().mockResolvedValue(committed),
      restoreCommittedPlaybackReplacement: vi.fn()
        .mockRejectedValueOnce(new TypeError('active restore response lost'))
        .mockResolvedValueOnce({ ...response(), sessionId: 'session-committed' }),
    } as unknown as PorticoClient;
    const source = new HttpPorticoDataSource(client);

    await expect(source.recoverPendingPlaybackTerminals(new AbortController().signal)).resolves.toBeUndefined();

    expect(client.retryPendingPlaybackTerminalMutation).toHaveBeenCalledOnce();
    expect(client.restoreCommittedPlaybackReplacement).toHaveBeenCalledTimes(2);
    expect(client.restoreCommittedPlaybackReplacement).toHaveBeenNthCalledWith(1, committed, undefined, expect.any(Object));
    expect(client.restoreCommittedPlaybackReplacement).toHaveBeenNthCalledWith(2, committed, undefined, expect.any(Object));
  });

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

  it('delegates active route replacement to Core with one durable ordered terminal envelope', async () => {
    const replacement = { ...response(), sessionId: 'session-2', media: { ...response().media, id: 'movie-2', title: 'Moon' } };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(response()))
      .mockResolvedValueOnce(jsonResponse(replacement));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    const signal = new AbortController().signal;
    await source.startPlayback('movie-1', {}, signal);

    const outcome = await source.replacePlaybackTarget({ kind: 'media', mediaId: 'movie-2' }, {
      sourceSessionId: 'session-1',
      previousTerminal: { disposition: 'stopped', positionSeconds: 14, durationSeconds: 60 },
      expectedQueueRevision: 0,
      expectedPlaybackRevision: 1,
    }, signal);

    expect(outcome).toMatchObject({ outcome: 'accepted', value: { sessionId: 'session-2' } });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    const body = JSON.parse(fetchMock.mock.calls[1][1].body as string);
    expect(body).toEqual(expect.objectContaining({
      mediaId: 'movie-2',
      replacement: expect.objectContaining({
        sourceSessionId: 'session-1',
        requestId: expect.any(String),
        expectedQueueRevision: 0,
        expectedPlaybackRevision: 1,
        previousTerminal: expect.objectContaining({
          disposition: 'stopped',
          positionSeconds: 14,
          durationSeconds: 60,
          generation: 1,
          eventSequence: 4,
          recordedAt: expect.any(String),
        }),
      }),
    }));
    expect(fetchMock.mock.calls.some(([, init]) => init.method === 'DELETE')).toBe(false);
  });

  it('delegates prepare and atomic handoff to Core without legacy terminal fields', async () => {
    const prepared = {
      preparedSessionId: 'prepared-2',
      playback: { ...response(), continuationCredential: null, sessionId: 'prepared-session', currentQueueEntryId: 'entry-2' },
      expiresAt: '2099-01-01T00:00:00Z',
      preloadPolicy: 'metadata',
      handoffMode: 'replace',
      queue: [],
      queueRevision: 1,
      playbackRevision: 1,
    };
    const replacement = { ...response(), sessionId: 'session-2', currentQueueEntryId: 'entry-2' };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(response()))
      .mockResolvedValueOnce(jsonResponse(prepared))
      .mockResolvedValueOnce(jsonResponse(replacement));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    const signal = new AbortController().signal;

    await source.startPlayback('movie-1', {}, signal);
    await source.prepareNextPlayback('session-1', signal, { entryId: 'entry-2' });
    await source.handoffPlayback('session-1', {
      requestId: 'web-handoff-1',
      preparedSessionId: 'prepared-2',
      entryId: 'entry-2',
      previousTerminal: { disposition: 'completed', positionSeconds: 60, durationSeconds: 60 },
    }, signal);

    const prepareBody = JSON.parse(fetchMock.mock.calls[1][1].body as string);
    expect(prepareBody).toEqual(expect.objectContaining({ entryId: 'entry-2', clientProfile: expect.any(Object) }));
    expect(prepareBody).not.toHaveProperty('commitPreviousEnd');
    const handoffBody = JSON.parse(fetchMock.mock.calls[2][1].body as string);
    expect(handoffBody).toEqual(expect.objectContaining({
      requestId: 'web-handoff-1',
      preparedSessionId: 'prepared-2',
      entryId: 'entry-2',
      previousTerminal: expect.objectContaining({
        disposition: 'completed',
        generation: 1,
        eventSequence: 4,
        positionSeconds: 60,
        durationSeconds: 60,
      }),
    }));
    expect(handoffBody).not.toHaveProperty('progressSeconds');
  });

  it('reloads and exact-replays a durable handoff after lost responses', async () => {
    const replacement = { ...response(), sessionId: 'session-recovered', currentQueueEntryId: 'entry-2' };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(response()))
      .mockRejectedValueOnce(new TypeError('first response lost'))
      .mockRejectedValueOnce(new TypeError('recovery response lost'))
      .mockResolvedValueOnce(jsonResponse(replacement));
    vi.stubGlobal('fetch', fetchMock);
    const signal = new AbortController().signal;
    const source = new HttpPorticoDataSource();
    await source.startPlayback('movie-1', {}, signal);
    const input = {
      requestId: 'web-restart-handoff-1',
      entryId: 'entry-2',
      previousTerminal: { disposition: 'completed' as const, positionSeconds: 60, durationSeconds: 60 },
    };
    await expect(source.handoffPlayback('session-1', input, signal)).rejects.toThrow('first response lost');

    const restoredSource = new HttpPorticoDataSource();
    await expect(restoredSource.recoverPendingPlaybackTerminals(signal)).resolves.toBeUndefined();

    const firstBody = fetchMock.mock.calls[1][1].body as string;
    expect(fetchMock.mock.calls[2][1].body).toBe(firstBody);
    expect(fetchMock.mock.calls[3][1].body).toBe(firstBody);
  });

  it('reloads and exact-replays a durable stop after lost responses', async () => {
    const stopBodies: string[] = [];
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method !== 'DELETE') return jsonResponse(response());
      const body = String(init.body);
      stopBodies.push(body);
      if (stopBodies.length < 3) throw new TypeError('terminal response lost');
      const request = JSON.parse(body) as { requestId: string; terminal: Record<string, unknown> };
      return jsonResponse({
        requestId: request.requestId,
        accepted: true,
        duplicate: false,
        sessionId: 'session-1',
        terminal: request.terminal,
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    const signal = new AbortController().signal;
    const source = new HttpPorticoDataSource();
    await source.startPlayback('movie-1', {}, signal);
    await expect(source.stopPlayback('session-1', {
      disposition: 'completed',
      positionSeconds: 60,
      durationSeconds: 60,
    }, signal)).rejects.toThrow('terminal response lost');

    const restoredSource = new HttpPorticoDataSource();
    await expect(restoredSource.recoverPendingPlaybackTerminals(signal)).resolves.toBeUndefined();

    expect(stopBodies).toHaveLength(3);
    expect(stopBodies[1]).toBe(stopBodies[0]);
    expect(stopBodies[2]).toBe(stopBodies[0]);
  });

  it('retains and exact-replays a terminal request after a 404 response', async () => {
    const stopBodies: string[] = [];
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method !== 'DELETE') return jsonResponse(response());
      const body = String(init.body);
      stopBodies.push(body);
      if (stopBodies.length === 1) {
        return jsonResponse({ code: 'session_not_found', detail: 'Outcome is not known.' }, false, 404);
      }
      const request = JSON.parse(body) as { requestId: string; terminal: Record<string, unknown> };
      return jsonResponse({
        requestId: request.requestId,
        accepted: true,
        duplicate: true,
        sessionId: 'session-1',
        terminal: request.terminal,
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    const signal = new AbortController().signal;
    const source = new HttpPorticoDataSource();
    await source.startPlayback('movie-1', {}, signal);
    await expect(source.stopPlayback('session-1', {
      disposition: 'completed',
      positionSeconds: 60,
      durationSeconds: 60,
    }, signal)).rejects.toMatchObject({ status: 404 });

    const restoredSource = new HttpPorticoDataSource();
    await expect(restoredSource.recoverPendingPlaybackTerminals(signal)).resolves.toBeUndefined();
    expect(stopBodies).toHaveLength(2);
    expect(stopBodies[1]).toBe(stopBodies[0]);
  });
});
