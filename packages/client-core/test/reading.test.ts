import test from 'node:test';
import assert from 'node:assert/strict';
import {HttpLocalApi, ReadingError, type Viewer} from '../src/index.ts';
import {routePath, type PostReadingSessionsResponse} from '../../contracts/src/index.ts';

const rev = 'a'.repeat(64);
const viewer: Viewer = {accountId: 'account', profileId: 'profile', serverId: 'server', authority: 'local', role: 'member'};
const chapter = new TextEncoder().encode('<html xmlns="http://www.w3.org/1999/xhtml"><body>Original text.</body></html>');
const png = Uint8Array.from(Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zl1sAAAAASUVORK5CYII=', 'base64'));
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), {status, headers: {'Content-Type': 'application/json', ETag: '"' + rev + '"'}});
type Call = {url: URL; init: RequestInit; headers: Headers};
function setup(format: 'pdf' | 'epub' | 'cbz' = 'pdf') {
  const manifest: PostReadingSessionsResponse = {itemId: 'book', sourceId: 'asset', sessionId: 'session', revision: rev, format, size: 131072, metadata: {title: 'Stored title'}};
  if (format === 'epub') { manifest.resources = [{path: 'EPUB/chapter.xhtml', mime: 'application/xhtml+xml', size: chapter.length}]; manifest.spine = ['EPUB/chapter.xhtml']; }
  if (format === 'cbz') manifest.pageNames = ['page2.png', 'page10.png'];
  const calls: Call[] = [];
  let hook: ((call: Call) => Promise<Response> | Response | undefined) | undefined;
  let current = true;
  const fetcher: typeof fetch = async (input, init = {}) => {
    const call = {url: new URL(String(input)), init, headers: new Headers(init.headers)}; calls.push(call);
    const answer = hook?.(call); if (answer !== undefined) return answer;
    const path = call.url.pathname;
    if (path === routePath('post_reading_sessions')) return json(manifest);
    if (path === routePath('delete_reading_sessions_sessionId', {sessionId: 'session'})) return new Response(null, {status: 204});
    if (path === routePath('get_reading_sessions_sessionId_progress', {sessionId: 'session'})) return json(init.method === 'PUT' ? {progress: null, saved: false} : {progress: null});
    if (path === routePath('get_reading_sessions_sessionId_assets', {sessionId: 'session'})) return new Response(chapter, {headers: {'Content-Type': 'application/xhtml+xml', 'Content-Length': String(chapter.length), ETag: '"' + rev + '"'}});
    if (path === routePath('get_reading_sessions_sessionId_pages_pageIndex', {sessionId: 'session', pageIndex: 1})) return new Response(png, {headers: {'Content-Type': 'image/png', 'Content-Length': String(png.length), ETag: '"' + rev + '"'}});
    if (path === routePath('get_reading_sessions_sessionId_source', {sessionId: 'session'})) {
      if (init.method === 'HEAD') return new Response(null, {headers: {'Content-Type': 'application/pdf', 'Content-Length': String(manifest.size), 'Accept-Ranges': 'bytes', ETag: '"' + rev + '"'}});
      const match = /^bytes=(\d+)-(\d+)$/.exec(call.headers.get('Range') ?? ''); assert.ok(match);
      const start = Number(match[1]), end = Number(match[2]);
      return new Response(Uint8Array.from({length: end - start + 1}, (_, i) => (start + i) % 251), {status: 206, headers: {'Content-Type': 'application/pdf', 'Content-Length': String(end - start + 1), 'Content-Range': `bytes ${start}-${end}/${manifest.size}`, ETag: '"' + rev + '"'}});
    }
    throw new Error('Unexpected generated operation');
  };
  const api = new HttpLocalApi('http://127.0.0.1:39117', 'captured-token', fetcher);
  const client = api.reading({viewer, isCurrent: () => current});
  return {api, client, calls, manifest, hook(value: typeof hook) { hook = value; }, retire() { current = false; }};
}
const coded = (code: string) => (error: unknown) => error instanceof ReadingError && error.code === code;

test('reading capture is lazy, freezes identity and uses generated paths with exact PDF proof', async () => {
  const f = setup(); assert.equal(f.calls.length, 0);
  const session = await f.client.open({itemId: 'book', sourceId: 'asset'});
  assert.equal(session.manifest.metadata.author, undefined);
  assert.ok(Object.isFrozen(session.manifest.metadata)); assert.ok(Object.isFrozen(session));
  await session.headPDF();
  const body = await session.readRange({start: 65536, endExclusive: 131072});
  assert.equal(body.length, 65536); assert.equal(body[0], 65536 % 251);
  for (const call of f.calls) {
    assert.equal(call.url.origin, 'http://127.0.0.1:39117'); assert.equal(call.url.searchParams.has('apiKey'), false);
    assert.equal(call.headers.get('Authorization'), 'Bearer captured-token');
    assert.equal(call.init.redirect, 'error'); assert.equal(call.init.credentials, 'omit');
    if (call.init.method !== 'POST') assert.equal(call.headers.get('If-Match'), '"' + rev + '"');
  }
  await assert.rejects(session.readRange({start: 0, endExclusive: 65537}), coded('invalid_range'));
  await session.close(); assert.equal(session.signal.aborted, true);
  assert.equal(f.calls.at(-1)!.init.method, 'DELETE');
  await assert.rejects(session.readRange({start: 0, endExclusive: 5}), coded('cancelled'));
  f.client.dispose();
});

test('200, changed ETag, wrong total and short ranges are rejected, never whole-file buffered', async t => {
  for (const mismatch of ['200', 'etag', 'total', 'short'] as const) await t.test(mismatch, async () => {
    const f = setup(), session = await f.client.open({itemId: 'book', sourceId: 'asset'});
    let cancelled = 0;
    f.hook(() => new Response(new ReadableStream<Uint8Array>({start(c) { c.enqueue(new Uint8Array(mismatch === 'short' ? 4 : 5)); c.close(); }, cancel() { cancelled++; }}), {status: mismatch === '200' ? 200 : 206, headers: {'Content-Type': 'application/pdf', 'Content-Length': '5', 'Content-Range': `bytes 0-4/${mismatch === 'total' ? 99 : 131072}`, ETag: '"' + (mismatch === 'etag' ? 'b'.repeat(64) : rev) + '"'}}));
    await assert.rejects(session.readRange({start: 0, endExclusive: 5}), error => error instanceof ReadingError);
    assert.equal(session.signal.aborted, true); assert.ok(session.signal.reason instanceof ReadingError); if (mismatch !== 'short') assert.equal(cancelled, 1);
    f.client.dispose();
  });
});

test('only two PDF extents are active; request abort cancels a stalled stream without ending the book', async () => {
  const f = setup(), session = await f.client.open({itemId: 'book', sourceId: 'asset'});
  let cancelled = 0;
  f.hook(() => new Response(new ReadableStream<Uint8Array>({pull() { return new Promise(() => {}); }, cancel() { cancelled++; }}), {status: 206, headers: {'Content-Type': 'application/pdf', 'Content-Length': '5', 'Content-Range': 'bytes 0-4/131072', ETag: '"' + rev + '"'}}));
  const one = new AbortController(), two = new AbortController();
  const a = session.readRange({start: 0, endExclusive: 5}, one.signal), b = session.readRange({start: 0, endExclusive: 5}, two.signal);
  await assert.rejects(session.readRange({start: 0, endExclusive: 5}), coded('range_busy'));
  one.abort(); two.abort(); await Promise.all([assert.rejects(a, coded('cancelled')), assert.rejects(b, coded('cancelled'))]);
  assert.equal(cancelled, 2); assert.equal(session.signal.aborted, false);
  f.hook(undefined); assert.equal((await session.readRange({start: 0, endExclusive: 5})).length, 5);
  f.client.dispose();
});

test('authority retirement aborts a pending response and cancels its late body without new credentials', async () => {
  const f = setup(), session = await f.client.open({itemId: 'book', sourceId: 'asset'});
  let release!: (r: Response) => void, cancelled = 0;
  f.hook(() => new Promise<Response>(resolve => { release = resolve; }));
  const pending = session.readRange({start: 0, endExclusive: 5});
  f.api.setAccessToken('replacement-token');
  await assert.rejects(pending, coded('cancelled')); assert.equal(session.signal.aborted, true);
  release(new Response(new ReadableStream({cancel() { cancelled++; }}))); await Promise.resolve(); await Promise.resolve();
  assert.equal(cancelled, 1); const count = f.calls.length;
  await assert.rejects(session.progress(), coded('cancelled')); assert.equal(f.calls.length, count);
  assert.ok(f.calls.every(call => call.headers.get('Authorization') === 'Bearer captured-token'));
});

test('EPUB reads only original enumerated bytes; CBZ exposes actual page names and bounded image responses', async () => {
  const epub = setup('epub'), book = await epub.client.open({itemId: 'book', sourceId: 'asset'});
  assert.deepEqual(await book.readAsset('EPUB/chapter.xhtml'), chapter);
  assert.equal(epub.calls.at(-1)!.url.searchParams.get('path'), 'EPUB/chapter.xhtml');
  const count = epub.calls.length;
  await assert.rejects(book.readAsset('../secret'), coded('invalid_asset'));
  await assert.rejects(book.readRange({start: 0, endExclusive: 5}), coded('invalid_format')); assert.equal(epub.calls.length, count);
  const comic = setup('cbz'), pages = await comic.client.open({itemId: 'book', sourceId: 'asset'});
  const page = await pages.page(1, {width: 2048});
  assert.equal(page.name, 'page10.png'); assert.equal(page.index, 1); assert.equal(page.mime, 'image/png'); assert.deepEqual(page.bytes, png);
  await assert.rejects(pages.page(0, {width: 31}), coded('invalid_page'));
  await assert.rejects(pages.page(2, {width: 320}), coded('invalid_page'));
  comic.hook(() => new Response(png, {headers: {'Content-Type': 'image/png', 'Content-Length': String((16 << 20) + 1), ETag: '"' + rev + '"'}}));
  await assert.rejects(pages.page(1, {width: 32}), coded('response_size'));
  epub.client.dispose(); comic.client.dispose();
});

test('nullable progress and paused history stay explicit; CAS conflict does not retire a valid source', async () => {
  const f = setup(), session = await f.client.open({itemId: 'book', sourceId: 'asset'});
  const initial = await session.progress(); assert.equal(initial.progress, null); assert.deepEqual(Object.keys(initial), ['progress']);
  const paused = await session.saveProgress({expectedRevision: 0, location: {format: 'pdf', pageIndex: 0, progression: .1}}); assert.equal(paused.progress, null); assert.equal(paused.saved, false);
  f.hook(call => call.init.method === 'PUT' ? json({error: {code: 'revision_mismatch'}}, 412) : undefined);
  await assert.rejects(session.saveProgress({expectedRevision: 0, location: {format: 'pdf', pageIndex: 0, progression: .1}}), coded('revision_mismatch'));
  assert.equal(session.signal.aborted, false);
  assert.equal((await session.progress()).progress, null);
  f.retire(); const count = f.calls.length;
  await assert.rejects(session.progress(), coded('cancelled')); assert.equal(f.calls.length, count);
});
