// Browser regression fixture: real App/Shell/Home/PlaybackHost, synthetic service boundary.
import {createRoot} from 'react-dom/client';
import {App} from '../../src/App';
import {router} from '../../src/router';
import {createMemoryHistory} from '@tanstack/react-router';
import {HomeService} from '../../../packages/client-core/src/home-service';
import type {ShellBindings} from '../../src/shell/bindings';
import '../../src/kit/tokens.css';
import '../../src/kit/fonts.css';
import '../../src/bootstrap.css';

const proof = {requests: 0, plays: 0, more: 0, signOut: 0, errors: [] as string[]};
const viewer = {serverId: 'synthetic-server', authority: 'local', accountId: 'test-account', profileId: 'test-profile', role: 'owner'};
const document = {serverId: viewer.serverId, viewerFence: 'synthetic-viewer', spotlight: [],
  layout: {revision: 1, rowOrder: [], hiddenRowIds: []}, revision: {catalog: 1, viewer: 1}, generatedAt: new Date().toISOString(),
  rows: Array.from({length: 6}, (_, row) => ({id: `row-${row}`, viewerFence: 'synthetic-viewer', title: `Test row ${row + 1}`, kind: 'recent', artworkShape: 'poster',
    entries: Array.from({length: 8}, (_, index) => ({id: `movie-${row}-${index}`, kind: 'movie', title: `Test title ${row + 1}.${index + 1}`, libraryId: 'movies', available: true,
      navigation: {view: 'item', entityId: `movie-${row}-${index}`}, playback: {itemId: `movie-${row}-${index}`}, canSelect: true,
      homeActions: {play: true, queue: false, playlist: false, myList: false, watched: false, played: false, favorite: false, rating: false, download: false, collection: false, details: false, editMetadata: false, refreshMetadata: false, delete: false, state: {watchlisted: false, favorite: false, watched: false, rating: null, personalRevision: 0, containerRevision: 0}}})),
    total: 8, start: 0, limit: 20, hasMore: false, nextCursor: '', revision: {catalog: 1, viewer: 1}}))};
const api = {request: async () => {proof.requests++; return structuredClone(document);}, getRouteSnapshot: () => ({phase: 'ready'}), subscribeRoute: () => () => {}};
const service = new HomeService(api as any);
await service.load();
const bindings = {api, events: null, scopeKey: 'synthetic-viewer', shell: {
  identity: {viewer, profileName: 'Test profile', profiles: [], canManageProfiles: false, accountFacts: [],
    libraries: [{id: 'movies', name: 'Movies', kind: 'movies'}, {id: 'music', name: 'Music', kind: 'music'}], liveAvailable: true, unread: 0, showNotificationDot: false},
  OwnerActivity: () => null, dockVisible: false, onSignOut: () => {proof.signOut++;},
}, home: {service, artwork: {foreground: () => undefined, ambient: () => undefined},
  time: {locale: 'en-US', timeZone: 'UTC'}, reducedMotion: true,
  effects: {play: async () => {proof.plays++;}, playChannel: async () => {}, openEntry() {}, openSeeAll() {}, openMore: () => {proof.more++;}, setMyList: async () => {}, reportError: (error: unknown) => proof.errors.push(String(error))},
  selection: {toolbar: null, card: () => undefined, channel: () => undefined, collection: () => undefined, registerWindow: () => undefined, registerSpotlight: () => undefined, moreAvailable: () => true, moreExpanded: () => false}}, playback: undefined,
} as unknown as ShellBindings;
(window as any).sizeShellProof = {proof, bindings, service, viewer};
router.update({history: createMemoryHistory({initialEntries: ['/']})});
createRoot(window.document.getElementById('root')!).render(<App binding={bindings} />);
