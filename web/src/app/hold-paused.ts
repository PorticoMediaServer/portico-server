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
    off?.();
    if (deadline !== undefined) { clearTimeout(deadline); deadline = undefined; }
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
  // A service that answered during subscribe released before its own unsubscribe was known.
  if (!live) stop();
  hold();
  // A start that never begins is not waited on for ever.
  if (live) deadline = setTimeout(release, 60000);
}
