/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn saved-collection/playlist page makes the right calls.
 *
 * A playlist or personal collection: Play and Shuffle send the whole list as one container, remove
 * asks the service and offers its undo, a refused change is presented (and nothing pretends it
 * happened), a move sends only the moved entry and its new anchor (reading the order window first
 * when it no longer covers the entry), delete confirms before calling, and a playlist row plays
 * from that occurrence. The two services are stubbed so the tests see exactly what was called.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {moveRequest} from '@core/presentation/manual-order.ts';
import * as savedCore from '@core/saved.ts';
import * as personalCore from '@core/personal-saved.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);

const media = (id: string, title: string) => ({id, kind: 'movie', title, libraryId: 'library', posterUrl: `/${id}.jpg`, backdropUrl: '', duration: 6000, progressSeconds: 0, watched: false, playback: {itemId: id, startSeconds: 0}});
const entry = (id: string, title: string) => ({id, media: media(id, title)});
const occurrence = (id: string, title: string) => ({id, kind: 'playlist_entry', hidden: false, media: media(id, title)});

const collection = (patch: any = {}) => ({
  route: {view: 'resource', resourceId: 'c1'}, loading: false, error: null, mutationError: null, pending: false, retryPending: false,
  resources: [], entries: [entry('e1', 'Dune'), entry('e2', 'Arrival')],
  resource: {id: 'c1', kind: 'collection', name: 'Rewatch', summary: '', visibility: 'private', role: 'owner', actions: ['entries', 'update', 'delete', 'pin', 'reorder'], pinned: false, entryCount: 2, viewerFence: 'fence-1'},
  browse: null, revision: 1, viewerFence: 'fence-1', cursor: null, history: [], nextCursor: '', result: null, candidates: [], candidateCursor: '', libraries: [], ...patch,
});
const playlist = (patch: any = {}) => ({
  phase: 'ready', error: null, errorContext: '', projection: {sections: [{id: 'p', entries: [occurrence('e1', 'Dune'), occurrence('e2', 'Arrival'), occurrence('e3', 'Heat')], nextCursor: ''}]},
  pagination: {cursor: null, canPrevious: false}, playlist: {id: 'p1', name: 'Night', summary: 'Late films', entryCount: 3, pinned: false, revision: 2, viewerFence: 'fence-2', actions: ['reorder', 'remove', 'rename', 'delete', 'pin']},
  order: {revision: 2, viewerFence: 'fence-2', entryIds: ['e1', 'e2', 'e3']}, lastReceipt: null, ...patch,
});

async function mount(options: {playlist?: boolean; snapshot?: any; mutateResult?: boolean; mutateError?: any; undo?: any} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const played: any[] = [];
  const calls: {name: string; args: any[]}[] = [];
  let pinsChanged = 0;
  const snap = options.snapshot ?? (options.playlist ? playlist() : collection());
  // The page reads two services: a playlist's own list and a personal resource. Only the one on
  // screen carries the state under test; the other is an idle, empty binding.
  const idleSaved = {phase: 'idle', error: null, projection: null, pagination: {cursor: null, canPrevious: false}, playlist: null, order: null, lastReceipt: null};
  const idlePersonal = {route: null, loading: false, error: null, mutationError: null, entries: [], resources: [], resource: null, cursor: null, nextCursor: '', viewerFence: 'fence-idle', history: [], result: null, pending: false};
  const binding = (state: any, primary: boolean) => ({
    getSnapshot: () => state,
    select: (route: any) => { calls.push({name: 'select', args: [route]}); return Promise.resolve(); },
    mutate: (intent: any) => {
      calls.push({name: 'mutate', args: [intent]});
      if (!primary) return Promise.resolve(true);
      if (options.mutateError) state.mutationError = options.mutateError;
      if (options.undo) state.result = {deleted: false, undo: options.undo};
      if (options.undo && options.playlist) state.lastReceipt = {undoToken: options.undo.token, undoUntil: options.undo.until};
      return Promise.resolve(options.mutateResult ?? true);
    },
    loadOrder: () => { calls.push({name: 'loadOrder', args: []}); return Promise.resolve(); },
    refresh: () => { calls.push({name: 'refresh', args: []}); return Promise.resolve(); },
    retry: () => { calls.push({name: 'retry', args: []}); return Promise.resolve(); },
    next: () => { calls.push({name: 'next', args: []}); return Promise.resolve(); },
    revalidate: () => Promise.resolve(),
    dispose: () => {},
  });
  const savedService = binding(options.playlist ? snap : idleSaved, !!options.playlist);
  const personalService = binding(options.playlist ? idlePersonal : snap, !options.playlist);
  const app = await componentModule(new URL('../src/screens/saved/Resource.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {
      useParams: () => (options.playlist ? {kind: 'playlist', resourceId: 'p1'} : {kind: 'collection', resourceId: 'c1'}),
      useSearch: () => ({title: undefined}),
      useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); },
    },
    '../../app/content': {cardCaptionOf: (value: any) => value.caption ?? '', useService: (make: any) => { const made = make(); return {service: made, snapshot: made.getSnapshot()}; }},
    '../../app/back': {useBack: (fallback: any) => fallback},
    '@core/saved.ts': {...savedCore, SavedService: class { constructor() { Object.assign(this, savedService); } }},
    '@core/personal-saved.ts': {...personalCore, PersonalSavedService: class { constructor() { Object.assign(this, personalService); } }},
    '../../app/title-layout': {mosaicPaths: () => []},
    '../../app/open': {useEntryHref: () => (value: any) => `/item/${value.id}`, useOpenEntry: () => (...args: any[]) => calls.push({name: 'open', args})},
    '../../app/container-play': {
      sequenceContainer: (kind: string, id: string, extra: any = {}) => ({container: {kind, id}, ...extra}),
      savedSequence: (id: string, extra: any = {}) => ({saved: {id}, ...extra}),
      occurrenceSequence: (id: string, entryId: string) => ({playlist: {id}, occurrence: {entryId}}),
      groupSequence: (_group: any, extra: any = {}) => ({group: true, ...extra}),
    },
    './RemovalUndo': {RemovalUndoToast: 'RemovalUndoToast'},
    '../../player/PlayerContext': {usePlayerActions: () => ({playSequence: (...args: any[]) => played.push(args)})},
    '../../app/session': {useSession: () => ({api: {}})},
    '../shared/Sections': {LoadingGrid: 'LoadingGrid', SECTION_WINDOW_AT: 200, useAppendedPages: (_key: string, _cursor: any, page: readonly any[]) => page},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', errorText: () => 'That did not work.'},
    './Saved': {NameDialog: 'NameDialog'},
    '../../app/viewer-scope': {useViewerScope: () => ({viewerId: 'viewer', serverId: 'server'})},
    '../../app/freshness': {useFreshDocument: () => {}},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/group-browse': {acrossGroup: () => undefined, useGroupBrowse: () => ({pending: false, active: false, collection: undefined})},
    '../../app/libraries': {useLibrariesContext: () => ({items: []})},
    '../shared/GroupBrowse': {GroupBar: 'GroupBar', GroupGrid: 'GroupGrid', GroupRows: 'GroupRows'},
    '../../app/pins': {announcePinsChanged: () => { pinsChanged++; }},
    '../shared/ManualOrder': {ManualOrderList: 'ManualOrderList'},
    '../../ui': {
      AnchoredMenu: 'AnchoredMenu', Artwork: 'Artwork', Button: 'Button', Card: 'Card', ConfirmDialog: 'ConfirmDialog', Grid: 'Grid', Inset: 'Inset',
      ListRow: 'ListRow', Menu: 'Menu', MenuRow: 'MenuRow', Notice: 'Notice', Page: 'Page', PageHeader: 'PageHeader', Skeleton: 'Skeleton',
      StateView: 'StateView', Surface: 'Surface', Switch: 'Switch', WindowedGrid: 'WindowedGrid', anchorOf: () => ({x: 0, y: 0, width: 1, height: 1}), createStaticCollection: () => ({collection: {dispose: () => {}} }),
    },
  });
  h.render(() => app.SavedResourceScreen());
  const flat = () => tree(h.render(() => app.SavedResourceScreen()));
  return {h, flat, navigated, played, calls, snap, pinsChanged: () => pinsChanged};
}

const pageMenu = (rendered: any[], label: string) => all(rendered, 'Menu').find(menu => menu.props.label === label)!;

test('Play and Shuffle send the whole collection as the saved container', async () => {
  const screen = await mount();
  const buttons = all(screen.flat(), 'Button');
  buttons.find(button => button.props.icon === 'play')!.props.onClick();
  buttons.find(button => button.props.icon === 'shuffle')!.props.onClick();
  assert.deepEqual(screen.played.map(args => args[3]), [{saved: {id: 'c1'}}, {saved: {id: 'c1'}, shuffle: true}]);
  assert.deepEqual(screen.played[0][0].map((item: any) => item.id), ['e1', 'e2']);
});

test('removing an entry calls the service and offers its undo; Undo sends the token back', async () => {
  const undo = {token: 'tok-1', until: '2026-10-24T22:00:00Z'};
  const screen = await mount({undo});
  const card = all(screen.flat(), 'Card')[0];
  card.props.onMore({x: 0, y: 0, width: 1, height: 1});
  const remove = all(screen.flat(), 'MenuRow').find(row => row.props.label === t('web.saved.removeFromCollection'))!;
  remove.props.onSelect();
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.deepEqual(screen.calls.filter(call => call.name === 'mutate').map(call => call.args[0]), [{action: 'entries', removeEntryIds: ['e1']}]);
  const toast = one(screen.flat(), 'RemovalUndoToast');
  assert.deepEqual(toast.props.removal, {token: 'tok-1', until: '2026-10-24T22:00:00Z', title: 'Dune', from: 'collection'});
  await toast.props.onUndo(toast.props.removal);
  assert.deepEqual(screen.calls.filter(call => call.name === 'mutate').at(-1)!.args[0], {action: 'entries', undoToken: 'tok-1'});
});

test('a refused removal is presented, keeps the list, and offers no undo', async () => {
  const screen = await mount({mutateResult: false, mutateError: {code: 'conflict'}});
  const card = all(screen.flat(), 'Card')[0];
  card.props.onMore({x: 0, y: 0, width: 1, height: 1});
  all(screen.flat(), 'MenuRow').find(row => row.props.label === t('web.saved.removeFromCollection'))!.props.onSelect();
  await new Promise(resolve => setTimeout(resolve, 0));
  const rendered = screen.flat();
  assert.equal(one(rendered, 'RemovalUndoToast').props.removal, null, 'nothing to undo');
  // The refusal is said in place (the inline problem wins over the snapshot's mutation error).
  const notice = one(rendered, 'Notice');
  assert.equal(notice.props.tone, 'error');
  assert.ok(String(notice.props.children).includes('That did not work.'));
  assert.equal(all(rendered, 'ErrorNotice').length, 0);
  assert.equal(all(rendered, 'Card').length, 2, 'the entry is still listed');
});

test('a move sends only the moved entry and its new anchor', async () => {
  const screen = await mount();
  all(screen.flat(), 'Button').find(button => button.props.label === t('order.reorder'))!.props.onClick();
  const list = one(screen.flat(), 'ManualOrderList');
  assert.deepEqual(list.props.entries.map((item: any) => item.id), ['e1', 'e2']);
  await list.props.onMove(0, 1);
  const expected = moveRequest(['e1', 'e2'], 0, 1)!;
  assert.deepEqual(screen.calls.filter(call => call.name === 'mutate').at(-1)!.args[0], {action: 'reorder', entryIds: [expected.entryId], afterEntryId: expected.afterEntryId});
});

test('delete asks first, then calls the service and returns to the collection list', async () => {
  const screen = await mount();
  pageMenu(screen.flat(), t('web.saved.collectionOptions')).props.onSelect('delete');
  let dialog = one(screen.flat(), 'ConfirmDialog');
  assert.equal(dialog.props.open, true);
  assert.ok(!screen.calls.some(call => call.name === 'mutate'), 'nothing is deleted before the confirmation');
  dialog.props.onConfirm();
  await new Promise(resolve => setTimeout(resolve, 0));
  dialog = one(screen.flat(), 'ConfirmDialog');
  assert.deepEqual(screen.calls.filter(call => call.name === 'mutate').at(-1)!.args[0], {action: 'delete'});
  assert.deepEqual(screen.navigated.at(-1), {to: '/saved', search: {view: 'collections'}});
  assert.equal(dialog.props.open, false);
});

test('pinning calls the service and announces the change to the pins', async () => {
  const screen = await mount();
  pageMenu(screen.flat(), t('web.saved.collectionOptions')).props.onSelect('pin');
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.deepEqual(screen.calls.filter(call => call.name === 'mutate').at(-1)!.args[0], {action: 'pin', pinned: true});
  assert.equal(screen.pinsChanged(), 1);
});

test('a playlist row plays from that occurrence', async () => {
  const screen = await mount({playlist: true});
  const rows = all(screen.flat(), 'ListRow');
  rows[1].props.onClick();
  const [list, index, start, container] = screen.played.at(-1);
  assert.deepEqual(list.map((item: any) => item.id), ['e1', 'e2', 'e3']);
  assert.equal(index, 1);
  assert.equal(start, 0);
  assert.deepEqual(container, {playlist: {id: 'p1'}, occurrence: {entryId: 'e2'}});
});

test('a playlist move reads the order window first when it no longer covers the entry, then sends the move', async () => {
  const partial = playlist();
  partial.order = {revision: 2, viewerFence: 'fence-2', entryIds: ['e1']};
  const screen = await mount({playlist: true, snapshot: partial});
  const menu = all(screen.flat(), 'Menu').find(node => node.props.label === t('web.dvr.actionsFor', {title: 'Arrival'}))!;
  menu.props.onSelect('up');
  await new Promise(resolve => setTimeout(resolve, 0));
  const names = screen.calls.map(call => call.name);
  assert.ok(names.indexOf('loadOrder') >= 0 && names.indexOf('loadOrder') < names.lastIndexOf('mutate'), 'the order is read before the move');
  assert.deepEqual(screen.calls.filter(call => call.name === 'mutate').at(-1)!.args[0], {action: 'reorder', entryIds: ['e2'], afterEntryId: ''});
});
