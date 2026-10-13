/**
 * Keeps a title that is starting paused until it is ready. The player sets a start to "playing"
 * as it begins; a group that is paused wants the opposite, and wants it before the first frame,
 * not as a correction after it.
 */
export function holdPaused(service: {getSnapshot(): {itemId?: string | null; phase: string; intent: string}; subscribe(listener: () => void): () => void; pause(): void}, itemId: string): void {
  let off: () => void = () => {};
  const hold = () => {
    const now = service.getSnapshot();
    // Another title, or this one has finished starting (or failed): the hold is over.
    if ((now.itemId ?? '') !== itemId && now.phase !== 'idle') { off(); return; }
    if (now.phase === 'starting' && now.intent === 'playing') service.pause();
    if (now.phase === 'ready' || now.phase === 'error' || now.phase === 'ended') off();
  };
  off = service.subscribe(hold);
  hold();
  // A start that never begins is not waited on for ever.
  setTimeout(() => off(), 60000);
}
