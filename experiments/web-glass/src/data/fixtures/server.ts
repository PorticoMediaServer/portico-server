// Owner/admin console fixtures: the server, its hardware, sessions, logs and settings.
import { NOW, art, daysAgo, hoursAgo, minutesAgo, rng } from './util.ts';

export interface ServerInfo {
  id: string;
  name: string;
  version: string;
  latestVersion: string;
  releaseNotes: string[];
  platform: string;
  kernel: string;
  cpu: string;
  cores: number;
  memoryGb: number;
  gpu: string;
  startedAt: string;
  localUrl: string;
  remoteUrl: string;
  publicIp: string;
  owner: string;
  machineId: string;
  database: string;
  dataDir: string;
  transcodeDir: string;
}

export const server: ServerInfo = {
  id: 'srv-harbor',
  name: 'Harbor',
  version: '3.2.1',
  latestVersion: '3.2.2',
  releaseNotes: [
    'Fixes subtitle offset drift on long PGS tracks after seeking',
    'HDHomeRun FLEX 4K: faster channel tuning (−380 ms average)',
    'Scanner: 18% faster first scan on libraries over 100,000 items',
    'Security: session tokens now rotate on password change for every device',
  ],
  platform: 'Ubuntu 24.04.3 LTS',
  kernel: 'Linux 6.8.0-71-generic x86_64',
  cpu: 'Intel Core i5-13500 (14 cores / 20 threads)',
  cores: 20,
  memoryGb: 64,
  gpu: 'Intel UHD Graphics 770 (Quick Sync, VA-API)',
  startedAt: daysAgo(19, 3),
  localUrl: 'http://192.168.1.20:32500',
  remoteUrl: 'https://harbor.lindqvist.family',
  publicIp: '73.186.41.207',
  owner: 'maya@lindqvist.family',
  machineId: '7f3c9a21-5d4e-4b8f-9e02-6a1c8d33f0b7',
  database: 'SQLite 3.46 (WAL) · 1.42 GB',
  dataDir: '/var/lib/portico',
  transcodeDir: '/transcode (tmpfs, 16 GB)',
};

// ---------- live metrics ----------
export interface MetricPoint {
  t: number;
  cpu: number; // %
  gpu: number; // %
  mem: number; // GB used
  netIn: number; // Mbps
  netOut: number; // Mbps
  disk: number; // MB/s
  streams: number;
  transcodes: number;
}

export function metricAt(t: number): MetricPoint {
  const s = t / 1000;
  const wave = (p: number, a: number) => a * Math.sin((2 * Math.PI * s) / p);
  const noise = (k: number) => {
    const x = Math.sin(s * 12.9898 + k * 78.233) * 43758.5453;
    return x - Math.floor(x);
  };
  const transcodes = 2;
  return {
    t,
    cpu: Math.max(4, Math.min(96, 31 + wave(47, 9) + wave(13, 4) + noise(1) * 10)),
    gpu: Math.max(2, Math.min(90, 38 + wave(31, 12) + noise(2) * 8)),
    mem: 21.4 + wave(300, 0.9) + noise(3) * 0.4,
    netIn: Math.max(0.1, 2.8 + wave(23, 1.6) + noise(4) * 1.4),
    netOut: Math.max(1, 71 + wave(37, 14) + wave(11, 6) + noise(5) * 9),
    disk: Math.max(0, 118 + wave(19, 40) + noise(6) * 60),
    streams: 5,
    transcodes,
  };
}

export const metricHistory = (minutes = 60, stepSec = 15): MetricPoint[] => {
  const n = Math.floor((minutes * 60) / stepSec);
  return Array.from({ length: n }, (_, i) => metricAt(NOW - (n - i) * stepSec * 1000));
};

// ---------- sessions ----------
export interface Session {
  id: string;
  user: string;
  userId: string;
  device: string;
  platform: string;
  app: string;
  ip: string;
  location: string;
  local: boolean;
  itemId: string;
  title: string;
  subtitle: string;
  image: string;
  state: 'playing' | 'paused' | 'buffering';
  position: number;
  duration: number;
  bandwidthMbps: number;
  decision: 'Direct Play' | 'Direct Stream' | 'Transcode';
  reason?: string;
  video: { source: string; target: string; hw?: boolean; speed?: number };
  audio: { source: string; target: string };
  subtitles?: string;
  container: string;
  quality: string;
  startedAt: string;
}

export const sessionsSeed: Session[] = [
  {
    id: 'ses-1',
    user: 'Daniel',
    userId: 'daniel',
    device: 'Living Room',
    platform: 'Apple TV 4K',
    app: 'Portico for tvOS 3.2',
    ip: '192.168.1.40',
    location: 'LAN',
    local: true,
    itemId: 'glass-harbor',
    title: 'Glass Harbor',
    subtitle: '2025 · Movie',
    image: art.backdrop('glass-harbor'),
    state: 'playing',
    position: 63 * 60 + 12,
    duration: 117 * 60,
    bandwidthMbps: 64.2,
    decision: 'Direct Play',
    video: { source: '4K HEVC Main 10 · Dolby Vision P8', target: 'Direct Play' },
    audio: { source: 'English TrueHD Atmos 7.1', target: 'Direct Play (passthrough)' },
    subtitles: 'English (PGS) · burned: no',
    container: 'MKV',
    quality: 'Original (62.8 Mbps)',
    startedAt: minutesAgo(64),
  },
  {
    id: 'ses-2',
    user: 'Noor',
    userId: 'noor',
    device: 'Noor’s iPad',
    platform: 'iPadOS 26',
    app: 'Portico for iOS 3.2',
    ip: '192.168.1.57',
    location: 'LAN',
    local: true,
    itemId: 'tokyo-nightwalkers-s1e9',
    title: 'Tokyo Nightwalkers',
    subtitle: 'S1 · E9 · Daybreak',
    image: art.still('tokyo-nightwalkers-s1e9'),
    state: 'playing',
    position: 9 * 60 + 40,
    duration: 24 * 60,
    bandwidthMbps: 4.1,
    decision: 'Transcode',
    reason: 'Subtitle burn-in: styled ASS subtitles are not supported by the client renderer',
    video: { source: '1080p H.264 High', target: '1080p H.264 (burn-in)', hw: true, speed: 6.4 },
    audio: { source: 'Japanese FLAC 2.0', target: 'AAC 2.0 256 kbps' },
    subtitles: 'English (ASS) · burned in',
    container: 'MKV → fMP4 (HLS)',
    quality: 'Original · 4.1 Mbps',
    startedAt: minutesAgo(14),
  },
  {
    id: 'ses-3',
    user: 'Grandma Ingrid',
    userId: 'ingrid',
    device: 'Chrome on Windows',
    platform: 'Windows 11',
    app: 'Portico Web 3.2',
    ip: '85.228.101.44',
    location: 'Göteborg, Sweden',
    local: false,
    itemId: 'hv-mormor',
    title: 'Mormor’s 80th Birthday',
    subtitle: 'Home video · 2026',
    image: art.still('hv-mormor'),
    state: 'playing',
    position: 4 * 60 + 18,
    duration: 12 * 60 + 36,
    bandwidthMbps: 3.9,
    decision: 'Transcode',
    reason: 'Remote bandwidth limit for this user (4 Mbps)',
    video: { source: '4K HEVC Main 10 (HLG)', target: '720p H.264 · tone-mapped', hw: true, speed: 3.1 },
    audio: { source: 'AAC 2.0 (iPhone)', target: 'Direct Stream' },
    container: 'MOV → fMP4 (HLS)',
    quality: '720p · 4 Mbps (limit)',
    startedAt: minutesAgo(5),
  },
  {
    id: 'ses-4',
    user: 'Maya',
    userId: 'maya',
    device: 'Kitchen',
    platform: 'HomePod (AirPlay)',
    app: 'Portico for iOS 3.2',
    ip: '192.168.1.52',
    location: 'LAN',
    local: true,
    itemId: 'lo-fi-mornings-4',
    title: 'Seoul, 7 A.M.',
    subtitle: 'Sunday Arcade — Lo-Fi Mornings',
    image: art.cover('lo-fi-mornings'),
    state: 'playing',
    position: 71,
    duration: 172,
    bandwidthMbps: 0.26,
    decision: 'Direct Play',
    video: { source: '—', target: '—' },
    audio: { source: 'AAC 256 kbps', target: 'Direct Play (AirPlay 2)' },
    container: 'M4A',
    quality: 'Original',
    startedAt: minutesAgo(22),
  },
  {
    id: 'ses-5',
    user: 'Sam Carter',
    userId: 'sam',
    device: 'Sam’s Pixel 10',
    platform: 'Android 17',
    app: 'Portico for Android 3.1',
    ip: '104.28.55.12',
    location: 'Montréal, Canada',
    local: false,
    itemId: 'the-long-quiet',
    title: 'The Long Quiet',
    subtitle: '2024 · Movie',
    image: art.backdrop('the-long-quiet'),
    state: 'paused',
    position: 31 * 60,
    duration: 121 * 60,
    bandwidthMbps: 0,
    decision: 'Direct Stream',
    reason: 'Container remux: MKV → MP4 for Android',
    video: { source: '1080p HEVC', target: 'Direct Stream' },
    audio: { source: 'Norwegian DTS-HD MA 5.1', target: 'Norwegian E-AC3 5.1 (converted)' },
    subtitles: 'English (Forced) + English (SRT)',
    container: 'MKV → MP4',
    quality: 'Original · 8.4 Mbps',
    startedAt: minutesAgo(48),
  },
];

// ---------- activity ----------
export interface ActivityEvent {
  id: string;
  at: string;
  kind: 'playback' | 'library' | 'user' | 'security' | 'system' | 'dvr' | 'backup' | 'warning';
  actor: string;
  text: string;
  detail?: string;
}

export const activitySeed: ActivityEvent[] = [
  { id: 'a1', at: minutesAgo(5), kind: 'playback', actor: 'Grandma Ingrid', text: 'started Mormor’s 80th Birthday', detail: 'Chrome on Windows · Göteborg · transcoding to 720p (bandwidth limit)' },
  { id: 'a2', at: minutesAgo(9), kind: 'library', actor: 'Scanner', text: 'added 3 episodes to TV Shows', detail: 'Meridian S3E5 · Dispatch from Mars S1E6 · The Night Desk S2E8' },
  { id: 'a3', at: minutesAgo(14), kind: 'playback', actor: 'Noor', text: 'started Tokyo Nightwalkers S1E9', detail: 'Noor’s iPad · burning in ASS subtitles (hardware)' },
  { id: 'a4', at: minutesAgo(22), kind: 'playback', actor: 'Maya', text: 'started Lo-Fi Mornings on Kitchen', detail: 'AirPlay 2 · Direct Play' },
  { id: 'a5', at: minutesAgo(48), kind: 'playback', actor: 'Sam Carter', text: 'paused The Long Quiet', detail: 'Sam’s Pixel 10 · Montréal · Direct Stream' },
  { id: 'a6', at: minutesAgo(64), kind: 'playback', actor: 'Daniel', text: 'started Glass Harbor', detail: 'Living Room · 4K Dolby Vision · Direct Play' },
  { id: 'a7', at: hoursAgo(2), kind: 'dvr', actor: 'DVR', text: 'scheduled 4 recordings from series rules', detail: 'Open Flame ×2 · Little Explorers ×2' },
  { id: 'a8', at: hoursAgo(7), kind: 'backup', actor: 'Backups', text: 'completed nightly backup to Backblaze B2', detail: '1.8 GB · 4 min 12 s · 30 versions retained' },
  { id: 'a9', at: hoursAgo(9), kind: 'system', actor: 'Tasks', text: 'optimized database', detail: 'Reclaimed 212 MB · VACUUM + ANALYZE in 38 s' },
  { id: 'a10', at: hoursAgo(19), kind: 'dvr', actor: 'DVR', text: 'finished recording Planet Shoreline — “Fjords”', detail: 'PBN · tuner 1 · 59 min · 4.1 GB' },
  { id: 'a11', at: hoursAgo(26), kind: 'security', actor: 'Daniel', text: 'enabled a passkey', detail: 'iCloud Keychain · MacBook Pro' },
  { id: 'a12', at: daysAgo(2, 4), kind: 'warning', actor: 'Storage', text: 'SMART warning on sdc (bay 3)', detail: 'Reallocated sectors: 0 → 8 in 72 h. Drive is still healthy; plan a replacement.' },
  { id: 'a13', at: daysAgo(2, 10), kind: 'user', actor: 'Grandma Ingrid', text: 'accepted an invitation', detail: 'Access: Movies, Photos, Home Videos · Remote limit 4 Mbps' },
  { id: 'a14', at: daysAgo(3, 16), kind: 'library', actor: 'Maya', text: 'fixed match for “Stardust Diner”', detail: 'Was: Stardust (2007) · Now: Stardust Diner (2022 TV)' },
  { id: 'a15', at: daysAgo(4, 21), kind: 'user', actor: 'Noor', text: 'requested Tokyo Nightwalkers Season 2' },
  { id: 'a16', at: daysAgo(5, 3), kind: 'system', actor: 'Updater', text: 'Portico Server 3.2.2 is available', detail: 'Released 5 days ago · 4 fixes' },
  { id: 'a17', at: daysAgo(6, 9), kind: 'security', actor: 'Daniel', text: 'signed in from a new device', detail: 'Chrome on Windows · Boston, MA · 73.114.22.9' },
];

// ---------- logs ----------
export type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR';
export interface LogLine {
  id: number;
  t: number;
  level: LogLevel;
  source: string;
  msg: string;
}

const LOG_TEMPLATES: [LogLevel, string, string, number][] = [
  ['INFO', 'http', 'GET /api/v3/library/lib-movies/items?offset={n}&limit=120 200 {ms}ms', 22],
  ['INFO', 'http', 'GET /api/v3/home?profile=maya 200 {ms}ms', 10],
  ['DEBUG', 'http', 'GET /art/poster/{title}.webp 304 1ms', 18],
  ['INFO', 'stream', 'session {ses} position {pos}s state=playing bitrate={br}kbps', 20],
  ['DEBUG', 'transcoder', '[{ses}] segment {seg}.m4s encoded in {ms}ms (qsv h264, speed {sp}x)', 16],
  ['INFO', 'transcoder', '[{ses}] throttling: buffer ahead 120s, pausing encoder', 3],
  ['INFO', 'scanner', 'watch: change detected in /volume1/media/TV/{show}', 4],
  ['DEBUG', 'scanner', 'probe /volume1/media/TV/{show}/S01E0{e}.mkv: hevc 1920x1080, 2 audio, 4 subtitle', 6],
  ['INFO', 'metadata', 'matched "{title}" → tmdb:{id} (confidence 0.{conf})', 4],
  ['DEBUG', 'metadata', 'artwork cache hit {title} poster 480x720', 6],
  ['INFO', 'dvr', 'tuner {tuner} locked 7.1 signal {sig}% SNQ {snq}%', 3],
  ['DEBUG', 'dlna', 'SSDP M-SEARCH from 192.168.1.{ip}:1900', 5],
  ['INFO', 'auth', 'token refreshed for device {dev}', 4],
  ['WARN', 'transcoder', '[{ses}] tone mapping HLG → SDR on client request (no HDR display)', 2],
  ['WARN', 'network', 'remote client {rip} RTT 184ms, reducing adaptive bitrate ceiling to 3.2 Mbps', 2],
  ['WARN', 'storage', 'sdc: SMART attribute 5 (Reallocated_Sector_Ct) raw=8', 1],
  ['ERROR', 'metadata', 'musicbrainz: 503 Service Unavailable for release-group {id} (will retry in 60s)', 1],
  ['ERROR', 'dvr', 'tuner 2: signal lost on 2.1 WHBR for 192s during "The Precinct"', 0.4],
  ['INFO', 'backup', 'snapshot db → b2://harbor-backups/portico/{date}.tar.zst (1.8 GB)', 0.5],
  ['DEBUG', 'db', 'checkpoint WAL 4096 pages in {ms}ms', 4],
];

const fill = (s: string, r: () => number) =>
  s.replace(/\{(\w+)\}/g, (_, k: string) => {
    const x = r();
    switch (k) {
      case 'n': return String(Math.floor(x * 40) * 120);
      case 'ms': return String(Math.floor(x * 90) + 3);
      case 'title': return ['deep-field', 'glass-harbor', 'meridian', 'northbound', 'tidewater', 'the-long-quiet', 'neon-orchard'][Math.floor(x * 7)];
      case 'ses': return ['ses-1', 'ses-2', 'ses-3', 'ses-5'][Math.floor(x * 4)];
      case 'pos': return String(Math.floor(x * 5000));
      case 'br': return String(Math.floor(x * 60000) + 2000);
      case 'seg': return String(Math.floor(x * 900));
      case 'sp': return (2 + x * 6).toFixed(1);
      case 'show': return ['Meridian', 'The Night Desk', 'Dispatch from Mars', 'Saltmarsh'][Math.floor(x * 4)];
      case 'e': return String(Math.floor(x * 8) + 1);
      case 'id': return String(Math.floor(x * 900000) + 10000);
      case 'conf': return String(Math.floor(x * 9) + 90);
      case 'tuner': return String(Math.floor(x * 4) + 1);
      case 'sig': return String(Math.floor(x * 20) + 78);
      case 'snq': return String(Math.floor(x * 12) + 86);
      case 'ip': return String(Math.floor(x * 60) + 30);
      case 'dev': return ['Living Room', 'Maya’s iPhone', 'Noor’s iPad', 'Kids Tablet'][Math.floor(x * 4)];
      case 'rip': return '85.228.101.44';
      case 'date': return new Date(NOW - 7 * 3600_000).toISOString().slice(0, 10);
      default: return k;
    }
  });

const weights = LOG_TEMPLATES.reduce((a, t) => a + t[3], 0);
export function logLine(id: number, t: number): LogLine {
  const r = rng(id * 7919 + 13);
  let x = r() * weights;
  let tpl = LOG_TEMPLATES[0];
  for (const tt of LOG_TEMPLATES) {
    x -= tt[3];
    if (x <= 0) { tpl = tt; break; }
  }
  return { id, t, level: tpl[0], source: tpl[1], msg: fill(tpl[2], r) };
}

/** 24 hours of logs, ~1 line every 2.4 s. Generated lazily by index so the console stays O(visible). */
export const LOG_COUNT = 36_000;
export const LOG_SPAN_MS = 24 * 3_600_000;
export const logAt = (index: number): LogLine => logLine(index, NOW - LOG_SPAN_MS + (index / LOG_COUNT) * LOG_SPAN_MS);

// ---------- libraries (admin view) ----------
export interface LibraryAdmin {
  id: string;
  name: string;
  kind: string;
  paths: { path: string; items: number; sizeBytes: number; online: boolean }[];
  agent: string;
  language: string;
  scanInterval: string;
  watchFolders: boolean;
  lastScan: string;
  lastScanDuration: string;
  scan?: { progress: number; phase: string; current: string; eta: string };
  options: Record<string, boolean | string>;
  issues: number;
}

// ---------- users ----------
export interface UserAdmin {
  id: string;
  name: string;
  email: string;
  role: 'Owner' | 'Admin' | 'Member' | 'Kid' | 'Friend';
  status: 'active' | 'invited' | 'suspended';
  household: boolean;
  libraries: string[] | 'all';
  remoteLimitMbps?: number;
  downloads: boolean;
  liveTv: boolean;
  dvr: boolean;
  mfa: 'passkey' | 'totp' | 'none';
  lastSeen: string;
  lastIp: string;
  plays30d: number;
  hours30d: number;
  maxRating: string;
  avatarColor: string;
  devices: number;
  joinedAt: string;
}

export const usersSeed: UserAdmin[] = [
  { id: 'maya', name: 'Maya Lindqvist', email: 'maya@lindqvist.family', role: 'Owner', status: 'active', household: true, libraries: 'all', downloads: true, liveTv: true, dvr: true, mfa: 'passkey', lastSeen: minutesAgo(1), lastIp: '192.168.1.31', plays30d: 64, hours30d: 41.5, maxRating: 'None', avatarColor: '#409fd4', devices: 4, joinedAt: daysAgo(1400) },
  { id: 'daniel', name: 'Daniel Okafor', email: 'daniel.okafor@lindqvist.family', role: 'Admin', status: 'active', household: true, libraries: 'all', downloads: true, liveTv: true, dvr: true, mfa: 'passkey', lastSeen: minutesAgo(8), lastIp: '192.168.1.40', plays30d: 48, hours30d: 52.0, maxRating: 'None', avatarColor: '#f0a02a', devices: 3, joinedAt: daysAgo(1400) },
  { id: 'noor', name: 'Noor Okafor', email: 'noor@lindqvist.family', role: 'Member', status: 'active', household: true, libraries: ['Movies', 'TV Shows', 'Anime', 'Music', 'Audiobooks', 'Photos', 'Home Videos'], downloads: true, liveTv: true, dvr: false, mfa: 'totp', lastSeen: minutesAgo(14), lastIp: '192.168.1.57', plays30d: 91, hours30d: 38.2, maxRating: 'PG-13 / TV-14', avatarColor: '#e25aa8', devices: 2, joinedAt: daysAgo(1100) },
  { id: 'theo', name: 'Theo Okafor', email: '—', role: 'Kid', status: 'active', household: true, libraries: ['Movies', 'TV Shows', 'Music', 'Audiobooks'], downloads: true, liveTv: false, dvr: false, mfa: 'none', lastSeen: hoursAgo(20), lastIp: '192.168.1.60', plays30d: 57, hours30d: 19.6, maxRating: 'G / TV-Y7', avatarColor: '#35c28a', devices: 1, joinedAt: daysAgo(700) },
  { id: 'ingrid', name: 'Ingrid Lindqvist', email: 'ingrid.lindqvist@telia.se', role: 'Friend', status: 'active', household: false, libraries: ['Movies', 'Photos', 'Home Videos'], remoteLimitMbps: 4, downloads: false, liveTv: false, dvr: false, mfa: 'none', lastSeen: minutesAgo(5), lastIp: '85.228.101.44', plays30d: 12, hours30d: 6.1, maxRating: 'None', avatarColor: '#c9a86a', devices: 1, joinedAt: daysAgo(2) },
  { id: 'sam', name: 'Sam Carter', email: 'sam.carter@fastmail.com', role: 'Friend', status: 'active', household: false, libraries: ['Movies', 'TV Shows', 'Music'], remoteLimitMbps: 12, downloads: true, liveTv: false, dvr: false, mfa: 'totp', lastSeen: minutesAgo(48), lastIp: '104.28.55.12', plays30d: 23, hours30d: 17.9, maxRating: 'None', avatarColor: '#8a5cf0', devices: 2, joinedAt: daysAgo(540) },
  { id: 'priya', name: 'Priya Natarajan', email: 'priya.n@proton.me', role: 'Friend', status: 'invited', household: false, libraries: ['Movies', 'TV Shows', 'Anime'], remoteLimitMbps: 8, downloads: true, liveTv: false, dvr: false, mfa: 'none', lastSeen: '—', lastIp: '—', plays30d: 0, hours30d: 0, maxRating: 'None', avatarColor: '#3fae6b', devices: 0, joinedAt: daysAgo(1) },
  { id: 'marcus', name: 'Marcus Webb', email: 'mwebb@icloud.com', role: 'Friend', status: 'suspended', household: false, libraries: ['Movies'], remoteLimitMbps: 4, downloads: false, liveTv: false, dvr: false, mfa: 'none', lastSeen: daysAgo(120), lastIp: '172.58.12.90', plays30d: 0, hours30d: 0, maxRating: 'None', avatarColor: '#e0513f', devices: 1, joinedAt: daysAgo(800) },
];

// ---------- transcoding ----------
export const transcoderSeed = {
  hardwareAcceleration: true,
  device: 'Intel UHD Graphics 770 (/dev/dri/renderD128)',
  api: 'Intel Quick Sync (QSV)',
  hevcEncoding: true,
  av1Encoding: false,
  toneMapping: 'Hardware (OpenCL)' as string,
  maxTranscodes: 6,
  quality: 'Prefer higher quality' as string,
  throttleBuffer: 120,
  segmentDuration: 4,
  tempDir: '/transcode',
  tempLimitGb: 16,
  backgroundPreset: 'veryfast' as string,
  deinterlace: 'bwdif' as string,
  subtitleBurnIn: 'Only image-based and styled' as string,
  capabilities: [
    ['H.264', 'decode', 'encode'],
    ['HEVC 8-bit', 'decode', 'encode'],
    ['HEVC 10-bit', 'decode', 'encode'],
    ['VP9', 'decode', '—'],
    ['AV1', 'decode', '—'],
    ['MPEG-2', 'decode', '—'],
  ] as [string, string, string][],
  optimizeQueue: [
    { title: 'Deep Field', target: '1080p 10 Mbps (Mobile)', progress: 0.42, eta: '11 min' },
    { title: 'Neon Orchard', target: '1080p 10 Mbps (Mobile)', progress: 0, eta: 'queued' },
    { title: 'Tidewater', target: '720p 4 Mbps (Remote)', progress: 0, eta: 'queued' },
  ],
};

// ---------- network ----------
export const networkSeed = {
  remoteAccess: 'Connected' as 'Connected' | 'Unreachable' | 'Disabled',
  method: 'Manual port forward' as string,
  publicPort: 32500,
  localPort: 32500,
  upnp: false,
  customDomain: 'harbor.lindqvist.family',
  certificate: { issuer: "Let's Encrypt R11", expires: daysAgo(-62), autoRenew: true },
  secureConnections: 'Required' as string,
  lanNetworks: ['192.168.1.0/24', 'fd00:1a2b::/64'],
  allowWithoutAuth: [] as string[],
  ipv6: true,
  relay: false,
  uploadMbps: 300,
  downloadMbps: 940,
  remoteLimitMbps: 40,
  perStreamLimitMbps: 12,
  lanLimit: 'Unlimited' as string,
  checks: [
    { name: 'Port 32500 reachable from the internet', ok: true, detail: 'Verified from 3 regions · 38 ms' },
    { name: 'TLS certificate valid', ok: true, detail: 'harbor.lindqvist.family · 62 days left · auto-renew on' },
    { name: 'DNS resolves to this server', ok: true, detail: 'A 73.186.41.207 · AAAA 2601:18c::1a' },
    { name: 'Double NAT', ok: true, detail: 'Not detected' },
    { name: 'Upload bandwidth', ok: true, detail: '300 Mbps measured · supports ~24 remote 1080p streams' },
  ],
};

// ---------- storage ----------
export const storageSeed = {
  volumes: [
    { id: 'v1', name: 'Media', mount: '/volume1', fs: 'ZFS · RAID-Z2 (6 × 12 TB)', size: 48e12, used: 34.2e12, health: 'Degraded soon' as string, readMBs: 118, writeMBs: 6 },
    { id: 'v2', name: 'Photos & Home', mount: '/volume2', fs: 'ZFS · Mirror (2 × 8 TB)', size: 8e12, used: 4.3e12, health: 'Healthy' as string, readMBs: 12, writeMBs: 1 },
    { id: 'v3', name: 'System & Metadata', mount: '/var/lib/portico', fs: 'ext4 · NVMe SSD', size: 1e12, used: 0.38e12, health: 'Healthy' as string, readMBs: 21, writeMBs: 4 },
    { id: 'v4', name: 'Transcode cache', mount: '/transcode', fs: 'tmpfs', size: 16e9, used: 1.9e9, health: 'Healthy' as string, readMBs: 64, writeMBs: 58 },
    { id: 'v5', name: 'Backups (USB)', mount: '/mnt/backup', fs: 'exFAT · WD Elements 4 TB', size: 4e12, used: 2.44e12, health: 'Healthy' as string, readMBs: 0, writeMBs: 0 },
  ],
  breakdown: [
    { label: 'Movies', bytes: 14.1e12, color: '#409fd4' },
    { label: 'TV Shows', bytes: 12.6e12, color: '#5cb3e2' },
    { label: 'Anime', bytes: 3.9e12, color: '#8a7cf0' },
    { label: 'Music', bytes: 1.2e12, color: '#35c28a' },
    { label: 'Audiobooks', bytes: 0.2e12, color: '#f0a02a' },
    { label: 'DVR', bytes: 2.2e12, color: '#e25aa8' },
  ],
  drives: [
    { dev: 'sda', bay: 1, model: 'WD Red Plus 12 TB', serial: 'WD-C80K1A2L', temp: 36, hours: 21844, realloc: 0, status: 'OK' },
    { dev: 'sdb', bay: 2, model: 'WD Red Plus 12 TB', serial: 'WD-C80K1A9P', temp: 37, hours: 21840, realloc: 0, status: 'OK' },
    { dev: 'sdc', bay: 3, model: 'WD Red Plus 12 TB', serial: 'WD-C80K1C4T', temp: 39, hours: 21839, realloc: 8, status: 'Warning' },
    { dev: 'sdd', bay: 4, model: 'Seagate IronWolf 12 TB', serial: 'ZL2AB7QH', temp: 35, hours: 9021, realloc: 0, status: 'OK' },
    { dev: 'sde', bay: 5, model: 'Seagate IronWolf 12 TB', serial: 'ZL2AB9M1', temp: 35, hours: 9019, realloc: 0, status: 'OK' },
    { dev: 'sdf', bay: 6, model: 'Seagate IronWolf 12 TB', serial: 'ZL2AC0X4', temp: 36, hours: 9018, realloc: 0, status: 'OK' },
    { dev: 'nvme0n1', bay: 0, model: 'Samsung 990 Pro 1 TB', serial: 'S6Z1NF0W40', temp: 44, hours: 11220, realloc: 0, status: 'OK' },
  ],
  caches: [
    { label: 'Artwork & thumbnails', bytes: 38.4e9 },
    { label: 'Video preview frames', bytes: 112.7e9 },
    { label: 'Transcoder temp', bytes: 1.9e9 },
    { label: 'Optimized versions', bytes: 486e9 },
    { label: 'Logs', bytes: 2.1e9 },
  ],
};

// ---------- backups ----------
export const backupsSeed = {
  schedule: 'Daily at 03:30',
  retention: '30 daily · 12 monthly',
  includes: ['Database', 'Settings', 'Watch history & progress', 'User accounts', 'Custom artwork', 'DVR rules'],
  excludes: ['Media files', 'Transcoder cache', 'Preview thumbnails'],
  destinations: [
    { id: 'd1', name: 'Backblaze B2', target: 'b2://harbor-backups/portico', kind: 'Cloud (S3-compatible)', encrypted: true, lastOk: hoursAgo(7), usedBytes: 41e9 },
    { id: 'd2', name: 'USB drive', target: '/mnt/backup/portico', kind: 'Local disk', encrypted: false, lastOk: hoursAgo(31), usedBytes: 58e9 },
  ],
  history: Array.from({ length: 14 }, (_, i) => ({
    id: `bk-${i}`,
    at: new Date(NOW - (i * 24 + 7) * 3600_000).toISOString(),
    destination: i % 2 === 0 ? 'Backblaze B2' : 'USB drive',
    sizeBytes: 1.8e9 - i * 7e6,
    duration: `${3 + (i % 3)} min ${(i * 17) % 60} s`,
    status: i === 5 ? ('failed' as const) : ('ok' as const),
    note: i === 5 ? 'USB drive not mounted (/mnt/backup)' : undefined,
  })),
};

// ---------- scheduled tasks ----------
export interface Task {
  id: string;
  name: string;
  description: string;
  schedule: string;
  lastRun: string;
  lastDuration: string;
  nextRun: string;
  status: 'idle' | 'running' | 'failed' | 'disabled';
  progress?: number;
  category: 'Library' | 'Maintenance' | 'Media analysis' | 'Live TV' | 'System';
}

export const tasksSeed: Task[] = [
  { id: 't1', name: 'Scan all libraries', description: 'Detect new, changed and removed files.', schedule: 'Every 6 hours + file watcher', lastRun: minutesAgo(9), lastDuration: '2 min 41 s', nextRun: hoursAgo(-6), status: 'running', progress: 0.64, category: 'Library' },
  { id: 't2', name: 'Refresh metadata', description: 'Update ratings, artwork and descriptions older than 30 days.', schedule: 'Weekly · Sunday 02:00', lastRun: daysAgo(6, 2), lastDuration: '14 min 3 s', nextRun: daysAgo(-1, 2), status: 'idle', category: 'Library' },
  { id: 't3', name: 'Generate video preview thumbnails', description: 'Scrubbing previews for new videos.', schedule: 'Daily 01:00–05:00', lastRun: hoursAgo(12), lastDuration: '3 h 58 min', nextRun: hoursAgo(-8), status: 'idle', category: 'Media analysis' },
  { id: 't4', name: 'Detect intros and credits', description: 'Audio fingerprinting for Skip Intro and Up Next.', schedule: 'After scan', lastRun: minutesAgo(8), lastDuration: '41 s', nextRun: 'After next scan', status: 'idle', category: 'Media analysis' },
  { id: 't5', name: 'Analyze loudness', description: 'ReplayGain / EBU R128 for music and audiobooks.', schedule: 'Daily 04:00', lastRun: hoursAgo(20), lastDuration: '6 min 12 s', nextRun: hoursAgo(-4), status: 'idle', category: 'Media analysis' },
  { id: 't6', name: 'Optimize database', description: 'VACUUM and ANALYZE the library database.', schedule: 'Weekly · Wednesday 03:00', lastRun: hoursAgo(9), lastDuration: '38 s', nextRun: daysAgo(-7, 3), status: 'idle', category: 'Maintenance' },
  { id: 't7', name: 'Clean transcoder cache', description: 'Remove orphaned segments older than 24 hours.', schedule: 'Hourly', lastRun: minutesAgo(31), lastDuration: '2 s', nextRun: minutesAgo(-29), status: 'idle', category: 'Maintenance' },
  { id: 't8', name: 'Back up database', description: 'Snapshot to every configured destination.', schedule: 'Daily 03:30', lastRun: hoursAgo(7), lastDuration: '4 min 12 s', nextRun: hoursAgo(-17), status: 'idle', category: 'Maintenance' },
  { id: 't9', name: 'Update guide data', description: 'Fetch 14 days of XMLTV listings.', schedule: 'Every 12 hours', lastRun: hoursAgo(3), lastDuration: '19 s', nextRun: hoursAgo(-9), status: 'idle', category: 'Live TV' },
  { id: 't10', name: 'Download missing subtitles', description: 'OpenSubtitles for preferred languages.', schedule: 'Daily 05:00', lastRun: hoursAgo(17), lastDuration: '1 min 4 s', nextRun: hoursAgo(-7), status: 'failed', category: 'Library' },
  { id: 't11', name: 'Fetch lyrics', description: 'Time-synced lyrics from LRCLIB.', schedule: 'Daily 05:30', lastRun: hoursAgo(16), lastDuration: '48 s', nextRun: hoursAgo(-8), status: 'idle', category: 'Library' },
  { id: 't12', name: 'Rebuild search index', description: 'Full-text index over titles, people and lyrics.', schedule: 'Weekly · Monday 04:30', lastRun: daysAgo(5, 4), lastDuration: '1 min 52 s', nextRun: daysAgo(-2, 4), status: 'idle', category: 'Maintenance' },
  { id: 't13', name: 'Prune old logs', description: 'Keep 14 days of logs, compress older.', schedule: 'Daily 00:15', lastRun: hoursAgo(21), lastDuration: '3 s', nextRun: hoursAgo(-3), status: 'idle', category: 'Maintenance' },
  { id: 't14', name: 'Check for updates', description: 'Stable channel.', schedule: 'Daily', lastRun: hoursAgo(5), lastDuration: '1 s', nextRun: hoursAgo(-19), status: 'idle', category: 'System' },
  { id: 't15', name: 'Generate chapter thumbnails', description: 'Disabled: preview thumbnails cover this.', schedule: 'Disabled', lastRun: daysAgo(90), lastDuration: '—', nextRun: '—', status: 'disabled', category: 'Media analysis' },
];

// ---------- live TV sources ----------
export const liveSourcesSeed = {
  tuners: [
    { id: 'hdhr', name: 'HDHomeRun FLEX 4K', model: 'HDFX-4K', ip: '192.168.1.44', firmware: '20250815', tuners: [
      { n: 1, state: 'Recording', channel: '5.1 PBN', signal: 94, quality: 100, symbol: 100 },
      { n: 2, state: 'Idle', channel: '—', signal: 0, quality: 0, symbol: 0 },
      { n: 3, state: 'Live TV', channel: '7.1 Summit 7', signal: 81, quality: 92, symbol: 100 },
      { n: 4, state: 'Idle', channel: '—', signal: 0, quality: 0, symbol: 0 },
    ], channels: 7, atsc3: true },
  ],
  iptv: [{ id: 'm3u', name: 'IPTV playlist', url: 'http://192.168.1.20:8080/playlist.m3u', channels: 7, streams: 'HLS · 1080p', lastRefresh: hoursAgo(3) }],
  guide: [
    { id: 'xmltv', name: 'XMLTV (Schedules Direct)', lineup: 'USA-OTA-04101', days: 14, lastUpdate: hoursAgo(3), channels: 14 },
  ],
  dvr: { path: '/volume1/media/DVR', format: 'MPEG-TS → MKV remux', commercialDetection: 'Mark chapters' as string, defaultPaddingBefore: 1, defaultPaddingAfter: 2, deleteWatchedAfter: '7 days' as string, maxDiskGb: 3000 },
};

// ---------- metadata ----------
export const metadataSeed = {
  agents: [
    { id: 'local', name: 'Local media assets (NFO, embedded tags, folder art)', enabled: true },
    { id: 'tmdb', name: 'The Movie Database', enabled: true },
    { id: 'tvdb', name: 'TheTVDB', enabled: true },
    { id: 'anidb', name: 'AniDB + AniList (anime)', enabled: true },
    { id: 'mb', name: 'MusicBrainz + Cover Art Archive', enabled: true },
    { id: 'ol', name: 'Open Library (audiobooks)', enabled: true },
    { id: 'lrc', name: 'LRCLIB (synced lyrics)', enabled: true },
    { id: 'os', name: 'OpenSubtitles', enabled: false },
  ],
  unmatched: [
    { id: 'u1', path: '/volume1/media/Movies/The.Understudy.2024.2160p.UHD.BluRay.x265-GRP/The.Understudy.2024.2160p.mkv', guess: 'The Understudy (2024)', candidates: ['The Understudy (2024) · tmdb 1184732', 'The Understudy (2008) · tmdb 32190'], library: 'Movies' },
    { id: 'u2', path: '/volume1/media/TV/Foxglove Lane/Season 2/Foxglove.Lane.S02E03.1080p.mkv', guess: 'Foxglove Lane S2E3', candidates: ['Foxglove Lane (2020) S2E3 “Lily of the Valley”'], library: 'TV Shows' },
    { id: 'u3', path: '/volume1/media/Music/Various/Unknown Album/track07.flac', guess: '—', candidates: [], library: 'Music' },
    { id: 'u4', path: '/volume1/media/Anime/Hoshikuzu Diner/[Group] Hoshikuzu Diner - 12 [1080p].mkv', guess: 'Stardust Diner E12', candidates: ['Stardust Diner (2022) E12 “Clear Skies”'], library: 'Anime' },
    { id: 'u5', path: '/volume2/home-videos/2026/IMG_8812.MOV', guess: 'Home video (no metadata needed)', candidates: [], library: 'Home Videos' },
  ],
  duplicates: [
    { id: 'dup1', title: 'Northbound (2023)', files: ['Northbound.2023.2160p.HDR.mkv · 58.2 GB', 'Northbound.2023.1080p.mkv · 11.4 GB'] },
    { id: 'dup2', title: 'Salt & Cedar — The Tidewater Band', files: ['FLAC 24/96 · 1.1 GB', 'MP3 320 · 142 MB'] },
  ],
  missingArt: 4,
  lastRefresh: daysAgo(6, 2),
};

// ---------- diagnostics ----------
export const diagnosticsSeed = {
  checks: [
    { id: 'c1', area: 'Database', name: 'Integrity check', status: 'ok' as const, detail: 'PRAGMA integrity_check: ok · 1.42 GB · WAL 3.1 MB' },
    { id: 'c2', area: 'Storage', name: 'Free space on /volume1', status: 'ok' as const, detail: '13.8 TB free (29%)' },
    { id: 'c3', area: 'Storage', name: 'SMART status', status: 'warn' as const, detail: 'sdc: 8 reallocated sectors (bay 3)' },
    { id: 'c4', area: 'Transcoding', name: 'Hardware encoder', status: 'ok' as const, detail: 'QSV: h264 ✓ hevc ✓ · test encode 1080p @ 412 fps' },
    { id: 'c5', area: 'Network', name: 'Remote access', status: 'ok' as const, detail: 'Reachable on 32500/tcp from 3 regions' },
    { id: 'c6', area: 'Network', name: 'TLS certificate', status: 'ok' as const, detail: '62 days remaining' },
    { id: 'c7', area: 'Live TV', name: 'Tuner signal', status: 'warn' as const, detail: '2.1 WHBR signal 61% — consider re-aiming the antenna' },
    { id: 'c8', area: 'Metadata', name: 'Provider reachability', status: 'ok' as const, detail: 'TMDB 84 ms · TVDB 131 ms · MusicBrainz 212 ms' },
    { id: 'c9', area: 'System', name: 'Time sync', status: 'ok' as const, detail: 'NTP offset +0.4 ms' },
    { id: 'c10', area: 'Updates', name: 'Server version', status: 'info' as const, detail: '3.2.2 available (you have 3.2.1)' },
  ],
};

export const settingsSeed = {
  friendlyName: 'Harbor',
  language: 'English (US)',
  timezone: 'America/New_York',
  updateChannel: 'Stable',
  autoUpdate: 'Notify only',
  telemetry: false,
  crashReports: true,
  directSignIn: true,
  cloudAccount: false,
  guestAccess: false,
  requireAuthOnLan: true,
  sessionLength: '90 days',
  loginRateLimit: '10 attempts / 15 min',
  dlna: false,
  dlnaName: 'Harbor (Portico)',
};
