import type {Services} from '../data/services';
import type {Item} from '../data/types';
import {ui} from '../design/overlays';
import {player} from '../state/player';
import {go, goSection, replaceAll, type RouteName} from './routes';

/**
 * Every screen and state, reachable for review: `portico-cinema://screen/<id>` or Settings › About ›
 * Screen index. Screens that need a title pick a representative one from the library.
 */
export type ScreenEntry = {id: string; label: string; group: string; open: (s: Services) => Promise<void> | void};

const first = async (s: Services, lib: string, kind?: Item['kind']) => {
  const page = await s.catalog.browse({libraryId: lib, filters: {quick: [], fields: {}}, sort: {field: 'title', direction: 'asc'}, pageSize: 400});
  return page.items.find(i => !kind || i.kind === kind);
};
const nav = (route: RouteName, params?: object) => () => go(route as never, params as never);
const section = (route: RouteName, params?: object) => () => goSection(route as never, params as never);

export const screenIndex: ScreenEntry[] = [
  // Getting in
  {id: 'landing', label: 'Landing', group: 'Getting in', open: () => replaceAll('Landing')},
  {id: 'signin', label: 'Portico Account sign-in', group: 'Getting in', open: nav('SignIn', {mode: 'account'})},
  {id: 'signin-create', label: 'Create a Portico Account', group: 'Getting in', open: nav('SignIn', {mode: 'create'})},
  {id: 'signin-forgot', label: 'Forgot password', group: 'Getting in', open: nav('SignIn', {mode: 'forgot'})},
  {id: 'signin-code', label: 'Two-step code', group: 'Getting in', open: nav('SignIn', {mode: 'code'})},
  {id: 'signin-direct', label: 'Direct server sign-in', group: 'Getting in', open: nav('SignIn', {mode: 'direct'})},
  {id: 'signin-new-password', label: 'Choose a new password', group: 'Getting in', open: nav('SignIn', {mode: 'newPassword'})},
  {id: 'signin-recovery', label: 'Owner recovery sign-in', group: 'Getting in', open: nav('SignIn', {mode: 'recovery'})},
  {id: 'chooser', label: 'Choose a server', group: 'Getting in', open: nav('Chooser')},
  {id: 'profiles', label: 'Who’s watching (profiles + PIN)', group: 'Getting in', open: nav('Profiles', {switching: true})},
  {id: 'device-waiting', label: 'Waiting for device approval', group: 'Getting in', open: nav('DeviceWaiting')},
  {id: 'join', label: 'Join by invitation', group: 'Getting in', open: nav('Join')},
  {id: 'join-direct', label: 'Join a server directly', group: 'Getting in', open: nav('Join', {direct: true})},
  {id: 'link-tv', label: 'Link a TV (code + QR)', group: 'Getting in', open: nav('LinkTV')},
  {id: 'device-approve', label: 'Approve a device code', group: 'Getting in', open: nav('DeviceApprove')},
  {id: 'claim', label: 'Claim a server', group: 'Getting in', open: nav('Claim')},
  {id: 'setup', label: 'Server setup (first run)', group: 'Getting in', open: nav('Setup')},
  {id: 'restore', label: 'Restore saved connection', group: 'Getting in', open: nav('Restore')},
  {id: 'verify-email', label: 'Email verification', group: 'Getting in', open: nav('VerifyEmail')},
  {id: 'reset-password', label: 'Password reset', group: 'Getting in', open: nav('ResetPassword')},
  {id: 'account-return', label: 'Finishing sign-in', group: 'Getting in', open: nav('AccountReturn')},
  {id: 'server-unavailable', label: 'Server unavailable', group: 'Getting in', open: nav('ServerUnavailable')},
  {id: 'offline', label: 'Offline', group: 'Getting in', open: nav('Offline')},
  {id: 'signin-ended', label: 'Sign-in ended', group: 'Getting in', open: nav('SignInEnded')},
  // Home
  {id: 'home', label: 'Home', group: 'Home', open: section('Home')},
  {id: 'home-row', label: 'Row: See all', group: 'Home', open: nav('HomeRow', {rowId: 'picks'})},
  {id: 'home-customise', label: 'Customize Home', group: 'Home', open: nav('HomeCustomise')},
  {id: 'home-empty', label: 'Home: empty state', group: 'Home', open: nav('HomeEmpty')},
  // Libraries
  {id: 'libraries', label: 'Libraries', group: 'Libraries', open: section('Libraries')},
  {id: 'library-movies', label: 'Movies: Discover', group: 'Libraries', open: section('Library', {libraryId: 'lib-movies'})},
  {id: 'library-browse', label: 'Movies: Browse', group: 'Libraries', open: section('Library', {libraryId: 'lib-movies', view: 'browse'})},
  {id: 'library-collections', label: 'Movies: Collections', group: 'Libraries', open: section('Library', {libraryId: 'lib-movies', view: 'collections'})},
  {id: 'library-categories', label: 'Movies: Categories', group: 'Libraries', open: section('Library', {libraryId: 'lib-movies', view: 'categories'})},
  {id: 'library-tv', label: 'TV Shows', group: 'Libraries', open: section('Library', {libraryId: 'lib-tv'})},
  {id: 'library-anime', label: 'Anime (scanning)', group: 'Libraries', open: section('Library', {libraryId: 'lib-anime'})},
  {id: 'library-music', label: 'Music', group: 'Libraries', open: section('Library', {libraryId: 'lib-music'})},
  {id: 'library-songs', label: 'Music: Songs', group: 'Libraries', open: section('Library', {libraryId: 'lib-music', view: 'songs'})},
  {id: 'library-books', label: 'Audiobooks', group: 'Libraries', open: section('Library', {libraryId: 'lib-books'})},
  {id: 'library-authors', label: 'Audiobooks: Authors', group: 'Libraries', open: section('Library', {libraryId: 'lib-books', view: 'authors'})},
  {id: 'library-photos', label: 'Photos', group: 'Libraries', open: section('Library', {libraryId: 'lib-photos'})},
  {id: 'library-homevideos', label: 'Home Videos', group: 'Libraries', open: section('Library', {libraryId: 'lib-home'})},
  {id: 'entity-genre', label: 'Entity: genre', group: 'Libraries', open: nav('Entity', {libraryId: 'lib-movies', view: 'categories', entityId: 'genre:Drama'})},
  {id: 'entity-collection', label: 'Entity: collection', group: 'Libraries', open: nav('Entity', {libraryId: 'lib-movies', view: 'collection', entityId: 'col-public-domain-classics'})},
  {id: 'no-libraries', label: 'No libraries (owner)', group: 'Libraries', open: nav('NoLibraries')},
  {id: 'no-libraries-member', label: 'No libraries (member)', group: 'Libraries', open: nav('NoLibraries', {member: true})},
  // Titles
  {id: 'movie', label: 'Movie detail', group: 'Titles', open: nav('Movie', {id: 'm-charade'})},
  {id: 'movie-invented', label: 'Movie detail (4K, versions)', group: 'Titles', open: async s => { const m = (await first(s, 'lib-movies', 'movie')) as Item | undefined; const page = await s.catalog.browse({libraryId: 'lib-movies', filters: {quick: ['uhd'], fields: {}}, sort: {field: 'rating', direction: 'desc'}}); go('Movie', {id: page.items[0]?.id ?? m?.id ?? 'm-charade'}); }},
  {id: 'show', label: 'Show detail', group: 'Titles', open: async s => { const it = await first(s, 'lib-tv', 'show'); if (it) go('Show', {id: it.id}); }},
  {id: 'episode', label: 'Episode page', group: 'Titles', open: async s => { const it = await first(s, 'lib-tv', 'show'); if (it) go('Episode', {id: `${it.id}-s1e2`}); }},
  {id: 'anime', label: 'Anime detail', group: 'Titles', open: async s => { const it = await first(s, 'lib-anime', 'show'); if (it) go('Show', {id: it.id}); }},
  {id: 'person', label: 'Person page', group: 'Titles', open: nav('Person', {id: 'p-cary-grant'})},
  {id: 'metadata-editor', label: 'Metadata editor', group: 'Titles', open: nav('MetadataEditor', {id: 'm-charade'})},
  {id: 'feedback', label: 'Report a problem', group: 'Titles', open: nav('Feedback', {itemId: 'm-charade'})},
  {id: 'item-menu', label: 'Title More menu', group: 'Titles', open: async s => { const {openItemMenu} = await import('../screens/shared/actions'); const it = await s.catalog.item('m-charade'); if (it) openItemMenu(s, it, {origin: 'recommendation', manager: true}); }},
  // Music, books, photos
  {id: 'artist', label: 'Artist', group: 'Music', open: async s => { const it = await first(s, 'lib-music', 'artist'); if (it) go('Artist', {id: it.id}); }},
  {id: 'album', label: 'Album', group: 'Music', open: async s => { const p = await s.catalog.browse({libraryId: 'lib-music', kind: 'releases', filters: {quick: [], fields: {}}, sort: {field: 'title', direction: 'asc'}}); if (p.items[0]) go('Album', {id: p.items[0].id}); }},
  {id: 'playlist', label: 'Playlist', group: 'Music', open: nav('Resource', {kind: 'playlist', id: 'pl-late-night-drive'})},
  {id: 'now-playing', label: 'Now playing + lyrics + queue', group: 'Music', open: async s => { const p = await s.catalog.browse({libraryId: 'lib-music', kind: 'releases', filters: {quick: [], fields: {}}, sort: {field: 'title', direction: 'asc'}}); const al = p.items[0] as {trackIds?: string[]} | undefined; if (al?.trackIds) { player.play(al.trackIds, {kind: 'audio'}); go('NowPlaying'); } }},
  {id: 'audiobook', label: 'Audiobook', group: 'Audiobooks', open: async s => { const it = await first(s, 'lib-books', 'audiobook'); if (it) go('Audiobook', {id: it.id}); }},
  {id: 'photo-album', label: 'Photo album', group: 'Photos', open: async s => { const it = await first(s, 'lib-photos', 'photoAlbum'); if (it) go('PhotoAlbum', {id: it.id}); }},
  {id: 'photo-viewer', label: 'Photo viewer + slideshow', group: 'Photos', open: async s => { const it = await first(s, 'lib-photos', 'photoAlbum'); if (it) go('PhotoViewer', {albumId: it.id, index: 0, slideshow: true}); }},
  // Search, saved
  {id: 'search', label: 'Search', group: 'Search & Saved', open: section('Search')},
  {id: 'search-results', label: 'Search results', group: 'Search & Saved', open: section('Search', {query: 'the'})},
  {id: 'saved', label: 'Saved', group: 'Search & Saved', open: section('Saved')},
  {id: 'saved-history', label: 'Saved: History', group: 'Search & Saved', open: section('Saved', {tab: 'history'})},
  {id: 'resource-collection', label: 'Collection page', group: 'Search & Saved', open: nav('Resource', {kind: 'collection', id: 'col-my-comfort'})},
  {id: 'resource-view', label: 'Saved view page', group: 'Search & Saved', open: nav('Resource', {kind: 'view', id: 'sv-unwatched-4k'})},
  // Live
  {id: 'live', label: 'Live TV: Guide', group: 'Live TV', open: section('Live', {tab: 'guide'})},
  {id: 'live-channels', label: 'Live TV: Channels', group: 'Live TV', open: section('Live', {tab: 'channels'})},
  {id: 'dvr', label: 'DVR', group: 'Live TV', open: section('Live', {tab: 'dvr'})},
  {id: 'live-player', label: 'Live player + mini guide', group: 'Live TV', open: async s => { const ch = await s.live.channels(); if (ch[0]) go('LivePlayer', {channelId: ch[0].id}); }},
  // Player
  {id: 'player', label: 'Video player', group: 'Player', open: nav('Player', {id: 'm-charade', from: 1260})},
  {id: 'player-episode', label: 'Player: episode (skip intro, next up)', group: 'Player', open: async s => { const it = await first(s, 'lib-tv', 'show'); if (it) go('Player', {id: `${it.id}-s1e1`, from: 20}); }},
  {id: 'player-postplay', label: 'Player: post-play countdown', group: 'Player', open: async s => { const it = await first(s, 'lib-tv', 'show'); if (it) { const d = s.catalog.durationOf(`${it.id}-s1e1`); go('Player', {id: `${it.id}-s1e1`, from: d - 40}); } }},
  // Together, downloads
  {id: 'together', label: 'Watch Together', group: 'Together & Downloads', open: section('Together')},
  {id: 'together-group', label: 'Watch Together group', group: 'Together & Downloads', open: nav('TogetherGroup', {id: 'tg-1'})},
  {id: 'together-join', label: 'Join a group by link', group: 'Together & Downloads', open: nav('TogetherJoin', {code: 'KXWP7M2Q'})},
  {id: 'downloads', label: 'Downloads', group: 'Together & Downloads', open: section('Downloads')},
  // Account & settings
  {id: 'notifications', label: 'Notifications', group: 'Account & Settings', open: section('Notifications')},
  {id: 'account', label: 'Account: profiles', group: 'Account & Settings', open: nav('Account', {tab: 'profiles'})},
  {id: 'account-security', label: 'Account: security', group: 'Account & Settings', open: nav('Account', {tab: 'security'})},
  {id: 'account-sessions', label: 'Account: where you’re signed in', group: 'Account & Settings', open: nav('Account', {tab: 'sessions'})},
  {id: 'account-devices', label: 'Account: devices', group: 'Account & Settings', open: nav('Account', {tab: 'devices'})},
  {id: 'account-servers', label: 'Account: servers', group: 'Account & Settings', open: nav('Account', {tab: 'servers'})},
  {id: 'account-invitations', label: 'Account: invitations', group: 'Account & Settings', open: nav('Account', {tab: 'invitations'})},
  {id: 'account-notifications', label: 'Account: notification settings', group: 'Account & Settings', open: nav('Account', {tab: 'notifications'})},
  {id: 'settings', label: 'Settings', group: 'Account & Settings', open: section('Settings')},
  {id: 'settings-preferences', label: 'Preferences', group: 'Account & Settings', open: nav('SettingsPreferences')},
  {id: 'settings-profiles', label: 'Profiles', group: 'Account & Settings', open: nav('SettingsProfiles')},
  {id: 'profile-limits', label: 'Profile limits', group: 'Account & Settings', open: nav('ProfileLimits', {profileId: 'pr-kids'})},
  {id: 'settings-servers', label: 'Servers', group: 'Account & Settings', open: nav('SettingsServers')},
  {id: 'settings-about', label: 'About', group: 'Account & Settings', open: nav('SettingsAbout')},
  // Admin
  ...['overview', 'now-playing', 'activity', 'logs', 'diagnostics', 'feedback', 'libraries', 'library-settings', 'metadata', 'storage', 'live-tv', 'dvr', 'channels', 'channel-builder', 'transcoding', 'network', 'people', 'access', 'schedules', 'maintenance', 'backups', 'alerts', 'general'].map(sec => ({id: 'admin/' + sec, label: 'Server: ' + sec.replace(/-/g, ' '), group: 'Server administration', open: section('Admin', {section: sec})})),
  {id: 'admin-denied', label: 'Server: members can’t manage', group: 'Server administration', open: nav('AdminDenied')},
  // States
  {id: 'states', label: 'Loading, empty and error states', group: 'States', open: nav('States')},
  {id: 'reconnecting', label: 'Reconnecting banner', group: 'States', open: () => reconnect()},
  {id: 'not-found', label: 'Not found', group: 'States', open: nav('NotFound')},
  {id: 'toast', label: 'Toast with undo', group: 'States', open: () => ui.toast('Removed “Charade” from Continue Watching.', {action: {label: 'Undo', onPress: () => ui.toast('Put back.')}})},
];

let reconnectHook: (() => void) | null = null;
export function setReconnectHook(fn: () => void) {
  reconnectHook = fn;
}
function reconnect() {
  reconnectHook?.();
}
