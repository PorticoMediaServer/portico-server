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
} as const;
