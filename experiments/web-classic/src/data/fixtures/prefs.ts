import type {PrefDef, PrefValue} from '../types';

const secs = (list: number[]) => list.map(v => ({value: v, label: `${v} seconds`}));
const skip = [{value: 'ask', label: 'Ask each time'}, {value: 'auto', label: 'Skip automatically'}, {value: 'off', label: 'Never'}];
const quality = [{value: 'automatic', label: 'Automatic'}, {value: 'original', label: 'Original quality'}, {value: 'high', label: 'High'}, {value: 'standard', label: 'Standard'}, {value: 'data-saver', label: 'Data saver'}];
const res = [{value: '0', label: 'No limit'}, {value: '2160', label: '4K'}, {value: '1080', label: '1080p'}, {value: '720', label: '720p'}, {value: '480', label: '480p'}];

/** The preference registry, as the server publishes it (groups mirror Portico's). */
export const prefDefs: PrefDef[] = [
  {key: 'playback.skipBackSeconds', group: 'Playback', label: 'Skip back', help: 'How far the back button rewinds.', type: 'choice', options: secs([5, 10, 15, 30]), scope: 'profile'},
  {key: 'playback.skipForwardSeconds', group: 'Playback', label: 'Skip forward', help: 'How far the forward button jumps.', type: 'choice', options: secs([10, 15, 30, 60]), scope: 'profile'},
  {key: 'playback.autoplayNext', group: 'Playback', label: 'Play next automatically', help: 'Start the next episode when one ends.', type: 'toggle', scope: 'profile'},
  {key: 'playback.upNextCountdownSeconds', group: 'Playback', label: 'Up Next countdown', help: 'How long the next episode waits before starting.', type: 'choice', options: [{value: 0, label: 'Immediately'}, ...secs([5, 10, 15, 30])], scope: 'profile'},
  {key: 'playback.passoutProtection', group: 'Playback', label: 'Still watching check', help: 'Pause autoplay after several episodes with no one using the remote.', type: 'toggle', scope: 'profile'},
  {key: 'playback.passoutAfterEpisodes', group: 'Playback', label: 'Ask after', help: 'Episodes in a row before Portico checks you’re still watching.', type: 'choice', options: [2, 3, 4, 5, 6].map(v => ({value: v, label: `${v} episodes`})), scope: 'profile'},
  {key: 'playback.introSkip', group: 'Playback', label: 'Intros', help: 'Skip detected intros.', type: 'choice', options: skip, scope: 'profile'},
  {key: 'playback.creditsSkip', group: 'Playback', label: 'Credits', help: 'Skip detected credits.', type: 'choice', options: skip, scope: 'profile'},
  {key: 'playback.recapSkip', group: 'Playback', label: 'Recaps', help: 'Skip detected recaps.', type: 'choice', options: skip, scope: 'profile'},
  {key: 'playback.defaultSpeed', group: 'Playback', label: 'Default speed', type: 'choice', options: [0.75, 1, 1.25, 1.5, 2].map(v => ({value: v, label: v === 1 ? 'Normal' : `${v}×`})), scope: 'profile'},
  {key: 'playback.startedThresholdPercent', group: 'Playback', label: 'Counts as started', help: 'How much you watch before a title joins Continue Watching.', type: 'choice', options: [1, 3, 5, 10].map(v => ({value: v, label: `${v}%`})), scope: 'profile'},
  {key: 'playback.playedThresholdPercent', group: 'Playback', label: 'Counts as watched', help: 'How much you watch before a title is marked watched.', type: 'choice', options: [85, 90, 95].map(v => ({value: v, label: `${v}%`})), scope: 'profile'},
  {key: 'playback.preferredAudioLanguages', group: 'Languages', label: 'Preferred audio languages', help: 'In order of preference.', type: 'languages', scope: 'profile'},
  {key: 'playback.preferredSubtitleLanguages', group: 'Languages', label: 'Preferred subtitle languages', help: 'In order of preference.', type: 'languages', scope: 'profile'},
  {key: 'playback.subtitlesEnabled', group: 'Subtitles', label: 'Subtitles on by default', type: 'toggle', scope: 'profile'},
  {key: 'playback.subtitleSize', group: 'Subtitles', label: 'Subtitle size', type: 'choice', options: [{value: 'small', label: 'Small'}, {value: 'medium', label: 'Medium'}, {value: 'large', label: 'Large'}, {value: 'extra-large', label: 'Extra large'}], scope: 'profile'},
  {key: 'playback.subtitleBackground', group: 'Subtitles', label: 'Subtitle background', type: 'choice', options: [{value: 'none', label: 'None'}, {value: 'shadow', label: 'Drop shadow'}, {value: 'box', label: 'Dark box'}], scope: 'profile'},
  {key: 'playback.showSyncedLyrics', group: 'Music', label: 'Show synced lyrics', type: 'toggle', scope: 'profile'},
  {key: 'delivery.directPlay', group: 'Streaming', label: 'Play original files', help: 'Play the original file when this device can.', type: 'toggle', scope: 'device'},
  {key: 'delivery.directStream', group: 'Streaming', label: 'Repackage when needed', help: 'Change only the file’s container, keeping full quality, when that’s enough to play it.', type: 'toggle', scope: 'device'},
  {key: 'delivery.transcode', group: 'Streaming', label: 'Allow conversion', help: 'Let your server convert video that this device can’t play.', type: 'toggle', scope: 'device'},
  {key: 'quality.local', group: 'Quality', label: 'Home network quality', type: 'choice', options: quality, scope: 'device'},
  {key: 'quality.localResolution', group: 'Quality', label: 'Home network: highest resolution', type: 'choice', options: res, scope: 'device'},
  {key: 'quality.localHDR', group: 'Quality', label: 'Allow HDR on home network', type: 'toggle', scope: 'device'},
  {key: 'quality.wifi', group: 'Quality', label: 'Wi-Fi quality', type: 'choice', options: quality, scope: 'device'},
  {key: 'quality.cellular', group: 'Quality', label: 'Cellular quality', type: 'choice', options: quality, scope: 'device'},
  {key: 'quality.remote', group: 'Quality', label: 'Away from home quality', type: 'choice', options: quality, scope: 'device'},
  {key: 'music.audioNormalization', group: 'Music', label: 'Volume normalization', type: 'choice', options: [{value: 'off', label: 'Off'}, {value: 'track', label: 'Per song'}, {value: 'album', label: 'Per album'}], scope: 'profile'},
  {key: 'music.crossfadeSeconds', group: 'Music', label: 'Crossfade', help: 'Overlap between songs.', type: 'choice', options: [{value: 0, label: 'Off'}, ...secs([2, 4, 6, 8])], scope: 'profile'},
  {key: 'music.gapless', group: 'Music', label: 'Gapless playback', type: 'toggle', scope: 'profile'},
  {key: 'music.shuffleDefault', group: 'Music', label: 'Shuffle by default', type: 'toggle', scope: 'profile'},
  {key: 'music.repeatDefault', group: 'Music', label: 'Repeat by default', type: 'choice', options: [{value: 'off', label: 'Off'}, {value: 'one', label: 'One song'}, {value: 'all', label: 'All'}], scope: 'profile'},
  {key: 'privacy.pauseWatchHistory', group: 'Privacy', label: 'Pause watch history', help: 'Nothing new is recorded while paused. Existing progress still resumes.', type: 'toggle', scope: 'profile'},
  {key: 'privacy.showActivityToMembers', group: 'Privacy', label: 'Show my activity to others on this server', type: 'toggle', scope: 'profile'},
  {key: 'privacy.includeInWatchTogether', group: 'Privacy', label: 'Include me in Watch Together activity', type: 'toggle', scope: 'profile'},
  {key: 'search.rememberHistory', group: 'Search', label: 'Remember recent searches', type: 'toggle', scope: 'profile'},
  {key: 'region.locale', group: 'Language and region', label: 'Language and region', type: 'choice', options: [{value: 'en-US', label: 'English (United States)'}, {value: 'en-CA', label: 'English (Canada)'}], scope: 'profile'},
  {key: 'region.hourCycle', group: 'Language and region', label: 'Clock', type: 'choice', options: [{value: 'auto', label: 'Follow system'}, {value: 'h12', label: '12-hour'}, {value: 'h23', label: '24-hour'}], scope: 'profile'},
  {key: 'appearance.showBackdrops', group: 'Appearance', label: 'Show backdrops', type: 'toggle', scope: 'device'},
  {key: 'appearance.cardSizePercent', group: 'Appearance', label: 'Card size', help: 'Relative to the standard poster size.', type: 'choice', options: [85, 100, 115, 130].map(v => ({value: v, label: `${v}%`})), scope: 'device'},
  {key: 'appearance.reduceMotion', group: 'Appearance', label: 'Reduce motion', help: 'Shorter transitions and no zoom on focus.', type: 'toggle', scope: 'device'},
  {key: 'appearance.receiveFromDevices', group: 'Appearance', label: 'Play from your other devices', help: 'Phones on this profile can send what they’re watching to this TV.', type: 'toggle', scope: 'device'},
  {key: 'notifications.badges', group: 'Notifications', label: 'Show unread badge', type: 'toggle', scope: 'profile'},
];

export const defaultPrefs: Record<string, PrefValue> = {
  'playback.skipBackSeconds': 10, 'playback.skipForwardSeconds': 30, 'playback.autoplayNext': true, 'playback.upNextCountdownSeconds': 10,
  'playback.passoutProtection': true, 'playback.passoutAfterEpisodes': 3, 'playback.introSkip': 'ask', 'playback.creditsSkip': 'ask', 'playback.recapSkip': 'auto',
  'playback.defaultSpeed': 1, 'playback.startedThresholdPercent': 3, 'playback.playedThresholdPercent': 90,
  'playback.preferredAudioLanguages': ['en', 'fr'], 'playback.preferredSubtitleLanguages': ['en'], 'playback.subtitlesEnabled': false, 'playback.subtitleSize': 'medium', 'playback.subtitleBackground': 'shadow', 'playback.showSyncedLyrics': true,
  'delivery.directPlay': true, 'delivery.directStream': true, 'delivery.transcode': true,
  'quality.local': 'original', 'quality.localResolution': '0', 'quality.localHDR': true, 'quality.wifi': 'automatic', 'quality.cellular': 'standard', 'quality.remote': 'high',
  'music.audioNormalization': 'album', 'music.crossfadeSeconds': 0, 'music.gapless': true, 'music.shuffleDefault': false, 'music.repeatDefault': 'off',
  'privacy.pauseWatchHistory': false, 'privacy.showActivityToMembers': true, 'privacy.includeInWatchTogether': true, 'search.rememberHistory': true,
  'region.locale': 'en-US', 'region.hourCycle': 'auto', 'appearance.showBackdrops': true, 'appearance.cardSizePercent': 100, 'appearance.reduceMotion': false, 'appearance.receiveFromDevices': true, 'notifications.badges': true,
};

export const languageNames: Record<string, string> = {en: 'English', fr: 'French', de: 'German', es: 'Spanish', it: 'Italian', ja: 'Japanese', pt: 'Portuguese', nl: 'Dutch', sv: 'Swedish', ko: 'Korean', zh: 'Chinese'};
