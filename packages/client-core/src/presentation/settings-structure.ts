import type {IconId} from '../../../design/src/icons.ts';
import type {MessageId, MessageValues} from '../../../i18n/src/index.ts';
import type {PreferencePatch, PreferenceValue} from '../preferences.ts';
import type {ServerFormId} from '../server-admin/server-forms.ts';

/**
 * The one Settings screen, authored (Justin, 2 Oct 2026). Personal and server settings are one
 * destination: pages listed under two headings, **Account** (personal) and **Server** (owners).
 *
 * The preference registry and the server's capabilities are vocabulary, not layout. This file is
 * the layout: which pages exist, in what order, which rows each holds, what each row says and
 * which control it uses. A registry key no row references does not appear. Every client draws
 * this structure with its own controls — a two-pane screen on the web and iPad, a pushed list on
 * iPhone, a column of pages beside the rows on television — and none of them invents a row.
 *
 * Rows:
 * - `preference`: one registry key (`operations/preferences.go` stays the storage and validation
 *   contract).
 * - `composite`: one decision a person makes that reads and writes several keys ("Playback
 *   method"), or a value this device keeps for itself ("Default library view").
 * - `action`: something done, not set (Sign out, Reset recommendations, Link a TV).
 * - `custom`: a panel the client draws natively (profiles, devices, the subtitle preview).
 * - `server-setting`: one value of a server settings document (`server-admin/server-forms.ts`),
 *   edited as a draft and saved with the page's one Save.
 */

export type SettingsPlatform = 'web' | 'phone' | 'tv';
export type SettingsHeadingId = 'account' | 'server';
/** Whose the value is: the profile's on this server (it follows them), or this device's. Said on the row. */
export type SettingsScope = 'profile' | 'device';

export type SettingsCapabilities = Readonly<{
  /** Registry keys this server publishes; a `preference` row whose key is missing is left out. Absent: all are assumed. */
  preferenceKeys?: ReadonlySet<string>;
  /** Signed in with an account that lives on this server: profiles, password, two-step and devices are managed here. */
  localAccount?: boolean;
  /** A Portico Account member of this server: password and two-step verification are the account's. */
  porticoMember?: boolean;
  /** A Portico Account sign-in through Portico's servers. */
  hostedAccount?: boolean;
  /** This device can keep downloads. */
  downloads?: boolean;
  /** The client can say what this device is called in device lists. */
  deviceName?: boolean;
  /** Password and two-step verification can be changed here. Absent: a server account that is not a Portico Account member. */
  manageSignIn?: boolean;
  /** The server offers Live TV. */
  liveTV?: boolean;
  /** This device's player can change playback speed. Absent: it can (a Roku's player cannot). */
  playbackSpeed?: boolean;
  /** This device renders audio itself: gapless, crossfade and volume leveling. Absent: it does (a Roku's player cannot). */
  audioEffects?: boolean;
}>;

export type SettingsContext = Readonly<{owner: boolean; platform: SettingsPlatform; capabilities: SettingsCapabilities}>;

export type SettingsChoice = Readonly<{value: string | number | boolean; label: MessageId; labelValues?: MessageValues}>;

export type SettingsControl =
  | Readonly<{type: 'switch'}>
  | Readonly<{type: 'choice'; choices: readonly SettingsChoice[]}>
  /** An ordered list of languages, picked by name. Never comma-separated text. */
  | Readonly<{type: 'languages'}>
  | Readonly<{type: 'locale'}>;

export type CompositeId =
  | 'quality.home' | 'quality.away' | 'quality.cellular' | 'playbackMethod' | 'hdr' | 'cardSize'
  | 'libraryView' | 'reduceMotion' | 'textSize' | 'receiveFromDevices';
export type SettingsActionId = 'signOut' | 'resetRecommendations' | 'linkTV';
export type SettingsCustomId =
  | 'identity' | 'profiles' | 'password' | 'twoStep' | 'devices' | 'porticoAccount' | 'servers'
  | 'subtitlePreview' | 'deviceName' | 'downloadsStorage' | 'about'
  // The Server heading's panels: each is drawn natively by the client.
  | 'server.nowPlaying' | 'server.alerts' | 'server.health' | 'server.activity' | 'server.statistics'
  | 'server.playHistory'
  | 'server.libraries'
  | 'server.liveSources' | 'server.tuners' | 'server.recordingRules' | 'server.recordingStorage' | 'server.channels'
  | 'server.accounts' | 'server.invitations' | 'server.apiKeys'
  | 'server.transcodeStatus'
  | 'server.remoteStatus' | 'server.accountConnection' | 'server.certificateStatus' | 'server.addresses'
  | 'server.diskUsage' | 'server.retention' | 'server.remoteSources' | 'server.backups' | 'server.deletedTitles'
  | 'server.windows' | 'server.jobs'
  | 'server.logs' | 'server.detailWindow' | 'server.records' | 'server.capabilities' | 'server.supportExport' | 'server.feedback' | 'server.stateFolder'
  | 'server.identity' | 'server.updates';

type RowCopy = Readonly<{
  label: MessageId;
  help?: MessageId;
  /** Shown as "This device" on the row. Profile settings say nothing: they are the rule. */
  scope?: SettingsScope;
  /** Shown only while another preference has this value (the countdown, while auto-play is on). */
  when?: Readonly<{key: string; equals: PreferenceValue}>;
}>;

export type SettingsRow =
  | (RowCopy & Readonly<{kind: 'preference'; key: string; control: SettingsControl}>)
  | (RowCopy & Readonly<{kind: 'composite'; id: CompositeId; /** `device`: kept by this device itself, not in the registry. */ storage: 'registry' | 'device'; control: Extract<SettingsControl, {type: 'choice' | 'switch'}>}>)
  | (RowCopy & Readonly<{kind: 'action'; id: SettingsActionId; icon?: IconId; destructive?: boolean}>)
  | Readonly<{kind: 'custom'; id: SettingsCustomId; /** For search and for a client that titles the panel. */ label?: MessageId}>
  | ServerSettingRow;

/** How a server setting is edited. A number shows `unit` after it; `scale` converts what is shown to what is stored (minutes shown, seconds stored: 60). */
export type ServerControl =
  | Readonly<{type: 'switch'}>
  | Readonly<{type: 'choice'; choices: readonly SettingsChoice[]}>
  /** Choices the server names at run time (network interfaces, what a recording can be converted to): resolved by `serverChoices`. */
  | Readonly<{type: 'choiceFrom'; source: 'interfaces' | 'conversionModes'}>
  | Readonly<{type: 'number'; min: number; max: number; unit?: MessageId; scale?: number; /** An empty field stores this (no limit) and shows the placeholder. */ empty?: Readonly<{stores: null | 0; placeholder: MessageId}>}>
  | Readonly<{type: 'text'; mono?: boolean; example?: string; maxLength?: number}>
  /** A list edited one entry per line (networks, addresses). */
  | Readonly<{type: 'lines'; example?: string}>;

export type ServerSettingRow = Readonly<{
  kind: 'server-setting';
  form: ServerFormId;
  /** Dotted path into the form's draft (`keepPolicy.mode`). A row whose value the server did not send is absent. */
  key: string;
  label: MessageId;
  help?: MessageId;
  control: ServerControl;
  /** Shown only while another value of the same form matches. */
  when?: Readonly<{key: string; equals?: unknown; not?: unknown}>;
}>;

export type SettingsSection = Readonly<{id: string; title?: MessageId; /** A sentence under the title. */ description?: MessageId; /** A quiet group below the rest ("Advanced"), collapsed until asked for. */ advanced?: boolean; /** The sub-page this section belongs to, on a page that has them. */ tab?: string; rows: readonly SettingsRow[]}>;
export type SettingsTab = Readonly<{id: string; title: MessageId}>;
export type SettingsPage = Readonly<{id: string; title: MessageId; icon: IconId; /** Sub-pages (Sources · Recording · Library Channels); each section names the one it is on. */ tabs?: readonly SettingsTab[]; sections: readonly SettingsSection[]}>;
export type SettingsHeading = Readonly<{id: SettingsHeadingId; title: MessageId; pages: readonly SettingsPage[]}>;
export type SettingsStructure = Readonly<{headings: readonly SettingsHeading[]}>;

// ── Authoring helpers ────────────────────────────────────────────────────────

type Only = Readonly<{platforms?: readonly SettingsPlatform[]; needs?: (c: SettingsCapabilities) => boolean}>;
type Authored<T> = T & Only;
type AuthoredSection = Readonly<{id: string; title?: MessageId; description?: MessageId; advanced?: boolean; tab?: string; rows: readonly Authored<SettingsRow>[]}> & Only;
type AuthoredPage = Readonly<{id: string; title: MessageId; icon: IconId; tabs?: readonly SettingsTab[]; sections: readonly AuthoredSection[]}> & Only;

const seconds = (...values: number[]): SettingsChoice[] => values.map(value => ({value, label: 'pref.unit.seconds', labelValues: {count: value}}));
const speeds: SettingsChoice[] = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2].map(value => (value === 1 ? {value, label: 'pref.unit.speedNormal'} : {value, label: 'pref.unit.speed', labelValues: {value}}));
const skipChoices = (key: string): SettingsChoice[] => (['ask', 'auto', 'off'] as const).map(value => ({value, label: `pref.value.${key}.${value}` as MessageId}));
const limitChoices: SettingsChoice[] = [
  {value: 'automatic', label: 'settings.quality.limit.automatic'},
  {value: 'original', label: 'settings.quality.limit.original'},
  {value: '2160', label: 'pref.unit.4k'},
  {value: '1080', label: 'pref.unit.height', labelValues: {value: '1080'}},
  {value: '720', label: 'pref.unit.height', labelValues: {value: '720'}},
  {value: '480', label: 'pref.unit.height', labelValues: {value: '480'}},
];
const pref = (key: string, label: MessageId, control: SettingsControl, more: Partial<RowCopy> & Only = {}): Authored<SettingsRow> => ({kind: 'preference', key, label, control, ...more});
const toggle = (key: string, label: MessageId, more: Partial<RowCopy> & Only = {}) => pref(key, label, {type: 'switch'}, more);
const custom = (id: SettingsCustomId, label?: MessageId, only: Only = {}): Authored<SettingsRow> => ({kind: 'custom', id, ...(label ? {label} : {}), ...only});

// ── Account ─────────────────────────────────────────────────────────────────

const accountPages: readonly AuthoredPage[] = [
  {
    id: 'profile', title: 'settings.page.profile', icon: 'profile', sections: [
      {id: 'who', rows: [custom('identity', 'settings.row.identity')]},
      // Direct-sign-in servers manage profiles, sessions and two-step here; a Portico Account manages its own on the account site.
      {id: 'profiles', title: 'settings.section.profiles', rows: [custom('profiles', 'settings.section.profiles')], needs: c => !!c.localAccount || !!c.hostedAccount},
      // Television hands typing-heavy tasks to a phone or computer; a profile that isn't the account's main one manages nothing.
      {id: 'signIn', title: 'settings.section.signIn', rows: [custom('password', 'settings.row.password'), custom('twoStep', 'settings.row.twoStep')], platforms: ['web', 'phone'], needs: c => c.manageSignIn ?? (!!c.localAccount && !c.porticoMember)},
      // What the Portico Account itself manages, and this server's link to it; a client with nothing to say draws nothing.
      {id: 'porticoAccount', rows: [custom('porticoAccount', 'settings.row.porticoAccount')]},
      {id: 'devices', title: 'settings.section.devices', rows: [custom('devices', 'settings.section.devices')], needs: c => !!c.localAccount},
      {id: 'session', rows: [{kind: 'action', id: 'signOut', label: 'action.signOut', icon: 'signOut', destructive: true}]},
    ],
  },
  {
    id: 'playback', title: 'settings.page.playback', icon: 'play', sections: [
      {id: 'next', title: 'settings.section.nextEpisode', rows: [
        toggle('playback.autoplayNext', 'settings.row.autoplayNext', {help: 'pref.help.playback.autoplayNext'}),
        pref('playback.upNextCountdownSeconds', 'settings.row.countdown', {type: 'choice', choices: [{value: 0, label: 'pref.value.playback.upNextCountdownSeconds.0'}, ...seconds(5, 10, 15)]}, {when: {key: 'playback.autoplayNext', equals: true}}),
        toggle('playback.passoutProtection', 'settings.row.stillWatching', {help: 'settings.help.stillWatching'}),
        pref('playback.passoutAfterEpisodes', 'settings.row.stillWatchingAfter', {type: 'choice', choices: [2, 3, 4, 5].map(value => ({value, label: 'pref.unit.episodes' as MessageId, labelValues: {count: value}}))}, {when: {key: 'playback.passoutProtection', equals: true}}),
      ]},
      {id: 'skipping', title: 'settings.section.skipping', rows: [
        pref('playback.introSkip', 'pref.label.playback.introSkip', {type: 'choice', choices: skipChoices('playback.introSkip')}),
        pref('playback.creditsSkip', 'pref.label.playback.creditsSkip', {type: 'choice', choices: skipChoices('playback.creditsSkip')}),
        pref('playback.recapSkip', 'pref.label.playback.recapSkip', {type: 'choice', choices: skipChoices('playback.recapSkip')}),
        pref('playback.skipBackSeconds', 'pref.label.playback.skipBackSeconds', {type: 'choice', choices: seconds(5, 10, 15, 30)}, {help: 'pref.help.playback.skipBackSeconds'}),
        pref('playback.skipForwardSeconds', 'pref.label.playback.skipForwardSeconds', {type: 'choice', choices: seconds(10, 15, 30, 60)}, {help: 'pref.help.playback.skipForwardSeconds'}),
      ]},
      // No music speed: a song plays at the speed it was recorded.
      {id: 'speed', title: 'settings.section.speed', needs: c => c.playbackSpeed !== false, rows: [
        pref('playback.defaultSpeed', 'settings.row.videoSpeed', {type: 'choice', choices: speeds}),
        pref('audiobooks.defaultSpeed', 'settings.row.audiobookSpeed', {type: 'choice', choices: speeds}),
      ]},
      {id: 'advanced', title: 'settings.section.advanced', advanced: true, rows: [
        pref('playback.playedThresholdPercent', 'settings.row.watchedThreshold', {type: 'choice', choices: [80, 85, 90, 95, 100].map(value => ({value, label: 'pref.unit.percent' as MessageId, labelValues: {value}}))}, {help: 'pref.help.playback.playedThresholdPercent'}),
      ]},
    ],
  },
  {
    id: 'languages', title: 'settings.page.audioSubtitles', icon: 'subtitles', sections: [
      {id: 'audio', title: 'settings.section.audio', rows: [
        pref('playback.preferredAudioLanguages', 'pref.label.playback.preferredAudioLanguages', {type: 'languages'}, {help: 'settings.help.audioLanguages'}),
      ]},
      {id: 'subtitles', title: 'settings.section.subtitles', rows: [
        pref('playback.subtitleMode', 'settings.row.subtitleMode', {type: 'choice', choices: [
          {value: 'off', label: 'settings.subtitleMode.off'}, {value: 'forced', label: 'settings.subtitleMode.forced'}, {value: 'always', label: 'settings.subtitleMode.always'},
        ]}, {help: 'settings.help.subtitleMode'}),
        pref('playback.preferredSubtitleLanguages', 'pref.label.playback.preferredSubtitleLanguages', {type: 'languages'}, {help: 'settings.help.subtitleLanguages'}),
      ]},
      // The web player draws its own subtitles; Apple's system player styles them from the device's Accessibility settings.
      {id: 'appearance', title: 'settings.section.subtitleAppearance', platforms: ['web'], rows: [
        custom('subtitlePreview', 'settings.section.subtitleAppearance'),
        pref('playback.subtitleSize', 'settings.row.subtitleSize', {type: 'choice', choices: (['small', 'medium', 'large', 'extra-large'] as const).map(value => ({value, label: `settings.subtitleSize.${value}` as MessageId}))}),
        pref('playback.subtitleBackground', 'settings.row.subtitleBackground', {type: 'choice', choices: (['none', 'translucent', 'opaque'] as const).map(value => ({value, label: `settings.subtitleBackground.${value}` as MessageId}))}),
      ]},
    ],
  },
  {
    id: 'quality', title: 'settings.page.quality', icon: 'quality', sections: [
      // One limit per network. The registry's mode and ceiling pairs stay there for API clients.
      {id: 'limits', title: 'settings.section.qualityLimits', rows: [
        {kind: 'composite', id: 'quality.home', storage: 'registry', label: 'settings.row.qualityHome', help: 'settings.help.qualityHome', scope: 'device', control: {type: 'choice', choices: limitChoices}},
        {kind: 'composite', id: 'quality.away', storage: 'registry', label: 'settings.row.qualityAway', help: 'settings.help.qualityAway', scope: 'device', control: {type: 'choice', choices: limitChoices}},
        {kind: 'composite', id: 'quality.cellular', storage: 'registry', label: 'settings.row.qualityCellular', scope: 'device', control: {type: 'choice', choices: limitChoices}, platforms: ['phone']},
      ]},
      {id: 'method', rows: [
        // One choice in place of three four-way policies; it writes a combination that makes sense.
        {kind: 'composite', id: 'playbackMethod', storage: 'registry', label: 'settings.row.playbackMethod', help: 'settings.help.playbackMethod', scope: 'device', control: {type: 'choice', choices: [
          {value: 'automatic', label: 'settings.playbackMethod.automatic'}, {value: 'original', label: 'settings.playbackMethod.original'}, {value: 'convert', label: 'settings.playbackMethod.convert'},
        ]}},
        {kind: 'composite', id: 'hdr', storage: 'registry', label: 'settings.row.hdr', help: 'settings.help.hdr', scope: 'device', control: {type: 'choice', choices: [{value: 'automatic', label: 'settings.hdr.automatic'}, {value: 'off', label: 'settings.hdr.off'}]}},
      ]},
    ],
  },
  {
    id: 'music', title: 'settings.page.music', icon: 'music', sections: [
      {id: 'sound', rows: [
        pref('music.audioNormalization', 'settings.row.volumeLeveling', {type: 'choice', choices: (['off', 'track', 'album'] as const).map(value => ({value, label: `pref.value.music.audioNormalization.${value}` as MessageId}))}, {help: 'settings.help.volumeLeveling', needs: c => c.audioEffects !== false}),
        pref('music.crossfadeSeconds', 'pref.label.music.crossfadeSeconds', {type: 'choice', choices: [{value: 0, label: 'pref.value.music.crossfadeSeconds.0'}, ...seconds(2, 4, 6, 8, 10, 12)]}, {help: 'pref.help.music.crossfadeSeconds', needs: c => c.audioEffects !== false}),
        toggle('music.gapless', 'pref.label.music.gapless', {help: 'settings.help.gapless', needs: c => c.audioEffects !== false}),
        toggle('playback.showSyncedLyrics', 'settings.row.lyrics', {help: 'settings.help.lyrics'}),
      ]},
    ],
  },
  {
    id: 'appearance', title: 'settings.page.appearance', icon: 'sliders', sections: [
      {id: 'look', rows: [
        {kind: 'composite', id: 'cardSize', storage: 'registry', label: 'settings.row.cardSize', scope: 'device', platforms: ['web'], control: {type: 'choice', choices: [{value: 'small', label: 'settings.cardSize.small'}, {value: 'medium', label: 'settings.cardSize.medium'}, {value: 'large', label: 'settings.cardSize.large'}]}},
        toggle('appearance.showBackdrops', 'pref.label.appearance.showBackdrops', {scope: 'device', help: 'settings.help.backdrops', platforms: ['web']}),
        toggle('appearance.reduceMotion', 'pref.label.appearance.reduceMotion', {scope: 'device', help: 'pref.help.appearance.reduceMotion.web', platforms: ['web']}),
        // Apple keeps this on the device, beside the system's own Reduce Motion.
        {kind: 'composite', id: 'reduceMotion', storage: 'device', label: 'pref.label.appearance.reduceMotion', help: 'settings.help.reduceMotionDevice', scope: 'device', platforms: ['phone', 'tv'], control: {type: 'switch'}},
        {kind: 'composite', id: 'textSize', storage: 'device', label: 'settings.row.textSize', scope: 'device', platforms: ['tv'], control: {type: 'choice', choices: [{value: 'standard', label: 'settings.textSize.standard'}, {value: 'large', label: 'settings.textSize.large'}]}},
        {kind: 'composite', id: 'libraryView', storage: 'device', label: 'settings.row.libraryView', help: 'settings.help.libraryView', scope: 'device', platforms: ['web'], control: {type: 'choice', choices: [{value: 'grid', label: 'settings.libraryView.grid'}, {value: 'list', label: 'settings.libraryView.list'}]}},
        toggle('notifications.badges', 'settings.row.unreadBadge', {help: 'settings.help.unreadBadge', platforms: ['web']}),
      ]},
    ],
  },
  {
    id: 'region', title: 'settings.page.region', icon: 'globe', sections: [
      {id: 'region', rows: [
        pref('region.locale', 'settings.row.language', {type: 'locale'}),
        // No time zone: every client shows times in the device's own zone (Justin, 5 Oct 2026).
        pref('region.hourCycle', 'pref.label.region.hourCycle', {type: 'choice', choices: (['auto', 'h12', 'h23'] as const).map(value => ({value, label: `pref.value.region.hourCycle.${value}` as MessageId}))}),
      ]},
    ],
  },
  {
    id: 'privacy', title: 'settings.page.privacy', icon: 'shield', sections: [
      {id: 'history', rows: [
        toggle('privacy.pauseWatchHistory', 'pref.label.privacy.pauseWatchHistory', {help: 'pref.help.privacy.pauseWatchHistory'}),
        toggle('privacy.showActivityToMembers', 'settings.row.showActivity', {help: 'settings.help.showActivity'}),
        toggle('search.rememberHistory', 'pref.label.search.rememberHistory', {platforms: ['web']}),
      ]},
      {id: 'recommendations', rows: [{kind: 'action', id: 'resetRecommendations', label: 'settings.resetRecommendations', help: 'settings.resetRecommendations.help', icon: 'refresh'}]},
    ],
  },
  {
    id: 'device', title: 'settings.page.device', icon: 'tvDevice', sections: [
      {id: 'device', rows: [
        custom('deviceName', 'settings.row.deviceName', {needs: c => !!c.deviceName}),
        {kind: 'action', id: 'linkTV', label: 'settings.linkTV', help: 'settings.linkTVHelp', icon: 'tvDevice', platforms: ['web', 'phone']},
        {kind: 'composite', id: 'receiveFromDevices', storage: 'device', label: 'settings.row.receiveFromDevices', help: 'settings.help.receiveFromDevices', scope: 'device', platforms: ['tv'], control: {type: 'switch'}},
        custom('downloadsStorage', 'settings.row.downloadsStorage', {platforms: ['phone'], needs: c => !!c.downloads}),
      ]},
      {id: 'servers', title: 'settings.section.servers', rows: [custom('servers', 'settings.section.servers')]},
    ],
  },
  {id: 'about', title: 'settings.about', icon: 'info', sections: [{id: 'about', rows: [custom('about', 'settings.about')]}]},
];

// ── Server (owners) ─────────────────────────────────────────────────────────
// Ten pages in place of the console's fifteen sections; each duplicated concept has one home.
// Status is at the top of a page and settings below it. Plain settings are `server-setting`
// rows, written once for the web and the iPhone app; the rest are panels each client draws.
// Television shows the Dashboard only (who is watching, alerts, health).

const TV_SERVER_PAGES = new Set(['dashboard']);
const panel = (id: string, panelId: SettingsCustomId, title?: MessageId, more: Only & {tab?: string; description?: MessageId} = {}): AuthoredSection => ({id, ...(title ? {title} : {}), rows: [custom(panelId, title)], ...more});
const setting = (form: ServerFormId, key: string, label: MessageId, control: ServerControl, more: {help?: MessageId; when?: ServerSettingRow['when']} = {}): Authored<SettingsRow> => ({kind: 'server-setting', form, key, label, control, ...more});
const on = (form: ServerFormId, key: string, label: MessageId, help?: MessageId, when?: ServerSettingRow['when']) => setting(form, key, label, {type: 'switch'}, {...(help ? {help} : {}), ...(when ? {when} : {})});
const choices = (prefix: string, values: readonly string[]): SettingsChoice[] => values.map(value => ({value, label: `${prefix}.${value}` as MessageId}));
const unlimited = {stores: null, placeholder: 'settings.server.unlimited'} as const;
const noLimit = {stores: 0, placeholder: 'settings.server.unlimited'} as const;
const PAUSED_MINUTES = [0, 15, 30, 60, 120, 240];

const serverPages: readonly AuthoredPage[] = [
  {id: 'dashboard', title: 'settings.server.dashboard', icon: 'pulse', sections: [
    panel('nowPlaying', 'server.nowPlaying', 'settings.server.nowPlaying'),
    panel('alerts', 'server.alerts', 'settings.server.alerts'),
    panel('health', 'server.health', 'settings.server.health'),
    panel('activity', 'server.activity', 'settings.server.activity'),
    panel('statistics', 'server.statistics', 'settings.server.statistics', {platforms: ['web', 'phone']}),
  ]},
  // Every play on the server, with filters: the owner's record (a viewer's own History is under Saved).
  {id: 'history', title: 'settings.server.playHistory', icon: 'clock', sections: [panel('plays', 'server.playHistory')]},
  {id: 'libraries', title: 'settings.server.libraries', icon: 'library', sections: [panel('libraries', 'server.libraries')]},
  {id: 'live', title: 'settings.server.live', icon: 'live', needs: c => c.liveTV !== false,
    tabs: [{id: 'sources', title: 'settings.server.liveSources'}, {id: 'recording', title: 'settings.server.recording'}, {id: 'channels', title: 'settings.server.libraryChannels'}],
    sections: [
      panel('sources', 'server.liveSources', undefined, {tab: 'sources'}),
      {id: 'liveDefaults', tab: 'sources', title: 'web.dvrSettings.liveDefaults', description: 'web.dvrSettings.liveDefaultsLede', rows: [
        setting('liveDefaults', 'streamBufferSeconds', 'web.dvrSettings.buffer', {type: 'number', min: 0, max: 120, unit: 'settings.unit.seconds'}, {help: 'web.dvrSettings.bufferHelp'}),
        setting('liveDefaults', 'retryWindowSeconds', 'web.dvrSettings.retryWindow', {type: 'number', min: 0, max: 600, unit: 'settings.unit.seconds'}, {help: 'web.dvrSettings.retryWindowHelp'}),
        setting('liveDefaults', 'guideDays', 'web.dvrSettings.guideDays', {type: 'number', min: 1, max: 21, unit: 'settings.unit.days'}, {help: 'web.dvrSettings.guideDaysHelp'}),
        setting('liveDefaults', 'userAgent', 'web.dvrSettings.userAgent', {type: 'text', mono: true}, {help: 'web.dvrSettings.userAgentHelp'}),
        on('liveDefaults', 'logoImport', 'web.dvrSettings.logoImport'),
        on('liveDefaults', 'discoveryEnabled', 'web.dvrSettings.discovery', 'web.dvrSettings.discoveryHelp'),
      ]},
      panel('tuners', 'server.tuners', undefined, {tab: 'recording'}),
      // Who may record, and the standing rules.
      panel('recordingRules', 'server.recordingRules', undefined, {tab: 'recording'}),
      // Only what the server honours: the rows that said "not available on this server" are gone.
      {id: 'recordingDefaults', tab: 'recording', title: 'web.dvrSettings.defaults', description: 'web.dvrSettings.defaultsLede', rows: [
        setting('recording', 'prePaddingSeconds', 'web.dvr.startEarly', {type: 'number', min: 0, max: 120, unit: 'settings.unit.minutes', scale: 60}, {help: 'web.dvrSettings.startEarlyHelp'}),
        setting('recording', 'postPaddingSeconds', 'web.dvrSettings.finishLate', {type: 'number', min: 0, max: 240, unit: 'settings.unit.minutes', scale: 60}, {help: 'web.dvrSettings.finishLateHelp'}),
        setting('recording', 'keepPolicy.mode', 'web.dvr.keepLabel', {type: 'choice', choices: [{value: 'keep-all', label: 'web.logs.everything'}, {value: 'keep-count', label: 'web.dvrSettings.keepNewest'}, {value: 'keep-days', label: 'web.dvrSettings.keepDays'}]}),
        setting('recording', 'keepPolicy.keepCount', 'web.dvrSettings.howMany', {type: 'number', min: 1, max: 1000}, {when: {key: 'keepPolicy.mode', equals: 'keep-count'}}),
        setting('recording', 'keepPolicy.keepDays', 'web.dvrSettings.howManyDays', {type: 'number', min: 1, max: 3650, unit: 'settings.unit.days'}, {when: {key: 'keepPolicy.mode', equals: 'keep-days'}}),
        setting('recording', 'conversion.mode', 'web.dvrSettings.afterRecording', {type: 'choiceFrom', source: 'conversionModes'}),
        on('recording', 'conversion.deleteOriginal', 'web.dvrSettings.removeOriginal', undefined, {key: 'conversion.mode', not: 'none'}),
        setting('recording', 'folderTemplate', 'web.dvrSettings.folderShows', {type: 'text', mono: true}),
        setting('recording', 'movieFolderTemplate', 'web.dvrSettings.folderMovies', {type: 'text', mono: true}),
      ]},
      panel('recordingStorage', 'server.recordingStorage', undefined, {tab: 'recording'}),
      panel('channels', 'server.channels', undefined, {tab: 'channels'}),
    ]},
  {id: 'people', title: 'settings.server.people', icon: 'people',
    tabs: [{id: 'accounts', title: 'settings.server.accounts'}, {id: 'invitations', title: 'settings.server.invitations'}, {id: 'keys', title: 'settings.server.apiKeys'}],
    sections: [
      panel('accounts', 'server.accounts', undefined, {tab: 'accounts'}),
      panel('invitations', 'server.invitations', undefined, {tab: 'invitations'}),
      panel('apiKeys', 'server.apiKeys', undefined, {tab: 'keys'}),
    ]},
  {id: 'streaming', title: 'settings.server.playback', icon: 'transcode', sections: [
    panel('status', 'server.transcodeStatus'),
    {id: 'transcoding', title: 'settings.server.transcoding', description: 'web.transcoding.conversionLede', rows: [
      on('runtime', 'transcodingEnabled', 'settings.server.allowConverting', 'web.transcoding.allowTranscodingHelp'),
      setting('runtime', 'hardwareBackend', 'settings.server.hardware', {type: 'choice', choices: choices('settings.server.hardware', ['auto', 'software', 'videotoolbox', 'vaapi', 'qsv', 'nvenc', 'amf'])}, {help: 'settings.server.hardwareHelp'}),
      setting('runtime', 'hardwareDevice', 'settings.server.hardwareDevice', {type: 'text', mono: true, example: '/dev/dri/renderD128'}, {help: 'settings.server.hardwareDeviceHelp', when: {key: 'hardwareBackend', not: 'software'}}),
      setting('runtime', 'x264Preset', 'settings.server.encoderSpeed', {type: 'choice', choices: choices('settings.server.encoderSpeed', ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium', 'slow'])}, {help: 'settings.server.encoderSpeedHelp'}),
      on('runtime', 'hdrToneMapping', 'settings.server.toneMapping', 'settings.server.toneMappingHelp'),
      setting('runtime', 'hdrToneMappingAlgorithm', 'settings.server.toneMappingMethod', {type: 'choice', choices: choices('settings.server.toneMappingMethod', ['hable', 'mobius', 'reinhard', 'gamma', 'linear', 'clip'])}, {when: {key: 'hdrToneMapping', equals: true}}),
      on('runtime', 'directStreamRemux', 'settings.server.repackage', 'settings.server.repackageHelp'),
      setting('runtime', 'planningPolicy', 'settings.server.planning', {type: 'choice', choices: choices('settings.server.planning', ['maximum_fidelity', 'maximum_compatibility', 'minimize_server_work'])}, {help: 'settings.server.planningHelp'}),
      setting('runtime', 'temporaryDirectory', 'settings.server.tempFolder', {type: 'text', mono: true}, {help: 'settings.server.tempFolderHelp'}),
    ]},
    {id: 'capacity', title: 'web.transcoding.capacity', description: 'web.transcoding.capacityLede', rows: [
      setting('runtime', 'serverCap', 'web.transcoding.serverCap', {type: 'number', min: 1, max: 1000000, empty: unlimited}, {help: 'web.transcoding.serverCapHelp'}),
      setting('runtime', 'perAccountCap', 'web.transcoding.accountCap', {type: 'number', min: 1, max: 1000000, empty: unlimited}, {help: 'settings.server.accountCapHelp'}),
      setting('runtime', 'maxConcurrentSessions', 'settings.server.conversions', {type: 'number', min: 0, max: 64, empty: noLimit}, {help: 'settings.server.conversionsHelp'}),
      setting('runtime', 'maxHardwareSessions', 'settings.server.conversionsHardware', {type: 'number', min: 0, max: 64, empty: noLimit}),
      setting('runtime', 'maxSoftwareSessions', 'settings.server.conversionsSoftware', {type: 'number', min: 0, max: 64, empty: noLimit}),
      setting('runtime', 'maxBackgroundSessions', 'settings.server.conversionsBackground', {type: 'number', min: 0, max: 64, empty: noLimit}, {help: 'settings.server.conversionsBackgroundHelp'}),
    ]},
    // Filed by what they are for: these three sat under "Access from outside".
    {id: 'limits', title: 'settings.server.streamingLimits', rows: [
      setting('connectivity', 'remoteBitrateLimitKbps', 'web.connectivity.qualityLimit', {type: 'number', min: 0, max: 200, unit: 'settings.unit.mbps', scale: 1000, empty: noLimit}, {help: 'settings.server.qualityLimitHelp'}),
      setting('connectivity', 'uploadCapacityKbps', 'web.connectivity.uploadSpeed', {type: 'number', min: 0, max: 10000, unit: 'settings.unit.mbps', scale: 1000, empty: noLimit}, {help: 'settings.server.uploadSpeedHelp'}),
      setting('connectivity', 'pausedSessionTimeoutMinutes', 'web.connectivity.pausedLimit', {type: 'choice', choices: PAUSED_MINUTES.map(value => ({value, label: (value === 0 ? 'web.connectivity.pausedNever' : value === 60 ? 'web.connectivity.pausedHour' : value % 60 === 0 ? 'web.connectivity.pausedHours' : 'web.connectivity.pausedMinutes') as MessageId, labelValues: {count: value % 60 === 0 ? value / 60 : value}}))}, {help: 'web.connectivity.pausedLimitHelp'}),
    ]},
  ]},
  {id: 'remote', title: 'settings.server.remote', icon: 'network', sections: [
    panel('status', 'server.remoteStatus'),
    panel('account', 'server.accountConnection', 'web.general.accountConnection'),
    {id: 'reach', title: 'web.network.remoteTitle', description: 'web.network.remoteLede', rows: [
      on('remote', 'enabled', 'web.network.remoteTitle', 'web.network.remoteHelp'),
      on('remote', 'ipv6Open', 'web.network.ipv6', 'web.network.ipv6Help'),
    ]},
    {id: 'certificate', title: 'web.network.certTitle', description: 'web.network.certLede', rows: [
      custom('server.certificateStatus'),
      on('certificate', 'enabled', 'web.network.certRequest', 'web.network.certRequestHelp'),
      setting('certificate', 'publicPort', 'web.network.publicPort', {type: 'number', min: 1, max: 65535}, {when: {key: 'enabled', equals: true}}),
    ]},
    {id: 'outside', title: 'web.connectivity.outside', description: 'web.connectivity.outsideLede', rows: [
      setting('connectivity', 'remoteSignInPolicy', 'web.connectivity.whoMaySignIn', {type: 'choice', choices: choices('settings.server.signIn', ['allow', 'owner-only', 'off'])}),
      setting('connectivity', 'secureConnectionsPolicy', 'web.connectivity.encrypted', {type: 'choice', choices: choices('settings.server.secure', ['required', 'preferred', 'lan-plain-allowed'])}, {help: 'web.connectivity.encryptedHelp'}),
    ]},
    {id: 'home', title: 'settings.server.homeNetwork', rows: [
      on('connectivity', 'lanDiscoveryEnabled', 'web.connectivity.discovery', 'settings.server.discoveryHelp'),
      setting('connectivity', 'lanNetworks', 'web.connectivity.homeNetworks', {type: 'lines', example: '192.168.2.0/24'}, {help: 'web.connectivity.homeNetworksHelp'}),
      on('connectivity', 'treatWanAsLan', 'web.connectivity.wanAsLan', 'web.connectivity.wanAsLanHelp'),
      setting('connectivity', 'accessUrls', 'web.connectivity.extraAddresses', {type: 'lines', example: 'https://media.example.com'}, {help: 'web.connectivity.extraAddressesHelp'}),
      custom('server.addresses'),
    ]},
    {id: 'advanced', title: 'settings.section.advanced', advanced: true, rows: [
      on('remote', 'mapping', 'web.network.mappingTitle', 'web.network.mappingHelp'),
      on('remote', 'pcp', 'settings.server.mapping.pcp', undefined, {key: 'mapping', equals: true}),
      on('remote', 'natpmp', 'settings.server.mapping.natpmp', undefined, {key: 'mapping', equals: true}),
      on('remote', 'upnp', 'settings.server.mapping.upnp', undefined, {key: 'mapping', equals: true}),
      setting('remote', 'publicPort', 'web.network.publicPort', {type: 'number', min: 1, max: 65535}),
      setting('remote', 'gateway', 'web.network.gateway', {type: 'text', mono: true, example: '192.168.1.1'}, {help: 'web.network.gatewayHelp'}),
      setting('connectivity', 'trustedProxies', 'web.connectivity.trustedProxies', {type: 'lines', example: '127.0.0.1'}, {help: 'web.connectivity.trustedProxiesHelp'}),
      setting('connectivity', 'advertisedInterface', 'web.connectivity.interface', {type: 'choiceFrom', source: 'interfaces'}, {help: 'web.connectivity.interfaceHelp'}),
      setting('connectivity', 'customCertificateDomain', 'settings.server.customCertDomain', {type: 'text', mono: true, example: 'media.example.com'}, {help: 'web.connectivity.customCertHelp'}),
      setting('connectivity', 'customCertificatePath', 'web.connectivity.customCertPath', {type: 'text', mono: true, example: '/etc/letsencrypt/live/media.example.com/fullchain.pem'}),
      setting('connectivity', 'customCertificateKeyPath', 'web.connectivity.customCertKey', {type: 'text', mono: true, example: '/etc/letsencrypt/live/media.example.com/privkey.pem'}),
    ]},
  ]},
  {id: 'storage', title: 'settings.server.storage', icon: 'storage', sections: [
    panel('disk', 'server.diskUsage', 'settings.server.diskUsage'),
    // Retention in one place: it was split between Maintenance and Logs & diagnostics.
    {id: 'retention', title: 'settings.server.retention', description: 'settings.server.retentionLede', rows: [
      setting('runtime', 'diagnosticDays', 'web.logs.includeServer', {type: 'number', min: 1, max: 30, unit: 'settings.unit.days'}, {help: 'web.logs.serverRecordsHelp'}),
      setting('runtime', 'notificationDays', 'web.logs.retentionNotifications', {type: 'number', min: 1, max: 180, unit: 'settings.unit.days'}, {help: 'web.logs.retentionNotificationsHelp'}),
      // Kept in full unless the owner asks for less: an empty field is "forever".
      setting('runtime', 'playHistoryDays', 'settings.server.playHistoryKeep', {type: 'number', min: 1, max: 36500, unit: 'settings.unit.days', empty: {stores: 0, placeholder: 'settings.server.keepForever'}}, {help: 'settings.server.playHistoryKeepHelp'}),
      setting('runtime', 'jobDays', 'web.logs.retentionTasks', {type: 'number', min: 1, max: 30, unit: 'settings.unit.days'}, {help: 'web.logs.retentionTasksHelp'}),
      setting('maintenance', 'backupKeepCount', 'web.maintenance.backupsToKeep', {type: 'number', min: 1, max: 365}),
      custom('server.retention'),
    ]},
    panel('sources', 'server.remoteSources', 'settings.server.remoteSources'),
    panel('backups', 'server.backups', 'settings.server.backups'),
    panel('deleted', 'server.deletedTitles', 'settings.server.deletedTitles'),
  ]},
  {id: 'schedule', title: 'settings.server.schedule', icon: 'calendar', sections: [
    panel('jobs', 'server.jobs', 'settings.server.jobs'),
    panel('windows', 'server.windows', 'settings.server.windows', {description: 'settings.server.windowsLede'}),
    {id: 'priority', rows: [
      setting('maintenance', 'backgroundTaskPriority', 'web.maintenance.backgroundPriority', {type: 'choice', choices: [{value: 'lower', label: 'web.maintenance.backgroundLower'}, {value: 'normal', label: 'web.maintenance.backgroundNormal'}]}, {help: 'web.maintenance.backgroundPriorityHelp'}),
    ]},
  ]},
  {id: 'troubleshooting', title: 'settings.server.troubleshooting', icon: 'wrench', sections: [
    // One control for more detail, in place of three.
    {id: 'detail', rows: [custom('server.detailWindow', 'settings.server.detailWindow')]},
    panel('logs', 'server.logs', 'settings.server.logs'),
    panel('records', 'server.records', 'settings.server.records'),
    panel('capabilities', 'server.capabilities', 'settings.server.capabilities'),
    panel('export', 'server.supportExport', 'settings.server.supportExport'),
    panel('feedback', 'server.feedback', 'settings.server.feedback'),
    panel('state', 'server.stateFolder'),
  ]},
  {id: 'general', title: 'settings.server.general', icon: 'server', sections: [
    {id: 'identity', title: 'web.general.identity', rows: [
      setting('runtime', 'name', 'web.general.serverName', {type: 'text', maxLength: 100}, {help: 'web.general.serverNameHelp'}),
      custom('server.identity'),
    ]},
    panel('updates', 'server.updates', 'settings.server.updates'),
    {id: 'alerts', title: 'settings.server.alertThresholds', description: 'web.alerts.lede', rows: [
      setting('alerts', 'storageWarningPercent', 'web.alerts.storageWarning', {type: 'number', min: 1, max: 50, unit: 'settings.unit.percent'}, {help: 'web.alerts.storageWarningHelp'}),
      setting('alerts', 'storageCriticalPercent', 'web.alerts.storageCritical', {type: 'number', min: 1, max: 50, unit: 'settings.unit.percent'}, {help: 'web.alerts.storageCriticalHelp'}),
      setting('alerts', 'certificateWarningDays', 'web.alerts.certificateWarning', {type: 'number', min: 1, max: 60, unit: 'settings.unit.days'}, {help: 'web.alerts.certificateWarningHelp'}),
    ]},
  ]},
];

// ── The structure ────────────────────────────────────────────────────────────

function allowed(item: Only, context: SettingsContext): boolean {
  return (!item.platforms || item.platforms.includes(context.platform)) && (!item.needs || item.needs(context.capabilities));
}

function strip<T extends Only>(item: T): Omit<T, 'platforms' | 'needs'> {
  const {platforms: _platforms, needs: _needs, ...rest} = item;
  return rest;
}

function resolve(pages: readonly AuthoredPage[], context: SettingsContext): SettingsPage[] {
  const keys = context.capabilities.preferenceKeys;
  const out: SettingsPage[] = [];
  for (const authored of pages) {
    if (!allowed(authored, context)) continue;
    const sections: SettingsSection[] = [];
    for (const section of authored.sections) {
      if (!allowed(section, context)) continue;
      const rows = section.rows.filter(row => allowed(row, context) && (row.kind !== 'preference' || !keys || keys.has(row.key))).map(row => strip(row) as SettingsRow);
      if (rows.length) sections.push({id: section.id, ...(section.title ? {title: section.title} : {}), ...(section.description ? {description: section.description} : {}), ...(section.advanced ? {advanced: true} : {}), ...(section.tab ? {tab: section.tab} : {}), rows});
    }
    const tabs = authored.tabs?.filter(tab => sections.some(section => section.tab === tab.id));
    if (sections.length) out.push({id: authored.id, title: authored.title, icon: authored.icon, ...(tabs && tabs.length > 1 ? {tabs} : {}), sections});
  }
  return out;
}

/** The Settings screen for this viewer on this device: Account for everyone, Server for owners. */
export function settingsStructure(context: SettingsContext): SettingsStructure {
  const headings: SettingsHeading[] = [{id: 'account', title: 'settings.heading.account', pages: resolve(accountPages, context)}];
  if (context.owner) {
    const pages = resolve(context.platform === 'tv' ? serverPages.filter(p => TV_SERVER_PAGES.has(p.id)) : serverPages, context);
    if (pages.length) headings.push({id: 'server', title: 'settings.heading.server', pages});
  }
  return Object.freeze({headings});
}

/** The page an address names (`/settings/<id>`), or the first page. Server pages are addressed as `server-<id>`. */
export function settingsPageId(heading: SettingsHeadingId, pageId: string): string {
  return heading === 'server' ? `server-${pageId}` : pageId;
}

export function findSettingsPage(structure: SettingsStructure, address: string | undefined): Readonly<{heading: SettingsHeading; page: SettingsPage}> | undefined {
  for (const heading of structure.headings) for (const p of heading.pages) if (settingsPageId(heading.id, p.id) === address) return {heading, page: p};
  const heading = structure.headings[0];
  return heading?.pages[0] ? {heading, page: heading.pages[0]} : undefined;
}

/** A row's stable identity within its page (for keys, anchors and search). */
export function settingsRowId(row: SettingsRow): string {
  return row.kind === 'preference' ? row.key : row.kind === 'server-setting' ? `${row.form}.${row.key}` : row.id;
}

/** The forms a page's rows are bound to: what its one Save covers besides the panels' own forms. */
export function serverFormsOf(page: SettingsPage): readonly ServerFormId[] {
  const out = new Set<ServerFormId>();
  for (const section of page.sections) for (const row of section.rows) if (row.kind === 'server-setting') out.add(row.form);
  return [...out];
}

/** The choices of a `choiceFrom` control, from what the server sent with the form. */
export function serverChoices(source: 'interfaces' | 'conversionModes', status: unknown, t: (id: MessageId, values?: MessageValues) => string, current?: string): readonly Readonly<{value: string; label: string}>[] {
  if (source === 'interfaces') {
    const interfaces = ((status as {interfaces?: readonly {name: string; up: boolean; loopback: boolean; virtual: boolean; addresses: readonly string[]}[]} | undefined)?.interfaces ?? []).filter(i => i.up && !i.loopback && i.addresses.length);
    return [
      {value: '', label: t('web.connectivity.interfaceAuto')},
      ...interfaces.map(i => ({value: i.name, label: t(i.virtual ? 'web.connectivity.interfaceVirtual' : 'web.connectivity.interfaceOption', {name: i.name, address: i.addresses[0]!.split('/')[0]!})})),
      // An interface that has gone away stays selectable as what it was, so saving another row does not silently change it.
      ...(current && !interfaces.some(i => i.name === current) ? [{value: current, label: current}] : []),
    ];
  }
  const modes = (status as {enumerations?: Readonly<Record<string, readonly string[]>>} | undefined)?.enumerations?.conversionMode ?? ['none'];
  return modes.map(value => ({value, label: t(value === 'none' ? 'web.dvrSettings.leaveRecorded' : value === 'remux' ? 'web.dvrSettings.repackage' : 'web.dvrSettings.convertSmaller')}));
}

/** Whether a server-setting row is shown for this draft: its value was sent by the server and its condition holds. */
export function serverRowShown(row: ServerSettingRow, value: (key: string) => unknown): boolean {
  if (value(row.key) === undefined) return false;
  if (!row.when) return true;
  const other = value(row.when.key);
  return 'equals' in row.when ? other === row.when.equals : other !== row.when.not;
}

export type SettingsMatch = Readonly<{address: string; pageTitle: string; rowId: string; label: string}>;

/** Rows whose words contain the query, across every page: the search field above the page list. */
export function searchSettings(structure: SettingsStructure, query: string, t: (id: MessageId, values?: MessageValues) => string, limit = 12): readonly SettingsMatch[] {
  const needle = query.trim().toLowerCase();
  if (needle.length < 2) return [];
  const out: SettingsMatch[] = [];
  for (const heading of structure.headings) {
    for (const p of heading.pages) {
      const pageTitle = t(p.title);
      for (const section of p.sections) {
        for (const row of section.rows) {
          if (!('label' in row) || !row.label) continue;
          const label = t(row.label);
          const help = 'help' in row && row.help ? t(row.help) : '';
          const choices = row.kind !== 'custom' && row.kind !== 'action' && row.control.type === 'choice' ? row.control.choices.map(c => t(c.label, c.labelValues)).join(' ') : '';
          if (`${label} ${help} ${choices} ${section.title ? t(section.title) : ''}`.toLowerCase().includes(needle)) out.push({address: settingsPageId(heading.id, p.id), pageTitle, rowId: settingsRowId(row), label});
          if (out.length >= limit) return out;
        }
      }
      if (pageTitle.toLowerCase().includes(needle) && !out.some(m => m.address === settingsPageId(heading.id, p.id))) out.push({address: settingsPageId(heading.id, p.id), pageTitle, rowId: '', label: pageTitle});
      if (out.length >= limit) return out;
    }
  }
  return out;
}

// ── Composite rows: one decision, several keys ───────────────────────────────

type Read = (key: string) => PreferenceValue | undefined;
const NETWORKS = ['local', 'wifi', 'cellular', 'unknown'] as const;
/** The registry networks behind each limit row: "Away from home" is Wi-Fi elsewhere and any network the device cannot name. */
const LIMIT_NETWORKS: Readonly<Record<string, readonly string[]>> = {'quality.home': ['local'], 'quality.away': ['wifi', 'unknown'], 'quality.cellular': ['cellular']};
const CARD_SIZE: Readonly<Record<string, number>> = {small: 85, medium: 100, large: 125};
const METHODS: Readonly<Record<string, Readonly<Record<string, string>>>> = {
  automatic: {'delivery.directPlay': 'prefer', 'delivery.directStream': 'allow', 'delivery.transcode': 'allow'},
  original: {'delivery.directPlay': 'require', 'delivery.directStream': 'never', 'delivery.transcode': 'never'},
  convert: {'delivery.directPlay': 'never', 'delivery.directStream': 'never', 'delivery.transcode': 'require'},
};

/** The tallest picture one network lane allows as the server reads it (`delivery_policy.go`): Infinity for original files. */
function laneLimit(read: Read, network: string): number {
  const mode = read(`quality.${network}.mode`);
  if (mode === 'original') return Infinity;
  const mbps = Number(read(`quality.${network}.maxVideoBitrateMbps`) ?? 0);
  const preset = mbps >= 20 ? 2160 : mbps >= 8 ? 1080 : mbps >= 2 ? 720 : mbps >= 1 ? 480 : 4320;
  const byMode = mode === 'standard' ? 1080 : mode === 'data-saver' || mode === 'off' ? 480 : 4320;
  return Math.min(Number(read(`quality.${network}.maxVideoHeight`) || 4320), preset, byMode);
}

/** Composites whose value lives in the registry (the rest are the device's own). */
export function registryComposite(id: CompositeId): boolean {
  return id === 'playbackMethod' || id === 'hdr' || id === 'cardSize' || id in LIMIT_NETWORKS;
}

/**
 * The choice a registry composite shows for the stored keys. A combination only an API client
 * could have written reads as the nearest choice a person can make, never as a fourth option.
 */
export function readComposite(id: CompositeId, read: Read): string | boolean {
  const networks = LIMIT_NETWORKS[id];
  if (networks) {
    // Several networks behind one row read as the strictest of them, so the row never overstates.
    const limit = Math.min(...networks.map(network => laneLimit(read, network)));
    if (limit === Infinity) return 'original';
    if (limit >= 4320) return 'automatic';
    return limit >= 2160 ? '2160' : limit >= 1080 ? '1080' : limit >= 720 ? '720' : '480';
  }
  switch (id) {
    case 'playbackMethod': {
      if (read('delivery.directPlay') === 'never') return 'convert';
      if (read('delivery.transcode') === 'never') return 'original';
      return 'automatic';
    }
    case 'hdr': return NETWORKS.some(n => read(`quality.${n}.allowHDR`) !== false) ? 'automatic' : 'off';
    case 'cardSize': {
      const percent = Number(read('appearance.cardSizePercent') ?? 100);
      return percent < 90 ? 'small' : percent > 115 ? 'large' : 'medium';
    }
    default: return '';
  }
}

/** The keys a choice writes: one consistent combination, never a contradictory one. */
export function compositePatch(id: CompositeId, choice: string | boolean): PreferencePatch {
  const networks = LIMIT_NETWORKS[id];
  if (networks) {
    const patch: Record<string, PreferenceValue> = {};
    for (const network of networks) {
      patch[`quality.${network}.mode`] = choice === 'original' ? 'original' : 'automatic';
      patch[`quality.${network}.maxVideoHeight`] = choice === 'automatic' || choice === 'original' ? 4320 : Number(choice);
      // The height is the limit; a bit rate ceiling left behind would quietly lower it again.
      patch[`quality.${network}.maxVideoBitrateMbps`] = 0;
      patch[`quality.${network}.maxAudioBitrateKbps`] = 0;
    }
    return patch;
  }
  switch (id) {
    case 'playbackMethod': return {...(METHODS[String(choice)] ?? METHODS.automatic!)};
    case 'hdr': return Object.fromEntries(NETWORKS.map(n => [`quality.${n}.allowHDR`, choice !== 'off']));
    case 'cardSize': return {'appearance.cardSizePercent': CARD_SIZE[String(choice)] ?? 100};
    default: return {};
  }
}
