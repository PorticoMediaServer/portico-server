/**
 * The fixture backend: implements every service interface over the in-memory database with a
 * little latency, so loading states are real. Replace `createFixtureServices()` with an adapter
 * over Portico's HTTP APIs to ship.
 */
import type {BrowseQuery, Page, Services} from '../services';
import type {
  Audiobook, Channel, Collection, DeviceSession, DownloadSettings, Episode, HomeLayoutEntry, Item, Movie, Playlist, Profile, RecordingRule, Row, Season, Show, TogetherGroup, UserState,
} from '../types';
import {createAdmin} from './adminData';
import {buildDB, DAY, emptyState, HOUR, LIB, NOW} from './db';
import {hash, rng, slug} from './rng';

const wait = (ms = 180) => new Promise<void>(r => setTimeout(r, ms + (Math.random() * ms) / 2));

export function createFixtureServices(): Services {
  const db = buildDB();
  let version = 1;
  const listeners = new Set<() => void>();
  const bump = () => {
    version++;
    listeners.forEach(l => l());
  };
  const get = (id: string) => db.items.get(id);
  const st = (id: string): UserState => db.state.get(id) ?? emptyState();
  const setSt = (id: string, patch: Partial<UserState>) => db.state.set(id, {...st(id), ...patch});
  const isVideo = (i: Item) => i.kind === 'movie' || i.kind === 'show' || i.kind === 'episode';
  const movies = () => (db.byLibrary.get(LIB.movies) ?? []).map(id => get(id) as Movie);
  const shows = () => [...(db.byLibrary.get(LIB.shows) ?? []), ...(db.byLibrary.get(LIB.anime) ?? [])].map(id => get(id) as Show);
  const visible = (id: string) => !st(id).notInterested;

  // --- Show helpers
  const episodesOf = (showId: string) => {
    const s = get(showId) as Show;
    return s.seasonIds.filter(x => !x.endsWith('-s0')).flatMap(x => (get(x) as Season).episodeIds);
  };
  const nextUp = (showId: string) => {
    const eps = episodesOf(showId);
    const inProg = eps.find(e => st(e).progress > 0 && !st(e).watched);
    if (inProg) return inProg;
    let last = -1;
    eps.forEach((e, i) => st(e).watched && (last = i));
    return last >= 0 && last < eps.length - 1 ? eps[last + 1] : undefined;
  };

  // --- Home rows
  const continueIds = () => {
    const ids = [...db.state.entries()].filter(([id, s]) => s.progress > 0 && !s.watched && !db.continueHidden.has(id) && get(id) && isVideo(get(id)!)).sort((a, b) => (b[1].lastPlayedAt ?? 0) - (a[1].lastPlayedAt ?? 0)).map(([id]) => id);
    return ids;
  };
  const buildRows = (): Row[] => {
    const ms = movies();
    const unwatched = ms.filter(m => !st(m.id).watched && visible(m.id));
    const lastWatched = db.history.map(h => get(h.itemId)).find(i => i?.kind === 'movie') as Movie | undefined;
    const genreOf = lastWatched?.genres[0] ?? 'Drama';
    const because = unwatched.filter(m => lastWatched && m.id !== lastWatched.id && m.genres.some(g => lastWatched.genres.includes(g))).slice(0, 18);
    const actorCounts = new Map<string, number>();
    ms.forEach(m => m.credits.filter(c => c.department === 'Acting').forEach(c => actorCounts.set(c.personId, (actorCounts.get(c.personId) ?? 0) + 1)));
    const topActor = [...actorCounts.entries()].sort((a, b) => b[1] - a[1])[0]?.[0];
    const directorCounts = new Map<string, number>();
    ms.forEach(m => m.credits.filter(c => c.job === 'Director').forEach(c => directorCounts.set(c.personId, (directorCounts.get(c.personId) ?? 0) + 1)));
    const topDirector = [...directorCounts.entries()].sort((a, b) => b[1] - a[1])[0]?.[0];
    const r = rng('rows');
    const upNext = shows().map(s => nextUp(s.id)).filter(Boolean) as string[];
    const recentShows = [...(db.byLibrary.get(LIB.shows) ?? [])].sort((a, b) => get(b)!.addedAt - get(a)!.addedAt);
    const featured = [...ms.filter(m => m.real && m.art.backdrop).slice(0, 4), ...unwatched.filter(m => m.art.backdrop && (m.criticScore ?? 0) > 80).slice(0, 4), ...shows().filter(s => s.art.backdrop).slice(0, 2)];
    const listening = [...db.state.entries()].filter(([id, s]) => (s.progress > 0 || s.lastPlayedAt) && (get(id)?.kind === 'audiobook' || get(id)?.kind === 'album')).sort((a, b) => (b[1].lastPlayedAt ?? 0) - (a[1].lastPlayedAt ?? 0)).map(([id]) => id).slice(0, 12);
    const moodPick = 'Mind-bending';
    const rows: Row[] = [
      {id: 'featured', kind: 'featured', title: 'Featured', itemIds: [...new Set(featured.map(m => m.id))], shape: 'poster'},
      {id: 'continue', kind: 'continue', title: 'Continue Watching', itemIds: continueIds(), shape: 'landscape'},
      {id: 'upnext', kind: 'upNext', title: 'Up Next', itemIds: upNext.filter(id => !continueIds().includes(id)), shape: 'landscape'},
      {id: 'picks', kind: 'picks', title: 'Picks for you', itemIds: r.shuffle(unwatched.filter(m => (m.audienceScore ?? 0) > 70)).slice(0, 20).map(m => m.id), shape: 'poster', recommendation: true},
      {id: 'recent-movies', kind: 'recentIn', title: 'Recently added in', subject: 'Movies', itemIds: [...ms].sort((a, b) => b.addedAt - a.addedAt).slice(0, 20).map(m => m.id), shape: 'poster'},
      {id: 'because', kind: 'because', title: 'Because you watched', subject: lastWatched?.title, itemIds: because.map(m => m.id), shape: 'poster', recommendation: true},
      {id: 'continue-listening', kind: 'continueListening', title: 'Continue Listening', itemIds: listening, shape: 'square'},
      {id: 'recent-shows', kind: 'recentIn', title: 'Recently added in', subject: 'TV Shows', itemIds: recentShows.slice(0, 18), shape: 'poster'},
      {id: 'genre', kind: 'genre', title: `${genreOf} for you`, itemIds: unwatched.filter(m => m.genres.includes(genreOf)).slice(0, 18).map(m => m.id), shape: 'poster', recommendation: true},
      {id: 'starring', kind: 'starring', title: 'Starring', subject: topActor ? db.people.get(topActor)?.name : undefined, itemIds: topActor ? ms.filter(m => m.credits.some(c => c.personId === topActor)).map(m => m.id) : [], shape: 'poster', recommendation: true},
      {id: 'watchlist', kind: 'watchlist', title: 'Watchlist', itemIds: [...db.state.entries()].filter(([, s]) => s.watchlist).map(([id]) => id).filter(id => get(id)), shape: 'poster'},
      {id: 'mood', kind: 'mood', title: `${moodPick} picks for you`, itemIds: unwatched.filter(m => m.mood === moodPick).slice(0, 16).map(m => m.id), shape: 'poster', recommendation: true},
      {id: 'classics', kind: 'collection', title: 'Public Domain Classics', itemIds: db.collections.find(c => c.id === 'col-public-domain-classics')?.itemIds ?? [], shape: 'poster'},
      {id: 'hidden-gems', kind: 'hiddenGems', title: 'Hidden gems for you', itemIds: unwatched.filter(m => (m.criticScore ?? 0) > 82 && (m.audienceScore ?? 100) < 75).slice(0, 16).map(m => m.id), shape: 'poster', recommendation: true},
      {id: 'directed-by', kind: 'directedBy', title: 'Directed by', subject: topDirector ? db.people.get(topDirector)?.name : undefined, itemIds: topDirector ? ms.filter(m => m.credits.some(c => c.personId === topDirector)).map(m => m.id) : [], shape: 'poster', recommendation: true},
      {id: 'recent-anime', kind: 'recentIn', title: 'Recently added in', subject: 'Anime', itemIds: [...(db.byLibrary.get(LIB.anime) ?? [])].slice(0, 16), shape: 'poster'},
      {id: 'top-rated', kind: 'topRated', title: 'Top rated you haven’t seen', itemIds: [...unwatched].sort((a, b) => (b.criticScore ?? 0) - (a.criticScore ?? 0)).slice(0, 18).map(m => m.id), shape: 'poster', recommendation: true},
      {id: 'trending', kind: 'trending', title: 'Trending now', itemIds: r.shuffle(ms.filter(m => (m.year ?? 0) >= 2018)).slice(0, 16).map(m => m.id), shape: 'poster', recommendation: true},
      {id: 'popular', kind: 'popular', title: 'Popular on this server', itemIds: r.shuffle([...ms.slice(0, 40), ...shows().slice(0, 8)]).slice(0, 18).map(m => m.id), shape: 'poster'},
      {id: 'also-watched', kind: 'alsoWatched', title: 'Viewers also watched', itemIds: r.shuffle(unwatched).slice(0, 16).map(m => m.id), shape: 'poster', recommendation: true},
      {id: 'rediscover', kind: 'rediscover', title: 'Rediscover', itemIds: ms.filter(m => st(m.id).watched && (st(m.id).lastPlayedAt ?? NOW) < NOW - 60 * DAY).slice(0, 16).map(m => m.id), shape: 'poster', recommendation: true},
      {id: 'new-for-you', kind: 'newForYou', title: 'New for you', itemIds: unwatched.filter(m => (m.year ?? 0) >= 2023).slice(0, 16).map(m => m.id), shape: 'poster', recommendation: true},
      {id: 'favorites', kind: 'favorites', title: 'Favorites', itemIds: [...db.state.entries()].filter(([, s]) => s.favorite).map(([id]) => id).filter(id => get(id) && isVideo(get(id)!)), shape: 'poster'},
    ];
    for (const v of db.savedViews.filter(v => v.onHome)) {
      rows.push({id: 'view-' + v.id, kind: 'savedView', title: v.title, itemIds: browseIds({libraryId: v.libraryId, filters: v.filters, sort: v.sort}).slice(0, 20), shape: 'poster'});
    }
    return rows.map(row => ({...row, itemIds: row.itemIds.filter(id => get(id) && (row.recommendation ? visible(id) : true)), total: row.itemIds.length}));
  };
  const defaultLayout = (rows: Row[]): HomeLayoutEntry[] => rows.map(r => ({rowId: r.id, visible: !['rediscover', 'favorites', 'also-watched', 'new-for-you'].includes(r.id), alwaysOn: r.id === 'continue' || r.id === 'featured'}));

  // --- Browse
  const libItems = (libraryId: string, kind?: string) => {
    if (libraryId === LIB.music) {
      if (kind === 'songs') return [...db.items.values()].filter(i => i.kind === 'track');
      if (kind === 'releases') return [...db.items.values()].filter(i => i.kind === 'album');
      return (db.byLibrary.get(libraryId) ?? []).map(get).filter(i => i?.kind === 'artist') as Item[];
    }
    return (db.byLibrary.get(libraryId) ?? []).map(id => get(id)!).filter(Boolean);
  };
  const decadeOf = (y?: number) => (y ? `${Math.floor(y / 10) * 10}s` : '');
  const matches = (i: Item, q: BrowseQuery) => {
    const s = st(i.id);
    for (const quick of q.filters.quick) {
      if (quick === 'unwatched' && (s.watched || (i.kind === 'show' && episodesOf(i.id).every(e => st(e).watched)))) return false;
      if (quick === 'inProgress' && !(s.progress > 0 && !s.watched)) return false;
      if (quick === 'watched' && !s.watched) return false;
      if (quick === 'favorites' && !s.favorite) return false;
      if (quick === 'watchlist' && !s.watchlist) return false;
      if (quick === 'recentlyAdded' && i.addedAt < NOW - 30 * DAY) return false;
      if (quick === 'uhd' && !('versions' in i && i.versions.some(v => v.resolution === '4K'))) return false;
      if (quick === 'hd' && !('versions' in i && i.versions.some(v => v.resolution === '1080p' || v.resolution === '4K'))) return false;
    }
    for (const [field, values] of Object.entries(q.filters.fields)) {
      if (!values.length) continue;
      if (field === 'genre' && !values.some(v => i.genres.includes(v))) return false;
      if (field === 'decade' && !values.includes(decadeOf(i.year))) return false;
      if (field === 'contentRating' && !('contentRating' in i && values.includes(i.contentRating))) return false;
      if (field === 'studio' && !('studio' in i && values.includes(i.studio ?? ''))) return false;
      if (field === 'network' && !('network' in i && values.includes(i.network))) return false;
      if (field === 'language' && !('language' in i && values.includes(i.language ?? ''))) return false;
      if (field === 'mood' && !('mood' in i && values.includes(i.mood ?? ''))) return false;
      if (field === 'duration' && !('runtime' in i && values.some(v => (v === 'short' ? i.runtime < 90 : v === 'long' ? i.runtime > 140 : true)))) return false;
      if (field === 'resolution' && !('versions' in i && i.versions.some(v => values.includes(v.resolution)))) return false;
      if (field === 'person' && !('credits' in i && i.credits.some(c => values.includes(c.name)))) return false;
    }
    if (q.filters.yearFrom && (i.year ?? 0) < q.filters.yearFrom) return false;
    if (q.filters.yearTo && (i.year ?? 9999) > q.filters.yearTo) return false;
    return true;
  };
  const sorter = (q: BrowseQuery) => (a: Item, b: Item) => {
    const d = q.sort.direction === 'asc' ? 1 : -1;
    switch (q.sort.field) {
      case 'year': return ((a.year ?? 0) - (b.year ?? 0)) * d || a.sortTitle.localeCompare(b.sortTitle);
      case 'added': return (a.addedAt - b.addedAt) * d;
      case 'rating': return (((a as Movie).criticScore ?? 0) - ((b as Movie).criticScore ?? 0)) * d;
      case 'runtime': return (((a as Movie).runtime ?? 0) - ((b as Movie).runtime ?? 0)) * d;
      case 'played': return ((st(a.id).lastPlayedAt ?? 0) - (st(b.id).lastPlayedAt ?? 0)) * d;
      case 'random': return (hash(a.id) % 997) - (hash(b.id) % 997);
      default: return a.sortTitle.localeCompare(b.sortTitle) * d;
    }
  };
  const browseIds = (q: BrowseQuery) => libItems(q.libraryId, q.kind).filter(i => matches(i, q)).sort(sorter(q)).map(i => i.id);
  const letterOf = (i: Item) => {
    const c = i.sortTitle.charAt(0).toUpperCase();
    return /[A-Z]/.test(c) ? c : '#';
  };

  // --- Search
  const norm = (s: string) => s.toLowerCase().normalize('NFKD').replace(/[̀-ͯ]/g, '');

  const services: Services = {
    subscribe(l) {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    version: () => version,

    auth: {
      async servers() { await wait(); return db.servers; },
      async account() { return db.account; },
      async signIn(u, p) { await wait(500); if (!u || !p) throw new Error('Enter your username or email and your password.'); if (p === 'wrong') throw new Error('That username or email and password didn’t match a Portico Account. Check them and try again.'); return {needsCode: db.account.twoStep}; },
      async signInDirect(a, u, p) { await wait(600); if (!a) throw new Error('That isn’t a server address Portico can use. Check it and try again. It usually looks like 192.168.1.20:32500.'); if (!u || !p) throw new Error('Enter your username and password for your server.'); return {needsCode: false, mustChangePassword: u === 'newuser'}; },
      async verifyCode(code) { await wait(400); if (code.replace(/\s/g, '').length < 6) throw new Error('That code didn’t work. Enter the current code from your authenticator app, or a recovery code.'); },
      async createAccount() { await wait(700); },
      async requestPasswordReset() { await wait(500); },
      async resetPassword() { await wait(500); },
      async verifyEmail(token) { await wait(700); return token === 'expired' ? 'expired' : 'done'; },
      async profiles() { await wait(); return db.profiles; },
      async unlockProfile(id, pin) { await wait(250); const p = db.profiles.find(x => x.id === id); return !p?.pin || p.pin === pin; },
      async tvLinkCode() { await wait(); return {code: 'KQ7-MXP', url: 'getportico.tv/link', expiresAt: NOW + 15 * 60_000}; },
      async lookupDevice(code) { await wait(500); return code.replace(/[^A-Z0-9]/gi, '').length >= 6 ? {name: 'Living Room Apple TV', platform: 'Apple TV', location: 'Portland, US · Home network'} : null; },
      async approveDevice() { await wait(500); },
      async invitation() { await wait(); return {server: 'Sam’s Film Club', inviter: 'Sam Okafor', libraries: 4}; },
      async acceptInvitation() { await wait(500); },
      async joinDirect(code) { await wait(600); if (code.length < 6) throw new Error('Check the invitation code and username: 3 to 64 characters, no spaces or @.'); },
      async claim() { await wait(); return {server: 'Ehler Home Theater', address: '192.168.1.20:32500', requested: NOW - 2 * 60_000}; },
      async setupServer() { await wait(900); },
      async signOut() { await wait(300); },
    },

    catalog: {
      async libraries() { await wait(90); return db.libraries; },
      async item(id) { await wait(80); return get(id); },
      async items(ids) { await wait(60); return ids.map(get).filter(Boolean) as Item[]; },
      async person(id) {
        await wait(120);
        const p = db.people.get(id);
        if (p) return p;
        // Audiobook authors are people too.
        const book = [...db.items.values()].find(i => i.kind === 'audiobook' && (i as Audiobook).authorId === id) as Audiobook | undefined;
        return book ? {id, kind: 'person', name: book.author, art: {}, tint: book.tint, bio: book.authorBio, knownFor: 'Writing'} : undefined;
      },
      async personCredits(id) {
        await wait(120);
        const c = db.credits.get(id);
        if (c) return c;
        const books = [...db.items.values()].filter(i => i.kind === 'audiobook' && (i as Audiobook).authorId === id).map(i => ({itemId: i.id, job: 'Author'}));
        return {cast: [], crew: books};
      },
      async home() {
        await wait(260);
        const rows = buildRows();
        const layout = db.homeLayout ?? defaultLayout(rows);
        const byId = new Map(rows.map(r => [r.id, r]));
        return layout.filter(l => l.visible).map(l => byId.get(l.rowId)).filter((r): r is Row => !!r && r.itemIds.length > 0);
      },
      async homeLayout() {
        await wait(120);
        const rows = buildRows();
        const layout = db.homeLayout ?? defaultLayout(rows);
        const known = new Set(layout.map(l => l.rowId));
        return [...layout, ...rows.filter(r => !known.has(r.id)).map(r => ({rowId: r.id, visible: false}))];
      },
      async setHomeLayout(layout) { await wait(150); db.homeLayout = layout; bump(); },
      async row(id) {
        await wait(150);
        const rows = buildRows();
        return rows.find(r => r.id === id);
      },
      async discover(libraryId) {
        await wait(240);
        const all = libItems(libraryId);
        const lib = db.libraries.find(l => l.id === libraryId)!;
        const r = rng(libraryId + ':discover');
        const shape = lib.kind === 'music' ? 'square' : lib.kind === 'photos' || lib.kind === 'homevideos' ? 'landscape' : 'poster';
        const rows: Row[] = [];
        if (lib.kind === 'music') {
          const albums = libItems(libraryId, 'releases');
          rows.push({id: `${libraryId}-recent`, kind: 'recent', title: 'Recently added', itemIds: [...albums].sort((a, b) => b.addedAt - a.addedAt).slice(0, 16).map(i => i.id), shape: 'square'});
          rows.push({id: `${libraryId}-played`, kind: 'continueListening', title: 'Recently played', itemIds: albums.filter(a => st(a.id).lastPlayedAt).map(a => a.id), shape: 'square'});
          rows.push({id: `${libraryId}-artists`, kind: 'custom', title: 'Artists you love', itemIds: all.slice(0, 16).map(a => a.id), shape: 'circle'});
          const genres = [...new Set(albums.flatMap(a => a.genres))].slice(0, 4);
          genres.forEach(g => rows.push({id: `${libraryId}-g-${slug(g)}`, kind: 'genre', title: g, itemIds: albums.filter(a => a.genres.includes(g)).map(a => a.id).slice(0, 16), shape: 'square'}));
          rows.push({id: `${libraryId}-playlists`, kind: 'custom', title: 'Your playlists', itemIds: [], shape: 'square'});
          return rows.filter(x => x.itemIds.length);
        }
        const cont = continueIds().filter(id => get(id)?.libraryId === libraryId);
        if (cont.length) rows.push({id: `${libraryId}-continue`, kind: 'continue', title: lib.kind === 'audiobooks' ? 'Continue Listening' : 'Continue Watching', itemIds: cont, shape: 'landscape'});
        if (lib.kind === 'audiobooks') {
          const books = all as Audiobook[];
          const inProg = books.filter(b => st(b.id).progress > 0);
          if (inProg.length) rows.push({id: `${libraryId}-continue`, kind: 'continueListening', title: 'Continue Listening', itemIds: inProg.map(b => b.id), shape: 'poster'});
        }
        rows.push({id: `${libraryId}-recent`, kind: 'recent', title: 'Recently added', itemIds: [...all].sort((a, b) => b.addedAt - a.addedAt).slice(0, 18).map(i => i.id), shape});
        if (lib.kind === 'movies' || lib.kind === 'shows' || lib.kind === 'anime') {
          rows.push({id: `${libraryId}-picks`, kind: 'picks', title: 'Picks for you', itemIds: r.shuffle(all.filter(i => !st(i.id).watched)).slice(0, 18).map(i => i.id), shape, recommendation: true});
          const genres = [...new Set(all.flatMap(i => i.genres))];
          r.shuffle(genres).slice(0, 5).forEach(g => rows.push({id: `${libraryId}-g-${slug(g)}`, kind: 'genre', title: g, itemIds: all.filter(i => i.genres.includes(g)).slice(0, 18).map(i => i.id), shape}));
          if (lib.kind === 'movies') {
            rows.push({id: `${libraryId}-decade`, kind: 'custom', title: 'From the vault', subject: 'before 1970', itemIds: all.filter(i => (i.year ?? 3000) < 1970).map(i => i.id), shape});
            rows.push({id: `${libraryId}-unwatched4k`, kind: 'custom', title: 'Stunning in', subject: '4K', itemIds: all.filter(i => 'versions' in i && i.versions.some(v => v.resolution === '4K')).slice(0, 18).map(i => i.id), shape});
          }
        } else if (lib.kind === 'audiobooks') {
          rows.push({id: `${libraryId}-classics`, kind: 'custom', title: 'Timeless classics', itemIds: all.filter(i => (i as Audiobook).publicDomain).map(i => i.id), shape});
          rows.push({id: `${libraryId}-series`, kind: 'custom', title: 'Series to start', itemIds: all.filter(i => (i as Audiobook).series).map(i => i.id), shape});
        } else {
          const years = [...new Set(all.map(i => i.year))].sort((a, b) => (b ?? 0) - (a ?? 0));
          years.forEach(y => rows.push({id: `${libraryId}-y-${y}`, kind: 'custom', title: String(y), itemIds: all.filter(i => i.year === y).map(i => i.id), shape}));
        }
        return rows.filter(x => x.itemIds.length);
      },
      async browse(q): Promise<Page<Item>> {
        await wait(200);
        const ids = browseIds(q);
        const size = q.pageSize ?? 60;
        const start = q.cursor ? parseInt(q.cursor, 10) : 0;
        return {items: ids.slice(start, start + size).map(get) as Item[], total: ids.length, cursor: start + size < ids.length ? String(start + size) : null};
      },
      async letters(q) {
        const ids = browseIds(q);
        const out: {letter: string; index: number}[] = [];
        ids.forEach((id, i) => {
          const l = letterOf(get(id)!);
          if (!out.some(o => o.letter === l)) out.push({letter: l, index: i});
        });
        return out;
      },
      async facets(libraryId) {
        await wait(120);
        const all = libItems(libraryId);
        const count = (vals: string[]) => {
          const m = new Map<string, number>();
          vals.filter(Boolean).forEach(v => m.set(v, (m.get(v) ?? 0) + 1));
          return [...m.entries()].map(([value, c]) => ({value, count: c})).sort((a, b) => b.count - a.count);
        };
        return {
          genre: count(all.flatMap(i => i.genres)),
          decade: count(all.map(i => decadeOf(i.year))).sort((a, b) => a.value.localeCompare(b.value)),
          contentRating: count(all.map(i => ('contentRating' in i ? i.contentRating : ''))),
          studio: count(all.map(i => ('studio' in i ? i.studio ?? '' : ''))).slice(0, 30),
          network: count(all.map(i => ('network' in i ? i.network : ''))),
          language: count(all.map(i => ('language' in i ? i.language ?? '' : ''))),
          mood: count(all.map(i => ('mood' in i ? i.mood ?? '' : ''))),
          resolution: count(all.flatMap(i => ('versions' in i ? [...new Set(i.versions.map(v => v.resolution))] : []))),
        };
      },
      async categories(libraryId) {
        await wait(160);
        const all = libItems(libraryId);
        const out: {id: string; title: string; kind: 'genre' | 'studio' | 'network' | 'decade' | 'mood' | 'country'; count: number; art?: string; tint: string}[] = [];
        const group = (kind: 'genre' | 'studio' | 'network' | 'decade' | 'mood' | 'country', val: (i: Item) => string[]) => {
          const m = new Map<string, Item[]>();
          all.forEach(i => val(i).filter(Boolean).forEach(v => m.set(v, [...(m.get(v) ?? []), i])));
          [...m.entries()].sort((a, b) => b[1].length - a[1].length).forEach(([v, its]) => {
            const lead = its.find(x => x.art.backdrop) ?? its[0];
            out.push({id: `${kind}:${v}`, title: v, kind, count: its.length, art: lead.art.backdrop ?? lead.art.poster ?? lead.art.square, tint: lead.tint});
          });
        };
        group('genre', i => i.genres);
        group('mood', i => ('mood' in i && i.mood ? [i.mood] : []));
        group('decade', i => [decadeOf(i.year)]);
        group('studio', i => ('studio' in i && i.studio ? [i.studio] : []));
        group('network', i => ('network' in i ? [i.network] : []));
        return out;
      },
      async entity(libraryId, view, entityId) {
        await wait(180);
        const [kind, ...rest] = entityId.split(':');
        const value = rest.join(':');
        if (view === 'collection') {
          const c = db.collections.find(x => x.id === entityId);
          return {title: c?.title ?? 'Collection', subtitle: c?.description ?? '', itemIds: c?.itemIds ?? [], tint: c?.tint ?? '#409fd4', art: c && get(c.itemIds[0])?.art.backdrop};
        }
        const all = libItems(libraryId, kind === 'author' || kind === 'series' ? undefined : undefined);
        const pick = all.filter(i => {
          if (kind === 'genre') return i.genres.includes(value);
          if (kind === 'mood') return 'mood' in i && i.mood === value;
          if (kind === 'decade') return decadeOf(i.year) === value;
          if (kind === 'studio') return 'studio' in i && i.studio === value;
          if (kind === 'network') return 'network' in i && i.network === value;
          if (kind === 'author') return i.kind === 'audiobook' && (i as Audiobook).author === value;
          if (kind === 'series') return i.kind === 'audiobook' && (i as Audiobook).series?.name === value;
          return false;
        });
        const sorted = kind === 'series' ? pick.sort((a, b) => ((a as Audiobook).series?.number ?? 0) - ((b as Audiobook).series?.number ?? 0)) : pick.sort((a, b) => a.sortTitle.localeCompare(b.sortTitle));
        const label = {genre: 'Genre', mood: 'Mood', decade: 'Decade', studio: 'Studio', network: 'Network', author: 'Author', series: 'Series'}[kind] ?? 'Category';
        const lead = sorted.find(i => i.art.backdrop) ?? sorted[0];
        return {title: value, subtitle: `${label} · ${sorted.length} ${sorted.length === 1 ? 'title' : 'titles'}`, itemIds: sorted.map(i => i.id), art: lead?.art.backdrop, tint: lead?.tint ?? '#409fd4'};
      },
      async collections(libraryId) { await wait(140); return db.collections.filter(c => !libraryId || c.libraryId === libraryId); },
      async related(id) {
        await wait(150);
        const it = get(id);
        if (!it) return [];
        const pool = libItems(it.libraryId).filter(x => x.id !== id);
        return pool.map(x => ({x, score: x.genres.filter(g => it.genres.includes(g)).length * 10 + (hash(x.id + id) % 7)})).sort((a, b) => b.score - a.score).slice(0, 16).map(o => o.x.id);
      },
      async children(id) {
        const it = get(id);
        if (!it) return [];
        if (it.kind === 'show') return it.seasonIds;
        if (it.kind === 'season') return it.episodeIds;
        if (it.kind === 'album') return it.trackIds;
        if (it.kind === 'artist') return it.albumIds;
        if (it.kind === 'photoAlbum') return it.photoIds;
        return [];
      },
      async nextUp(showId) { return nextUp(showId) ?? episodesOf(showId)[0]; },
      async search(query, kind) {
        await wait(220);
        const q = norm(query.trim());
        if (q.length < 2) return [];
        const hit = (s: string) => norm(s).includes(q);
        const groups: {kind: string; label: string; ids: string[]; total: number}[] = [];
        const push = (k: string, label: string, ids: string[]) => {
          if (ids.length && (kind === 'all' || kind === k)) groups.push({kind: k, label, ids: kind === 'all' ? ids.slice(0, 12) : ids, total: ids.length});
        };
        const all = [...db.items.values()];
        push('movies', 'Movies', all.filter(i => (i.kind === 'movie' || i.kind === 'homevideo') && hit(i.title)).map(i => i.id));
        push('shows', 'Shows', all.filter(i => i.kind === 'show' && hit(i.title)).map(i => i.id));
        push('episodes', 'Episodes', all.filter(i => i.kind === 'episode' && hit(i.title)).map(i => i.id));
        push('people', 'People', [...db.people.values()].filter(p => hit(p.name)).map(p => p.id));
        push('artists', 'Artists', all.filter(i => i.kind === 'artist' && hit(i.title)).map(i => i.id));
        push('albums', 'Albums', all.filter(i => i.kind === 'album' && hit(i.title)).map(i => i.id));
        push('songs', 'Songs', all.filter(i => i.kind === 'track' && hit(i.title)).map(i => i.id));
        push('books', 'Books', all.filter(i => i.kind === 'audiobook' && (hit(i.title) || hit((i as Audiobook).author))).map(i => i.id));
        push('channels', 'Channels', db.channels.filter(c => hit(c.name) || hit(c.number)).map(c => c.id));
        return groups;
      },
      async recentSearches() { return db.recentSearches; },
      async addRecentSearch(q) { db.recentSearches = [q, ...db.recentSearches.filter(x => x !== q)].slice(0, 8); bump(); },
      async clearRecentSearches() { db.recentSearches = []; bump(); },
      durationOf(id) {
        const it = get(id);
        if (!it) return 1800;
        if (it.kind === 'movie' || it.kind === 'homevideo' || it.kind === 'episode') return it.runtime * 60;
        if (it.kind === 'track') return it.duration;
        if (it.kind === 'audiobook') return it.duration;
        return 1800;
      },
      async lyrics(trackId) {
        const t = get(trackId);
        if (!t || t.kind !== 'track') return null;
        return db.lyrics[`${t.albumId}::${t.title}`] ?? null;
      },
      async scanLibrary(libraryId) {
        await wait(200);
        const lib = db.libraries.find(l => l.id === libraryId);
        if (lib) lib.scanning = true;
        bump();
        setTimeout(() => {
          if (lib) lib.scanning = false;
          bump();
        }, 9000);
      },
    },

    user: {
      state: st,
      async set(ids, patch) {
        ids.forEach(id => {
          const extra: Partial<UserState> = {};
          if (patch.watched === true) Object.assign(extra, {progress: 0, position: 0, lastPlayedAt: NOW});
          if (patch.watched === false) Object.assign(extra, {progress: 0, position: 0});
          setSt(id, {...patch, ...extra});
          const it = get(id);
          if (patch.watched !== undefined && it?.kind === 'show') episodesOf(id).forEach(e => setSt(e, {watched: patch.watched!, progress: 0}));
          if (patch.watched && it) db.history.unshift({itemId: id, at: Date.now()});
        });
        bump();
      },
      async markContainerWatched(id, watched) {
        const it = get(id);
        const eps = it?.kind === 'show' ? episodesOf(id) : it?.kind === 'season' ? it.episodeIds : [];
        eps.forEach(e => setSt(e, {watched, progress: 0, position: 0}));
        bump();
        return eps.length;
      },
      async removeFromContinue(id) { db.continueHidden.add(id); bump(); },
      async restoreContinue(id) { db.continueHidden.delete(id); bump(); },
      async resetRecommendations() { await wait(400); [...db.state.values()].forEach(s => (s.notInterested = false)); bump(); },
      async history(period) {
        await wait(150);
        const span = {'24h': DAY, '7d': 7 * DAY, '30d': 30 * DAY, '90d': 90 * DAY}[period] ?? Infinity;
        const seen = new Set<string>();
        return db.history.filter(h => Date.now() - h.at <= span && !seen.has(h.itemId) && (seen.add(h.itemId), true));
      },
      async clearHistory() { db.history.length = 0; bump(); },
    },

    lists: {
      async playlists() { await wait(120); return db.playlists; },
      async playlist(id) { await wait(100); return db.playlists.find(p => p.id === id); },
      async createPlaylist(title, description, shared, itemIds = []) {
        const itemsIn = itemIds.map(get).filter(Boolean) as Item[];
        const p: Playlist = {id: 'pl-' + slug(title) + '-' + (version % 1000), kind: 'playlist', title, description, itemIds, shared, media: itemsIn.every(i => i.kind === 'track' || i.kind === 'album') ? 'music' : 'video', updatedAt: Date.now(), tint: itemsIn[0]?.tint ?? '#409fd4'};
        db.playlists.unshift(p);
        bump();
        return p;
      },
      async addToPlaylist(id, ids) {
        const p = db.playlists.find(x => x.id === id);
        if (!p) throw new Error('Playlist not found');
        const expanded = ids.flatMap(x => (get(x)?.kind === 'album' ? (get(x) as {trackIds: string[]}).trackIds : [x]));
        p.itemIds.push(...expanded);
        p.updatedAt = Date.now();
        bump();
      },
      async removeFromPlaylist(id, index) { const p = db.playlists.find(x => x.id === id); p?.itemIds.splice(index, 1); bump(); },
      async movePlaylistItem(id, from, to) {
        const p = db.playlists.find(x => x.id === id);
        if (!p || to < 0 || to >= p.itemIds.length) return;
        const [x] = p.itemIds.splice(from, 1);
        p.itemIds.splice(to, 0, x);
        bump();
      },
      async renamePlaylist(id, title) { const p = db.playlists.find(x => x.id === id); if (p) p.title = title; bump(); },
      async deletePlaylist(id) { db.playlists = db.playlists.filter(p => p.id !== id); bump(); },
      async createCollection(title, description, shared, itemIds = []) {
        const c: Collection = {id: 'col-' + slug(title) + '-' + (version % 1000), kind: 'collection', title, description, itemIds, shared, owner: 'me', tint: get(itemIds[0])?.tint ?? '#409fd4'};
        db.collections.unshift(c);
        bump();
        return c;
      },
      async addToCollection(id, ids) { const c = db.collections.find(x => x.id === id); c?.itemIds.push(...ids.filter(x => !c.itemIds.includes(x))); bump(); },
      async removeFromCollection(id, itemId) { const c = db.collections.find(x => x.id === id); if (c) c.itemIds = c.itemIds.filter(x => x !== itemId); bump(); },
      async renameCollection(id, title) { const c = db.collections.find(x => x.id === id); if (c) c.title = title; bump(); },
      async deleteCollection(id) { db.collections = db.collections.filter(c => c.id !== id); bump(); },
      async savedViews() { await wait(100); return db.savedViews; },
      async saveView(v) { const sv = {...v, id: 'sv-' + slug(v.title) + '-' + (version % 1000)}; db.savedViews.unshift(sv); bump(); return sv; },
      async renameView(id, title) { const v = db.savedViews.find(x => x.id === id); if (v) v.title = title; bump(); },
      async deleteView(id) { db.savedViews = db.savedViews.filter(v => v.id !== id); bump(); },
    },

    live: {
      async channels() { await wait(150); return db.channels; },
      async guide(from, to) { await wait(200); return db.programmes.filter(p => p.end > from && p.start < to); },
      async programme(id) { return db.programmes.find(p => p.id === id); },
      async setChannel(id, patch) { const c = db.channels.find(x => x.id === id) as Channel | undefined; if (c) Object.assign(c, patch); bump(); },
      async recordings() { await wait(150); return db.recordings; },
      async rules() { await wait(120); return db.rules; },
      async record(programmeId, opts) {
        await wait(250);
        const p = db.programmes.find(x => x.id === programmeId);
        if (!p) return;
        if (opts.series) {
          const rule: RecordingRule = {id: 'rule-' + slug(p.title), title: p.title, channelId: p.channelId, newOnly: opts.newOnly ?? true, keep: opts.keep ?? 'all', startEarly: opts.startEarly ?? 0, endLate: opts.endLate ?? 0, paused: false, priority: 1, scheduledCount: 0};
          db.rules.unshift(rule);
          const upcoming = db.programmes.filter(x => x.title === p.title && x.start >= p.start).slice(0, 6);
          upcoming.forEach(u => !db.recordings.some(r => r.programmeId === u.id) && db.recordings.push({id: 'rec-' + u.id, programmeId: u.id, channelId: u.channelId, title: u.title, episodeTitle: u.episodeTitle, start: u.start, end: u.end, state: u.start <= Date.now() ? 'recording' : 'scheduled', kept: false, sizeGB: 0, watched: false, ruleId: rule.id}));
          rule.scheduledCount = upcoming.length;
        } else if (!db.recordings.some(r => r.programmeId === programmeId)) {
          db.recordings.push({id: 'rec-' + p.id, programmeId: p.id, channelId: p.channelId, title: p.title, episodeTitle: p.episodeTitle, start: p.start, end: p.end, state: p.start <= Date.now() ? 'recording' : 'scheduled', kept: false, sizeGB: 0, watched: false});
        }
        bump();
      },
      async cancelRecording(id) { const r = db.recordings.find(x => x.id === id); if (r) { r.state = 'cancelled'; r.reason = 'Canceled by this profile.'; } bump(); },
      async deleteRecording(id) { db.recordings = db.recordings.filter(r => r.id !== id); bump(); },
      async keepRecording(id, keep) { const r = db.recordings.find(x => x.id === id); if (r) r.kept = keep; bump(); },
      async resolveConflict(id, choice) { const r = db.recordings.find(x => x.id === id); if (r) { r.state = choice === 'record' ? 'scheduled' : 'cancelled'; r.reason = choice === 'skip' ? 'Skipped to resolve a tuner conflict.' : undefined; } bump(); },
      async updateRule(id, patch) { const r = db.rules.find(x => x.id === id); if (r) Object.assign(r, patch); bump(); },
      async deleteRule(id, cancelUpcoming) {
        db.rules = db.rules.filter(r => r.id !== id);
        if (cancelUpcoming) db.recordings.filter(r => r.ruleId === id && r.state === 'scheduled').forEach(r => (r.state = 'cancelled'));
        bump();
      },
      async reminders() { return [...db.reminders]; },
      async toggleReminder(id) { const on = !db.reminders.has(id); if (on) db.reminders.add(id); else db.reminders.delete(id); bump(); return on; },
      async storage() { await wait(100); const used = db.recordings.reduce((a, r) => a + r.sizeGB, 0) + 412; return {usedGB: +used.toFixed(1), totalGB: 2000, hoursLeft: Math.round((2000 - used) / 3.4), retentionDays: 30, pendingDeleteGB: 4.2}; },
    },

    downloads: {
      async list() { await wait(120); return db.downloads; },
      async requests() { await wait(100); return db.downloadRequests; },
      async settings() { return settings; },
      async updateSettings(patch) { Object.assign(settings, patch); bump(); },
      async add(ids, quality) {
        await wait(300);
        let n = 0;
        ids.forEach(id => {
          if (db.downloads.some(d => d.itemId === id && d.where === 'device')) return;
          db.downloads.unshift({id: 'dl-' + id + '-' + version, itemId: id, quality, sizeMB: 700 + (hash(id) % 2800), progress: 0.02, state: 'downloading', where: 'device'});
          n++;
        });
        bump();
        return n;
      },
      async addShow(showId, policy, keepNext) {
        await wait(300);
        const total = policy === 'next' ? keepNext : episodesOf(showId).filter(e => policy === 'all' || !st(e).watched).length;
        db.downloadRequests.unshift({id: 'dr-' + showId + version, showId, policy, keepNext, ready: 0, total, failed: 0});
        bump();
      },
      async pause(id) { const d = db.downloads.find(x => x.id === id); if (d) d.state = 'paused'; bump(); },
      async resume(id) { const d = db.downloads.find(x => x.id === id); if (d) d.state = d.where === 'server' ? 'preparing' : 'downloading'; bump(); },
      async remove(id) { db.downloads = db.downloads.filter(d => d.id !== id); db.downloadRequests = db.downloadRequests.filter(d => d.id !== id); bump(); },
      async storage() { const used = db.downloads.filter(d => d.where === 'device').reduce((a, d) => a + d.sizeMB * d.progress, 0) / 1024; return {usedGB: +used.toFixed(1), freeGB: 86.4, serverSpaceGB: 212}; },
    },

    account: {
      async profiles() { await wait(120); return db.profiles; },
      async saveProfile(p) { const i = db.profiles.findIndex(x => x.id === p.id); if (i >= 0) db.profiles[i] = p; bump(); },
      async addProfile(name, color, kids) {
        const p: Profile = {id: 'pr-' + slug(name) + version, name, color, initials: name.charAt(0).toUpperCase(), kids, pin: null, main: false, limits: kids ? {maxRating: 'PG', allowUnrated: false, downloads: true, liveTv: false, dvr: false, together: false, hiddenLabels: []} : {maxRating: null, allowUnrated: true, downloads: true, liveTv: true, dvr: true, together: true, hiddenLabels: []}};
        db.profiles.push(p);
        bump();
        return p;
      },
      async deleteProfile(id) { db.profiles = db.profiles.filter(p => p.id !== id); bump(); },
      async moveProfile(id, dir) {
        const i = db.profiles.findIndex(p => p.id === id);
        const j = i + dir;
        if (i < 0 || j < 0 || j >= db.profiles.length) return;
        [db.profiles[i], db.profiles[j]] = [db.profiles[j], db.profiles[i]];
        bump();
      },
      async sessions() { await wait(120); return db.sessions; },
      async signOutSession(id) { db.sessions = db.sessions.filter(s => s.id !== id); bump(); },
      async signOutOthers() { db.sessions = db.sessions.filter(s => s.current); bump(); },
      async updateSession(id, patch) { const s = db.sessions.find(x => x.id === id) as DeviceSession | undefined; if (s) Object.assign(s, patch); bump(); },
      async account() { return db.account; },
      async updateAccount(patch) { Object.assign(db.account, patch); bump(); },
      async notifications() { await wait(120); return db.notifications; },
      async updateNotification(id, patch) { const n = db.notifications.find(x => x.id === id); if (n) Object.assign(n, patch); bump(); },
      async markAllRead() { db.notifications.forEach(n => (n.read = true)); bump(); },
      async invitations() { await wait(120); return db.invitations; },
      async answerInvitation(id, accept) { const i = db.invitations.find(x => x.id === id); if (i) i.state = accept ? 'accepted' : 'declined'; bump(); },
      async servers() { await wait(120); return db.servers; },
      async leaveServer(id) { db.servers = db.servers.filter(s => s.id !== id); bump(); },
    },

    together: {
      async groups() { await wait(120); return db.togetherGroups; },
      async group(id) { await wait(80); return db.togetherGroups.find(g => g.id === id); },
      async create(name, control) {
        await wait(300);
        const g: TogetherGroup = {id: 'tg-' + version, name, code: 'QRT' + (hash(name) % 100000).toString().padStart(5, '7'), state: 'lobby', control, members: [{id: 'me', name: 'Justin', color: '#409fd4', status: 'inStep', host: true, you: true, ready: true}], queue: [], nowPlaying: null, position: 0};
        db.togetherGroups.unshift(g);
        bump();
        return g;
      },
      async join(code) { await wait(400); return db.togetherGroups.find(g => g.code === code.replace(/\s/g, '').toUpperCase()) ?? (code.replace(/\s/g, '').length === 8 ? db.togetherGroups[0] : null); },
      async update(id, patch) { const g = db.togetherGroups.find(x => x.id === id); if (g) Object.assign(g, patch); bump(); },
      async leave(id) { db.togetherGroups = db.togetherGroups.filter(g => g.id !== id); bump(); },
      async end(id) { const g = db.togetherGroups.find(x => x.id === id); if (g) g.state = 'ended'; bump(); },
      async addToQueue(id, itemId) { const g = db.togetherGroups.find(x => x.id === id) ?? db.togetherGroups[0]; if (g && !g.queue.includes(itemId)) g.queue.push(itemId); bump(); },
    },

    feedback: {
      async send(input) { await wait(500); return input.message.toLowerCase().includes('again') ? 'duplicate' : 'sent'; },
      async reports() { return []; },
      async updateReport() {},
    },

    admin: createAdmin(db, bump),
  };
  const settings: DownloadSettings = {wifiOnly: true, removeAfterWatching: false, nextEpisode: true, limitGB: 32, quality: '1080p'};
  return services;
}

export type FixtureDB = ReturnType<typeof buildDB>;
export {HOUR};
