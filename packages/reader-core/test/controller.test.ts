import assert from 'node:assert/strict';
import {test} from 'node:test';
import {ReaderController} from '../src/controller.ts';
import type {ReaderAdapter, ReaderDocument, ReaderMount, ReaderView} from '../src/ports.ts';

type Prefs = {theme: string; textSize: number; twoPages: boolean};
const defaults: Prefs = {theme: 'dark', textSize: 5, twoPages: true};
const source = {id: 'lawful-local', revision: 'immutable-fixture-v1', size: 1, async read() { return new Uint8Array([1]); }};
const location = {format: 'epub' as const, cfi: 'epubcfi(/6/2!/4/2)', progression: 0};
const container = {clientWidth: 1180, clientHeight: 820} as HTMLElement;
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(r => {resolve = r;}); return {promise, resolve}; }
function fixture() {
  const seen: Readonly<Prefs>[] = [];
  let viewsDisposed = 0, docsDisposed = 0;
  let mount!: ReaderMount<Prefs>;
  const view: ReaderView<Prefs> = {
    async navigate() {}, async applyPreferences(p) {seen.push(p);}, async resize() {},
    getLocation() {return location;}, dispose() {viewsDisposed++;}
  };
  const document: ReaderDocument<Prefs> = {id: source.id, revision: source.revision, format: 'epub', metadata: {title: 'Actual local book'},
    toc: [{id: 'chapter-1', label: 'I', location}],
    async mount(options) {mount = options; return view;},
    async *search(_query, options) {
      for (let i = 0; i < 10; i++) { if (options.signal.aborted) throw new Error('aborted'); yield {id: String(i), excerpt: 'Actual quotation', location}; }
    }, dispose() {docsDisposed++;}
  };
  const adapter: ReaderAdapter<Prefs> = {format: 'epub', async open() {return document;}};
  const controller = new ReaderController({adapters: [adapter], preferences: defaults});
  return {controller, adapter, document, view, seen, get mount() {return mount;}, get viewsDisposed() {return viewsDisposed;}, get docsDisposed() {return docsDisposed;}};
}
test('live defaults update unoverridden fields; overrides clear on close and never mutate input', async () => {
  const f = fixture(); let publications = 0;
  const unsubscribe = f.controller.subscribe(() => publications++);
  const before = f.controller.getSnapshot(); assert.equal(before, f.controller.getSnapshot());
  await f.controller.open({source, format: 'epub', container});
  await f.controller.setOverride({textSize: 7});
  await f.controller.updatePreferences({theme: 'sepia', textSize: 2, twoPages: false});
  assert.deepEqual(f.controller.getSnapshot().preferences, {theme: 'sepia', textSize: 7, twoPages: false});
  assert.deepEqual(f.seen.at(-1), f.controller.getSnapshot().preferences);
  assert.deepEqual(defaults, {theme: 'dark', textSize: 5, twoPages: true});
  assert.ok(Object.isFrozen(f.controller.getSnapshot()));
  assert.equal(f.controller.getSnapshot(), f.controller.getSnapshot());
  await f.controller.close();
  assert.deepEqual(f.controller.getSnapshot().preferences, {theme: 'sepia', textSize: 2, twoPages: false});
  assert.deepEqual(f.controller.getSnapshot().overriddenKeys, []);
  assert.equal(f.viewsDisposed, 1); assert.equal(f.docsDisposed, 1); assert.ok(publications > 3);
  unsubscribe();
});
test('close during async open aborts the source and disposes the late document without mounting', async () => {
  const f = fixture(), pending = deferred<ReaderDocument<Prefs>>(); let signal!: AbortSignal, mounted = 0;
  f.document.mount = async () => {mounted++; return f.view;};
  f.adapter.open = async options => {signal = options.signal; return pending.promise;};
  const opening = f.controller.open({source, format: 'epub', container});
  await new Promise(resolve => setImmediate(resolve));
  await f.controller.close(); assert.ok(signal.aborted);
  pending.resolve(f.document); await opening;
  assert.equal(f.docsDisposed, 1); assert.equal(mounted, 0); assert.equal(f.controller.getSnapshot().status, 'closed');
});
test('overlapping open cannot adopt old callbacks or dispose a newer view', async () => {
  const first = fixture(), second = fixture(), pending = deferred<ReaderDocument<Prefs>>(); let calls = 0;
  first.adapter.open = async () => ++calls === 1 ? pending.promise : second.document;
  const a = first.controller.open({source, format: 'epub', container});
  await new Promise(resolve => setImmediate(resolve));
  await first.controller.open({source, format: 'epub', container});
  pending.resolve(first.document); await a;
  assert.equal(first.docsDisposed, 1); assert.equal(second.docsDisposed, 0); assert.equal(first.controller.getSnapshot().status, 'ready');
  await first.controller.close(); assert.equal(second.docsDisposed, 1);
});
test('changes arriving while mounting are applied before ready', async () => {
  const f = fixture(), pending = deferred<ReaderView<Prefs>>(); f.document.mount = async () => pending.promise;
  const opening = f.controller.open({source, format: 'epub', container});
  await new Promise(resolve => setImmediate(resolve));
  await f.controller.setOverride({theme: 'black'});
  await f.controller.updatePreferences({theme: 'light', textSize: 3, twoPages: false});
  pending.resolve(f.view); await opening;
  assert.deepEqual(f.seen.at(-1), {theme: 'black', textSize: 3, twoPages: false});
  await f.controller.close();
});
test('search limits results and is fenced by session close; stale locations are ignored', async () => {
  const f = fixture(); await f.controller.open({source, format: 'epub', container});
  const results = [];
  for await (const r of f.controller.search('quote', {signal: new AbortController().signal, limit: 2})) results.push(r);
  assert.equal(results.length, 2);
  const iter = f.controller.search('quote', {signal: new AbortController().signal, limit: 5})[Symbol.asyncIterator]();
  await iter.next(); await f.controller.close();
  await assert.rejects(iter.next());
  const snapshot = f.controller.getSnapshot(); f.mount.onLocation({...location, progression: 1});
  assert.equal(snapshot, f.controller.getSnapshot());
});
test('opening errors expose codes and no arbitrary exception text; cleanup covers view failure', async () => {
  const f = fixture(); f.adapter.open = async () => {throw new Error('private URL/token must not be snapshot copy');};
  await f.controller.open({source, format: 'epub', container});
  assert.equal(f.controller.getSnapshot().error, 'render_failed');
  assert.equal(f.controller.getSnapshot().status, 'error');
  assert.ok(!JSON.stringify(f.controller.getSnapshot()).includes('private URL'));
  const g = fixture(); await g.controller.open({source, format: 'epub', container});
  g.view.dispose = () => {throw new Error('failed renderer cleanup');};
  await assert.rejects(g.controller.close()); assert.equal(g.docsDisposed, 1);
});
