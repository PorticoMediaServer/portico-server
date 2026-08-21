export type SeekResult = 'completed' | 'superseded' | 'timed-out';

export type SeekTransaction = {
  seek(targetSeconds: number): Promise<SeekResult>;
  cancel(): void;
};

function clampToSeekable(media: HTMLMediaElement, targetSeconds: number): number {
  const duration = Number.isFinite(media.duration) ? media.duration : Number.POSITIVE_INFINITY;
  let target = Math.max(0, Math.min(duration, targetSeconds));
  if (media.seekable.length === 0) return target;
  for (let index = 0; index < media.seekable.length; index += 1) {
    const start = media.seekable.start(index);
    const end = media.seekable.end(index);
    if (target >= start && target <= end) return target;
  }
  const first = media.seekable.start(0);
  const last = media.seekable.end(media.seekable.length - 1);
  target = Math.max(first, Math.min(last, target));
  return target;
}

/** Owns exactly one bounded seek. A newer seek fences every callback from the old one. */
export function createSeekTransaction(media: HTMLMediaElement, timeoutMs = 8_000): SeekTransaction {
  let generation = 0;
  let cleanup: (() => void) | undefined;
  const cancel = () => {
    generation += 1;
    cleanup?.();
    cleanup = undefined;
  };
  return {
    cancel,
    seek(targetSeconds) {
      cancel();
      const transaction = generation;
      const shouldResume = !media.paused;
      media.pause();
      media.currentTime = clampToSeekable(media, targetSeconds);
      return new Promise<SeekResult>((resolve) => {
        let settled = false;
        const finish = (result: SeekResult) => {
          if (settled) return;
          settled = true;
          media.removeEventListener('seeked', onSeeked);
          media.removeEventListener('canplay', onCanPlay);
          window.clearTimeout(timer);
          if (cleanup === dispose) cleanup = undefined;
          if (result === 'completed' && generation === transaction && shouldResume) void media.play().catch(() => undefined);
          resolve(result);
        };
        const onSeeked = () => finish(generation === transaction ? 'completed' : 'superseded');
        const onCanPlay = () => {
          if (!media.seeking) onSeeked();
        };
        const timer = window.setTimeout(() => finish(generation === transaction ? 'timed-out' : 'superseded'), timeoutMs);
        const dispose = () => finish('superseded');
        cleanup = dispose;
        media.addEventListener('seeked', onSeeked, { once: true });
        media.addEventListener('canplay', onCanPlay);
        if (!media.seeking) queueMicrotask(onSeeked);
      });
    },
  };
}

export type StallReason = 'paused' | 'seeking' | 'background' | 'buffered' | 'stalled';

export function playbackStallReason(media: HTMLMediaElement, documentHidden: boolean, bufferedSeconds: number): StallReason {
  if (media.paused) return 'paused';
  if (media.seeking) return 'seeking';
  if (documentHidden) return 'background';
  if (media.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA && bufferedSeconds >= 1.5) return 'buffered';
  return 'stalled';
}
