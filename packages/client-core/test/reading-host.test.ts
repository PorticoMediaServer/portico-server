import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createReadingHost, connectReadingHost, type ReadingHostSession} from '../src/reading-host.ts';
import {parsePreferenceSnapshot} from '../src/preferences.ts';
import {ReadingError, type ReadingManifest} from '../src/reading.ts';
const preferences = parsePreferenceSnapshot(JSON.parse(readFileSync(new URL('../../../server/testdata/contracts/get-bootstrap.json', import.meta.url), 'utf8')).response.preferences);
function pair(format: 'pdf' | 'epub' | 'cbz' = 'pdf') {
  const life = new AbortController(), sent: string[] = [], calls: string[] = [];
  const payload = Uint8Array.from({length: 170003}, (_, i) => i % 251);
  const manifest: ReadingManifest = Object.freeze({sessionId: 'session', sourceId: 'asset', itemId: 'item', revision: 'a'.repeat(64), size: payload.length, format, metadata: {title: 'Actual title'},
    ...(format === 'epub' ? {resources: [{path: 'EPUB/chapter.xhtml', mime: 'application/xhtml+xml', size: payload.length}], spine: ['EPUB/chapter.xhtml']} : format === 'cbz' ? {pageNames: ['page.png']} : {})});
  let input: ((message: string) => void) | undefined, back = 0, settings = 0, current = true;
  const facade: ReadingHostSession = {manifest, signal: life.signal,
    async headPDF() {calls.push('head');},
    async readRange({start, endExclusive}, signal) {assert.equal(signal?.aborted, false); calls.push('range'); return payload.slice(start, endExclusive);},
    async readAsset() {calls.push('asset'); return payload.slice();},
    async page(index) {calls.push('page'); return {index, name: 'page.png', mime: 'image/png', bytes: payload.slice()};},
    async progress() {return {progress: null};}, async saveProgress() {calls.push('save'); return {progress: null, saved: false};},
  };
  const host = createReadingHost({session: facade, channel: 'captured_unique_channel', preferences: () => preferences, initialProgress: {progress: null}, current: () => current,
    send(message) {sent.push(message); input?.(message);}, onBack() {back++;}, onSettings() {settings++;}});
  const connection = connectReadingHost({send: host.receive, subscribe(listener) {input = listener; return () => {input = undefined;};}});
  return {connection, host, life, calls, payload, sent, facade, get back() {return back;}, get settings() {return settings;}, retire() {current = false; host.dispose();}};
}
test('trusted bridge delivers actual source bytes in bounded chunks, metadata omission and confirmed preferences', async () => {
  for (const format of ['pdf', 'epub', 'cbz'] as const) {
    const f = pair(format), guest = await f.connection.ready;
    assert.equal(guest.session.manifest.metadata.author, undefined); assert.ok(Object.isFrozen(guest.session.manifest.metadata));
    let bytes: Uint8Array;
    if (format === 'pdf') {await guest.session.headPDF(); bytes = await guest.session.readRange({start: 100, endExclusive: 65000}); assert.deepEqual(bytes, f.payload.slice(100, 65000));}
    else if (format === 'epub') {bytes = await guest.session.readAsset('EPUB/chapter.xhtml'); assert.deepEqual(bytes, f.payload);}
    else {const page = await guest.session.page(0, {width: 320}); bytes = page.bytes; assert.equal(page.name, 'page.png'); assert.deepEqual(bytes, f.payload);}
    bytes[0] = 255; assert.equal(f.payload[0], 0);
    const chunks = f.sent.map(s => JSON.parse(s)).filter(m => m.kind === 'reply' && m.value?.bytes);
    assert.ok(chunks.length >= 2); assert.ok(chunks.every(m => m.value.bytes.length <= 65536));
    let confirmations = 0; const off = guest.preferences.subscribe(() => confirmations++);
    f.host.confirm(); assert.equal(confirmations, 1); assert.deepEqual(guest.preferences.getSnapshot(), preferences); off();
    await guest.openSettings(); await guest.back(); assert.equal(f.settings, 1); assert.equal(f.back, 1);
    const saved = await guest.session.saveProgress({expectedRevision: 0, location: format === 'epub' ? {format, progression: 0, cfi: 'epubcfi(/6/2!/4/1:0)'} : {format, progression: 0, pageIndex: 0}});
    assert.deepEqual(saved, {progress: null, saved: false});
    guest.dispose(); f.host.dispose();
  }
});
test('range bounds and wrong channel fail before any facade I/O; retirement aborts the structural source', async () => {
  const f = pair(), guest = await f.connection.ready;
  await assert.rejects(guest.session.readRange({start: 0, endExclusive: 65537}), (error: unknown) => error instanceof ReadingError && error.code === 'invalid_range');
  f.host.receive(JSON.stringify({v: 1, channel: 'another_channel_name', kind: 'request', id: 100, method: 'head'}));
  assert.deepEqual(f.calls, []);
  f.retire(); assert.equal(guest.session.signal.aborted, true);
  await assert.rejects(guest.session.headPDF(), /cancelled/);
  guest.dispose();
});
test('caller cancellation aborts the actual in-flight facade callback and releases its transfer', async () => {
  const f = pair('epub'); let aborted = false;
  f.facade.readAsset = async (_path, signal) => new Promise((_resolve, reject) => {
    signal!.addEventListener('abort', () => {aborted = true; reject(new ReadingError('cancelled'));}, {once: true});
  });
  const guest = await f.connection.ready, caller = new AbortController();
  const pending = guest.session.readAsset('EPUB/chapter.xhtml', caller.signal);
  await new Promise<void>(resolve => setImmediate(resolve)); caller.abort();
  await assert.rejects(pending, /cancelled/); assert.equal(aborted, true);
  guest.dispose(); f.host.dispose();
});
