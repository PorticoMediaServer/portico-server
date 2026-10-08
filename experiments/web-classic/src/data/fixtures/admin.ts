import type {AdminAction, AdminField, AdminPage, AdminSectionMeta, Block, ListItem, Tone} from '../adminTypes';
import type {AdminApi} from '../adminApi';
import {catalog, channelsRaw, logoFor, personArt, posterFor} from './catalog';
import {devices, profilesByServer} from './household';
import {hash, latency, rng} from './util';


export const adminSections: AdminSectionMeta[] = [
  {id: 'overview', label: 'Overview', group: 'Console', icon: 'console', lede: 'Health, attention items and who is watching right now.'},
  {id: 'now-playing', label: 'Now Playing', group: 'Console', icon: 'play', lede: 'Active playback sessions on this server.'},
  {id: 'activity', label: 'Activity', group: 'Console', icon: 'activity', lede: 'Playback sessions, scans and background tasks.'},
  {id: 'logs', label: 'Logs', group: 'Console', icon: 'list', lede: 'Server logs, detailed capture for troubleshooting, and support bundles.'},
  {id: 'diagnostics', label: 'Diagnostics', group: 'Console', icon: 'gauge', lede: 'What this server can do right now, and why not when it can’t.'},
  {id: 'feedback', label: 'Viewer feedback', group: 'Console', icon: 'mail', lede: 'Problems viewers reported from their apps.'},
  {id: 'alerts', label: 'Alerts', group: 'Console', icon: 'bell', lede: 'When the server puts a warning in your notifications.'},
  {id: 'libraries', label: 'Libraries', group: 'Media', icon: 'folder', lede: 'What the server knows about and where it comes from.'},
  {id: 'library-settings', label: 'Library settings', group: 'Media', icon: 'sliders', lede: 'Matching, scrubbing pictures, scanning and deleting, per library.'},
  {id: 'metadata', label: 'Metadata', group: 'Media', icon: 'tag', lede: 'Where titles, artwork and details come from, and what needs a match.'},
  {id: 'storage', label: 'Storage', group: 'Media', icon: 'storage', lede: 'Remote storage attached to this server.'},
  {id: 'live-tv', label: 'Live TV', group: 'Live TV', icon: 'live', lede: 'Tuners, playlists and the program guide.'},
  {id: 'dvr', label: 'DVR', group: 'Live TV', icon: 'recordings', lede: 'Recording defaults, tuners, who can record and recording space.'},
  {id: 'channels', label: 'Library Channels', group: 'Live TV', icon: 'channels', lede: 'Always-on channels generated from your libraries.'},
  {id: 'channel-builder', label: 'Channel Builder', group: 'Live TV', icon: 'wand', lede: 'Describe what a Library Channel plays and preview its schedule.'},
  {id: 'transcoding', label: 'Transcoding', group: 'Server', icon: 'transcode', lede: 'Whether the server may convert media, and how much at once.'},
  {id: 'network', label: 'Network', group: 'Server', icon: 'network', lede: 'How devices reach this server at home and away.'},
  {id: 'people', label: 'People', group: 'Server', icon: 'together', lede: 'Who can use this server and what they may see.'},
  {id: 'access', label: 'Access & API keys', group: 'Server', icon: 'shield', lede: 'Roles, per-person limits, invitations, device approval and API keys.'},
  {id: 'schedules', label: 'Scheduled tasks', group: 'Server', icon: 'calendar', lede: 'When heavy maintenance is allowed to run.'},
  {id: 'maintenance', label: 'Maintenance', group: 'Server', icon: 'broom', lede: 'Quiet hours, server storage, deleted titles and updates.'},
  {id: 'backups', label: 'Backups', group: 'Server', icon: 'archive', lede: 'The server’s database and settings, kept as plain files you can copy.'},
  {id: 'general', label: 'General', group: 'Server', icon: 'settings', lede: 'Server name, Portico Account connection and version.'},
];

const MIN = 60000;
const ago = (ms: number) => { const m = Math.round(ms / MIN); return m < 1 ? 'Just now' : m < 60 ? `${m} min ago` : m < 1440 ? `${Math.round(m / 60)} h ago` : `${Math.round(m / 1440)} days ago`; };
const clock = (t: number) => new Date(t).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
const spark = (seed: string, n = 24, base = 30, amp = 25) => { const r = rng(hash(seed)); let v = base; return Array.from({length: n}, () => (v = Math.max(2, Math.min(100, v + (r() - 0.5) * amp)))); };
const act = (id: string, label: string, extra: Partial<AdminAction> = {}): AdminAction => ({id, label, ...extra});


/** The server's disks, as the folder picker sees them. */
const FOLDER_TREE: Record<string, string[]> = {
  '/': ['media', 'mnt', 'Volumes', 'home', 'var', 'root'],
  '/media': ['Movies', 'TV', 'Anime', 'Family', 'Music', 'Audiobooks', 'Photos', 'Home Videos', 'Downloads'],
  '/media/Movies': ['Classics', 'Blender', '4K'],
  '/media/TV': ['Harbor Station', 'Copper County', 'Kessler & Vane'],
  '/media/Music': ['The Lantern Keepers', 'Ember Choir', 'Paper Kites of June'],
  '/media/Downloads': ['incoming', 'done'],
  '/mnt': ['nas', 'backup'],
  '/mnt/nas': ['Movies', 'Shows', 'Concerts', 'Scans'],
  '/mnt/backup': [],
  '/Volumes': ['Archive 8TB', 'Scratch'],
  '/Volumes/Archive 8TB': ['Films 1920-1959', 'Films 1960-1999', 'Documentaries'],
  '/home': ['justin'],
  '/home/justin': ['Videos', 'Music', 'Pictures'],
  '/var': ['lib'],
  '/var/lib': ['portico'],
  '/var/lib/portico': ['dvr', 'transcode', 'backups'],
};
const NO_ACCESS = new Set(['/root', '/mnt/backup']);
const LINKS = new Set(['/media/Downloads', '/Volumes/Scratch']);
const join = (dir: string, name: string) => (dir === '/' ? `/${name}` : `${dir}/${name}`);

/** `viewer` is the household's client, so a scan started here also shows in the sidebar, as it would on a real server. */
export function createAdminFixtures(viewer?: {libraries: {scan(id: string): Promise<void>}}): AdminApi {
  const listeners = new Set<() => void>();
  const emit = () => listeners.forEach(fn => fn());
  const values = new Map<string, unknown>(Object.entries({
    'transcoding.allow': true, 'transcoding.serverCap': 4, 'transcoding.accountCap': 2, 'transcoding.hardware': 'videotoolbox', 'transcoding.tonemap': true, 'transcoding.throttle': true, 'transcoding.tempPath': '/var/lib/portico/transcode',
    'network.remote': true, 'network.mapping': true, 'network.publicPort': 32500, 'network.gateway': '', 'network.outside': 'everyone', 'network.encrypted': 'preferred', 'network.discovery': true, 'network.qualityLimit': 20, 'network.homeNetworks': '192.168.1.0/24', 'network.extraAddresses': 'https://media.ehler.family', 'network.uploadSpeed': 40, 'network.pausedLimit': '60', 'network.wanAsLan': false, 'network.ipv6': true, 'network.trustedProxies': '', 'network.interface': 'auto',
    'alerts.storageWarning': 15, 'alerts.storageCritical': 5, 'alerts.certificateWarning': 14,
    'dvr.startEarly': 1, 'dvr.finishLate': 3, 'dvr.keep': 'all', 'dvr.keepCount': 5, 'dvr.after': 'remux', 'dvr.removeOriginal': false, 'dvr.folderShows': '{show}/Season {season}', 'dvr.folderMovies': '{title} ({year})', 'dvr.buffer': 4, 'dvr.retry': 30, 'dvr.guideDays': 14, 'dvr.userAgent': '', 'dvr.logos': true, 'dvr.discovery': true, 'dvr.leaveFree': 50, 'dvr.cap': 800, 'dvr.retention': 30,
    'maintenance.pauseHeavy': true, 'maintenance.priority': 'lower', 'maintenance.keepBackups': 7, 'maintenance.keepLogs': 14, 'maintenance.keepTasks': 30, 'maintenance.keepNotifications': 60,
    'general.name': 'Lighthouse', 'logs.level': 'info', 'logs.component': 'playback', 'logs.duration': '15', 'feedback.status': 'open',
  }));
  const v = <T,>(k: string) => values.get(k) as T;
  const setting = (kind: AdminField['kind'], key: string, label: string, help?: string, extra: Record<string, unknown> = {}): AdminField => ({kind, key, label, help, value: values.get(key), ...extra} as AdminField);

  // ── Mutable state ──────────────────────────────────────────────────
  const now = Date.now();
  let sessions = [
    {id: 'ps1', viewer: 'Maya', account: 'maya.ehler', device: "Maya's iPad", platform: 'iPad', item: 'm-charade-1963', title: 'Charade', state: 'Playing', delivery: 'Direct play', location: 'Home network', rate: 12.4, progress: 0.46, started: now - 52 * MIN, quality: '1080p · H.264'},
    {id: 'ps2', viewer: 'Theo', account: 'justin', device: 'Living Room Apple TV', platform: 'Apple TV', item: 'm-big-buck-bunny-2008', title: 'Big Buck Bunny', state: 'Playing', delivery: 'Direct play', location: 'Home network', rate: 8.1, progress: 0.72, started: now - 7 * MIN, quality: '4K · HEVC'},
    {id: 'ps3', viewer: 'Nora', account: 'nora.k', device: 'Chrome on Windows', platform: 'Web', item: catalog.shows[2]!.id, title: `${catalog.shows[2]!.title} · S1 E3`, state: 'Paused', delivery: 'Converting video', location: 'Remote', rate: 4.2, progress: 0.18, started: now - 21 * MIN, quality: '1080p → 720p'},
    {id: 'ps4', viewer: 'Walter', account: 'walter.e', device: 'SHIELD Android TV', platform: 'Android TV', item: 'ch-harbor-one', title: 'Harbor One · The Evening Six', state: 'Playing', delivery: 'Direct stream', location: 'Remote', rate: 6.6, progress: 0.3, started: now - 12 * MIN, quality: 'Live · 1080i'},
  ];
  let custodyPending = true;
  let alerts = [
    {id: 'al1', tone: 'warn' as Tone, title: 'Studio Annex hasn’t checked in for 3 days', body: 'Portico hasn’t heard from this server since Tuesday. Check its power and network connection.'},
    {id: 'al2', tone: 'bad' as Tone, title: 'A device is waiting for approval', body: 'Chrome on Windows (Denver, CO) signed in and can’t browse until you approve it under Access.'},
    {id: 'al3', tone: 'warn' as Tone, title: '14 files need attention in TV Shows', body: 'Portico couldn’t tell which show, season or episode these files are.'},
  ];
  let reports = [
    {id: 'fb1', who: 'Nora', category: 'Playback', about: 'Harbor Station · S2 E4', when: now - 3 * 3600000, status: 'open', message: 'The audio drops out about 12 minutes in and doesn’t come back until I restart the episode.'},
    {id: 'fb2', who: 'Maya', category: 'Metadata', about: 'Low Tide at Kessock', when: now - 28 * 3600000, status: 'open', message: 'The poster is for a different film, and the year says 1998 but it came out in 2019.'},
    {id: 'fb3', who: 'Theo', category: 'Something else', about: undefined, when: now - 72 * 3600000, status: 'answered', message: 'Can we get more cartoons on the Saturday Cartoons channel please?'},
    {id: 'fb4', who: 'Walter', category: 'Subtitles', about: 'Metropolis', when: now - 150 * 3600000, status: 'closed', message: 'The English intertitle subtitles are out of sync after the intermission.'},
  ];
  let libraries = catalog.libraries.map((l, i) => ({...l, path: ['/media/Movies', '/media/TV', '/media/Anime', '/media/Family', '/media/Music', '/media/Audiobooks', '/media/Photos', '/media/Home Videos'][i]!, folders: [['/media/Movies', '/mnt/nas/Films'], ['/media/TV'], ['/media/Anime'], ['/media/Family'], ['/media/Music', '/mnt/nas/Vinyl Rips'], ['/media/Audiobooks'], ['/media/Photos'], ['/media/Home Videos']][i]!, scanning: false, lastScan: `${2 + i * 3} hours ago`, attention: i === 1 ? 14 : i === 4 ? 3 : 0, source: i === 4 ? 'local' : 'online', settings: {language: 'en', country: 'US', every: 10, width: 320, chapters: true, allowDelete: i === 0 || i === 7, keepDays: 30, interval: 'hourly', missing: 'week'}}));
  let backups = [
    {id: 'bk-0926', kind: 'Scheduled', when: 'Today, 3:00 am', size: '184 MB', schema: '42', version: '1.4.2'},
    {id: 'bk-0925', kind: 'Scheduled', when: 'Yesterday, 3:00 am', size: '183 MB', schema: '42', version: '1.4.2'},
    {id: 'bk-0921', kind: 'Manual', when: 'Sep 21, 9:40 pm', size: '181 MB', schema: '42', version: '1.4.1'},
    {id: 'bk-0918', kind: 'Before restore', when: 'Sep 18, 11:02 am', size: '179 MB', schema: '41', version: '1.4.0'},
    {id: 'bk-0912', kind: 'Scheduled', when: 'Sep 12, 3:00 am', size: '176 MB', schema: '41', version: '1.4.0'},
  ];
  let invitations = [
    {id: 'iv1', label: 'Grandma Rosa', role: 'Member', expires: 'Oct 2'},
    {id: 'iv2', label: 'Book club', role: 'Member', expires: 'Sep 30'},
  ];
  let apiKeys = [
    {id: 'k1', name: 'Home Assistant', access: 'Read only', hint: '9f2c', used: '2 min ago'},
    {id: 'k2', name: 'Backup script', access: 'Library management', hint: 'a71d', used: 'Yesterday'},
  ];
  let accessDevices = devices.map(d => ({...d}));
  let people = [
    {id: 'u-justin', name: 'Justin Ehler', username: 'justin', role: 'Owner', kind: 'Portico Account', profiles: profilesByServer['srv-lighthouse']!.length, libraries: 'All libraries', disabled: false, streams: 0, devices: 0},
    {id: 'u-maya', name: 'Maya Ehler', username: 'maya.ehler', role: 'Administrator', kind: 'Portico Account', profiles: 2, libraries: 'All libraries', disabled: false, streams: 0, devices: 0},
    {id: 'u-nora', name: 'Nora Kavanagh', username: 'nora.k', role: 'Member', kind: 'Server account', profiles: 1, libraries: 'Movies, TV Shows, Music', disabled: false, streams: 2, devices: 4},
    {id: 'u-walter', name: 'Walter Ehler', username: 'walter.e', role: 'Member', kind: 'Portico Account', profiles: 1, libraries: 'Movies, Family, Live TV', disabled: false, streams: 1, devices: 2},
    {id: 'u-old', name: 'Old Roommate', username: 'sam.b', role: 'Member', kind: 'Server account', profiles: 1, libraries: 'Movies', disabled: true, streams: 1, devices: 1},
  ];
  let trash = [
    {id: 't1', title: 'Low Tide at Kessock (duplicate copy)', files: 1, size: '4.2 GB', deleted: '3 days ago', until: 'Oct 23'},
    {id: 't2', title: 'Harbor Station · S1 E2 (old rip)', files: 1, size: '1.1 GB', deleted: '6 days ago', until: 'Oct 20'},
    {id: 't3', title: 'Glasshouse (sample)', files: 2, size: '380 MB', deleted: '28 days ago', until: 'Tomorrow'},
  ];
  let libChannels = [
    {id: 'lc-noir', name: 'Noir After Dark', on: true, desc: 'Film noir from the Movies library, around the clock.', programs: 212, candidates: 7, reach: '6 days'},
    {id: 'lc-keaton', name: 'Silent Laughs', on: true, desc: 'Keaton, Chaplin and Lloyd on shuffle.', programs: 180, candidates: 9, reach: '7 days'},
    {id: 'lc-harbor', name: 'Harbor Station Marathon', on: true, desc: 'Every episode of Harbor Station in order.', programs: 96, candidates: 1, reach: '4 days'},
    {id: 'lc-kids', name: 'Saturday Cartoons', on: false, desc: 'Family library favorites.', programs: 0, candidates: 22, reach: 'Not generated yet'},
  ];
  let sources = [
    {id: 'src-hdhr', name: 'HDHomeRun Flex 4K', kind: 'Tuner', address: '192.168.1.60', channels: 12, tuners: 4, guide: 'Refreshed 2 h ago', paused: false},
    {id: 'src-m3u', name: 'Harbor IPTV', kind: 'M3U playlist', address: 'https://iptv.harborcable.net/lighthouse.m3u', channels: 16, tuners: 2, guide: 'Refreshed 5 h ago', paused: false},
    {id: 'src-xmltv', name: 'Schedules Direct', kind: 'Guide only (XMLTV)', address: 'https://json.schedulesdirect.org/lineups/USA-OTA-98101', channels: 0, tuners: 0, guide: 'Refreshed 1 h ago', paused: false},
  ];
  const storageMounts = [
    {id: 'wd1', name: 'Nextcloud Photos', kind: 'WebDAV share', address: 'https://cloud.ehler.family/remote.php/dav', state: 'Mounted', tone: 'good' as Tone},
    {id: 'rc1', name: 'b2:portico-archive', kind: 'rclone remote', address: 'Backblaze B2 · /Films', state: 'Mounted', tone: 'good' as Tone},
    {id: 'rc2', name: 'gdrive:Vinyl', kind: 'rclone remote', address: 'Google Drive · /Vinyl Rips', state: 'Reconnecting', tone: 'warn' as Tone},
  ];
  let tasks = [
    {id: 'tk1', task: 'Scan TV Shows', state: 'Running', progress: 0.64, when: 'Started 4 min ago'},
    {id: 'tk2', task: 'Make scrubbing pictures · Movies', state: 'Running', progress: 0.21, when: 'Started 18 min ago'},
    {id: 'tk3', task: 'Refresh guide · HDHomeRun Flex 4K', state: 'Finished', progress: 1, when: '2 h ago'},
    {id: 'tk4', task: 'Analyze audio loudness · Music', state: 'Waiting for quiet hours', progress: 0, when: 'Next at 1:00 am'},
    {id: 'tk5', task: 'Back up database', state: 'Finished', progress: 1, when: 'Today, 3:00 am'},
    {id: 'tk6', task: 'Match new titles · Anime', state: 'Failed', progress: 0.9, when: 'Yesterday'},
  ];

  const logLines = (() => {
    const r = rng(7);
    // Each message comes from the part of the server that would log it.
    const msgs: [string, string, string][] = [
      ['info', 'playback', 'Playback started for a profile on Living Room Apple TV (direct play).'],
      ['info', 'scanner', 'Library scan finished: 3 new, 1 changed, 0 missing.'],
      ['warn', 'metadata', 'Metadata provider answered slowly (2.4 s); retrying later.'],
      ['info', 'live', 'Guide refresh finished for HDHomeRun Flex 4K: 12 channels, 2,904 programs.'],
      ['error', 'storage', 'Remote gdrive:Vinyl stopped responding; reconnecting.'],
      ['info', 'dvr', 'Recording started: The Evening Six on Harbor One.'],
      ['warn', 'transcoder', 'Conversion fell back to the processor: hardware encoder busy.'],
      ['info', 'auth', 'New device signed in and is waiting for approval.'],
      ['debug', 'playback', 'Segment 184 delivered in 38 ms.'],
      ['info', 'network', 'Certificate renewed; valid until Dec 24.'],
      ['error', 'scanner', 'Could not read a file in TV Shows: permission denied.'],
      ['info', 'backup', 'Backup complete (184 MB).'],
    ];
    return Array.from({length: 60}, (_, i) => {
      const [level, component, msg] = msgs[Math.floor(r() * msgs.length)]!;
      return {id: `lg${i}`, level, component, at: now - i * 97000, msg};
    });
  })();

  const lib = (id: string) => libraries.find(l => l.id === id) ?? libraries[0]!;
  const kindLabel = (k: string) => ({movie: 'Movies', tv: 'TV shows', anime: 'Anime', music: 'Music', audiobook: 'Audiobooks', photo: 'Photos', homevideo: 'Home videos'} as Record<string, string>)[k] ?? k;
  const addLibraryForm = {title: 'Add a library', lede: 'Portico scans the folder and keeps it up to date.', submit: 'Add library', fields: [
    {kind: 'text', key: 'name', label: 'Name', placeholder: 'Movies'},
    {kind: 'choice', key: 'kind', label: 'Kind', help: 'Decides how titles are matched and presented.', value: 'movie', options: [{value: 'movie', label: 'Movies', help: 'Films, one per file or folder'}, {value: 'tv', label: 'TV shows', help: 'Series organized by season'}, {value: 'anime', label: 'Anime', help: 'Series matched with AniList naming and absolute episode numbers'}, {value: 'music', label: 'Music', help: 'Artists and albums'}, {value: 'audiobook', label: 'Audiobooks', help: 'Books with chapters'}, {value: 'photo', label: 'Photos'}, {value: 'homevideo', label: 'Home videos'}]},
    {kind: 'folder', key: 'folder', label: 'Folder on the server', help: 'A folder the server can read. You can add more folders afterwards.', placeholder: '/media/Movies'},
    {kind: 'choice', key: 'source', label: 'Metadata', value: 'online', options: [{value: 'online', label: 'Portico online metadata', help: 'Looks up titles, artwork, cast and ratings online.'}, {value: 'local', label: 'Local metadata only', help: 'Uses only file names, embedded tags and sidecar files.'}]},
  ]} as AdminAction['form'];

  function page(section: string, param?: string): AdminPage {
    const meta = adminSections.find(s => s.id === section) ?? adminSections[0]!;
    const base = {title: meta.label, lede: meta.lede};
    switch (meta.id) {
      case 'overview': return {...base, blocks: [
        {type: 'stats', items: [
          {label: 'Watching now', value: String(sessions.length), sub: `${sessions.filter(s => s.delivery.startsWith('Convert')).length} converting`, tone: 'accent'},
          {label: 'Processor', value: '23%', sub: 'Apple M2 Pro · 10 cores', spark: spark('cpu')},
          {label: 'Memory', value: '6.1 GB', sub: 'of 16 GB', spark: spark('mem', 24, 40, 6)},
          {label: 'Sent to viewers', value: '31 Mb/s', sub: 'Upload 40 Mb/s', spark: spark('net', 24, 50, 30)},
          {label: 'Media storage', value: '7.9 TB', sub: '92% used · 690 GB free', tone: 'warn'},
          {label: 'Libraries', value: '8', sub: `${catalog.movies.length} movies · ${catalog.shows.length} shows`},
        ]},
        alerts.length ? {type: 'list', title: 'Needs attention', items: alerts.map(a => ({id: a.id, title: a.title, subtitle: a.body, icon: 'alert', badge: {label: a.tone === 'bad' ? 'Urgent' : 'Notice', tone: a.tone}, actions: [act('ack', 'Acknowledge', {target: a.id, tone: 'secondary'}), ...(a.id === 'al2' ? [act('go', 'Review devices', {navigate: {section: 'access'}, tone: 'ghost'})] : a.id === 'al3' ? [act('go', 'Review files', {navigate: {section: 'libraries', param: 'lib-tv'}, tone: 'ghost'})] : [])]}))} : {type: 'notice', tone: 'good', title: 'All clear', body: 'Nothing needs your attention right now.'},
        {type: 'list', title: 'Watching now', items: sessions.map(sessionItem), actions: [act('go', 'Open Now Playing', {navigate: {section: 'now-playing'}})]},
        {type: 'chart', title: 'Viewing', lede: 'Hours watched per day, last 14 days.', series: [{label: 'Hours', values: spark('viewing', 14, 9, 8), tone: 'accent'}], unit: 'h', labels: Array.from({length: 14}, (_, i) => new Date(now - (13 - i) * 86400000).toLocaleDateString('en-US', {weekday: 'narrow'}))},
        {type: 'list', title: 'Recent playback', items: catalog.movies.slice(40, 46).map((m, i) => ({id: `rp${i}`, title: m.title, subtitle: `${['Maya', 'Justin', 'Nora', 'Theo', 'Walter', 'Maya'][i]} · ${['Apple TV', 'iPhone', 'Web', 'Apple TV', 'Android TV', 'iPad'][i]}`, meta: ago((i + 1) * 3 * 3600000), image: posterFor(m.id)}))},
        {type: 'actions', title: 'Message everyone', lede: 'Appears in each person’s notifications. It is not an email.', actions: [act('broadcast', 'Message everyone', {icon: 'mail', form: {title: 'Message everyone', lede: 'Appears in each person’s notifications. It is not an email.', submit: 'Send', fields: [{kind: 'text', key: 'title', label: 'Title', placeholder: 'The server will be down on Saturday morning'}, {kind: 'text', key: 'message', label: 'Message', multiline: true}, {kind: 'choice', key: 'audience', label: 'Who sees it', value: 'everyone', options: [{value: 'everyone', label: 'Everyone'}, {value: 'admins', label: 'Administrators only'}]}, {kind: 'choice', key: 'importance', label: 'Importance', value: 'info', options: [{value: 'info', label: 'For information'}, {value: 'notice', label: 'Worth noticing'}, {value: 'urgent', label: 'Urgent'}]}]}})]},
      ]};
      case 'now-playing': return {...base, blocks: [
        {type: 'stats', items: [{label: 'Streams', value: String(sessions.length), tone: 'accent'}, {label: 'Converting', value: String(sessions.filter(s => s.delivery.startsWith('Convert')).length)}, {label: 'Bandwidth', value: `${sessions.reduce((n, s) => n + s.rate, 0).toFixed(1)} Mb/s`}, {label: 'Away from home', value: String(sessions.filter(s => s.location === 'Remote').length)}]},
        {type: 'list', title: 'Active playback sessions', items: sessions.map(s => ({...sessionItem(s), detail: [{label: 'Delivery', value: s.delivery}, {label: 'Quality', value: s.quality}, {label: 'Where', value: s.location}, {label: 'Started', value: clock(s.started)}, {label: 'Bitrate', value: `${s.rate} Mb/s`}], actions: [act('stop', 'Stop', {target: s.id, tone: 'danger', form: {title: `Stop playback of “${s.title}”?`, lede: `Playback stops on ${s.viewer}’s device, and they see that the server owner stopped it.`, submit: 'Stop playback', fields: [{kind: 'text', key: 'message', label: 'Message to the viewer', help: 'Shown with the notice. Up to 500 characters.', placeholder: 'For example: The server is restarting in five minutes.', multiline: true}]}})]})), empty: {title: 'Nobody is watching right now', body: 'Playback on any device appears here as it starts.'}},
      ]};
      case 'activity': return {...base, blocks: [
        {type: 'list', title: 'Playback now', items: sessions.map(sessionItem)},
        {type: 'table', title: 'Background tasks', lede: 'Scans, metadata refreshes and maintenance appear here as they run.', columns: ['Task', 'State', 'Progress', 'When'], rows: tasks.map(t => [t.task, t.state, t.progress ? `${Math.round(t.progress * 100)}%` : '—', t.when])},
        {type: 'list', title: 'Recent', items: tasks.map(t => ({id: t.id, title: t.task, subtitle: t.state, meta: t.when, progress: t.state === 'Running' ? t.progress : undefined, icon: t.state === 'Failed' ? 'alert' : t.state === 'Running' ? 'refresh' : 'check', badge: t.state === 'Failed' ? {label: 'Failed', tone: 'bad'} : t.state === 'Running' ? {label: 'Running', tone: 'accent'} : undefined, actions: t.state === 'Running' ? [act('cancelTask', 'Cancel', {target: t.id, tone: 'ghost'})] : t.state === 'Failed' ? [act('retryTask', 'Try again', {target: t.id, tone: 'secondary'})] : []}))},
      ]};
      case 'logs': {
        const level = v<string>('logs.level');
        const order = ['debug', 'info', 'warn', 'error'];
        const lines = logLines.filter(l => order.indexOf(l.level) >= order.indexOf(level));
        return {...base, blocks: [
          {type: 'list', title: 'Messages', lede: 'What the server has been saying. Nothing here includes file paths, passwords or tokens.', filters: {key: 'logs.level', value: level, options: [{value: 'debug', label: 'Everything'}, {value: 'info', label: 'Info and above'}, {value: 'warn', label: 'Warnings and above'}, {value: 'error', label: 'Errors'}]}, items: lines.slice(0, 30).map(l => ({id: l.id, title: l.msg, subtitle: `${l.component} · ${clock(l.at)}`, icon: l.level === 'error' ? 'alert' : l.level === 'warn' ? 'alert' : 'info', badge: {label: l.level.toUpperCase(), tone: l.level === 'error' ? 'bad' : l.level === 'warn' ? 'warn' : 'neutral'}})), empty: {title: 'No messages match', body: 'Loosen the level filter to see more.'}},
          {type: 'settings', title: 'Temporary capture', lede: 'Collect verbose records for one component for a short time. It switches itself off.', fields: [setting('choice', 'logs.component', 'Component', undefined, {options: ['playback', 'scanner', 'metadata', 'network', 'live', 'transcoder', 'auth'].map(x => ({value: x, label: x[0]!.toUpperCase() + x.slice(1)}))}), setting('choice', 'logs.duration', 'Duration', 'Verbose records use more storage while the capture runs.', {options: ['5', '15', '30', '60'].map(x => ({value: x, label: `${x} min`}))})]},
          {type: 'actions', actions: [act('capture', 'Start capture', {icon: 'record', tone: 'secondary'})]},
          {type: 'actions', title: 'Support export', lede: 'A redacted bundle you can inspect before sending. Media, credentials and raw databases are never included.', actions: [act('export', 'Prepare export', {icon: 'download', tone: 'secondary', form: {title: 'Support export', submit: 'Prepare', fields: [{kind: 'choice', key: 'window', label: 'Time window', value: '24h', options: [{value: '1h', label: 'Last hour'}, {value: '24h', label: 'Last 24 hours'}, {value: '7d', label: 'Last 7 days'}]}, {kind: 'multi', key: 'include', label: 'Include', value: ['server', 'snapshot'], options: [{value: 'server', label: 'Server records'}, {value: 'client', label: 'Client uploads'}, {value: 'audit', label: 'Audit'}, {value: 'snapshot', label: 'Health snapshot'}]}]}})]},
          {type: 'settings', title: 'Retention', lede: 'How long records are kept. Shortening a period prunes soon after saving.', fields: [setting('number', 'maintenance.keepLogs', 'Server records', 'Diagnostic and log history.', {unit: 'days', min: 1, max: 90}), setting('number', 'maintenance.keepNotifications', 'Notifications', 'Owner alerts and viewer notices.', {unit: 'days', min: 7, max: 365}), setting('number', 'maintenance.keepTasks', 'Task history', 'Finished scans, refreshes and maintenance runs.', {unit: 'days', min: 1, max: 180})]},
        ]};
      }
      case 'diagnostics': return {...base, blocks: [
        {type: 'list', title: 'Server capabilities', lede: 'What this server can do right now, and why not when it can’t.', items: [
          {id: 'c1', title: 'Live TV', subtitle: 'Tuners and playlists can be watched.', icon: 'live', badge: {label: 'Available', tone: 'good'}},
          {id: 'c2', title: 'Recording', subtitle: 'The recording folder is writable.', icon: 'recordings', badge: {label: 'Available', tone: 'good'}},
          {id: 'c3', title: 'Prepared media', subtitle: 'Downloads can be prepared for devices.', icon: 'download', badge: {label: 'Available', tone: 'good'}},
          {id: 'c4', title: 'Hardware encoding', subtitle: 'VideoToolbox on Apple M2 Pro.', icon: 'bolt', badge: {label: 'Available', tone: 'good'}},
          {id: 'c5', title: 'Decoder isolation', subtitle: 'This host can’t isolate the video tools, so conversions run with baseline protections.', icon: 'shield', badge: {label: 'Unavailable', tone: 'warn'}},
          {id: 'c6', title: 'Dolby Vision conversion', subtitle: 'The server hasn’t checked this yet. It does when the feature is first used.', icon: 'quality', badge: {label: 'Not checked', tone: 'neutral'}},
        ]},
        {type: 'facts', title: 'Health checks', rows: [{label: 'Database integrity', value: 'Passed · checked 3 h ago', tone: 'good'}, {label: 'Data folder permissions', value: 'Other accounts can read Portico data', tone: 'warn'}, {label: 'Reachable from outside', value: 'Yes · 42 ms from the relay check', tone: 'good'}, {label: 'Clock', value: 'In sync (±0.2 s)', tone: 'good'}, {label: 'FFmpeg', value: '7.1 · ffprobe 7.1', tone: 'good'}, {label: 'Free space (data)', value: '212 GB', tone: 'good'}]},
        {type: 'actions', actions: [act('runChecks', 'Run checks again', {icon: 'refresh', tone: 'secondary'}), act('fixPerms', 'Fix permissions', {icon: 'shield', tone: 'secondary', confirm: {title: 'Fix permissions?', body: 'Portico makes its files readable only by the account it runs as.', action: 'Fix permissions'}}), act('speed', 'Test transcoding speed', {icon: 'gauge', tone: 'ghost'})]},
      ]};
      case 'feedback': {
        const status = v<string>('feedback.status');
        const list = reports.filter(r => status === 'all' || r.status === status);
        return {...base, blocks: [{type: 'list', filters: {key: 'feedback.status', value: status, options: [{value: 'open', label: 'Open'}, {value: 'answered', label: 'Answered'}, {value: 'closed', label: 'Closed'}, {value: 'all', label: 'All'}]}, items: list.map(r => ({id: r.id, title: `${r.category}${r.about ? ` · ${r.about}` : ''}`, subtitle: r.message, meta: `${r.who} · ${ago(now - r.when)}`, icon: 'flag', badge: {label: r.status[0]!.toUpperCase() + r.status.slice(1), tone: r.status === 'open' ? 'accent' : 'neutral'}, actions: [act('reply', 'Update report', {target: r.id, tone: 'secondary', form: {title: 'Update report', lede: `Reported ${ago(now - r.when)} by ${r.who}`, submit: 'Save', fields: [{kind: 'choice', key: 'status', label: 'Status', value: r.status, options: [{value: 'open', label: 'Open'}, {value: 'answered', label: 'Answered'}, {value: 'closed', label: 'Closed'}]}, {kind: 'text', key: 'reply', label: 'Reply to the viewer', help: 'The viewer sees this in their notifications.', multiline: true}]}})]})), empty: {title: 'No reports', body: 'When a viewer reports a problem from their app it appears here.'}}]};
      }
      case 'alerts': return {...base, blocks: [
        {type: 'settings', title: 'Storage', fields: [setting('number', 'alerts.storageWarning', 'Warn when free space falls below', 'Percent of a drive that is still free', {unit: '%', min: 1, max: 50}), setting('number', 'alerts.storageCritical', 'Warn urgently below', 'Recordings and conversions may start failing at this point', {unit: '%', min: 1, max: 20})]},
        {type: 'settings', title: 'Certificates', fields: [setting('number', 'alerts.certificateWarning', 'Warn before a certificate ends', 'Days ahead. Certificates renew by themselves; this only fires if renewal is stuck.', {unit: 'days', min: 1, max: 60})]},
        {type: 'list', title: 'Current alerts', items: alerts.map(a => ({id: a.id, title: a.title, subtitle: a.body, icon: 'alert', badge: {label: a.tone === 'bad' ? 'Urgent' : 'Notice', tone: a.tone}, actions: [act('ack', 'Acknowledge', {target: a.id, tone: 'secondary'})]})), empty: {title: 'Nothing needs your attention', body: 'New alerts appear here and in your notifications.'}},
      ]};
      case 'libraries': {
        if (param) {
          const l = lib(param);
          return {title: l.name, lede: `${kindLabel(l.kind)} · ${l.folders.length} folder${l.folders.length === 1 ? '' : 's'}`, back: {section: 'libraries', label: 'Libraries'}, blocks: [
            l.attention ? {type: 'notice', tone: 'warn', title: `${l.attention} files need attention`, body: 'Portico couldn’t tell which show, season or episode these files are. Rename them, then scan again.', actions: [act('review', 'Review', {target: l.id, tone: 'secondary', form: {title: 'Files that need attention', submit: 'Done', fields: [{kind: 'choice', key: 'file', label: 'Files', value: 'a', options: Array.from({length: Math.min(8, l.attention)}, (_, i) => ({value: String(i), label: `/media/TV/Unsorted/episode_${i + 101}.mkv`, help: 'No show, season or episode number found'}))}]}})]} : {type: 'notice', tone: 'good', title: 'All files matched', body: 'No files need attention now.'},
            {type: 'facts', title: 'Scan', lede: 'Scans find new, changed and missing files and keep metadata current.', rows: [{label: 'Last scan', value: l.scanning ? 'Scanning now…' : l.lastScan}, {label: 'Titles', value: String(l.kind === 'movie' ? catalog.movies.filter(m => m.library === l.id).length : l.kind === 'tv' || l.kind === 'anime' ? catalog.shows.filter(s => s.library === l.id).length : 120)}, {label: 'Metadata', value: l.source === 'online' ? 'Portico online metadata (TMDB, TVDB)' : 'Local metadata only'}]},
            {type: 'actions', actions: [act('scan', l.scanning ? 'Cancel scan' : 'Scan now', {target: l.id, icon: 'refresh', tone: 'primary'}), act('scanAll', 'Scan all files', {target: l.id, tone: 'secondary', confirm: {title: 'Scan all files?', body: 'Every file is read again, changed or not. Large libraries can take a while.', action: 'Scan all files'}}), act('go', 'Library settings', {navigate: {section: 'library-settings', param: l.id}, tone: 'ghost', icon: 'sliders'})]},
            {type: 'list', title: 'Folders', lede: 'Where this library’s files live. Each folder is checked and scanned on its own.', items: l.folders.map((f, i) => ({id: `${l.id}-f${i}`, title: f.split('/').pop()!, subtitle: f, icon: 'folder', meta: i === 1 ? 'Network share' : 'Local disk', badge: {label: 'Available', tone: 'good'}, actions: [act('scanFolder', 'Scan this folder', {target: f, tone: 'ghost'}), act('removeFolder', 'Remove', {target: f, tone: 'danger', confirm: {title: `Remove folder “${f.split('/').pop()}”?`, body: 'Titles found only in this folder become unavailable. Files on disk are not deleted.', action: 'Remove folder'}})]})), actions: [act('addFolder', 'Add folder', {target: l.id, icon: 'plus', form: {title: 'Add folder', submit: 'Add folder', fields: [{kind: 'folder', key: 'path', label: 'Folder path', placeholder: '/media/More Movies'}, {kind: 'choice', key: 'location', label: 'Location', help: 'Network folders are checked more patiently before files are marked missing.', value: 'local', options: [{value: 'local', label: 'Local disk'}, {value: 'network', label: 'Network share'}]}, {kind: 'toggle', key: 'symlinks', label: 'Follow symbolic links', value: false}]}})]},
            {type: 'actions', title: 'Manage', actions: [act('rename', 'Rename', {target: l.id, tone: 'secondary', form: {title: 'Rename library', submit: 'Save', fields: [{kind: 'text', key: 'name', label: 'Name', value: l.name}]}}), act('deleteLibrary', 'Delete library', {target: l.id, tone: 'danger', confirm: {title: `Delete ${l.name}?`, body: 'The library and its watch history are removed from Portico. Your files are not deleted.', action: 'Delete library', typeToConfirm: l.name}})]},
          ]};
        }
        return {...base, actions: [act('addLibrary', 'Add library', {icon: 'plus', tone: 'primary', form: addLibraryForm})], blocks: [
          {type: 'list', items: libraries.map(l => ({id: l.id, title: l.name, subtitle: `${kindLabel(l.kind)} · ${l.path}`, meta: l.scanning ? 'Scanning…' : `Scanned ${l.lastScan}`, icon: ({movie: 'movie', tv: 'tv', anime: 'anime', music: 'music', audiobook: 'audiobook', photo: 'photo', homevideo: 'homeVideo'} as Record<string, string>)[l.kind], badge: l.attention ? {label: `${l.attention} to fix`, tone: 'warn'} : l.scanning ? {label: 'Scanning', tone: 'accent'} : undefined, open: {section: 'libraries', param: l.id}}))},
        ]};
      }
      case 'library-settings': {
        if (!param) return {...base, blocks: [{type: 'list', title: 'Choose a library', items: libraries.map(l => ({id: l.id, title: `${l.name} settings`, subtitle: kindLabel(l.kind), icon: 'sliders', open: {section: 'library-settings', param: l.id}}))}]};
        const l = lib(param);
        const k = (s: string) => `lib.${l.id}.${s}`;
        const val = (s: string, d: unknown) => (values.has(k(s)) ? values.get(k(s)) : d);
        return {title: `${l.name} settings`, lede: 'Matching, scrubbing pictures, scanning and deleting.', back: {section: 'library-settings', label: 'Library settings'}, blocks: [
          {type: 'settings', title: 'Matching', lede: 'Where Portico looks up titles, artwork and descriptions for this library.', fields: [
            {kind: 'choice', key: k('source'), label: 'Source', value: val('source', l.source) as string, options: [{value: 'online', label: 'Portico online metadata', help: 'Looks up titles, artwork, cast and ratings online. Your own files and NFO details are used first.'}, {value: 'local', label: 'Local metadata only', help: 'Uses only file and folder names, embedded tags, NFO and other sidecar files.'}]},
            {kind: 'choice', key: k('language'), label: 'Metadata language', help: 'Titles, summaries and artwork text are fetched in this language when available.', value: val('language', 'en') as string, options: [{value: 'en', label: 'English'}, {value: 'fr', label: 'French'}, {value: 'de', label: 'German'}, {value: 'es', label: 'Spanish'}, {value: 'ja', label: 'Japanese'}]},
            {kind: 'text', key: k('country'), label: 'Country or region', value: val('country', 'US') as string, placeholder: 'e.g. US'},
            {kind: 'text', key: k('apiKey'), label: 'Your own key (optional)', value: '', placeholder: 'Your API key', secure: true},
          ]},
          {type: 'settings', title: 'Scrubbing and chapters', lede: 'Pictures made for the seek bar and the chapter list.', fields: [
            {kind: 'number', key: k('every'), label: 'A picture every', help: 'Seconds between seek-bar pictures. Smaller is smoother and uses more disk.', value: val('every', 10) as number, unit: 's', min: 2, max: 60},
            {kind: 'number', key: k('width'), label: 'Picture width', help: 'In pixels.', value: val('width', 320) as number, unit: 'px', min: 160, max: 640, step: 40},
            {kind: 'toggle', key: k('chapters'), label: 'Chapter pictures', value: val('chapters', true) as boolean},
          ]},
          {type: 'settings', title: 'Scanning', fields: [
            {kind: 'choice', key: k('interval'), label: 'Rescan interval', value: val('interval', 'hourly') as string, options: [{value: '15m', label: 'Every 15 minutes'}, {value: 'hourly', label: 'Every hour'}, {value: '6h', label: 'Every 6 hours'}, {value: 'daily', label: 'Daily'}, {value: 'manual', label: 'Only when I ask'}]}, {kind: 'choice', key: k('depth'), label: 'Scan depth', help: 'How many folders down Portico looks for files. Deeper finds more but takes longer.', value: val('depth', 'all') as string, options: [{value: '2', label: 'Two levels'}, {value: '4', label: 'Four levels'}, {value: 'all', label: 'Every folder'}]},
            {kind: 'choice', key: k('missing'), label: 'Mark missing files after', help: 'Gives unplugged drives and offline shares time to come back.', value: val('missing', 'week') as string, options: [{value: 'now', label: 'At once'}, {value: 'day', label: '1 day'}, {value: 'week', label: '1 week'}]},
            ...(l.kind === 'music' ? [{kind: 'toggle', key: k('lyrics'), label: 'Fetch missing lyrics', value: val('lyrics', true) as boolean} as AdminField] : []),
          ]},
          {type: 'settings', title: 'Deleting', lede: 'Off by default: nothing in Portico can remove a file from this library until you allow it here.', fields: [
            {kind: 'toggle', key: k('allowDelete'), label: 'Allow deleting files', value: val('allowDelete', l.settings.allowDelete) as boolean},
            {kind: 'number', key: k('keepDays'), label: 'Keep deleted files for', help: 'Days before a deleted title is removed for good. Zero removes at once.', value: val('keepDays', 30) as number, unit: 'days', min: 0, max: 90},
          ]},
        ]};
      }
      case 'metadata': return {...base, blocks: [
        {type: 'list', title: 'Metadata source', lede: 'Per library. You can change this later in the library’s settings.', items: libraries.map(l => {
          const src = values.get(`lib.${l.id}.source`) ?? l.source;
          // Home Videos was added without the OK for online lookups, so it waits.
          const waiting = l.id === 'lib-homevideos' && !values.has(`lib.${l.id}.source`);
          return {id: l.id, title: l.name, subtitle: waiting ? 'Not matching yet: online lookups are waiting for your OK. Choose Allow online lookups in the library’s settings to start.' : src === 'online' ? 'Titles, artwork and details are looked up online (TMDB, TVDB, MusicBrainz).' : 'Off for this library. Titles come only from the files and folders.', icon: 'tag', badge: waiting ? {label: 'Waiting', tone: 'warn' as Tone} : src === 'online' ? {label: 'On', tone: 'good' as Tone} : {label: 'Off', tone: 'neutral' as Tone}, open: {section: 'library-settings', param: l.id}};
        })},
        {type: 'list', title: 'Needs your choice', lede: 'Titles Portico couldn’t match with confidence.', items: catalog.movies.slice(120, 125).map((m, i) => ({id: `mm${i}`, title: m.title, subtitle: ['Needs your choice · 3 close matches', 'Needs an episode order', 'Not matched', 'Provider unavailable', 'Needs your choice · 2 close matches'][i], image: posterFor(m.id), badge: {label: ['Needs your choice', 'Needs an order', 'Not matched', 'Unavailable', 'Needs your choice'][i]!, tone: i === 3 ? 'bad' : 'warn'}, actions: [act('fixMatch', 'Fix match', {target: m.id, tone: 'secondary', form: {title: `Fix match for “${m.title}”`, submit: 'Use this match', fields: [{kind: 'text', key: 'query', label: 'Search the provider', help: 'Title, or a provider id', value: m.title}, {kind: 'choice', key: 'candidate', label: 'Candidates', value: 'a', options: [{value: 'a', label: `${m.title} (${m.year})`, help: 'TMDB · 98% match'}, {value: 'b', label: `${m.title} (${m.year - 3})`, help: 'TMDB · 71% match'}, {value: 'c', label: `The ${m.title} Story (${m.year + 2})`, help: 'TVDB · 44% match'}]}]}})]}))},
        {type: 'facts', title: 'Providers', rows: [{label: 'TMDB', value: 'Healthy · 212 ms', tone: 'good'}, {label: 'TVDB', value: 'Healthy · 340 ms', tone: 'good'}, {label: 'AniList', value: 'Healthy · 180 ms', tone: 'good'}, {label: 'MusicBrainz', value: 'Slow · 2.4 s', tone: 'warn'}, {label: 'Portico Title Dataset', value: 'Version 2026.09 · CC BY-SA 4.0', tone: 'neutral'}]},
      ]};
      case 'storage': return {...base, actions: [act('addSource', 'Add remote source', {icon: 'plus', tone: 'primary', form: {title: 'Add remote source', submit: 'Add', fields: [{kind: 'choice', key: 'kind', label: 'Type', value: 'webdav', options: [{value: 'webdav', label: 'WebDAV share', help: 'Nextcloud, ownCloud…'}, {value: 'rclone', label: 'rclone remote', help: 'S3, Drive, SMB via rclone'}]}, {kind: 'text', key: 'name', label: 'Name'}, {kind: 'text', key: 'address', label: 'Address', placeholder: 'https://cloud.example.com/remote.php/dav'}, {kind: 'text', key: 'user', label: 'Username'}, {kind: 'text', key: 'password', label: 'Password', secure: true, help: 'The password is stored on the server and never shown again.'}, {kind: 'toggle', key: 'insecure', label: 'Allow an unencrypted (http) address on the local network', help: 'Only for shares that never leave your home network.', value: false}]}})], blocks: [
        {type: 'meters', title: 'Media volumes', items: [{label: 'Media (/media)', value: 0.92, detail: '7.9 TB of 8.6 TB · 690 GB free', tone: 'warn'}, {label: 'NAS (/mnt/nas)', value: 0.61, detail: '11 TB of 18 TB', tone: 'good'}, {label: 'Recordings (/var/lib/portico/dvr)', value: 0.26, detail: '412 GB of 1.6 TB', tone: 'good'}]},
        {type: 'list', title: 'Remote storage', lede: 'Library folders on remote storage are added under Libraries once the source is mounted and healthy.', items: storageMounts.map(s => ({id: s.id, title: s.name, subtitle: `${s.kind} · ${s.address}`, icon: s.kind.startsWith('WebDAV') ? 'cloud' : 'link', badge: {label: s.state, tone: s.tone}, actions: [act('unmount', s.state === 'Mounted' ? 'Unmount' : 'Mount', {target: s.id, tone: 'secondary'}), act('restartMount', 'Restart', {target: s.id, tone: 'ghost'}), act('removeMount', 'Remove', {target: s.id, tone: 'danger', confirm: {title: `Remove “${s.name}”?`, body: 'The mount is stopped and its saved configuration removed. Libraries using it lose access to those files; nothing on the remote is deleted.', action: 'Remove'}})]})), empty: {title: 'No remote storage yet', body: 'Attach a WebDAV share or an rclone remote to use media stored elsewhere.'}},
        {type: 'facts', title: 'rclone', rows: [{label: 'rclone binary on the server', value: '/usr/local/bin/rclone (1.68.2)'}, {label: 'Remotes', value: '2 of 4 allowed'}]},
      ]};
      case 'live-tv': return {...base, actions: [act('addTvSource', 'Add source', {icon: 'plus', tone: 'primary', form: {title: 'Add a live TV source', lede: 'Portico checks the source first and shows what it found before saving.', submit: 'Check source', fields: [{kind: 'choice', key: 'kind', label: 'Source', value: 'remote', options: [{value: 'remote', label: 'Playlist URL or tuner', help: 'M3U, Xtream, HDHomeRun'}, {value: 'discover', label: 'Find tuners on the network', help: 'HDHomeRun'}, {value: 'upload', label: 'Upload playlist files', help: 'M3U + XMLTV'}]}, {kind: 'text', key: 'url', label: 'Playlist URL or tuner address', placeholder: 'http://192.168.1.60'}, {kind: 'text', key: 'guide', label: 'Guide (XMLTV) URL'}, {kind: 'choice', key: 'who', label: 'Who can watch', value: 'everyone', options: [{value: 'everyone', label: 'Everyone on this server'}, {value: 'admins', label: 'Administrators'}]}, {kind: 'choice', key: 'refresh', label: 'Refresh the guide', value: '12h', options: [{value: '6h', label: 'Every 6 hours'}, {value: '12h', label: 'Every 12 hours'}, {value: '24h', label: 'Daily'}]}]}})], blocks: [
        {type: 'list', title: 'Sources', items: sources.map(s => ({id: s.id, title: s.name, subtitle: `${s.kind} · ${s.address}`, meta: `${s.channels} channels · ${s.tuners ? `${s.tuners} tuners` : 'guide only'} · ${s.guide}`, icon: s.kind === 'Tuner' ? 'tuner' : 'live', badge: s.paused ? {label: 'Paused', tone: 'neutral'} : {label: 'Healthy', tone: 'good'}, actions: [act('refreshGuide', 'Refresh guide now', {target: s.id, tone: 'secondary'}), act('channelMap', 'Channels and numbers', {target: s.id, tone: 'ghost', form: {title: `Channels from ${s.name}`, lede: 'Give a channel its own number, hide it, or match it to a guide listing.', submit: 'Save changes', fields: channelsRaw.filter(c => c.source === s.id).slice(0, 8).map(c => ({kind: 'text', key: `num.${c.id}`, label: c.name, value: c.number} as AdminField))}}), act('pauseSource', s.paused ? 'Enable' : 'Pause', {target: s.id, tone: 'ghost'}), act('removeSource', 'Remove source', {target: s.id, tone: 'danger', confirm: {title: `Remove “${s.name}”?`, body: 'Its channels leave the guide and any saved credentials are deleted. Recordings already made are kept.', action: 'Remove source'}})]}))},
        {type: 'settings', title: 'Live TV defaults', lede: 'How the server tunes and keeps a stream going.', fields: [setting('number', 'dvr.buffer', 'Buffer', 'Seconds held before playing. More rides out a shaky source; less is closer to live.', {unit: 's', min: 0, max: 30}), setting('number', 'dvr.retry', 'Keep retrying for', 'Seconds to keep trying a source that drops before giving up.', {unit: 's', min: 5, max: 120}), setting('number', 'dvr.guideDays', 'Guide days', 'How far ahead the guide is kept.', {unit: 'days', min: 1, max: 21}), setting('text', 'dvr.userAgent', 'Identify as', 'Some providers only answer a particular player. Leave empty for Portico’s own.'), setting('toggle', 'dvr.logos', 'Fetch channel logos'), setting('toggle', 'dvr.discovery', 'Look for tuners on the network', 'Finds HDHomeRun devices by themselves.')]},
        {type: 'list', title: 'Channels', lede: `${channelsRaw.filter(c => !c.library).length} channels from ${sources.length - 1} sources.`, items: channelsRaw.filter(c => !c.library).slice(0, 10).map(c => ({id: c.id, title: `${c.number}  ${c.name}`, subtitle: `${c.group} · ${c.source === 'src-hdhr' ? 'HDHomeRun Flex 4K' : 'Harbor IPTV'}`, image: logoFor(c.id), badge: c.hd ? {label: 'HD', tone: 'neutral'} : undefined}))},
      ]};
      case 'dvr': {
        const tunersFrom = now - 3600000;
        return {...base, blocks: [
          {type: 'settings', title: 'Recording defaults', lede: 'What a new recording does unless you say otherwise.', fields: [setting('number', 'dvr.startEarly', 'Start early', 'Minutes before the listed start.', {unit: 'min', min: 0, max: 30}), setting('number', 'dvr.finishLate', 'Finish late', 'Minutes after the listed end. Live sport often overruns.', {unit: 'min', min: 0, max: 120}), setting('choice', 'dvr.keep', 'Keep', undefined, {options: [{value: 'all', label: 'Keep every episode'}, {value: 'newest', label: 'The newest'}, {value: 'days', label: 'For a number of days'}]}), setting('number', 'dvr.keepCount', 'How many', undefined, {min: 1, max: 50}), setting('choice', 'dvr.after', 'After recording', undefined, {options: [{value: 'none', label: 'Leave as recorded', help: 'Kept exactly as broadcast.'}, {value: 'remux', label: 'Repackage', help: 'Repackaged so it seeks and plays everywhere; picture and sound untouched.'}, {value: 'convert', label: 'Convert to a smaller file', help: 'Converted to a smaller file. This takes server time.'}]}), setting('toggle', 'dvr.removeOriginal', 'Remove the original afterwards'), setting('text', 'dvr.folderShows', 'Folder for shows', 'Pieces you can use: {show} {season} {episode} {title} {year}'), setting('text', 'dvr.folderMovies', 'Folder for movies')]},
          {type: 'timeline', title: 'Tuners', lede: 'In use now and over the next 6 hours.', from: tunersFrom, to: tunersFrom + 7 * 3600000, lanes: [
            {label: 'HDHomeRun · Tuner 1', spans: [{start: now - 30 * MIN, end: now + 30 * MIN, label: 'The Evening Six (recording)', tone: 'bad'}, {start: now + 3 * 3600000, end: now + 6 * 3600000, label: 'Pro Basketball Tonight', tone: 'accent'}]},
            {label: 'HDHomeRun · Tuner 2', spans: [{start: now - 12 * MIN, end: now + 70 * MIN, label: 'Walter watching Harbor One', tone: 'good'}, {start: now + 3 * 3600000, end: now + 5 * 3600000, label: 'Motor Racing: Qualifying', tone: 'accent'}]},
            {label: 'HDHomeRun · Tuner 3', spans: [{start: now + 3 * 3600000, end: now + 5 * 3600000, label: 'Orbital', tone: 'accent'}]},
            {label: 'HDHomeRun · Tuner 4', spans: [{start: now + 3 * 3600000, end: now + 4.5 * 3600000, label: 'Conflict', tone: 'warn'}]},
          ]},
          {type: 'list', title: 'Who can record', lede: 'Choose which profiles may schedule recordings. A setting on a profile takes priority.', items: profilesByServer['srv-lighthouse']!.map(p => ({id: p.id, title: p.name, subtitle: p.kids ? 'Kids profile' : p.role === 'owner' ? 'Owner' : p.role === 'admin' ? 'Administrator' : 'Member', icon: 'user', badge: p.limits?.dvr === false ? {label: 'Not allowed', tone: 'neutral'} : {label: 'Allowed', tone: 'good'}, actions: [act('toggleRecord', p.limits?.dvr === false ? 'Allow recording' : 'Stop allowing', {target: p.id, tone: 'secondary'})]})), actions: [act('addRecorder', 'Add a profile', {icon: 'plus', form: {title: 'Recording profile', submit: 'Save permission', fields: [{kind: 'choice', key: 'profile', label: 'Choose an account and profile', value: 'walter', options: [{value: 'walter', label: 'Walter Ehler · Walter'}, {value: 'nora', label: 'Nora Kavanagh · Nora'}]}, {kind: 'toggle', key: 'allow', label: 'Allow recording', value: true}, {kind: 'toggle', key: 'inherit', label: 'Let other profiles on this account inherit', help: 'Only profiles with current server access can inherit. An explicit denial still wins.', value: false}]}})]},
          {type: 'list', title: 'Recording rules', lede: 'Record a series from the guide for a permitted profile.', items: ['The Evening Six', 'Midnight Kitchen', 'Ocean Giants', 'Late Show Live'].map((t, i) => ({id: `rr${i}`, title: t, subtitle: `For ${['Justin', 'Maya', 'Justin', 'Maya'][i]} · ${i === 1 ? 'any channel' : 'this channel'} · ${i === 2 ? 'all episodes' : 'new episodes only'}`, icon: 'recordings', badge: i === 3 ? {label: 'Paused', tone: 'neutral'} : {label: 'On', tone: 'good'}, actions: [act('editRule', 'Edit', {target: `rr${i}`, tone: 'ghost'}), act('pauseRule', i === 3 ? 'Resume' : 'Pause', {target: `rr${i}`, tone: 'ghost'})]})), actions: [act('newRule', 'New rule', {icon: 'plus', form: {title: 'New recording rule', submit: 'Save rule', fields: [{kind: 'text', key: 'name', label: 'Rule name'}, {kind: 'choice', key: 'for', label: 'Record for', value: 'justin', options: [{value: 'justin', label: 'Justin'}, {value: 'maya', label: 'Maya'}]}, {kind: 'choice', key: 'series', label: 'Series from the guide', value: 'a', options: [{value: 'a', label: 'The Evening Six · Harbor One · 6:00 pm'}, {value: 'b', label: 'Ocean Giants · Wild Planet · 8:00 pm'}, {value: 'c', label: 'Midnight Kitchen · Cookhouse · 11:00 pm'}]}, {kind: 'number', key: 'priority', label: 'Priority', help: 'Higher wins a tuner clash', value: 1, min: 1, max: 10}]}})]},
          {type: 'settings', title: 'Recording space', lede: 'Where recordings live and how much space they may use.', fields: [setting('number', 'dvr.retention', 'Keep recordings for', '0 keeps them until you delete them.', {unit: 'days', min: 0, max: 365}), setting('number', 'dvr.leaveFree', 'Always leave free', 'Recording stops before the volume drops below this.', {unit: 'GB', min: 0, max: 500}), setting('number', 'dvr.cap', 'Recordings may use up to', '0 means no cap beyond the free-space floor.', {unit: 'GB', min: 0, max: 4000, step: 50})]},
        ]};
      }
      case 'channels': return {...base, actions: [act('go', 'Create a channel', {icon: 'plus', tone: 'primary', navigate: {section: 'channel-builder'}})], blocks: [
        {type: 'list', items: libChannels.map(c => ({id: c.id, title: c.name, subtitle: c.desc, meta: c.on ? `${c.programs} programs · schedule reaches ${c.reach}` : 'Not generated yet', image: logoFor(c.id), badge: c.on ? {label: 'On air', tone: 'good'} : {label: 'Off', tone: 'neutral'}, actions: [act('go', 'Edit', {navigate: {section: 'channel-builder', param: c.id}, tone: 'secondary'}), act('regen', 'Regenerate schedule', {target: c.id, tone: 'ghost'}), act('health', 'Check health', {target: c.id, tone: 'ghost'}), act('toggleChannel', c.on ? 'Turn off' : 'Turn on', {target: c.id, tone: 'ghost'}), act('deleteChannel', 'Delete channel', {target: c.id, tone: 'danger', confirm: {title: `Delete “${c.name}”?`, body: 'The channel disappears from Live TV for everyone. Your media is not affected.', action: 'Delete channel'}})]})), empty: {title: 'No Library Channels yet', body: 'A Library Channel plays your own movies and shows on a schedule, like a TV channel.'}},
        {type: 'list', title: 'Start from a template', items: [{id: 'tp1', title: '80s Movie Marathon', subtitle: 'Movies from the 1980s, shuffled, no repeats.', icon: 'movie'}, {id: 'tp2', title: 'Sitcom Rewind', subtitle: 'Comedies, one episode of each show in turn.', icon: 'tv'}, {id: 'tp3', title: 'Kids’ Saturday', subtitle: 'G and PG titles from the Family library.', icon: 'sparkle'}, {id: 'tp4', title: 'Newly Added', subtitle: 'Anything added in the last 30 days.', icon: 'plus'}].map(t => ({...t, actions: [act('go', 'Use template', {navigate: {section: 'channel-builder', param: t.id}, tone: 'secondary'})]}))},
      ]};
      case 'channel-builder': {
        const editing = libChannels.find(c => c.id === param);
        const samples = catalog.movies.filter(m => m.genres.includes('Crime') || m.year < 1960).slice(0, 12);
        return {title: editing ? `Edit “${editing.name}”` : param?.startsWith('tp') ? 'Review template before creating it' : 'New channel', lede: 'Describe what the channel plays. The preview updates as you change it.', back: {section: 'channels', label: 'Library Channels'}, actions: [act('saveChannel', editing ? 'Save channel' : 'Create channel', {tone: 'primary', icon: 'check'})], blocks: [
          {type: 'settings', title: 'Channel', fields: [{kind: 'text', key: 'cb.name', label: 'Name', value: editing?.name ?? (param === 'tp1' ? '80s Movie Marathon' : '')}, {kind: 'text', key: 'cb.desc', label: 'Description', value: editing?.desc ?? ''}, {kind: 'toggle', key: 'cb.on', label: 'Channel is on', help: 'Everyone with access to these libraries sees this channel.', value: editing?.on ?? true}]},
          {type: 'settings', title: 'What plays', fields: [{kind: 'multi', key: 'cb.libraries', label: 'From these libraries', value: ['lib-movies'], options: [{value: 'lib-movies', label: 'Movies'}, {value: 'lib-tv', label: 'TV Shows'}, {value: 'lib-family', label: 'Family'}, {value: 'lib-anime', label: 'Anime'}]}, {kind: 'multi', key: 'cb.kinds', label: 'Include', value: ['movies'], options: [{value: 'movies', label: 'Movies'}, {value: 'shows', label: 'TV shows'}]}, {kind: 'choice', key: 'cb.match', label: 'Only titles that match', value: 'all', options: [{value: 'all', label: 'all of these'}, {value: 'any', label: 'any of these'}]}, {kind: 'text', key: 'cb.cond1', label: 'Condition: Genre is', value: 'Crime, Thriller'}, {kind: 'text', key: 'cb.cond2', label: 'Condition: Year is before', value: '1960'}, {kind: 'text', key: 'cb.mentions', label: 'Title or summary mentions', help: 'Words, like “shark”. Every word must appear.'}, {kind: 'text', key: 'cb.always', label: 'Always include', value: 'Detour, D.O.A.'}, {kind: 'text', key: 'cb.never', label: 'Never include'}]},
          {type: 'settings', title: 'Play order', fields: [{kind: 'choice', key: 'cb.order', label: 'Play order', value: 'shuffle', options: [{value: 'shuffle', label: 'Shuffle, no repeats'}, {value: 'random', label: 'Random (repeats allowed)'}, {value: 'inOrder', label: 'In order'}, {value: 'thenShuffle', label: 'In order, then shuffled'}]}, {kind: 'choice', key: 'cb.sort', label: 'Order by', value: 'release', options: [{value: 'title', label: 'Title'}, {value: 'release', label: 'Release date (oldest first)'}, {value: 'recent', label: 'Recently added'}, {value: 'rating', label: 'Highest rated'}, {value: 'episode', label: 'Show, then episode'}]}, {kind: 'choice', key: 'cb.episodes', label: 'Episodes', value: 'none', options: [{value: 'none', label: 'Mixed with everything else'}, {value: 'inOrder', label: 'Each show in episode order'}, {value: 'marathon', label: 'Marathon: one show at a time'}, {value: 'rotate', label: 'One episode of each show in turn'}, {value: 'random', label: 'Random episodes'}]}]},
          {type: 'settings', title: 'Advanced', fields: [{kind: 'choice', key: 'cb.whenDone', label: 'When everything has played', value: 'loop', options: [{value: 'loop', label: 'Start again'}, {value: 'slate', label: 'Show a slate'}]}, {kind: 'number', key: 'cb.avoid', label: 'Avoid repeats within', help: 'Programs', value: 20, min: 0, max: 200}, {kind: 'choice', key: 'cb.quality', label: 'Quality', value: 'auto', options: [{value: 'auto', label: 'Automatic'}, {value: 'original', label: 'Original only'}, {value: 'limited', label: 'Limited bitrate'}]}, {kind: 'text', key: 'cb.tz', label: 'Time zone', value: 'America/Los_Angeles'}, {kind: 'choice', key: 'cb.recent', label: 'Added in the last', value: 'any', options: [{value: 'any', label: 'Any time'}, {value: '7', label: '7 days'}, {value: '30', label: '30 days'}, {value: '90', label: '90 days'}]}]},
          {type: 'stats', items: [{label: 'Preview', value: `${samples.length + 23} titles match`, tone: 'accent'}, {label: 'Programming', value: 'About 41 hours'}, {label: 'Can’t be scheduled yet', value: '1 title', sub: 'Its length isn’t known', tone: 'warn'}]},
          {type: 'list', title: 'Today’s schedule', items: samples.slice(0, 8).map((m, i) => ({id: `sch${i}`, title: m.title, subtitle: `${m.year} · ${m.runtime} min`, meta: clock(now + i * 100 * MIN - (now % (30 * MIN))), image: posterFor(m.id)}))},
        ]};
      }
      case 'transcoding': return {...base, blocks: [
        {type: 'settings', title: 'Conversion', lede: 'When a device cannot play a file as it is, the server can convert it on the fly.', fields: [setting('toggle', 'transcoding.allow', 'Allow transcoding', 'When a device can’t play a file, the server converts it. If you turn this off, those devices show an error instead.'), setting('choice', 'transcoding.hardware', 'Hardware acceleration', undefined, {options: [{value: 'none', label: 'Off (processor only)'}, {value: 'videotoolbox', label: 'VideoToolbox (Apple)'}, {value: 'qsv', label: 'Intel Quick Sync'}, {value: 'nvenc', label: 'NVIDIA NVENC'}, {value: 'vaapi', label: 'VA-API'}]}), setting('toggle', 'transcoding.tonemap', 'Convert HDR for screens that can’t show it'), setting('toggle', 'transcoding.throttle', 'Throttle conversions ahead of the viewer'), setting('text', 'transcoding.tempPath', 'Temporary folder')]},
        {type: 'settings', title: 'Capacity', lede: 'Leave a field empty for no limit.', fields: [setting('number', 'transcoding.serverCap', 'Simultaneous streams', 'Across everyone using this server.', {min: 0, max: 20}), setting('number', 'transcoding.accountCap', 'Per account', 'Applies to each account. Set a member’s own stream limit in Access.', {min: 0, max: 10})]},
        {type: 'list', title: 'Account overrides', items: [{id: 'ov1', title: 'Walter Ehler', subtitle: 'Cap: 1 stream', icon: 'user', actions: [act('removeOverride', 'Remove override', {target: 'ov1', tone: 'ghost'})]}], actions: [act('addOverride', 'Add an account override', {icon: 'plus', form: {title: 'Add an account override', submit: 'Save', fields: [{kind: 'choice', key: 'account', label: 'Choose an account', value: 'nora', options: [{value: 'nora', label: 'Nora Kavanagh'}, {value: 'maya', label: 'Maya Ehler'}]}, {kind: 'number', key: 'cap', label: 'Cap', value: 2, min: 0, max: 10}]}})]},
        {type: 'stats', items: [{label: 'Converting now', value: '1 of 4', tone: 'accent'}, {label: 'Video encoder', value: '18%', spark: spark('enc')}, {label: 'Graphics processor', value: '11%', spark: spark('gpu', 24, 15, 10)}]},
      ]};
      case 'network': return {...base, blocks: [
        {type: 'settings', title: 'Remote access', lede: 'Lets your devices reach this server from outside your home through your Portico Account.', fields: [setting('toggle', 'network.remote', 'Remote access', 'When off, verified local connections keep working.')]},
        {type: 'facts', title: 'Addresses', rows: [{label: 'This server answers on', value: 'https://lighthouse.portico.direct:32500', copy: true}, {label: 'Home network', value: 'https://192.168.1.20:32500', copy: true}, {label: 'Public address', value: '203.0.113.42 · detected automatically'}, {label: 'Router mappings', value: 'UPnP · port 32500 open', tone: 'good'}, {label: 'HTTPS certificate', value: 'Trusted · valid until Dec 24', tone: 'good'}]},
        {type: 'settings', title: 'Advanced', lede: 'Router port mapping, the public port and the gateway.', fields: [setting('toggle', 'network.mapping', 'Automatic port mapping', 'Ask the router to open a port. Works on most home routers.'), setting('number', 'network.publicPort', 'Public port', undefined, {min: 1024, max: 65535}), setting('text', 'network.gateway', 'Router address', 'Leave empty to detect automatically.'), setting('toggle', 'network.ipv6', 'Allow incoming IPv6 connections', 'Lets apps away from home reach this server over IPv6, when your internet provider offers it.')]},
        {type: 'settings', title: 'Access from outside', lede: 'Rules for devices that are not on your home network.', fields: [setting('choice', 'network.outside', 'Who may sign in from outside', undefined, {options: [{value: 'everyone', label: 'Everyone with access'}, {value: 'admins', label: 'Administrators only'}, {value: 'nobody', label: 'Nobody'}]}), setting('choice', 'network.encrypted', 'Encrypted connections', 'Whether a device may connect without HTTPS.', {options: [{value: 'required', label: 'Required'}, {value: 'preferred', label: 'Preferred'}, {value: 'off', label: 'Off'}]}), setting('toggle', 'network.discovery', 'Find this server on the home network'), setting('number', 'network.qualityLimit', 'Quality limit away from home', 'Highest bitrate for anyone outside your home network, in Mbps. Zero means no limit.', {unit: 'Mbps', min: 0, max: 200}), setting('number', 'network.uploadSpeed', 'Upload speed', 'Your home internet upload speed, in Mbps.', {unit: 'Mbps', min: 1, max: 2000}), setting('choice', 'network.pausedLimit', 'End paused videos after', 'Frees the stream for someone else. Music, audiobooks and Live TV are never ended.', {options: [{value: '0', label: 'Never'}, {value: '30', label: '30 minutes'}, {value: '60', label: '1 hour'}, {value: '240', label: '4 hours'}]}), setting('text', 'network.homeNetworks', 'Home networks', 'Addresses or ranges that count as your home network, one per line.', {multiline: true}), setting('text', 'network.extraAddresses', 'Extra addresses', 'HTTPS addresses members’ apps can also use.', {multiline: true}), setting('toggle', 'network.wanAsLan', 'Count your public address as home'), setting('text', 'network.trustedProxies', 'Trusted reverse proxies'), setting('choice', 'network.interface', 'Address for home devices', 'Which network interface apps on your home network use.', {options: [{value: 'auto', label: 'Automatic'}, {value: 'en0', label: 'en0 · 192.168.1.20'}, {value: 'utun4', label: 'utun4 · 100.84.2.9 (virtual)'}]}), setting('folder', 'network.certPath', 'Custom certificate', 'Use your own HTTPS certificate instead of the one Portico manages. Leave empty to keep Portico’s.', {placeholder: '/etc/ssl/lighthouse.pem'}), setting('folder', 'network.certKey', 'Certificate key', 'The private key that goes with the certificate.', {placeholder: '/etc/ssl/lighthouse.key'}), setting('text', 'network.certDomain', 'Certificate domain', 'The name the certificate was issued for. Devices must reach the server by this name.', {placeholder: 'media.example.com'})]},
      ]};
      case 'people': return {...base, actions: [act('addAccount', 'Add account', {icon: 'plus', tone: 'primary', form: {title: 'Add an account', submit: 'Add account', fields: [{kind: 'text', key: 'name', label: 'Name'}, {kind: 'text', key: 'username', label: 'Username'}, {kind: 'text', key: 'password', label: 'Password', secure: true}, {kind: 'choice', key: 'role', label: 'Role', value: 'member', options: [{value: 'member', label: 'Member'}, {value: 'admin', label: 'Administrator'}]}, {kind: 'multi', key: 'libraries', label: 'Libraries they can see', value: ['lib-movies', 'lib-tv'], options: catalog.libraries.map(l => ({value: l.id, label: l.name}))}]}}), act('invite', 'Invite', {icon: 'mail', tone: 'secondary', navigate: {section: 'access'}})], blocks: [
        {type: 'list', title: 'Accounts on this server', lede: 'Everyone who can use this server: Portico Account members and accounts made here. Each has its own profiles.', items: people.map(p => ({id: p.id, title: p.name, subtitle: `${p.username} · ${p.kind} · ${p.profiles} profile${p.profiles === 1 ? '' : 's'}`, meta: p.libraries, icon: 'user', badge: p.disabled ? {label: 'Disabled', tone: 'neutral'} : p.role === 'Owner' ? {label: 'Owner', tone: 'accent'} : p.role === 'Administrator' ? {label: 'Admin', tone: 'accent'} : undefined, actions: p.role === 'Owner' ? [] : [act('editPerson', 'Edit', {target: p.id, tone: 'secondary', form: {title: `Edit ${p.name}`, submit: 'Save', fields: [{kind: 'text', key: 'profileName', label: 'Profile name', help: 'Shown when they pick a profile.', value: p.name.split(' ')[0]}, {kind: 'multi', key: 'libraries', label: 'Libraries they can see', value: ['lib-movies', 'lib-tv'], options: catalog.libraries.map(l => ({value: l.id, label: l.name}))}, {kind: 'toggle', key: 'disabled', label: 'Disable this account', help: 'They cannot sign in until it is enabled again.', value: p.disabled}]}}), act('deletePerson', 'Delete', {target: p.id, tone: 'danger', confirm: {title: `Delete “${p.name}”?`, body: 'Its watch history, Watchlist and playlists are deleted. This cannot be undone.', action: 'Delete'}})]}))},
        {type: 'list', title: 'Your profiles', lede: 'Profiles on your account keep separate history, Watchlist and preferences.', items: profilesByServer['srv-lighthouse']!.map(p => ({id: p.id, title: p.name, subtitle: p.kids ? 'Kids profile' : p.pin ? 'Protected by a PIN' : 'No PIN', icon: 'user', badge: p.main ? {label: 'Main', tone: 'neutral'} : undefined}))},
      ]};
      case 'access': return {...base, blocks: [
        {type: 'list', title: 'People', lede: 'Administrators can manage libraries and people. Only the owner can change who is an administrator.', items: people.filter(p => p.role !== 'Owner').map(p => ({id: p.id, title: p.name, subtitle: `${p.role} · ${p.streams ? `${p.streams} ${p.streams === 1 ? 'stream' : 'streams'} at once` : 'No stream limit'} · ${p.devices ? `${p.devices} ${p.devices === 1 ? 'device' : 'devices'}` : 'No device limit'}`, icon: 'user', actions: [act('role', p.role === 'Administrator' ? 'Make member' : 'Make administrator', {target: p.id, tone: 'secondary'}), act('limits', 'Limits…', {target: p.id, tone: 'ghost', form: {title: `Limits for ${p.name}`, lede: 'Zero means no limit.', submit: 'Save limits', fields: [{kind: 'number', key: 'streams', label: 'Streams at once', value: p.streams, min: 0, max: 10}, {kind: 'number', key: 'devices', label: 'Signed-in devices', value: p.devices, min: 0, max: 20}, {kind: 'number', key: 'remote', label: 'Quality away from home', help: 'Highest bitrate when not on your home network, in Mbps.', value: 8, unit: 'Mbps', min: 0, max: 100}, {kind: 'choice', key: 'rating', label: 'Highest rating', value: 'none', options: [{value: 'none', label: 'No limit'}, {value: 'PG-13', label: 'PG-13 (13+)'}, {value: 'PG', label: 'PG (8+)'}, {value: 'G', label: 'G (all ages)'}]}, {kind: 'text', key: 'labels', label: 'Hidden labels', help: 'Separate with commas.'}]}})]}))},
        {type: 'list', title: 'Invitations', lede: 'Make a link and a code to share. They join with their Portico Account, or make an account on this server. Portico sends no email.', items: invitations.map(i => ({id: i.id, title: i.label, subtitle: `${i.role} · expires ${i.expires}`, icon: 'mail', actions: [act('revokeInvite', 'Revoke', {target: i.id, tone: 'danger', confirm: {title: `Revoke the invitation for ${i.label}?`, body: 'The link and code stop working at once.', action: 'Revoke'}})]})), empty: {title: 'No invitations waiting', body: 'Invite someone to share this server.'}, actions: [act('createInvite', 'Invite someone', {icon: 'plus', form: {title: 'Invite someone', lede: 'Portico makes a link and a code for this server. Share them yourself; Portico sends no email.', submit: 'Create invitation', fields: [{kind: 'text', key: 'label', label: 'Who it’s for (optional)', help: 'A name or email, so you can tell your invitations apart.'}, {kind: 'choice', key: 'role', label: 'Role', value: 'member', options: [{value: 'member', label: 'Member'}, {value: 'admin', label: 'Administrator'}]}, {kind: 'multi', key: 'libraries', label: 'Libraries they can see', value: ['lib-movies', 'lib-tv', 'lib-music'], options: catalog.libraries.map(l => ({value: l.id, label: l.name}))}]}})]},
        {type: 'list', title: 'Devices', lede: `Every device that has signed in to this server.${accessDevices.some(d => d.state === 'waiting') ? ` ${accessDevices.filter(d => d.state === 'waiting').length} device is waiting for your approval.` : ''}`, items: accessDevices.map(d => ({id: d.id, title: d.name, subtitle: `${({web: 'Web browser', ios: 'iPhone or iPad', tvos: 'Apple TV', android: 'Android', androidtv: 'Android TV'} as Record<string, string>)[d.platform]} · last seen ${d.lastSeen.toLowerCase()}`, meta: d.location, icon: d.platform === 'tvos' || d.platform === 'androidtv' ? 'tvDevice' : d.platform === 'web' ? 'laptop' : 'phone', badge: d.state === 'waiting' ? {label: 'Waiting', tone: 'warn'} : d.state === 'blocked' ? {label: 'Blocked', tone: 'bad'} : d.trusted ? {label: 'Trusted', tone: 'good'} : {label: 'Approved', tone: 'neutral'}, actions: d.state === 'waiting' ? [act('approveDevice', 'Approve', {target: d.id, tone: 'primary'}), act('blockDevice', 'Block', {target: d.id, tone: 'danger'})] : [act('trustDevice', d.trusted ? 'Stop trusting' : 'Trust', {target: d.id, tone: 'ghost'}), act('blockDevice', d.state === 'blocked' ? 'Unblock' : 'Block', {target: d.id, tone: 'ghost'})]}))},
        {type: 'list', title: 'API keys', lede: 'For scripts and other apps that use this server as you. A key can only do what you allow when you create it.', items: apiKeys.map(k => ({id: k.id, title: k.name, subtitle: `${k.access} · ends in ${k.hint} · last used ${k.used.toLowerCase()}`, icon: 'key', actions: [act('revokeKey', 'Revoke', {target: k.id, tone: 'danger', confirm: {title: `Revoke “${k.name}”?`, body: 'Anything using this key stops working at once. This cannot be undone.', action: 'Revoke'}})]})), empty: {title: 'No API keys', body: 'Create one for a script or another app.'}, actions: [act('createKey', 'Create a key', {icon: 'plus', form: {title: 'Create an API key', submit: 'Create', fields: [{kind: 'text', key: 'name', label: 'Name', help: 'What will use this key.'}, {kind: 'choice', key: 'scope', label: 'What it may do', value: 'read', options: [{value: 'read', label: 'Read only'}, {value: 'library', label: 'Library management'}, {value: 'full', label: 'Everything I can do'}]}]}})]},
      ]};
      case 'schedules': return {...base, blocks: ['Back up database', 'Scan all libraries', 'Refresh metadata', 'Make scrubbing pictures', 'Analyze audio loudness', 'Clean up old conversions', 'Refresh program guide'].map((t, i) => ({type: 'settings', title: t, lede: ['Nightly copy of the database and settings.', 'Finds new, changed and missing files everywhere.', 'Updates details for titles older than 30 days.', 'Seek-bar pictures for new titles.', 'Evens out volume between songs.', 'Removes conversion leftovers.', 'Downloads the latest listings for every source.'][i], fields: [{kind: 'toggle', key: `sch.${i}.on`, label: 'Enabled', help: 'Disabling stops new runs; a run already in progress finishes.', value: (values.get(`sch.${i}.on`) ?? i !== 4) as boolean}, {kind: 'choice', key: `sch.${i}.start`, label: 'Window starts', help: 'Local time (America/Los_Angeles).', value: (values.get(`sch.${i}.start`) ?? ['03:00', '02:00', '04:00', '01:00', '01:00', '05:00', '00:30'][i]) as string, options: ['00:30', '01:00', '02:00', '03:00', '04:00', '05:00'].map(x => ({value: x, label: x}))}, {kind: 'number', key: `sch.${i}.length`, label: 'Window length', help: 'Minutes the task may run for.', value: (values.get(`sch.${i}.length`) ?? [60, 180, 120, 240, 120, 30, 60][i]) as number, unit: 'min', min: 15, max: 480, step: 15}, {kind: 'toggle', key: `sch.${i}.catch`, label: 'Catch up after a missed window', help: 'Run at the next opportunity if the server was off during the window.', value: (values.get(`sch.${i}.catch`) ?? true) as boolean}]} as Block))};
      case 'maintenance': return {...base, blocks: [
        {type: 'settings', title: 'Quiet hours for heavy work', lede: 'Scanning, analysis and picture generation are the jobs that load the server. Give them a window when nobody is watching.', fields: [setting('toggle', 'maintenance.pauseHeavy', 'Only do heavy work inside these hours', 'Off: work runs as usual. On: selected jobs wait for an enabled window.'), {kind: 'text', key: 'mw.name', label: 'Window name', value: 'Overnight'}, {kind: 'multi', key: 'mw.days', label: 'Days', value: ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'], options: ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'].map(d => ({value: d, label: d[0]!.toUpperCase() + d.slice(1)}))}, {kind: 'choice', key: 'mw.start', label: 'Starts at', value: '01:00', options: ['23:00', '00:00', '01:00', '02:00'].map(x => ({value: x, label: x}))}, {kind: 'number', key: 'mw.hours', label: 'Hours', value: 6, min: 1, max: 12}]},
        {type: 'settings', title: 'Background work', lede: 'Lower priority leaves more capacity for browsing and playback.', fields: [setting('choice', 'maintenance.priority', 'Background task priority', 'Lower is recommended. Normal uses up to one worker per CPU core for a faster first import.', {options: [{value: 'lower', label: 'Lower (recommended)'}, {value: 'normal', label: 'Normal'}]})]},
        {type: 'meters', title: 'Storage', items: [{label: 'Artwork cache', value: 0.34, detail: '6.8 GB · rebuilt automatically when needed'}, {label: 'Scrubbing pictures', value: 0.52, detail: '10.4 GB · rebuilt automatically when needed'}, {label: 'Conversion leftovers', value: 0.08, detail: '1.6 GB'}, {label: 'Database', value: 0.05, detail: '912 MB'}, {label: 'Logs', value: 0.02, detail: '214 MB'}]},
        {type: 'actions', actions: [act('cleanUp', 'Clean up conversion leftovers', {icon: 'broom', tone: 'secondary', confirm: {title: 'Clean up conversion leftovers?', body: 'These files are rebuilt when next needed, which costs the server some work.', action: 'Clean up'}}), act('cleanArtwork', 'Clear artwork cache', {tone: 'ghost', confirm: {title: 'Clean up the artwork cache?', body: 'These files are rebuilt when next needed, which costs the server some work.', action: 'Clean up'}})]},
        {type: 'list', title: 'Deleted titles', lede: 'Files removed through Portico wait here before they are gone for good.', items: trash.map(t => ({id: t.id, title: t.title, subtitle: `${t.files} file${t.files === 1 ? '' : 's'} · ${t.size} · deleted ${t.deleted} · kept until ${t.until}`, icon: 'trash', actions: [act('putBack', 'Put back', {target: t.id, tone: 'secondary'})]})), empty: {title: 'Nothing is waiting to be removed', body: 'Deleted titles appear here first.'}, actions: [act('removeDue', 'Remove what’s due', {tone: 'ghost'}), act('removeAll', 'Remove everything', {tone: 'danger', confirm: {title: 'Remove these files for good?', body: `This permanently deletes ${trash.length} titles from disk. It cannot be undone.`, action: 'Remove for good', typeToConfirm: String(trash.length)}})]},
        {type: 'facts', title: 'Updates', lede: 'Portico never installs an update by itself.', rows: [{label: 'This server', value: 'Portico Server 1.4.2'}, {label: 'Update check', value: 'An update is available · 1.5.0', tone: 'accent'}, {label: 'Channel', value: 'Stable'}, {label: 'Built', value: 'Sep 19, 2026'}]},
        {type: 'actions', actions: [act('releaseNotes', 'Release notes', {icon: 'external', tone: 'secondary'}), act('checkUpdates', 'Check again', {icon: 'refresh', tone: 'ghost'})]},
      ]};
      case 'backups': return {...base, actions: [act('backupNow', 'Back up now', {icon: 'archive', tone: 'primary', confirm: {title: 'Back up now?', body: 'Large libraries can take a few minutes. The server keeps working.', action: 'Back up'}})], blocks: [
        {type: 'notice', tone: 'neutral', title: 'Plain files you can copy', body: 'Backups are plain SQLite files you can copy or open with any SQLite tool. Media files are never included.'},
        {type: 'list', title: 'Backups', items: backups.map(b => ({id: b.id, title: b.when, subtitle: `${b.kind} · ${b.size} · Schema ${b.schema} · Server ${b.version}`, icon: 'archive', actions: [act('restore', 'Restore', {target: b.id, tone: 'secondary', confirm: {title: 'Restore this backup?', body: 'The server restarts and playback stops. A copy of the current data is kept as a Before restore backup. People may need to sign in again.', action: 'Restore backup'}}), act('deleteBackup', 'Delete', {target: b.id, tone: 'danger', confirm: {title: 'Delete this backup?', body: `The backup file is removed from this server. This cannot be undone. Type ${b.id} to confirm.`, action: 'Delete backup', typeToConfirm: b.id}})]})), empty: {title: 'No backups yet', body: 'Make one before an upgrade or a move to new hardware.'}, actions: [act('restoreFile', 'Restore from a file or folder…', {icon: 'folder', tone: 'ghost', form: {title: 'Choose a backup', lede: 'Pick a backup folder or a database file on this server.', submit: 'Use this file or folder', fields: [{kind: 'text', key: 'path', label: 'Folder path', value: '/var/lib/portico/backups/'}]}})]},
        {type: 'settings', title: 'Keep', fields: [setting('number', 'maintenance.keepBackups', 'Backups to keep', undefined, {min: 1, max: 60})]},
      ]};
      case 'general': return {...base, blocks: [
        ...(custodyPending ? [{type: 'notice' as const, tone: 'accent' as Tone, title: 'Accept ownership of this server’s Portico registration', body: 'You’re the owner of Lighthouse now. Accept so your Portico Account holds its registration, which covers remote access and invitations.', actions: [act('acceptCustody', 'Accept', {tone: 'primary'})]}] : []),
        {type: 'settings', title: 'Identity', fields: [setting('text', 'general.name', 'Server name', 'Shown on every device that connects and in your Portico Account.')]},
        {type: 'facts', title: 'Version', rows: [{label: 'Portico Server', value: '1.4.2 (stable)'}, {label: 'Server ID', value: 'srv_7f3c2a91d04e', copy: true}, {label: 'Platform', value: 'macOS 26 · Apple M2 Pro'}, {label: 'Data folder', value: '/var/lib/portico'}]},
        {type: 'facts', title: 'Portico Account connection', lede: 'Connecting the server to a Portico Account enables remote access, invitations and switching between servers.', rows: [{label: 'Status', value: 'Connected · signed by your Portico Account', tone: 'good'}, {label: 'Owner account', value: 'justin (Justin Ehler)'}, {label: 'Recovery owner', value: 'A local owner account remains available for recovery even when the server is connected.'}]},
        {type: 'actions', actions: [act('checkStatus', 'Check status', {icon: 'refresh', tone: 'secondary'}), act('transfer', 'Transfer ownership…', {icon: 'crown', tone: 'ghost', form: {title: 'Transfer ownership', lede: 'Choose the member who’ll own this server.', submit: 'Transfer', fields: [{kind: 'choice', key: 'member', label: 'New owner', value: 'maya', options: [{value: 'maya', label: 'Maya Ehler'}, {value: 'walter', label: 'Walter Ehler'}]}]}})]},
      ]};
    }
    return {...base, blocks: []};
  }

  function sessionItem(s: typeof sessions[number]): ListItem {
    return {id: s.id, title: s.title, subtitle: `${s.viewer} · ${s.device}`, meta: `${s.delivery} · ${s.rate} Mb/s · ${s.location}`, image: posterFor(s.item) ?? logoFor(s.item), progress: s.progress, badge: {label: s.state, tone: s.state === 'Playing' ? 'good' : 'neutral'}};
  }

  return {
    sections: adminSections,
    async load(section, param) { await latency(); return page(section, param); },
    async folders(path) {
      await latency(120);
      const at = path && FOLDER_TREE[path] !== undefined ? path : '/';
      const entry = (p: string) => ({name: p.split('/').pop() || '/', path: p, access: !NO_ACCESS.has(p), link: LINKS.has(p)});
      const parent = at === '/' ? undefined : at.slice(0, at.lastIndexOf('/')) || '/';
      return {path: at, parent, entries: (FOLDER_TREE[at] ?? []).map(n => entry(join(at, n))), roots: ['/media', '/mnt/nas', '/Volumes', '/home/justin'].map(entry)};
    },
    async setValue(key, value) { values.set(key, value); emit(); },
    async act(action, target, input) {
      await latency(250);
      switch (action) {
        case 'acceptCustody': custodyPending = false; emit(); return 'You now hold Lighthouse’s Portico registration.';
        case 'ack': alerts = alerts.filter(a => a.id !== target); break;
        case 'stop': sessions = sessions.filter(s => s.id !== target); emit(); return 'Playback stopped. The viewer sees that the server owner stopped it.';
        case 'reply': reports = reports.map(r => (r.id === target ? {...r, status: String(input?.status ?? 'answered')} : r)); break;
        case 'scan': { const was = libraries.find(l => l.id === target)?.scanning; libraries = libraries.map(l => (l.id === target ? {...l, scanning: !l.scanning, lastScan: l.scanning ? 'Just now' : l.lastScan} : l)); if (!was && target) void viewer?.libraries.scan(target); emit(); return was ? 'Scan cancelled.' : 'Scan started. New titles appear as they’re found.'; }
        case 'scanAll': emit(); return 'Full scan started. Every file is read again, changed or not.';
        case 'addLibrary': libraries = [...libraries, {...libraries[0]!, id: `lib-${Date.now()}`, name: String(input?.name || 'New library'), kind: (input?.kind as never) ?? 'movie', path: String(input?.folder || '/media/New'), folders: [String(input?.folder || '/media/New')], scanning: true, lastScan: 'Never', attention: 0}]; emit(); return 'Library added. Scanning your files…';
        case 'addFolder': libraries = libraries.map(l => (l.id === target ? {...l, folders: [...l.folders, String(input?.path || '/media/More')]} : l)); break;
        case 'removeFolder': libraries = libraries.map(l => ({...l, folders: l.folders.filter(f => f !== target)})); break;
        case 'rename': libraries = libraries.map(l => (l.id === target ? {...l, name: String(input?.name || l.name)} : l)); break;
        case 'deleteLibrary': libraries = libraries.filter(l => l.id !== target); emit(); return 'Library deleted.';
        case 'backupNow': backups = [{id: `bk-${Date.now() % 10000}`, kind: 'Manual', when: 'Just now', size: '184 MB', schema: '42', version: '1.4.2'}, ...backups]; emit(); return 'Backup complete.';
        case 'deleteBackup': backups = backups.filter(b => b.id !== target); emit(); return 'Backup deleted.';
        case 'restore': return 'Restored. The server is starting again with the restored data.';
        case 'createInvite': invitations = [{id: `iv${Date.now()}`, label: String(input?.label || 'Invitation'), role: input?.role === 'admin' ? 'Administrator' : 'Member', expires: 'in 7 days'}, ...invitations]; emit(); return 'Invitation ready: https://getportico.tv/join#k7qd-2mxf · code K7QD 2MXF. It works once and expires in 7 days.';
        case 'revokeInvite': invitations = invitations.filter(i => i.id !== target); break;
        case 'createKey': apiKeys = [...apiKeys, {id: `k${Date.now()}`, name: String(input?.name || 'New key'), access: input?.scope === 'full' ? 'Everything' : input?.scope === 'library' ? 'Library management' : 'Read only', hint: 'c3e8', used: 'Never'}]; emit(); return 'Copy your key now: ptk_live_5f1c9e7a2b64d80c3e8. This is the only time the key is shown.';
        case 'revokeKey': apiKeys = apiKeys.filter(k => k.id !== target); break;
        case 'approveDevice': accessDevices = accessDevices.map(d => (d.id === target ? {...d, state: 'approved'} : d)); alerts = alerts.filter(a => a.id !== 'al2'); break;
        case 'blockDevice': accessDevices = accessDevices.map(d => (d.id === target ? {...d, state: d.state === 'blocked' ? 'approved' : 'blocked'} : d)); break;
        case 'trustDevice': accessDevices = accessDevices.map(d => (d.id === target ? {...d, trusted: !d.trusted} : d)); break;
        case 'role': people = people.map(p => (p.id === target ? {...p, role: p.role === 'Administrator' ? 'Member' : 'Administrator'} : p)); break;
        case 'editPerson': people = people.map(p => (p.id === target ? {...p, disabled: !!input?.disabled} : p)); break;
        case 'deletePerson': people = people.filter(p => p.id !== target); break;
        case 'addAccount': people = [...people, {id: `u${Date.now()}`, name: String(input?.name || 'New person'), username: String(input?.username || 'new.person'), role: input?.role === 'admin' ? 'Administrator' : 'Member', kind: 'Server account', profiles: 1, libraries: 'Movies, TV Shows', disabled: false, streams: 0, devices: 0}]; break;
        case 'putBack': trash = trash.filter(t => t.id !== target); emit(); return 'Put back. The title is in its library again.';
        case 'removeAll': trash = []; break;
        case 'removeDue': trash = trash.filter(t => t.until !== 'Tomorrow'); break;
        case 'toggleChannel': libChannels = libChannels.map(c => (c.id === target ? {...c, on: !c.on} : c)); break;
        case 'deleteChannel': libChannels = libChannels.filter(c => c.id !== target); break;
        case 'saveChannel': emit(); return 'Channel saved. Its schedule is being made now.';
        case 'pauseSource': sources = sources.map(s => (s.id === target ? {...s, paused: !s.paused} : s)); break;
        case 'removeSource': sources = sources.filter(s => s.id !== target); break;
        case 'refreshGuide': emit(); return 'Refresh started.';
        case 'cancelTask': tasks = tasks.map(t => (t.id === target ? {...t, state: 'Canceled', progress: 0} : t)); break;
        case 'retryTask': tasks = tasks.map(t => (t.id === target ? {...t, state: 'Running', progress: 0.05} : t)); break;
        case 'broadcast': emit(); return 'Sent. It appears in everyone’s notifications.';
        case 'capture': emit(); return `Capturing verbose records for ${values.get('logs.component')} for ${values.get('logs.duration')} minutes.`;
        case 'export': emit(); return 'Ready to download. Includes server records and health snapshot · redaction on · expires in 24 hours.';
      }
      emit();
      return undefined;
    },
    subscribe(fn) { listeners.add(fn); return () => listeners.delete(fn); },
  };
}

export const personImage = personArt;
