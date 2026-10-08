import test from 'node:test';
import assert from 'node:assert/strict';
import { LyricsService, parseLyricsView, activeLyricLine, lyricSeekPosition } from '../src/lyrics.ts';
import { readLyricsResponse } from '../src/lyrics-http.ts';
import { HttpLocalApi } from '../src/index.ts';
const scope = { serverId: 'server', viewerId: 'viewer-profile-session' }, target = { itemId: 'song', libraryId: 'music', sessionId: 'session', sessionGeneration: 2 };
const source = { id: 'asset', version: 'b'.repeat(64), startSeconds: 10, duration: 90, sourceDuration: 100, local: true };
const document = { format: 'lrc' as const, text: '[00:12]First\n[00:15]Next', lines: [{ atMs: 12000, text: 'First' }, { atMs: 15000, text: 'Next' }], embeddedOffsetMs: 0 };
const revision = { id: 'lyric', revision: 1, scope: 'private' as const, canManage: true, deleted: false, language: 'en', offsetMs: 500, format: 'lrc' as const, digest: 'c'.repeat(64), provenance: { origin: 'upload' as const, providerId: '', label: 'Uploaded text', rights: '' }, createdAt: '2026-09-06T00:00:00Z', document };
function view() { return { scope: { ...target, serverId: 'server', viewerFence: 'a'.repeat(64) }, sources: [source], source, resources: [revision], selection: { revision: 1, resource: revision }, canShare: false, providers: { lrclib: false, configured: true, revision: 1 } }; }
function deferred<T>() { let resolve!: (v: T) => void; let reject!: (e: unknown) => void; const promise = new Promise<T>((r, j) => { resolve = r; reject = j; }); return { promise, resolve, reject }; }
function api(request: (path: string, method?: string, body?: unknown, signal?: AbortSignal) => Promise<unknown>) { return { request: request as <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => Promise<T> }; }
test('uses accepted source time, positive delay, backward seeks and clipped seek actions', () => {
    assert.equal(activeLyricLine(document, 2.49, 500, 10), -1);
    assert.equal(activeLyricLine(document, 2.5, 500, 10), 0);
    assert.equal(activeLyricLine(document, 5.5, 500, 10), 1);
    assert.equal(activeLyricLine(document, 2.5, 500, 10), 0);
    assert.equal(lyricSeekPosition(document.lines[0], 500, source), 2.5);
    assert.equal(lyricSeekPosition({ atMs: 5000, text: 'outside' }, 0, source), null);
    assert.equal(lyricSeekPosition({ atMs: null, text: 'plain' }, 0, source), null);
    assert.equal(activeLyricLine({ ...document, format: 'text' }, 2, 0), -1);
});
test('validates song, viewer fence, playback generation, source and ordered timeline', () => {
    assert.equal(parseLyricsView(view(), scope, target).selection.resource?.id, 'lyric');
    for (const field of ['serverId', 'libraryId', 'itemId', 'sessionId', 'sessionGeneration', 'viewerFence']) {
        const raw = structuredClone(view());
        (raw.scope as Record<string, unknown>)[field] = 'wrong';
        assert.throws(() => parseLyricsView(raw, scope, target));
    }
    const wrong = structuredClone(view());
    wrong.selection.resource.document.lines[1].atMs = 12000;
    assert.equal(parseLyricsView(wrong, scope, target).selection.resource?.document.lines.length, 2);
    wrong.selection.resource.document.lines[1].atMs = 11999;
    assert.throws(() => parseLyricsView(wrong, scope, target));
    assert.throws(() => parseLyricsView(view(), scope, { ...target, sourceId: 'different' }));
});
test('late transport responses cannot overwrite a newer view or poison its viewer fence', async () => {
    const first = deferred<unknown>(), second = deferred<unknown>();
    let n = 0;
    const service = new LyricsService(api(() => ++n === 1 ? first.promise : second.promise), scope, target);
    const a = service.refresh(), b = service.refresh();
    const newest = view();
    newest.scope.viewerFence = 'd'.repeat(64);
    second.resolve(newest);
    await b;
    first.resolve(view());
    await a;
    assert.equal(service.getSnapshot().data?.scope.viewerFence, 'd'.repeat(64));
    assert.equal(service.getSnapshot().error, '');
    service.dispose();
});
test('cancel clears protected text and fences a response even if abort is ignored', async () => {
    const pending = deferred<unknown>();
    const service = new LyricsService(api(() => pending.promise), scope, target);
    const work = service.refresh();
    service.cancel();
    pending.resolve(view());
    await work;
    assert.equal(service.getSnapshot().data, null);
    assert.equal(service.getSnapshot().busy, false);
    await service.upload({ scope: 'private', language: 'en', text: 'old', format: 'text' });
    assert.equal(service.getSnapshot().data, null);
    service.dispose();
});
test('choose Off and replacements carry exact source and CAS intent', async () => {
    const requests: {
        path: string;
        method?: string;
        body?: unknown;
    }[] = [];
    const service = new LyricsService(api(async (path, method, body) => { requests.push({ path, method, body }); return view(); }), scope, target);
    await service.refresh();
    await service.choose(null);
    assert.deepEqual(requests[1].body, { sourceId: 'asset', sourceVersion: source.version, sessionId: 'session', expectedSelectionRevision: 1, resourceId: '', revision: 0 });
    await service.upload({ resourceId: 'lyric', expectedRevision: 1, scope: 'private', language: 'en', format: 'text', text: 'Replacement' });
    assert.equal((requests[3].body as {
        expectedRevision: number;
    }).expectedRevision, 1);
    assert.equal((requests[3].body as {
        sourceVersion: string;
    }).sourceVersion, source.version);
    service.dispose();
});
test('authorization loss clears private view, preview and candidates', async () => {
    let denied = false;
    const service = new LyricsService(api(async () => { if (denied)
        throw Object.assign(new Error(), { status: 403 }); return view(); }), scope, target);
    await service.refresh();
    denied = true;
    await service.refresh();
    assert.equal(service.getSnapshot().data, null);
    assert.match(service.getSnapshot().error, /no longer access/);
    service.dispose();
});
test('provider search clears older results before a failed new search', async () => {
    const candidate = { id: 'candidate', language: 'und', format: 'text', provenance: revision.provenance, preview: 'Preview' };
    let fail = false;
    const service = new LyricsService(api(async (path) => { if (!path.endsWith('/search'))
        return view(); if (fail)
        throw new Error(); return { candidates: [candidate], warnings: [] }; }), scope, target);
    await service.refresh();
    await service.search('local', '', 'und');
    assert.equal(service.getSnapshot().candidates.length, 1);
    fail = true;
    await service.search('local', '', 'und');
    assert.equal(service.getSnapshot().candidates.length, 0);
    service.dispose();
});
test('bounded transport rejects excess bytes, HTML and invalid UTF8; preserves authorization status', async () => {
    const signal = new AbortController().signal;
    await assert.rejects(readLyricsResponse(new Response('{}', { headers: { 'Content-Type': 'text/html' } }), signal));
    await assert.rejects(readLyricsResponse(new Response('{}', { headers: { 'Content-Type': 'application/json', 'Content-Length': '99999999' } }), signal));
    await assert.rejects(readLyricsResponse(new Response(new Uint8Array([0xff]), { headers: { 'Content-Type': 'application/json' } }), signal));
    await assert.rejects(readLyricsResponse(new Response(JSON.stringify({ error: { code: 'unauthorized' } }), { status: 403, headers: { 'Content-Type': 'application/json' } }), signal), e => (e as {
        status: number;
    }).status === 403);
});
test('HttpLocalApi captures bearer and rejects a response after token replacement', async () => {
    const pending = deferred<Response>();
    let authorization = '';
    const client = new HttpLocalApi('https://example.test', 'token-a', async (_url, init) => { authorization = (init?.headers as Record<string, string>).Authorization; return pending.promise; });
    const request = client.requestLyrics('/v1/items/song/lyrics');
    client.setAccessToken('token-b');
    pending.resolve(new Response('{}', { headers: { 'Content-Type': 'application/json' } }));
    await assert.rejects(request, /old session/);
    assert.equal(authorization, 'Bearer token-a');
    await assert.rejects(client.requestLyrics('https://provider.test/lyrics'));
});

test('equal timestamps select the last line in document order, also after backward seeks', () => {
 const simultaneous = { ...document, lines: [
  { atMs: 12000, text: 'First voice' }, { atMs: 12000, text: 'Second voice' },
  { atMs: 12000, text: 'Translation' }, { atMs: 15000, text: 'Next' }
 ] };
 assert.equal(activeLyricLine(simultaneous, 2.499, 500, 10), -1);
 assert.equal(activeLyricLine(simultaneous, 2.5, 500, 10), 2);
 assert.equal(activeLyricLine(simultaneous, 5.5, 500, 10), 3);
 assert.equal(activeLyricLine(simultaneous, 2.5, 500, 10), 2);
});

// A song with no lyrics answers 404: the empty state ("No lyrics"), not "could not be loaded".
test('a 404 lyrics read is the empty state, not an error', async () => {
    const service = new LyricsService(api(async () => { throw Object.assign(new Error('not found'), { status: 404, code: 'not_found' }); }), scope, target);
    await service.refresh();
    const s = service.getSnapshot();
    assert.equal(s.error, '');
    assert.equal(s.data, null);
    assert.equal(s.busy, false);
});
