/**
 * A channel start the server can't proceed with (occurrence `status: "recoverable"`,
 * `linear.media.errorCode`, be/playback 68cc6c36) in plain words. Unknown codes get the general
 * line; a code never reaches the screen.
 */
export function channelProblemId(code: string | undefined): string {
  switch (code) {
    case 'channel_start_failed': return 'web.channelProblem.startFailed';
    case 'source_unavailable': return 'web.channelProblem.sourceUnavailable';
    case 'timeshift_storage_unavailable': return 'web.channelProblem.storage';
    case 'channel_preparation_stalled': return 'web.channelProblem.stalled';
    case 'capacity-unavailable': case 'capacity_unavailable': return 'web.channelProblem.tunersBusy';
    default: return 'web.channelProblem.general';
  }
}
