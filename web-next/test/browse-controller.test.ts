import test from 'node:test';
import assert from 'node:assert/strict';
import {BrowseController} from '../src/screens/browse/controller.ts';
const pivot = {id: 'movies', labelKey: 'library.movies', entityKinds: ['movie'], defaultSort: [{field: 'title', direction: 'asc'}], supportedViews: ['grid', 'list'], browsable: true, paging: 'offset'};
const caps = {library: {id: 'lib', name: 'Movies', kind: 'movie', defaultView: 'discover'}, pivots: [pivot], resolvedPivot: pivot,
 fields: [], sorts: [{id: 'title', labelKey: 'sort.title', directions: ['asc', 'desc'], defaultDirection: 'asc', applicableKinds: ['movie']}], quickFilters: [],
 queryLimits: {maximumDepth: 5, maximumClauses: 40, maximumBytes: 65536, maximumSorts: 3, defaultLimit: 60, maximumLimit: 100, cursorTtlMs: 1800000}};
const tick = () => new Promise<void>(resolve => setTimeout(resolve, 0));
async function settle() {for (let n = 0; n < 8; n++) await tick();}
function payload(start: number, total = 50_000, revision = 'revision-one') {
 return {pivot: 'movies', applied: {query: null, sort: [{field: 'title', direction: 'asc'}], seek: null},
 entries: Array.from({length: Math.min(60, total - start)}, (_, n) => ({id: `movie-${start + n}`, kind: 'movie', title: `Movie ${start + n}`, libraryId: 'lib', navigation: {view: 'item'}})),
 pageInfo: {start, total, totalIsLowerBound: false, revision, hasMore: start + 60 < total, nextCursor: ''}, positionIndex: [{key: 'Z', index: 49_980}]};
}
test('sparse distant windows never scan intermediate pages and later ranges carry the actual revision', async () => {
 const requests: {start: number; revision?: string}[] = [];
 const controller = new BrowseController({async request<T>(path: string, _method?: string, body?: unknown) {
  if (path.includes('capabilities')) return caps as T;
  const range = (body as {range: {start: number; revision?: string}}).range; requests.push(range);return payload(range.start) as T;
 }}, {libraryId: 'lib', pivot: 'movies'}, () => true);
 try {await controller.load();await settle();
  assert.equal(controller.collection.getSnapshot().total, 50_000);assert.equal(controller.collection.itemAt(0)?.entry.id, 'movie-0');
  controller.collection.ensureRange(49_980, 49_990);await settle();
  assert.equal(controller.collection.itemAt(49_980)?.entry.id, 'movie-49980');assert.ok(requests.length <= 5);
  assert.ok(requests.every(r => r.start < 120 || r.start > 49_800));assert.ok(requests.slice(1).every(r => r.revision === 'revision-one'));
  assert.ok(controller.collection.resident().items <= 360);
 } finally {controller.dispose();}
});
test('an authority change discards an in-flight result and disposal cancels requests', async () => {
 let current = true;let resolve!: (v: unknown) => void;let signal: AbortSignal | undefined;
 const controller = new BrowseController({async request<T>(path: string, _method?: string, _body?: unknown, provided?: AbortSignal) {
  if (path.includes('capabilities')) return caps as T;signal = provided;return await new Promise(r => {resolve = r;}) as T;
 }}, {libraryId: 'lib', pivot: 'movies'}, () => current);
 await controller.load();await tick();current = false;controller.dispose();assert.equal(signal?.aborted, true);resolve(payload(0));await tick();
 assert.equal(controller.collection.itemAt(0), undefined);
});
test('a different revision cannot enter the displayed collection', async () => {
 const controller = new BrowseController({async request<T>(path: string, _method?: string, body?: unknown) {
  if (path.includes('capabilities')) return caps as T;const start = (body as {range: {start: number}}).range.start;
  return payload(start, 500, start === 0 ? 'revision-one' : 'revision-two') as T;
 }}, {libraryId: 'lib', pivot: 'movies'}, () => true);
 try {await controller.load();await settle();assert.equal(controller.collection.itemAt(0)?.entry.id, 'movie-0');
  controller.collection.ensureRange(60, 65);await settle();assert.equal(controller.collection.itemAt(60), undefined);assert.equal(controller.collection.getSnapshot().status, 'error');
 } finally {controller.dispose();}
});
test('Home display pages, facet queries and batch counts keep the immutable source membership', async () => {
 const membership = {field: 'homeRow', operator: 'equals' as const, value: 'continue'};
 const scopedCaps = {...caps, library: {id: '', kind: '', name: ''}, fields: [
  {id: 'homeRow', labelKey: 'browse.field.homeRow', type: 'identity-set', operators: ['equals'], controlHint: 'text', applicableKinds: ['movie']},
  {id: 'library', labelKey: 'browse.field.library', type: 'enum', operators: ['in'], controlHint: 'facet-multi-select', applicableKinds: ['movie'], facetSource: {endpoint: '/ignored', field: 'library'}}],
  sorts: [...caps.sorts, {id: 'groupOrder', labelKey: 'sort.groupOrder', directions: ['asc'], defaultDirection: 'asc', requiresGroup: true, applicableKinds: ['movie']}]};
 const requests: {path: string; body: unknown}[] = [];
 const controller = new BrowseController({async request<T>(path: string, _method?: string, body?: unknown) {
  requests.push({path, body});
  if (path.includes('capabilities')) return scopedCaps as T;
  if (path.includes('facets')) return {field: 'library', total: 1, values: [{value: 'lib', label: 'Movies', count: 80}]} as T;
  if (path.includes('counts')) return {counts: [{id: 'draft', total: 80, totalIsLowerBound: false, revision: 'actual-count-revision'}]} as T;
  return payload((body as {range: {start: number}}).range.start, 80) as T;
 }}, {libraryId: '*', pivot: 'movies', homeRowId: 'continue', base: [membership], sort: {field: 'groupOrder', direction: 'asc'}}, () => true);
 try {await controller.load();await settle();const stop = new AbortController();
  await controller.facets('library', '', [], stop.signal);await controller.count([], stop.signal);
  const page = requests.find(r => r.path.endsWith('/browse'))!.body as {homeRowDisplay: string;query: unknown};
  assert.equal(page.homeRowDisplay, 'continue');assert.deepEqual(page.query, membership);
  const facet = requests.find(r => r.path.includes('facets'))!;assert.deepEqual(JSON.parse(new URL(facet.path,'https://local.invalid').searchParams.get('query')!), membership);
  const count = requests.find(r => r.path.includes('counts'))!.body as {queries: {query: unknown}[]};assert.deepEqual(count.queries[0].query, membership);
 } finally {controller.dispose();}
});
