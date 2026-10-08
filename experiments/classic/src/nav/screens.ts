import type {PorticoClient} from '../data/services';
import {push, root} from './nav';
import type {AuthRoutes} from './routes';

type Hooks = {
  auth: (route: keyof AuthRoutes, params?: object) => void;
  enterProfile: (id: string) => Promise<void>;
  actions: () => {more: (e: never) => void; play: (e: never, o?: object) => Promise<void>; editMetadata: (e: never[]) => void; feedback: (e?: never) => void} | undefined;
};
let hooks: Hooks | undefined;
export function registerScreenHooks(h: Hooks) {
  hooks = h;
}

type Def = {id: string; label: string; group: string; open: (c: PorticoClient) => void | Promise<void>};
const movie = 'm-metropolis-1927';
const genMovie = 'm-glasshouse';
const show = 's-harbor-station';
const entry = (c: PorticoClient, id: string) => c.catalog.entry(id) as never;

/** The review index: every screen and notable state, by id. */
export const SCREENS: Def[] = [
  {id: 'landing', label: 'Landing', group: 'Getting in', open: () => hooks?.auth('Landing')},
  {id: 'sign-in', label: 'Portico Account sign-in', group: 'Getting in', open: () => hooks?.auth('AccountSignIn')},
  {id: 'create-account', label: 'Create account', group: 'Getting in', open: () => hooks?.auth('AccountSignIn', {mode: 'create'})},
  {id: 'forgot-password', label: 'Forgot password', group: 'Getting in', open: () => hooks?.auth('AccountSignIn', {mode: 'forgot'})},
  {id: 'two-step', label: 'Two-step code', group: 'Getting in', open: () => hooks?.auth('TwoStep', {next: 'servers'})},
  {id: 'direct-sign-in', label: 'Direct server sign-in', group: 'Getting in', open: () => hooks?.auth('DirectSignIn')},
  {id: 'server-chooser', label: 'Choose a server (Hosted chooser)', group: 'Getting in', open: () => hooks?.auth('ServerChooser')},
  {id: 'profiles', label: 'Profile grid with PIN', group: 'Getting in', open: () => hooks?.auth('Profiles')},
  {id: 'join', label: 'Join a server by invitation', group: 'Getting in', open: () => hooks?.auth('Join', {code: 'K7QD2MXF'})},
  {id: 'activate', label: 'Activate this TV (code and QR)', group: 'Getting in', open: () => hooks?.auth('Activate')},
  {id: 'quick-connect', label: 'Quick Connect (direct server code)', group: 'Getting in', open: () => hooks?.auth('QuickConnect')},
  {id: 'link-tv', label: 'Link a TV / device approval', group: 'Getting in', open: () => push('SettingsSection', {section: 'linkTv'})},
  {id: 'claim', label: 'Claim a server', group: 'Getting in', open: () => hooks?.auth('Claim')},
  {id: 'setup', label: 'Server setup (first run)', group: 'Getting in', open: () => hooks?.auth('Setup')},
  {id: 'restore', label: 'Restore saved connection', group: 'Getting in', open: () => hooks?.auth('Restore')},
  {id: 'verify-email', label: 'Email verification', group: 'Getting in', open: () => hooks?.auth('VerifyEmail', {token: 'ok'})},
  {id: 'reset-password', label: 'Password reset', group: 'Getting in', open: () => hooks?.auth('ResetPassword', {token: 'ok'})},
  {id: 'device-approval', label: 'Waiting for device approval', group: 'Getting in', open: () => hooks?.auth('DeviceApproval')},
  {id: 'server-unavailable', label: 'Server unavailable', group: 'Getting in', open: () => hooks?.auth('ServerUnavailable', {serverId: 'srv-annex'})},
  {id: 'offline', label: 'Offline (with downloads)', group: 'Getting in', open: () => hooks?.auth('Offline')},

  {id: 'home', label: 'Home', group: 'Browse', open: () => root('Home')},
  {id: 'home-row', label: 'Home row · See all', group: 'Browse', open: () => push('HomeRow', {rowId: 'picks', title: 'Picks for you'})},
  {id: 'library-movies', label: 'Movies library · Discover', group: 'Browse', open: () => root('Library', {libraryId: 'lib-movies'})},
  {id: 'library-browse', label: 'Movies library · Browse (filters, sort, A–Z)', group: 'Browse', open: () => root('Library', {libraryId: 'lib-movies', view: 'browse'})},
  {id: 'library-collections', label: 'Movies library · Collections', group: 'Browse', open: () => root('Library', {libraryId: 'lib-movies', view: 'collections'})},
  {id: 'library-categories', label: 'Movies library · Categories', group: 'Browse', open: () => root('Library', {libraryId: 'lib-movies', view: 'categories'})},
  {id: 'library-tv', label: 'TV Shows library', group: 'Browse', open: () => root('Library', {libraryId: 'lib-tv'})},
  {id: 'library-anime', label: 'Anime library', group: 'Browse', open: () => root('Library', {libraryId: 'lib-anime'})},
  {id: 'library-music', label: 'Music library', group: 'Browse', open: () => root('Library', {libraryId: 'lib-music'})},
  {id: 'library-songs', label: 'Music · Songs', group: 'Browse', open: () => root('Library', {libraryId: 'lib-music', view: 'songs'})},
  {id: 'library-audiobooks', label: 'Audiobooks library', group: 'Browse', open: () => root('Library', {libraryId: 'lib-audiobooks'})},
  {id: 'library-photos', label: 'Photos library', group: 'Browse', open: () => root('Library', {libraryId: 'lib-photos'})},
  {id: 'library-homevideos', label: 'Home Videos library', group: 'Browse', open: () => root('Library', {libraryId: 'lib-homevideos'})},
  {id: 'entity-genre', label: 'Genre page', group: 'Browse', open: () => push('Entity', {libraryId: 'lib-movies', kind: 'genre', id: 'Drama', title: 'Drama'})},
  {id: 'entity-studio', label: 'Studio page', group: 'Browse', open: () => push('Entity', {libraryId: 'lib-movies', kind: 'studio', id: 'Northlight Pictures', title: 'Northlight Pictures'})},
  {id: 'entity-collection', label: 'Collection page', group: 'Browse', open: () => push('Entity', {libraryId: 'lib-movies', kind: 'collection', id: 'col-expressionism', title: 'German Expressionism'})},
  {id: 'search', label: 'Search', group: 'Browse', open: () => root('Search', {query: 'har'})},

  {id: 'movie', label: 'Movie detail (real film)', group: 'Titles', open: () => push('Movie', {id: movie})},
  {id: 'movie-generated', label: 'Movie detail (versions, extras)', group: 'Titles', open: () => push('Movie', {id: genMovie})},
  {id: 'movie-blender', label: 'Movie detail (Blender open movie)', group: 'Titles', open: () => push('Movie', {id: 'm-sintel-2010'})},
  {id: 'show', label: 'Show detail', group: 'Titles', open: () => push('Show', {id: show})},
  {id: 'episode', label: 'Episode page', group: 'Titles', open: () => push('Episode', {id: `${show}-s1e3`})},
  {id: 'person', label: 'Person page', group: 'Titles', open: () => push('Person', {id: 'p-buster-keaton'})},
  {id: 'artist', label: 'Artist', group: 'Titles', open: c => push('Artist', {id: (c.catalog.entry('ar-the-lantern-keepers') ?? {id: 'ar-the-lantern-keepers'}).id})},
  {id: 'album', label: 'Album', group: 'Titles', open: async c => { const a = await c.catalog.artist('ar-the-lantern-keepers'); push('Album', {id: a.albums[0]!.id}); }},
  {id: 'audiobook', label: 'Audiobook', group: 'Titles', open: () => push('Audiobook', {id: 'b-the-adventures-of-sherlock-holmes'})},
  {id: 'photo-album', label: 'Photo album', group: 'Titles', open: () => push('PhotoAlbum', {id: 'pa-summer-trip'})},
  {id: 'home-video', label: 'Home video', group: 'Titles', open: () => push('HomeVideo', {id: 'hv-beach-day-2024'})},
  {id: 'playlist', label: 'Playlist', group: 'Titles', open: () => push('Resource', {kind: 'playlist', id: 'pl-road'})},
  {id: 'collection', label: 'Personal collection', group: 'Titles', open: () => push('Resource', {kind: 'collection', id: 'pc-date'})},
  {id: 'saved-view', label: 'Saved view', group: 'Titles', open: () => push('Resource', {kind: 'view', id: 'sv-unwatched-noir'})},

  {id: 'actions', label: 'Actions menu (long press)', group: 'Dialogs', open: c => hooks?.actions()?.more(entry(c, movie))},
  {id: 'metadata-editor', label: 'Metadata editor', group: 'Dialogs', open: c => hooks?.actions()?.editMetadata([entry(c, genMovie)])},
  {id: 'feedback', label: 'Report a problem', group: 'Dialogs', open: c => hooks?.actions()?.feedback(entry(c, movie))},

  {id: 'player', label: 'Video player', group: 'Playback', open: c => hooks?.actions()?.play(entry(c, genMovie))},
  {id: 'player-episode', label: 'Episode player (skip intro, up next)', group: 'Playback', open: c => hooks?.actions()?.play(entry(c, `${show}-s1e2`))},
  {id: 'player-live', label: 'Live player (mini guide, captions)', group: 'Playback', open: () => hooks?.actions()?.play({id: 'ch-harbor-one', kind: 'channel', title: 'The Evening Six', subtitle: '2.1 · Harbor One', shape: 'wide'} as never)},
  {id: 'player-channel-problem', label: 'Live channel that can’t start (tuners busy)', group: 'Playback', open: () => hooks?.actions()?.play({id: 'ch-kestrel-sports', kind: 'channel', title: 'Sports Desk', subtitle: '11.1 · Kestrel Sports', shape: 'wide'} as never)},
  {id: 'now-playing', label: 'Music now playing (lyrics, queue)', group: 'Playback', open: async c => { const a = await c.catalog.artist('ar-the-lantern-keepers'); await hooks?.actions()?.play(a.albums[0] as never); }},
  {id: 'audiobook-player', label: 'Audiobook player (chapters, speed, sleep)', group: 'Playback', open: c => hooks?.actions()?.play(entry(c, 'b-the-adventures-of-sherlock-holmes'))},

  {id: 'live-guide', label: 'Live TV guide', group: 'Live TV', open: () => root('Live', {view: 'guide'})},
  {id: 'live-channels', label: 'Channels', group: 'Live TV', open: () => root('Live', {view: 'channels'})},
  {id: 'live-recordings', label: 'Recordings (DVR)', group: 'Live TV', open: () => root('Live', {view: 'recordings'})},

  {id: 'saved', label: 'Saved', group: 'You', open: () => root('Saved')},
  {id: 'saved-history', label: 'Saved · History', group: 'You', open: () => root('Saved', {tab: 'history'})},
  {id: 'together', label: 'Watch Together', group: 'You', open: () => root('Together')},
  {id: 'downloads', label: 'Downloads', group: 'You', open: () => root('Downloads')},
  {id: 'settings', label: 'Settings', group: 'You', open: () => root('Settings')},
  {id: 'preferences', label: 'Preferences', group: 'You', open: () => push('SettingsSection', {section: 'preferences'})},
  {id: 'notifications', label: 'Notifications', group: 'You', open: () => push('SettingsSection', {section: 'notifications'})},
  {id: 'profiles-settings', label: 'Profiles (PIN, limits)', group: 'You', open: () => push('SettingsSection', {section: 'profiles'})},
  {id: 'security', label: 'Security (two-step)', group: 'You', open: () => push('SettingsSection', {section: 'security'})},
  {id: 'devices', label: 'Devices', group: 'You', open: () => push('SettingsSection', {section: 'devices'})},
  {id: 'servers', label: 'Servers', group: 'You', open: () => push('SettingsSection', {section: 'servers'})},
  {id: 'about', label: 'About', group: 'You', open: () => push('SettingsSection', {section: 'about'})},
  {id: 'account', label: 'Portico Account', group: 'You', open: () => root('Account')},

  ...['overview', 'now-playing', 'activity', 'logs', 'diagnostics', 'feedback', 'alerts', 'libraries', 'library-settings', 'metadata', 'storage', 'live-tv', 'dvr', 'channels', 'channel-builder', 'transcoding', 'network', 'people', 'access', 'schedules', 'maintenance', 'backups', 'general'].map(s => ({id: `admin-${s}`, label: `Manage server · ${s.replace(/-/g, ' ')}`, group: 'Server administration', open: () => root('Admin', {section: s})})),
  {id: 'admin-library-detail', label: 'Manage server · library detail', group: 'Server administration', open: () => root('Admin', {section: 'libraries', param: 'lib-tv'})},

  {id: 'state-reconnecting', label: 'Reconnecting banner', group: 'States', open: c => c.session.setConnectivity('reconnecting')},
  {id: 'state-offline', label: 'Offline banner', group: 'States', open: c => c.session.setConnectivity('offline')},
  {id: 'state-online', label: 'Back online', group: 'States', open: c => c.session.setConnectivity('online')},
  {id: 'state-kids', label: 'Kids profile (Theo)', group: 'States', open: async () => { await hooks?.enterProfile('pr-theo'); root('Home'); }},
  {id: 'state-owner', label: 'Owner profile (Justin)', group: 'States', open: async () => { await hooks?.enterProfile('pr-justin'); root('Home'); }},
  {id: 'state-member', label: 'Member profile (Nora, PIN 1234)', group: 'States', open: () => hooks?.auth('Profiles')},
  {id: 'state-empty', label: 'Empty and error states', group: 'States', open: () => push('Entity', {libraryId: 'lib-movies', kind: 'genre', id: 'Opera', title: 'Opera'})},
  {id: 'not-found', label: 'Not found', group: 'States', open: () => push('NotFound')},
];

export function openScreen(id: string, client: PorticoClient) {
  const def = SCREENS.find(s => s.id === id);
  if (def) void def.open(client);
  return !!def;
}
