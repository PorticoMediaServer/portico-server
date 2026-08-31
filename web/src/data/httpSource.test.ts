import { afterEach, describe, expect, it, vi } from 'vitest';
import type { HostedServicesClient, PorticoClient } from '@porticomediaserver/client-core';
import { HttpPorticoDataSource, LocalProfileSelectionRequiredError, imagePath } from './httpSource';
import { createBrowserHostedConnectionVault } from '../runtime/hostedConnectionVault';
import clientCompatibilityFixture from '../../../api/openapi/fixtures/client-compatibility-conformance.json';

function jsonResponse(payload: unknown, ok = true, status = 200) {
  const body = payload && typeof payload === 'object' && 'authenticated' in payload && payload.authenticated === true && !('authority' in payload)
    ? { ...payload, authority: 'accountMode' in payload && payload.accountMode === 'portico' ? 'hosted' : 'local' }
    : payload;
  const encoded = JSON.stringify(body);
  return { ok, status, statusText: ok ? 'OK' : 'Service Unavailable', headers: new Headers({ 'Content-Length': String(new TextEncoder().encode(encoded).byteLength) }), json: async () => body, text: async () => encoded } as Response;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function productContract(apiVersion: string) {
  return {
    apiVersion,
    libraryKinds: [],
    entityKinds: [],
    browseFields: [],
    browseSorts: [],
    browseOperators: [],
    presentationFields: [],
    queryLimits: {},
    serverCapabilities: [],
    entitySemantics: [],
    search: {
      revision: 'search-v1',
      groups: [{ id: 'shows', title: 'Shows', entityKinds: ['show'], defaultLimit: 8, maximumLimit: 50 }],
      groupOrder: ['shows'],
      sorts: [{ id: 'relevance', label: 'Relevance', directions: ['desc'], defaultDirection: 'desc', applicableGroups: ['shows'] }],
      filters: [{ id: 'entityKinds', label: 'Result type', allowedValues: ['show'] }],
      limits: { minimumQueryLength: 1, maximumQueryLength: 200, quickInitialGroupLimit: 6, fullDefaultGroupLimit: 24, maximumGroupLimit: 50 },
      resultSemantics: { kindMappings: [] },
    },
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('HttpPorticoDataSource', () => {
	it('does not infer remote ownership from a stale local connection flag', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({
			setupRequired: false,
			remoteAccess: { porticoConnected: true, settings: { claimStatus: 'unclaimed' } },
		})));

		await expect(new HttpPorticoDataSource().porticoSetupStatus(new AbortController().signal)).resolves.toMatchObject({
			remoteAccess: { porticoConnected: false, settings: { claimStatus: 'unclaimed' } },
		});
	});

  it('roots bundled playback resources at the current Portico server', () => {
    const source = new HttpPorticoDataSource();
    const resource = new URL(source.playbackResourceUrl('/api/media/episode-1/hls/master.m3u8'));

    expect(resource.origin).toBe(window.location.origin);
    expect(resource.pathname).toBe('/api/media/episode-1/hls/master.m3u8');
    expect(resource.hostname).not.toBe('portico.local');
  });

  it('dispatches the production playback DELETE when progress delivery is stalled', async () => {
    const calls: Array<{ url: string; method: string; body?: string; keepalive?: boolean }> = [];
    let releaseProgress!: (response: Response) => void;
    let markProgressStarted!: () => void;
    const stalledProgress = new Promise<Response>((resolve) => { releaseProgress = resolve; });
    const progressStarted = new Promise<void>((resolve) => { markProgressStarted = resolve; });
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = init?.method ?? 'GET';
      calls.push({ url: String(input), method, body: typeof init?.body === 'string' ? init.body : undefined, keepalive: init?.keepalive });
      if (method === 'PATCH') {
        markProgressStarted();
        return stalledProgress;
      }
      if (method === 'DELETE') {
        const request = JSON.parse(String(init?.body)) as {
          requestId: string;
          terminal: Record<string, unknown>;
        };
        return jsonResponse({
          requestId: request.requestId,
          accepted: true,
          duplicate: false,
          sessionId: 'session-stalled',
          terminal: request.terminal,
        });
      }
      return jsonResponse({ ok: true });
    }));
    const source = new HttpPorticoDataSource();
    source.porticoClient().acceptPlaybackSession({
      sessionId: 'session-stalled',
      media: { id: 'm1', title: 'Movie', type: 'movie', state: {} },
      sourceUrl: '/api/media/m1/stream', directPlay: true,
      generation: 1, nextEventSequence: 1, playbackRevision: 1, queueRevision: 1, repeatMode: 'off',
      timeline: { type: 'vod', durationSeconds: 60, canPause: true, canSeek: true },
      continuationCredential: { token: 'continuation', origin: window.location.origin, generation: 1, expiresAt: '2026-08-28T20:00:00Z' },
      mediaGrant: { token: 'grant', expiresAt: '2026-08-28T20:00:00Z' },
      decision: { mode: 'direct_play', reason: '', requiresTranscode: false, isProxied: true, isServerCached: false },
      qualityOffers: { contractId: 'PC-PLAYBACK', schemaVersion: 'quality-offers.v1', mediaId: 'm1', versionId: 'qver-m1', sourceRevision: 'qsrc-m1', offerRevision: 'qrev-m1', offers: [{ selectionId: 'qsel-auto', label: 'Automatic', kind: 'automatic' }, { selectionId: 'qsel-original', label: 'Original Quality', kind: 'original' }] },
      qualitySelection: { mode: 'automatic' },
      audioStreams: [], subtitleStreams: [], chapters: [], queue: [],
      resources: [{ id: 'movie-active', sourceUrl: '/api/media/m1/stream', streamFormat: 'http', default: true }],
    } as unknown as Parameters<PorticoClient['acceptPlaybackSession']>[0]);

    const progress = source.touchPlayback('session-stalled', { positionSeconds: 12 });
    await progressStarted;
    await source.stopPlayback('session-stalled', { disposition: 'completed', positionSeconds: 60, durationSeconds: 60 }, undefined, true);

    expect(calls.map(({ method }) => method)).toEqual(['PATCH', 'DELETE']);
    expect(calls[1]?.url).toContain('/api/playback-sessions/session-stalled');
    const stopRequest = JSON.parse(calls[1]?.body ?? '{}');
    expect(stopRequest).toMatchObject({
      requestId: expect.any(String),
      terminal: {
        disposition: 'completed', generation: 1, eventSequence: 2,
        positionSeconds: 60, durationSeconds: 60,
      },
    });
    expect(stopRequest.terminal.recordedAt).toMatch(/^\d{4}-\d{2}-\d{2}T/);
    expect(calls[1]?.keepalive).toBe(true);
    releaseProgress(jsonResponse({ accepted: true, duplicate: false, stale: false, highestEventSequence: 1 }));
    await expect(progress).rejects.toMatchObject({ code: 'playback_progress_stopped' });
  });

  it('does not infer terminal success from a 404 response', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({
        sessionId: 'session-404',
        media: { id: 'm1', title: 'Movie', type: 'movie', state: {} },
        sourceUrl: '/api/media/m1/stream', directPlay: true,
        generation: 1, nextEventSequence: 1, playbackRevision: 1, queueRevision: 1, repeatMode: 'off',
        timeline: { type: 'vod', durationSeconds: 60, canPause: true, canSeek: true },
        continuationCredential: { token: 'continuation', origin: window.location.origin, generation: 1, expiresAt: '2099-08-28T20:00:00Z' },
        mediaGrant: { token: 'grant', expiresAt: '2099-08-28T20:00:00Z' },
        decision: { mode: 'direct_play', reason: '', requiresTranscode: false, isProxied: true, isServerCached: false },
        qualityOffers: { contractId: 'PC-PLAYBACK', schemaVersion: 'quality-offers.v1', mediaId: 'm1', versionId: 'qver-m1', sourceRevision: 'qsrc-m1', offerRevision: 'qrev-m1', offers: [{ selectionId: 'qsel-auto', label: 'Automatic', kind: 'automatic' }, { selectionId: 'qsel-original', label: 'Original Quality', kind: 'original' }] },
        qualitySelection: { mode: 'automatic' },
        audioStreams: [], subtitleStreams: [], chapters: [], queue: [], resources: [{ id: 'movie-active', sourceUrl: '/api/media/m1/stream', streamFormat: 'http', default: true }],
      }))
      .mockResolvedValueOnce(jsonResponse({ code: 'session_not_found', detail: 'Not found.' }, false, 404));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    await source.startPlayback('m1', {}, new AbortController().signal);

    await expect(source.stopPlayback('session-404', {
      disposition: 'completed', positionSeconds: 60, durationSeconds: 60,
    })).rejects.toMatchObject({ status: 404 });
  });

  it('configures the bundled client with measured browser playback capabilities', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}, false, 503));
    vi.stubGlobal('fetch', fetchMock);
    const canPlayType = vi.spyOn(HTMLMediaElement.prototype, 'canPlayType').mockImplementation((type) => type.includes('mp4') || type.includes('avc1') || type.includes('mp4a') ? 'probably' : '');
    try {
      const source = new HttpPorticoDataSource();
      await expect(source.porticoClient().startPlayback('movie-1')).rejects.toThrow();

      const request = JSON.parse(String(fetchMock.mock.calls[0][1]?.body)) as { clientProfile?: { capabilitySchemaVersion?: string; clientFamily?: string; platform?: string; capabilityEvidence?: Array<{ tuples?: Array<{ mediaKind?: string; audio?: { codec?: string; layout?: string; maxChannels?: number } }> }> } };
      expect(request.clientProfile).toEqual(expect.objectContaining({
        capabilitySchemaVersion: 'playback-capability-v2',
        clientFamily: expect.stringMatching(/^(chromium|edge|safari|firefox)$/),
        platform: 'web',
      }));
      const tuples = request.clientProfile?.capabilityEvidence?.flatMap((evidence) => evidence.tuples ?? []) ?? [];
      expect(tuples).toEqual(expect.arrayContaining([
        expect.objectContaining({ mediaKind: 'audiovisual', audio: expect.objectContaining({ codec: 'aac', layout: 'mono', maxChannels: 1 }) }),
        expect.objectContaining({ mediaKind: 'audiovisual', audio: expect.objectContaining({ codec: 'aac', layout: 'stereo', maxChannels: 2 }) }),
      ]));
    } finally {
      canPlayType.mockRestore();
    }
  });

  it('coalesces Product Contract requests without letting one consumer cancel the shared fetch', async () => {
    const response = deferred<Response>();
    const fetchMock = vi.fn().mockReturnValue(response.promise);
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    const firstController = new AbortController();
    const secondController = new AbortController();

    const first = source.productContract(firstController.signal);
    const second = source.productContract(secondController.signal);
    await Promise.resolve();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][1]?.signal).not.toBe(firstController.signal);
    firstController.abort();
    await expect(first).rejects.toMatchObject({ name: 'AbortError' });

    response.resolve(jsonResponse(productContract('v1')));
    await expect(second).resolves.toMatchObject({ apiVersion: 'v1' });
    await expect(source.productContract(new AbortController().signal)).resolves.toMatchObject({ apiVersion: 'v1' });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('generation-fences an invalidated Product Contract response and starts a fresh request', async () => {
    const staleResponse = deferred<Response>();
    const currentResponse = deferred<Response>();
    const fetchMock = vi.fn()
      .mockReturnValueOnce(staleResponse.promise)
      .mockReturnValueOnce(currentResponse.promise);
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();

    const stale = source.productContract(new AbortController().signal);
    await Promise.resolve();
    const staleAssertion = expect(stale).rejects.toMatchObject({ name: 'AbortError' });
    source.invalidateProductContract();
    const current = source.productContract(new AbortController().signal);
    await Promise.resolve();
    expect(fetchMock).toHaveBeenCalledTimes(2);

    await staleAssertion;
    staleResponse.resolve(jsonResponse(productContract('stale')));
    currentResponse.resolve(jsonResponse(productContract('current')));
    await expect(current).resolves.toMatchObject({ apiVersion: 'current' });
    await expect(source.productContract(new AbortController().signal)).resolves.toMatchObject({ apiVersion: 'current' });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('invalidates the cached Product Contract at a successful sign-out boundary', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(productContract('authenticated')))
      .mockResolvedValueOnce(jsonResponse({ ok: true }))
      .mockResolvedValueOnce(jsonResponse(productContract('signed-out')));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    const signal = new AbortController().signal;

    await expect(source.productContract(signal)).resolves.toMatchObject({ apiVersion: 'authenticated' });
    await source.logout(signal);
    await expect(source.productContract(signal)).resolves.toMatchObject({ apiVersion: 'signed-out' });

    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/product-contract');
    expect(String(fetchMock.mock.calls[1][0])).toContain('/api/auth/logout');
    expect(String(fetchMock.mock.calls[2][0])).toContain('/api/product-contract');
  });

  it('invalidates the cached Product Contract when the verified authorization revision changes', async () => {
    const viewer = (authorizationRevision: string) => ({
      authenticated: true,
      authority: 'local',
      setupRequired: false,
      accountMode: 'local',
      accountId: 'account-1',
      serverId: 'server-1',
      profileId: 'profile-1',
      authorizationRevision,
      serverFriendlyName: 'Living Room',
      user: { id: 'account-1', displayName: 'Owner', email: 'owner@example.test', role: 'owner', authOrigin: 'local', authProvider: 'local', permissions: {}, preferences: { sidebarOrder: [] } },
    });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(viewer('policy-1')))
      .mockResolvedValueOnce(jsonResponse(productContract('policy-1')))
      .mockResolvedValueOnce(jsonResponse(viewer('policy-2')))
      .mockResolvedValueOnce(jsonResponse(productContract('policy-2')));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    const signal = new AbortController().signal;

    await source.viewer(signal);
    await expect(source.productContract(signal)).resolves.toMatchObject({ apiVersion: 'policy-1' });
    await source.viewer(signal);
    await expect(source.productContract(signal)).resolves.toMatchObject({ apiVersion: 'policy-2' });

    expect(fetchMock).toHaveBeenCalledTimes(4);
    expect(String(fetchMock.mock.calls[1][0])).toContain('/api/product-contract');
    expect(String(fetchMock.mock.calls[3][0])).toContain('/api/product-contract');
  });

  it('surfaces an HTTP failure instead of inventing empty data', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({}, false, 503)));
    await expect(new HttpPorticoDataSource().search('Fargo', new AbortController().signal)).rejects.toThrow('Service Unavailable');
  });

  it('loads operational-notice recipients from the dedicated owner-only directory', async () => {
    const directory = {
      profiles: [{ authority: 'local', audience: 'profile', accountId: 'account-1', profileId: 'child-1', accountName: 'Rivera household', profileName: 'Kids' }],
      accountAdmins: [{ authority: 'hosted', audience: 'account-admin', accountId: 'account-2', accountName: 'Sam Rivera' }],
    };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(directory));
    vi.stubGlobal('fetch', fetchMock);

    await expect(new HttpPorticoDataSource().ownerViewerNotificationRecipients(new AbortController().signal)).resolves.toEqual(directory);
    expect(new URL(String(fetchMock.mock.calls[0][0]), window.location.origin).pathname).toBe('/api/admin/viewer-notification-recipients');
    expect(fetchMock.mock.calls[0][1]).toEqual(expect.objectContaining({ credentials: 'include', method: 'GET' }));
  });

  it('preserves per-group search failure status and Product Language identity', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(jsonResponse(productContract('search-v1')))
      .mockResolvedValueOnce(jsonResponse({
      query: 'Fargo',
      sort: 'relevance',
      direction: 'desc',
      groups: [{
        id: 'shows', title: 'Shows', entityKind: 'show', status: 'error',
        errorCode: 'search_group_timeout', messageId: 'search.group-timeout',
        items: [], hasMore: false,
      }],
      })));

    const response = await new HttpPorticoDataSource().searchPage({ query: 'Fargo' }, new AbortController().signal);

    expect(response.groups).toEqual([expect.objectContaining({
      id: 'shows', status: 'error', errorCode: 'search_group_timeout', messageId: 'search.group-timeout',
    })]);
  });

  it('preserves server hierarchy truncation and nested children alongside the playback target', async () => {
    const base = {
      sortTitle: '', genres: [], tags: [], labels: [], addedAt: '2026-07-12T12:00:00Z',
      images: {}, state: { watchlisted: false, favorite: false, watched: false, progressSeconds: 0, rating: 0 }, actions: [],
    };
    const playbackTarget = {
      ...base, id: 'episode-9', entityKind: 'episode', title: 'The Castle', sortTitle: 'The Castle',
      seasonNumber: 2, episodeNumber: 9, durationSeconds: 2880,
      state: { ...base.state, progressSeconds: 330 }, actions: ['play'],
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({
      ...base, id: 'show-1', entityKind: 'show', title: 'Fargo', sortTitle: 'Fargo',
      childrenTruncated: true,
      children: [{
        ...base, id: 'season-2', entityKind: 'season', title: 'Season 2', sortTitle: 'Season 2', seasonNumber: 2,
        childrenTruncated: true,
        children: [{ ...base, id: 'episode-9', entityKind: 'episode', title: 'The Castle', sortTitle: 'The Castle', seasonNumber: 2, episodeNumber: 9 }],
      }],
      playbackTarget,
    })));

    const detail = await new HttpPorticoDataSource().media('show-1', new AbortController().signal);

    expect(detail.children).toHaveLength(1);
    expect(detail.childrenTruncated).toBe(true);
    expect(detail.children?.[0]).toMatchObject({ id: 'season-2', entityKind: 'season', childrenTruncated: true });
    expect(detail.children?.[0].children?.[0]).toMatchObject({ id: 'episode-9', entityKind: 'episode', episodeNumber: 9 });
    expect(detail.playbackTarget).toMatchObject({ id: 'episode-9', entityKind: 'episode', seasonNumber: 2, episodeNumber: 9, progressSeconds: 330, actions: ['play'] });
  });

  it('uses authoritative media-job, version, and authenticated download routes', async () => {
    const job = { id: 'job_1', type: 'media_analyze', status: 'queued', progress: 0, message: 'Queued.', createdAt: '2026-07-11T00:00:00Z', updatedAt: '2026-07-11T00:00:00Z' };
    const options = { options: [], optimizedVersions: [], profiles: [], defaultProfile: '1080p-high', canDownload: true };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(job, true, 201))
      .mockResolvedValueOnce(jsonResponse(options))
      .mockResolvedValueOnce(jsonResponse({ ...job, type: 'optimize_version' }, true, 201))
      .mockResolvedValueOnce(jsonResponse({
        id: 'preparation-1', mediaId: 'movie/1', mediaTitle: 'Movie', qualityProfile: 'source', state: 'ready',
        progress: 100, sizeKind: 'unknown', canPause: false, canCancel: false, canRetry: false, canRemove: true,
        createdAt: '2026-07-11T00:00:00Z', updatedAt: '2026-07-11T00:00:00Z',
      }, true, 201))
      .mockResolvedValueOnce(jsonResponse({
        downloadUrl: '/api/media/movie%2F1/download?profile=source',
        expiresAt: '2026-07-11T00:02:00Z',
        profile: 'source',
      }, true, 201));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    const signal = new AbortController().signal;

    await source.queueMediaJob('movie/1', 'media_analyze', { analysisMode: 'probe' }, signal);
    await source.mediaDownloadOptions('movie/1', signal);
    await source.createOptimizedVersion('movie/1', '720p-medium', signal);
    const url = await source.createMediaDownloadURL('movie/1', 'source', signal);

    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/media/movie%2F1/jobs');
    expect(fetchMock.mock.calls[0][1]).toEqual(expect.objectContaining({ method: 'POST', body: JSON.stringify({ type: 'media_analyze', analysisMode: 'probe' }) }));
    expect(String(fetchMock.mock.calls[1][0])).toContain('/api/media/movie%2F1/download-options');
    expect(String(fetchMock.mock.calls[2][0])).toContain('/api/media/movie%2F1/optimized');
    expect(String(fetchMock.mock.calls[3][0])).toContain('/api/download-preparations');
    expect(fetchMock.mock.calls[3][1]).toEqual(expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ mediaId: 'movie/1', qualityProfile: 'source' }),
    }));
    expect(String(fetchMock.mock.calls[4][0])).toContain('/api/download-preparations/preparation-1/grant');
    expect(fetchMock.mock.calls[4][1]).toEqual(expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ delivery: 'browser' }),
    }));
    expect(fetchMock).toHaveBeenCalledTimes(5);
    expect(url).toContain('/api/media/movie%2F1/download?profile=source');
    expect(url).not.toContain('grant=');
  });

  it('rejects cross-origin download grants and artwork URLs', async () => {
    const client = {
      createDownloadPreparation: vi.fn().mockResolvedValue({ id: 'preparation-1', state: 'ready' }),
      createDownloadPreparationGrant: vi.fn().mockResolvedValue({ downloadUrl: 'https://attacker.example/collect?grant=secret' }),
      resourceUrl: (path: string) => new URL(path, 'https://server.example').toString(),
    } as unknown as PorticoClient;
    const source = new HttpPorticoDataSource(client);
    await expect(source.createMediaDownloadURL('movie-1', 'source', new AbortController().signal))
      .rejects.toThrow('untrusted download address');

    const resolve = (path: string) => new URL(path, 'https://server.example').toString();
    expect(imagePath('//evil.example/pixel', 'fallback', resolve)).toBe('fallback');
    expect(imagePath('http://localhost.evil/pixel', 'fallback', resolve)).toBe('fallback');
    expect(imagePath('https://server.example/api/media/movie-1/image', 'fallback', resolve))
      .toBe('https://server.example/api/media/movie-1/image');
  });

  it('maps stored artwork and uses the authenticated media-image operations', async () => {
    const item = {
      id: 'movie/1', type: 'movie', title: 'Arrival', sortTitle: 'Arrival', addedAt: '2026-07-11T00:00:00Z',
      actions: ['metadata.edit'], images: { poster: '/api/artwork/movie%2F1/poster.jpg', backdrop: '', thumb: '' },
      state: { watchlisted: false, favorite: false, watched: false, progressSeconds: 0, rating: 0 },
      genres: [], tags: [], labels: [], mediaImages: [{ id: 'image-1', type: 'poster', source: 'provider', provider: 'tmdb', remoteUrl: 'https://image.example/poster.jpg', preferred: true, createdAt: '2026-07-11T00:00:00Z' }],
    };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(item))
      .mockResolvedValueOnce(jsonResponse(item, true, 201))
      .mockResolvedValueOnce(jsonResponse({ ok: true }))
      .mockResolvedValueOnce(jsonResponse(item))
      .mockResolvedValueOnce(jsonResponse(item));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    const signal = new AbortController().signal;

    const detail = await source.media('movie/1', signal);
    await source.uploadMediaImage('movie/1', 'poster', new File(['image'], 'poster.png', { type: 'image/png' }), 4, signal);
    await source.deleteMediaImage('movie/1', 'image-1', 4, signal);
    await source.setPreferredMediaImage('movie/1', 'image-2', 4, signal);
    await source.reorderMediaImages('movie/1', ['image-2', 'image-1'], 4, signal);

    expect(detail.mediaImages?.[0]).not.toHaveProperty('remoteUrl');
    expect(String(fetchMock.mock.calls[1][0])).toContain('/api/media/movie%2F1/images');
    expect(fetchMock.mock.calls[1][1]).toEqual(expect.objectContaining({ method: 'POST', credentials: 'include', body: expect.any(FormData) }));
    expect((fetchMock.mock.calls[1][1]?.body as FormData).get('type')).toBe('poster');
    expect((fetchMock.mock.calls[1][1]?.body as FormData).get('expectedRevision')).toBe('4');
    expect(String(fetchMock.mock.calls[2][0])).toContain('/api/media/movie%2F1/images/image-1?expectedRevision=4');
    expect(fetchMock.mock.calls[2][1]).toEqual(expect.objectContaining({ method: 'DELETE', credentials: 'include' }));
    expect(String(fetchMock.mock.calls[3][0])).toContain('/api/media/movie%2F1/images/image-2/preferred');
    expect(fetchMock.mock.calls[3][1]).toEqual(expect.objectContaining({ body: JSON.stringify({ expectedRevision: 4 }) }));
    expect(String(fetchMock.mock.calls[4][0])).toContain('/api/media/movie%2F1/images/order');
    expect(fetchMock.mock.calls[4][1]).toEqual(expect.objectContaining({ method: 'POST', body: JSON.stringify({ imageIds: ['image-2', 'image-1'], expectedRevision: 4 }) }));
  });

  it('uses the authoritative subtitle, lyrics, and optimized-version contracts', async () => {
    const item = {
      id: 'track/1', type: 'track', title: 'Kiara', sortTitle: 'Kiara', addedAt: '2026-07-11T00:00:00Z',
      actions: ['metadata.edit'], images: { poster: '', backdrop: '', thumb: '' },
      state: { watchlisted: false, favorite: false, watched: false, progressSeconds: 0, rating: 0 },
      genres: [], tags: [], labels: [],
    };
    const candidate = { provider: 'lrclib', externalId: 'lyrics-1', trackName: 'Kiara', format: 'lrc', synced: true };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(item, true, 201))
      .mockResolvedValueOnce(jsonResponse(item))
      .mockResolvedValueOnce(jsonResponse({ ok: true }))
      .mockResolvedValueOnce(jsonResponse(item, true, 201))
      .mockResolvedValueOnce(jsonResponse(item))
      .mockResolvedValueOnce(jsonResponse({ items: [candidate], total: 1 }))
      .mockResolvedValueOnce(jsonResponse(item))
      .mockResolvedValueOnce(jsonResponse({ ok: true }))
      .mockResolvedValueOnce(jsonResponse({ ok: true }));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    const signal = new AbortController().signal;

    await source.uploadSubtitle('track/1', new File(['WEBVTT'], 'english.vtt'), 'en', 'English', signal);
    await source.updateSubtitle('track/1', 'subtitle/1', 750, signal);
    await source.deleteSubtitle('track/1', 'subtitle/1', signal);
    await source.uploadLyrics('track/1', new File(['lyrics'], 'kiara.lrc'), 'en', signal);
    await source.fetchLyrics('track/1', signal);
    const results = await source.searchLyrics('track/1', 'Kiara Bonobo', signal);
    await source.applyLyrics('track/1', results[0], signal);
    await source.deleteLyrics('track/1', 'lyrics/1', signal);
    await source.deleteOptimizedVersion('track/1', '720p-medium', signal);

    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/media/track%2F1/subtitles');
    expect((fetchMock.mock.calls[0][1]?.body as FormData).get('label')).toBe('English');
    expect(String(fetchMock.mock.calls[1][0])).toContain('/api/media/track%2F1/subtitles/subtitle%2F1');
    expect(fetchMock.mock.calls[1][1]).toEqual(expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ offsetMs: 750 }) }));
    expect(fetchMock.mock.calls[2][1]).toEqual(expect.objectContaining({ method: 'DELETE' }));
    expect(String(fetchMock.mock.calls[5][0])).toContain('/api/media/track%2F1/lyrics/search?query=Kiara+Bonobo');
    expect(fetchMock.mock.calls[6][1]).toEqual(expect.objectContaining({ method: 'POST', body: JSON.stringify({ provider: 'lrclib', externalId: 'lyrics-1' }) }));
    expect(String(fetchMock.mock.calls[8][0])).toContain('/api/media/track%2F1/optimized/720p-medium');
    expect(fetchMock.mock.calls[8][1]).toEqual(expect.objectContaining({ method: 'DELETE' }));
  });

  it('keeps private share discovery and saved-resource permissions inside the production adapter', async () => {
    const updated = {
      id: 'playlist/1', ownerUserId: 'owner/1', title: 'Weekend queue', summary: 'Ready to watch',
      visibility: 'private', canEdit: true, itemCount: 2,
      shares: [{ userId: 'user/2', displayName: 'Sam Rivera', canEdit: true }],
      createdAt: '2026-07-11T00:00:00Z', updatedAt: '2026-07-11T01:00:00Z',
    };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ items: [{ userId: 'user/2', displayName: 'Sam Rivera' }], hasMore: false }))
      .mockResolvedValueOnce(jsonResponse(updated));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    const signal = new AbortController().signal;

    const candidates = await source.savedShareCandidates('Sam Rivera', 20, signal);
    const resource = await source.updateSavedResource('playlist', 'playlist/1', {
      title: 'Weekend queue',
      summary: 'Ready to watch',
      visibility: 'private',
      shares: [{ userId: 'user/2', canEdit: true }],
    }, signal);

    expect(candidates).toEqual({ items: [{ userId: 'user/2', displayName: 'Sam Rivera' }], hasMore: false });
    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/saved/share-candidates?q=Sam+Rivera&limit=20');
    expect(fetchMock.mock.calls[0][1]).toEqual(expect.objectContaining({ credentials: 'include' }));
    expect(String(fetchMock.mock.calls[1][0])).toContain('/api/playlists/playlist%2F1');
    expect(fetchMock.mock.calls[1][1]).toEqual(expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify({
        title: 'Weekend queue',
        summary: 'Ready to watch',
        visibility: 'private',
        shares: [{ userId: 'user/2', canEdit: true }],
      }),
    }));
    expect(resource).toMatchObject({ ownerUserId: 'owner/1', shares: [{ userId: 'user/2', displayName: 'Sam Rivera', canEdit: true }] });
  });

  it('exchanges the profile-selection token as the browser session grant', async () => {
    const policy = { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({
        accountAuthenticationToken: 'account-authentication-token',
        expiresAt: '2099-07-16T21:00:00.000Z',
        directory: { authority: 'local', accountId: 'account-1', serverId: 'server-1', profilesAllowed: true, profiles: [{ id: 'profile-1', name: 'Living room', isPrimary: true, isAccountAdmin: true, hasPIN: true, pinRevision: 2, sortOrder: 0, policy }] },
      }))
      .mockResolvedValueOnce(jsonResponse({ token: 'profile-selection-token', authority: 'local', accountId: 'account-1', serverId: 'server-1', profileId: 'profile-1', pinRevision: 2, installationId: 'installation-1', expiresAt: '2099-07-16T21:00:00.000Z' }))
      .mockResolvedValueOnce(jsonResponse({ authenticated: true, authority: 'local', setupRequired: false, accountMode: 'local', accountId: 'account-1', serverId: 'server-1', profileId: 'profile-1', authorizationRevision: 'revision-1', serverFriendlyName: 'Family Media', user: { id: 'account-1', displayName: 'Living room', email: 'owner@example.test', role: 'owner', authOrigin: 'local', authProvider: 'local', permissions: {}, preferences: { sidebarOrder: [] } } }))
      .mockResolvedValueOnce(jsonResponse(clientCompatibilityFixture.system))
      .mockResolvedValueOnce(jsonResponse(clientCompatibilityFixture.productContract))
      .mockResolvedValueOnce(jsonResponse({ accountServerInstallation: { version: 'v1', revision: 4, values: { rememberAccount: true, profileSelection: 'last-used' } } }))
      .mockResolvedValueOnce(jsonResponse({ version: 'v1', revision: 5, values: { rememberAccount: true, profileSelection: 'last-used', lastProfileId: 'profile-1' } }));
    vi.stubGlobal('fetch', fetchMock);

    await new HttpPorticoDataSource(undefined, async () => 'installation-1').switchLocalProfile({ login: 'owner', password: 'password', profileId: 'profile-1', pin: '1234' }, new AbortController().signal);

    expect(fetchMock.mock.calls[0][1]).toEqual(expect.objectContaining({ body: expect.stringContaining('"platform":"web"') }));
    expect(String(fetchMock.mock.calls[1][0])).toContain('/api/auth/profile-selections/local');
    expect(fetchMock.mock.calls[1][1]).toEqual(expect.objectContaining({ body: expect.stringContaining('"pin":"1234"') }));
    expect(String(fetchMock.mock.calls[2][0])).toContain('/api/auth/profile-sessions/browser');
    expect(JSON.parse(String(fetchMock.mock.calls[2][1]?.body))).toEqual({ rememberOnBrowser: true, selectionGrant: 'profile-selection-token' });
    expect(String(fetchMock.mock.calls[3][0])).toContain('/api/system');
    expect(String(fetchMock.mock.calls[4][0])).toContain('/api/product-contract');
    expect(String(fetchMock.mock.calls[5][0])).toMatch(/\/api\/preferences$/);
    expect(String(fetchMock.mock.calls[6][0])).toContain('/api/preferences/profile-activation');
    expect(fetchMock.mock.calls[6][1]).toEqual(expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ version: 'v1', expectedRevision: 4 }),
    }));
  });

  it('runs remembered Local Auth accounts through the canonical chooser before creating a profile session', async () => {
    const policy = { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
    const vault = createBrowserHostedConnectionVault(undefined);
    const installationId = await vault.installationId();
    const challenge = {
      accountAuthenticationToken: 'remembered-account-proof',
      expiresAt: '2099-01-01T00:00:00.000Z',
      directory: { authority: 'local', accountId: 'account-restart', serverId: 'server-restart', profilesAllowed: true, profiles: [
        { id: 'owner', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy },
        { id: 'kids', name: 'Kids', isPrimary: false, isAccountAdmin: false, hasPIN: true, pinRevision: 2, sortOrder: 1, policy },
      ] },
    };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse(challenge))
      .mockResolvedValueOnce(jsonResponse({ token: 'selected-kids', authority: 'local', accountId: 'account-restart', serverId: 'server-restart', profileId: 'kids', pinRevision: 2, installationId: 'legacy-installation-metadata', expiresAt: '2099-01-01T00:00:00.000Z' }))
      .mockResolvedValueOnce(jsonResponse({ authenticated: true, setupRequired: false, accountMode: 'local', accountId: 'account-restart', serverId: 'server-restart', profileId: 'kids', authorizationRevision: 'revision-1', serverFriendlyName: 'Family Media', user: { id: 'account-restart', displayName: 'Kids', email: 'owner@example.test', role: 'owner', authOrigin: 'local', authProvider: 'local', permissions: {}, preferences: { sidebarOrder: [] } } }))
      .mockResolvedValueOnce(jsonResponse(clientCompatibilityFixture.system))
      .mockResolvedValueOnce(jsonResponse(clientCompatibilityFixture.productContract))
      .mockResolvedValueOnce(jsonResponse({ accountServerInstallation: { version: 'v1', revision: 4, values: { rememberAccount: true, profileSelection: 'last-used' } } }))
      .mockResolvedValueOnce(jsonResponse({ version: 'v1', revision: 5, values: { rememberAccount: true, profileSelection: 'last-used', lastProfileId: 'kids' } }));
    vi.stubGlobal('fetch', fetchMock);
    await vault.saveProfileLaunchPreference({ authority: 'local', accountId: 'account-restart', serverId: 'server-restart', profileId: 'owner', deviceClass: 'web', installationId }, { rememberAccount: true, profileSelection: 'ask', lastProfileId: 'owner' });
    const source = new HttpPorticoDataSource(undefined, async () => installationId, vault);
    const signal = new AbortController().signal;

    const loginChallenge = await source.beginLocalProfileLogin({ login: 'owner', password: 'password', rememberOnBrowser: true }, signal);
    const selection = await source.verifyLocalProfileSelection(loginChallenge, 'kids', '1234', signal);
    const viewer = await source.publishLocalProfileSession(selection, signal);

    expect(viewer.viewerScope?.profileId).toBe('kids');
    expect(fetchMock.mock.calls[0][1]).toEqual(expect.objectContaining({ body: expect.stringContaining('"installationId"') }));
    expect(fetchMock.mock.calls[1][1]).toEqual(expect.objectContaining({ body: expect.stringContaining('"profileId":"kids"') }));
    expect(String(fetchMock.mock.calls[2][0])).toContain('/api/auth/profile-sessions/browser');
  });

  it('automatically opens the unlocked last-used Local Auth profile on restart', async () => {
    const policy = { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
    const vault = createBrowserHostedConnectionVault(undefined);
    const installationId = await vault.installationId();
    await vault.saveProfileLaunchPreference({ authority: 'local', accountId: 'account-last', serverId: 'server-last', profileId: 'owner', deviceClass: 'web', installationId }, { rememberAccount: true, profileSelection: 'last-used', lastProfileId: 'kids' });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ accountAuthenticationToken: 'remembered-last', expiresAt: '2099-01-01T00:00:00.000Z', directory: { authority: 'local', accountId: 'account-last', serverId: 'server-last', profilesAllowed: true, profiles: [
        { id: 'owner', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy },
        { id: 'kids', name: 'Kids', isPrimary: false, isAccountAdmin: false, hasPIN: false, pinRevision: 0, sortOrder: 1, policy },
      ] } }))
      .mockResolvedValueOnce(jsonResponse({ token: 'selected-kids', authority: 'local', accountId: 'account-last', serverId: 'server-last', profileId: 'kids', pinRevision: 0, installationId, expiresAt: '2099-01-01T00:00:00.000Z' }))
      .mockResolvedValueOnce(jsonResponse({ authenticated: true, setupRequired: false, accountMode: 'local', accountId: 'account-last', serverId: 'server-last', profileId: 'kids', authorizationRevision: 'revision-1', serverFriendlyName: 'Family Media', user: { id: 'account-last', displayName: 'Kids', email: 'owner@example.test', role: 'owner', authOrigin: 'local', authProvider: 'local', permissions: {}, preferences: { sidebarOrder: [] } } }))
      .mockResolvedValueOnce(jsonResponse(clientCompatibilityFixture.system))
      .mockResolvedValueOnce(jsonResponse(clientCompatibilityFixture.productContract))
      .mockResolvedValueOnce(jsonResponse({ accountServerInstallation: { version: 'v1', revision: 4, values: { rememberAccount: true, profileSelection: 'last-used', lastProfileId: 'kids' } } }));
    vi.stubGlobal('fetch', fetchMock);

    const viewer = await new HttpPorticoDataSource(undefined, undefined, vault).switchBrowserAccount('account-last', new AbortController().signal);

    expect(viewer.viewerScope?.profileId).toBe('kids');
    expect(fetchMock.mock.calls[1][1]).toEqual(expect.objectContaining({ body: expect.stringContaining('"profileId":"kids"') }));
  });

  it('rejects mismatched locked-profile trust and leaves the restart at the chooser', async () => {
    const policy = { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
    const vault = createBrowserHostedConnectionVault(undefined);
    const installationId = await vault.installationId();
    const scope = { authority: 'local' as const, accountId: 'account-locked', serverId: 'server-locked', profileId: 'owner', deviceClass: 'web' as const, installationId };
    await vault.saveProfileLaunchPreference(scope, { rememberAccount: true, profileSelection: 'last-used', lastProfileId: 'kids' });
    await vault.saveAutomaticProfileTrust({ version: 'v1', purpose: 'automatic-profile-selection', token: 'stale-trust', authority: 'local', accountId: 'account-locked', serverId: 'server-locked', profileId: 'kids', pinRevision: 1, installationId, expiresAt: '2099-01-01T00:00:00.000Z' });
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ accountAuthenticationToken: 'remembered-locked', expiresAt: '2099-01-01T00:00:00.000Z', directory: { authority: 'local', accountId: 'account-locked', serverId: 'server-locked', profilesAllowed: true, profiles: [
      { id: 'owner', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy },
      { id: 'kids', name: 'Kids', isPrimary: false, isAccountAdmin: false, hasPIN: true, pinRevision: 2, sortOrder: 1, policy },
    ] } }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(new HttpPorticoDataSource(undefined, undefined, vault).switchBrowserAccount('account-locked', new AbortController().signal)).rejects.toBeInstanceOf(LocalProfileSelectionRequiredError);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('does not update lastProfileId until the authoritative Local Auth profile session succeeds', async () => {
    const policy = { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
    const vault = createBrowserHostedConnectionVault(undefined);
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ accountAuthenticationToken: 'remembered-fail', expiresAt: '2099-01-01T00:00:00.000Z', directory: { authority: 'local', accountId: 'account-fail', serverId: 'server-fail', profilesAllowed: true, profiles: [{ id: 'owner', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy }] } }))
      .mockResolvedValueOnce(jsonResponse({ token: 'selected-owner', authority: 'local', accountId: 'account-fail', serverId: 'server-fail', profileId: 'owner', pinRevision: 0, installationId: await vault.installationId(), expiresAt: '2099-01-01T00:00:00.000Z' }))
      .mockResolvedValueOnce(jsonResponse({ code: 'session_failed' }, false, 500));
    vi.stubGlobal('fetch', fetchMock);

    await expect(new HttpPorticoDataSource(undefined, undefined, vault).switchBrowserAccount('account-fail', new AbortController().signal)).rejects.toThrow();
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes('/api/preferences'))).toBe(false);
  });

  it('skips the chooser for one unlocked remembered Local Auth profile', async () => {
    const policy = { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
    const vault = createBrowserHostedConnectionVault(undefined);
    const installationId = await vault.installationId();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ accountAuthenticationToken: 'remembered-one', expiresAt: '2099-01-01T00:00:00.000Z', directory: { authority: 'local', accountId: 'account-one', serverId: 'server-one', profilesAllowed: true, profiles: [{ id: 'owner', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy }] } }))
      .mockResolvedValueOnce(jsonResponse({ token: 'selected-owner', authority: 'local', accountId: 'account-one', serverId: 'server-one', profileId: 'owner', pinRevision: 0, installationId, expiresAt: '2099-01-01T00:00:00.000Z' }))
      .mockResolvedValueOnce(jsonResponse({ authenticated: true, setupRequired: false, accountMode: 'local', accountId: 'account-one', serverId: 'server-one', profileId: 'owner', authorizationRevision: 'revision-1', serverFriendlyName: 'Family Media', user: { id: 'account-one', displayName: 'Owner', email: 'owner@example.test', role: 'owner', authOrigin: 'local', authProvider: 'local', permissions: {}, preferences: { sidebarOrder: [] } } }))
      .mockResolvedValueOnce(jsonResponse(clientCompatibilityFixture.system))
      .mockResolvedValueOnce(jsonResponse(clientCompatibilityFixture.productContract))
      .mockResolvedValueOnce(jsonResponse({ accountServerInstallation: { version: 'v1', revision: 1, values: { rememberAccount: true, profileSelection: 'ask' } } }))
      .mockResolvedValueOnce(jsonResponse({ version: 'v1', revision: 2, values: { rememberAccount: true, profileSelection: 'ask', lastProfileId: 'owner' } }));
    vi.stubGlobal('fetch', fetchMock);

    const viewer = await new HttpPorticoDataSource(undefined, undefined, vault).switchBrowserAccount('account-one', new AbortController().signal);

    expect(viewer.viewerScope?.profileId).toBe('owner');
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes('/api/auth/profile-selections/local'))).toBe(true);
  });

  it('routes Hosted profile directory, administration, and switching through Portico Account', async () => {
    const policy = { version: 'v1' as const, maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
    const profiles = [
      { id: 'primary', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: true, pinRevision: 1, sortOrder: 0, policy },
      { id: 'kids', name: 'Kids', isPrimary: false, isAccountAdmin: false, hasPIN: true, pinRevision: 2, sortOrder: 1, policy },
    ];
    const request = vi.fn().mockResolvedValue({ authenticated: true, authority: 'hosted', setupRequired: false, accountMode: 'portico', accountId: 'account-1', serverId: 'server-1', profileId: 'primary', authorizationRevision: 'revision-1', serverFriendlyName: 'Family Media', user: { id: 'account-1', displayName: 'Owner', email: 'owner@example.test', role: 'owner', authOrigin: 'portico', authProvider: 'portico', permissions: {}, preferences: { sidebarOrder: [] } } });
    const accountProfiles = vi.fn().mockResolvedValue({ authority: 'hosted', accountId: 'server-mirror-account-1', serverId: 'server-1', profilesAllowed: true, profiles, canManage: false });
    const client = { request, accountProfiles } as unknown as PorticoClient;
    const createProfileAdministrationSession = vi.fn().mockResolvedValue({ token: 'cloud-proof', expiresAt: '2099-01-01T00:00:00Z' });
    const createProfile = vi.fn().mockResolvedValue({ revision: 8, profile: { ...profiles[1], id: 'guest', name: 'Guest', hasPIN: false, pinRevision: 0 } });
    const hosted = { request: vi.fn(), profiles: vi.fn().mockResolvedValue({ accountId: 'hosted-account-1', revision: 7, total: 2, profiles }), createProfileAdministrationSession, createProfile } as unknown as HostedServicesClient;
    const switchHostedProfile = vi.fn().mockResolvedValue({ authenticated: true, setupRequired: false, serverName: 'Family Media', viewerScope: { authority: 'hosted', accountId: 'account-1', serverId: 'server-1', profileId: 'kids', authorizationRevision: 'revision-2' } });
    const source = new HttpPorticoDataSource(client, { hostedClient: hosted, connectionVault: createBrowserHostedConnectionVault(undefined), switchHostedProfile });
    const signal = new AbortController().signal;

    await source.viewer(signal);
    const directory = await source.accountProfiles(signal);
    const proof = await source.createProfileAdministrationProof({ pin: '1234' }, signal);
    await source.createAccountProfile({ name: 'Guest', policy }, proof.token, signal);
    const selected = await source.switchHostedProfile({ profileId: 'kids', pin: '2468' }, signal);

    expect(directory).toMatchObject({ authority: 'hosted', canManage: true, profiles: [{ id: 'primary' }, { id: 'kids' }] });
    expect(createProfileAdministrationSession).toHaveBeenCalledWith({ pin: '1234' }, { signal });
    expect(createProfile).toHaveBeenCalledWith(expect.objectContaining({ name: 'Guest', restrictions: policy }), { token: 'cloud-proof' }, { signal });
	expect(switchHostedProfile).toHaveBeenCalledWith('kids', '2468', signal);
    expect(selected.viewerScope?.profileId).toBe('kids');
  });

	it('preserves the authoritative viewer scope when account profile fields change', async () => {
		const fetchMock = vi.fn()
			.mockResolvedValueOnce(jsonResponse({
				authenticated: true, setupRequired: false, accountMode: 'local', accountId: 'account-1', serverId: 'server-1', profileId: 'profile-1', authorizationRevision: 'revision-1', serverFriendlyName: 'Family Media',
				user: { id: 'account-1', displayName: 'Before', email: 'before@example.test', role: 'owner', authOrigin: 'local', authProvider: 'local', permissions: {}, preferences: { sidebarOrder: [] } },
			}))
			.mockResolvedValueOnce(jsonResponse({ id: 'account-1', displayName: 'After', email: 'after@example.test', role: 'owner', authOrigin: 'local', authProvider: 'local', permissions: {}, preferences: { sidebarOrder: [] } }));
		vi.stubGlobal('fetch', fetchMock);
		const source = new HttpPorticoDataSource();
		const signal = new AbortController().signal;
		const before = await source.viewer(signal);
		const after = await source.updateProfile({ displayName: 'After', email: 'after@example.test' }, signal);

		expect(after.viewerScope).toEqual(before.viewerScope);
		expect(after.user).toMatchObject({ displayName: 'After', email: 'after@example.test' });
	});

  it('transmits the current account password for Local Auth PIN set and clear operations', async () => {
    const setAccountProfilePIN = vi.fn().mockResolvedValue(undefined);
    const clearAccountProfilePIN = vi.fn().mockResolvedValue(undefined);
    const client = { setAccountProfilePIN, clearAccountProfilePIN } as unknown as PorticoClient;
    const source = new HttpPorticoDataSource(client);
    const signal = new AbortController().signal;

    await source.setAccountProfilePin('profile-1', { pin: '2468', password: 'current-password' }, 'primary-proof', signal);
    await source.clearAccountProfilePin('profile-1', { password: 'current-password' }, 'primary-proof', signal);

    expect(setAccountProfilePIN).toHaveBeenCalledWith('profile-1', { pin: '2468', password: 'current-password' }, 'primary-proof', { signal });
    expect(clearAccountProfilePIN).toHaveBeenCalledWith('profile-1', { password: 'current-password' }, 'primary-proof', { signal });
  });

  it('uses Hosted primary-PIN recovery and MFA-aware PIN mutation operations', async () => {
    const viewerResponse = { authenticated: true, authority: 'hosted', setupRequired: false, accountMode: 'portico', accountId: 'account-1', serverId: 'server-1', profileId: 'primary', authorizationRevision: 'revision-1', serverFriendlyName: 'Family Media', user: { id: 'account-1', displayName: 'Owner', email: 'owner@example.test', role: 'owner', authOrigin: 'portico', authProvider: 'portico', permissions: {}, preferences: { sidebarOrder: [] } } };
    const createProfileAdministrationSession = vi.fn().mockResolvedValue({ token: 'recovered-proof', expiresAt: '2099-01-01T00:00:00Z' });
    const mfaStatus = vi.fn().mockResolvedValue({ enabled: true });
    const requestPasswordReset = vi.fn().mockResolvedValue({ ok: true });
    const setProfilePIN = vi.fn().mockResolvedValue({ ok: true, pinRevision: 4 });
    const clearProfilePIN = vi.fn().mockResolvedValue({ ok: true });
    const hosted = { request: vi.fn(), createProfileAdministrationSession, mfaStatus, requestPasswordReset, setProfilePIN, clearProfilePIN } as unknown as HostedServicesClient;
    const source = new HttpPorticoDataSource({ request: vi.fn().mockResolvedValue(viewerResponse) } as unknown as PorticoClient, {
      hostedClient: hosted,
      connectionVault: createBrowserHostedConnectionVault(undefined),
      switchHostedProfile: vi.fn(),
    });
    const signal = new AbortController().signal;

    await source.viewer(signal);
    expect(await source.porticoProfileMFAStatus(signal)).toEqual({ enabled: true });
    await source.requestPorticoProfileRecoveryEmail(signal);
    await source.createProfileAdministrationProof({ replacementPin: '8642', password: 'current-password', recoveryCode: 'PORTICO-RECOVERY' }, signal);
    await source.setAccountProfilePin('kids', { pin: '2468', password: 'current-password', mfaCode: '123456' }, 'primary-proof', signal);
    await source.clearAccountProfilePin('kids', { password: 'current-password', recoveryCode: 'PORTICO-RECOVERY' }, 'primary-proof', signal);

	expect(requestPasswordReset).toHaveBeenCalledWith({ email: 'owner@example.test' }, { signal });
    expect(createProfileAdministrationSession).toHaveBeenCalledWith(expect.objectContaining({
      replacementPin: '8642', password: 'current-password', recoveryCode: 'PORTICO-RECOVERY',
    }), { signal });
    expect(setProfilePIN).toHaveBeenCalledWith('kids', { pin: '2468', password: 'current-password', mfaCode: '123456' }, { token: 'primary-proof' }, { signal });
    expect(clearProfilePIN).toHaveBeenCalledWith('kids', { password: 'current-password', recoveryCode: 'PORTICO-RECOVERY' }, { token: 'primary-proof' }, { signal });
  });

  it('accepts only a strictly local return URL in a Hosted server setup claim', async () => {
    const request = vi.fn().mockResolvedValue({
      remoteAccess: {
        claim: {
          claimUrl: 'https://web.getportico.tv/claim?code=SETUP123&serverName=Family%20Room&returnUrl=http%3A%2F%2Flocalhost%3A32500%2F%3FporticoSetup%3Dcontinue',
        },
      },
    });
    const source = new HttpPorticoDataSource({ request } as unknown as PorticoClient);
    const signal = new AbortController().signal;

    await expect(source.startPorticoSetup('Family Room', signal)).resolves.toMatchObject({
      claimUrl: expect.stringContaining('returnUrl=http%3A%2F%2Flocalhost%3A32500%2F%3FporticoSetup%3Dcontinue'),
    });
    expect(request).toHaveBeenCalledWith('/api/auth/portico-setup/claim/start', {
      method: 'POST',
      body: { serverName: 'Family Room' },
      signal,
    });

    request.mockResolvedValueOnce({
      remoteAccess: {
        claim: {
          claimUrl: 'https://web.getportico.tv/claim?code=SETUP123&returnUrl=https%3A%2F%2Fevil.example%2F%3FporticoSetup%3Dcontinue',
        },
      },
    });
    await expect(source.startPorticoSetup('Family Room', signal)).rejects.toThrow('untrusted claim link');
  });

  it('uses typed discovery client operations for search history, people, and media children', async () => {
    const card = {
      id: 'episode/1', libraryId: 'shows', entityKind: 'episode', title: 'Pilot', artwork: {},
      userState: { watchlisted: false, favorite: false, watched: false, progressSeconds: 0 },
      availability: { status: 'available', fileCount: 1, missingFileCount: 0 }, actions: [],
    };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ items: [{ query: 'arrival', useCount: 2, lastUsedAt: '2026-07-11T00:00:00Z' }] }))
      .mockResolvedValueOnce(jsonResponse({}))
      .mockResolvedValueOnce(jsonResponse({
        person: { id: 'person/1', name: 'Amy Adams', roles: ['Actor'] },
        credits: [card],
        pageInfo: { hasMore: false, nextCursor: null },
      }))
      .mockResolvedValueOnce(jsonResponse({ items: [card], pageInfo: { hasMore: false, nextCursor: null } }));
    vi.stubGlobal('fetch', fetchMock);
    const source = new HttpPorticoDataSource();
    const signal = new AbortController().signal;

    await expect(source.searchHistory(signal)).resolves.toMatchObject([{ query: 'arrival', useCount: 2 }]);
    await source.clearSearchHistory(signal);
    await expect(source.person('person/1', signal, 'credit/20')).resolves.toMatchObject({ name: 'Amy Adams', knownFor: 'Actor', credits: [{ id: 'episode/1', rating: 'Not rated' }] });
    await expect(source.mediaChildren('show/1', signal, 'episode/20', 25)).resolves.toMatchObject({ items: [{ id: 'episode/1', rating: 'Not rated' }], hasMore: false });

    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/search/history');
    expect(fetchMock.mock.calls[1][1]).toEqual(expect.objectContaining({ method: 'DELETE' }));
    expect(String(fetchMock.mock.calls[2][0])).toContain('/api/people/person%2F1?cursor=credit%2F20');
    expect(String(fetchMock.mock.calls[3][0])).toContain('/api/media/show%2F1/children?limit=25&cursor=episode%2F20');
  });
});
