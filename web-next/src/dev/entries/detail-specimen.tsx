import type {HttpLocalApi} from '@core/index.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import type {PreferenceSnapshot} from '@core/preferences.ts';
import type {ShellArtwork} from '../../shell/artwork';
import {createArtworkRuntime} from '../../kit/artwork/source';

/**
 * The title page's public DEV facts. The frame's own artwork bytes never enter source: every
 * image slot is fed by one neutral inline image, and the measured comparator excludes artwork
 * contents. The screen itself is the registered production Item route and its real bindings; this
 * module only supplies the server answers a public fixture would send and records the real
 * commands the screen issues. It is imported by the DEV entry alone, which fails closed outside
 * the development server.
 *
 * Facts and copy come from `design/ui-v1/desktop-detail.html` frame 1 and
 * `design/ui-v1/notes/detail-desktop.md`: Dune: Part Two, not started, More like this, the two
 * drawn recommendation rows and the 16 drawn catalogue cards.
 */
export type SpecimenEvent = Readonly<{kind: string; at: number; value?: unknown}>;

const serverId = 'specimen-server';
const libraryId = 'movies';
const itemId = 'dune-part-two';
const viewerFence = 'specimen-viewer-fence';
const personalFence = JSON.stringify(['local', 'specimen-account', 'demo']);
const image = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zl+0AAAAASUVORK5CYII=';
const artPath = (kind: string, id: string) => `specimen-art://${kind}/${encodeURIComponent(id)}`;

const posterRow = (items: readonly (readonly [string, number])[]) => items.map(([title, year]) => {
  const id = title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  return {
    id, kind: 'movie', libraryId, title, year, available: true, posterUrl: artPath('poster', id),
    navigation: {view: 'item', entityId: id}, playback: {itemId: id},
  } as const;
});

const moreLike = posterRow([
  ['Dune', 2021], ['Blade Runner', 1982], ['Interstellar', 2014], ['Star Wars', 1977],
  ['Avatar', 2009], ['The Empire Strikes Back', 1980], ['The Matrix', 1999], ['Gladiator', 2000],
]);
const viewersAlso = posterRow([
  ['Oppenheimer', 2023], ['Inception', 2010], ['The Dark Knight', 2008], ['Barbie', 2023],
  ['Spider-Man: Across the Spider-Verse', 2023], ['Fight Club', 1999], ['Alien', 1979], ['Project Hail Mary', 2026],
]);

const credits = [
  {id: 'denis-villeneuve', name: 'Denis Villeneuve', role: 'Director', department: 'Directing', provider: 'specimen'},
  {id: 'timothee-chalamet', name: 'Timothée Chalamet', role: 'Paul Atreides', department: 'Acting', provider: 'specimen'},
  {id: 'zendaya', name: 'Zendaya', role: 'Chani', department: 'Acting', provider: 'specimen'},
  {id: 'rebecca-ferguson', name: 'Rebecca Ferguson', role: 'Lady Jessica', department: 'Acting', provider: 'specimen'},
  {id: 'javier-bardem', name: 'Javier Bardem', role: 'Stilgar', department: 'Acting', provider: 'specimen'},
  {id: 'jon-spaihts', name: 'Jon Spaihts', role: 'Screenplay', department: 'Writing', provider: 'specimen'},
] as const;

const overview = 'Follow the mythic journey of Paul Atreides as he unites with Chani and the Fremen while on a path of revenge against the conspirators who destroyed his family. Facing a choice between the love of his life and the fate of the known universe, Paul endeavors to prevent a terrible future only he can foresee.';

function basePersonal() {
  return {watchlisted: false, favorite: false, rating: null, revision: 1, watched: false, progressMs: 0, lastPlayedAt: '', status: 'current', conflicts: [] as unknown[]};
}

const metric = {catalog: 1, viewer: 1};

function detailDocument(personal: ReturnType<typeof basePersonal>) {
  return {
    scope: {serverId, libraryId, itemId, viewerFence},
    revision: {...metric},
    item: {
      id: itemId, libraryId, kind: 'movie', title: 'Dune: Part Two', year: 2024, durationMs: 9_960_000,
      progressMs: 0, available: true, addedAt: '2026-10-17T00:00:00Z', overview,
      posterUrl: artPath('poster', itemId), backdropUrl: artPath('backdrop', itemId),
    },
    personal: {...personal},
    actions: [
      {id: 'play', labelKey: 'entry.play', enabled: true, playback: {itemId, startMs: 0}},
      {id: 'watchlist', labelKey: 'entry.addToWatchlist', enabled: true},
      {id: 'watched', labelKey: 'entry.markWatched', enabled: true},
      {id: 'favorite', labelKey: 'entry.addToFavorites', enabled: true},
    ],
    facts: {
      contentRating: 'PG-13', tagline: 'Long live the fighters.', releaseDate: '2024-02-27',
      studio: 'Legendary Pictures', country: 'United States', originalLanguage: 'en',
    },
    metadata: {
      status: 'available', ratings: [], attributions: [],
      genres: [
        {id: 'science-fiction', name: 'Science Fiction', provider: 'specimen'},
        {id: 'adventure', name: 'Adventure', provider: 'specimen'},
      ],
      credits: [...credits], creditTotals: {cast: 4, crew: 2},
    },
    languages: {audio: ['en'], subtitles: ['en']},
    collections: [{id: 'dune-collection', name: 'Dune collection', memberCount: 2, position: 0, posterUrls: []}],
    markers: [],
  };
}

function recommendationRow(id: string, title: string, entries: readonly unknown[], subtitleText?: {code: string; fallback: string}) {
  return {
    id, viewerFence, title, kind: 'recommendations', artworkShape: 'poster', endpoint: 'item-recommendations',
    privacySensitivity: 'public', policyState: 'current', priority: id === 'more-like' ? 1 : 2, required: false,
    hideable: true, reorderable: false, critical: false, cursorCapable: false, titleText: {code: `specimen.${id}`, fallback: title},
    ...(subtitleText ? {subtitleText} : {}), entries, total: entries.length, start: 0, limit: entries.length,
    hasMore: false, nextCursor: '', revision: {...metric},
  };
}

function recommendationsDocument() {
  return {
    itemId, revision: {...metric},
    rows: [
      recommendationRow('more-like', 'More like Dune: Part Two', moreLike),
      recommendationRow('viewers-also-watched', 'Viewers also watched', viewersAlso,
        {code: 'specimen.viewersAlsoWatchedNote', fallback: 'What people on this server played after it'}),
    ],
  };
}

function activityDocument(now: string) {
  const session = (id: string) => ({
    id, user: {id: 'specimen-account', username: 'demo'}, profile: {id: 'demo', name: 'Demo'},
    device: {id: 'specimen-device', name: 'Portico web'}, kind: 'vod', state: 'playing', startedAt: now,
    problem: false, converting: false, positionMs: 60_000, decision: {},
  });
  return {
    scope: {serverId, viewerFence: 'a'.repeat(64)}, observedAt: now, serverName: 'Portico demo',
    streams: {count: 2, convertingCount: 0, overflow: {count: 0, convertingCount: 0}, items: [session('stream-1'), session('stream-2')]},
    scans: [], tasks: {count: 0, items: []}, update: {available: false, state: 'unconfigured', current: ''},
    health: {degraded: false},
  };
}

export function createDetailSpecimen() {
  const records: SpecimenEvent[] = [];
  const record = (kind: string, value?: unknown): void => {
    records.push(Object.freeze({kind, at: Date.now(), ...(value === undefined ? {} : {value})}));
  };
  let personal = basePersonal();
  const detail = () => detailDocument(personal);
  const item = () => ({id: itemId, libraryId, kind: 'movie', title: 'Dune: Part Two', available: true, durationMs: 9_960_000, progressMs: 0});
  const recommendations = () => recommendationsDocument();

  const request = async <T,>(path: string, method = 'GET', body?: unknown): Promise<T> => {
    const route = path.split('?')[0];
    record('request', {path: route, method});
    if (method === 'GET' && route === `/v1/items/${itemId}`) return item() as T;
    if (method === 'GET' && route === `/v1/items/${itemId}/detail`) return detail() as T;
    if (method === 'GET' && route === `/v1/items/${itemId}/recommendations`) return recommendations() as T;
    if (method === 'PUT' && route === `/v1/items/${itemId}/personal-state`) {
      const intent = (body ?? {}) as {operationId?: unknown; watchlisted?: unknown; favorite?: unknown; watched?: unknown; rating?: unknown};
      personal = {
        ...personal,
        ...(typeof intent.watchlisted === 'boolean' ? {watchlisted: intent.watchlisted} : {}),
        ...(typeof intent.favorite === 'boolean' ? {favorite: intent.favorite} : {}),
        ...(typeof intent.watched === 'boolean' ? {watched: intent.watched} : {}),
        revision: personal.revision + 1,
      };
      record('personal-state', {watchlisted: personal.watchlisted, favorite: personal.favorite, watched: personal.watched});
      return {operationId: intent.operationId, serverId, libraryId, itemId, viewerFence, current: {...personal}} as T;
    }
    throw Object.assign(new Error(`The detail specimen has no answer for ${method} ${route}.`), {code: 'not_found', status: 404});
  };

  const requestRaw = async (path: string) => {
    if (path.split('?')[0] === '/v1/admin/activity') return {status: 200, headers: {}, body: activityDocument(new Date().toISOString())};
    return {status: 404, headers: {}, body: {error: {code: 'not_found'}}};
  };

  const runtime = createArtworkRuntime({
    scopeKey: 'detail-specimen',
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

  const events = {
    on: () => () => {}, onResync: () => () => {}, onAny: () => () => {},
    getStatus: () => ({connected: true}), subscribeStatus: () => () => {},
  } as unknown as EventsClient;

  const preferences = {
    deviceClass: 'web', registry: {fields: [], revision: 'detail-specimen'},
    documents: [], effective: {'appearance.showBackdrops': true}, effectiveSource: {},
    clampedFields: [], registryRevision: 'detail-specimen',
  } as unknown as PreferenceSnapshot;
  const subscribePreferences = () => () => {};

  const api = {
    request, requestRaw, getRouteSnapshot: () => ({phase: 'ready'}), subscribeRoute: () => () => {},
    reading: () => {
      const life = new AbortController();
      return {signal: life.signal, versions: async () => ({versions: []}), dispose: () => life.abort()};
    },
    dispose: () => runtime.dispose(),
  } as unknown as HttpLocalApi;

  return Object.freeze({
    serverId, libraryId, itemId, viewerFence, personalFence, records, record,
    api, events, artwork, preferences, subscribePreferences,
    identity: {serverId, viewerId: personalFence}, detail, recommendations,
  });
}

export type DetailSpecimen = ReturnType<typeof createDetailSpecimen>;
