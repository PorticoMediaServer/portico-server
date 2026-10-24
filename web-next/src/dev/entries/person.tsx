import {createRoot} from 'react-dom/client';
import {RouterProvider, createMemoryHistory} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import type {Library, LocalSession, Viewer} from '@core/index.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import type {PreferenceSnapshot} from '@core/preferences.ts';
import type {HttpLocalApi} from '@core/index.ts';
import {router} from '../../router';
import {WebScreenProvider} from '../../route-registry-view';
import {ShellBindingsProvider, type ShellBindings} from '../../shell/bindings';
import type {ShellAssemblyContext} from '../../shell/bootstrap';
import {createOwnerActivity} from '../../shell/Activity';
import type {ShellIdentity} from '../../shell/Shell';
import type {HomeBindings} from '../../screens/home/Home';
import {IconSprite} from '../../kit/Icon';
import {createArtworkRuntime} from '../../kit/artwork/source';
import type {ShellArtwork} from '../../shell/artwork';
import '../../kit/tokens.css';
import '../../kit/fonts.css';
import '../../bootstrap.css';

// The person page's public DEV facts. The frame's own artwork bytes never enter source:
// every image slot is fed by one neutral inline image. The screen itself is the registered
// production Person route and its real bindings; this module only supplies the server
// answers a public fixture would send. It is imported by the DEV entry alone, which fails
// closed outside the development server.
//
// Facts and copy come from design/ui-v1/desktop-detail.html frame 7 and
// design/ui-v1/notes/detail-desktop.md: Rebecca Ferguson, Acting, born Stockholm,
// Dune: Part Two and Dune as Lady Jessica, Silo as Juliette Nichols and Executive producer.
export type SpecimenEvent = Readonly<{kind: string; at: number; value?: unknown}>;
export type PersonSpecimen = ReturnType<typeof createPersonSpecimen>;

const serverId = 'specimen-server';
const personId = 'rebecca-ferguson';
const viewerFence = 'specimen-viewer-fence';
const personalFence = JSON.stringify(['local', 'specimen-account', 'demo']);
const image = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zl+0AAAAASUVORK5CYII=';
const artPath = (kind: string, id: string) => `specimen-art://${kind}/${encodeURIComponent(id)}`;

type MediaFact = Readonly<{id: string; libraryId: string; kind: 'movie' | 'show'; title: string; year: number; duration: number; progressSeconds: number; watched: boolean; posterUrl: string; backdropUrl: string}>;

const movie = (id: string, title: string, year: number, duration: number, progressSeconds: number, watched: boolean): MediaFact => ({
  id, libraryId: 'movies', kind: 'movie', title, year, duration, progressSeconds, watched,
  posterUrl: artPath('poster', id), backdropUrl: artPath('backdrop', id),
});
const show = (id: string, title: string, year: number, duration: number, progressSeconds: number): MediaFact => ({
  id, libraryId: 'tv', kind: 'show', title, year, duration, progressSeconds, watched: false,
  posterUrl: artPath('poster', id), backdropUrl: artPath('backdrop', id),
});
const dunePartTwo = movie('dune-part-two', 'Dune: Part Two', 2024, 9_960, 0, false);
const dune = movie('dune', 'Dune', 2021, 9_300, 0, true);
const silo = show('silo', 'Silo', 2023, 2_700, 1_080);

function entry(media: MediaFact) {
  return {id: media.id, libraryId: media.libraryId, kind: media.kind, title: media.title, year: media.year,
    available: true, watched: media.watched, duration: media.duration, progressSeconds: media.progressSeconds,
    posterUrl: media.posterUrl, backdropUrl: media.backdropUrl, addedAt: '2026-10-17T00:00:00Z',
    navigation: {view: media.kind === 'movie' ? 'item' : 'show', entityId: media.id},
    playback: {itemId: media.id, startSeconds: media.progressSeconds}} as const;
}

const acting = (character: string) => ({roles: Object.freeze([Object.freeze({role: character, department: 'Acting'})]),
  media: entry(dunePartTwo), role: character, character, department: 'Acting', creditKind: 'cast' as const});
const duneCredit = {...acting('Lady Jessica'), media: entry(dune)};
const siloCredit = {roles: Object.freeze([Object.freeze({role: 'Juliette Nichols', department: 'Acting'}), Object.freeze({role: 'Executive producer', department: 'Production'})]),
  media: entry(silo), role: 'Juliette Nichols', character: 'Juliette Nichols', department: 'Acting', creditKind: 'cast' as const};

const biography = 'Rebecca Louisa Ferguson Sundström is a Swedish actress. After starting out in Swedish television, she came to wider attention as Elizabeth Woodville in The White Queen (2013), which earned her a Golden Globe nomination. She went on to play Ilsa Faust in three Mission: Impossible films, Lady Jessica in Dune and its sequel, and Juliette Nichols in Silo, which she also executive produces.';
const knownFor = [entry(dunePartTwo), entry(silo), entry(dune)];
const groups = Object.freeze([Object.freeze({id: 'movie', kind: 'movie', total: 2}), Object.freeze({id: 'show', kind: 'show', total: 1})]);
const credits = Object.freeze([acting('Lady Jessica'), duneCredit, siloCredit]);
const person = Object.freeze({id: personId, name: 'Rebecca Ferguson', sortName: 'Ferguson, Rebecca', biography,
  biographySource: 'TMDB', birthDate: '1983-10-19', deathDate: '', birthPlace: 'Stockholm, Sweden',
  portraitUrl: '', roles: Object.freeze(['Acting']), knownFor: Object.freeze(knownFor), providerIds: Object.freeze({}), revision: 1});
const metric = Object.freeze({catalog: 1, viewer: 1});

function pageFor(role: string | null, kind: string | null, limit: number) {
  const byRole = credits.filter(credit => role === 'cast' ? credit.roles.some(item => /^(acting|cast)$/i.test(item.department))
    : role === 'crew' ? credit.roles.some(item => !/^(acting|cast)$/i.test(item.department)) : true);
  const selected = byRole.filter(credit => kind === null || credit.media.kind === kind);
  return {serverId, viewerFence, revision: metric, person, groups, credits: selected.slice(0, limit),
    pageInfo: {total: selected.length, nextCursor: selected.length > limit ? 'specimen-next' : ''}};
}

function activityDocument(now: string) {
  const session = (id: string) => ({
    id, user: {id: 'specimen-account', username: 'demo'}, profile: {id: 'demo', name: 'Demo'}, // lint-strings-allow: public DEV specimen identity
    device: {id: 'specimen-device', name: 'Portico web'}, kind: 'vod', state: 'playing', startedAt: now, // lint-strings-allow: public DEV specimen device
    problem: false, converting: false, positionMs: 60_000, decision: {},
  });
  return {scope: {serverId, viewerFence: 'a'.repeat(64)}, observedAt: now, serverName: 'Portico demo', // lint-strings-allow: public DEV specimen server
    streams: {count: 2, convertingCount: 0, overflow: {count: 0, convertingCount: 0}, items: [session('stream-1'), session('stream-2')]},
    scans: [], tasks: {count: 0, items: []}, update: {available: false, state: 'unconfigured', current: ''}, health: {degraded: false}};
}

export function createPersonSpecimen() {
  const records: SpecimenEvent[] = [];
  const record = (kind: string, value?: unknown): void => {
    records.push(Object.freeze({kind, at: Date.now(), ...(value === undefined ? {} : {value})}));
  };
  const request = async <T,>(path: string, method = 'GET'): Promise<T> => {
    const url = new URL(path, 'http://specimen.invalid'), route = url.pathname;
    record('request', {path: route, method});
    if (method === 'GET' && route === `/v1/people/${personId}`) {
      const limit = Number(url.searchParams.get('limit') ?? '40');
      if (!Number.isInteger(limit) || limit < 1 || limit > 100) throw Object.assign(new Error('Person page bound refused.'), {code: 'invalid_person', status: 400});
      return pageFor(url.searchParams.get('role'), url.searchParams.get('kind'), limit) as T;
    }
    throw Object.assign(new Error(`The person specimen has no answer for ${method} ${route}.`), {code: 'not_found', status: 404});
  };
  const requestRaw = async (path: string) => {
    if (path.split('?')[0] === '/v1/admin/activity') return {status: 200, headers: {}, body: activityDocument(new Date().toISOString())};
    return {status: 404, headers: {}, body: {error: {code: 'not_found'}}};
  };
  const runtime = createArtworkRuntime({
    scopeKey: 'person-specimen',
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
  const events = {on: () => () => {}, onResync: () => () => {}, onAny: () => () => {},
    getStatus: () => ({connected: true}), subscribeStatus: () => () => {}} as unknown as EventsClient;
  const preferences = {deviceClass: 'web', registry: {fields: [], revision: 'person-specimen'}, documents: [],
    effective: {'appearance.showBackdrops': true}, effectiveSource: {}, clampedFields: [], registryRevision: 'person-specimen'} as unknown as PreferenceSnapshot;
  const subscribePreferences = () => () => {};
  const api = {
    request, requestRaw, getRouteSnapshot: () => ({phase: 'ready'}), subscribeRoute: () => () => {},
    reading: () => { const life = new AbortController(); return {signal: life.signal, versions: async () => ({versions: []}), dispose: () => life.abort()}; },
    dispose: () => runtime.dispose(),
  } as unknown as HttpLocalApi;
  return Object.freeze({serverId, personId, viewerFence, personalFence, records, record,
    api, events, artwork, preferences, subscribePreferences, person, credits, groups});
}

if (!import.meta.env.DEV) throw new Error('Person specimens require the development server.');

const specimen = createPersonSpecimen();
const time = {t: defaultI18n.t, locale: 'en-US', timeZone: 'UTC'};
const viewer: Viewer = {serverId: specimen.serverId, authority: 'local', accountId: 'specimen-account', profileId: 'demo', role: 'owner'};
const session: LocalSession = {accessToken: 'specimen-public', refreshToken: '', sessionFamilyId: 'specimen-family', tokenGeneration: '1',
  installationId: 'i'.repeat(43), expiresAt: '2099-01-01T00:00:00Z', authorizationHorizon: '2099-02-01T00:00:00Z', viewer};
const libraries: readonly Library[] = [
  {id: 'movies', name: 'Movies', kind: 'movie'}, {id: 'tv', name: 'TV Shows', kind: 'show'},
  {id: 'anime', name: 'Anime', kind: 'show'}, {id: 'music', name: 'Music', kind: 'music'},
  {id: 'audiobooks', name: 'Audiobooks', kind: 'audiobook'},
];
const identity: ShellIdentity = {viewer, username: 'demo', profileName: 'Demo', profileArt: 'blue', serverName: 'Portico demo',
  primaryProfile: true, profiles: [], canManageProfiles: false, accountFacts: [], libraries, liveAvailable: true,
  unread: 3, showNotificationDot: true, capabilities: {manageAccount: false, managePeople: false, manageServer: true, manageSignIn: false}};
const preferences = specimen.preferences as unknown as PreferenceSnapshot;
const homeService = {getSnapshot: () => ({phase: 'idle'}), subscribe: () => () => {}} as unknown as HomeBindings['service'];
const home: HomeBindings = {
  service: homeService, artwork: specimen.artwork, time, reducedMotion: true,
  selection: {toolbar: null, card: () => undefined, channel: () => undefined, collection: () => undefined,
    registerWindow: () => undefined, registerSpotlight: () => undefined, moreAvailable: () => false, moreExpanded: () => false},
  effects: {play: async () => { specimen.record('play'); }, playChannel: async () => {}, openEntry: () => {},
    openSeeAll: () => {}, openMore: () => {}, setMyList: async () => {},
    reportError: error => { specimen.record('report-error', error); }},
};
const contextBase = {
  api: specimen.api, session: () => session, events: specimen.events, artwork: specimen.artwork,
  facts: {identity, preferences, serverName: identity.serverName}, preferences: {getSnapshot: () => preferences, subscribe: specimen.subscribePreferences},
  epoch: 1, isCurrent: () => true, signOut: () => { specimen.record('sign-out'); },
  signOutPorticoAccount: () => { specimen.record('sign-out-portico'); },
  switchProfile: async () => {}, switchServer: async () => {}, savedServers: [],
} as unknown as ShellAssemblyContext;
const activity = createOwnerActivity(contextBase, time, () => { specimen.record('open-dashboard'); }, () => { specimen.record('open-update'); });
const binding: ShellBindings = {
  api: specimen.api, events: specimen.events, scopeKey: 'person-specimen',
  shell: {identity, OwnerActivity: activity.OwnerActivity, dockVisible: false, onSignOut: () => { specimen.record('sign-out'); }},
  home,
};
(window as Window & {__porticoPersonSpecimen?: unknown}).__porticoPersonSpecimen = {records: specimen.records, person: specimen.person, groups: specimen.groups};
router.update({history: createMemoryHistory({initialEntries: [`/person/${specimen.personId}`]})});
createRoot(document.getElementById('root')!).render(
  <WebScreenProvider value={{binding, context: contextBase}}>
    <IconSprite />
    <ShellBindingsProvider value={binding}>
      <RouterProvider router={router} />
    </ShellBindingsProvider>
  </WebScreenProvider>,
);
