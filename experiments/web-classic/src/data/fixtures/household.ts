import type {Account, Device, Invitation, Notice, Personal, Profile, Server} from '../types';
import {catalog, episodes, movies, shows, books, albums} from './catalog';
import {hash, rng, shuffle} from './util';

/** The fixture household: one Portico Account, three servers, four profiles. */
export const account: Account = {id: 'acct-1', username: 'justin', displayName: 'Justin Ehler', email: 'justin@ehler.family', kind: 'hosted', twoStep: true, recoveryCodesLeft: 8, providers: ['apple']};

export const servers: Server[] = [
  {id: 'srv-lighthouse', name: 'Lighthouse', host: 'lighthouse.local:32500', status: 'online', owner: 'Justin', version: '1.4.2', lastSeen: 'Just now', yours: true, libraries: 8},
  {id: 'srv-cabin', name: 'The Cabin', host: 'cabin.portico.direct', status: 'online', owner: 'Maya Ehler', version: '1.4.0', lastSeen: '2 minutes ago', yours: false, libraries: 3},
  {id: 'srv-annex', name: 'Studio Annex', host: 'annex.portico.direct', status: 'unreachable', owner: 'Justin', version: '1.3.7', lastSeen: '3 days ago', yours: true, libraries: 2},
];

export const profilesByServer: Record<string, Profile[]> = {
  'srv-lighthouse': [
    {id: 'pr-justin', name: 'Justin', art: 'blue', pin: false, kids: false, role: 'owner', main: true},
    {id: 'pr-maya', name: 'Maya', art: 'violet', pin: true, kids: false, role: 'admin'},
    {id: 'pr-nora', name: 'Nora', art: 'mint', pin: true, kids: false, role: 'member', limits: {rating: 'PG-13', unrated: true, downloads: true, liveTv: true, dvr: false, together: true}},
    {id: 'pr-theo', name: 'Theo', art: 'gold', pin: false, kids: true, role: 'member', limits: {rating: 'PG', unrated: false, downloads: false, liveTv: true, dvr: false, together: false}},
  ],
  'srv-cabin': [
    {id: 'pr-cabin-justin', name: 'Justin', art: 'blue', pin: false, kids: false, role: 'member', main: true},
    {id: 'pr-cabin-theo', name: 'Theo', art: 'gold', pin: false, kids: true, role: 'member'},
  ],
  'srv-annex': [{id: 'pr-annex-justin', name: 'Justin', art: 'blue', pin: false, kids: false, role: 'owner', main: true}],
};

/** Fixture PINs (all profiles with a PIN use 1234). */
export const PIN = '1234';

export const devices: Device[] = [
  {id: 'dev-this', name: 'Chrome on Mac', platform: 'web', app: 'Portico web', lastSeen: 'Now', firstSeen: 'Sep 2, 2026', current: true, trusted: true, state: 'approved', location: 'Home network'},
  {id: 'dev-tv', name: 'Living Room Apple TV', platform: 'tvos', app: 'Portico', lastSeen: '12 minutes ago', firstSeen: 'Mar 14, 2025', current: false, trusted: true, state: 'approved', location: 'Home network'},
  {id: 'dev-mac', name: 'Safari on MacBook Pro', platform: 'web', app: 'Portico web', lastSeen: 'Yesterday', firstSeen: 'Jan 3, 2025', current: false, trusted: true, state: 'approved', location: 'Home network'},
  {id: 'dev-ipad', name: "Maya's iPad", platform: 'ios', app: 'Portico', lastSeen: '3 days ago', firstSeen: 'Jun 9, 2025', current: false, trusted: false, state: 'approved', location: 'Portland, OR'},
  {id: 'dev-shield', name: 'SHIELD Android TV', platform: 'androidtv', app: 'Portico', lastSeen: '2 weeks ago', firstSeen: 'Nov 22, 2025', current: false, trusted: false, state: 'approved', location: 'Cabin network'},
  {id: 'dev-new', name: 'Chrome on Windows', platform: 'web', app: 'Portico web', lastSeen: '5 minutes ago', firstSeen: 'Today', current: false, trusted: false, state: 'waiting', location: 'Denver, CO'},
];

export const invitations: Invitation[] = [
  {id: 'inv-1', server: 'Grandpa’s Film Vault', from: 'Walter Ehler', libraries: 2, expires: 'Oct 3, 2026', state: 'waiting'},
  {id: 'inv-2', server: 'The Cabin', from: 'Maya Ehler', libraries: 3, expires: 'Aug 1, 2026', state: 'accepted'},
  {id: 'inv-3', server: 'Book Club Audio', from: 'Nadia Petrov', libraries: 1, expires: 'Jul 12, 2026', state: 'expired'},
];

const H = 3600 * 1000;
export function seedNotices(): Notice[] {
  const now = Date.now();
  return [
    {id: 'n1', title: 'Reply to your report about “Harbor Station”', body: 'Thanks! The missing audio on S2 E4 is fixed. Play it again from the start.', at: now - 1.2 * H, read: false, archived: false, audience: 'profile', kind: 'feedback'},
    {id: 'n2', title: 'Your download is ready', body: '“Sintel” is ready to watch offline.', at: now - 3 * H, read: false, archived: false, audience: 'profile', kind: 'download'},
    {id: 'n3', title: 'Recording finished', body: 'Evening News at Six on Harbor One was recorded (58 min).', at: now - 5 * H, read: false, archived: false, audience: 'profile', kind: 'recording'},
    {id: 'n4', title: 'Studio Annex has been quiet', body: 'Portico hasn’t heard from Studio Annex for 3 days. Check its power and network connection.', at: now - 26 * H, read: true, archived: false, audience: 'admin', kind: 'server', importance: 'notice'},
    {id: 'n5', title: 'New device waiting for approval', body: 'Chrome on Windows signed in to Lighthouse and is waiting for you under Server › Access.', at: now - 0.2 * H, read: false, archived: false, audience: 'admin', kind: 'security', importance: 'urgent'},
    {id: 'n6', title: 'The server will restart Saturday morning', body: 'Lighthouse restarts at 7 am for an update. It should take about five minutes.', at: now - 50 * H, read: true, archived: false, audience: 'profile', kind: 'broadcast', importance: 'info'},
    {id: 'n7', title: 'Storage is nearly full on Lighthouse', body: 'The Media volume has 8% free. Free up space before adding more media.', at: now - 80 * H, read: true, archived: true, audience: 'admin', kind: 'server', importance: 'notice'},
    {id: 'n8', title: 'Maya accepted your invitation', body: 'Maya Ehler joined Lighthouse.', at: now - 200 * H, read: true, archived: true, audience: 'admin', kind: 'security'},
  ];
}

/** Seeds a believable watch history for a profile. */
export function seedPersonal(profileId: string, kids: boolean): Map<string, Personal> {
  const r = rng(hash(profileId));
  const map = new Map<string, Personal>();
  const base = (): Personal => ({watched: false, progress: 0, watchlisted: false, favorite: false});
  const set = (id: string, patch: Partial<Personal>) => map.set(id, {...(map.get(id) ?? base()), ...patch});
  const now = Date.now();
  const movieList = catalog.movies.filter(m => !kids || m.library === 'lib-family' || ['G', 'PG'].includes(m.rating));
  const shuffled = shuffle(r, movieList);
  shuffled.slice(0, 48).forEach((m, i) => set(m.id, {watched: true, lastPlayed: now - (i + 2) * 26 * H, rating: r() < 0.5 ? 3 + Math.floor(r() * 3) : undefined}));
  shuffled.slice(48, 56).forEach((m, i) => set(m.id, {progress: Math.floor(m.runtime * 60 * (0.15 + r() * 0.7)), lastPlayed: now - (i + 0.5) * 9 * H}));
  shuffled.slice(56, 78).forEach(m => set(m.id, {watchlisted: true}));
  shuffled.slice(0, 14).forEach(m => set(m.id, {favorite: true}));
  // Shows in progress: watched a run of episodes, one partly watched.
  const showList = shuffle(r, catalog.shows.filter(s => !kids || ['TV-Y7', 'TV-PG'].includes(s.rating))).slice(0, 9);
  showList.forEach((s, si) => {
    const all = s.seasons.filter(x => x.number > 0).flatMap(x => x.episodes);
    const upto = si < 3 ? Math.min(all.length - 2, 3 + Math.floor(r() * 10)) : si < 6 ? all.length : Math.floor(r() * 4);
    all.slice(0, upto).forEach((e, i) => set(e.id, {watched: true, lastPlayed: now - (si * 30 + (upto - i)) * H}));
    const cur = all[upto];
    if (cur && si < 4) set(cur.id, {progress: Math.floor(cur.runtime * 60 * (0.2 + r() * 0.5)), lastPlayed: now - (si + 1) * 3 * H});
    if (si === 1 || si === 4) set(s.id, {favorite: true});
    if (si === 7) set(s.id, {watchlisted: true});
  });
  shuffle(r, catalog.shows).slice(0, 5).forEach(s => set(s.id, {watchlisted: map.get(s.id)?.watchlisted || r() < 0.8}));
  // Music and audiobooks.
  shuffle(r, catalog.albums).slice(0, 8).forEach(a => set(a.id, {favorite: true}));
  shuffle(r, catalog.books).slice(0, 3).forEach((b, i) => {
    const total = b.chapters.reduce((n, c) => n + c.duration, 0);
    set(b.id, {progress: Math.floor(total * (0.1 + 0.25 * (i + 1))), lastPlayed: now - (i + 1) * 20 * H});
  });
  return map;
}

export function isAllowed(profile: Profile, rating: string | undefined, library?: string): boolean {
  if (!profile.kids) return true;
  if (library && ['lib-music', 'lib-audiobooks', 'lib-photos', 'lib-homevideos', 'lib-family'].includes(library)) return true;
  return ['G', 'PG', 'TV-Y7', 'TV-PG', 'TV-Y'].includes(rating ?? '');
}

export const allEpisodesOf = (showId: string) => shows.get(showId)!.seasons.filter(s => s.number > 0).flatMap(s => s.episodes);
export {movies, episodes, books, albums};
