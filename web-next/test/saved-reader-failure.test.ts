/** Prepared offline: actual SavedScreen/SavedLists split-reader callback wiring through
 * the published web/test componentModule + createRenderer. Controlled SavedService
 * snapshots; models callback composition, not native React paint. UNRUN. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from '../../web/test/helpers/component-harness.mjs';
import {all, createRenderer, one, tree, textOf} from '../../web/test/helpers/interaction-harness.mjs';

const route = {view: 'watchlist', sort: 'added', direction: 'desc'} as const;
const artwork = {foreground: () => undefined, ambient: () => undefined, backdrop: () => undefined};
const presentation = {view: 'grid' as const, cardWidth: 160, setView() {}, backdrops: false, backdrop: undefined, reducedMotion: true};
const section = (id: string, count: number) => ({id, heading: {label: id}, totalCount: count,
  entries: count ? [{kind: 'movie' as const, id: `${id}-entry`, title: id}] : [], nextCursor: null});
const projection = (sections: any[]) => ({sections, sorts: [], filters: [], query: {sort: 'added', direction: 'desc'}, actions: []});

type Shape = 'posters' | 'squares' | undefined;
function snapshot(shape: Shape, phase: 'idle' | 'loading' | 'ready' | 'error', sections: any[] = []) {
  return {phase, route: shape ? {...route, shape} : {...route}, projection: phase === 'ready' ? projection(sections) : null, error: phase === 'error' ? {code: 'failed'} : null};
}
function service(name: 'posters' | 'squares' | 'saved', shape: Shape, initial: any, calls: Record<string, number>) {
  let current = initial, next = initial;
  return {getSnapshot: () => current, subscribe: () => () => {},
    select: async () => {calls[name] = (calls[name] ?? 0) + 1; current = next;},
    revalidate: async () => {}, next: async () => {}, retry: async () => {}, retryRead: async () => {},
    set(value: any) {current = value;}, plan(value: any) {next = value;}};
}

async function mount(initial: {posters: any; squares: any}) {
  const h = createRenderer(), errors: unknown[] = [], calls: Record<string, number> = {};
  const posters = service('posters', 'posters', initial.posters, calls);
  const squares = service('squares', 'squares', initial.squares, calls);
  const saved = service('saved', undefined, snapshot(undefined, 'ready', [section('saved', 0)]), calls);
  const operations = {current: true, run: async (run: (signal: AbortSignal) => Promise<void>) => run(new AbortController().signal), dispose() {}};
  const runtime: any = {scope: {authority: 'local', serverId: 's', accountId: 'a', profileId: 'p', sessionFamilyId: 'f'},
    operations, posters, squares, saved, select: async () => {},
    summary: {subscribe: () => () => {}, getSnapshot: () => ({value: null, error: null}), refresh: async () => {}},
    personal: {subscribe: () => () => {}, getSnapshot: () => ({route: null, resources: []})},
    downloads: {}, installReadyDownloads() {}, dispose() {}};
  const effects = {openEntry() {}, playEntry: async () => {}, selectRoute() {}, openItem() {}, saveFile: async () => {},
    reportError(error: unknown) {errors.push(error);}};
  const app = await componentModule(new URL('../src/screens/saved/Saved.tsx', import.meta.url), {
    react: h.react,
    '@i18n': {defaultI18n: {t: (key: string) => key, has: () => false, number: (value: number) => String(value),
      bytes: (value: number) => `${value} B`, region: {uiLocale: 'en-US', formatLocale: 'en-US'}}},
    '@core/presentation/server-text.ts': {serverTextLabel: (_i18n: any, heading: any) => heading?.label ?? 'section'},
    '@core/presentation/card-shape.ts': {shapeFor: () => 'poster'},
    '@core/freshness-hub.ts': {dependsOn: {personal: () => 'personal', progress: () => 'progress', content: () => 'content', settings: () => 'settings'}},
    '../../kit/controls': {Button: 'Button', Chip: 'Chip', Tabs: 'Tabs', Switch: 'Switch'},
    '../../kit/browse': {FiltersButton: 'FiltersButton', SortMenu: 'SortMenu', ViewToggle: 'ViewToggle'},
    '../../kit/overlays': {MenuRow: 'MenuRow', Popover: 'Popover'},
    '../../kit/cards': {PlaylistMosaic: 'PlaylistMosaic', PosterCard: 'PosterCard', SquareCard: 'SquareCard', NewPlaylistCard: 'NewPlaylistCard'},
    '../../kit/artwork': {AmbientArt: 'AmbientArt', CoverMosaic: 'CoverMosaic'},
    '../../kit/Icon': {Icon: 'Icon'},
    '../../kit/layout': {WindowedGrid: 'WindowedGrid'},
    '../../kit/states': {Skeleton: 'Skeleton', StateView: 'StateView'},
    '../../platform/freshness': {useQuietRefresh: () => {}},
    '../../shell/viewport': {useShellWidth: () => 1440},
    '../home/model': {homeCardModel: (media: any) => ({title: media.title})},
    '../browse': {BrowseEntryCard: 'BrowseEntryCard'},
    '../browse/document-selection': {CatalogueSelectionEntry: 'CatalogueSelectionEntry', useCatalogueSelectionActive: () => false},
    './Downloads': {LocalDownloads: 'LocalDownloads', ServerDownloads: 'ServerDownloads'},
    './runtime.ts': {savedReaderRoute: (value: any) => value},
    './window.ts': {savedSectionWindow: () => ({collection: {}})},
    './text.ts': {savedText: (key: string) => key},
    './Resource': {SavedHistory: 'SavedHistory', SavedResourceScreen: 'SavedResourceScreen'},
    './ResourceFields': {ResourceFields: 'ResourceFields'},
    './SavedDialog': {SavedDialog: 'SavedDialog'},
    './SavedPhoneDirectory': {SavedPhoneDirectory: 'SavedPhoneDirectory'},
    './SavedPhoneNav': {SavedPhoneNav: 'SavedPhoneNav'},
    './route.ts': {sameSavedRoute: (left: any, right: any) => JSON.stringify(left) === JSON.stringify(right), savedViewKey: () => 'view'},
    './use-view-operations.ts': {useSavedViewOperations: () => operations},
    './saved.module.css': {default: {}},
    './ReadingGroupCard': {ReadingGroupCard: 'ReadingGroupCard'},
  });
  const flat = () => tree(h.render(() => app.SavedScreen({runtime, route, online: true, downloadsOffered: false,
    artwork, preparationArtwork: artwork, presentation, effects})));
  flat();
  return {flat, runtime, posters, squares, saved, calls, errors};
}

const compact = (f: any) => all(f.flat(), 'StateView').filter((node: any) => node.props.variant === 'compact');
const retryOf = (node: any) => node.props.primary;

test('actual SavedLists keeps the healthy poster rows beside a square reader failure', async t => {
  const savedGlobals = {ResizeObserver: (globalThis as any).ResizeObserver, window: (globalThis as any).window, devicePixelRatio: (globalThis as any).devicePixelRatio};
  class FakeResizeObserver {observe() {} disconnect() {} unobserve() {}}
  (globalThis as any).ResizeObserver = FakeResizeObserver;
  (globalThis as any).window = {innerHeight: 900};
  (globalThis as any).devicePixelRatio = 2;
  t.after(() => Object.assign(globalThis, savedGlobals));

  const f = await mount({posters: snapshot('posters', 'ready', [section('posters', 1)]), squares: snapshot('squares', 'error')});
  const failures = compact(f);
  assert.equal(failures.length, 1);
  assert.equal(retryOf(failures[0]!).id, 'retry-squares');
  assert.ok(all(f.flat(), 'WindowedGrid').some((node: any) => node.props.label === 'posters'), 'the healthy posters section stays mounted');
  assert.equal(textOf(f.flat()).includes('lib.count'), false, 'a partial read never claims a combined exact total');
});

test('actual SavedLists shows the empty state only when both readers are ready and empty', async t => {
  const savedGlobals = {ResizeObserver: (globalThis as any).ResizeObserver, window: (globalThis as any).window, devicePixelRatio: (globalThis as any).devicePixelRatio};
  class FakeResizeObserver {observe() {} disconnect() {} unobserve() {}}
  (globalThis as any).ResizeObserver = FakeResizeObserver;
  (globalThis as any).window = {innerHeight: 900};
  (globalThis as any).devicePixelRatio = 2;
  t.after(() => Object.assign(globalThis, savedGlobals));

  const partial = await mount({posters: snapshot('posters', 'loading'), squares: snapshot('squares', 'ready', [])});
  assert.equal(compact(partial).length, 0);
  assert.equal(all(partial.flat(), 'StateView').some((node: any) => node.props.title === 'web.saved.empty.watchlist'), false);
  partial.posters.set(snapshot('posters', 'ready', []));
  partial.flat();
  assert.ok(all(partial.flat(), 'StateView').some((node: any) => node.props.title === 'web.saved.empty.watchlist'), 'both ready and empty reaches empty');

  const inverse = await mount({posters: snapshot('posters', 'error'), squares: snapshot('squares', 'ready', [section('squares', 1)])});
  assert.equal(compact(inverse).length, 1);
  assert.equal(retryOf(compact(inverse)[0]!).id, 'retry-posters');
  assert.ok(all(inverse.flat(), 'WindowedGrid').some((node: any) => node.props.label === 'squares'), 'the healthy squares section stays mounted');
  assert.equal(textOf(inverse.flat()).includes('lib.count'), false);
});

test('actual SavedLists shows one scoped recovery per failed reader and coalesces retry', async t => {
  const savedGlobals = {ResizeObserver: (globalThis as any).ResizeObserver, window: (globalThis as any).window, devicePixelRatio: (globalThis as any).devicePixelRatio};
  class FakeResizeObserver {observe() {} disconnect() {} unobserve() {}}
  (globalThis as any).ResizeObserver = FakeResizeObserver;
  (globalThis as any).window = {innerHeight: 900};
  (globalThis as any).devicePixelRatio = 2;
  t.after(() => Object.assign(globalThis, savedGlobals));

  const f = await mount({posters: snapshot('posters', 'error'), squares: snapshot('squares', 'error')});
  const failures = compact(f);
  assert.equal(failures.length, 2);
  assert.deepEqual(failures.map((node: any) => retryOf(node).id).sort(), ['retry-posters', 'retry-squares']);
  f.posters.plan(snapshot('posters', 'ready', [section('posters', 1)]));
  f.squares.plan(snapshot('squares', 'ready', []));
  retryOf(failures[0]!).onPress();
  retryOf(failures[1]!).onPress();
  assert.deepEqual(f.calls, {posters: 1, squares: 1}, 'duplicate clicks issue one recovery per reader');
  await Promise.resolve(); f.flat();
  assert.equal(compact(f).length, 0, 'one explicit retry recovers both shapes');
  assert.ok(all(f.flat(), 'WindowedGrid').some((node: any) => node.props.label === 'posters'));
  assert.ok(textOf(f.flat()).includes('lib.count'), 'both ready restores the exact combined total');
});

test('an old failure retry cannot request into a successor route', async t => {
  const savedGlobals = {ResizeObserver: (globalThis as any).ResizeObserver, window: (globalThis as any).window, devicePixelRatio: (globalThis as any).devicePixelRatio};
  class FakeResizeObserver {observe() {} disconnect() {} unobserve() {}}
  (globalThis as any).ResizeObserver = FakeResizeObserver;
  (globalThis as any).window = {innerHeight: 900};
  (globalThis as any).devicePixelRatio = 2;
  t.after(() => Object.assign(globalThis, savedGlobals));

  const f = await mount({posters: snapshot('posters', 'error'), squares: snapshot('squares', 'error')});
  const oldRetry = retryOf(compact(f)[0]!);
  // The successor route commits; the old retained handler must see the replaced routes.
  f.posters.set({...snapshot('posters', 'error'), route: {view: 'favorites', sort: 'added', direction: 'desc', shape: 'posters'}});
  f.squares.set({...snapshot('squares', 'error'), route: {view: 'favorites', sort: 'added', direction: 'desc', shape: 'squares'}});
  f.flat();
  oldRetry.onPress();
  assert.deepEqual(f.calls, {}, 'the retired route callback requested nothing');
});
