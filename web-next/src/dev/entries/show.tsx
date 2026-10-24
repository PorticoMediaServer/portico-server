import {createRoot} from 'react-dom/client';
import {RouterProvider, createMemoryHistory} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import type {HttpLocalApi, Library, LocalSession, Viewer} from '@core/index.ts';
import {parsePreferenceSnapshot} from '@core/preferences.ts';
import bootstrapWire from '../../../../server/testdata/contracts/get-bootstrap.json?raw';
import type {EventsClient} from '@core/playback-v1/events.ts';
import type {ShellArtwork} from '../../shell/artwork';
import {createArtworkRuntime} from '../../kit/artwork/source';
import {router} from '../../router';
import {WebScreenProvider} from '../../route-registry-view';
import {ShellBindingsProvider, type ShellBindings} from '../../shell/bindings';
import type {ShellAssemblyContext} from '../../shell/bootstrap';
import {createOwnerActivity} from '../../shell/Activity';
import type {ShellIdentity} from '../../shell/Shell';
import type {HomeBindings} from '../../screens/home/Home';
import {IconSprite} from '../../kit/Icon';
import '../../kit/tokens.css';
import '../../kit/fonts.css';
import '../../bootstrap.css';

/**
 * Frame 4's public DEV specimen: the genuine production Show screen, mounted through the
 * registered library route and shell. This module only supplies the answers a public fixture
 * would send (The Last of Us, its four stored episodes, the owner's show settings) and records
 * the real commands the screen issues. It fails closed outside the development server; no
 * production module imports it and no private artwork, credential or server state is used.
 * Facts and copy come from `design/ui-v1/desktop-detail.html` frame 4 and
 * `design/ui-v1/notes/detail-desktop.md`; every image slot carries one neutral inline image.
 */
if (!import.meta.env.DEV) throw new Error('Show specimens require the development server.');
const frame = new URLSearchParams(location.search).get('frame');
if (frame !== null && frame !== '4') throw new Error('The show specimen measures desktop frame 4 only.');

type SpecimenEvent = Readonly<{kind: string; at: number; value?: unknown}>;

const serverId = 'specimen-server';
const libraryId = 'tv';
const showId = 'the-last-of-us';
const seasonId = 'season-1';
const viewerFence = 'specimen-viewer-fence';
const image = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zl+0AAAAASUVORK5CYII=';
const artPath = (id: string, role: 'poster' | 'backdrop' | 'still', query: string) => `/v1/metadata/show/${encodeURIComponent(id)}/art/${role}?${query}`;
const imageArt = (kind: string, id: string) => `specimen-art://${kind}/${encodeURIComponent(id)}`;

const overview = 'After a global pandemic destroys civilization, a hardened survivor takes charge of a 14-year-old girl who may be humanity’s last hope.';
const episodeOverview = 'Joel and Tess head to the Capitol to retrieve something from a dead drop, but their plans change when they meet a young girl who may hold the key to a cure.'; // lint-strings-allow

const credits = [
  {id: 'pedro-pascal', name: 'Pedro Pascal', role: 'Joel Miller', department: 'Acting', ordinal: 0}, // lint-strings-allow
  {id: 'bella-ramsey', name: 'Bella Ramsey', role: 'Ellie Williams', department: 'Acting', ordinal: 1}, // lint-strings-allow
  {id: 'gabriel-luna', name: 'Gabriel Luna', role: 'Tommy Miller', department: 'Acting', ordinal: 2},
  {id: 'isabela-merced', name: 'Isabela Merced', role: 'Dina', department: 'Acting', ordinal: 3},
  {id: 'young-mazino', name: 'Young Mazino', role: 'Jesse', department: 'Acting', ordinal: 4},
  {id: 'kaitlyn-dever', name: 'Kaitlyn Dever', role: 'Abby', department: 'Acting', ordinal: 5},
  {id: 'anna-torv', name: 'Anna Torv', role: 'Tess', department: 'Acting', ordinal: 6},
  {id: 'nick-offerman', name: 'Nick Offerman', role: 'Bill', department: 'Acting', ordinal: 7},
  {id: 'murray-bartlett', name: 'Murray Bartlett', role: 'Frank', department: 'Acting', ordinal: 8},
  {id: 'lamar-johnson', name: 'Lamar Johnson', role: 'Henry', department: 'Acting', ordinal: 9},
  {id: 'melanie-lynskey', name: 'Melanie Lynskey', role: 'Kathleen', department: 'Acting', ordinal: 10},
  {id: 'keivonn-woodard', name: 'Keivonn Montreal Woodard', role: 'Sam', department: 'Acting', ordinal: 11},
  {id: 'storm-reid', name: 'Storm Reid', role: 'Riley', department: 'Acting', ordinal: 12},
  {id: 'scott-shepherd', name: 'Scott Shepherd', role: 'David', department: 'Acting', ordinal: 13},
  {id: 'troy-baker', name: 'Troy Baker', role: 'James', department: 'Acting', ordinal: 14},
  {id: 'ashley-johnson', name: 'Ashley Johnson', role: 'Anna', department: 'Acting', ordinal: 15},
  {id: 'jeffrey-pierce', name: 'Jeffrey Pierce', role: 'Perry', department: 'Acting', ordinal: 16},
  {id: 'graham-greene', name: 'Graham Greene', role: 'Marlon', department: 'Acting', ordinal: 17},
  {id: 'elaine-miles', name: 'Elaine Miles', role: 'Florence', department: 'Acting', ordinal: 18},
  {id: 'rutina-wesley', name: 'Rutina Wesley', role: 'Maria', department: 'Acting', ordinal: 19},
  {id: 'josh-brener', name: 'Josh Brener', role: 'Murray', department: 'Acting', ordinal: 20},
  {id: 'christine-hakim', name: 'Christine Hakim', role: 'Ratna', department: 'Acting', ordinal: 21},
  {id: 'danny-ramirez', name: 'Danny Ramirez', role: 'Manny', department: 'Acting', ordinal: 22},
  {id: 'ariza-lopez', name: 'Ariela Barer', role: 'Mel', department: 'Acting', ordinal: 23},
  {id: 'tati-gabrielle', name: 'Tati Gabrielle', role: 'Nora', department: 'Acting', ordinal: 24},
  {id: 'spencer-lord', name: 'Spencer Lord', role: 'Owen', department: 'Acting', ordinal: 25},
  {id: 'ben-ayers', name: 'Ben Ayers', role: 'Jordan', department: 'Acting', ordinal: 26},
  {id: 'hattie-morahan', name: 'Hattie Morahan', role: 'Julia', department: 'Acting', ordinal: 27},
  {id: 'ian-carr', name: 'Ian Carr', role: 'Isaac', department: 'Acting', ordinal: 28},
  {id: 'philip-philmar', name: 'Philip Philmar', role: 'Ranger', department: 'Acting', ordinal: 29},
  {id: 'robert-smith', name: 'Robert John Burke', role: 'Ranger', department: 'Acting', ordinal: 30},
  {id: 'max-casella', name: 'Max Casella', role: 'Ranger', department: 'Acting', ordinal: 31},
  {id: 'noah-lamanna', name: 'Noah Lamanna', role: 'Ranger', department: 'Acting', ordinal: 32},
  {id: 'catherine-ohara', name: 'Catherine O’Hara', role: 'Ranger', department: 'Acting', ordinal: 33},
  {id: 'jason-ritter', name: 'Jason Ritter', role: 'Ranger', department: 'Acting', ordinal: 34},
  {id: 'aimee-carrero', name: 'Aimee Carrero', role: 'Ranger', department: 'Acting', ordinal: 35},
  {id: 'craig-mazin', name: 'Craig Mazin', role: 'Creator', department: 'Writing', ordinal: 36}, // lint-strings-allow
  {id: 'neil-druckmann', name: 'Neil Druckmann', role: 'Creator', department: 'Writing', ordinal: 37},
] as const;

const episodes = [
  {id: 'ep-1', title: 'When You’re Lost in the Darkness', airDate: '2023-01-15', progressSeconds: 30205, watched: false, overview: episodeOverview}, // lint-strings-allow
  {id: 'ep-2', title: 'Infected', airDate: '2023-01-22', progressSeconds: 0, watched: false, overview: 'A horrifying discovery forces Joel and Tess to change course.'}, // lint-strings-allow
  {id: 'ep-3', title: 'Long, Long Time', airDate: '2023-01-29', progressSeconds: 0, watched: false, overview: 'A stranger asks Bill and Frank for help.'}, // lint-strings-allow
  {id: 'ep-4', title: 'Kin', airDate: '2023-02-05', progressSeconds: 0, watched: false, overview: 'Joel and Ellie reach the Capitol and meet an old friend.'}, // lint-strings-allow
] as const;

const episodeEntry = (index: number) => {
  const episode = episodes[index];
  return {id: episode.id, kind: 'episode', libraryId, title: episode.title, seasonNumber: 1, episodeNumber: index + 1,
    airDate: episode.airDate, duration: 45_000, progressSeconds: episode.progressSeconds, watched: episode.watched,
    overview: episode.overview, stillUrl: imageArt('still', episode.id), available: true,
    navigation: {view: 'item', entityId: episode.id}, playback: {itemId: episode.id}};
};

function showWorkspace() {
  const revision = {catalog: 1, viewer: 1};
  return {
    scope: {serverId, libraryId, viewerFence}, revision,
    show: {id: showId, libraryId, title: 'The Last of Us', year: 2023, libraryKind: 'tv', providerMatchStatus: 'matched', // lint-strings-allow
      overview, network: 'HBO', studio: 'PlayStation Productions', country: 'United States', contentRating: 'TV-MA', // lint-strings-allow
      status: 'Continuing', firstAired: '2023-01-15', originalLanguage: 'en', addedAt: '2026-10-17T00:00:00Z', episodeCount: episodes.length,
      posterUrl: artPath(showId, 'poster', 'w=600'), backdropUrl: artPath(showId, 'backdrop', 'w=1920'),
      genres: ['Drama']}, // lint-strings-allow
    showCredits: credits.map(credit => ({...credit})),
    seasons: [{id: seasonId, showId, number: 1, title: 'Season 1', episodeCount: episodes.length, watchedCount: 0, unwatchedCount: episodes.length, // lint-strings-allow
      firstAired: '2023-01-15', lastAired: '2023-02-05'}],
    seasonTotalCount: 1, nextSeasonCursor: '', groups: [],
    selected: {seasonId},
    episodes: {scope: {serverId, libraryId, libraryKind: 'tv', view: 'season', entityId: seasonId, viewerFence}, revision,
      query: {sort: 'episode', direction: 'asc', category: '', q: '', limit: 40, searchMode: 'none'},
      sections: [{id: 'episodes', type: 'list', heading: 'Season 1', totalCount: episodes.length, nextCursor: '', entries: episodes.map((_, index) => episodeEntry(index))}]},
    nextUp: episodeEntry(0),
    settings: {hideSeasons: false, absoluteNumbering: false, ranges: false, newestFirst: false, revision: 1},
  };
}

const containerState = () => ({scope: {serverId, libraryId, viewerFence}, revision: 1, watched: false, watermark: '', watchlisted: false, favorite: false, watchedCount: 0, unwatchedCount: episodes.length, libraryId});

function episodeDetail() {
  const revision = {catalog: 1, viewer: 1};
  return {
    scope: {serverId, libraryId, itemId: 'ep-1', viewerFence}, revision,
    item: {id: 'ep-1', libraryId, kind: 'episode', title: episodes[0].title, year: 2023, durationMs: 45_000, progressMs: 30_205,
      available: true, addedAt: '2026-10-17T00:00:00Z', overview: episodeOverview,
      posterUrl: imageArt('poster', 'ep-1'), backdropUrl: imageArt('backdrop', 'ep-1')},
    personal: {watchlisted: false, favorite: false, rating: null, revision: 1, watched: false, progressMs: 30_205, lastPlayedAt: '', status: 'current', conflicts: []},
    actions: [{id: 'play', labelKey: 'entry.play', enabled: true, playback: {itemId: 'ep-1', startMs: 0}},
      {id: 'watchlist', labelKey: 'entry.addToWatchlist', enabled: true},
      {id: 'watched', labelKey: 'entry.markWatched', enabled: true}],
    facts: {contentRating: 'TV-MA', releaseDate: '2023-01-15', studio: 'PlayStation Productions', country: 'United States', originalLanguage: 'en'}, // lint-strings-allow
    metadata: {status: 'available', ratings: [], attributions: [], genres: [{id: 'drama', name: 'Drama', provider: 'specimen'}], // lint-strings-allow
      credits: [{id: 'craig-mazin', name: 'Craig Mazin', role: 'Writer', department: 'Writing', provider: 'specimen'}, // lint-strings-allow
        {id: 'pedro-pascal', name: 'Pedro Pascal', role: 'Joel Miller', department: 'Acting', provider: 'specimen'}, // lint-strings-allow
        {id: 'bella-ramsey', name: 'Bella Ramsey', role: 'Ellie Williams', department: 'Acting', provider: 'specimen'}], // lint-strings-allow
      creditTotals: {cast: 2, crew: 1}},
    languages: {audio: ['en'], subtitles: ['en']},
    collections: [], markers: [],
  };
}

function activityDocument(now: string) {
  const session = (id: string) => ({id, user: {id: 'specimen-account', username: 'demo'}, profile: {id: 'demo', name: 'Demo'}, // lint-strings-allow
    device: {id: 'specimen-device', name: 'Portico web'}, kind: 'vod', state: 'playing', startedAt: now, // lint-strings-allow
    problem: false, converting: false, positionMs: 60_000, decision: {}});
  return {scope: {serverId, viewerFence: 'a'.repeat(64)}, observedAt: now, serverName: 'Portico demo', // lint-strings-allow
    streams: {count: 2, convertingCount: 0, overflow: {count: 0, convertingCount: 0}, items: [session('stream-1'), session('stream-2')]},
    scans: [], tasks: {count: 0, items: []}, update: {available: false, state: 'unconfigured', current: ''}, health: {degraded: false}};
}

const records: SpecimenEvent[] = [];
const record = (kind: string, value?: unknown) => {records.push(Object.freeze({kind, at: Date.now(), ...(value === undefined ? {} : {value})}));};

const request = async <T,>(path: string, method = 'GET', body?: unknown): Promise<T> => {
  const route = path.split('?')[0];
  record('request', {path: route, method});
  if (method === 'GET' && route === `/v1/libraries/${libraryId}/show-workspace`) return showWorkspace() as T;
  if (method === 'GET' && route === `/v1/containers/show/${showId}/personal-state`) return containerState() as T;
  if (method === 'GET' && route === '/v1/items/ep-1/detail') return episodeDetail() as T;
  if (method === 'PUT' && route === `/v1/admin/libraries/${libraryId}/shows/${showId}/settings`) {
    const settings = (body ?? {}) as Record<string, unknown>;
    record('show-settings', settings);
    return {...showWorkspace().settings, ...settings, revision: 2} as T;
  }
  throw Object.assign(new Error(`The show specimen has no answer for ${method} ${route}.`), {code: 'not_found', status: 404});
};

const requestRaw = async (path: string) => {
  if (path.split('?')[0] === '/v1/admin/activity') return {status: 200, headers: {}, body: activityDocument(new Date().toISOString())};
  return {status: 404, headers: {}, body: {error: {code: 'not_found'}}};
};

const runtime = createArtworkRuntime({
  scopeKey: 'show-specimen',
  classify: () => ({retryable: false}),
  load: async ({url}, signal) => {
    const response = await fetch(url, {signal});
    const blob = await response.blob();
    const bitmap = await createImageBitmap(blob);
    const width = bitmap.width, height = bitmap.height; bitmap.close();
    return {src: URL.createObjectURL(blob), bytes: blob.size, width, height};
  },
  release: imageValue => { try { URL.revokeObjectURL(imageValue.src); } catch { /* A closed page owns nothing. */ } },
});
const artwork = {
  ...runtime,
  foreground(url: string | undefined) {
    return url ? runtime.source({url: image, cacheKey: `foreground:${url}`}) : undefined;
  },
  ambient(url: string | undefined) {
    return url ? runtime.ambient({url: image, cacheKey: `ambient:${url}`, variant: 'ambient-v1', maxEdge: 64, version: 'a'.repeat(64)}) : undefined;
  },
} as unknown as ShellArtwork;

const events = {on: () => () => {}, onResync: () => () => {}, onAny: () => () => {}, getStatus: () => ({connected: true}), subscribeStatus: () => () => {}} as unknown as EventsClient;
const preferences = parsePreferenceSnapshot((JSON.parse(bootstrapWire) as {response: {preferences: unknown}}).response.preferences);
const subscribePreferences = () => () => {};
const api = {
  request, requestRaw, getRouteSnapshot: () => ({phase: 'ready'}), subscribeRoute: () => () => {},
  reading: () => { const life = new AbortController(); return {signal: life.signal, versions: async () => ({versions: []}), dispose: () => life.abort()}; },
  dispose: () => runtime.dispose(),
} as unknown as HttpLocalApi;

const time = {t: defaultI18n.t, locale: 'en-US', timeZone: 'UTC'};
const viewer: Viewer = {serverId, authority: 'local', accountId: 'specimen-account', profileId: 'demo', role: 'owner'};
const session: LocalSession = {accessToken: 'specimen-public', refreshToken: '', sessionFamilyId: 'specimen-family', tokenGeneration: '1',
  installationId: 'i'.repeat(43), expiresAt: '2099-01-01T00:00:00Z', authorizationHorizon: '2099-02-01T00:00:00Z', viewer};
const libraries: readonly Library[] = [
  {id: 'movies', name: 'Movies', kind: 'movie'},
  {id: libraryId, name: 'TV Shows', kind: 'show'},
  {id: 'anime', name: 'Anime', kind: 'show'},
  {id: 'music', name: 'Music', kind: 'music'},
  {id: 'audiobooks', name: 'Audiobooks', kind: 'audiobook'},
];
const identity: ShellIdentity = {viewer, username: 'demo', profileName: 'Demo', profileArt: 'blue', serverName: 'Portico demo', // lint-strings-allow
  primaryProfile: true, profiles: [], canManageProfiles: false, accountFacts: [], libraries, liveAvailable: true, unread: 3, showNotificationDot: true,
  capabilities: {manageAccount: false, managePeople: false, manageServer: true, manageSignIn: false}};

const contextBase = {api, session: () => session, events, artwork, facts: {identity, preferences, serverName: identity.serverName},
  preferences: {getSnapshot: () => preferences, subscribe: subscribePreferences}, epoch: 1, isCurrent: () => true,
  signOut: () => record('sign-out'), signOutPorticoAccount: () => record('sign-out-portico'), switchProfile: async () => {}, switchServer: async () => {}, savedServers: []} as unknown as ShellAssemblyContext;
const activity = createOwnerActivity(contextBase, time, () => record('open-dashboard'), () => record('open-update'));
const homeService = {getSnapshot: () => ({phase: 'idle'}), subscribe: () => () => {}} as unknown as HomeBindings['service'];
const home: HomeBindings = {service: homeService, artwork, time, reducedMotion: true,
  selection: {toolbar: null, card: () => undefined, channel: () => undefined, collection: () => undefined, registerWindow: () => undefined, registerSpotlight: () => undefined, moreAvailable: () => false, moreExpanded: () => false},
  effects: {play: async () => {record('play');}, playChannel: async () => {}, openEntry: () => {}, openSeeAll: () => {}, openMore: () => {},
    setMyList: async () => {}, reportError: () => {record('report-error');}}};
const binding: ShellBindings = {api, events, scopeKey: 'show-specimen', shell: {identity, OwnerActivity: activity.OwnerActivity, dockVisible: false, onSignOut: () => record('sign-out')}, home};

// The fixture's real command receipts are inspectable from the page; no credentials or server data.
(window as Window & {__porticoShowSpecimen?: unknown}).__porticoShowSpecimen = {records, show: showWorkspace, container: containerState, detail: episodeDetail};

router.update({history: createMemoryHistory({initialEntries: [`/library/${libraryId}?view=show&entityId=${showId}`]})});
createRoot(document.getElementById('root')!).render(
  <WebScreenProvider value={{binding, context: contextBase}}>
    <IconSprite />
    <ShellBindingsProvider value={binding}>
      <RouterProvider router={router} />
    </ShellBindingsProvider>
  </WebScreenProvider>,
);

/** Frame 4 is the scrolled Details tab with the owner's Episode display menu open. */
let tries = 0;
const activate = () => {
  if (++tries > 600) return;
  const details = [...document.querySelectorAll<HTMLElement>('[role="tab"]')].find(tab => tab.textContent?.trim() === defaultI18n.t('next.controls.details'));
  if (!details) { requestAnimationFrame(activate); return; }
  if (details.getAttribute('aria-selected') !== 'true') {
    details.dispatchEvent(new MouseEvent('mousedown', {bubbles: true, button: 0}));
    details.click();
  }
  requestAnimationFrame(() => {
    const change = document.querySelector<HTMLElement>('[data-show-settings] button');
    if (change && change.getAttribute('aria-expanded') !== 'true') change.click();
    requestAnimationFrame(() => document.documentElement.setAttribute('data-show-frame4-ready', 'true'));
  });
};
requestAnimationFrame(activate);
