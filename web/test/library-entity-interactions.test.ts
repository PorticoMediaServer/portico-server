/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn Library entity page makes the right calls.
 *
 * A collection, album or book page: the hero plays the whole list as its own container (or
 * resumes a book where it stopped), Shuffle asks the server for a shuffled container, a track row
 * plays on from that song, the owner's collection reorder sends the moved title and its new anchor
 * (reading the collection's revision first), Back returns to the library, and failures offer their
 * recovery. The content service, group browse and container state are stubbed.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {moveRequest} from '@core/presentation/manual-order.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);

const media = (id: string, title: string, patch: any = {}) => ({id, kind: 'movie', title, libraryId: 'library', posterUrl: `/${id}.jpg`, backdropUrl: '', duration: 6000, progressSeconds: 0, watched: false, playback: {itemId: id, startSeconds: 0}, ...patch});
const song = (id: string, title: string, index: number) => media(id, title, {kind: 'song', trackNumber: index + 1, duration: 200});
const entity = (patch: any = {}) => ({id: 'col1', kind: 'collection', title: 'Rewatch', libraryId: 'library', posterUrl: '/c.jpg', backdropUrl: '', actions: [], navigation: {view: 'collection', entityId: 'col1'}, ...patch});
const section = (id: string, entries: any[], patch: any = {}) => ({id, type: 'rail', heading: {key: id, fallback: id}, entries, totalCount: entries.length, nextCursor: '', ...patch});
const snapshot = (patch: any = {}) => ({phase: 'ready', sections: [], appended: [], projection: {entity: entity()}, error: null, ...patch});

async function mount(options: {owner?: boolean; view?: string; entity?: any; sections?: any[]; phase?: string; error?: any; groupActive?: boolean; groupTotal?: number} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const played: any[] = [];
  const requests: [string, string, any][] = [];
  const containerCalls: any[] = [];
  const sections = options.sections ?? [section('titles', [media('m1', 'Dune'), media('m2', 'Arrival')])];
  const snap = snapshot({phase: options.phase ?? 'ready', error: options.error ?? null, sections, appended: sections, projection: {entity: options.entity ?? entity()}});
  const collection = {itemAt: () => undefined, subscribe: () => () => {}, getSnapshot: () => ({total: 2})};
  const app = await componentModule(new URL('../src/screens/library/Entity.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {
      useParams: () => ({libraryId: 'library', view: options.view ?? 'collection', entityId: 'col1'}),
      useSearch: () => ({}),
      useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); },
    },
    '../../app/selection': {useSelectionRefresh: () => {}},
    '../../app/back': {useBack: (fallback: any) => fallback},
    '../../app/content': {useContent: () => ({snapshot: snap, retry: () => { requests.push(['', 'RETRY', undefined]); }, refresh: () => { requests.push(['', 'REFRESH', undefined]); }, next: () => {}})},
    '../../player/engine': {usePlayingItemId: () => undefined},
    '../../app/title-layout': {mosaicPaths: () => []},
    '../../app/library-words': {albumHeroFacts: () => [], artistHeroFacts: () => [], discGroups: () => undefined, trackSubtitleForAlbum: (value: string) => value},
    '../../app/open': {useOpenEntry: () => (...args: any[]) => requests.push(['', 'OPEN', args])},
    '../../app/container-play': {
      sequenceContainer: (kind: string, id: string, extra: any = {}) => ({container: {kind, id}, ...extra}),
      groupSequence: (_group: any, extra: any = {}) => ({group: true, ...extra}),
    },
    '../../player/PlayerContext': {usePlayerActions: () => ({play: (...args: any[]) => played.push(args), playSequence: (...args: any[]) => played.push(args), more: () => {}})},
    '../../app/session': {useSession: () => ({api: {request: (path: string, method: string, body?: any) => { requests.push([path, method, body]); return Promise.resolve(method === 'PUT' ? {revision: 2} : {revision: 1}); }}, owner: !!options.owner, administrator: false})},
    '../../app/downloads': {useDownloads: () => ({state: {unavailable: true}, service: null, ask: () => {}})},
    '../shared/container-watched': {containerKindFor: () => 'collection', useContainerWatched: () => ({state: {watched: false, favorite: false, watchlisted: false}, saving: false, carries: () => true, setFlag: (flag: string, value: boolean) => containerCalls.push([flag, value]), setWatched: (value: boolean) => containerCalls.push(['watched', value])})},
    '../shared/ManualOrder': {WindowedOrderList: 'WindowedOrderList'},
    '../shared/Sections': {LoadingShelves: 'LoadingShelves', SectionView: 'SectionView', sectionHeading: (_i18n: any, heading: any) => heading.fallback ?? heading.key},
    '../shared/GroupBrowse': {GroupBar: 'GroupBar', GroupGrid: 'GroupGrid', GroupRows: 'GroupRows'},
    '../../app/group-browse': {entityGroups: () => ({}), useGroupBrowse: () => ({pending: false, active: !!options.groupActive, total: options.groupTotal ?? 2, collection: options.groupActive ? collection : undefined, refresh: () => {}})},
    '../../app/libraries': {useLibrariesContext: () => ({items: [{id: 'library', kind: 'movie'}]})},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', ErrorState: 'ErrorState', changedError: {}},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/quiet-refresh': {useRefreshRequiredRetry: () => false},
    '../../ui': {Button: 'Button', Inset: 'Inset', KeyValue: 'KeyValue', ListRow: 'ListRow', Page: 'Page', Section: 'Section', Skeleton: 'Skeleton', StateView: 'StateView', Surface: 'Surface', Text: 'Text', TitleHero: 'TitleHero'},
  });
  h.render(() => app.EntityScreen());
  const flat = () => tree(h.render(() => app.EntityScreen()));
  return {h, flat, navigated, played, requests, containerCalls, snap};
}

const hero = (rendered: any[]) => one(rendered, 'TitleHero');

test('the hero plays the whole collection as its own container', async () => {
  const screen = await mount();
  hero(screen.flat()).props.primary.onClick();
  const [list, index, start, container] = screen.played.at(-1);
  assert.deepEqual(list.map((item: any) => item.id), ['m1', 'm2']);
  assert.equal(index, 0);
  assert.deepEqual(container, {container: {kind: 'collection', id: 'col1'}, shuffle: false});
});

test('Shuffle asks the server for a shuffled container, not the loaded page', async () => {
  const screen = await mount();
  const shuffle = all(screen.flat(), 'Button').find(button => button.props.icon === 'shuffle')!;
  shuffle.props.onClick();
  const [, , , container] = screen.played.at(-1);
  assert.deepEqual(container, {container: {kind: 'collection', id: 'col1'}, shuffle: true});
});

test('a book resumes where the viewer stopped', async () => {
  const book = entity({id: 'b1', kind: 'book', title: 'Piranesi', playback: {itemId: 'm1', startSeconds: 300}, duration: 6000});
  const screen = await mount({view: 'book', entity: book, sections: [section('parts', [media('m1', 'Part one')])]});
  const primary = hero(screen.flat()).props.primary;
  primary.onClick();
  assert.deepEqual(screen.played.at(-1).slice(0, 2), ['m1', 300]);
});

test('a track row plays on from that song in the album’s container', async () => {
  const songs = [song('s1', 'One', 0), song('s2', 'Two', 1), song('s3', 'Three', 2)];
  const album = entity({id: 'al1', kind: 'album', title: 'Record', artist: {id: 'ar1', name: 'Band'}});
  const screen = await mount({view: 'album', entity: album, sections: [section('songs', songs)]});
  const rows = all(screen.flat(), 'ListRow');
  assert.ok(rows.length >= 3);
  rows[1].props.onClick();
  const [list, index, start, container] = screen.played.at(-1);
  assert.deepEqual(list.map((item: any) => item.id), ['s2', 's3'], 'the rest of the list from the pressed song');
  assert.equal(index, 0);
  assert.deepEqual(container, {container: {kind: 'album', id: 'al1'}, shuffle: false});
});

test('an owner’s collection reorder reads the revision, then sends only the moved title', async () => {
  const screen = await mount({owner: true, groupActive: true});
  all(screen.flat(), 'Button').find(button => button.props.label === t('order.reorder'))!.props.onClick();
  const list = one(screen.flat(), 'WindowedOrderList');
  const request = moveRequest(['m1', 'm2'], 0, 1)!;
  await list.props.onMove(request);
  const writes = screen.requests.filter(entry => entry[1] === 'PUT');
  assert.equal(writes.length, 1);
  assert.deepEqual(writes[0][2], {expectedRevision: 1, itemIds: [request.entryId], afterItemId: request.afterEntryId});
  assert.equal(screen.requests.filter(entry => entry[1] === 'GET').length, 1, 'the revision is read first');
});

test('Back returns to the library, and a failed page offers its recovery', async () => {
  let screen = await mount();
  hero(screen.flat()).props.onBack();
  assert.deepEqual(screen.navigated.at(-1), {to: '/library/$libraryId', params: {libraryId: 'library'}, search: {}});
  screen = await mount({phase: 'error', error: {code: 'request_failed', message: 'The page could not load.', retryable: true}, sections: []});
  one(screen.flat(), 'ErrorState').props.retry();
  assert.deepEqual(screen.requests.at(-1), ['', 'RETRY', undefined]);
});
