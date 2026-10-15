/**
 * en-US messages for subtitles the player draws itself (styled and image subtitles) and for the
 * "up next" card shown during the credits. Merged into `enUS` statically like `web.ts`.
 */
export const drawnSubtitleMessages = {
  'player.subtitles.position': 'Position',
  'player.subtitles.positionHelp': 'Move subtitles up or down the picture',
  'player.subtitles.moveUp': 'Up',
  'player.subtitles.moveDown': 'Down',
  'player.subtitles.positionReset': 'Reset',
  'player.subtitles.positionNotice': 'Subtitles moved {direction}',
  'player.subtitles.positionUp': 'up',
  'player.subtitles.positionDown': 'down',
  'player.subtitles.positionBack': 'Subtitles back where they were made to sit',
  'player.subtitles.loadingDrawn': 'Loading subtitles…',
  'player.subtitles.drawFailed': 'These subtitles could not be drawn on this device, so they are being shown another way.',
  'player.subtitles.drawUnavailable': 'These subtitles could not be shown.',
  'player.subtitles.forcedOnly': 'Forced only',
  'player.subtitles.forcedOnlyHelp': 'Show only the lines marked forced: foreign dialogue and signs',
  'player.upNext.duringCredits': 'Up next',
  'player.upNext.playNow': 'Play now',
  'player.upNext.watchCredits': 'Watch credits',
  'player.upNext.notNow': 'Not now',
  'player.upNext.startsIn': 'Starts in {time}',
} as const satisfies Record<string, string>;
