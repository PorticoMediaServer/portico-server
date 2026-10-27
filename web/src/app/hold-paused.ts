/**
 * Keeps a title that is starting paused until it is ready. The player sets a start to "playing"
 * as it begins; a group that is paused wants the opposite, and wants it before the first frame,
 * not as a correction after it.
 */
export function holdPaused(service: {getSnapshot(): {itemId?: string | null; phase: string; intent: string}; subscribe(listener: () => void): () => void; pause(): void}, itemId: string): void {
  let off: (() => void) | undefined;
  let live = true;
  // The deadline belongs to the hold: once the hold ends it is cleared, so a finished start never
  // leaves a minute-long timer behind in the process.
  let deadline: ReturnType<typeof setTimeout> | undefined;
  const release = () => {
    if (!live) return;
    live = false;
    const stop = off;
    off = undefined;
    // The deadline is cleared even when the service's unsubscribe throws: the hold is over either way.
    try { stop?.(); } finally {
      if (deadline !== undefined) { clearTimeout(deadline); deadline = undefined; }
    }
  };
  const hold = () => {
    const now = service.getSnapshot();
    // Another title, or this one has finished starting (or failed): the hold is over.
    if ((now.itemId ?? '') !== itemId && now.phase !== 'idle') { release(); return; }
    if (now.phase === 'starting' && now.intent === 'playing') service.pause();
    if (now.phase === 'ready' || now.phase === 'error' || now.phase === 'ended') release();
  };
  const stop = service.subscribe(hold);
  off = stop;
  if (!live) {
    // The service answered during subscribe and the hold has already ended. Its unsubscribe is
    // known only now, and the retired hold must not read or pause a start that followed it.
    stop();
  } else {
    hold();
    // A start that never begins is not waited on for ever.
    if (live) deadline = setTimeout(release, 60000);
  }
}
