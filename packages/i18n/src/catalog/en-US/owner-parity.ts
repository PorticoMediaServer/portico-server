/**
 * en-US copy for owner tools that reached every client: moving someone's playback to a point,
 * the title a remembered playback problem belongs to, whose a tuner is, a source's own import
 * limits, the subtitle provider's account, what an administrator may do, and merging titles on
 * a phone. Merged into `enUS` statically.
 */
export const ownerParityMessages = {
  // ── Watching now: a position bar for playback on another device ──
  'web.nowPlaying.positionOf': 'At {position} of {duration}',
  'web.nowPlaying.moveTo': 'Move to…',
  'web.nowPlaying.moveTitle': 'Move “{title}”',
  'web.nowPlaying.moveBody': 'Playback on {viewer}’s device jumps to the point you choose.',
  'web.nowPlaying.moveConfirm': 'Move here',
  // ── Streaming › Playback problems ──
  'server.routes.unknownTitle': 'A file no longer in a library',
  'server.routes.openTitle': 'Open the title',
  // ── Live TV › Recording › Tuners ──
  'server.tuners.whoseProfile': '{profile} ({account})',
  // ── Live TV › a source › Import limits ──
  'server.liveSource.limits': 'Import limits for this source',
  'server.liveSource.limitsHelp': 'Lower than the server’s limits for this source alone. Leave a field empty to use the server’s.',
  'server.liveSource.limitDefault': 'Server: {value}',
  // ── Metadata › Subtitles found online ──
  'server.metadata.subtitles': 'Subtitles found online',
  'server.metadata.subtitlesLede': 'Portico looks for subtitles at OpenSubtitles when you ask it to. Use your own API key and account for more searches and downloads a day.',
  'server.metadata.openSubtitlesKey': 'OpenSubtitles API key',
  'server.metadata.openSubtitlesUser': 'OpenSubtitles username',
  'server.metadata.openSubtitlesPassword': 'OpenSubtitles password',
  'server.metadata.openSubtitlesKeyOwn': 'Your own key is in use',
  'server.metadata.openSubtitlesKeyServer': 'The server’s own key is in use, if it has one',
  'server.metadata.openSubtitlesPasswordSaved': 'A password is saved',
  'server.metadata.openSubtitlesUseServer': 'Use the server’s key',
  'server.metadata.openSubtitlesSignOut': 'Remove the account',
  // ── Profiles: forgetting the devices that remember one ──
  'profile.devicesForgotten': 'Devices that remembered {name} will ask for its PIN again.',
  // ── Merging titles from a phone ──
  'title.merge.find': 'Title to merge with',
  'title.merge.none': 'No other title matches.',
} as const;
