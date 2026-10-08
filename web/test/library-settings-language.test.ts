/**
 * Edit library › Language (MU12): when the library's metadata agent lists languages, the
 * free-text input becomes a select; a stored tag from before the list (en-US) stays first
 * and selected so saving unchanged keeps it. Without a list, the free-text input stays.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

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
  const t = tree as {props?: Record<string, unknown>};
  return [tree, ...Object.values(t.props ?? {}).flatMap(nodes)];
}

const names: Record<string, string> = {'language.en': 'English', 'language.ja': 'Japanese'};
const i18n = {t: (id: string) => names[id] ?? id, has: (id: string) => id in names};

const docWithLanguage = (language: string) => ({
  revision: 1,
  settings: {
    allowMediaDeletion: false, trashRetentionDays: 30,
    providers: [{mediaKind: 'movie', provider: 'tmdb', apiKeySet: false, language, region: 'US'}],
    analysis: [], navigation: {trickplayIntervalSeconds: 5, trickplayTileWidth: 160, chapterThumbnailMode: 'embedded', videoPreviewEnabled: false, videoPreviewSeconds: 0},
  },
  analysisMatrix: {costClasses: [], operations: []},
  availableProviders: [{id: 'tmdb', name: 'TMDB', mediaKinds: ['movie'], supportsApiKey: false, supportsLocale: true, attributionNote: ''}],
  enumerations: {},
});

const agentWithLanguages = (languages: string[]) => ({
  data: {
    libraryId: 'lib', libraryKind: 'movie', revision: 2, agent: 'online',
    agents: [
      {id: 'online', name: 'Online', description: 'Online.', providers: ['tmdb'], languages, defaultLanguage: 'en'},
      {id: 'local', name: 'Local', description: 'Local.', providers: [], languages: []},
    ],
  },
  loading: false, reload: () => {},
});

async function mountSettings(doc: unknown, agent: unknown) {
  const h = effectHooks();
  const app = await componentModule(new URL('../src/screens/server/LibrarySettings.tsx', import.meta.url), {
    react: h.react,
    '@core/administration.ts': {},
    '../../admin/library-policy': {setLibraryAnalysis: (s: unknown) => s},
    '../../admin/console': {problem: (e: unknown) => `problem:${(e as {code?: string})?.code ?? 'unknown'}`, useAction: () => ({busy: false, error: '', notice: '', run: async (fn: () => Promise<unknown>) => { await fn(); return true; }, clear: () => {}}), useRead: () => ({data: doc, loading: false, reload: () => {}})},
    // The page's Save bar owns saving now; the form here only reports its state.
    '../settings/ServerForms': {useInlineForm: () => ({saving: false, error: undefined})},
    './operation-ids': {createOperationIds: () => ({forPayload: () => 'op', release: () => {}})},
    '../../app/session': {useSession: () => ({system: {id: 'srv'}, session: undefined, api: {request: async () => ({})}})},
    '../../app/i18n': {currentI18n: () => i18n},
    './MetadataLookup': {useScreenLookup: () => ({}), LookupStatusRow: 'LookupStatusRow'},
    './MetadataSource': {MetadataSourceGroup: 'MetadataSourceGroup', useMetadataAgent: () => agent},
    '../../ui': {Button: 'Button', Dialog: 'Dialog', Input: 'Input', ListRow: 'ListRow', Loading: 'Loading', Notice: 'Notice', Select: 'Select', SettingsGroup: 'SettingsGroup', SettingsRow: 'SettingsRow', Surface: 'Surface', Switch: 'Switch', Text: 'Text'},
  }) as any;
  // The draft copies the document in an effect, so the first pass runs the effects and the
  // second sees the form.
  const render = () => {
    h.render(() => app.LibrarySettingsSections({library: {id: 'lib', name: 'Movies'}, tab: 'metadata'}));
    return h.render(() => app.LibrarySettingsSections({library: {id: 'lib', name: 'Movies'}, tab: 'metadata'}));
  };
  return {render};
}

const languageSelect = (tree: unknown) => nodes(tree).filter(n => n.type === 'Select').find(n => n.props.label === 'web.librarySettings.language');
const languageInput = (tree: unknown) => nodes(tree).filter(n => n.type === 'Input').find(n => n.props.label === 'web.librarySettings.language');

test('Edit library: a stored en-US shows as "English (en-US)" and stays selected', async () => {
  const m = await mountSettings(docWithLanguage('en-US'), agentWithLanguages(['en', 'ja']));
  const tree = m.render();
  const sel = languageSelect(tree);
  assert.ok(sel, 'the language is a select when the agent lists languages');
  assert.equal(sel.props.value, 'en-US', 'the stored tag stays selected');
  assert.deepEqual(sel.props.options, [
    {value: 'en-US', label: 'English (en-US)'},
    {value: 'en', label: 'English'},
    {value: 'ja', label: 'Japanese'},
  ]);
  sel.props.onChange({target: {value: 'ja'}});
  const next = m.render();
  assert.equal(languageSelect(next).props.value, 'ja', 'choosing a listed language updates the draft');
});

test('Edit library: a listed stored language needs no extra option', async () => {
  const m = await mountSettings(docWithLanguage('ja'), agentWithLanguages(['en', 'ja']));
  const sel = languageSelect(m.render());
  assert.equal(sel.props.value, 'ja');
  assert.deepEqual(sel.props.options, [{value: 'en', label: 'English'}, {value: 'ja', label: 'Japanese'}]);
});

test('Edit library: without a listed language the free-text input stays', async () => {
  const m = await mountSettings(docWithLanguage('en-US'), agentWithLanguages([]));
  const tree = m.render();
  assert.equal(languageSelect(tree), undefined, 'no select without a list');
  const input = languageInput(tree);
  assert.ok(input, 'the free-text input stays');
  assert.equal(input.props.value, 'en-US');
});
