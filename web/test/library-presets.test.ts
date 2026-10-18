import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

// A library's presets on the web (`useLibraryPresets`), its own code with React's hooks modelled:
// one service per library and pivot (switching between two libraries of the same pivot reads the
// new one's, and a late answer for the old never shows), and opening the picker reads it again.
function miniReact() {
  const slots: any[] = []; let cursor = 0; const effects: (() => void)[] = [];
  const same = (a?: unknown[], b?: unknown[]) => !!a && !!b && a.length === b.length && a.every((v, i) => v === b[i]);
  const react = {
    useMemo: (fn: () => unknown, deps: unknown[]) => { const n = cursor++; if (!slots[n] || !same(slots[n].deps, deps)) slots[n] = {deps, value: fn()}; return slots[n].value; },
    useEffect: (fn: () => (() => void) | void, deps: unknown[]) => { const n = cursor++; const old = slots[n]; if (old && same(old.deps, deps)) return; slots[n] = {deps}; effects.push(() => { old?.cleanup?.(); slots[n].cleanup = fn() ?? undefined; }); },
    useSyncExternalStore: (_: unknown, get: () => unknown) => get(),
    useState: (v: unknown) => [v, () => {}],
  };
  return {react: {...react, default: react}, render<T>(fn: () => T): T { cursor = 0; const out = fn(); effects.splice(0).forEach(e => e()); return out; }};
}
const settle = async () => { for (let i = 0; i < 6; i++) await new Promise(setImmediate); };
const view = (id: string, libraryId: string) => ({serverId: 'server', viewerFence: 'fence', id, kind: 'view', name: id, summary: '', visibility: 'private', revision: 1, role: 'owner', entryCount: 0, actions: ['pin', 'update', 'delete'], status: 'ready', invalidComponents: [], pinned: false, pinRevision: 0, shares: [], definition: {libraryId, pivot: 'shows', query: null, sort: [{field: 'title', direction: 'asc'}], presentation: 'grid'}});
const page = (resources: unknown[]) => ({serverId: 'server', viewerFence: 'fence', revision: 1, resources, nextCursor: ''});

test('presets: same-pivot library switch gets a service of its own; a late answer for the old never shows; opening reads again', async () => {
  const waiting = new Map<string, (v: unknown) => void>();
  let rows: Record<string, unknown[]> = {anime: [view('anime-1', 'anime')], tv: []};
  let quiet = false;
  const api = {request: (path: string) => { const lib = new URL('http://x' + path).searchParams.get('libraryId')!; return quiet ? Promise.resolve(page(rows[lib]!)) : new Promise(resolve => { waiting.set(lib, resolve); }); }};
  const r = miniReact();
  const session = {api, session: {serverId: 'server', viewer: {authority: 'local', accountId: 'account', profileId: 'profile'}}};
  const m = await componentModule(new URL('../src/screens/library/Presets.tsx', import.meta.url), {
    react: r.react, '../../app/session': {useSession: () => session}, '../../app/freshness': {useFreshDocument: () => () => {}},
    '@core/presentation/index.ts': {viewerScope: () => ({serverId: 'server', viewerId: '["local","account","profile"]'})},
  }) as any;
  let libraryId = 'anime';
  const draw = () => r.render(() => m.useLibraryPresets(libraryId, 'shows', true));
  const first = draw();
  libraryId = 'tv';
  const second = draw();
  assert.notEqual(second.service, first.service);
  waiting.get('tv')!(page(rows.tv!));
  await settle();
  waiting.get('anime')!(page(rows.anime!));
  await settle();
  assert.deepEqual(draw().snapshot.resources.map((x: {id: string}) => x.id), [], 'the old library\'s late answer does not show');
  // Saved on another device; opening the picker reads it again.
  rows = {...rows, tv: [view('from-phone', 'tv')]};
  quiet = true;
  draw().refresh();
  await settle();
  assert.deepEqual(draw().snapshot.resources.map((x: {id: string}) => x.id), ['from-phone']);
});

// ---- R4: applying, unconfirmed changes held as themselves, and paging -------------------------

const ready = (definition: object, id = 'p1', name = 'Mine') => ({serverId: 'server', viewerFence: 'fence', id, kind: 'view', name, summary: '', visibility: 'private', revision: 1, role: 'owner', entryCount: 0, actions: ['pin', 'update', 'delete'], status: 'ready', invalidComponents: [], pinned: false, pinRevision: 0, shares: [], definition});
async function destinations() {
  const browse = await componentModule(new URL('../src/app/browse.ts', import.meta.url), {}) as any;
  const m = await componentModule(new URL('../src/screens/library/preset-destination.ts', import.meta.url), {'../../app/browse': browse}) as any;
  return {presetDestination: m.presetDestination, decodePredicates: browse.decodePredicates};
}

test('applying a flat preset carries its filters in the address; one too long for the address opens the saved view whole, never the library unfiltered', async () => {
  const {presetDestination, decodePredicates} = await destinations();
  const capabilities = {fields: [{id: 'genre', operators: ['contains-any', 'in']}, {id: 'watched', operators: ['equals']}]};
  const sort = [{field: 'title', direction: 'asc'}];
  // R4's case: one leaf, 80 multiword values (within the contract's 100), past the address's 2,048 characters.
  const many = Array.from({length: 80}, (_, i) => `Genre number ${i} of many words`);
  const long = {field: 'genre', operator: 'contains-any', value: many};
  const big = presetDestination('movie', ready({libraryId: 'films', pivot: 'movies', query: long, sort, presentation: 'grid'}, 'big', 'Everything'), capabilities);
  assert.deepEqual(big, {to: 'saved', resourceId: 'big', title: 'Everything'}, 'the saved view itself, which runs its whole query');
  // An ordinary flat preset: the tab's controls, and the address decodes to exactly its filters.
  const flat = {all: [{field: 'genre', operator: 'contains-any', value: ['Drama']}, {field: 'watched', operator: 'equals', value: false}]};
  const small = presetDestination('movie', ready({libraryId: 'films', pivot: 'movies', query: flat, sort, presentation: 'grid'}), capabilities);
  assert.equal(small.to, 'library');
  assert.deepEqual([small.search.view, small.search.sort, small.search.direction], ['browse', 'title', 'asc']);
  assert.deepEqual(decodePredicates(small.search.filters), flat.all, 'the browse request gets every filter');
  // No query: the tab with no filter, which is that view.
  const none = presetDestination('movie', ready({libraryId: 'films', pivot: 'movies', query: null, sort: [{field: 'year', direction: 'desc'}], presentation: 'grid'}), capabilities);
  assert.deepEqual(none, {to: 'library', search: {view: 'browse', sort: 'year', direction: 'desc'}});
  // Nested, or a seeded shuffle: whole.
  assert.equal(presetDestination('movie', ready({libraryId: 'films', pivot: 'movies', query: {any: flat.all}, sort, presentation: 'grid'}), capabilities).to, 'saved');
  assert.equal(presetDestination('movie', ready({libraryId: 'films', pivot: 'movies', query: null, sort: [{field: 'random', direction: 'asc', seed: 7}], presentation: 'grid'}), capabilities).to, 'saved');
  // Needs review: nothing.
  assert.equal(presetDestination('movie', {...ready({}), status: 'needs-review', definition: undefined}, capabilities), undefined);
});

/** Every element in a rendered tree, through children and element-valued props (a Dialog's actions). */
function elements(node: unknown, out: any[] = []): any[] {
  if (!node || typeof node !== 'object') return out;
  if (Array.isArray(node)) { node.forEach(n => elements(n, out)); return out; }
  const el = node as {type?: unknown; props?: Record<string, unknown>};
  if (!('type' in el)) return out;
  out.push(el);
  for (const value of Object.values(el.props ?? {})) if (value && typeof value === 'object') elements(value, out);
  return out;
}
const text = (n: any) => [n?.props?.children].flat().join('');
const t = (id: string, values?: object) => (values ? `${id}:${JSON.stringify(values)}` : id);
/** The scoped list as the server serves it: 40 a page, cursors bound to the list's revision; writes answered or timed out. */
function presetApi(count: number) {
  const s = {rev: 1, deleted: new Set<string>(), writes: [] as {method: string; path: string; body: any}[], answer: 'ok' as 'ok' | 'timeout', hold: undefined as undefined | {resolve: (v: unknown) => void; reject: (e: unknown) => void}};
  const list = () => Array.from({length: count}, (_, i) => ready({libraryId: 'films', pivot: 'movies', query: null, sort: [{field: 'title', direction: 'asc'}], presentation: 'grid'}, `p${i}`, `View ${i}`)).filter(r => !s.deleted.has(r.id));
  const request = async (path: string, method = 'GET', body?: any) => {
    if (method === 'GET') {
      const cursor = new URL('http://x' + path).searchParams.get('cursor');
      let at = 0; if (cursor) { const [rev, offset] = cursor.split(':').map(Number); if (rev !== s.rev) throw Object.assign(new Error('changed'), {code: 'stale_continuation'}); at = offset!; }
      const all = list();
      return {serverId: 'server', viewerFence: 'fence', revision: s.rev, resources: all.slice(at, at + 40), nextCursor: at + 40 < all.length ? `${s.rev}:${at + 40}` : ''};
    }
    s.writes.push({method, path, body});
    const id = decodeURIComponent(path.split('/').at(-1)!);
    if (method === 'DELETE') s.deleted.add(id);
    s.rev++;
    if (s.hold) { const held = s.hold; s.hold = undefined; await new Promise((resolve, reject) => { held.resolve = resolve; held.reject = reject; }); }
    if (s.answer === 'timeout') throw Object.assign(new Error('The server took too long.'), {code: 'timeout'});
    return {serverId: 'server', viewerFence: 'fence', receipt: {operationId: body.operationId, resourceId: method === 'POST' ? 'new' : id, revision: s.rev, deleted: method === 'DELETE'}};
  };
  return Object.assign(s, {request});
}
async function presetsUi(api: ReturnType<typeof presetApi>) {
  const {hooks} = await import('./helpers/component-harness.mjs');
  const session = {api, session: {serverId: 'server', viewer: {authority: 'local', accountId: 'account', profileId: 'profile'}}};
  const common = {'../../app/session': {useSession: () => session}, '../../app/freshness': {useFreshDocument: () => {}}, '../../app/i18n': {useI18n: () => ({t})}, '../../app/errors': {errorText: (e: any) => e?.message ?? 'failed'},
    '@core/presentation/index.ts': {viewerScope: () => ({serverId: 'server', viewerId: '["local","account","profile"]'})}, '../../ui': new Proxy({}, {get: (_, name) => String(name)})};
  const hook = hooks(), dialog = hooks(), form = hooks();
  const m = await componentModule(new URL('../src/screens/library/Presets.tsx', import.meta.url), {react: hook.react, ...common}) as any;
  const d = await componentModule(new URL('../src/screens/library/Presets.tsx', import.meta.url), {react: dialog.react, ...common}) as any;
  const f = await componentModule(new URL('../src/screens/library/SaveView.tsx', import.meta.url), {react: form.react, ...common}) as any;
  let presets = hook.render(() => m.useLibraryPresets('films', 'movies', true));
  await presets.service.select({view: 'views', libraryId: 'films', pivot: 'movies'});
  return {
    get presets() { presets = hook.render(() => m.useLibraryPresets('films', 'movies', true)); return presets; },
    manage: () => elements(dialog.render(() => d.ManagePresetsDialog({open: true, onClose: () => {}, presets}))),
    save: (props: object) => elements(form.render(() => f.SaveViewDialog({open: true, onClose: () => {}, ...props}))),
  };
}
const settleAll = async () => { for (let i = 0; i < 12; i++) await new Promise(setImmediate); };

test('Manage: a delete with no answer is shown above the rows on any page, even once its row has gone; Try again sends that same delete, and nothing else is sent until then', async () => {
  const api = presetApi(90);
  const ui = await presetsUi(api);
  await ui.presets.service.next(); await ui.presets.service.next();
  void ui.presets;
  let tree = ui.manage();
  tree.find(n => n.type === 'Button' && n.props.label === 'lib.delete' && n.props.onClick && !n.props.variant?.includes('danger') && tree.indexOf(n) > tree.findIndex(x => x.type === 'Text' && text(x) === 'View 85'))!.props.onClick();
  void ui.presets;
  tree = ui.manage();
  api.answer = 'timeout';
  tree.find(n => n.type === 'Button' && n.props.variant === 'danger')!.props.onClick();
  await settleAll();
  void ui.presets;
  tree = ui.manage();
  const held = () => ui.manage().find(n => n.type === 'Notice' && n.props.tone === 'warning');
  assert.equal(text(held()), 'lib.savedViewHeldDelete:{"name":"View 85"}', 'what it was, and that it may already have happened');
  assert.equal(tree.some(n => n.type === 'Text' && text(n).startsWith('lib.deleteSavedView')), false, 'not a confirmation left open in a row');
  assert.ok(tree.filter(n => n.type === 'Button' && (n.props.label === 'lib.rename' || n.props.label === 'lib.delete')).every(n => n.props.disabled), 'no other change offered meanwhile');
  // Paged back to the first page, where the (deleted) row never was: still offered.
  await ui.presets.service.first();
  void ui.presets;
  assert.ok(held(), 'independent of the rows and the page');
  await assert.rejects(() => ui.presets.save('Another', {libraryId: 'films', pivot: 'movies', query: null, sort: [{field: 'title', direction: 'asc'}], presentation: 'grid'}), (e: any) => e.code === 'unconfirmed');
  assert.equal(api.writes.length, 1, 'a different change is not sent');
  api.answer = 'ok';
  held()!.props.action.onClick();
  await settleAll();
  void ui.presets;
  assert.equal(api.writes.length, 2);
  assert.deepEqual(api.writes[1], api.writes[0], 'the same delete, the same operation');
  assert.equal(held(), undefined, 'resolved');
});

test('Manage: a held change can be set aside, and then a new change is a new operation', async () => {
  const api = presetApi(3);
  const ui = await presetsUi(api);
  api.answer = 'timeout';
  await assert.rejects(() => ui.presets.rename(ui.presets.snapshot.resources[1], 'Renamed'));
  const notice = () => { void ui.presets; return ui.manage().find(n => n.type === 'Notice' && n.props.tone === 'warning'); };
  assert.equal(text(notice()), 'lib.savedViewHeldRename:{"name":"View 1","to":"Renamed"}');
  notice()!.props.secondaryAction.onClick();
  await settleAll();
  assert.equal(notice(), undefined, 'set aside');
  api.answer = 'ok';
  await ui.presets.rename(ui.presets.snapshot.resources[1], 'Again');
  assert.notEqual(api.writes[1].body.operationId, api.writes[0].body.operationId, 'a new operation');
});

test('Save view: closed while its save is on the way, the toolbar changed, then no answer: Save sends the held save as it was, not the toolbar\'s filters now', async () => {
  const api = presetApi(0);
  const ui = await presetsUi(api);
  // The library's own callback: the current filters, sort and presentation, read when it is called.
  let current = {libraryId: 'films', pivot: 'movies', query: {all: [{field: 'genre', operator: 'in', value: ['Drama']}]}, sort: [{field: 'title', direction: 'asc'}], presentation: 'grid'};
  const props = () => { const p = ui.presets; return {onSave: (name: string) => p.save(name, current), held: p.unconfirmed?.intent.action === 'create' ? p.unconfirmed.intent.name : undefined, onRetry: p.retryHeld, onDiscard: p.discard, describe: (e: any) => e.code}; };
  let form = ui.save(props());
  form.find(n => n.type === 'Input')!.props.onChange({target: {value: 'Dramas'}});
  form = ui.save(props());
  api.hold = {resolve: () => {}, reject: () => {}};
  const pending = api.hold;
  form.find(n => n.type === 'Button' && n.props.label === 'lib.saveView')!.props.onClick();
  await settleAll();
  // Dismissed while pending (X, Escape, the backdrop), then the toolbar changes.
  form.find(n => n.type === 'Dialog')!.props.onOpenChange(false);
  current = {...current, query: null as never, sort: [{field: 'year', direction: 'desc'}], presentation: 'list'};
  api.answer = 'timeout';
  pending.resolve(undefined);
  await settleAll();
  form = ui.save(props());
  assert.equal(form.find(n => n.type === 'Input')!.props.value, 'Dramas', 'reopened: the held name');
  assert.equal(form.find(n => n.type === 'Input')!.props.readOnly, true);
  api.answer = 'ok';
  form.find(n => n.type === 'Button' && n.props.label === 'lib.saveView')!.props.onClick();
  await settleAll();
  assert.equal(api.writes.length, 2, 'sent again');
  assert.deepEqual(api.writes[1].body, api.writes[0].body, 'as it was sent: its filters, sort and presentation, and its operation');
  assert.deepEqual(api.writes[1].body.definition.query, {all: [{field: 'genre', operator: 'in', value: ['Drama']}]});
  assert.equal(ui.presets.unconfirmed, undefined);
});
