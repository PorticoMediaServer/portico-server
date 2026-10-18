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
