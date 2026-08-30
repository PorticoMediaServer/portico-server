import type { PlaybackPreparedResponse } from '@porticomediaserver/client-core';

type ActivePreparation = {
  key: string;
  controller: AbortController;
  promise: Promise<PlaybackPreparedResponse>;
};

/** The single lifecycle authority for preparing a successor to one playback session. */
export class PlaybackPreparationOwner {
  private active?: ActivePreparation;

  prepare(
    key: string,
    start: (signal: AbortSignal) => Promise<PlaybackPreparedResponse>,
    replace = false,
  ): Promise<PlaybackPreparedResponse> {
    if (!replace && this.active?.key === key) return this.active.promise;
    this.active?.controller.abort();
    const controller = new AbortController();
    const promise = start(controller.signal);
    this.active = { key, controller, promise };
    return promise;
  }

  owns(promise: Promise<PlaybackPreparedResponse>): boolean {
    return this.active?.promise === promise;
  }

  preserveSession(sessionId: string) {
    if (!this.active) return;
    try {
      const key = JSON.parse(this.active.key) as { sessionId?: unknown };
      if (key.sessionId === sessionId) return;
    } catch { /* malformed/non-session keys are never preserved */ }
    this.cancel();
  }

  cancel(clear = true) {
    this.active?.controller.abort();
    if (clear) this.active = undefined;
  }
}

// PlayerSurface can be remounted while its authenticated DataProvider and the
// underlying playback session remain current. Keep preparation authority at
// that provider/module lifetime so a new bridge instance consumes the same
// per-session/candidate promise and settled result.
export const playbackPreparationOwner = new PlaybackPreparationOwner();
