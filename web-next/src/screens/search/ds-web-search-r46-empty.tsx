import {createRoot} from 'react-dom/client';
import type {HttpLocalApi} from '@core/index.ts';
import {FreshnessProvider} from '../../platform/freshness';
import {createArtworkRuntime} from '../../kit/artwork/source';
import type {ArtworkImage} from '../../kit/artwork/types';
import {SearchScreen} from './Search';
import {createSearchBindings} from './bindings';
import type {ScreenContext} from '../detail/context';
import '../../kit/tokens.css';
import '../../kit/fonts.css';
import '../../bootstrap.css';

// Drawn demo facts exist only in this DEV specimen, never in runtime adapters.
if (!import.meta.env.DEV) throw new Error('Search specimens require development.');

const serverId = 'specimen-server';
const artPath = (id: string) => `/v1/items/${id}/art/backdrop?v=${'a'.repeat(64)}&w=1920`;
const genre = (value: string, label: string, count: number) => ({value, label, count,
  backdrop: {kind: 'item', id: value, path: artPath(value)}, covers: []});
const genres = [
  genre('animation', 'Animation', 14), genre('science-fiction', 'Science fiction', 16), genre('drama', 'Drama', 31),
  genre('crime', 'Crime', 9), genre('fantasy', 'Fantasy', 8), genre('horror', 'Horror', 6),
  genre('comedy', 'Comedy', 12), genre('adventure', 'Adventure', 18),
];
const history = ['the last of us', 'spirited away', 'pedro pascal', 'fleetwood mac', 'hail mary', 'toy story', 'severance', 'dune'];
type ChannelFixture = {id: string; name: string; number: string; title: string; start: string; end: string; subtitle?: string; live?: boolean};
const channels: readonly ChannelFixture[] = [
  {id: 'northwind', name: 'Northwind News', number: '101', title: 'World Tonight', start: '2026-10-24T18:00:00Z', end: '2026-10-24T19:30:00Z'},
  {id: 'summit', name: 'Summit Sports', number: '201', title: 'Cycling: Mountain Stage', live: true, start: '2026-10-24T18:00:00Z', end: '2026-10-24T21:30:00Z'},
  {id: 'pebble', name: 'Pebble Kids', number: '401', title: 'Story Time Island', subtitle: 'Homecoming', start: '2026-10-24T18:00:00Z', end: '2026-10-24T19:30:00Z'},
  {id: 'atlas', name: 'Atlas Documentaries', number: '501', title: 'How Bridges Stand', subtitle: 'Fault Lines', start: '2026-10-24T18:00:00Z', end: '2026-10-24T19:15:00Z'},
];
const source = {id: 'synthetic', name: 'Synthetic Live TV', generation: 'g1', publishedAt: '2026-10-24T12:00:00Z',
  availableStart: '', availableEnd: '', provenance: 'live-source', refreshState: 'ready'};
const guideChannel = (channel: ChannelFixture, programmes: readonly unknown[]) => ({id: channel.id, sourceId: source.id,
  provenance: 'live-source', name: channel.name, number: channel.number, group: 'Synthetic', generation: source.generation,
  preferenceRevision: 0, programmes, tuneAvailable: true, recordAvailable: true, favorite: false, hidden: false});
const programme = (channel: ChannelFixture) => ({id: 'programme-' + channel.id, channelId: channel.id, title: channel.title,
  subtitle: channel.subtitle, start: channel.start, end: channel.end, flags: channel.live ? {live: true} : undefined});
const artworkData = 'data:image/svg+xml,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="64" height="36"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#2f4f63"/><stop offset="1" stop-color="#10212d"/></linearGradient></defs><rect width="64" height="36" fill="url(#g)"/></svg>');

async function fixture(path: string): Promise<unknown> {
  const url = new URL(path, 'https://fixture.invalid');
  if (url.pathname === '/v1/search/history') return {serverId, viewerFence: 'fence', maxEntries: 20,
    entries: history.map((q, index) => ({q, updatedAt: new Date(Date.parse('2026-10-24T12:00:00Z') - index * 60000).toISOString(), uses: 1}))};
  if (url.pathname === '/v1/search/genres') return {revision: 'genre-revision', genres};
  if (url.pathname === '/v1/guide/channels') return {protocolVersion: '1.0', serverId,
    directory: {viewerFence: 'fence', revision: 'directory-revision', total: channels.length, offset: 0,
      limit: Number(url.searchParams.get('limit')), sources: [source], channels: channels.map(channel => guideChannel(channel, []))}};
  if (url.pathname === '/v1/guide') {
    const start = url.searchParams.get('start')!, end = url.searchParams.get('end')!, ids = (url.searchParams.get('channels') ?? '').split(',');
    return {protocolVersion: '1.0', serverId, guide: {state: 'ready', viewerFence: 'fence', revision: 'guide-revision', start, end,
      timezone: url.searchParams.get('timezone'), observedAt: start, nextCursor: '', sources: [source],
      channels: channels.filter(channel => ids.includes(channel.id)).map(channel => guideChannel(channel, [programme(channel)]))}};
  }
  throw new Error('No specimen fixture for ' + url.pathname);
}

const artwork = createArtworkRuntime({scopeKey: serverId, classify: () => ({retryable: false}),
  load: async (): Promise<ArtworkImage> => ({src: artworkData, bytes: 0, width: 64, height: 36})});
const effective = {'search.rememberHistory': true, 'appearance.cardSizePercent': 100, 'appearance.showBackdrops': true};
const preferenceSnapshot = {effective} as never;
const preferences = {getSnapshot: () => preferenceSnapshot, subscribe: () => () => {}};
const api = {
  request: async <T>(path: string) => await fixture(path) as T,
  requestRaw: async () => { throw new Error('No specimen transport.'); },
  getRouteSnapshot: () => ({phase: 'ready', origin: 'https://fixture.invalid', revision: 1, network: 'online', message: '', discovery: 'available'}),
  subscribeRoute: () => () => {},
};
const context = {
  api: api as unknown as HttpLocalApi, events: null, scope: {serverId, viewerId: '["local","account","profile"]'},
  isCurrent: () => true, identity: {liveAvailable: true} as never, preferences,
  artwork: {foreground: (url: string | undefined) => url ? artwork.source({url, cacheKey: url}) : undefined, ambient: () => undefined} as never,
  effects: {openEntry: () => {}, openSettings: () => {}, openBrowse: () => {}, play: async () => {}, reportError: () => {}},
} as unknown as ScreenContext;
const binding = createSearchBindings(context);

function Specimen() {
  return <FreshnessProvider api={api as unknown as HttpLocalApi} viewerId="profile" serverId={serverId} events={null}>
    <SearchScreen binding={binding} />
  </FreshnessProvider>;
}
createRoot(document.getElementById('root')!).render(<Specimen />);
