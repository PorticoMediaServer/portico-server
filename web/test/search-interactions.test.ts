/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn Search controls do what they say.
 *
 * Search is core flow 6 (type, see results, open one). The client-core search model has its own
 * tests; the screen's wiring had none. These tests drive the real component callbacks: the field,
 * its keyboard hand-off, the type chips, See all, the result cards, the recent-search chips, and
 * the empty, partial and failed states. No browser and no server: the shared interaction harness
 * runs hooks and effects and expands the screen's own components, so nested controls are reached.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import * as searchCore from '@core/search.ts';
import {SEARCH_CHIPS} from '@core/presentation/index.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, pause, textOf, tick, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);

type Entry = {id: string; kind: string; title: string; caption?: string; playback?: any; [key: string]: any};
const entry = (id: string, kind: string, title: string, extra: Partial<Entry> = {}): Entry => ({
  id, kind, title, caption: '', libraryId: 'library', posterUrl: `/${id}.jpg`, backdropUrl: '',
  duration: 0, progressSeconds: 0, watched: false, playback: {itemId: id, startSeconds: 0}, ...extra,
});
const section = (id: string, items: readonly Entry[], extra: any = {}) => ({
  id, title: id, entityKind: items[0]?.kind ?? 'movie', status: 'success', errorCode: '',
  items, totalCount: items.length, totalCountExact: true, hasMore: false, nextCursor: '', ...extra,
});
const snapshot = (patch: any = {}) => ({
  generation: 1, scope: {viewerId: 'viewer', serverId: 'server'}, query: null, phase: 'idle', data: null,
  groups: [], pagination: {cursor: null, canPrevious: false}, error: null, ...patch,
});

function fakeSearch(initial: any) {
  let state = initial;
  const listeners = new Set<() => void>();
  const calls = {select: [] as any[], next: [] as string[], previous: 0, retry: 0, refresh: 0, revalidate: 0, dispose: 0};
  const service = {
    subscribe: (listener: () => void) => { listeners.add(listener); return () => listeners.delete(listener); },
    getSnapshot: () => state,
    select: (q: string, options?: any) => { calls.select.push([q, options]); return Promise.resolve(); },
    next: (group: string) => { calls.next.push(group); return Promise.resolve(); },
    previous: () => { calls.previous++; return Promise.resolve(); },
    retry: () => { calls.retry++; return Promise.resolve(); },
    refresh: () => { calls.refresh++; return Promise.resolve(); },
    revalidate: () => { calls.revalidate++; return Promise.resolve(); },
    dispose: () => { calls.dispose++; },
  };
  const set = (next: any) => { state = next; for (const listener of listeners) listener(); };
  return {service, calls, set};
}

async function mount(options: {
  snapshot?: any; libraries?: any; recent?: string[]; remember?: boolean;
  clearHistory?: () => Promise<void>; readHistory?: () => Promise<any>;
} = {}) {
  const h = createRenderer();
  const fake = fakeSearch(options.snapshot ?? snapshot());
  const ui = ['Artwork', 'Button', 'Card', 'Chip', 'Chips', 'Grid', 'Icon', 'Inset', 'ListRow', 'Loading', 'Menu', 'Notice', 'Page', 'PageHeader', 'Row', 'Shelf', 'StateView', 'Text', 'TitleHero']
    .reduce<Record<string, string>>((given, name) => ({...given, [name]: name}), {});
  const opened: any[] = [];
  const navigated: any[] = [];
  const reads: string[] = [];
  const clears = {count: 0};
  const api = {};
  const app = await componentModule(new URL('../src/screens/search/Search.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); }, useRouter: () => ({buildLocation: ({to}: any) => ({href: to})}), useSearch: () => ({})},
    '../shared/NoLibraries': {NoLibraries: 'NoLibraries'},
    '../../app/content': {cardCaptionOf: (value: any) => value.caption ?? ''},
    '../../app/libraries': {useLibrariesContext: () => options.libraries ?? {items: [{id: 'library'}], loading: false}},
    '../../app/open': {useEntryHref: () => (value: any) => `/item/${value.id}`, useOpenEntry: () => (...args: any[]) => opened.push(args)},
    '../../app/session': {useSession: () => ({api})},
    '../../ui': ui,
    '../shared/Sections': {LoadingShelves: 'LoadingShelves'},
    './Search.module.css': {default: {}},
    '../../app/server-preferences': {useServerPreferences: () => ({value: (_key: string, fallback: any) => options.remember ?? fallback})},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', ErrorState: 'ErrorState', changedError: {code: 'changed', message: 'The library changed.', retryable: false}},
    '../../app/viewer-scope': {useViewerScope: () => ({viewerId: 'viewer', serverId: 'server'})},
    '../../app/freshness': {useFreshDocument: () => {}},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/quiet-refresh': {useRefreshRequiredRetry: () => false},
    '@core/search.ts': {
      ...searchCore,
      SearchService: class { constructor() { Object.assign(this, fake.service); } },
      readSearchHistory: () => { reads.push('read'); return options.readHistory ? options.readHistory() : Promise.resolve({entries: (options.recent ?? []).map(q => ({q, updatedAt: '2026-10-24T00:00:00Z', uses: 1}))}); },
      clearSearchHistory: () => { clears.count++; return options.clearHistory ? options.clearHistory() : Promise.resolve(); },
    },
  });
  const draw = () => tree(h.render(() => app.SearchScreen()));
  const rendered = draw();
  return {app, fake, draw, rendered: () => rendered, opened, navigated, reads, clears, h};
}

const field = (rendered: any[]) => one(rendered, 'input');
const chips = (rendered: any[]) => all(rendered, 'Chip');
const chipLabels = () => SEARCH_CHIPS.map(chip => t(chip.label));

test('the field, the type chips and the clear control are drawn, and Clear empties the field', async () => {
  const screen = await mount();
  let rendered = screen.rendered();
  const input = field(rendered);
  assert.equal(input.props['aria-label'], t('web.search.title'));
  assert.equal(input.props.placeholder, t('web.search.placeholder'));
  assert.equal(input.props.value, '');
  assert.equal(input.props.type, 'search');
  // Every drawn chip is on screen in the authored order, with All pressed before a search.
  assert.deepEqual(chips(rendered).map(chip => chip.props.label), chipLabels());
  assert.equal(chips(rendered)[0].props.pressed, true);
  assert.ok(chips(rendered).slice(1).every(chip => chip.props.pressed === false));
  // Nothing typed: no clear control. Typed: Clear appears, empties the field and returns focus to it.
  assert.equal(all(rendered, 'Button').filter(button => button.props['aria-label'] === t('web.search.clear')).length, 0);
  input.props.onChange({target: {value: 'dune'}});
  rendered = screen.draw();
  const clear = one(all(rendered, 'Button').filter(button => button.props['aria-label'] === t('web.search.clear')), 'Button');
  assert.equal(field(rendered).props.value, 'dune');
  screen.h.focused.length = 0;
  clear.props.onClick();
  rendered = screen.draw();
  assert.equal(field(rendered).props.value, '');
  assert.deepEqual(screen.h.focused, ['field']);
  assert.equal(all(rendered, 'Button').filter(button => button.props['aria-label'] === t('web.search.clear')).length, 0);
});

test('typing searches once after the pause, and Enter searches at once and records the search', async () => {
  const screen = await mount();
  field(screen.rendered()).props.onChange({target: {value: '  dune  '}});
  screen.draw();
  assert.deepEqual(screen.fake.calls.select, [], 'nothing is sent while the words are still being typed');
  await pause();
  assert.deepEqual(screen.fake.calls.select, [['dune', undefined]], 'one debounced search, trimmed, with every type');
  // Enter sends the search and records it for Recent searches; typing alone never records.
  field(screen.draw()).props.onKeyDown({key: 'Enter', preventDefault() {}});
  const sent = screen.fake.calls.select;
  assert.deepEqual(sent.at(-1), ['dune', {record: true}]);
  assert.ok(!sent.slice(0, -1).some(([, options]: any) => options?.record), 'no earlier keystroke recorded the word');
  // A failed search is not recorded either: only a valid one reaches the service with record.
  screen.fake.calls.select.length = 0;
  field(screen.draw()).props.onChange({target: {value: '!!!'}});
  field(screen.draw()).props.onKeyDown({key: 'Enter', preventDefault() {}});
  assert.ok(!screen.fake.calls.select.some(([, options]: any) => options?.record));
});

test('a query the server will not search is said in place and sent nowhere', async () => {
  for (const [value, problem] of [['!!!', 'web.search.tooShort'], ['one two three four five six seven eight nine', 'web.search.tooManyWords'], ['a'.repeat(129), 'web.search.tooLong']] as const) {
    const screen = await mount();
    field(screen.rendered()).props.onChange({target: {value}});
    screen.draw();
    await pause();
    const rendered = screen.draw();
    const status = all(rendered, 'Text').find(node => node.props.role === 'status');
    assert.ok(status, `a status line for ${value.slice(0, 12)}`);
    assert.equal(textOf(status), t(problem));
    assert.deepEqual(screen.fake.calls.select.at(-1), ['', undefined], 'the previous answer is not left on screen');
  }
});

test('a type chip searches that type, shows its count, and pages it', async () => {
  const movie = entry('m1', 'movie', 'Dune');
  const screen = await mount({snapshot: snapshot({phase: 'ready', query: {q: 'dune'}, groups: [section('movies', [movie], {totalCount: 12, nextCursor: 'more'})]})});
  field(screen.rendered()).props.onChange({target: {value: 'dune'}});
  let rendered = screen.draw();
  const movieChip = chips(rendered).find(chip => chip.props.label === t('search.chip.movies'))!;
  assert.equal(movieChip.props.count, 12);
  movieChip.props.onClick();
  rendered = screen.draw();
  assert.equal(chips(rendered).find(chip => chip.props.label === t('search.chip.movies'))!.props.pressed, true);
  assert.deepEqual(screen.fake.calls.select.at(-1), ['dune', 'movies'], 'the chip searches that type only');
  // The group page counts what it shows and pages with the drawn Previous and Next.
  assert.ok(textOf(rendered).includes(t('web.search.someOf', {shown: 1, total: 12})));
  const next = all(rendered, 'Button').find(button => button.props.label === t('web.search.next'))!;
  const previous = all(rendered, 'Button').find(button => button.props.label === t('web.live.previous'))!;
  assert.equal(previous.props.disabled, true, 'nothing to go back to on the first page');
  next.props.onClick();
  assert.deepEqual(screen.fake.calls.next, ['movies']);
  // See all on a shelf is the same chip: it narrows to that type rather than leaving the screen.
  const allChip = chips(screen.draw()).find(chip => chip.props.label === t('search.chip.all'))!;
  allChip.props.onClick();
  rendered = screen.draw();
  const shelf = one(rendered, 'Shelf');
  assert.equal(shelf.props.action.label, t('web.search.seeAll'));
  shelf.props.action.onClick();
  rendered = screen.draw();
  assert.equal(chips(rendered).find(chip => chip.props.label === t('search.chip.movies'))!.props.pressed, true);
});

test('a result opens, plays and offers its menu; the top result has Play and Details', async () => {
  const show = entry('s1', 'show', 'The Last of Us');
  const movie = entry('m1', 'movie', 'Dune');
  const screen = await mount({snapshot: snapshot({phase: 'ready', query: {q: 'the last of us'}, groups: [section('movies', [movie]), section('shows', [show])]})});
  field(screen.rendered()).props.onChange({target: {value: 'the last of us'}});
  const rendered = screen.draw();
  // The top result is the title itself, with both drawn buttons.
  const top = rendered.find(node => node.type === 'section' && node.props['aria-label'] === t('search.topResult'));
  assert.ok(top, 'a top result section');
  const topPlay = tree(top).find(node => node.type === 'Button' && node.props.label === t('action.play'));
  assert.ok(topPlay, 'Play is drawn on the top result');
  const cards = all(rendered, 'Card');
  const card = cards.find(node => node.props.title === 'The Last of Us')!;
  card.props.onOpen();
  card.props.onPlay();
  card.props.onMore({x: 1, y: 2, width: 3, height: 4});
  assert.deepEqual(screen.opened.map(args => [args[0], args[1], args[2]]), [
    [show, undefined, undefined],
    [show, 'play', undefined],
    [show, 'more', undefined],
  ]);
});

test('the empty states are never a dead end: no libraries, before a search, and no matches', async () => {
  // No libraries at all: the screen says so rather than showing an empty search box.
  let screen = await mount({libraries: {items: [], loading: false}});
  assert.ok(screen.rendered().some(node => node.type === 'NoLibraries'));
  // Libraries but nothing typed and no history: the empty prompt, with the words from the catalogue.
  screen = await mount();
  let rendered = screen.rendered();
  assert.equal(one(rendered, 'StateView').props.title, t('web.search.emptyTitle'));
  assert.equal(one(rendered, 'StateView').props.body, t('web.search.emptyBody'));
  // Typed, answered, nothing found: say what happened, then what to try.
  screen = await mount({snapshot: snapshot({phase: 'ready', query: {q: 'casablanca'}, groups: []})});
  field(screen.rendered()).props.onChange({target: {value: 'casablanca'}});
  rendered = screen.draw();
  assert.equal(one(rendered, 'StateView').props.title, t('web.search.noMatches', {query: 'casablanca'}));
  assert.equal(one(rendered, 'StateView').props.body, t('web.search.noMatchesBody'));
  assert.equal(one(rendered, 'StateView').props.action, undefined);
  // Inside one chip: name the chip and offer the way back to everything.
  const movieChip = chips(rendered).find(chip => chip.props.label === t('search.chip.movies'))!;
  movieChip.props.onClick();
  rendered = screen.draw();
  const empty = one(rendered, 'StateView');
  assert.equal(empty.props.body, t('web.search.nothingIn', {group: t('search.chip.movies')}));
  assert.equal(empty.props.action.label, t('web.search.everything'));
  empty.props.action.onClick();
  rendered = screen.draw();
  assert.equal(chips(rendered)[0].props.pressed, true, 'Search everything returns to All');
});

test('a partial answer names the types that failed, and a failed search offers Try again', async () => {
  const failed = {...section('shows', [], {status: 'error', errorCode: 'search_group_timeout', items: []})};
  const screen = await mount({snapshot: snapshot({phase: 'ready', query: {q: 'dune'}, groups: [section('movies', [entry('m1', 'movie', 'Dune')]), failed]})});
  field(screen.rendered()).props.onChange({target: {value: 'dune'}});
  const notice = one(screen.draw(), 'Notice');
  assert.equal(notice.props.tone, 'warning');
  assert.ok(textOf(notice).includes(t('web.search.partial', {groups: t('search.chip.shows')})));
  notice.props.action.onClick();
  assert.equal(screen.fake.calls.refresh, 1);
  // Total failure: ErrorNotice with its retry going back to the service.
  const broken = await mount({snapshot: snapshot({phase: 'error', query: {q: 'dune'}, error: {code: 'request_failed', message: 'Search could not connect to this server. Try again.', retryable: true}})});
  field(broken.rendered()).props.onChange({target: {value: 'dune'}});
  const error = one(broken.draw(), 'ErrorNotice');
  error.props.retry();
  assert.equal(broken.fake.calls.retry, 1);
});

test('recent searches are the profile’s own: chosen, cleared, and put back when clearing fails', async () => {
  let fail = false;
  const screen = await mount({recent: ['dune', 'severance'], clearHistory: () => (fail ? Promise.reject(new Error('offline')) : Promise.resolve())});
  await tick();
  let rendered = screen.draw();
  assert.ok(textOf(rendered).includes(t('web.search.recent')));
  const recent = chips(rendered).filter(chip => chip.props.icon === 'clock');
  assert.deepEqual(recent.map(chip => chip.props.label), ['dune', 'severance']);
  // Clear removes them at once, and the server copy too; if the server refuses, they come back with a notice.
  fail = true;
  all(rendered, 'Button').find(button => button.props.label === t('web.search.clearRecent'))!.props.onClick();
  rendered = screen.draw();
  assert.equal(screen.clears.count, 1);
  await tick();
  rendered = screen.draw();
  assert.deepEqual(chips(rendered).filter(chip => chip.props.icon === 'clock').map(chip => chip.props.label), ['dune', 'severance']);
  assert.ok(textOf(rendered).includes(t('web.search.clearFailed')));
  // Choosing one fills the field and returns focus to it; the chips give way to its results.
  screen.h.focused.length = 0;
  chips(rendered).filter(chip => chip.props.icon === 'clock')[0].props.onClick();
  rendered = screen.draw();
  assert.equal(field(rendered).props.value, 'dune');
  assert.deepEqual(screen.h.focused, ['field']);
  // With Remember search history off, nothing is read and nothing is shown.
  const off = await mount({recent: ['dune'], remember: false});
  await tick();
  assert.deepEqual(off.reads, []);
  assert.ok(!textOf(off.draw()).includes(t('web.search.recent')));
});

test('ArrowDown leaves the field for the first result, including when nothing matched', async () => {
  const screen = await mount({snapshot: snapshot({phase: 'ready', query: {q: 'dune'}, groups: [section('movies', [entry('m1', 'movie', 'Dune')])]})});
  field(screen.rendered()).props.onChange({target: {value: 'dune'}});
  let prevented = 0;
  screen.h.focused.length = 0;
  field(screen.draw()).props.onKeyDown({key: 'ArrowDown', preventDefault() { prevented++; }});
  assert.equal(prevented, 1);
  assert.deepEqual(screen.h.focused, ['first-result']);
  // With no results the key is still handled in place: focus simply has nowhere to go.
  const empty = await mount();
  prevented = 0;
  field(empty.rendered()).props.onKeyDown({key: 'ArrowDown', preventDefault() { prevented++; }});
  assert.equal(prevented, 1);
});
