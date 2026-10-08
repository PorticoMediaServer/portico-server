import type { CastTarget, Device, Download, HistoryEntry, Notification, Profile, ProgressEntry } from '../types.ts';
import { art, daysAgo, hoursAgo, minutesAgo } from './util.ts';

export const HOUSEHOLD = {
  name: 'The Lindqvist-Okafor Household',
  serverName: 'Harbor',
  ownerEmail: 'maya@lindqvist.family',
};

const base = {
  language: 'en-US',
  subtitleMode: 'auto' as const,
  subtitleLanguage: 'en',
  audioLanguage: 'original',
  autoplayNext: true,
  autoplayPreviews: true,
  streamingQuality: 'original' as const,
  cellularQuality: 'auto' as const,
};

export const profiles: Profile[] = [
  {
    ...base,
    id: 'maya',
    name: 'Maya',
    role: 'owner',
    avatarColor: '#409fd4',
    initials: 'M',
    hasPin: true,
    maxRating: 'R',
    maxTvRating: 'TV-MA',
    libraries: 'all',
    email: 'maya@lindqvist.family',
    createdAt: daysAgo(1400),
    lastActive: minutesAgo(1),
  },
  {
    ...base,
    id: 'daniel',
    name: 'Daniel',
    role: 'admin',
    avatarColor: '#f0a02a',
    initials: 'D',
    hasPin: false,
    maxRating: 'R',
    maxTvRating: 'TV-MA',
    libraries: 'all',
    email: 'daniel.okafor@lindqvist.family',
    subtitleMode: 'always',
    createdAt: daysAgo(1400),
    lastActive: minutesAgo(8),
  },
  {
    ...base,
    id: 'noor',
    name: 'Noor',
    role: 'member',
    avatarColor: '#e25aa8',
    initials: 'N',
    hasPin: false,
    maxRating: 'PG-13',
    maxTvRating: 'TV-14',
    libraries: ['lib-movies', 'lib-tv', 'lib-anime', 'lib-music', 'lib-audiobooks', 'lib-photos', 'lib-homevideos'],
    audioLanguage: 'ja',
    subtitleLanguage: 'en',
    streamingQuality: 'auto',
    createdAt: daysAgo(1100),
    lastActive: minutesAgo(14),
    restrictions: { bedtime: '22:30' },
  },
  {
    ...base,
    id: 'theo',
    name: 'Theo',
    role: 'kid',
    avatarColor: '#35c28a',
    initials: 'T',
    hasPin: false,
    maxRating: 'G',
    maxTvRating: 'TV-Y7',
    libraries: ['lib-movies', 'lib-tv', 'lib-music', 'lib-audiobooks'],
    autoplayNext: false,
    autoplayPreviews: false,
    streamingQuality: 'auto',
    createdAt: daysAgo(700),
    lastActive: hoursAgo(20),
    restrictions: { bedtime: '19:30', dailyLimitMin: 90, blockedGenres: ['Horror'] },
  },
];

export const devices: Device[] = [
  { id: 'dev-browser', name: 'Safari on MacBook Pro', platform: 'macOS 26', app: 'Portico Web 3.2', location: 'Portland, ME', ip: '192.168.1.31', lastSeen: minutesAgo(0), current: true, profileId: 'maya', kind: 'browser' },
  { id: 'dev-atv', name: 'Living Room', platform: 'Apple TV 4K (tvOS 26)', app: 'Portico for tvOS 3.2', location: 'Portland, ME', ip: '192.168.1.40', lastSeen: minutesAgo(3), profileId: 'daniel', kind: 'tv' },
  { id: 'dev-iphone-maya', name: 'Maya’s iPhone', platform: 'iPhone 17 Pro (iOS 26)', app: 'Portico for iOS 3.2', location: 'Portland, ME', ip: '192.168.1.52', lastSeen: hoursAgo(1), profileId: 'maya', kind: 'phone' },
  { id: 'dev-ipad-noor', name: 'Noor’s iPad', platform: 'iPad Air (iPadOS 26)', app: 'Portico for iOS 3.2', location: 'Portland, ME', ip: '192.168.1.57', lastSeen: minutesAgo(14), profileId: 'noor', kind: 'tablet' },
  { id: 'dev-bedroom', name: 'Bedroom TV', platform: 'Google TV Streamer', app: 'Portico for Android TV 3.1', location: 'Portland, ME', ip: '192.168.1.44', lastSeen: daysAgo(2), profileId: 'maya', kind: 'tv' },
  { id: 'dev-kids-tablet', name: 'Kids Tablet', platform: 'iPad (iPadOS 26)', app: 'Portico for iOS 3.2', location: 'Portland, ME', ip: '192.168.1.60', lastSeen: hoursAgo(20), profileId: 'theo', kind: 'tablet' },
  { id: 'dev-work-laptop', name: 'Chrome on Windows', platform: 'Windows 11', app: 'Portico Web 3.2', location: 'Boston, MA', ip: '73.114.22.9', lastSeen: daysAgo(6), profileId: 'daniel', kind: 'browser' },
];

export const castTargets: CastTarget[] = [
  { id: 'cast-atv', name: 'Living Room', kind: 'tv', protocol: 'Portico', room: 'Living Room' },
  { id: 'cast-bedroom', name: 'Bedroom TV', kind: 'tv', protocol: 'Google Cast', room: 'Bedroom' },
  { id: 'cast-kitchen', name: 'Kitchen', kind: 'speaker', protocol: 'AirPlay', room: 'Kitchen' },
  { id: 'cast-downstairs', name: 'Downstairs', kind: 'group', protocol: 'AirPlay', room: 'Kitchen + Living Room' },
  { id: 'cast-projector', name: 'Garage Projector', kind: 'tv', protocol: 'DLNA', room: 'Garage' },
];

export const notificationsSeed: Notification[] = [
  { id: 'n1', kind: 'new-episode', title: 'New episode of Meridian', body: 'S3 · E5 “Crossing Guard” is ready to watch.', at: minutesAgo(40), itemId: 'meridian-s3e5', href: '/show/meridian', image: art.still('meridian-s3e5') },
  { id: 'n2', kind: 'recording', title: 'Recording finished', body: 'Planet Shoreline — “Fjords” (PBN) · 59 min · 4.1 GB', at: hoursAgo(19), href: '/live/dvr', image: art.program('s35') },
  { id: 'n3', kind: 'new-episode', title: 'New episode of The Wind and the Whale', body: 'E8 “Eye of the Storm” — with English subtitles.', at: hoursAgo(26), href: '/show/kaze-to-kujira', image: art.still('kaze-to-kujira-s1e8') },
  { id: 'n4', kind: 'download', title: 'Download complete', body: 'Deep Field (1080p, 6.2 GB) is available offline on Maya’s iPhone.', at: hoursAgo(30), href: '/downloads', image: art.backdrop('deep-field') },
  { id: 'n5', kind: 'share', title: 'Grandma Ingrid accepted your invite', body: 'She can now see Photos, Home Videos and Movies.', at: daysAgo(2), href: '/console/users', read: true },
  { id: 'n6', kind: 'server', title: 'Disk sdc is reporting reallocated sectors', body: '8 reallocated sectors on WD Red Plus 12 TB (bay 3). Consider replacing it soon.', at: daysAgo(2, 4), href: '/console/storage', adminOnly: true },
  { id: 'n7', kind: 'server', title: 'Backup completed', body: 'Nightly backup to Backblaze B2 · 1.8 GB · 4 min 12 s', at: hoursAgo(7), href: '/console/backups', adminOnly: true, read: true },
  { id: 'n8', kind: 'security', title: 'New sign-in on Chrome on Windows', body: 'Daniel signed in from Boston, MA. Not you? Review devices.', at: daysAgo(6), href: '/settings/devices', read: true },
  { id: 'n9', kind: 'new-episode', title: 'Dispatch from Mars is back', body: 'S1 · E6 “Blackout” is now available.', at: daysAgo(3), href: '/show/dispatch-from-mars', image: art.still('dispatch-from-mars-s1e6'), read: true },
  { id: 'n10', kind: 'request', title: 'Noor requested a title', body: '“Tokyo Nightwalkers” Season 2 — add it when it’s available?', at: daysAgo(4), href: '/show/tokyo-nightwalkers', adminOnly: true, read: true },
];

export const downloadsSeed: Download[] = [
  { id: 'dl-1', itemId: 'deep-field', itemKind: 'movie', title: 'Deep Field', subtitle: 'Movie · 2 h 18 min', image: art.backdrop('deep-field'), quality: 'High (1080p)', sizeBytes: 6.2e9, progress: 1, status: 'complete', device: 'Maya’s iPhone', expiresAt: daysAgo(-28) },
  { id: 'dl-2', itemId: 'meridian-s3e4', itemKind: 'episode', title: 'Meridian', subtitle: 'S3 · E4 · Negative Space', image: art.still('meridian-s3e4'), quality: 'High (1080p)', sizeBytes: 2.3e9, progress: 1, status: 'complete', device: 'Maya’s iPhone', smart: true },
  { id: 'dl-3', itemId: 'meridian-s3e5', itemKind: 'episode', title: 'Meridian', subtitle: 'S3 · E5 · Crossing Guard', image: art.still('meridian-s3e5'), quality: 'High (1080p)', sizeBytes: 2.4e9, progress: 0.64, status: 'downloading', device: 'Maya’s iPhone', smart: true },
  { id: 'dl-4', itemId: 'northern-static', itemKind: 'album', title: 'Northern Static', subtitle: 'Aurora Vale · 10 songs', image: art.cover('northern-static'), quality: 'Original', sizeBytes: 412e6, progress: 1, status: 'complete', device: 'Maya’s iPhone' },
  { id: 'dl-5', itemId: 'the-time-machine', itemKind: 'audiobook', title: 'The Time Machine', subtitle: 'H. G. Wells · 3 h 25 min', image: art.cover('the-time-machine'), quality: 'Original', sizeBytes: 98e6, progress: 1, status: 'complete', device: 'Maya’s iPhone' },
  { id: 'dl-6', itemId: 'little-explorers-s1e3', itemKind: 'episode', title: 'Little Explorers', subtitle: 'S1 · E3 · Leaf Detectives', image: art.still('little-explorers-s1e3'), quality: 'Medium (720p)', sizeBytes: 310e6, progress: 0, status: 'queued', device: 'Kids Tablet', smart: true },
  { id: 'dl-7', itemId: 'the-summit-line', itemKind: 'movie', title: 'The Summit Line', subtitle: 'Movie · 2 h 4 min', image: art.backdrop('the-summit-line'), quality: 'High (1080p)', sizeBytes: 5.8e9, progress: 0.21, status: 'error', device: 'Maya’s iPhone', error: 'Not enough space on device — 3.1 GB free, 4.6 GB needed' },
  { id: 'dl-8', itemId: 'tidewater', itemKind: 'movie', title: 'Tidewater', subtitle: 'Movie · 2 h 13 min', image: art.backdrop('tidewater'), quality: 'High (1080p)', sizeBytes: 6.0e9, progress: 1, status: 'expired', device: 'Maya’s iPhone', error: 'Offline license expired after 30 days. Reconnect to renew.' },
];

const m = 60;
export const progressSeed: Record<string, ProgressEntry[]> = {
  maya: [
    { itemId: 'deep-field', position: 48 * m + 12, duration: 138 * m, updatedAt: hoursAgo(2) },
    { itemId: 'meridian-s3e4', position: 12 * m + 40, duration: 54 * m, updatedAt: hoursAgo(20) },
    { itemId: 'glass-harbor', position: 81 * m, duration: 117 * m, updatedAt: daysAgo(3) },
    { itemId: 'saltmarsh-s1e3', position: 20 * m + 5, duration: 58 * m, updatedAt: daysAgo(5) },
    { itemId: 'sora-no-kagami-s2e3', position: 11 * m, duration: 24 * m, updatedAt: daysAgo(1) },
    { itemId: 'the-weight-of-light', position: 35 * m, duration: 96 * m, updatedAt: daysAgo(8) },
    { itemId: 'pride-and-prejudice', position: 3 * 3600 + 14 * m, duration: 11 * 3600 + 35 * m, updatedAt: hoursAgo(9) },
    { itemId: 'the-adventures-of-sherlock-holmes', position: 5 * 3600 + 2 * m, duration: 10 * 3600 + 25 * m, updatedAt: daysAgo(4) },
    { itemId: 'hv-mormor', position: 4 * m + 10, duration: 12 * m + 36, updatedAt: daysAgo(6) },
  ],
  daniel: [
    { itemId: 'glass-harbor', position: 63 * m, duration: 117 * m, updatedAt: minutesAgo(8) },
    { itemId: 'ridgeline-s1e5', position: 30 * m, duration: 56 * m, updatedAt: daysAgo(1) },
    { itemId: 'the-night-desk-s2e6', position: 5 * m, duration: 48 * m, updatedAt: daysAgo(2) },
    { itemId: 'the-summit-line', position: 90 * m, duration: 124 * m, updatedAt: daysAgo(4) },
  ],
  noor: [
    { itemId: 'tokyo-nightwalkers-s1e9', position: 9 * m, duration: 24 * m, updatedAt: minutesAgo(14) },
    { itemId: 'kaze-to-kujira-s1e7', position: 16 * m, duration: 24 * m, updatedAt: daysAgo(1) },
    { itemId: 'paper-satellites', position: 44 * m, duration: 104 * m, updatedAt: daysAgo(3) },
  ],
  theo: [
    { itemId: 'little-explorers-s1e6', position: 4 * m, duration: 11 * m, updatedAt: hoursAgo(20) },
    { itemId: 'biscuit-takes-manhattan', position: 51 * m, duration: 92 * m, updatedAt: daysAgo(2) },
    { itemId: 'the-wind-in-the-willows', position: 42 * m, duration: 5 * 3600 + 50 * m, updatedAt: daysAgo(1) },
  ],
};

export const watchedSeed: Record<string, string[]> = {
  maya: ['the-long-quiet', 'northbound', 'the-summit-line', 'meridian-s1e1', 'meridian-s1e2', 'meridian-s1e3', 'meridian-s1e4', 'meridian-s1e5', 'meridian-s1e6', 'meridian-s1e7', 'meridian-s1e8', 'meridian-s2e1', 'meridian-s2e2', 'meridian-s2e3', 'meridian-s2e4', 'meridian-s2e5', 'meridian-s2e6', 'meridian-s2e7', 'meridian-s2e8', 'meridian-s3e1', 'meridian-s3e2', 'meridian-s3e3', 'saltmarsh-s1e1', 'saltmarsh-s1e2', 'sora-no-kagami-s1e1', 'sora-no-kagami-s1e2', 'sora-no-kagami-s1e3', 'sora-no-kagami-s1e4', 'sora-no-kagami-s1e5', 'sora-no-kagami-s1e6', 'sora-no-kagami-s1e7', 'sora-no-kagami-s1e8', 'sora-no-kagami-s1e9', 'sora-no-kagami-s1e10', 'sora-no-kagami-s1e11', 'sora-no-kagami-s1e12', 'sora-no-kagami-s2e1', 'sora-no-kagami-s2e2', 'charade', 'the-general', 'coffee-run', 'spring', 'small-hours', 'lanterns-over-kyoto', 'the-cartographers-daughter', 'the-understudy', 'midnight-at-the-aurora-palace', 'wild-country', 'hoshikuzu-diner-s1e1', 'hoshikuzu-diner-s1e2', 'hoshikuzu-diner-s1e3'],
  daniel: ['the-summit-line', 'signal-fire', 'ironclad-express', 'ridgeline-s1e1', 'ridgeline-s1e2', 'ridgeline-s1e3', 'ridgeline-s1e4', 'the-night-desk-s1e1', 'the-night-desk-s1e2', 'the-night-desk-s1e3', 'the-night-desk-s1e4', 'the-night-desk-s1e5', 'the-night-desk-s1e6', 'the-night-desk-s1e7', 'the-night-desk-s1e8', 'the-night-desk-s2e1', 'the-night-desk-s2e2', 'the-night-desk-s2e3', 'the-night-desk-s2e4', 'the-night-desk-s2e5'],
  noor: ['tokyo-nightwalkers-s1e1', 'tokyo-nightwalkers-s1e2', 'tokyo-nightwalkers-s1e3', 'tokyo-nightwalkers-s1e4', 'tokyo-nightwalkers-s1e5', 'tokyo-nightwalkers-s1e6', 'tokyo-nightwalkers-s1e7', 'tokyo-nightwalkers-s1e8', 'kaze-to-kujira-s1e1', 'kaze-to-kujira-s1e2', 'kaze-to-kujira-s1e3', 'kaze-to-kujira-s1e4', 'kaze-to-kujira-s1e5', 'kaze-to-kujira-s1e6', 'summer-radio-s1e1', 'summer-radio-s1e2', 'everything-is-weather'],
  theo: ['big-buck-bunny', 'caminandes-llamigos', 'the-summer-of-two-dogs', 'little-explorers-s1e1', 'little-explorers-s1e2', 'little-explorers-s1e3', 'little-explorers-s1e4', 'little-explorers-s1e5'],
};

export const watchlistSeed: Record<string, string[]> = {
  maya: ['neon-orchard', 'tidewater', 'the-last-orchard', 'frostline', 'undercurrent', 'metropolis', 'kaze-to-kujira', 'black-sand', 'sunset-protocol'],
  daniel: ['sunset-protocol', 'nine-bridges', 'the-crossing', 'dispatch-from-mars'],
  noor: ['blade-of-the-ninth-moon', 'neon-orchard', 'hoshikuzu-diner', 'lanterns-over-kyoto'],
  theo: ['heist-at-hollow-creek', 'spring'],
};

export const ratingsSeed: Record<string, Record<string, number>> = {
  maya: { 'the-long-quiet': 5, northbound: 4, 'the-summit-line': 3, meridian: 5, charade: 5 },
};

export const historySeed = (): HistoryEntry[] => {
  const rows: [string, string, string, string, string, number, string, boolean][] = [
    ['h1', 'deep-field', 'movie', 'Deep Field', 'Movie · stopped at 48:12', 2, 'Living Room', false],
    ['h2', 'meridian-s3e4', 'episode', 'Meridian', 'S3 · E4 · Negative Space', 20, 'Maya’s iPhone', false],
    ['h3', 'meridian-s3e3', 'episode', 'Meridian', 'S3 · E3 · The Archive', 27, 'Living Room', true],
    ['h4', 'northern-static', 'album', 'Northern Static', 'Aurora Vale · 10 songs', 30, 'Kitchen', true],
    ['h5', 'sora-no-kagami-s2e3', 'episode', 'Mirror of the Sky', 'S2 · E3 · The Salt Road', 24, 'Safari on MacBook Pro', false],
    ['h6', 'pride-and-prejudice', 'audiobook', 'Pride and Prejudice', 'Chapter 14', 9, 'Maya’s iPhone', false],
    ['h7', 'sora-no-kagami-s2e2', 'episode', 'Mirror of the Sky', 'S2 · E2 · Low Water', 49, 'Safari on MacBook Pro', true],
    ['h8', 'glass-harbor', 'movie', 'Glass Harbor', 'Movie · stopped at 1:21:00', 72, 'Living Room', false],
    ['h9', 'the-long-quiet', 'movie', 'The Long Quiet', 'Movie', 100, 'Living Room', true],
    ['h10', 'saltmarsh-s1e2', 'episode', 'Saltmarsh', 'S1 · E2 · Flod', 125, 'Bedroom TV', true],
    ['h11', 'velvet-hours', 'album', 'Velvet Hours', 'Nadia Okoro', 130, 'Maya’s iPhone', true],
    ['h12', 'the-weight-of-light', 'movie', 'The Weight of Light', 'Movie · stopped at 35:00', 190, 'Safari on MacBook Pro', false],
    ['h13', 'charade', 'movie', 'Charade', 'Movie', 220, 'Living Room', true],
    ['h14', 'hv-mormor', 'homevideo', 'Mormor’s 80th Birthday', 'Home video', 144, 'Living Room', false],
    ['h15', 'coffee-run', 'movie', 'Coffee Run', 'Short film', 260, 'Safari on MacBook Pro', true],
  ];
  return rows.map(([id, itemId, kind, title, subtitle, h, device, completed]) => ({
    id,
    itemId,
    itemKind: kind as HistoryEntry['itemKind'],
    title,
    subtitle,
    image:
      kind === 'episode' ? art.still(itemId) : kind === 'album' || kind === 'audiobook' ? art.cover(itemId) : kind === 'homevideo' ? art.still(itemId) : art.backdrop(itemId),
    at: hoursAgo(h),
    device,
    profileId: 'maya',
    completed,
  }));
};
