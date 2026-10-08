/**
 * Server administration model (owner and admins). One typed state object per console section,
 * read and patched through `AdminService`, mirroring the web console's `/server/$section` pages.
 */

export type Health = 'ok' | 'warn' | 'bad';
export type AttentionItem = {id: string; severity: 'info' | 'warn' | 'urgent'; title: string; body: string; section: string; acknowledged: boolean};
export type PlaybackSession = {id: string; itemId: string; title: string; subtitle: string; profile: string; account: string; device: string; platform: string; state: 'playing' | 'paused' | 'starting'; progress: number; delivery: 'Direct play' | 'Direct stream' | 'Converting video' | 'Converting audio'; location: 'Home network' | 'Remote'; rateMbps: number; startedAt: number; quality: string; hardware?: string};
export type TaskRow = {id: string; name: string; state: 'running' | 'queued' | 'done' | 'failed' | 'cancelled'; progress: number; startedAt: number; detail: string};
export type LogLine = {id: string; at: number; level: 'debug' | 'info' | 'warn' | 'error'; component: string; message: string};
export type Capability = {id: string; label: string; state: 'available' | 'unavailable' | 'unknown'; reason?: string; technical?: string};

export type AdminLibrary = {id: string; name: string; kind: string; items: number; sizeTB: number; lastScan: number; scanning: boolean; scanProgress: number; folders: {id: string; path: string; location: 'Local disk' | 'Network share'; available: boolean; depth: number}[]; attention: {path: string; reason: string}[]; metadata: 'online' | 'local'; language: string; country: string; rescan: string; missingAfter: string; trickplayEvery: number; trickplayWidth: number; chapterPictures: boolean; allowDelete: boolean; keepDeletedDays: number; fetchLyrics: boolean};

export type StorageMount = {id: string; kind: 'webdav' | 'rclone'; name: string; address: string; state: 'mounted' | 'unmounted' | 'error'; usedTB: number; libraries: string[]};
export type LiveSource = {id: string; name: string; kind: 'HDHomeRun' | 'M3U' | 'Xtream' | 'Upload' | 'Guide only'; address: string; channels: number; tuners: number; paused: boolean; lastRefresh: number; guideFrom: string | null; numbering: 'asSent' | 'inOrder'; startAt: number; logos: boolean; health: Health};
export type Tuner = {id: string; name: string; source: string; inUse: string | null; next: string | null};
export type LibraryChannel = {id: string; name: string; number: string; on: boolean; libraries: string[]; kinds: ('movies' | 'shows')[]; order: string; episodes: string; conditions: {field: string; op: string; value: string}[]; scheduleHours: number; candidates: number; unresolved: number; health: 'upToDate' | 'beingMade' | 'notGenerated'; description: string};
export type AdminAccount = {id: string; name: string; username: string; kind: 'Portico Account' | 'Server account'; role: 'owner' | 'admin' | 'member'; disabled: boolean; profiles: {id: string; name: string; color: string; kids: boolean}[]; libraries: 'all' | string[]; limits: {streams: number; devices: number; remoteMbps: number; rating: string | null}; lastSeen: number};
export type AdminInvite = {id: string; label: string; role: 'member' | 'admin'; expires: number; code: string; link: string};
export type AdminDevice = {id: string; name: string; platform: string; account: string; trust: 'approved' | 'blocked' | 'waiting'; lastSeen: number};
export type ApiKey = {id: string; name: string; access: 'Read only' | 'Playback' | 'Full'; hint: string; lastUsed: number | null; created: number};
export type Schedule = {id: string; name: string; description: string; enabled: boolean; start: string; lengthMin: number; catchUp: boolean; lastRun: number; nextRun: number; lastResult: 'ok' | 'failed' | 'skipped'};
export type Backup = {id: string; kind: 'Scheduled' | 'Manual' | 'Before restore'; at: number; sizeMB: number; schema: number; version: string};
export type TrashEntry = {id: string; title: string; files: number; sizeGB: number; deletedAt: number; keptUntil: number; due: boolean};
export type StorageBucket = {id: string; name: string; description: string; sizeGB: number; canRecreate: boolean};
export type FeedbackRow = {id: string; from: string; category: string; message: string; title?: string; at: number; status: 'new' | 'looking' | 'fixed' | 'wontFix'; reply: string};
export type Telemetry = {cpu: number[]; memory: number[]; netOut: number[]; diskRead: number[]; gpu: number[]; viewingHours: number[]};

export type AdminState = {
  overview: {serverName: string; version: string; uptimeDays: number; health: {label: string; state: Health; detail: string}[]; attention: AttentionItem[]; telemetry: Telemetry; recent: {title: string; profile: string; at: number; device: string}[]; storage: {label: string; usedTB: number; totalTB: number}[]};
  nowPlaying: PlaybackSession[];
  activity: {tasks: TaskRow[]};
  logs: {lines: LogLine[]; level: 'debug' | 'info' | 'warn' | 'error'; following: boolean; capture: {component: string; minutes: number; until: number | null}; retention: {records: number; notifications: number; tasks: number}; export: {window: string; server: boolean; client: boolean; audit: boolean; snapshot: boolean; ready: boolean}};
  diagnostics: {capabilities: Capability[]; checks: {label: string; state: Health; detail: string}[]};
  feedback: FeedbackRow[];
  libraries: AdminLibrary[];
  metadata: {providers: {id: string; name: string; enabled: boolean; kinds: string; key: boolean}[]; consent: 'granted' | 'waiting' | 'declined'; language: string; country: string; refreshDays: number; lyrics: boolean};
  storage: StorageMount[];
  liveTv: {sources: LiveSource[]; recordingsPath: string; writable: boolean; keepDays: number; episodesPerSeries: number; leaveFreeGB: number; capGB: number; usedGB: number; totalGB: number};
  dvr: {startEarly: number; endLate: number; keepMode: 'newest' | 'days' | 'all'; keepCount: number; afterRecording: 'none' | 'remux' | 'convert'; removeOriginal: boolean; folderShows: string; folderMovies: string; sidecarNfo: boolean; sidecarFanart: boolean; tuners: Tuner[]; buffer: number; retry: number; guideDays: number; userAgent: string; logos: boolean; discovery: boolean; recorders: {profile: string; account: string; allowed: boolean; inherit: boolean}[]; rules: {id: string; name: string; owner: string; channel: string; newOnly: boolean; paused: boolean; priority: number}[]};
  channels: LibraryChannel[];
  transcoding: {allow: boolean; serverCap: number; accountCap: number; hardware: 'auto' | 'videotoolbox' | 'vaapi' | 'nvenc' | 'off'; overrides: {account: string; cap: number}[]; tonemap: boolean; sandbox: boolean};
  network: {remote: boolean; publicAddress: string; publicPort: number; localAddresses: string[]; mapping: {enabled: boolean; state: 'open' | 'refused' | 'pending'; protocol: string}; certificate: {state: 'trusted' | 'ready' | 'off'; domain: string; expires: number}; outside: 'everyone' | 'accountsOnly' | 'nobody'; encrypted: 'required' | 'preferred' | 'optional'; discovery: boolean; qualityLimit: number; uploadSpeed: number; pausedLimit: number; homeNetworks: string; extraAddresses: string; wanAsLan: boolean; ipv6: boolean; trustedProxies: string; interface: string};
  people: AdminAccount[];
  access: {invites: AdminInvite[]; devices: AdminDevice[]; keys: ApiKey[]};
  schedules: Schedule[];
  maintenance: {quietHours: {id: string; name: string; days: string[]; start: string; hours: number; enabled: boolean}[]; onlyInWindows: boolean; priority: 'lower' | 'normal'; buckets: StorageBucket[]; trash: TrashEntry[]; update: {current: string; latest: string; channel: string; builtAt: number; available: boolean; notes: string[]}; permissionsOk: boolean; broadcast: {title: string; message: string; audience: 'everyone' | 'admins'; importance: 'info' | 'notice' | 'urgent'}};
  backups: {list: Backup[]; keep: number; schedule: string; lastResult: string};
  alerts: {storageWarning: number; storageCritical: number; certificateDays: number; offlineMinutes: number; failedScans: boolean};
  general: {name: string; version: string; serverId: string; connection: 'connected' | 'pending' | 'notConnected'; ownerAccount: string; recoveryOwner: string; platform: string; dataFolder: string};
};

export interface AdminService {
  get<K extends keyof AdminState>(key: K): Promise<AdminState[K]>;
  set<K extends keyof AdminState>(key: K, value: AdminState[K]): Promise<void>;
  /** Named operations that do more than store a value (scan, back up, stop a session…). */
  run(op: string, args?: Record<string, unknown>): Promise<string>;
}
