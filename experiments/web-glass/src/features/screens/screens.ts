// Every screen and state, for the screen index page and for `npm run shots`.
export interface ScreenDef {
  group: string;
  name: string;
  path: string;
  as?: string; // profile to sign in as ('signed-out' for the sign-in flow)
  note?: string;
}

export const SCREENS: ScreenDef[] = [
  { group: 'Onboarding', name: 'Welcome', path: '/welcome', as: 'signed-out' },
  { group: 'Onboarding', name: 'Sign in', path: '/signin', as: 'signed-out' },
  { group: 'Onboarding', name: 'Choose server', path: '/servers', as: 'none' },
  { group: 'Onboarding', name: 'Who’s watching', path: '/profiles', as: 'none' },
  { group: 'Onboarding', name: 'Profile PIN', path: '/profiles?unlock=maya', as: 'none', note: 'Fixture PIN 1234' },
  { group: 'Onboarding', name: 'Server setup · name', path: '/setup/welcome', as: 'signed-out' },
  { group: 'Onboarding', name: 'Server setup · libraries', path: '/setup/libraries', as: 'signed-out' },
  { group: 'Onboarding', name: 'Server setup · remote access', path: '/setup/remote', as: 'signed-out' },
  { group: 'Onboarding', name: 'Server setup · finish', path: '/setup/done', as: 'signed-out' },

  { group: 'Home & Browse', name: 'Home', path: '/', as: 'maya' },
  { group: 'Home & Browse', name: 'Home · kids profile', path: '/', as: 'theo' },
  { group: 'Home & Browse', name: 'Movies · browse', path: '/movies', as: 'maya' },
  { group: 'Home & Browse', name: 'Movies · library grid', path: '/movies?view=library', as: 'maya' },
  { group: 'Home & Browse', name: 'Movies · list, by date added', path: '/movies?view=library&layout=list&sort=added&dir=desc', as: 'maya' },
  { group: 'Home & Browse', name: 'Movies · collections', path: '/movies?view=collections', as: 'maya' },
  { group: 'Home & Browse', name: 'TV Shows', path: '/tv', as: 'maya' },
  { group: 'Home & Browse', name: 'Anime · all series', path: '/anime?view=library', as: 'maya' },
  { group: 'Home & Browse', name: 'Library hub', path: '/library', as: 'maya' },
  { group: 'Home & Browse', name: 'Search · empty', path: '/search', as: 'maya' },
  { group: 'Home & Browse', name: 'Search · results', path: '/search?q=cary%20grant', as: 'maya' },
  { group: 'Home & Browse', name: 'Search · no results', path: '/search?q=zzzqx', as: 'maya' },

  { group: 'Details', name: 'Movie', path: '/movie/deep-field', as: 'maya' },
  { group: 'Details', name: 'Movie · open movie (CC BY)', path: '/movie/coffee-run', as: 'maya' },
  { group: 'Details', name: 'Movie · public domain classic', path: '/movie/charade', as: 'maya' },
  { group: 'Details', name: 'Show', path: '/show/meridian', as: 'maya' },
  { group: 'Details', name: 'Anime series', path: '/show/sora-no-kagami', as: 'maya' },
  { group: 'Details', name: 'Episode', path: '/episode/meridian-s3e4', as: 'maya' },
  { group: 'Details', name: 'Collection', path: '/collection/col-blender', as: 'maya' },
  { group: 'Details', name: 'Person', path: '/person/p-anna-lindgren', as: 'maya' },
  { group: 'Details', name: 'Home video', path: '/home-video/hv-mormor', as: 'maya' },

  { group: 'Playback', name: 'Video player · episode', path: '/watch/meridian-s3e4?t=30', as: 'maya' },
  { group: 'Playback', name: 'Video player · film', path: '/watch/coffee-run', as: 'maya' },
  { group: 'Playback', name: 'Video player · live channel', path: '/watch/ch-7', as: 'maya' },

  { group: 'Music & Books', name: 'Music · listen now', path: '/music', as: 'maya' },
  { group: 'Music & Books', name: 'Music · songs', path: '/music?view=songs', as: 'maya' },
  { group: 'Music & Books', name: 'Music · artists', path: '/music?view=artists', as: 'maya' },
  { group: 'Music & Books', name: 'Album', path: '/album/northern-static', as: 'maya' },
  { group: 'Music & Books', name: 'Artist', path: '/artist/aurora-vale', as: 'maya' },
  { group: 'Music & Books', name: 'Playlist', path: '/playlist/pl-sunday-morning', as: 'maya' },
  { group: 'Music & Books', name: 'Smart playlist', path: '/playlist/pl-top-25', as: 'maya' },
  { group: 'Music & Books', name: 'Audiobooks', path: '/audiobooks', as: 'maya' },
  { group: 'Music & Books', name: 'Audiobook', path: '/audiobook/pride-and-prejudice', as: 'maya' },

  { group: 'Photos & Video', name: 'Photos · timeline', path: '/photos', as: 'maya' },
  { group: 'Photos & Video', name: 'Photos · albums', path: '/photos?view=albums', as: 'maya' },
  { group: 'Photos & Video', name: 'Photos · memories', path: '/photos?view=memories', as: 'maya' },
  { group: 'Photos & Video', name: 'Photo viewer', path: '/photos?photo=ph-182', as: 'maya' },
  { group: 'Photos & Video', name: 'Home videos', path: '/home-videos', as: 'maya' },

  { group: 'Live TV', name: 'Guide', path: '/live', as: 'maya' },
  { group: 'Live TV', name: 'Recordings', path: '/live/dvr', as: 'maya' },

  { group: 'Yours', name: 'Watchlist', path: '/watchlist', as: 'maya' },
  { group: 'Yours', name: 'Watchlist · empty filter', path: '/watchlist?kind=show', as: 'theo' },
  { group: 'Yours', name: 'Downloads · offline mode', path: '/downloads?offline=1', as: 'maya' },
  { group: 'Yours', name: 'History', path: '/history', as: 'maya' },
  { group: 'Yours', name: 'Downloads', path: '/downloads', as: 'maya' },

  { group: 'Settings', name: 'Account', path: '/settings/account', as: 'maya' },
  { group: 'Settings', name: 'Profiles & parental controls', path: '/settings/profiles', as: 'maya' },
  { group: 'Settings', name: 'Playback & quality', path: '/settings/playback', as: 'maya' },
  { group: 'Settings', name: 'Sign-in & security', path: '/settings/security', as: 'maya' },
  { group: 'Settings', name: 'Appearance & accessibility', path: '/settings/appearance', as: 'maya' },
  { group: 'Settings', name: 'Settings · kids profile', path: '/settings/security', as: 'theo' },

  { group: 'Server Console', name: 'Overview', path: '/console', as: 'maya' },
  { group: 'Server Console', name: 'Now playing', path: '/console/sessions', as: 'maya' },
  { group: 'Server Console', name: 'Activity', path: '/console/activity', as: 'maya' },
  { group: 'Server Console', name: 'Logs', path: '/console/logs', as: 'maya' },
  { group: 'Server Console', name: 'Libraries', path: '/console/libraries', as: 'maya' },
  { group: 'Server Console', name: 'Metadata', path: '/console/metadata', as: 'maya' },
  { group: 'Server Console', name: 'Live TV & DVR', path: '/console/livetv', as: 'maya' },
  { group: 'Server Console', name: 'Users & sharing', path: '/console/users', as: 'maya' },
  { group: 'Server Console', name: 'Transcoding', path: '/console/transcoding', as: 'maya' },
  { group: 'Server Console', name: 'Network', path: '/console/network', as: 'maya' },
  { group: 'Server Console', name: 'Storage', path: '/console/storage', as: 'maya' },
  { group: 'Server Console', name: 'Backups', path: '/console/backups', as: 'maya' },
  { group: 'Server Console', name: 'Scheduled tasks', path: '/console/tasks', as: 'maya' },
  { group: 'Server Console', name: 'Diagnostics', path: '/console/diagnostics', as: 'maya' },
  { group: 'Server Console', name: 'Server settings', path: '/console/settings', as: 'maya' },
  { group: 'Server Console', name: 'Console · non-admin', path: '/console', as: 'noor' },

  { group: 'Overlays', name: 'Command palette', path: '/?open=palette', as: 'maya' },
  { group: 'Overlays', name: 'Notifications', path: '/?open=notifications', as: 'maya' },
  { group: 'Overlays', name: 'Play on (casting)', path: '/?open=cast', as: 'maya' },
  { group: 'Overlays', name: 'Keyboard shortcuts', path: '/?open=shortcuts', as: 'maya' },
  { group: 'Overlays', name: 'Mini player', path: '/music?open=mini', as: 'maya' },
  { group: 'Overlays', name: 'Now Playing', path: '/music?open=nowplaying', as: 'maya' },
  { group: 'Overlays', name: 'Now Playing · lyrics', path: '/music?open=lyrics', as: 'maya' },
  { group: 'Overlays', name: 'Now Playing · up next', path: '/music?open=queue', as: 'maya' },
  { group: 'Overlays', name: 'Audiobook player', path: '/audiobooks?open=book', as: 'maya' },
  { group: 'Overlays', name: 'Download options', path: '/movie/tidewater?sheet=download', as: 'maya' },
  { group: 'Overlays', name: 'Edit metadata', path: '/movie/northbound?sheet=edit', as: 'maya' },
  { group: 'Overlays', name: 'Fix match', path: '/movie/the-understudy?sheet=fix', as: 'maya' },
  { group: 'Overlays', name: 'Recommend to family', path: '/movie/small-hours?sheet=share', as: 'maya' },
  { group: 'Overlays', name: 'Program details', path: '/live?program=ch-5-10', as: 'maya' },
  { group: 'Overlays', name: 'Player · audio & subtitles', path: '/watch/the-long-quiet?t=600&panel=tracks', as: 'maya' },
  { group: 'Overlays', name: 'Player · quality & speed', path: '/watch/the-long-quiet?t=600&panel=settings', as: 'maya' },
  { group: 'Overlays', name: 'Player · chapters', path: '/watch/deep-field?t=900&panel=chapters', as: 'maya' },
  { group: 'Overlays', name: 'Player · episodes', path: '/watch/meridian-s2e3?t=300&panel=episodes', as: 'maya' },
  { group: 'Overlays', name: 'Player · watch together', path: '/watch/glass-harbor?t=1200&panel=together', as: 'maya' },
  { group: 'Overlays', name: 'Player · up next', path: '/watch/meridian-s2e3?t=3285', as: 'maya' },

  { group: 'States', name: 'Loading (network hangs)', path: '/?latency=hang', as: 'maya', note: 'Reset with ?latency=normal' },
  { group: 'States', name: 'Slow network', path: '/movie/glass-harbor?latency=slow', as: 'maya' },
  { group: 'States', name: 'Server unreachable', path: '/?fail=all', as: 'maya', note: 'Reset with ?fail=none' },
  { group: 'States', name: 'Error on one page', path: '/music?fail=getMusicHome', as: 'maya' },
  { group: 'States', name: 'Movies at 250,000 items', path: '/movies?view=library&scale=250000', as: 'maya', note: 'Virtualized grid; reset with ?scale=0' },
  { group: 'States', name: 'Not found', path: '/nowhere', as: 'maya' },
  { group: 'States', name: 'Missing title', path: '/movie/not-a-movie', as: 'maya' },
];

export const screenUrl = (s: ScreenDef) => {
  const [p, q] = s.path.split('?');
  const params = new URLSearchParams(q);
  if (s.as) params.set('as', s.as);
  if (!params.has('latency')) params.set('latency', 'normal');
  if (!params.has('fail')) params.set('fail', 'none');
  if (!params.has('scale')) params.set('scale', '0');
  return `${p}?${params.toString()}`;
};

