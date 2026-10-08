import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';
import * as agentModule from '../src/admin/metadata-agent.ts';

const load = () => componentModule(new URL('../src/screens/server/Libraries.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  '../../admin/metadata-agent': agentModule,
  '@tanstack/react-router': {useNavigate: () => () => {}, useSearch: () => ({})},
  '@core/library-management.ts': {}, '@core/library-inventory.ts': {},
  '../../admin/console': {}, '../../app/content': {}, '@core/presentation/index.ts': {},
  '../../app/session': {}, '../../app/viewer-scope': {}, '../../app/libraries': {libraryKindLabel: (kind: string) => ({movie: 'Movies', tv: 'TV Shows', anime: 'Anime', music: 'Music', audiobook: 'Audiobooks'} as Record<string, string>)[kind] ?? 'Library'},
  '../../app/i18n': {useI18n: () => ({t: (s: string) => s})}, '../../ui': {}, './Server': {},
  './LibrarySettings': {}, './MetadataLookup': {}, './MetadataSource': {},
}) as Promise<any>;

const serviceWith = (mutation: unknown) => ({
  getSnapshot: () => ({mutation, directory: {phase: 'ready', data: {items: []}}}),
});

test('Add library: a refused create stays open with the server error, not a silent close', async () => {
  const {libraryMutationError} = await load();
  const refused = libraryMutationError(serviceWith({phase: 'error', kind: 'create', error: {code: 'invalid_request', message: 'Bad path', retryable: false}}) as never);
  assert.ok(refused, 'a refusal surfaces');
  assert.equal(refused!.code, 'invalid_request', 'the presented error code travels through the existing error path');
  const ambiguous = libraryMutationError(serviceWith({phase: 'ambiguous', kind: 'create', error: {code: 'request_failed', message: 'Timeout', retryable: true}}) as never);
  assert.ok(ambiguous, 'an ambiguous outcome also stays open instead of closing');
  assert.equal(libraryMutationError(serviceWith({phase: 'complete', kind: 'create', error: null}) as never), null);
  assert.equal(libraryMutationError(serviceWith({phase: 'idle', kind: null, error: null}) as never), null);
});

test('Add library: the name defaults from the type and stays editable', async () => {
  const {defaultLibraryName} = await load();
  assert.equal(defaultLibraryName('movie'), 'Movies');
  assert.equal(defaultLibraryName('tv'), 'TV Shows');
  assert.equal(defaultLibraryName('anime'), 'Anime');
  assert.equal(defaultLibraryName('music'), 'Music');
  assert.equal(defaultLibraryName('audiobook'), 'Audiobooks');
});

test('Add library: the initial language follows the UI language, else the default, else the first', async () => {
  const {initialAgentLanguage} = await load();
  const option = (languages: string[], defaultLanguage?: string) => ({id: 'online', name: '', description: '', providers: [], languages, ...(defaultLanguage ? {defaultLanguage} : {})});
  assert.equal(initialAgentLanguage(option(['en', 'ja'], 'ja'), 'en'), 'en', 'the UI language wins when listed');
  assert.equal(initialAgentLanguage(option(['ja', 'de'], 'de'), 'fr'), 'de', 'otherwise the default');
  assert.equal(initialAgentLanguage(option(['ja', 'de']), 'fr'), 'ja', 'otherwise the first code');
  assert.equal(initialAgentLanguage(option([]), 'en'), '', 'no languages means no language');
  assert.equal(initialAgentLanguage(undefined, 'en'), '', 'no agent means no language');
});

// The dialog against a fake kind document: hook storage with working effects (the shared
// harness models useEffect as a no-op, so kind reads would never resolve there).
function effectHooks() {
  const slots: unknown[] = [];
  let cursor = 0;
  const effects: {deps: readonly unknown[] | undefined; cleanup: unknown}[] = [];
  let effectCursor = 0;
  const ref = (value: unknown) => {
    const n = cursor++;
    if (!(n in slots)) slots[n] = {current: value};
    return slots[n];
  };
  const createElement = (type: unknown, props: Record<string, unknown> | null, ...children: unknown[]) => ({type, props: {...(props ?? {}), children}});
  const react = {
    createElement,
    createContext: (value: unknown) => ({value, Provider: 'provider'}),
    useRef: ref,
    useState: (initial: unknown) => {
      const n = cursor++;
      if (!(n in slots)) slots[n] = typeof initial === 'function' ? (initial as () => unknown)() : initial;
      return [slots[n], (value: unknown) => { slots[n] = typeof value === 'function' ? (value as (p: unknown) => unknown)(slots[n]) : value; }];
    },
    useMemo: (fn: () => unknown) => fn(),
    useCallback: (fn: unknown) => fn,
    useEffect: (fn: () => unknown, deps?: readonly unknown[]) => {
      const n = effectCursor++;
      const prev = effects[n];
      if (!prev || !deps || !prev.deps || deps.some((v, i) => v !== (prev.deps as readonly unknown[])[i])) {
        if (typeof prev?.cleanup === 'function') (prev.cleanup as () => void)();
        effects[n] = {deps, cleanup: fn()};
      }
    },
    useSyncExternalStore: (_s: unknown, get: () => unknown) => get(),
    useContext: (context: {value: unknown}) => context.value,
  };
  return {react: {...react, default: react}, render: (fn: () => unknown) => { cursor = 0; effectCursor = 0; return fn(); }};
}

function nodes(tree: unknown): any[] {
  if (!tree || typeof tree !== 'object') return [];
  if (Array.isArray(tree)) return (tree as unknown[]).flatMap(nodes);
  // Dialog actions (and labels) ride on props other than children, so walk every prop value.
  const t = tree as {props?: Record<string, unknown>};
  return [tree, ...Object.values(t.props ?? {}).flatMap(nodes)];
}

const tick = async (n = 8) => { for (let i = 0; i < n; i++) await new Promise(r => setImmediate(r)); };

const movieDoc = () => ({libraryKind: 'movie', agents: [
  {id: 'online', name: 'Portico online metadata', description: 'Matches films online.', providers: ['tmdb'], languages: ['en', 'ja', 'de'], defaultLanguage: 'en'},
  {id: 'local', name: 'Local metadata only', description: 'Local files only.', providers: [], languages: []},
]});
const musicDoc = () => ({libraryKind: 'music', agents: [
  {id: 'online', name: 'Portico online metadata', description: 'Matches music online.', providers: ['musicbrainz'], languages: []},
  {id: 'local', name: 'Local metadata only', description: 'Local files only.', providers: [], languages: []},
]});

async function mountAddDialog({locale = 'en-US', docs = {movie: movieDoc(), music: musicDoc()} as Record<string, unknown>, failKinds = new Set<string>()}: {locale?: string; docs?: Record<string, unknown>; failKinds?: Set<string>} = {}) {
  const h = effectHooks();
  const requested: string[] = [];
  const names: Record<string, string> = {'language.en': 'English', 'language.ja': 'Japanese', 'language.de': 'German'};
  const api = {request: async (path: string) => {
    requested.push(path);
    const kind = decodeURIComponent(/\/v1\/library-kinds\/([^/]+)\/metadata-agents/.exec(path)?.[1] ?? '');
    if (failKinds.has(kind)) throw Object.assign(new Error('unavailable'), {status: 500, code: 'persistence_error'});
    const doc = docs[kind];
    if (!doc) throw Object.assign(new Error('bad kind'), {status: 400, code: 'invalid_request'});
    return doc;
  }};
  const items: {id: string; name: string; kind: string}[] = [];
  const createdInputs: unknown[] = [];
  const createdIds: (string | undefined)[] = [];
  let n = 0;
  const service = {
    getSnapshot: () => ({mutation: {phase: 'idle', kind: null, error: null}, directory: {phase: 'ready', data: {items}}, selected: {data: {library: {id: items.at(-1)?.id ?? 'none', lastScan: {id: 'job'}, actions: []}}}}),
    loadLibraries: async () => {},
    createLibrary: async (input: unknown) => { createdInputs.push(input); n++; items.push({id: `lib${n}`, name: (input as {name: string}).name, kind: (input as {kind: string}).kind}); },
    selectLibrary: async () => {},
  };
  const app = await componentModule(new URL('../src/screens/server/Libraries.tsx', import.meta.url), {
    react: h.react,
    '../../admin/metadata-agent': agentModule,
    '@tanstack/react-router': {useNavigate: () => () => {}, useSearch: () => ({})},
    '@core/library-management.ts': {}, '@core/library-inventory.ts': {},
    '../../admin/console': {problem: (e: unknown) => `problem:${(e as {code?: string})?.code ?? 'unknown'}`, useAction: () => ({busy: false, error: '', notice: '', run: async (fn: () => Promise<unknown>) => { await fn(); return true; }, clear: () => {}})},
    '../../app/content': {useService: () => ({})},
    '@core/presentation/index.ts': {iconFor: (k: string) => k},
    '../../app/session': {useSession: () => ({api})},
    '../../app/viewer-scope': {},
    '../../app/libraries': {libraryKindLabel: (kind: string) => ({movie: 'Movies', tv: 'TV Shows', anime: 'Anime', music: 'Music', audiobook: 'Audiobooks'} as Record<string, string>)[kind] ?? 'Library'},
    '../../app/i18n': {useI18n: () => ({t: (id: string) => names[id] ?? id, has: (id: string) => id in names, locale})},
    '../../ui': {Button: 'Button', Checkbox: 'Checkbox', Dialog: 'Dialog', Input: 'Input', Select: 'Select', Notice: 'Notice', Text: 'Text', Icon: 'Icon'},
    './LibrarySettings': {FolderPicker: 'FolderPicker', LibrarySettingsDialog: 'LibrarySettingsDialog'},
    './MetadataLookup': {LookupStatusRow: 'LookupStatusRow', useScreenLookup: () => ({})},
    './MetadataSource': {MetadataSourceChoice: 'MetadataSourceChoice'},
    './Server': {SectionHeader: 'SectionHeader'},
  }) as any;
  const render = () => h.render(() => app.AddLibraryDialog({open: true, onOpenChange: () => {}, service, onCreated: (id?: string) => { createdIds.push(id); }}));
  return {render, requested, createdInputs, createdIds};
}

const checkboxes = (tree: unknown) => nodes(tree).filter(n => n.type === 'Checkbox');
const select = (tree: unknown) => nodes(tree).find(n => n.type === 'Select');
const createButton = (tree: unknown) => nodes(tree).filter(n => n.type === 'Button').find(n => n.props.label === 'web.empty.addLibrary');
const folderInput = (tree: unknown) => nodes(tree).filter(n => n.type === 'Input').find(n => n.props.label === 'web.libraries.serverFolder');

test('Add library: movie with the online source sends the listed language', async () => {
  const m = await mountAddDialog();
  m.render();
  await tick();
  let tree = m.render();
  assert.ok(m.requested.includes('/v1/library-kinds/movie/metadata-agents'), 'the kind document is read');
  const sel = select(tree);
  assert.ok(sel, 'the language control shows for the online agent');
  assert.equal(sel.props.value, 'en', 'the UI language is selected when listed');
  assert.deepEqual(sel.props.options, [{value: 'en', label: 'English'}, {value: 'ja', label: 'Japanese'}, {value: 'de', label: 'German'}]);
  assert.deepEqual(checkboxes(tree).slice(5).map((c: any) => c.props.label), ['Portico online metadata', 'Local metadata only'], 'the source names come from the server');
  folderInput(tree).props.onChange({target: {value: '/media/movies'}});
  tree = m.render();
  await createButton(tree).props.onClick();
  await tick();
  assert.deepEqual(m.createdInputs, [{name: 'Movies', kind: 'movie', path: '/media/movies', metadataAgent: 'online', metadataLanguage: 'en'}]);
  assert.deepEqual(m.createdIds, ['lib1']);
});

test('Add library: the local source and music send no language', async () => {
  const m = await mountAddDialog();
  m.render();
  await tick();
  let tree = m.render();
  checkboxes(tree).slice(5)[1].props.onCheckedChange(true);
  tree = m.render();
  assert.equal(select(tree), undefined, 'the language control disappears for local');
  folderInput(tree).props.onChange({target: {value: '/media/local'}});
  tree = m.render();
  await createButton(tree).props.onClick();
  await tick();
  assert.deepEqual(m.createdInputs, [{name: 'Movies', kind: 'movie', path: '/media/local', metadataAgent: 'local'}]);
  checkboxes(tree).slice(0, 5)[3].props.onCheckedChange(true);
  tree = m.render();
  await tick();
  tree = m.render();
  assert.ok(m.requested.includes('/v1/library-kinds/music/metadata-agents'), 'the kind is re-read on change');
  assert.equal(select(tree), undefined, 'music has no language control');
  folderInput(tree).props.onChange({target: {value: '/media/music'}});
  tree = m.render();
  await createButton(tree).props.onClick();
  await tick();
  assert.equal(m.createdInputs.length, 2);
  assert.deepEqual(m.createdInputs[1], {name: 'Music', kind: 'music', path: '/media/music', metadataAgent: 'local'});
});

test('Add library: a failed choices read still creates without a language and shows no error', async () => {
  const m = await mountAddDialog({failKinds: new Set(['movie'])});
  m.render();
  await tick();
  const tree = m.render();
  assert.ok(m.requested.includes('/v1/library-kinds/movie/metadata-agents'), 'the read was attempted');
  assert.ok(nodes(tree).some(n => n.type === 'MetadataSourceChoice'), 'the form falls back to the fixed choices');
  assert.equal(select(tree), undefined, 'no language control without the server list');
  assert.equal(nodes(tree).filter(n => n.type === 'Notice').length, 0, 'no error is shown');
  folderInput(tree).props.onChange({target: {value: '/media/movies'}});
  const ready = m.render();
  await createButton(ready).props.onClick();
  await tick();
  assert.deepEqual(m.createdInputs, [{name: 'Movies', kind: 'movie', path: '/media/movies', metadataAgent: 'online'}]);
  assert.deepEqual(m.createdIds, ['lib1']);
});
