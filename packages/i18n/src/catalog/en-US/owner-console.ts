/**
 * en-US copy for the owner console's later screens (Server pages of Settings): transport on
 * Now Playing and on this profile's other devices, playback routes the server avoids, and the
 * shared words of console lists. Merged into `enUS` statically.
 */
export const ownerConsoleMessages = {
  // ── Now Playing: pause, resume and move someone's playback ──
  'web.nowPlaying.controls': 'Controls for {title}',
  'web.nowPlaying.position': 'At {position}',
  'error.sessionEnded': 'That playback has already ended.',
  // ── Settings › This device › Playing on your other devices ──
  'remote.moreOn': 'More controls on {device}',
  // ── People: devices and invitations ──
  'device.platform.api': 'API key or token',
  'server.people.blockTitle': 'Block “{name}”?',
  'server.people.blockBody': 'It is signed out at once and can’t use this server until you approve it again.',
  'server.people.deviceApproved': 'Device approved.',
  'server.people.deviceBlocked': 'Device blocked.',
  'server.people.revokeInvitationBody': 'The link and the code stop working at once.',
  'server.people.invitationRevoked': 'Invitation revoked.',
  'server.people.inviteCode': 'Or give them the code on its own: {code}',
  'server.people.accountAdded': 'Account added.',
  // ── Storage & backups: remote sources ──
  'server.row.actions': 'Actions for {name}',
  'server.storage.saved': 'Saved.',
  'server.storage.added': 'Added.',
  'server.storage.state.mounted': 'Mounted',
  'server.storage.state.stopped': 'Not mounted',
  'server.storage.state.starting': 'Mounting',
  'server.storage.state.stopping': 'Unmounting',
  'server.storage.state.failed': 'Couldn’t mount',
  'server.storage.state.quarantined': 'Stopped after repeated failures',
  'server.storage.state.unavailable': 'Unavailable',
  'server.storage.state.ready': 'Connected',
  'server.storage.state.checking': 'Checking',
  'server.storage.state.unreachable': 'Can’t be reached',
  'server.storage.state.credentials': 'Needs its password again',
  'server.storage.state.problem': 'Needs attention',
  // ── Storage & backups: confirmations and results ──
  'server.confirm.word.delete': 'delete',
  'server.backups.deleteBody': 'The backup file is removed from this server. This cannot be undone.',
  'server.backups.restoreWhich': 'Backup from {when}',
  'server.cleanup.body': '{canRecreate, select, yes {These files are rebuilt when next needed, which costs the server some work.} other {These files cannot be recreated.}}',
  'server.cleanup.done': 'Cleaned up.',
  'server.missing.removeBody': 'This removes {count, plural, one {# title} other {# titles}} from the library. Nothing on disk is touched, and watch history is kept in case they come back.',
  // ── What a console list is loading ("Loading {thing}") ──
  'server.thing.apiKeys': 'API keys',
  'server.thing.tokens': 'access tokens',
  'server.thing.webhooks': 'webhooks',
  'server.thing.invitations': 'invitations',
  'server.thing.devices': 'devices',
  'server.thing.accounts': 'accounts',
  'server.thing.reports': 'reports',
  'server.thing.remoteStorage': 'remote storage',
  'server.thing.backups': 'backups',
  'server.thing.liveSources': 'Live TV sources',
  'server.thing.tuners': 'tuners',
  'server.thing.recordingRules': 'recording rules',
  'server.thing.libraryChannels': 'Library Channels',
  'server.thing.usage': 'what is in use',
  'server.thing.downloads': 'downloads',
  'server.thing.diskUsage': 'disk usage',
  'server.thing.missingTitles': 'missing titles',
  'server.thing.storagePlaces': 'storage folders',
  'server.thing.avoidedRoutes': 'playback problems',
} as const;
