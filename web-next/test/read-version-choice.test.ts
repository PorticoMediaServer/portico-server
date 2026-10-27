import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from '../../web/test/helpers/component-harness.mjs';
import {all, createRenderer, one, tree, textOf} from '../../web/test/helpers/interaction-harness.mjs';
import * as versionChoice from '../src/screens/read/read-version-choice.ts';
import {readBackCommit, readBackDestination, readRetireAttempt, readRouteEntry, readSameRouteEntry, readVersionAlternatives,
  readVersionCanChoose, readVersionPick, readVersionPostRetire, readVersionPreRetire, readVersionRecovery,
  type ReadRouteEntry, type ReadVersion} from '../src/screens/read/read-version-choice.ts';

const epub = {sourceId: 'epub-source', format: 'epub' as const, size: 900, inventoryAvailable: true};
const pdf = {sourceId: 'pdf-source', format: 'pdf' as const, size: 1200, inventoryAvailable: true};
const stale = {sourceId: 'stale-source', format: 'pdf' as const, size: 800, inventoryAvailable: false};
const route = {} as never;
const entry = (key: string, href = `/item/book?entry=${key}`): ReadRouteEntry => ({href, key});
const location = (value: ReadRouteEntry) => ({href: value.href, state: {__TSR_key: value.key}});

test('a rejected default EPUB leaves the available PDF choosable', () => {
  const versions = [epub, pdf];
  assert.equal(readVersionCanChoose(versions), true);
  const picked = readVersionPick(versions, epub.sourceId, pdf.sourceId);
  assert.equal(picked?.sourceId, pdf.sourceId);
  assert.deepEqual(readVersionAlternatives(versions, epub.sourceId).map(v => v.sourceId), [pdf.sourceId]);
});

test('unavailable, duplicate and unchanged versions cannot dispatch', () => {
  assert.equal(readVersionPick([epub, stale], epub.sourceId, stale.sourceId), undefined);
  assert.equal(readVersionPick([epub, pdf], epub.sourceId, epub.sourceId), undefined);
  assert.equal(readVersionPick([epub, pdf], epub.sourceId, undefined), undefined);
  assert.equal(readVersionPick([epub, pdf], epub.sourceId, 'absent-source'), undefined);
  assert.equal(readVersionPick([epub, {...pdf, sourceId: pdf.sourceId}], epub.sourceId, pdf.sourceId), undefined, 'duplicate ids refuse');
});

test('a one-version item shows no chooser', () => {
  assert.equal(readVersionCanChoose([epub]), false);
  assert.equal(readVersionCanChoose([]), false);
  assert.deepEqual(readVersionAlternatives([epub], epub.sourceId), []);
});

type Reentry = {
  /** Start a successor attempt from inside the abort listener. */
  successor(value: unknown): void;
  /** Replace the current route from inside the abort listener. */
  route(value: ReadRouteEntry): void;
  /** Replace the committed eligibility list with an equal-contents new list. */
  versions(value: readonly ReadVersion[]): void;
  readonly attemptB: {live: boolean; ready: boolean; rereading: boolean};
};

/** Drive the exact caller sequence the component uses: pre-retire admission, the
 * component's dead/detach/abort retirement, then the post-retire witness, and only
 * then the source write. `reenter` runs from the abort listener. */
function recover(reenter?: (set: Reentry) => void) {
  const versions = [epub, pdf];
  const opened = entry('entry-a');
  const attemptA = {live: true, ready: true, rereading: true, target: (() => 1) as unknown};
  const attemptB = {live: true, ready: true, rereading: false};
  let currentAttempt: unknown = attemptA, currentRoute: ReadRouteEntry | undefined = opened, sourceId: string = epub.sourceId;
  let committed: {context: unknown; sourceId: string; versions: readonly ReadVersion[]} = {context: route, sourceId: epub.sourceId, versions};
  const rendered = {context: route, sourceId: epub.sourceId, versions, attempt: attemptA};
  const admitted = readVersionRecovery({
    liveRoute: true, route: opened, committedRoute: opened, currentRoute,
    rendered, committed, currentAttempt, attemptLive: attemptA.live, candidate: pdf,
    retire: value => {
      assert.equal(value, attemptA, 'retirement may only target the attempt the row was rendered with');
      readRetireAttempt(attemptA, () => {if (currentAttempt === attemptA) currentAttempt = undefined;}, () => {
        // The abort listener must see the attempt already dead and detached.
        assert.equal(attemptA.live, false, 'dead is published before abort');
        assert.equal(currentAttempt, undefined, 'the attempt ref is cleared before abort');
        reenter?.({successor: value => {currentAttempt = value;}, route: value => {currentRoute = value;},
          versions: value => {committed = {...committed, versions: value};}, attemptB});
      });
    },
    afterRetire: () => ({liveRoute: true, committedRoute: opened, currentRoute, successor: currentAttempt, committed}),
  });
  if (admitted) sourceId = pdf.sourceId;
  return {admitted, sourceId, attemptA, attemptB};
}

test('caller-level failed A really selects B after the original is retired', () => {
  const result = recover();
  assert.equal(result.admitted, true);
  assert.equal(result.sourceId, pdf.sourceId, 'the caller wrote the chosen source');
  assert.equal(result.attemptA.live, false, 'the original attempt was retired');
});

test('an abort-listener successor blocks the pending source write', () => {
  const result = recover(set => {set.successor(set.attemptB);});
  assert.equal(result.admitted, false);
  assert.equal(result.sourceId, epub.sourceId);
  assert.equal(result.attemptB.live, true, 'the old callback never retired the successor');
});

test('an abort-listener route replacement blocks the pending source write', () => {
  const result = recover(set => {set.route(entry('entry-b'));});
  assert.equal(result.admitted, false);
  assert.equal(result.sourceId, epub.sourceId);
});

test('an abort-listener list replacement blocks the pending source write', () => {
  const result = recover(set => {set.versions([epub, pdf]);});
  assert.equal(result.admitted, false);
  assert.equal(result.sourceId, epub.sourceId);
});

test('a reused component accepts the new entry and refuses the old callback', () => {
  const versions = [epub, pdf];
  const oldEntry = entry('entry-old'), newEntry = entry('entry-new');
  const attemptA = {live: true, aborted: false};
  const rendered = {context: route, sourceId: epub.sourceId, versions, attempt: attemptA};
  const committed = {context: route, sourceId: epub.sourceId, versions};
  const base = {liveRoute: true, route: oldEntry, committedRoute: newEntry, currentRoute: newEntry,
    rendered, committed, currentAttempt: attemptA, attemptLive: true, candidate: pdf};
  // The old entry's callback is refused after the new entry committed, even though the
  // same mounted component and the same attempt continue.
  assert.equal(readVersionPreRetire(base), false);
  // The new entry's callback is the fresh committed lifetime and works.
  assert.equal(readVersionPreRetire({...base, route: newEntry}), true);
  // Back: old callback does nothing; the new callback decides normally.
  assert.equal(readBackCommit(oldEntry, newEntry, newEntry, true), undefined);
  assert.equal(readBackCommit(newEntry, newEntry, newEntry, true), 'back');
  assert.equal(readBackCommit(newEntry, newEntry, newEntry, false), 'home');
});

test('a retained row is refused after a successor, replacement or navigation', () => {
  const versions = [epub, pdf];
  const opened = entry('entry-a'), elsewhere = entry('entry-b');
  const attemptA = {live: true, aborted: false}, attemptB = {live: true, aborted: false};
  const rendered = {context: route, sourceId: epub.sourceId, versions, attempt: attemptA};
  const committed = {context: route, sourceId: epub.sourceId, versions};
  const base = {liveRoute: true, route: opened, committedRoute: opened, currentRoute: opened,
    rendered, committed, currentAttempt: attemptA, attemptLive: true, candidate: pdf};
  assert.equal(readVersionPreRetire(base), true);
  // A successor attempt exists: the old callback must not retire it.
  assert.equal(readVersionPreRetire({...base, currentAttempt: attemptB}), false);
  // The original was already retired.
  assert.equal(readVersionPreRetire({...base, attemptLive: false}), false);
  // The committed source or list was replaced (even with equal contents).
  assert.equal(readVersionPreRetire({...base, committed: {...committed, sourceId: pdf.sourceId}}), false);
  assert.equal(readVersionPreRetire({...base, committed: {...committed, versions: [...versions]}}), false);
  // Another route entry replaced the original, or the new entry committed.
  assert.equal(readVersionPreRetire({...base, currentRoute: elsewhere}), false);
  assert.equal(readVersionPreRetire({...base, committedRoute: elsewhere}), false);
  assert.equal(readVersionPreRetire({...base, liveRoute: false}), false);
  // Both attempts undefined never admits.
  assert.equal(readVersionPreRetire({...base, rendered: {...rendered, attempt: undefined}, currentAttempt: undefined}), false);
  // The row itself must still be the unique, available, non-current row.
  assert.equal(readVersionPreRetire({...base, candidate: epub}), false);
  assert.equal(readVersionPreRetire({...base, rendered: {...rendered, versions: [epub, stale]}}), false);
});

test('post-retire still refuses a successor or a replaced witness', () => {
  const versions = [epub, pdf];
  const opened = entry('entry-a'), elsewhere = entry('entry-b');
  const attemptA = {live: false, aborted: true};
  const rendered = {context: route, sourceId: epub.sourceId, versions, attempt: attemptA};
  const committed = {context: route, sourceId: epub.sourceId, versions};
  const base = {liveRoute: true, route: opened, committedRoute: opened, currentRoute: opened,
    rendered, committed, successor: undefined, candidate: pdf};
  assert.equal(readVersionPostRetire(base), true);
  assert.equal(readVersionPostRetire({...base, successor: {}}), false);
  assert.equal(readVersionPostRetire({...base, currentRoute: elsewhere}), false);
  assert.equal(readVersionPostRetire({...base, committedRoute: elsewhere}), false);
  assert.equal(readVersionPostRetire({...base, committed: {...committed, versions: [...versions]}}), false);
});

test('the route witness is the real __TSR_key entry, not history.location.key', () => {
  assert.deepEqual(readRouteEntry(location(entry('entry-a'))), entry('entry-a'));
  assert.equal(readRouteEntry({href: '/item/book', state: {}}), undefined);
  assert.equal(readRouteEntry({href: '/item/book', state: {__TSR_key: ''}}), undefined);
  assert.equal(readRouteEntry({href: '/item/book', state: {__TSR_key: 7 as never}}), undefined);
  assert.equal(readSameRouteEntry(entry('entry-a'), entry('entry-a')), true);
  assert.equal(readSameRouteEntry(entry('entry-a'), entry('entry-b')), false);
  assert.equal(readSameRouteEntry(entry('entry-a'), {...entry('entry-a'), href: '/other'}), false);
  assert.equal(readSameRouteEntry(undefined, entry('entry-a')), false);
});

test('Back returns in-app only when the captured route really has a predecessor', () => {
  assert.equal(readBackDestination(true), 'back');
  assert.equal(readBackDestination(false), 'home');
});

// ---------------------------------------------------------------------------
// Actual ReadItemEntry interaction regression. The published component harness
// runs the screen's own hooks/effects and expands its real element callbacks; it
// models callback composition, not native React timing or paint.
// ---------------------------------------------------------------------------

const Menu = (props: any) => props.open ? {type: 'Menu', props} : null;
const MenuGroup = (props: any) => ({type: 'MenuGroup', props});
const ReadBookHero = (props: any) => ({type: 'ReadBookHero',
  props: {...props, children: [props.children, props.actions, props.under].filter(Boolean)}});
const i18n = {t: (key: string) => key, bytes: (value: number) => `${value} B`, number: (value: number) => String(value),
  duration: (value: number) => `${value}s`, percent: (value: number) => `${value}%`};

function makeSession(sourceId: string) {
  const format = sourceId === pdf.sourceId ? 'pdf' : 'epub';
  return {manifest: {sourceId, revision: 'r1', format, size: 1000, canSaveFile: false, contents: [],
    metadata: {title: `Book ${sourceId}`, author: 'Author', coverUrl: '/cover.jpg', language: 'en'}},
    progress: async () => ({progress: 0})};
}

function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {resolve = res; reject = rej;});
  return {promise, resolve, reject};
}

async function mountRead(options: {canGoBack?: boolean; phone?: boolean; open?: (sourceId: string) => Promise<any>} = {}) {
  const h = createRenderer(), calls: string[] = [], openCalls: string[] = [];
  let current = true, canGoBack = options.canGoBack !== false, phone = options.phone !== false;
  const location = (key: string) => ({href: '/item/book', pathname: '/item/book', search: {}, hash: '', state: {__TSR_key: key}});
  const router = {state: {location: location('key-1')},
    history: {canGoBack: () => canGoBack, back: () => calls.push('history.back')},
    navigate: async () => {}, subscribe: () => () => {}};
  let context: any = {
    isCurrent: () => current,
    effects: {back: () => calls.push('back'), openHome: () => calls.push('home'), reportError: (error: unknown) => calls.push(`error:${String(error)}`)},
    artwork: {foreground: (path: string) => path},
    session: () => ({viewer: {authority: 'local', role: 'owner', serverId: 's', accountId: 'a', profileId: 'p'}}),
    preferences: {getSnapshot: () => ({}), subscribe: () => () => {}},
  };
  const captured: any = {context, binding: {home: {effects: {reportError: context.effects.reportError}}, scopeKey: 'scope'}};
  let screenContext = context;
  let eligibility = {itemId: 'book', versions: [epub, pdf]};
  const bindingsFactory = () => ({
    client: {},
    authority: {isCurrent: () => true, subscribe: () => () => {}},
    open: async ({sourceId}: {sourceId: string}) => {openCalls.push(sourceId); return options.open ? options.open(sourceId) : makeSession(sourceId);},
    close: async () => {}, dispose: async () => {},
  });
  const app = await componentModule(new URL('../src/screens/read/ReadItemEntry.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useLocation: () => router.state.location},
    '@i18n': {defaultI18n: i18n},
    '@design': {generatedCoverTint: () => ({first: '#111', second: '#222'})},
    '@core/index.ts': {ApiError: class ApiError extends Error {}},
    '@core/reading-reread.ts': {rereadReadingSource: async () => {}},
    '@core/metadata-repair.ts': {validateRepair: () => true},
    '../../../../packages/contracts/src/routes.ts': {encodedRoutePath: (path: string) => path},
    '@portico/reader-core': {createReadingPublication: async () => ({contentsTarget: (index: number) => ({index}), dispose() {}})},
    './contents-handoff': {stageContentsTarget: () => {}, retireContentsTarget: () => {}},
    '../../kit/reader/ReaderPanel': {ReaderContents: 'ReaderContents'},
    './save-file': {saveReadingFile: async () => {}},
    '../../route-registry-view': {useProtectedWebScreenContext: () => captured},
    '../../router': {router},
    '../../kit/Icon': {Icon: 'Icon'},
    '../../kit/controls': {Button: 'Button', ResumeButton: 'ResumeButton'},
    '../../kit/overlays': {Menu, MenuGroup, MenuItem: 'MenuItem'},
    '../../kit/overlays/shared': {usePhone: () => phone},
    '../detail/context': {runScreenCommand: (_context: any, command: () => void) => command()},
    '../../kit/states': {Skeleton: 'Skeleton', StateView: 'StateView'},
    '../../kit/reader/ReadBookHero': {ReadBookFacts: 'ReadBookFacts', ReadBookHero, ReadBookSections: 'ReadBookSections'},
    '../../kit/shell': {ServerMark: 'ServerMark'},
    '../browse/screen-context': {useLibraryScreenContext: () => screenContext},
    './ReadMetadata': {ReadMetadata: 'ReadMetadata'},
    './bindings': {createReadBindings: bindingsFactory},
    './navigation': {readDestination: (itemId: string) => `/read/${itemId}`},
    './read-version-choice': versionChoice,
    './read-item.module.css': {default: {}},
    './useReadingFavorite': {useReadingFavorite: () => undefined},
    './ReadSavedAction': {ReadSavedAction: 'ReadSavedAction'},
    './ReadMarkAction': {ReadMarkAction: 'ReadMarkAction'},
  });
  const flat = () => tree(h.render(() => app.ReadItemEntry({itemId: 'book', eligibility})));
  const settle = async (rounds = 12) => {for (let i = 0; i < rounds; i++) {await Promise.resolve(); flat();}};
  flat();
  return {
    flat, settle, calls, openCalls, router, context, captured,
    setLocation(key: string) {router.state.location = location(key);},
    setEligibility(next: any) {eligibility = next;},
    setContext(next: any) {context = next; captured.context = next; screenContext = next;},
    retire() {current = false;},
    setGoBack(value: boolean) {canGoBack = value;},
  };
}

const backButton = (f: any) => all(f.flat(), 'Button').find((node: any) => node.props['aria-label'] === 'next.states.back');
const versionButton = (f: any) => all(f.flat(), 'Button').find((node: any) => node.props['aria-label'] === 'next.reader.version');
const versionItem = (f: any, format: string) => all(f.flat(), 'MenuItem').find((node: any) => textOf(node) === format);

test('actual ReadItemEntry keeps one phone Back through loading, failure, ready and direct entry', async () => {
  const loading = await mountRead({open: () => new Promise(() => {})});
  await loading.settle(2);
  backButton(loading).props.onClick();
  assert.ok(loading.calls.includes('back'), 'loading Back reaches the captured back effect');

  const failure = await mountRead({open: async sourceId => {if (sourceId === epub.sourceId) throw new Error('rejected'); return makeSession(sourceId);}});
  await failure.settle();
  assert.equal(failure.calls.includes('back'), false);
  backButton(failure).props.onClick();
  assert.ok(failure.calls.includes('back'), 'failure Back reaches the captured back effect');

  const ready = await mountRead({open: async sourceId => makeSession(sourceId)});
  await ready.settle();
  backButton(ready).props.onClick();
  assert.ok(ready.calls.includes('back'), 'ready Back reaches the captured back effect');

  const direct = await mountRead({canGoBack: false, open: async sourceId => makeSession(sourceId)});
  await direct.settle();
  backButton(direct).props.onClick();
  assert.ok(direct.calls.includes('home'), 'direct entry uses the captured Home effect');
  assert.equal(direct.calls.includes('back'), false);
});

test('actual ReadItemEntry Back is inert after the same-book entry replacement, new Back works', async () => {
  const f = await mountRead({open: async sourceId => makeSession(sourceId)});
  await f.settle();
  const oldBack = backButton(f);
  const before = f.calls.length;
  f.setLocation('key-2'); f.flat(); await f.settle();
  oldBack.props.onClick();
  assert.equal(f.calls.length, before, 'old Back did nothing after the new entry');
  backButton(f).props.onClick();
  assert.ok(f.calls.slice(before).includes('back'), 'the new entry Back works');
});

test('actual ReadItemEntry failure chooser selects B and only B publishes', async () => {
  const f = await mountRead({open: async sourceId => {if (sourceId === epub.sourceId) throw new Error('default rejected'); return makeSession(sourceId);}});
  await f.settle();
  versionButton(f).props.onClick(); f.flat();
  versionItem(f, 'PDF').props.onSelect(); f.flat(); await f.settle();
  assert.deepEqual(f.openCalls, [epub.sourceId, pdf.sourceId]);
  const hero = one(f.flat(), 'ReadBookHero');
  assert.ok(String(hero.props.title).includes(pdf.sourceId), 'the ready hero is B');
  assert.equal(String(hero.props.title).includes(epub.sourceId), false, 'A never published');
});

test('actual ReadItemEntry ignores a stale A response after B opens', async () => {
  const a = deferred<any>();
  const f = await mountRead({open: sourceId => sourceId === epub.sourceId ? a.promise : Promise.resolve(makeSession(sourceId))});
  await f.settle(3);
  versionButton(f).props.onClick(); f.flat();
  versionItem(f, 'PDF').props.onSelect(); f.flat(); await f.settle();
  assert.deepEqual(f.openCalls, [epub.sourceId, pdf.sourceId]);
  a.resolve(makeSession(epub.sourceId)); await f.settle();
  const hero = one(f.flat(), 'ReadBookHero');
  assert.ok(String(hero.props.title).includes(pdf.sourceId), 'stale A publication cannot replace B');
});

test('actual ReadItemEntry refuses an old menu after entry, list and context replacement', async () => {
  const f = await mountRead({open: async sourceId => {if (sourceId === epub.sourceId) throw new Error('default rejected'); return makeSession(sourceId);}});
  await f.settle();
  versionButton(f).props.onClick(); f.flat();
  const oldEntryPdf = versionItem(f, 'PDF');
  f.setLocation('key-2'); f.flat(); await f.settle();
  oldEntryPdf.props.onSelect(); f.flat();
  assert.equal(f.openCalls.includes(pdf.sourceId), false, 'old entry menu cannot open B');
  const oldListPdf = versionItem(f, 'PDF');
  f.setEligibility({itemId: 'book', versions: [epub, pdf]}); f.flat();
  oldListPdf.props.onSelect(); f.flat();
  assert.equal(f.openCalls.includes(pdf.sourceId), false, 'old list menu cannot open B');
  const oldContextPdf = versionItem(f, 'PDF');
  f.setContext({...f.context, effects: {...f.context.effects, back: () => f.calls.push('back2'), openHome: () => f.calls.push('home2')}});
  f.flat(); await f.settle();
  oldContextPdf.props.onSelect(); f.flat();
  assert.equal(f.openCalls.includes(pdf.sourceId), false, 'old context menu cannot open B');
  versionItem(f, 'PDF').props.onSelect(); f.flat(); await f.settle();
  assert.ok(f.openCalls.includes(pdf.sourceId), 'the current entry/list/context menu opens B');
});

test('actual ReadItemEntry refuses a source write when an abort listener replaces the entry', async t => {
  const realAbort = globalThis.AbortController, controllers: AbortController[] = [];
  class RecordingAbort extends realAbort {constructor() {super(); controllers.push(this);}}
  globalThis.AbortController = RecordingAbort as any;
  t.after(() => {globalThis.AbortController = realAbort;});
  const f = await mountRead({open: async sourceId => {if (sourceId === epub.sourceId) throw new Error('default rejected'); return makeSession(sourceId);}});
  await f.settle();
  assert.ok(controllers.length >= 1);
  controllers[0]!.signal.addEventListener('abort', () => f.setLocation('key-abort'));
  versionButton(f).props.onClick(); f.flat();
  versionItem(f, 'PDF').props.onSelect(); f.flat();
  assert.equal(f.openCalls.includes(pdf.sourceId), false, 'the post-retire route witness refused the write');
});
