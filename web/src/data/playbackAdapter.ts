import {
  createPlaybackProgressSequenceCoordinator,
  normalizePlaybackResponse,
  type PlaybackPreparedResponse,
  type PlaybackProgressEvent,
  type PlaybackProgressInput,
  type PlaybackRestoreResponse,
  type PlaybackResponse,
} from '@porticomediaserver/client-core';

/**
 * Owns the Web adapter's session-protocol mapping and ordered progress state.
 * PlayerSurface owns lifecycle/UI decisions; this boundary owns the server's
 * normalized PlaybackResponse shape and the event sequence associated with a
 * session. It deliberately does not mint, copy, or rewrite playback grants.
 */
export interface PlaybackSessionAdapter {
  acceptPlayback(response: PlaybackResponse): PlaybackResponse;
  acceptPreparedPlayback(response: PlaybackPreparedResponse): PlaybackPreparedResponse;
  acceptRestorePlayback(response: PlaybackRestoreResponse): PlaybackRestoreResponse;
  normalizePlayback(response: PlaybackResponse): PlaybackResponse;
  orderProgress(sessionId: string, event: PlaybackProgressInput): PlaybackProgressEvent;
  releaseSession(sessionId: string): void;
}

export function createPlaybackSessionAdapter(): PlaybackSessionAdapter {
  const progressSequences = createPlaybackProgressSequenceCoordinator();

  const normalize = (response: PlaybackResponse) => normalizePlaybackResponse(response);
  const acceptPlayback = (response: PlaybackResponse) => {
    const playback = normalize(response);
    progressSequences.seed(playback.sessionId, playback.nextEventSequence);
    return playback;
  };

  return {
    acceptPlayback,
    acceptPreparedPlayback: (response) => ({ ...response, playback: acceptPlayback(response.playback) }),
    acceptRestorePlayback: (response) => ({ ...response, playback: response.playback ? acceptPlayback(response.playback) : undefined }),
    normalizePlayback: normalize,
    orderProgress: (sessionId, event) => progressSequences.ordered(sessionId, event),
    releaseSession: (sessionId) => progressSequences.forget(sessionId),
  };
}
