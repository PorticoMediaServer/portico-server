import type {AdminService, AdminState} from '../admin';
import type {Episode, Movie} from '../types';
import {DAY, HOUR, MIN, NOW, type DB} from './db';
import {rng} from './rng';

const series = (seed: string, n: number, base: number, spread: number) => {
  const r = rng(seed);
  let v = base;
  return Array.from({length: n}, () => (v = Math.max(1, Math.min(100, v + (r.next() - 0.5) * spread))));
};

export function createAdmin(db: DB, bump: () => void): AdminService {
  const movies = [...db.items.values()].filter(i => i.kind === 'movie') as Movie[];
  const eps = [...db.items.values()].filter(i => i.kind === 'episode') as Episode[];
  const show = (e: Episode) => db.items.get(e.showId)?.title ?? '';
  const state: AdminState = {
    overview: {
      serverName: 'Ehler Home Theater', version: '1.4.2', uptimeDays: 23,
      health: [
        {label: 'Server', state: 'ok', detail: 'Running for 23 days · 4% processor'},
        {label: 'Storage', state: 'warn', detail: 'Media volume 94% full'},
        {label: 'Remote access', state: 'ok', detail: 'Reachable at portico.ehler.net'},
        {label: 'Metadata', state: 'ok', detail: 'All libraries matched'},
        {label: 'Live TV', state: 'ok', detail: '2 sources · 4 tuners'},
        {label: 'Backups', state: 'ok', detail: 'Last backup 6 hours ago'},
      ],
      attention: [
        {id: 'at1', severity: 'urgent', title: 'The Media volume is almost full', body: '6% free (412 GB). Recordings and conversions may start failing below 5%.', section: 'maintenance', acknowledged: false},
        {id: 'at2', severity: 'warn', title: '14 files need attention in Anime', body: 'Portico couldn’t tell which show, season or episode these files are.', section: 'libraries', acknowledged: false},
        {id: 'at3', severity: 'info', title: 'A device is waiting for approval', body: 'Bedroom TV signed in and can’t browse until you approve it.', section: 'access', acknowledged: false},
        {id: 'at4', severity: 'info', title: 'Portico 1.4.3 is available', body: 'Fixes for subtitle timing and faster library scans.', section: 'maintenance', acknowledged: true},
      ],
      telemetry: {cpu: series('cpu', 48, 18, 14), memory: series('mem', 48, 46, 4), netOut: series('net', 48, 30, 22), diskRead: series('disk', 48, 22, 18), gpu: series('gpu', 48, 12, 16), viewingHours: series('view', 30, 40, 30).map(v => Math.round(v / 6))},
      recent: [
        {title: movies[3]?.title ?? '', profile: 'Ava', at: NOW - 40 * MIN, device: 'Ava’s iPad'},
        {title: eps[10] ? `${show(eps[10])} · S1 E${eps[10].number}` : '', profile: 'Justin', at: NOW - 3 * HOUR, device: 'Living Room Apple TV'},
        {title: movies[8]?.title ?? '', profile: 'Leo', at: NOW - 7 * HOUR, device: 'Safari on Windows'},
        {title: movies[12]?.title ?? '', profile: 'Kids', at: NOW - 1 * DAY, device: 'Living Room Apple TV'},
      ],
      storage: [{label: 'Media', usedTB: 15.1, totalTB: 16}, {label: 'Archive', usedTB: 6.2, totalTB: 12}, {label: 'System', usedTB: 0.31, totalTB: 1}],
    },
    nowPlaying: [
      {id: 'ps1', itemId: movies[3]?.id ?? '', title: movies[3]?.title ?? '', subtitle: `${movies[3]?.year}`, profile: 'Ava', account: 'Ava Ehler', device: 'Ava’s iPad', platform: 'iPadOS', state: 'playing', progress: 0.42, delivery: 'Direct play', location: 'Remote', rateMbps: 11.2, startedAt: NOW - 40 * MIN, quality: '1080p · H.264'},
      {id: 'ps2', itemId: eps[10]?.id ?? '', title: eps[10]?.title ?? '', subtitle: eps[10] ? `${show(eps[10])} · S${eps[10].seasonNumber} E${eps[10].number}` : '', profile: 'Leo', account: 'Leo Ehler', device: 'Bedroom Chromecast', platform: 'Google TV', state: 'playing', progress: 0.71, delivery: 'Converting video', location: 'Home network', rateMbps: 8.4, startedAt: NOW - 22 * MIN, quality: '4K HEVC → 1080p H.264', hardware: 'Intel Quick Sync'},
      {id: 'ps3', itemId: movies[30]?.id ?? '', title: movies[30]?.title ?? '', subtitle: `${movies[30]?.year}`, profile: 'Sam', account: 'Sam Okafor', device: 'Firefox on Linux', platform: 'Web', state: 'paused', progress: 0.12, delivery: 'Direct stream', location: 'Remote', rateMbps: 0, startedAt: NOW - 9 * MIN, quality: '1080p · remuxed to MP4'},
    ],
    activity: {tasks: [
      {id: 't1', name: 'Scan Anime', state: 'running', progress: 0.63, startedAt: NOW - 6 * MIN, detail: '412 of 655 files'},
      {id: 't2', name: 'Generate seek-bar pictures · Movies', state: 'running', progress: 0.21, startedAt: NOW - 34 * MIN, detail: '58 of 276 titles'},
      {id: 't3', name: 'Refresh guide · Antenna', state: 'queued', progress: 0, startedAt: NOW + 20 * MIN, detail: 'Waits for quiet hours'},
      {id: 't4', name: 'Back up database', state: 'done', progress: 1, startedAt: NOW - 6 * HOUR, detail: '184 MB · 42 seconds'},
      {id: 't5', name: 'Match new titles · TV Shows', state: 'done', progress: 1, startedAt: NOW - 9 * HOUR, detail: '12 matched, 0 need a choice'},
      {id: 't6', name: 'Fetch missing lyrics · Music', state: 'failed', progress: 0.4, startedAt: NOW - 1 * DAY, detail: 'The lyrics provider didn’t answer. Retrying tonight.'},
      {id: 't7', name: 'Remove deleted titles', state: 'done', progress: 1, startedAt: NOW - 1 * DAY - 2 * HOUR, detail: '3 titles · 18.4 GB freed'},
    ]},
    logs: {
      lines: Array.from({length: 60}, (_, i) => {
        const r = rng('log' + i);
        const level = r.pick(['info', 'info', 'info', 'warn', 'debug', 'error'] as const);
        const comp = r.pick(['scanner', 'playback', 'transcoder', 'auth', 'livetv', 'metadata', 'network', 'dvr']);
        const msgs: Record<string, string[]> = {
          scanner: ['Scanned /Volumes/Media/Anime/Season 02 (48 files)', 'Found 3 new files in Movies', 'Skipped sample file under 50 MB'],
          playback: ['Session started: Direct play for Ava’s iPad', 'Session paused after 30 minutes idle', 'Client switched audio track to English 5.1'],
          transcoder: ['Hardware encoder Quick Sync ready', 'Converting 4K HEVC to 1080p for Bedroom Chromecast', 'Conversion finished in 0.6× real time'],
          auth: ['Device “Bedroom TV” is waiting for approval', 'Sign-in for justin via Portico Account', 'Refused an expired session token'],
          livetv: ['Guide refreshed: 42 channels, 2,814 programs', 'Tuner 2 released', 'Channel 7.1 retuned after a signal drop'],
          metadata: ['Matched “Glass Harbor” (2021)', 'Artwork refreshed for 12 titles', 'Provider rate limit reached; backing off 30s'],
          network: ['Router mapping on port 32500 renewed', 'Certificate renews in 41 days', 'Public address unchanged'],
          dvr: ['Recording started on 4.1', 'Recording finished: 1.8 GB', 'Tuner conflict at 8:00 PM; lower-priority rule skipped'],
        };
        return {id: 'l' + i, at: NOW - i * 97 * 1000, level, component: comp, message: r.pick(msgs[comp])};
      }),
      level: 'info', following: true, capture: {component: 'playback', minutes: 15, until: null}, retention: {records: 30, notifications: 90, tasks: 60},
      export: {window: '24 hours', server: true, client: true, audit: false, snapshot: true, ready: false},
    },
    diagnostics: {
      capabilities: [
        {id: 'live_tv', label: 'Live TV', state: 'available'},
        {id: 'recording', label: 'Recording', state: 'available'},
        {id: 'prepared_media', label: 'Prepared media', state: 'available'},
        {id: 'hardware_encoding', label: 'Hardware encoding', state: 'available', technical: 'Intel Quick Sync (VAAPI) · iHD driver 24.1'},
        {id: 'decoder_sandbox', label: 'Decoder isolation', state: 'unavailable', reason: 'This host can’t isolate the video tools, so conversions run with baseline protections.', technical: 'seccomp unavailable in container runtime'},
        {id: 'dolby_vision_conversion', label: 'Dolby Vision conversion', state: 'unknown', reason: 'The server hasn’t checked this yet. It does when the feature is first used or set up.'},
      ],
      checks: [
        {label: 'Reachable from outside', state: 'ok', detail: 'Portico Account relay confirmed https://portico.ehler.net:32500'},
        {label: 'Home network discovery', state: 'ok', detail: 'Announced on 192.168.1.20'},
        {label: 'Clock', state: 'ok', detail: 'Within 40 ms of network time'},
        {label: 'FFmpeg', state: 'ok', detail: 'Portico FFmpeg 7.1 · all codecs present'},
        {label: 'Disk speed', state: 'warn', detail: 'Media volume reads at 88 MB/s; 4K conversions may stutter under load'},
      ],
    },
    feedback: [
      {id: 'f1', from: 'Ava (Ava Ehler)', category: 'Subtitles', message: 'The Spanish subtitles for this are about two seconds late from the start.', title: movies[5]?.title, at: NOW - 5 * HOUR, status: 'new', reply: ''},
      {id: 'f2', from: 'Leo (Leo Ehler)', category: 'Wrong match', message: 'This is showing the poster and description for a different film with the same name.', title: movies[17]?.title, at: NOW - 2 * DAY, status: 'looking', reply: 'Thanks, I’ll fix the match tonight.'},
      {id: 'f3', from: 'Sam (Sam Okafor)', category: 'Playback', message: 'Buffers every few minutes when I watch from work.', at: NOW - 4 * DAY, status: 'fixed', reply: 'Raised the remote quality limit; try again.'},
      {id: 'f4', from: 'Kids', category: 'Other', message: 'can we have more dinosaur movies', at: NOW - 9 * DAY, status: 'wontFix', reply: ''},
    ],
    libraries: db.libraries.map((l, i) => ({
      id: l.id, name: l.name, kind: {movies: 'Movies', shows: 'TV', anime: 'Anime', music: 'Music', audiobooks: 'Audiobooks', photos: 'Photos', homevideos: 'Home videos'}[l.kind], items: l.count, sizeTB: [9.8, 4.1, 1.2, 0.4, 0.2, 0.3, 0.6][i] ?? 0.1, lastScan: NOW - (i + 1) * 3 * HOUR, scanning: l.scanning, scanProgress: l.scanning ? 0.63 : 0,
      folders: l.sourcePaths.map((p, k) => ({id: `${l.id}-f${k}`, path: p, location: p.startsWith('/Volumes/Archive') ? 'Network share' : 'Local disk', available: true, depth: 6})),
      attention: l.kind === 'anime' ? Array.from({length: 14}, (_, k) => ({path: `/Volumes/Media/Anime/Unsorted/[Fansub] Episode ${String(k + 3).padStart(2, '0')} [1080p].mkv`, reason: 'Couldn’t tell which show this episode belongs to'})) : [],
      metadata: l.kind === 'homevideos' || l.kind === 'photos' ? 'local' : 'online', language: 'en', country: 'US', rescan: 'Every hour', missingAfter: '1 week', trickplayEvery: 10, trickplayWidth: 320, chapterPictures: true, allowDelete: l.kind === 'movies', keepDeletedDays: 30, fetchLyrics: l.kind === 'music',
    })),
    metadata: {providers: [
      {id: 'tmdb', name: 'TMDB', enabled: true, kinds: 'Movies, TV, people', key: false},
      {id: 'tvdb', name: 'TheTVDB', enabled: true, kinds: 'TV episode orders', key: true},
      {id: 'anilist', name: 'AniList', enabled: true, kinds: 'Anime', key: false},
      {id: 'musicbrainz', name: 'MusicBrainz', enabled: true, kinds: 'Artists, albums', key: false},
      {id: 'openlibrary', name: 'Open Library', enabled: true, kinds: 'Audiobooks', key: false},
      {id: 'lrclib', name: 'LRCLIB', enabled: true, kinds: 'Lyrics', key: false},
    ], consent: 'granted', language: 'en', country: 'US', refreshDays: 30, lyrics: true},
    storage: [
      {id: 'st1', kind: 'webdav', name: 'Nextcloud Family', address: 'https://cloud.ehler.net/remote.php/dav/files/media', state: 'mounted', usedTB: 0.8, libraries: ['Photos']},
      {id: 'st2', kind: 'rclone', name: 'Backblaze archive', address: 'b2:ehler-archive/classics', state: 'mounted', usedTB: 2.4, libraries: ['Movies']},
      {id: 'st3', kind: 'rclone', name: 'Old Drive export', address: 'gdrive:Media/Old', state: 'error', usedTB: 0, libraries: []},
    ],
    liveTv: {
      sources: [
        {id: 'src1', name: 'Living room antenna', kind: 'HDHomeRun', address: '192.168.1.44', channels: 28, tuners: 4, paused: false, lastRefresh: NOW - 3 * HOUR, guideFrom: null, numbering: 'asSent', startAt: 1, logos: true, health: 'ok'},
        {id: 'src2', name: 'City IPTV', kind: 'M3U', address: 'https://iptv.example.net/playlist.m3u', channels: 14, tuners: 2, paused: false, lastRefresh: NOW - 7 * HOUR, guideFrom: 'src3', numbering: 'inOrder', startAt: 100, logos: true, health: 'warn'},
        {id: 'src3', name: 'XMLTV listings', kind: 'Guide only', address: 'https://guide.example.net/xmltv.xml', channels: 0, tuners: 0, paused: false, lastRefresh: NOW - 7 * HOUR, guideFrom: null, numbering: 'asSent', startAt: 1, logos: false, health: 'ok'},
      ],
      recordingsPath: '/Volumes/Media/Recordings', writable: true, keepDays: 30, episodesPerSeries: 5, leaveFreeGB: 100, capGB: 2000, usedGB: 438, totalGB: 2000,
    },
    dvr: {
      startEarly: 1, endLate: 3, keepMode: 'newest', keepCount: 5, afterRecording: 'remux', removeOriginal: true, folderShows: '{show}/Season {season}/{show} - S{season}E{episode}', folderMovies: '{title} ({year})', sidecarNfo: true, sidecarFanart: false,
      tuners: [
        {id: 'tu1', name: 'Tuner 1', source: 'Living room antenna', inUse: db.recordings.find(r => r.state === 'recording')?.title ?? null, next: 'Evening news · 6:00 PM'},
        {id: 'tu2', name: 'Tuner 2', source: 'Living room antenna', inUse: 'Watching: 7.1 (Leo)', next: null},
        {id: 'tu3', name: 'Tuner 3', source: 'Living room antenna', inUse: null, next: '8:00 PM'},
        {id: 'tu4', name: 'Tuner 4', source: 'Living room antenna', inUse: null, next: null},
      ],
      buffer: 4, retry: 30, guideDays: 14, userAgent: '', logos: true, discovery: true,
      recorders: [
        {profile: 'Justin', account: 'Justin Ehler', allowed: true, inherit: true},
        {profile: 'Ava', account: 'Ava Ehler', allowed: true, inherit: false},
        {profile: 'Leo', account: 'Leo Ehler', allowed: false, inherit: false},
      ],
      rules: db.rules.map(r => ({id: r.id, name: r.title, owner: 'Justin', channel: db.channels.find(c => c.id === r.channelId)?.name ?? 'Any channel', newOnly: r.newOnly, paused: r.paused, priority: r.priority})),
    },
    channels: db.channels.filter(c => c.library).map((c, i) => ({
      id: c.id, name: c.name, number: c.number, on: i !== 3, libraries: ['Movies'], kinds: ['movies'], order: i % 2 ? 'Shuffle, no repeats' : 'In order, then shuffled', episodes: 'Mixed with everything else',
      conditions: [{field: 'Genre', op: 'is', value: ['Crime', 'Comedy', 'Horror', 'Family', 'Science Fiction', 'Drama'][i % 6]}], scheduleHours: 72 - i * 6, candidates: 40 + i * 13, unresolved: i === 2 ? 3 : 0, health: i === 1 ? 'beingMade' : 'upToDate', description: 'Always-on channel built from your library.',
    })),
    transcoding: {allow: true, serverCap: 4, accountCap: 2, hardware: 'auto', overrides: [{account: 'Sam Okafor', cap: 1}], tonemap: true, sandbox: false},
    network: {remote: true, publicAddress: '198.51.100.23', publicPort: 32500, localAddresses: ['192.168.1.20', 'fd12:3456:789a::20'], mapping: {enabled: true, state: 'open', protocol: 'UPnP IGD'}, certificate: {state: 'trusted', domain: 'portico.ehler.net', expires: NOW + 41 * DAY}, outside: 'everyone', encrypted: 'required', discovery: true, qualityLimit: 12, uploadSpeed: 40, pausedLimit: 60, homeNetworks: '', extraAddresses: 'https://media.ehler.net', wanAsLan: false, ipv6: true, trustedProxies: '', interface: 'auto'},
    people: [
      {id: 'ac1', name: 'Justin Ehler', username: 'justin', kind: 'Portico Account', role: 'owner', disabled: false, profiles: db.profiles.map(p => ({id: p.id, name: p.name, color: p.color, kids: p.kids})), libraries: 'all', limits: {streams: 0, devices: 0, remoteMbps: 0, rating: null}, lastSeen: NOW},
      {id: 'ac2', name: 'Maya Ehler', username: 'maya', kind: 'Portico Account', role: 'admin', disabled: false, profiles: [{id: 'mp1', name: 'Maya', color: '#8a5fd0', kids: false}], libraries: 'all', limits: {streams: 0, devices: 0, remoteMbps: 0, rating: null}, lastSeen: NOW - 2 * DAY},
      {id: 'ac3', name: 'Sam Okafor', username: 'sam', kind: 'Portico Account', role: 'member', disabled: false, profiles: [{id: 'sp1', name: 'Sam', color: '#e8bc6a', kids: false}], libraries: ['Movies', 'TV Shows', 'Music'], limits: {streams: 2, devices: 4, remoteMbps: 8, rating: null}, lastSeen: NOW - 9 * MIN},
      {id: 'ac4', name: 'Grandma Ruth', username: 'ruth', kind: 'Server account', role: 'member', disabled: false, profiles: [{id: 'rp1', name: 'Ruth', color: '#2fa4a0', kids: false}], libraries: ['Movies', 'Photos', 'Home Videos'], limits: {streams: 1, devices: 2, remoteMbps: 6, rating: null}, lastSeen: NOW - 5 * DAY},
      {id: 'ac5', name: 'Old roommate', username: 'chris', kind: 'Server account', role: 'member', disabled: true, profiles: [{id: 'cp1', name: 'Chris', color: '#6f8796', kids: false}], libraries: ['Movies'], limits: {streams: 1, devices: 1, remoteMbps: 4, rating: 'R'}, lastSeen: NOW - 120 * DAY},
    ],
    access: {
      invites: [
        {id: 'iv1', label: 'uncle.dave@example.com', role: 'member', expires: NOW + 6 * DAY, code: 'H7K2-P9QX', link: 'https://getportico.tv/join#H7K2-P9QX'},
        {id: 'iv2', label: 'Invitation', role: 'admin', expires: NOW + 1 * DAY, code: 'M3RT-8WQA', link: 'https://getportico.tv/join#M3RT-8WQA'},
      ],
      devices: [...db.sessions.map(s => ({id: s.id, name: s.name, platform: s.platform as string, account: 'Justin Ehler', trust: (s.waiting ? 'waiting' : 'approved') as 'waiting' | 'approved', lastSeen: s.lastUsed})), {id: 'dx', name: 'Unknown Roku', platform: 'roku', account: 'Old roommate', trust: 'blocked' as const, lastSeen: NOW - 30 * DAY}],
      keys: [
        {id: 'k1', name: 'Home Assistant', access: 'Read only', hint: 'x7Q2', lastUsed: NOW - 12 * MIN, created: NOW - 200 * DAY},
        {id: 'k2', name: 'Backup script', access: 'Full', hint: '9aLm', lastUsed: NOW - 6 * HOUR, created: NOW - 90 * DAY},
      ],
    },
    schedules: [
      {id: 'sc1', name: 'Scan libraries', description: 'Find new, changed and missing files.', enabled: true, start: '02:00', lengthMin: 120, catchUp: true, lastRun: NOW - 22 * HOUR, nextRun: NOW + 2 * HOUR, lastResult: 'ok'},
      {id: 'sc2', name: 'Refresh metadata', description: 'Update details and artwork from providers.', enabled: true, start: '03:00', lengthMin: 90, catchUp: false, lastRun: NOW - 3 * DAY, nextRun: NOW + 4 * DAY, lastResult: 'ok'},
      {id: 'sc3', name: 'Generate seek-bar pictures', description: 'Make scrubbing previews for new titles.', enabled: true, start: '01:00', lengthMin: 240, catchUp: true, lastRun: NOW - 21 * HOUR, nextRun: NOW + 3 * HOUR, lastResult: 'ok'},
      {id: 'sc4', name: 'Back up database', description: 'Copy the database and settings.', enabled: true, start: '04:30', lengthMin: 30, catchUp: true, lastRun: NOW - 6 * HOUR, nextRun: NOW + 18 * HOUR, lastResult: 'ok'},
      {id: 'sc5', name: 'Fetch missing lyrics', description: 'Look up lyrics for songs without them.', enabled: false, start: '05:00', lengthMin: 60, catchUp: false, lastRun: NOW - 1 * DAY, nextRun: 0, lastResult: 'failed'},
      {id: 'sc6', name: 'Remove deleted titles', description: 'Empty deleted titles past their keep period.', enabled: true, start: '05:30', lengthMin: 30, catchUp: true, lastRun: NOW - 1 * DAY, nextRun: NOW + 23 * HOUR, lastResult: 'ok'},
    ],
    maintenance: {
      quietHours: [{id: 'q1', name: 'Overnight', days: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'], start: '01:00', hours: 6, enabled: true}, {id: 'q2', name: 'Weekday afternoons', days: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri'], start: '10:00', hours: 5, enabled: false}],
      onlyInWindows: true, priority: 'lower',
      buckets: [
        {id: 'b1', name: 'Seek-bar pictures', description: 'Scrubbing previews for every title.', sizeGB: 38.2, canRecreate: true},
        {id: 'b2', name: 'Converted media cache', description: 'Recently converted streams kept for reuse.', sizeGB: 21.7, canRecreate: true},
        {id: 'b3', name: 'Artwork', description: 'Posters, backdrops and pictures.', sizeGB: 9.4, canRecreate: true},
        {id: 'b4', name: 'Prepared downloads', description: 'Files prepared for people to save.', sizeGB: 12.1, canRecreate: true},
        {id: 'b5', name: 'Logs and support exports', description: 'Records kept for the retention period.', sizeGB: 0.8, canRecreate: false},
      ],
      trash: [
        {id: 'tr1', title: movies[40]?.title ?? 'Old copy', files: 1, sizeGB: 7.9, deletedAt: NOW - 3 * DAY, keptUntil: NOW + 27 * DAY, due: false},
        {id: 'tr2', title: movies[41]?.title ?? 'Duplicate', files: 2, sizeGB: 22.4, deletedAt: NOW - 31 * DAY, keptUntil: NOW - DAY, due: true},
        {id: 'tr3', title: 'Unsorted/sample.mkv', files: 1, sizeGB: 0.2, deletedAt: NOW - 40 * DAY, keptUntil: NOW - 10 * DAY, due: true},
      ],
      update: {current: '1.4.2', latest: '1.4.3', channel: 'Stable', builtAt: NOW - 18 * DAY, available: true, notes: ['Subtitles stay in sync after seeking in long films', 'Library scans are up to 30% faster on network shares', 'Guide refreshes no longer pause recordings']},
      permissionsOk: false,
      broadcast: {title: '', message: '', audience: 'everyone', importance: 'info'},
    },
    backups: {list: [
      {id: 'bk-0926', kind: 'Scheduled', at: NOW - 6 * HOUR, sizeMB: 184, schema: 42, version: '1.4.2'},
      {id: 'bk-0925', kind: 'Scheduled', at: NOW - 30 * HOUR, sizeMB: 183, schema: 42, version: '1.4.2'},
      {id: 'bk-0921', kind: 'Manual', at: NOW - 5 * DAY, sizeMB: 180, schema: 42, version: '1.4.2'},
      {id: 'bk-0910', kind: 'Before restore', at: NOW - 16 * DAY, sizeMB: 171, schema: 41, version: '1.4.1'},
    ], keep: 7, schedule: 'Daily at 4:30 AM', lastResult: 'Backup complete · 42 seconds'},
    alerts: {storageWarning: 10, storageCritical: 5, certificateDays: 14, offlineMinutes: 30, failedScans: true},
    general: {name: 'Ehler Home Theater', version: '1.4.2 (Stable)', serverId: 'srv_7c1f9a2e44d0', connection: 'connected', ownerAccount: 'justin@example.com', recoveryOwner: 'admin (local)', platform: 'Ubuntu 24.04 · Docker · x86-64', dataFolder: '/var/lib/portico'},
  };

  const wait = (ms = 160) => new Promise<void>(r => setTimeout(r, ms));
  return {
    async get(key) {
      await wait();
      return state[key];
    },
    async set(key, value) {
      await wait(120);
      state[key] = value;
      bump();
    },
    async run(op, args = {}) {
      await wait(350);
      switch (op) {
        case 'stopSession':
          state.nowPlaying = state.nowPlaying.filter(s => s.id !== args.id);
          break;
        case 'acknowledge':
          state.overview.attention = state.overview.attention.map(a => (a.id === args.id ? {...a, acknowledged: true} : a));
          break;
        case 'scan':
          state.libraries = state.libraries.map(l => (l.id === args.id ? {...l, scanning: true, scanProgress: 0.05} : l));
          break;
        case 'cancelScan':
          state.libraries = state.libraries.map(l => (l.id === args.id ? {...l, scanning: false, scanProgress: 0} : l));
          break;
        case 'backup':
          state.backups.list = [{id: 'bk-new-' + state.backups.list.length, kind: 'Manual', at: Date.now(), sizeMB: 185, schema: 42, version: '1.4.2'}, ...state.backups.list];
          bump();
          return 'Backup complete.';
        case 'deleteBackup':
          state.backups.list = state.backups.list.filter(b => b.id !== args.id);
          break;
        case 'putBack':
          state.maintenance.trash = state.maintenance.trash.filter(t => t.id !== args.id);
          break;
        case 'removeDue':
          state.maintenance.trash = state.maintenance.trash.filter(t => !t.due);
          break;
        case 'removeAll':
          state.maintenance.trash = [];
          break;
        case 'fixPermissions':
          state.maintenance.permissionsOk = true;
          break;
        case 'createInvite': {
          const code = Math.random().toString(36).slice(2, 6).toUpperCase() + '-' + Math.random().toString(36).slice(2, 6).toUpperCase();
          state.access.invites = [{id: 'iv' + Date.now(), label: String(args.label || 'Invitation'), role: (args.role as 'member' | 'admin') ?? 'member', expires: Date.now() + 7 * DAY, code, link: 'https://getportico.tv/join#' + code}, ...state.access.invites];
          bump();
          return code;
        }
        case 'createKey':
          state.access.keys = [{id: 'k' + Date.now(), name: String(args.name || 'New key'), access: (args.access as 'Read only') ?? 'Read only', hint: 'Qe4Z', lastUsed: null, created: Date.now()}, ...state.access.keys];
          bump();
          return 'ptk_demo_Qe4Zr8m2Hn0wXb7cV1sT5yL9kD3fJ6aP';
        default:
          break;
      }
      bump();
      return 'ok';
    },
  };
}
